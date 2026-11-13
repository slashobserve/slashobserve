package runsession

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/url"
	"os"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/claims"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/imports"
	"github.com/slashobserve/slashobserve/internal/llm/pricing"
	"github.com/slashobserve/slashobserve/internal/origin"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/internal/threads"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	userID  = "11111111-1111-4111-8111-111111111111"
	grantID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"

	proposalID = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"

	dashboardJSON = `{"specVersion":"0","title":"t","time":{"from":"now-15m","to":"now"},"panels":[{"id":"r1","type":"row","title":"R1","gridPos":{"x":0,"y":0,"w":24,"h":1},"panels":[{"id":"a","type":"stat","title":"A","gridPos":{"x":0,"y":1,"w":6,"h":4},"datasource":"prom","queries":[{"query":"up"}]}]}]}`
)

var (
	ada           = auth.Principal{Kind: auth.KindUser, ID: userID, Name: "Ada", Email: "ada@example.com", Role: auth.RoleEditor}
	grantClaimant = revisions.Claimant{Kind: revisions.AssignGrant, ID: grantID, Person: userID}
	builtin       = revisions.Claimant{Kind: revisions.AssignBuiltin}
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

type people struct{}

func (people) Principal(_ context.Context, kind auth.PrincipalKind, id string) (auth.Principal, error) {
	return auth.Principal{Kind: kind, ID: id, Name: ada.Name, Email: ada.Email, Role: auth.RoleEditor}, nil
}

type panels struct{}

func (panels) HasPanel(context.Context, pgtype.UUID, int, string) (bool, error) { return true, nil }

type stepLog struct {
	mu    sync.Mutex
	steps []revisions.Step
}

func (l *stepLog) RecordStep(_ context.Context, _ revisions.Request, st revisions.Step) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.steps = append(l.steps, st)
}

func (l *stepLog) all() []revisions.Step {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.steps)
}

func (l *stepLog) messages() []string {
	var out []string
	for _, st := range l.all() {
		out = append(out, st.Message)
	}
	return out
}

type store struct {
	mu       sync.Mutex
	t        testing.TB
	d        *dbtest.Database
	dash     pgtype.UUID
	latest   int
	build    *dashboards.Build
	rebaseOK bool

	proposeCommits bool
	proposalID     string
	commitErr      error
	commits        int
	commitOrigin   origin.Origin
	rebases        int
	discards       int
}

func (s *store) parse() *spec.Dashboard {
	parsed, err := spec.Parse([]byte(dashboardJSON))
	if err != nil {
		panic(err)
	}
	return parsed
}

func (s *store) Get(_ context.Context, id pgtype.UUID) (dashboards.Dashboard, dashboards.Version, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return dashboards.Dashboard{ID: id.String(), LatestVersion: s.latest}, dashboards.Version{VersionMeta: dashboards.VersionMeta{Number: s.latest}, Spec: s.parse()}, nil
}

func (s *store) GetVersion(_ context.Context, id pgtype.UUID, version int) (dashboards.Dashboard, dashboards.Version, error) {
	return dashboards.Dashboard{ID: id.String()}, dashboards.Version{VersionMeta: dashboards.VersionMeta{Number: version}, Spec: s.parse()}, nil
}

func (s *store) LatestVersion(context.Context, pgtype.UUID) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.latest, nil
}

func (s *store) Build(context.Context, pgtype.UUID) (dashboards.Build, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.build == nil {
		return dashboards.Build{}, false, nil
	}
	return *s.build, true, nil
}

func (s *store) CommitBuild(ctx context.Context, _ auth.Principal, requestID pgtype.UUID, _ int) (dashboards.Commit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.commitOrigin = origin.From(ctx)
	if s.build == nil {
		return dashboards.Commit{}, dashboards.ErrBuildNotFound
	}
	if s.commitErr != nil {
		return dashboards.Commit{}, s.commitErr
	}
	if s.build.BaseVersion != s.latest {
		return dashboards.Commit{}, &dashboards.ConflictError{Latest: s.latest}
	}
	s.commits++
	s.build = nil
	if s.proposeCommits {
		s.insertProposal(requestID)
		return dashboards.Commit{Dashboard: dashboards.Dashboard{ID: s.dash.String()}, Proposal: &dashboards.ProposalRef{ID: s.proposalID, Revision: 1, Opened: true}}, nil
	}
	s.latest++
	s.insertVersion(requestID)
	return dashboards.Commit{Version: dashboards.Version{VersionMeta: dashboards.VersionMeta{Number: s.latest}}}, nil
}

func (s *store) insertProposal(requestID pgtype.UUID) {
	author := mustUUID(s.t, userID)
	s.d.Exec(s.t, `INSERT INTO proposals (id, dashboard_id, title, author_kind, author_id, author_name, origin, run_id) VALUES ($1, $2, 't', 'user', $3, 'Ada', 'mcp', $4)`,
		mustUUID(s.t, s.proposalID), s.dash, author, requestID)
}

func (s *store) insertVersion(requestID pgtype.UUID) {
	author := pgtype.UUID{}
	_ = author.Scan(userID)
	_, err := s.d.Queries().InsertDashboardVersion(context.Background(), dbsqlc.InsertDashboardVersionParams{
		DashboardID: s.dash, Version: httpapi.Int32(s.latest), ParentVersion: pgtype.Int4{Int32: httpapi.Int32(s.latest - 1), Valid: true},
		Spec: []byte(`{}`), Summary: "s", AuthorKind: "user", AuthorID: author, AuthorName: "Ada", AgentRequestID: requestID,
	})
	if err != nil {
		panic(err)
	}
	s.d.Exec(s.t, "UPDATE dashboards SET latest_version = $2 WHERE id = $1", s.dash, s.latest)
}

func (s *store) DiscardBuild(context.Context, pgtype.UUID, int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.discards++
	s.build = nil
	return nil
}

func (s *store) RebaseBuild(context.Context, auth.Principal, pgtype.UUID, int) (int, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rebases++
	if s.build == nil || !s.rebaseOK {
		return 0, false, nil
	}
	s.build.BaseVersion = s.latest
	return s.latest, true, nil
}

func (s *store) saveBuildPanel(planned int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.build == nil {
		s.build = &dashboards.Build{DashboardID: s.dash, BaseVersion: s.latest, Spec: s.parse()}
	}
	s.build.Saves++
	s.build.PlannedPanels = planned
}

func (s *store) bump() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.latest++
}

type healthy struct{}

func (healthy) Health(context.Context) []datasource.Status {
	return []datasource.Status{{UID: "prom", Type: "prometheus", Healthy: true}}
}

type rig struct {
	t      *testing.T
	d      *dbtest.Database
	svc    *Service
	runs   *revisions.Service
	claims *claims.Service
	store  *store
	steps  *stepLog
	dash   pgtype.UUID
}

type rigOption func(*Config)

func withMaxCost(limit float64, prices pricing.Table) rigOption {
	return func(c *Config) { c.MaxRunCostUSD, c.Prices = limit, prices }
}

func newRig(t *testing.T, options ...rigOption) *rig {
	t.Helper()
	d := dbtest.New(t)
	logger := slog.New(slog.DiscardHandler)
	runs := revisions.NewService(d.DB, logger, revisions.WithBuiltInAgent(),
		revisions.WithPrincipals(people{}), revisions.WithPanels(panels{}), revisions.WithAccess(folders.NewAccess(d.DB)), revisions.WithThreads(threads.NewRuns(d.DB)))
	claimsService := claims.New(claims.Config{DB: d.DB, Runs: runs, Listen: d.Listen, PublicURL: &url.URL{Scheme: "https", Host: "so.example.com"}, RunTimeout: time.Hour, Logger: logger})
	r := &rig{t: t, d: d, runs: runs, claims: claimsService, steps: &stepLog{}}
	r.dash = seedDashboard(t, d)
	r.store = &store{t: t, d: d, dash: r.dash, latest: 1, proposalID: proposalID}
	cfg := Config{
		DB: d.DB, Runs: runs, Claims: claimsService, Builds: r.store, Dashboards: r.store, Imports: noImports{},
		Statuses: healthy{}, Steps: r.steps, Logger: logger,
	}
	for _, option := range options {
		option(&cfg)
	}
	r.svc = New(cfg)
	return r
}

type noImports struct{}

func (noImports) ForDashboard(context.Context, pgtype.UUID) (imports.Record, error) {
	return imports.Record{}, imports.ErrNotFound
}

func mustUUID(t testing.TB, s string) pgtype.UUID {
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

func (r *rig) submit(source revisions.Source, panelID string) dbsqlc.AgentRequest {
	r.t.Helper()
	row, err := r.runs.Submit(context.Background(), revisions.EnqueueParams{
		DashboardID: r.dash, Source: source, Prompt: "add a latency panel", PanelID: panelID, BaseVersion: 1, By: ada,
	})
	if err != nil {
		r.t.Fatal(err)
	}
	return row
}

func (r *rig) claim(who revisions.Claimant) revisions.Claimed {
	r.t.Helper()
	claimed, found, err := r.runs.Claim(context.Background(), who)
	if err != nil || !found {
		r.t.Fatalf("claim as %s: found %v, err %v", who.Key(), found, err)
	}
	return claimed
}

func (r *rig) revise() revisions.Request {
	r.t.Helper()
	row := r.submit(revisions.SourcePrompt, "")
	r.d.Exec(r.t, "UPDATE agent_requests SET assigned_kind = 'grant', assigned_id = $2 WHERE id = $1", row.ID, mustUUID(r.t, grantID))
	return r.claim(grantClaimant).Request
}

func (r *rig) investigate() revisions.Request {
	r.t.Helper()
	to := time.Now().UTC().Truncate(time.Second)
	from := to.Add(-time.Hour)
	row, err := r.runs.SubmitInvestigate(context.Background(), revisions.InvestigateParams{
		DashboardID: r.dash, PanelID: "a", Prompt: "why is a slow", From: &from, To: &to, By: ada,
	})
	if err != nil {
		r.t.Fatal(err)
	}
	r.d.Exec(r.t, "UPDATE agent_requests SET assigned_kind = 'grant', assigned_id = $2 WHERE id = $1", row.ID, mustUUID(r.t, grantID))
	return r.claim(grantClaimant).Request
}

func (r *rig) suggest() revisions.Request {
	r.t.Helper()
	r.submit(revisions.SourceSuggest, "a")
	return r.claim(builtin).Request
}

func (r *rig) stored(req revisions.Request) dbsqlc.AgentRequest {
	r.t.Helper()
	row, err := r.runs.GetByID(context.Background(), mustUUID(r.t, req.ID))
	if err != nil {
		r.t.Fatal(err)
	}
	return row
}

func (r *rig) sessionRows(req revisions.Request) int {
	r.t.Helper()
	var n int
	if err := r.d.SQLDB(r.t).QueryRow("SELECT count(*) FROM run_sessions WHERE agent_request_id = $1", mustUUID(r.t, req.ID)).Scan(&n); err != nil {
		r.t.Fatal(err)
	}
	return n
}

func (r *rig) tokenCount(req revisions.Request) int {
	r.t.Helper()
	var n int
	if err := r.d.SQLDB(r.t).QueryRow("SELECT count(*) FROM run_tokens WHERE agent_request_id = $1", mustUUID(r.t, req.ID)).Scan(&n); err != nil {
		r.t.Fatal(err)
	}
	return n
}

type dispatchCount struct{ n int }

func (c *dispatchCount) wrap(fn func(context.Context) (tools.Result, error)) func(context.Context) (tools.Result, error) {
	return func(ctx context.Context) (tools.Result, error) {
		c.n++
		return fn(ctx)
	}
}

func readOK(context.Context) (tools.Result, error) {
	return tools.Result{Content: map[string]any{"ok": true}}, nil
}

func (r *rig) call(req revisions.Request, name string, input string, turn *Turn, dispatch func(context.Context) (tools.Result, error)) Answer {
	r.t.Helper()
	answer, err := r.svc.Call(context.Background(), req, Call{Name: name, Input: json.RawMessage(input), Turn: turn}, dispatch)
	if err != nil {
		r.t.Fatalf("call %s: %v", name, err)
	}
	return answer
}

func (r *rig) read(req revisions.Request, n int) Answer {
	r.t.Helper()
	return r.call(req, "list_metrics", `{"q":"`+strconv.Itoa(n)+`"}`, nil, readOK)
}

func buildSaved(r *rig, panel string, planned int) func(context.Context) (tools.Result, error) {
	return func(context.Context) (tools.Result, error) {
		r.store.saveBuildPanel(planned)
		return tools.Result{Content: map[string]any{"build": map[string]any{}}, Edits: tools.Edits{Panels: []string{panel}}}, nil
	}
}

func rejectedSave(context.Context) (tools.Result, error) {
	return tools.Result{
		Content: map[string]any{"code": "invalid_spec", "issues": []map[string]string{{"code": "unknown_field"}}},
		IsError: true,
	}, nil
}

func versionConflict(latest int) func(context.Context) (tools.Result, error) {
	return func(context.Context) (tools.Result, error) {
		return tools.Result{Content: map[string]any{"code": "version_conflict", "latest_version": latest}, IsError: true}, nil
	}
}

func findingSaved(context.Context) (tools.Result, error) {
	return tools.Result{Content: map[string]any{"finding_id": "f1"}}, nil
}

func suggestionSaved(context.Context) (tools.Result, error) {
	return tools.Result{Content: map[string]any{"suggested": true, "panel": map[string]any{"id": "a", "type": "stat"}}}, nil
}
