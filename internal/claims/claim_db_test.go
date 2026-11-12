package claims

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/opaquetoken"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/internal/threads"
)

const (
	userID       = "11111111-1111-4111-8111-111111111111"
	grantID      = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	otherGrantID = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	instanceID   = "server-1"
)

var (
	ada          = auth.Principal{Kind: auth.KindUser, ID: userID, Name: "Ada", Email: "ada@example.com", Role: auth.RoleEditor}
	grantClaimer = Claimer{Kind: revisions.AssignGrant, ID: grantID, Person: userID, Client: "Claude Code"}

	builtinClaimer = Claimer{Kind: revisions.AssignBuiltin, ID: instanceID}
	modelSettings  = ModelSettings{Provider: "anthropic", Model: "claude-x", APIKey: "sk-test-key", MaxTokens: 4096, SessionRenewBytes: 262144}
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

type principals struct {
	gone atomic.Bool
}

func (p *principals) Principal(_ context.Context, kind auth.PrincipalKind, id string) (auth.Principal, error) {
	if p.gone.Load() {
		return auth.Principal{}, auth.ErrPrincipalGone
	}
	return auth.Principal{Kind: kind, ID: id, Name: ada.Name, Email: ada.Email, Role: auth.RoleEditor}, nil
}

type fixture struct {
	svc        *Service
	runs       *revisions.Service
	d          *dbtest.Database
	principals *principals
	dashboard  pgtype.UUID
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	d := dbtest.New(t)
	logger := slog.New(slog.DiscardHandler)
	people := &principals{}
	runs := revisions.NewService(d.DB, noReviser{}, logger,
		revisions.WithPrincipals(people), revisions.WithAccess(folders.NewAccess(d.DB)), revisions.WithThreads(threads.NewRuns(d.DB)))
	svc := New(Config{
		DB: d.DB, Runs: runs, Listen: d.Listen,
		PublicURL: &url.URL{Scheme: "https", Host: "so.example.com"}, InternalURL: &url.URL{Scheme: "http", Host: "127.0.0.1:8080"},
		InstanceID: instanceID, Model: func() *ModelSettings { settings := modelSettings; return &settings },
		RunTimeout: time.Hour, Logger: logger,
	})
	return &fixture{svc: svc, runs: runs, d: d, principals: people, dashboard: seedDashboard(t, d)}
}

type noReviser struct{}

func (noReviser) Revise(context.Context, revisions.Request) (revisions.Result, error) {
	return revisions.Result{}, nil
}

func mustUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	id, err := db.ParseUUID(s)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func seedDashboard(t *testing.T, d *dbtest.Database) pgtype.UUID {
	t.Helper()
	ctx := context.Background()
	author := mustUUID(t, userID)
	row, err := d.Queries().CreateDashboard(ctx, dbsqlc.CreateDashboardParams{
		Title: "t", Tags: []string{}, CreatedByKind: "user", CreatedByID: author, CreatedByName: "Ada", CreatedVia: "direct",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = d.Queries().InsertDashboardVersion(ctx, dbsqlc.InsertDashboardVersionParams{
		DashboardID: row.ID, Version: 1, Spec: []byte(`{}`), Summary: "s", AuthorKind: "user", AuthorID: author, AuthorName: "Ada",
	})
	if err != nil {
		t.Fatal(err)
	}
	d.Exec(t, "UPDATE dashboards SET latest_version = 1 WHERE id = $1", row.ID)
	return row.ID
}

func (f *fixture) queue(t *testing.T, kind revisions.AssigneeKind, assignee string) dbsqlc.AgentRequest {
	t.Helper()
	row, err := f.runs.Submit(context.Background(), revisions.EnqueueParams{
		DashboardID: f.dashboard, Source: revisions.SourcePrompt, Prompt: "add a latency panel", BaseVersion: 1, By: ada,
	})
	if err != nil {
		t.Fatal(err)
	}
	f.d.Exec(t, "UPDATE agent_requests SET assigned_kind = $2, assigned_id = $3 WHERE id = $1", row.ID, string(kind), mustUUID(t, assignee))
	return row
}

func (f *fixture) claim(t *testing.T, c Claimer) Offer {
	t.Helper()
	offer, err := f.svc.Claim(context.Background(), c, 0)
	if err != nil {
		t.Fatalf("claim as %s: %v", c.claimant().Key(), err)
	}
	if offer.Run == nil {
		t.Fatalf("claim as %s found no run", c.claimant().Key())
	}
	return offer
}

func (f *fixture) stored(t *testing.T, id pgtype.UUID) dbsqlc.AgentRequest {
	t.Helper()
	row, err := f.runs.GetByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func (f *fixture) tokenCount(t *testing.T, id pgtype.UUID) int {
	t.Helper()
	var n int
	if err := f.d.SQLDB(t).QueryRow("SELECT count(*) FROM run_tokens WHERE agent_request_id = $1", id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestGrantClaimerGetsItsAssignedRunWithARunToken(t *testing.T) {
	f := newFixture(t)
	queued := f.queue(t, revisions.AssignGrant, grantID)

	offer := f.claim(t, grantClaimer)

	if offer.Run.ID != queued.ID.String() || offer.Run.Attempt != 1 || offer.Run.Kind != "build" || offer.Run.RequestedBy.ID != userID {
		t.Fatalf("offered run = %+v", offer.Run)
	}
	if !opaquetoken.Shaped(offer.Session.Token, "sot_") || offer.Session.URL != "https://so.example.com/mcp" {
		t.Fatalf("session = %+v", offer.Session)
	}
	if offer.LeaseSeconds != 60 || offer.SystemPrompt == nil || offer.SystemPrompt.Name != "observe" || offer.SystemPrompt.Arguments["kind"] != "build" {
		t.Fatalf("lease %d, system prompt %+v", offer.LeaseSeconds, offer.SystemPrompt)
	}
	var runID pgtype.UUID
	var attempt int
	err := f.d.SQLDB(t).QueryRow("SELECT agent_request_id, attempt FROM run_tokens WHERE token_hash = $1", opaquetoken.Hash(offer.Session.Token)).Scan(&runID, &attempt)
	if err != nil {
		t.Fatalf("the token hash is not stored: %v", err)
	}
	if runID != queued.ID || attempt != 1 {
		t.Fatalf("stored token = run %v attempt %d, want %v attempt 1", runID, attempt, queued.ID)
	}
}

func TestClaimerLeavesRunsAssignedToAnotherAgent(t *testing.T) {
	f := newFixture(t)
	f.queue(t, revisions.AssignGrant, otherGrantID)

	offer, err := f.svc.Claim(context.Background(), grantClaimer, 0)
	if err != nil {
		t.Fatal(err)
	}
	if offer.Run != nil {
		t.Fatalf("claimed a run assigned to another grant: %+v", offer.Run)
	}
}

func TestServiceAccountTakesRunsOnlyWhenItIsAWorker(t *testing.T) {
	f := newFixture(t)
	account, err := f.d.Queries().CreateServiceAccount(context.Background(), dbsqlc.CreateServiceAccountParams{Name: "ci-agent", Role: "editor"})
	if err != nil {
		t.Fatal(err)
	}
	claimer := Claimer{Kind: revisions.AssignServiceAccount, ID: account.ID.String(), Client: "ci"}
	queued := f.queue(t, revisions.AssignServiceAccount, claimer.ID)

	if _, err := f.svc.Claim(context.Background(), claimer, 0); !errors.Is(err, ErrNotAWorker) {
		t.Fatalf("claim with takes_runs false = %v, want ErrNotAWorker", err)
	}
	f.d.Exec(t, "UPDATE service_accounts SET takes_runs = true WHERE id = $1", account.ID)

	if offer := f.claim(t, claimer); offer.Run.ID != queued.ID.String() {
		t.Fatalf("service account got run %s, want %s", offer.Run.ID, queued.ID)
	}
}

func TestClaimRefusesCredentialsThatAreNotWorkers(t *testing.T) {
	f := newFixture(t)
	for name, c := range map[string]Claimer{
		"built-in of another server": {Kind: revisions.AssignBuiltin, ID: "server-2"},
		"built-in without an id":     {Kind: revisions.AssignBuiltin},
		"unknown kind":               {Kind: "other", ID: grantID},
		"service account not found":  {Kind: revisions.AssignServiceAccount, ID: grantID},
		"service account bad id":     {Kind: revisions.AssignServiceAccount, ID: "not-a-uuid"},
	} {
		if _, err := f.svc.Claim(context.Background(), c, 0); !errors.Is(err, ErrNotAWorker) {
			t.Errorf("%s: claim = %v, want ErrNotAWorker", name, err)
		}
	}
}

func TestClaimWithNothingQueuedAnswersAtOnceWithARetryHint(t *testing.T) {
	f := newFixture(t)

	started := time.Now()
	offer, err := f.svc.Claim(context.Background(), grantClaimer, 0)
	if err != nil {
		t.Fatal(err)
	}
	if offer.Run != nil || offer.Session != nil || offer.RetryAfterSeconds != 5 {
		t.Fatalf("offer = %+v, want no run and retry after 5", offer)
	}
	if time.Since(started) > time.Second {
		t.Fatalf("a zero wait took %v", time.Since(started))
	}
}

func TestLongPollReturnsARunQueuedWhileItWaits(t *testing.T) {
	f := newFixture(t)
	f.svc.poll = 50 * time.Millisecond
	type result struct {
		offer Offer
		err   error
	}
	answered := make(chan result, 1)
	started := time.Now()
	go func() {
		offer, err := f.svc.Claim(context.Background(), grantClaimer, 2*time.Second)
		answered <- result{offer, err}
	}()
	time.Sleep(200 * time.Millisecond)
	row := f.queue(t, revisions.AssignGrant, grantID)

	got := <-answered
	if got.err != nil {
		t.Fatal(got.err)
	}
	if got.offer.Run == nil || got.offer.Run.ID != row.ID.String() {
		t.Fatalf("offer = %+v, want run %s", got.offer, row.ID)
	}
	if elapsed := time.Since(started); elapsed > 1500*time.Millisecond {
		t.Fatalf("long poll returned after %v, want shortly after the run was queued", elapsed)
	}
}

func TestLongPollEndsAtItsDeadlineWithNoRun(t *testing.T) {
	f := newFixture(t)
	f.svc.poll = 50 * time.Millisecond

	offer, err := f.svc.Claim(context.Background(), grantClaimer, 300*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if offer.Run != nil || offer.RetryAfterSeconds != 0 {
		t.Fatalf("offer = %+v, want no run and no retry hint", offer)
	}
	if f.svc.hub.total != 0 {
		t.Fatalf("%d waiter slots are still held", f.svc.hub.total)
	}
}

func TestLongPollStopsWhenItsContextEnds(t *testing.T) {
	f := newFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	if _, err := f.svc.Claim(ctx, grantClaimer, 10*time.Second); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("claim error = %v, want the context's deadline", err)
	}
	if f.svc.hub.total != 0 {
		t.Fatalf("%d waiter slots are still held", f.svc.hub.total)
	}
}

func TestClaimRecordsTheWorkerPresence(t *testing.T) {
	f := newFixture(t)
	if _, err := f.svc.Claim(context.Background(), grantClaimer, 0); err != nil {
		t.Fatal(err)
	}

	var kind, client string
	var principal pgtype.UUID
	err := f.d.SQLDB(t).QueryRow("SELECT principal_kind, principal_id, client FROM agent_workers WHERE id = $1", "grant:"+grantID).Scan(&kind, &principal, &client)
	if err != nil {
		t.Fatalf("no agent_workers row for the claimer: %v", err)
	}
	if kind != "grant" || principal != mustUUID(t, grantID) || client != "Claude Code" {
		t.Fatalf("worker = kind %s principal %v client %q", kind, principal, client)
	}
}
