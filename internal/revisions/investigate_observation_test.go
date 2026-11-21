package revisions_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
	"github.com/slashobserve/slashobserve/internal/observations"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const rootID = "66666666-6666-4666-8666-666666666666"

var root = auth.Principal{Kind: auth.KindUser, ID: rootID, Name: "Root", Role: auth.RoleAdmin}

type runObservations struct {
	service *observations.Service
}

func (r runObservations) OpenForRun(ctx context.Context, q *dbsqlc.Queries, in revisions.RunObservationOpen) (pgtype.UUID, error) {
	id, err := r.service.OpenForRun(ctx, q, observations.RunOpen{Run: in.Run, By: in.By, ObservationID: in.ObservationID, Strict: in.Strict})
	if errors.Is(err, observations.ErrNotFound) {
		return pgtype.UUID{}, revisions.ErrObservationNotFound
	}
	return id, err
}

type refusingObservations struct{}

func (refusingObservations) OpenForRun(context.Context, *dbsqlc.Queries, revisions.RunObservationOpen) (pgtype.UUID, error) {
	return pgtype.UUID{}, folders.ErrForbidden
}

type observationRig struct {
	svc          *revisions.Service
	d            *dbtest.Database
	observations *observations.Service
}

func newObservationRig(t *testing.T) observationRig {
	t.Helper()
	d := dbtest.New(t)
	obs := observations.New(d.DB, folders.NewAccess(d.DB), nil, nil, quietLogger())
	svc := newTestService(d.DB, succeedWith(2), quietLogger(), revisions.WithObservations(runObservations{obs}))
	return observationRig{svc: svc, d: d, observations: obs}
}

func (r observationRig) observation(t *testing.T, by auth.Principal, folder pgtype.UUID) observations.Observation {
	t.Helper()
	o, err := r.observations.Create(reading(t.Context()), by, observations.CreateInput{FolderID: folder, Title: "Checkout 5xx"})
	if err != nil {
		t.Fatalf("create observation: %v", err)
	}
	return o
}

func (r observationRig) count(t *testing.T, table string) int {
	t.Helper()
	var n int
	if err := r.d.SQLDB(t).QueryRowContext(reading(t.Context()), `SELECT count(*) FROM `+table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (r observationRig) fetch(t *testing.T, by auth.Principal, id pgtype.UUID) observations.Observation {
	t.Helper()
	o, err := r.observations.Get(reading(t.Context()), by, id)
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func TestAnInvestigationOpensAnObservationWithItsRunBlock(t *testing.T) {
	r := newObservationRig(t)
	dash := seedDashboard(t, r.d, 1)
	row := investigate(t, r.svc, revisions.InvestigateParams{DashboardID: dash, Prompt: "why are checkout errors up"})
	if !row.ObservationID.Valid {
		t.Fatal("the run names no observation")
	}
	o := r.fetch(t, user, row.ObservationID)
	if o.Title != "why are checkout errors up" || o.RunID != row.ID.String() || len(o.Blocks) != 1 || o.Blocks[0].Kind != observations.KindRun || o.Blocks[0].State != observations.BlockLive {
		t.Errorf("observation %+v", o)
	}
	stored, err := r.svc.GetByID(reading(t.Context()), row.ID)
	if err != nil || stored.ObservationID != row.ObservationID {
		t.Errorf("stored run names %v, %v", stored.ObservationID, err)
	}
}

func TestTheRunsJSONCarriesItsObservationId(t *testing.T) {
	r := newObservationRig(t)
	router := observeRouterAs(r.svc, user)
	rec := call(router, http.MethodPost, "/api/v1/observe", `{"prompt":"why","kind":"investigate","from":"now-2h","to":"now"}`)
	var created struct {
		Request struct {
			ID            string  `json:"id"`
			ObservationID *string `json:"observation_id"`
		} `json:"request"`
	}
	if rec.Code != http.StatusAccepted || json.Unmarshal(rec.Body.Bytes(), &created) != nil || created.Request.ObservationID == nil {
		t.Fatalf("POST: %d %s", rec.Code, rec.Body)
	}
	for _, path := range []string{"/api/v1/observe/" + created.Request.ID, "/api/v1/observe"} {
		rec := call(router, http.MethodGet, path, "")
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"observation_id":"`+*created.Request.ObservationID+`"`) {
			t.Errorf("GET %s: %d %s", path, rec.Code, rec.Body)
		}
	}
	build := call(router, http.MethodPost, "/api/v1/observe", `{"prompt":"a dashboard"}`)
	if build.Code != http.StatusAccepted || !strings.Contains(build.Body.String(), `"observation_id":null`) {
		t.Errorf("a build: %d %s", build.Code, build.Body)
	}
}

func TestAnInvestigationWritesIntoTheNamedObservation(t *testing.T) {
	r := newObservationRig(t)
	named := r.observation(t, user, pgtype.UUID{})
	router := observeRouterAs(r.svc, user)
	rec := call(router, http.MethodPost, "/api/v1/observe", `{"prompt":"why","kind":"investigate","observation_id":"`+named.ID+`"}`)
	if rec.Code != http.StatusAccepted || !strings.Contains(rec.Body.String(), `"observation_id":"`+named.ID+`"`) {
		t.Fatalf("POST: %d %s", rec.Code, rec.Body)
	}
	got := r.fetch(t, user, mustUUID(t, named.ID))
	if len(got.Blocks) != 1 || got.Blocks[0].Kind != observations.KindRun || r.count(t, "observations") != 1 {
		t.Errorf("blocks %+v", got.Blocks)
	}
}

func TestObservationIdChecksBeforeAnythingIsEnqueued(t *testing.T) {
	r := newObservationRig(t)
	hidden := foldertest.Restricted(t, r.d, "Payroll", "payroll-team")
	secret := r.observation(t, root, hidden)
	router := observeRouterAs(r.svc, user)
	cases := []struct {
		name, body string
		status     int
		code       string
	}{
		{"an observation the caller cannot see", `{"prompt":"why","kind":"investigate","observation_id":"` + secret.ID + `"}`, http.StatusNotFound, "not_found"},
		{"an observation that does not exist", `{"prompt":"why","kind":"investigate","observation_id":"00000000-0000-4000-8000-000000000000"}`, http.StatusNotFound, "not_found"},
		{"a value that is not a UUID", `{"prompt":"why","kind":"investigate","observation_id":"nope"}`, http.StatusBadRequest, "invalid_request"},
		{"a private ask about it", `{"prompt":"why","observation_id":"` + secret.ID + `"}`, http.StatusNotFound, "not_found"},
		{"a revise", `{"prompt":"why","kind":"revise","observation_id":"` + secret.ID + `"}`, http.StatusBadRequest, "invalid_request"},
	}
	for _, c := range cases {
		rec := call(router, http.MethodPost, "/api/v1/observe", c.body)
		if rec.Code != c.status || errorCode(t, rec) != c.code {
			t.Errorf("%s: %d %s, want %d %s", c.name, rec.Code, rec.Body, c.status, c.code)
		}
	}
	if n := r.count(t, "agent_requests"); n != 0 {
		t.Errorf("%d requests were enqueued", n)
	}
	if rec := call(router, http.MethodPost, "/api/v1/observe", `{"prompt":"why","kind":"revise","dashboard_id":"00000000-0000-4000-8000-000000000000","observation_id":"x"}`); !strings.Contains(rec.Body.String(), "observation_id is not allowed for kind revise") {
		t.Errorf("message: %s", rec.Body)
	}
}

func TestARefusedObservationAnswers403AndEnqueuesNothing(t *testing.T) {
	d := dbtest.New(t)
	svc := newTestService(d.DB, succeedWith(2), quietLogger(), revisions.WithObservations(refusingObservations{}))
	_, err := svc.SubmitInvestigate(reading(t.Context()), revisions.InvestigateParams{Prompt: "why", By: user})
	if !errors.Is(err, revisions.ErrForbidden) {
		t.Fatalf("err = %v, want the forbidden error", err)
	}
	var n int
	if err := d.SQLDB(t).QueryRowContext(reading(t.Context()), `SELECT count(*) FROM agent_requests`).Scan(&n); err != nil || n != 0 {
		t.Errorf("%d requests, %v", n, err)
	}
}

func TestRetryOfAnInvestigationReusesItsObservation(t *testing.T) {
	r := newObservationRig(t)
	dash := seedDashboard(t, r.d, 1)
	named := r.observation(t, user, pgtype.UUID{})
	first := investigate(t, r.svc, revisions.InvestigateParams{DashboardID: dash, ObservationID: mustUUID(t, named.ID)})
	if _, err := r.svc.Cancel(reading(t.Context()), first.ID, user); err != nil {
		t.Fatal(err)
	}
	next, err := r.svc.Retry(reading(t.Context()), first.ID, user)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if next.ObservationID != first.ObservationID || next.ID == first.ID {
		t.Fatalf("retry names %v, want %v", next.ObservationID, first.ObservationID)
	}
	got := r.fetch(t, user, mustUUID(t, named.ID))
	if len(got.Blocks) != 2 || got.Blocks[1].RunID != next.ID.String() {
		t.Errorf("blocks %+v", got.Blocks)
	}
}

func TestRetryByAnotherPersonFallsBackToANewObservation(t *testing.T) {
	r := newObservationRig(t)
	dash := seedDashboard(t, r.d, 1)
	hidden := foldertest.Restricted(t, r.d, "Payroll", "payroll-team")
	secret := r.observation(t, root, hidden)
	first := investigate(t, r.svc, revisions.InvestigateParams{DashboardID: dash, ObservationID: mustUUID(t, secret.ID), By: root})
	if first.ObservationID != mustUUID(t, secret.ID) {
		t.Fatalf("first run names %v", first.ObservationID)
	}
	if _, err := r.svc.Cancel(reading(t.Context()), first.ID, root); err != nil {
		t.Fatal(err)
	}
	next, err := r.svc.Retry(reading(t.Context()), first.ID, user)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if !next.ObservationID.Valid || next.ObservationID == first.ObservationID {
		t.Errorf("retry names %v, want a new observation", next.ObservationID)
	}
}

func TestWithoutObservationsAnInvestigationNamesNone(t *testing.T) {
	d := dbtest.New(t)
	svc := newTestService(d.DB, succeedWith(2), quietLogger())
	row := investigate(t, svc, revisions.InvestigateParams{DashboardID: seedDashboard(t, d, 1)})
	if row.ObservationID.Valid {
		t.Errorf("the run names %v", row.ObservationID)
	}
	rec := call(observeRouterAs(svc, user), http.MethodPost, "/api/v1/observe", `{"prompt":"why","kind":"investigate"}`)
	if rec.Code != http.StatusAccepted || !strings.Contains(rec.Body.String(), `"observation_id":null`) {
		t.Errorf("POST: %d %s", rec.Code, rec.Body)
	}
}

func claimedSteps(t *testing.T, svc *revisions.Service, id pgtype.UUID) []string {
	t.Helper()
	claimed, found := claimAs(t, svc, builtinClaimant)
	if !found || claimed.Row.ID != id {
		t.Fatalf("claim = %v found %v, want %v", claimed.Row.ID, found, id)
	}
	var stored []revisions.Step
	if err := json.Unmarshal(storedRun(t, svc, id.String()).Steps, &stored); err != nil {
		t.Fatal(err)
	}
	messages := make([]string, 0, len(stored))
	for _, s := range stored {
		messages = append(messages, s.Message)
	}
	return messages
}

func TestAnInvestigationWithNoObservationSaysTheAnswerStaysOnTheRun(t *testing.T) {
	cases := []struct {
		name  string
		start func(r observationRig) pgtype.UUID
	}{
		{"no observation was opened", func(r observationRig) pgtype.UUID {
			return investigate(t, newTestService(r.d.DB, succeedWith(2), quietLogger()), revisions.InvestigateParams{}).ID
		}},
		{"the named observation is full", func(r observationRig) pgtype.UUID {
			full := r.observation(t, user, pgtype.UUID{})
			for range observations.MaxBlocks {
				if _, err := r.observations.AddBlock(reading(t.Context()), user, mustUUID(t, full.ID), observations.BlockInput{Kind: observations.KindText, Body: json.RawMessage(`{"markdown":"x"}`)}); err != nil {
					t.Fatal(err)
				}
			}
			row := investigate(t, r.svc, revisions.InvestigateParams{DashboardID: seedDashboard(t, r.d, 1), ObservationID: mustUUID(t, full.ID)})
			if row.ObservationID.Valid {
				t.Fatalf("the run names %v", row.ObservationID)
			}
			return row.ID
		}},
		{"its observation was deleted before the claim", func(r observationRig) pgtype.UUID {
			row := investigate(t, r.svc, revisions.InvestigateParams{})
			r.d.Exec(t, "DELETE FROM observations WHERE id = $1", row.ObservationID)
			return row.ID
		}},
	}
	for _, c := range cases {
		r := newObservationRig(t)
		got := claimedSteps(t, r.svc, c.start(r))
		if len(got) != 1 || got[0] != "No observation to write into; the answer stays on this run" {
			t.Errorf("%s: steps %q", c.name, got)
		}
	}
}

func TestAClaimedRunWithSomewhereToWriteRecordsNoStep(t *testing.T) {
	cases := []struct {
		name  string
		start func(r observationRig, dash pgtype.UUID) pgtype.UUID
	}{
		{"an investigation with its observation", func(r observationRig, dash pgtype.UUID) pgtype.UUID {
			return investigate(t, r.svc, revisions.InvestigateParams{DashboardID: dash}).ID
		}},
		{"a private ask about a dashboard", func(r observationRig, dash pgtype.UUID) pgtype.UUID {
			first, err := r.svc.Start(reading(t.Context()), user, revisions.StartRequest{Prompt: text("why"), DashboardID: text(dash.String())})
			if err != nil {
				t.Fatal(err)
			}
			return first.ID
		}},
	}
	for _, c := range cases {
		r := newObservationRig(t)
		id := c.start(r, seedDashboard(t, r.d, 1))
		if got := claimedSteps(t, r.svc, id); len(got) != 0 {
			t.Errorf("%s: steps %q", c.name, got)
		}
	}
}

func TestARetriedPrivateAskStaysPrivateOnItsSubjectForWhoeverMayAskIt(t *testing.T) {
	r := newObservationRig(t)
	cy := auth.Principal{Kind: auth.KindUser, ID: strangerID, Name: "Cy", Role: auth.RoleCommenter}
	for _, p := range []auth.Principal{user, cy} {
		r.d.Exec(t, `INSERT INTO users (id, oidc_issuer, oidc_subject, email, name) VALUES ($1::uuid, 'https://idp', $2, $3, $2)`, p.ID, p.Name, p.Name+"@example.com")
	}
	dash := seedDashboard(t, r.d, 1)
	about := r.observation(t, user, pgtype.UUID{})
	cases := []struct {
		name string
		by   auth.Principal
		req  revisions.StartRequest
	}{
		{"an Editor's ask about a dashboard", user, revisions.StartRequest{Prompt: text("why"), DashboardID: text(dash.String())}},
		{"an Editor's ask about an observation", user, revisions.StartRequest{Prompt: text("why"), ObservationID: text(about.ID)}},
		{"a Commenter's ask about a dashboard", cy, revisions.StartRequest{Prompt: text("why"), DashboardID: text(dash.String())}},
	}
	for _, c := range cases {
		first, err := r.svc.Start(reading(t.Context()), c.by, c.req)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		r.d.Exec(t, `UPDATE agent_requests SET state = 'failed', status = 'failed', failure_code = 'agent_error', error = 'x', finished_at = clock_timestamp() WHERE id = $1`, first.ID)
		observations := r.count(t, "observations")
		next, err := r.svc.Retry(reading(t.Context()), first.ID, c.by)
		if err != nil {
			t.Fatalf("%s: retry: %v", c.name, err)
		}
		if !revisions.PrivateAsk(next) || next.ChatID != first.ChatID || next.DashboardID != first.DashboardID || next.SubjectObservationID != first.SubjectObservationID || next.ObservationID.Valid {
			t.Errorf("%s: retry = chat %v dashboard %v subject %v observation %v, want a private ask in chat %v on dashboard %v subject %v",
				c.name, next.ChatID, next.DashboardID, next.SubjectObservationID, next.ObservationID, first.ChatID, first.DashboardID, first.SubjectObservationID)
		}
		if got := r.count(t, "observations"); got != observations {
			t.Errorf("%s: observations = %d after the retry, want %d", c.name, got, observations)
		}
	}
}
