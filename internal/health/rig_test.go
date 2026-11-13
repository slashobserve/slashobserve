package health_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
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
	"github.com/slashobserve/slashobserve/internal/health"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

type stub struct {
	broken  *atomic.Bool
	queries *atomic.Int64
}

func (stub) Type() string                      { return "prometheus" }
func (stub) CheckHealth(context.Context) error { return nil }
func (stub) Close() error                      { return nil }
func (stub) Lint(context.Context, string) ([]datasource.Diagnostic, error) {
	return nil, nil
}

func (stub) Discover(context.Context, datasource.DiscoverRequest) (*datasource.Catalog, error) {
	return &datasource.Catalog{}, nil
}

func (s stub) VariableValues(_ context.Context, q datasource.Query, _ datasource.TimeRange) ([]string, error) {
	if s.broken.Load() && strings.Contains(q.Text, "gone") {
		return nil, nil
	}
	return []string{"v"}, nil
}

func (s stub) QueryRange(_ context.Context, q datasource.Query, _ datasource.TimeRange, _ time.Duration) ([]datasource.Frame, error) {
	return s.frames(q)
}

func (s stub) QueryInstant(_ context.Context, q datasource.Query, _ datasource.TimeRange) ([]datasource.Frame, error) {
	return s.frames(q)
}

func (s stub) frames(q datasource.Query) ([]datasource.Frame, error) {
	s.queries.Add(1)
	if s.broken.Load() {
		switch {
		case strings.Contains(q.Text, "empty"):
			return nil, nil
		case strings.Contains(q.Text, "fail"):
			return nil, errors.New("upstream refused the query")
		}
	}
	return []datasource.Frame{{Fields: []datasource.Field{
		{Name: "time", Type: datasource.FieldTime, Values: []any{1.0}},
		{Name: "value", Type: datasource.FieldNumber, Values: []any{2.0}},
	}}}, nil
}

type rig struct {
	db         *dbtest.Database
	dashboards *dashboards.Service
	revisions  *revisions.Service
	svc        *health.Service
	repairs    *health.Repairs
	noAgent    *health.Repairs
	broken     *atomic.Bool
	queries    *atomic.Int64
	editor     auth.Principal
	viewer     auth.Principal
	key        auth.Principal
	shared     pgtype.UUID
}

func newRig(t *testing.T) *rig {
	t.Helper()
	database := dbtest.New(t)
	logger := slog.New(slog.DiscardHandler)
	broken := &atomic.Bool{}
	queries := &atomic.Int64{}
	registry, err := datasource.NewRegistry(logger, datasource.Instance{UID: "prom", Datasource: stub{broken: broken, queries: queries}})
	if err != nil {
		t.Fatal(err)
	}
	access := folders.NewAccess(database.DB)
	base, err := url.Parse("https://observe.example.com")
	if err != nil {
		t.Fatal(err)
	}
	runner := queryproxy.NewRunner(registry, logger)
	queue := revisions.NewService(database.DB, logger, revisions.WithBuiltInAgent(), revisions.WithAccess(access))
	none := revisions.NewService(database.DB, logger, revisions.WithAccess(access))
	svc := health.New(health.Deps{DB: database.DB, Runner: runner, Access: access, PublicURL: base, Logger: logger, Job: "dashboard_health"})
	shared, err := database.Queries().CreateFolder(t.Context(), dbsqlc.CreateFolderParams{Title: "Shop"})
	if err != nil {
		t.Fatal(err)
	}
	return &rig{
		db:         database,
		dashboards: dashboards.New(database.DB, runner, access, logger),
		revisions:  queue,
		svc:        svc,
		repairs:    health.NewRepairs(svc, queue),
		noAgent:    health.NewRepairs(svc, none),
		broken:     broken,
		queries:    queries,
		editor:     user(t, database, "eda", auth.RoleEditor),
		viewer:     user(t, database, "vic", auth.RoleViewer),
		key:        auth.Principal{Kind: auth.KindServiceAccount, ID: "0b9e9a3e-5d5e-4f0e-9e1a-7d2f3c4b5a61", Name: "ci", Role: auth.RoleEditor},
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

func panelsJSON(queries []string) string {
	var panels []string
	for i, q := range queries {
		panels = append(panels, fmt.Sprintf(`{"id":"p%d","type":"timeseries","title":"Panel %d","gridPos":{"x":0,"y":%d,"w":12,"h":8},"datasource":"prom","queries":[{"query":%q}]}`, i, i, i*8, q))
	}
	return strings.Join(panels, ",")
}

func boardSpec(title string, queries ...string) []byte {
	return []byte(fmt.Sprintf(`{"specVersion":"0","title":%q,"time":{"from":"now-15m","to":"now"},"panels":[%s]}`, title, panelsJSON(queries)))
}

func variableSpec(title string, queries ...string) []byte {
	if len(queries) == 0 {
		queries = []string{"up"}
	}
	return []byte(fmt.Sprintf(`{"specVersion":"0","title":%q,"time":{"from":"now-15m","to":"now"},`+
		`"variables":[{"name":"pod","type":"query","datasource":"prom","query":"label_values(gone, pod)"}],`+
		`"panels":[%s]}`, title, panelsJSON(queries)))
}

func (r *rig) create(t *testing.T, raw []byte) pgtype.UUID {
	t.Helper()
	return r.createIn(t, r.shared, raw)
}

func (r *rig) createIn(t *testing.T, folder pgtype.UUID, raw []byte) pgtype.UUID {
	t.Helper()
	dash, _, err := r.dashboards.Create(t.Context(), r.editor, folder, raw, "create")
	if err != nil {
		t.Fatal(err)
	}
	id, err := db.ParseUUID(dash.ID)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (r *rig) list(t *testing.T, by auth.Principal, kind health.Kind) map[string]health.Item {
	t.Helper()
	page, err := r.svc.List(t.Context(), by, health.ListParams{Kind: kind, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	items := map[string]health.Item{}
	for _, it := range page.Items {
		items[it.ID] = it
	}
	return items
}

func (r *rig) folder(t *testing.T, title string) pgtype.UUID {
	t.Helper()
	f, err := r.db.Queries().CreateFolder(t.Context(), dbsqlc.CreateFolderParams{Title: title})
	if err != nil {
		t.Fatal(err)
	}
	return f.ID
}

func (r *rig) openedAgo(t *testing.T, id pgtype.UUID, ago time.Duration) {
	t.Helper()
	if err := r.svc.RecordView(t.Context(), r.viewer, id); err != nil {
		t.Fatal(err)
	}
	r.db.Exec(t, `UPDATE dashboard_views SET viewed_at = now() - make_interval(secs => $2) WHERE dashboard_id = $1`, id.String(), ago.Seconds())
}

func (r *rig) createdAgo(t *testing.T, id pgtype.UUID, ago time.Duration) {
	t.Helper()
	r.db.Exec(t, `UPDATE dashboards SET created_at = now() - make_interval(secs => $2) WHERE id = $1`, id.String(), ago.Seconds())
}

type request struct {
	prompt, status string
	resultVersion  *int
	failure        *string
	ago            time.Duration
}

func (r *rig) request(t *testing.T, dashboard pgtype.UUID, req request) string {
	t.Helper()
	const insert = `INSERT INTO agent_requests (dashboard_id, source, prompt, base_version, status, state, result_version, error,
		requested_by_kind, requested_by_id, requested_by_name, created_at, started_at, finished_at)
	SELECT $1, 'prompt', $2, 1, $3, $3, $4, $5, 'user', $6, 'eda', at,
		CASE WHEN $3 <> 'queued' THEN at + interval '1 second' END,
		CASE WHEN $3 IN ('succeeded', 'failed') THEN at + interval '2 seconds' END
	FROM (SELECT now() - make_interval(secs => $7) AS at) t
	RETURNING id`
	var id string
	row := r.db.SQLDB(t).QueryRowContext(t.Context(), insert, dashboard.String(), req.prompt, req.status, req.resultVersion, req.failure, r.editor.ID, req.ago.Seconds())
	if err := row.Scan(&id); err != nil {
		t.Fatalf("insert agent request: %v", err)
	}
	return id
}

func kinds(it health.Item) []health.Kind {
	out := make([]health.Kind, 0, len(it.Problems))
	for _, p := range it.Problems {
		out = append(out, p.Kind)
	}
	return out
}

func (r *rig) scan(t *testing.T, query string, arg any, dest ...any) {
	t.Helper()
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), query, arg).Scan(dest...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}
