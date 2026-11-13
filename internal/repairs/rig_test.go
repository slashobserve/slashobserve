package repairs_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/internal/repairs"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

type stub struct{}

func (stub) Type() string                      { return "prometheus" }
func (stub) CheckHealth(context.Context) error { return nil }
func (stub) Close() error                      { return nil }
func (stub) Lint(context.Context, string) ([]datasource.Diagnostic, error) {
	return nil, nil
}

func (stub) Discover(context.Context, datasource.DiscoverRequest) (*datasource.Catalog, error) {
	return &datasource.Catalog{}, nil
}

func (stub) VariableValues(context.Context, datasource.Query, datasource.TimeRange) ([]string, error) {
	return []string{"v"}, nil
}

func (stub) QueryRange(context.Context, datasource.Query, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
	return frames(), nil
}

func (stub) QueryInstant(context.Context, datasource.Query, datasource.TimeRange) ([]datasource.Frame, error) {
	return frames(), nil
}

func frames() []datasource.Frame {
	return []datasource.Frame{{Fields: []datasource.Field{
		{Name: "time", Type: datasource.FieldTime, Values: []any{1.0}},
		{Name: "value", Type: datasource.FieldNumber, Values: []any{2.0}},
	}}}
}

type rig struct {
	db         *dbtest.Database
	dashboards *dashboards.Service
	revisions  *revisions.Service
	svc        *repairs.Service
	noAgent    *repairs.Service
	editor     auth.Principal
	other      auth.Principal
	viewer     auth.Principal
	shared     pgtype.UUID
}

func newRig(t *testing.T) *rig {
	t.Helper()
	database := dbtest.New(t)
	logger := slog.New(slog.DiscardHandler)
	registry, err := datasource.NewRegistry(logger, datasource.Instance{UID: "prom", Datasource: stub{}})
	if err != nil {
		t.Fatal(err)
	}
	access := folders.NewAccess(database.DB)
	base, err := url.Parse("https://observe.example.com")
	if err != nil {
		t.Fatal(err)
	}
	queue := revisions.NewService(database.DB, logger, revisions.WithBuiltInAgent(), revisions.WithAccess(access))
	none := revisions.NewService(database.DB, logger, revisions.WithAccess(access))
	deps := func(r *revisions.Service) repairs.Deps {
		return repairs.Deps{DB: database.DB, Access: access, Revisions: r, PublicURL: base, Logger: logger}
	}
	shared, err := database.Queries().CreateFolder(t.Context(), dbsqlc.CreateFolderParams{Title: "Imported"})
	if err != nil {
		t.Fatal(err)
	}
	return &rig{
		db:         database,
		dashboards: dashboards.New(database.DB, queryproxy.NewRunner(registry, logger), access, logger),
		revisions:  queue,
		svc:        repairs.New(deps(queue)),
		noAgent:    repairs.New(deps(none)),
		editor:     user(t, database, "eda", auth.RoleEditor),
		other:      user(t, database, "oli", auth.RoleEditor),
		viewer:     user(t, database, "vic", auth.RoleViewer),
		shared:     shared.ID,
	}
}

func user(t *testing.T, database *dbtest.Database, name string, role auth.Role) auth.Principal {
	t.Helper()
	row, err := database.Queries().UpsertUser(t.Context(), dbsqlc.UpsertUserParams{OidcIssuer: "issuer", OidcSubject: name, Email: name + "@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	return auth.Principal{Kind: auth.KindUser, ID: row.ID.String(), Name: name, Email: name + "@example.com", Role: role}
}

func boardSpec(title string) []byte {
	return []byte(fmt.Sprintf(`{"specVersion":"0","title":%q,"time":{"from":"now-15m","to":"now"},"panels":[`+
		`{"id":"rps","type":"timeseries","title":"rps","gridPos":{"x":0,"y":0,"w":12,"h":8},"datasource":"prom","queries":[{"query":"up"}]},`+
		`{"id":"errs","type":"timeseries","title":"errs","gridPos":{"x":12,"y":0,"w":12,"h":8},"datasource":"prom","queries":[{"query":"up"}]}]}`, title))
}

func (r *rig) dashboard(t *testing.T, folder pgtype.UUID, title string) pgtype.UUID {
	t.Helper()
	dash, _, err := r.dashboards.Create(t.Context(), r.editor, folder, boardSpec(title), "create")
	if err != nil {
		t.Fatal(err)
	}
	id, err := db.ParseUUID(dash.ID)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

const onePanelUnverified = `{"panels":[{"panel_id":"rps","issues":[]}],"variables":[]}`

func (r *rig) failing(t *testing.T, folder pgtype.UUID, title string) pgtype.UUID {
	t.Helper()
	id := r.dashboard(t, folder, title)
	r.setUnverified(t, id, onePanelUnverified)
	return id
}

func (r *rig) setUnverified(t *testing.T, id pgtype.UUID, unverified string) {
	t.Helper()
	r.db.Exec(t, `UPDATE dashboard_versions v SET unverified = $2::jsonb FROM dashboards d
		WHERE d.id = $1 AND v.dashboard_id = d.id AND v.version = d.latest_version`, id, unverified)
}

func (r *rig) setPanels(t *testing.T, id pgtype.UUID, panels string) {
	t.Helper()
	r.db.Exec(t, `UPDATE dashboard_versions v SET spec = jsonb_set(v.spec, '{panels}', $2::jsonb) FROM dashboards d
		WHERE d.id = $1 AND v.dashboard_id = d.id AND v.version = d.latest_version`, id, panels)
}

func (r *rig) restrict(t *testing.T, folder pgtype.UUID) {
	t.Helper()
	r.db.Exec(t, "INSERT INTO folder_permissions (folder_id, group_name, level) VALUES ($1, 'secret-team', 'view')", folder)
}

func (r *rig) folder(t *testing.T, title string) pgtype.UUID {
	t.Helper()
	f, err := r.db.Queries().CreateFolder(t.Context(), dbsqlc.CreateFolderParams{Title: title})
	if err != nil {
		t.Fatal(err)
	}
	return f.ID
}

func (r *rig) list(t *testing.T, by auth.Principal, p repairs.ListParams) repairs.Page {
	t.Helper()
	if p.Limit == 0 {
		p.Limit = 50
	}
	page, err := r.svc.List(t.Context(), by, p)
	if err != nil {
		t.Fatal(err)
	}
	return page
}

func (r *rig) request(t *testing.T, target repairs.Target) repairs.Outcome {
	t.Helper()
	out, err := r.svc.Request(t.Context(), r.editor, target)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func byID(items []repairs.Item) map[string]repairs.Item {
	out := make(map[string]repairs.Item, len(items))
	for _, it := range items {
		out[it.ID] = it
	}
	return out
}

func reasons(out repairs.Outcome) map[string]string {
	got := make(map[string]string, len(out.Skipped))
	for _, s := range out.Skipped {
		got[s.DashboardID] = s.Reason
	}
	return got
}
