package grafanaconn_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/egress"
	"github.com/slashobserve/slashobserve/internal/egress/allowlist"
	"github.com/slashobserve/slashobserve/internal/egress/egresstest"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/grafanaconn"
	"github.com/slashobserve/slashobserve/internal/imports"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

const (
	grafanaHost  = "grafana.example.com"
	grafanaURL   = "https://" + grafanaHost
	grafanaToken = "glsa_test_token_0123456789" //nolint:gosec // the fake Grafana's token, not a credential
)

var secretKey = []byte("0123456789abcdef0123456789abcdef")

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

type stub struct{ kind string }

func (s stub) Type() string                    { return s.kind }
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

type fakeFolder struct {
	uid, title, parent string
}

type fakeDashboard struct {
	uid, title, folder string
	model              map[string]any
	version            int
	status             int
}

type fakeGrafana struct {
	mu          sync.Mutex
	token       string
	folders     []fakeFolder
	dashboards  []*fakeDashboard
	datasources []map[string]string
	fetched     map[string]int
	auth        []string
	hold        chan struct{}
}

func newFakeGrafana() *fakeGrafana {
	return &fakeGrafana{
		token: grafanaToken,
		datasources: []map[string]string{
			{"uid": "graf-prom", "name": "Prometheus", "type": "prometheus"},
			{"uid": "graf-ch", "name": "ClickHouse", "type": "grafana-clickhouse-datasource"},
			{"uid": "graf-loki", "name": "Loki", "type": "loki"},
		},
		fetched: map[string]int{},
	}
}

func (g *fakeGrafana) add(d *fakeDashboard) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.dashboards = append(g.dashboards, d)
}

func (g *fakeGrafana) bump(uid string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, d := range g.dashboards {
		if d.uid == uid {
			d.version++
		}
	}
}

func (g *fakeGrafana) authHeaders() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return slices.Clone(g.auth)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func (g *fakeGrafana) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if g.hold != nil && strings.HasPrefix(r.URL.Path, "/api/dashboards/uid/") {
		select {
		case <-g.hold:
		case <-r.Context().Done():
			return
		}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.auth = append(g.auth, r.Header.Get("Authorization"))
	if r.Header.Get("Authorization") != "Bearer "+g.token {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "invalid API key"})
		return
	}
	switch path := r.URL.Path; {
	case path == "/api/health":
		writeJSON(w, http.StatusOK, map[string]string{"database": "ok", "version": "11.3.0"})
	case path == "/api/folders":
		roots := []map[string]string{}
		for _, f := range g.folders {
			if f.parent == "" {
				roots = append(roots, map[string]string{"uid": f.uid, "title": f.title})
			}
		}
		writeJSON(w, http.StatusOK, roots)
	case path == "/api/search":
		writeJSON(w, http.StatusOK, g.search(r.URL.Query().Get("type"), r.URL.Query().Get("page")))
	case path == "/api/datasources":
		writeJSON(w, http.StatusOK, g.datasources)
	case strings.HasPrefix(path, "/api/dashboards/uid/"):
		g.serveDashboard(w, strings.TrimPrefix(path, "/api/dashboards/uid/"))
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "not found"})
	}
}

func (g *fakeGrafana) search(kind, page string) []map[string]any {
	hits := []map[string]any{}
	if page != "1" {
		return hits
	}
	if kind == "dash-folder" {
		for _, f := range g.folders {
			hits = append(hits, map[string]any{"uid": f.uid, "title": f.title, "type": kind, "folderUid": f.parent})
		}
		return hits
	}
	for _, d := range g.dashboards {
		hit := map[string]any{"uid": d.uid, "title": d.title, "type": kind, "tags": []string{}}
		if d.folder != "" {
			hit["folderUid"] = d.folder
		}
		hits = append(hits, hit)
	}
	return hits
}

func (g *fakeGrafana) serveDashboard(w http.ResponseWriter, uid string) {
	for _, d := range g.dashboards {
		if d.uid != uid {
			continue
		}
		g.fetched[uid]++
		if d.status != 0 {
			writeJSON(w, d.status, map[string]string{"message": "broken"})
			return
		}
		model := map[string]any{}
		for k, v := range d.model {
			model[k] = v
		}
		model["uid"], model["title"], model["version"] = d.uid, d.title, d.version
		writeJSON(w, http.StatusOK, map[string]any{"dashboard": model, "meta": map[string]any{"folderUid": d.folder}})
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"message": "Dashboard not found"})
}

func simpleModel(datasourceUID, pluginID string) map[string]any {
	return map[string]any{
		"schemaVersion": 39,
		"panels": []any{map[string]any{
			"id": 1, "type": "timeseries", "title": "Requests",
			"gridPos":    map[string]any{"x": 0, "y": 0, "w": 12, "h": 8},
			"datasource": map[string]any{"type": pluginID, "uid": datasourceUID},
			"targets":    []any{map[string]any{"refId": "A", "expr": "up"}},
		}},
	}
}

type principals struct {
	mu   sync.Mutex
	byID map[string]auth.Principal
}

func (p *principals) set(pr auth.Principal) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.byID[pr.ID] = pr
}

func (p *principals) Principal(_ context.Context, _ auth.PrincipalKind, id string) (auth.Principal, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	pr, ok := p.byID[id]
	if !ok {
		return auth.Principal{}, auth.ErrPrincipalGone
	}
	return pr, nil
}

type rig struct {
	db         *dbtest.Database
	grafana    *fakeGrafana
	domains    *allowlist.Store
	access     *folders.Access
	egress     *egress.Client
	imports    *imports.Service
	dashboard  *dashboards.Service
	principals *principals
	svc        *grafanaconn.Service
	secretKey  []byte
	admin      auth.Principal
	viewer     auth.Principal
	stop       context.CancelFunc
}

type rigOptions struct {
	secretKey []byte
	lock      allowlist.Lock
	hold      chan struct{}
}

func newRig(t *testing.T) *rig {
	t.Helper()
	return newRigWith(t, rigOptions{secretKey: secretKey})
}

func newRigWith(t *testing.T, opts rigOptions) *rig {
	t.Helper()
	database := dbtest.New(t)
	logger := discard()
	registry, err := datasource.NewRegistry(logger,
		datasource.Instance{UID: "prom", Datasource: stub{kind: "prometheus"}},
		datasource.Instance{UID: "ch", Datasource: stub{kind: "clickhouse"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	access := folders.NewAccess(database.DB)
	dash := dashboards.New(database.DB, queryproxy.NewRunner(registry, logger), access, logger)
	importsService := imports.New(database.DB, dash, access, registry, logger)
	domains := allowlist.New(database.DB, opts.lock, logger)
	fake := newFakeGrafana()
	fake.hold = opts.hold
	egressClient := egresstest.NewClient(t, domains, fake, grafanaHost)
	known := &principals{byID: map[string]auth.Principal{}}
	life, cancel := context.WithCancel(context.Background())
	svc := grafanaconn.New(life, grafanaconn.Deps{
		DB:         database.DB,
		Egress:     egressClient,
		Domains:    domains,
		Imports:    importsService,
		Access:     access,
		Principals: known,
		InstanceID: "server-1",
		SecretKey:  opts.secretKey,
		Logger:     logger,
	})
	t.Cleanup(func() {
		cancel()
		svc.Wait()
	})
	r := &rig{
		db: database, grafana: fake, domains: domains, access: access, egress: egressClient, imports: importsService,
		dashboard: dash, principals: known, svc: svc, secretKey: opts.secretKey,
		admin:  user(t, database, "ada", auth.RoleAdmin),
		viewer: user(t, database, "vic", auth.RoleViewer),
		stop:   cancel,
	}
	known.set(r.admin)
	known.set(r.viewer)
	return r
}

func (r *rig) peer(t *testing.T) *grafanaconn.Service {
	t.Helper()
	life, cancel := context.WithCancel(context.Background())
	svc := grafanaconn.New(life, grafanaconn.Deps{
		DB:         r.db.DB,
		Egress:     r.egress,
		Domains:    r.domains,
		Imports:    r.imports,
		Access:     r.access,
		Principals: r.principals,
		InstanceID: "server-2",
		SecretKey:  r.secretKey,
		Logger:     discard(),
	})
	t.Cleanup(func() {
		cancel()
		svc.Wait()
	})
	return svc
}

func (r *rig) insertRunning(t *testing.T, connectionID, starterID string, leaseLosses int) string {
	t.Helper()
	var id string
	err := r.db.SQLDB(t).QueryRowContext(t.Context(), `
		INSERT INTO grafana_syncs (connection_id, state, request, started_by_kind, started_by_id, lease_id, leased_by, lease_until, lease_losses)
		VALUES ($1, 'running', '{"all":true,"folders":[],"mapping":{}}', 'user', $2, gen_random_uuid(), 'gone', now() - interval '1 second', $3)
		RETURNING id`, connectionID, starterID, leaseLosses).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func user(t *testing.T, database *dbtest.Database, name string, role auth.Role) auth.Principal {
	t.Helper()
	row, err := database.Queries().UpsertUser(t.Context(), dbsqlc.UpsertUserParams{OidcIssuer: "issuer", OidcSubject: name, Email: name + "@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	return auth.Principal{Kind: auth.KindUser, ID: row.ID.String(), Name: name, Email: name + "@example.com", Role: role}
}

func (r *rig) connect(t *testing.T) grafanaconn.Connection {
	t.Helper()
	conn, err := r.svc.Create(t.Context(), r.admin, grafanaconn.Input{Name: "Production", URL: grafanaURL, Token: grafanaToken})
	if err != nil {
		t.Fatal(err)
	}
	return conn
}

func uuidOf(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	id, err := db.ParseUUID(s)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (r *rig) folderOf(t *testing.T, grafanaUID string) string {
	t.Helper()
	var title string
	err := r.db.SQLDB(t).QueryRowContext(t.Context(), `
		SELECT f.title FROM imports i
		JOIN dashboards d ON d.id = i.dashboard_id
		JOIN folders f ON f.id = d.folder_id
		WHERE i.grafana_uid = $1`, grafanaUID).Scan(&title)
	if err != nil {
		t.Fatalf("folder of %s: %v", grafanaUID, err)
	}
	return title
}

func (r *rig) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (r *rig) finish(t *testing.T, conn grafanaconn.Connection, req grafanaconn.SyncRequest) grafanaconn.Sync {
	t.Helper()
	started, err := r.svc.StartSync(t.Context(), r.admin, uuidOf(t, conn.ID), req)
	if err != nil {
		t.Fatal(err)
	}
	r.svc.Wait()
	done, err := r.svc.SyncStatus(t.Context(), uuidOf(t, conn.ID), started.ID)
	if err != nil {
		t.Fatal(err)
	}
	return done
}

func numbered(prefix string, i int) string {
	return prefix + "-" + strconv.Itoa(i)
}

func discard() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}
