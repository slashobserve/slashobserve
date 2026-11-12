package proposals

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

const (
	editorID     = "11111111-1111-4111-8111-111111111111"
	commenterID  = "44444444-4444-4444-8444-444444444444"
	otherID      = "66666666-6666-4666-8666-666666666666"
	viewerID     = "33333333-3333-4333-8333-333333333333"
	hiddenGroup  = "payroll-team"
	hiddenViewer = "77777777-7777-4777-8777-777777777777"
)

var (
	editor         = auth.Principal{Kind: auth.KindUser, ID: editorID, Name: "Ada", Email: "ada@example.com", Role: auth.RoleEditor}
	commenter      = auth.Principal{Kind: auth.KindUser, ID: commenterID, Name: "Bo", Role: auth.RoleCommenter}
	otherCommenter = auth.Principal{Kind: auth.KindUser, ID: otherID, Name: "Cy", Role: auth.RoleCommenter}
	viewer         = auth.Principal{Kind: auth.KindUser, ID: viewerID, Name: "Vic", Role: auth.RoleViewer}
	outsider       = auth.Principal{Kind: auth.KindUser, ID: hiddenViewer, Name: "Out", Role: auth.RoleEditor}
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

type rig struct {
	db         *dbtest.Database
	service    *Service
	dashboards *dashboards.Service
	access     *folders.Access
}

func newRig(t *testing.T) *rig {
	t.Helper()
	database := dbtest.New(t)
	logger := slog.New(slog.DiscardHandler)
	registry, err := datasource.NewRegistry(logger, datasource.Instance{UID: "prom", Datasource: fakeDatasource{}})
	if err != nil {
		t.Fatal(err)
	}
	access := folders.NewAccess(database.DB)
	dashboardService := dashboards.New(database.DB, queryproxy.NewRunner(registry, logger), access, logger)
	service := New(database.DB, access, dashboardService, logger)
	dashboardService.SetProposer(service.Propose)
	return &rig{db: database, service: service, dashboards: dashboardService, access: access}
}

type panel struct {
	id    string
	query string
}

func specOf(title string, panels ...panel) []byte {
	parts := make([]string, len(panels))
	for i, p := range panels {
		parts[i] = fmt.Sprintf(`{"id":%q,"type":"timeseries","title":%q,"gridPos":{"x":0,"y":%d,"w":12,"h":8},"datasource":"prom","queries":[{"query":%q}]}`, p.id, p.id, i*8, p.query)
	}
	return []byte(fmt.Sprintf(`{"specVersion":"0","title":%q,"time":{"from":"now-15m","to":"now"},"panels":[%s]}`, title, strings.Join(parts, ",")))
}

func (r *rig) createDashboard(t *testing.T, panels ...panel) pgtype.UUID {
	t.Helper()
	dash, _, err := r.dashboards.Create(t.Context(), editor, pgtype.UUID{}, specOf("t", panels...), "create")
	if err != nil {
		t.Fatal(err)
	}
	return mustParse(t, dash.ID)
}

func (r *rig) addVersion(t *testing.T, id pgtype.UUID, parent int, panels ...panel) {
	t.Helper()
	if _, _, _, err := r.dashboards.AddVersion(t.Context(), editor, id, parent, specOf("t", panels...), "next"); err != nil {
		t.Fatal(err)
	}
}

func (r *rig) save(ctx context.Context, by auth.Principal, id pgtype.UUID, base int, panels ...panel) (dashboards.SaveResult, error) {
	return r.dashboards.Save(ctx, by, dashboards.SaveInput{
		DashboardID:     id,
		Parent:          base,
		Spec:            specOf("t", panels...),
		Summary:         "proposed change",
		ProposeIfNeeded: true,
	})
}

func (r *rig) propose(t *testing.T, by auth.Principal, id pgtype.UUID, base int, panels ...panel) dashboards.ProposalRef {
	t.Helper()
	res, err := r.save(t.Context(), by, id, base, panels...)
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if res.Proposal == nil {
		t.Fatal("the save did not become a proposal")
	}
	return *res.Proposal
}

func (r *rig) latest(t *testing.T, id pgtype.UUID) int {
	t.Helper()
	n, err := r.dashboards.LatestVersion(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func (r *rig) activity(t *testing.T) []string {
	t.Helper()
	rows, err := r.db.SQLDB(t).QueryContext(t.Context(), `SELECT type FROM activity WHERE target_kind = 'proposal' ORDER BY seq`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var typ string
		if err := rows.Scan(&typ); err != nil {
			t.Fatal(err)
		}
		out = append(out, typ)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func (r *rig) activityData(t *testing.T, typ string) map[string]any {
	t.Helper()
	var raw []byte
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT data FROM activity WHERE type = $1 ORDER BY seq DESC LIMIT 1`, typ).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	return data
}

func mustParse(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	id, err := db.ParseUUID(s)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func proposalID(t *testing.T, ref dashboards.ProposalRef) pgtype.UUID {
	t.Helper()
	return mustParse(t, ref.ID)
}

func (r *rig) hiddenFolder(t *testing.T) pgtype.UUID {
	t.Helper()
	return foldertest.Restricted(t, r.db, "Payroll", hiddenGroup)
}

func (r *rig) moveInto(t *testing.T, dashboard, folder pgtype.UUID) {
	t.Helper()
	foldertest.Move(t, r.db, dashboard, folder)
}

func (r *rig) generalFolder(t *testing.T) pgtype.UUID {
	t.Helper()
	var raw string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT general_folder_id()::text`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	return mustParse(t, raw)
}
