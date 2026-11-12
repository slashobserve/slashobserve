package threads

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/annotations"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/observations"
	"github.com/slashobserve/slashobserve/internal/proposals"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

const (
	testUserID   = "11111111-1111-4111-8111-111111111111"
	commenterID  = "44444444-4444-4444-8444-444444444444"
	viewerID     = "33333333-3333-4333-8333-333333333333"
	adminID      = "55555555-5555-4555-8555-555555555555"
	payrollGroup = "payroll-team"
)

var (
	testUser  = auth.Principal{Kind: auth.KindUser, ID: testUserID, Name: "Ada", Email: "ada@example.com", Role: auth.RoleEditor}
	commenter = auth.Principal{Kind: auth.KindUser, ID: commenterID, Name: "Bo", Role: auth.RoleCommenter}
	viewer    = auth.Principal{Kind: auth.KindUser, ID: viewerID, Name: "Vic", Role: auth.RoleViewer}
	admin     = auth.Principal{Kind: auth.KindUser, ID: adminID, Name: "Root", Role: auth.RoleAdmin}
)

type fakeDatasource struct{}

func (fakeDatasource) Type() string                      { return "prometheus" }
func (fakeDatasource) CheckHealth(context.Context) error { return nil }
func (fakeDatasource) Lint(context.Context, string) ([]datasource.Diagnostic, error) {
	return nil, nil
}
func (fakeDatasource) Close() error { return nil }
func (fakeDatasource) Discover(context.Context, datasource.DiscoverRequest) (*datasource.Catalog, error) {
	return &datasource.Catalog{}, nil
}

func (fakeDatasource) VariableValues(context.Context, datasource.Query, datasource.TimeRange) ([]string, error) {
	return []string{"v"}, nil
}

func (fakeDatasource) QueryRange(context.Context, datasource.Query, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
	return fakeFrames(), nil
}

func (fakeDatasource) QueryInstant(context.Context, datasource.Query, datasource.TimeRange) ([]datasource.Frame, error) {
	return fakeFrames(), nil
}

func fakeFrames() []datasource.Frame {
	return []datasource.Frame{{Fields: []datasource.Field{
		{Name: "time", Type: datasource.FieldTime, Values: []any{1.0}},
		{Name: "value", Type: datasource.FieldNumber, Values: []any{2.0}},
	}}}
}

type noopReviser struct{}

func (noopReviser) Revise(context.Context, revisions.Request) (revisions.Result, error) {
	return revisions.Result{}, nil
}

type rig struct {
	db           *dbtest.Database
	service      *Service
	comments     *Comments
	revisions    *revisions.Service
	dashboards   *dashboards.Service
	findings     *findings.Service
	annotations  *annotations.Service
	observations *observations.Service
	proposals    *proposals.Service
}

func newRig(t *testing.T) *rig {
	t.Helper()
	return newAgentRig(t, false)
}

func newAgentRig(t *testing.T, agent bool) *rig {
	t.Helper()
	database := dbtest.New(t)
	logger := slog.New(slog.DiscardHandler)
	registry, err := datasource.NewRegistry(logger, datasource.Instance{UID: "prom", Datasource: fakeDatasource{}})
	if err != nil {
		t.Fatal(err)
	}
	access := folders.NewAccess(database.DB)
	dashboardService := dashboards.New(database.DB, queryproxy.NewRunner(registry, logger), access, logger)
	annotationService := annotations.New(database.DB, access)
	observationService := observations.New(database.DB, access, nil, nil, logger)
	findingService, err := findings.New(database.DB, access, annotationService, dashboardService, observationService, logger)
	if err != nil {
		t.Fatal(err)
	}
	var reviser revisions.Reviser
	if agent {
		reviser = noopReviser{}
	}
	queue := revisions.NewService(database.DB, reviser, logger, revisions.WithAccess(access))
	proposalService := proposals.New(database.DB, access, dashboardService, logger)
	dashboardService.SetProposer(proposalService.Propose)
	service := NewService(database.DB, access, dashboardService, annotationService, observationService, proposalService)
	return &rig{
		db:           database,
		service:      service,
		comments:     NewComments(service, queue),
		revisions:    queue,
		dashboards:   dashboardService,
		findings:     findingService,
		annotations:  annotationService,
		observations: observationService,
		proposals:    proposalService,
	}
}

func panelsSpec(title string, panelIDs ...string) []byte {
	panels := ""
	for i, id := range panelIDs {
		if i > 0 {
			panels += ","
		}
		panels += fmt.Sprintf(`{"id":%q,"type":"timeseries","title":%q,"gridPos":{"x":0,"y":%d,"w":12,"h":8},"datasource":"prom","queries":[{"query":"up"}]}`, id, id, i*8)
	}
	return []byte(fmt.Sprintf(`{"specVersion":"0","title":%q,"time":{"from":"now-15m","to":"now"},"panels":[%s]}`, title, panels))
}

func (r *rig) createDashboard(t *testing.T, panelIDs ...string) pgtype.UUID {
	t.Helper()
	if len(panelIDs) == 0 {
		panelIDs = []string{"rps"}
	}
	dash, _, err := r.dashboards.Create(t.Context(), testUser, pgtype.UUID{}, panelsSpec("t", panelIDs...), "create")
	if err != nil {
		t.Fatal(err)
	}
	return mustParse(t, dash.ID)
}

func (r *rig) addVersion(t *testing.T, dash pgtype.UUID, parent int) {
	t.Helper()
	if _, _, _, err := r.dashboards.AddVersion(t.Context(), testUser, dash, parent, panelsSpec(fmt.Sprintf("t%d", parent), "rps", "errors"), "next"); err != nil {
		t.Fatal(err)
	}
}

func (r *rig) createFinding(t *testing.T, by auth.Principal, dashboardID pgtype.UUID) pgtype.UUID {
	t.Helper()
	return r.saveFinding(t, by, findings.SaveInput{DashboardID: dashboardID})
}

func (r *rig) createFindingIn(t *testing.T, by auth.Principal, folder pgtype.UUID) pgtype.UUID {
	t.Helper()
	return r.saveFinding(t, by, findings.SaveInput{FolderID: folder})
}

func (r *rig) saveFinding(t *testing.T, by auth.Principal, in findings.SaveInput) pgtype.UUID {
	t.Helper()
	in.Title, in.Markdown, in.From, in.To = "Checkout is slow", "the 5xx rate rose", time.Now().Add(-time.Hour), time.Now()
	f, _, err := r.findings.Save(t.Context(), by, in)
	if err != nil {
		t.Fatal(err)
	}
	return mustParse(t, f.ID)
}

func (r *rig) observationOf(t *testing.T, finding pgtype.UUID) pgtype.UUID {
	t.Helper()
	id, err := r.db.Queries().ObservationOfFinding(t.Context(), finding)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (r *rig) createAnnotation(t *testing.T, by auth.Principal, dashboardID pgtype.UUID) pgtype.UUID {
	t.Helper()
	a, err := r.annotations.Create(t.Context(), by, annotations.CreateInput{DashboardID: dashboardID, Text: "deploy"})
	if err != nil {
		t.Fatal(err)
	}
	return mustParse(t, a.ID)
}

func (r *rig) createUser(t *testing.T, name, email string) pgtype.UUID {
	t.Helper()
	u, err := r.db.Queries().UpsertUser(t.Context(), dbsqlc.UpsertUserParams{OidcIssuer: "issuer", OidcSubject: email, Email: email, Name: name})
	if err != nil {
		t.Fatal(err)
	}
	return u.ID
}

type activityRow struct {
	Type       string
	TargetKind string
	TargetID   string
	Dashboard  string
	Folder     string
	ActorID    string
	ActorName  string
	Data       map[string]any
}

func (r *rig) activity(t *testing.T) []activityRow {
	t.Helper()
	rows, err := r.db.SQLDB(t).QueryContext(t.Context(), `SELECT type, target_kind, target_id::text, coalesce(dashboard_id::text, ''), coalesce(folder_id::text, ''), actor_id::text, actor_name, data FROM activity WHERE target_kind = 'thread' ORDER BY seq`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []activityRow
	for rows.Next() {
		var a activityRow
		var raw []byte
		if err := rows.Scan(&a.Type, &a.TargetKind, &a.TargetID, &a.Dashboard, &a.Folder, &a.ActorID, &a.ActorName, &raw); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &a.Data); err != nil {
			t.Fatal(err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func activityTypes(rows []activityRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Type
	}
	return out
}

func mustParse(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	id, err := db.ParseUUID(s)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func target(kind Kind, id pgtype.UUID) Target {
	return Target{Kind: kind, ID: id}
}
