package datasourceadmin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
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
	"github.com/slashobserve/slashobserve/internal/egress/allowlist"
	"github.com/slashobserve/slashobserve/internal/egress/domains"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/internal/sealed"
)

var testKey = sealed.Key("0123456789abcdef0123456789abcdef")

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

type slotTotals struct {
	mu     sync.Mutex
	totals map[string]int
}

func (s *slotTotals) SetTotal(uid string, total int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.totals == nil {
		s.totals = map[string]int{}
	}
	if total < 1 {
		delete(s.totals, uid)
		return
	}
	s.totals[uid] = total
}

func (s *slotTotals) total(uid string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.totals[uid]
}

type rig struct {
	db       *db.DB
	database *dbtest.Database
	registry *datasource.Registry
	slots    *slotTotals
	domains  *allowlist.Store
	access   *folders.Access
	admin    auth.Principal
}

type rigOptions struct {
	key  sealed.Key
	lock allowlist.Lock
	file []datasource.Instance
}

func newRig(t *testing.T, key sealed.Key, file ...datasource.Instance) (*rig, *Service) {
	t.Helper()
	return newRigWith(t, rigOptions{key: key, file: file})
}

func newRigWith(t *testing.T, opts rigOptions) (*rig, *Service) {
	t.Helper()
	database := dbtest.New(t)
	registry, err := datasource.NewRegistry(nil, opts.file...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = registry.Close() })
	r := &rig{
		db: database.DB, database: database, registry: registry, slots: &slotTotals{},
		domains: allowlist.New(database.DB, opts.lock, slog.New(slog.DiscardHandler)),
		access:  folders.NewAccess(database.DB),
	}
	user, err := database.Queries().UpsertUser(reading(t.Context()), dbsqlc.UpsertUserParams{OidcIssuer: "issuer", OidcSubject: "ada", Email: "ada@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	r.admin = auth.Principal{Kind: auth.KindUser, ID: user.ID.String(), Name: "ada", Email: "ada@example.com", Role: auth.RoleAdmin}
	return r, New(Deps{
		DB: database.DB, Registry: registry, Slots: r.slots, Key: opts.key,
		Domains: r.domains, OutboundLocked: opts.lock.Locked, Access: r.access,
	})
}

func prometheus(uid, address string) Definition {
	return Definition{UID: uid, Type: TypePrometheus, Name: "Metrics " + uid, Address: address}
}

func (r *rig) insert(t *testing.T, key sealed.Key, def Definition, secrets Secrets) dbsqlc.Datasource {
	t.Helper()
	id := newID()
	config, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	sealedSecrets, err := sealSecrets(key, id, secrets)
	if err != nil {
		t.Fatal(err)
	}
	row, err := r.db.Queries().InsertDatasource(reading(t.Context()), dbsqlc.InsertDatasourceParams{
		ID: id, Uid: def.UID, Type: def.Type, Name: def.Name, Audience: string(normalizeDefinition(def).Audience), Config: config,
		Secrets: sealedSecrets, SecretNames: namesOrEmpty(secrets),
	})
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func namesOrEmpty(secrets Secrets) []string {
	if names := secrets.Names(); names != nil {
		return names
	}
	return []string{}
}

func upstream(t *testing.T, seenAuth *string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if seenAuth != nil {
			*seenAuth = r.Header.Get("Authorization")
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/query_range") {
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[{"metric":{},"values":[[1700000000,"1"],[1700000060,"2"]]}]}}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1700000000,"1"]}]}}`))
	}))
	t.Cleanup(server.Close)
	return server
}

func health(t *testing.T, r *rig, uid string) datasource.Status {
	t.Helper()
	status, ok := r.registry.Recheck(reading(context.Background()), uid)
	if !ok {
		t.Fatalf("datasource %q is not registered", uid)
	}
	return status
}

func TestLoadRegistersStoredDatasourcesAsAppSources(t *testing.T) {
	var auth string
	server := upstream(t, &auth)
	r, svc := newRig(t, testKey)
	def := prometheus("added", server.URL)
	def.GrafanaUIDs = []string{"grafana-prom"}
	def.Limits.MaxConcurrentQueries = 4
	r.insert(t, testKey, def, Secrets{BearerToken: "token-1"})

	if err := svc.Load(reading(t.Context())); err != nil {
		t.Fatal(err)
	}

	in, ok := registered(t, r.registry, "added")
	if !ok || in.Source != datasource.SourceApp || in.Name != "Metrics added" || len(in.GrafanaUIDs) != 1 {
		t.Fatalf("instance = %+v, registered = %v", in, ok)
	}
	if status := health(t, r, "added"); !status.Healthy || status.Source != datasource.SourceApp {
		t.Fatalf("status = %+v", status)
	}
	if auth != "Bearer token-1" {
		t.Fatalf("the upstream saw Authorization %q, want the stored bearer token", auth)
	}
	if got := r.slots.total("added"); got != 4 {
		t.Fatalf("slot total = %d, want 4", got)
	}
}

func TestLoadWithoutASecretKeyMarksCredentialedRowsUnavailable(t *testing.T) {
	server := upstream(t, nil)
	r, svc := newRig(t, nil)
	r.insert(t, testKey, prometheus("secured", server.URL), Secrets{BearerToken: "token-1"})
	r.insert(t, nil, prometheus("open", server.URL), Secrets{})

	if err := svc.Load(reading(t.Context())); err != nil {
		t.Fatal(err)
	}

	secured := health(t, r, "secured")
	if secured.Healthy || secured.Message != "This datasource's credentials need SLASHOBSERVE_SECRET_KEY on the server" {
		t.Fatalf("secured = %+v", secured)
	}
	if open := health(t, r, "open"); !open.Healthy {
		t.Fatalf("a row without credentials needs no key: %+v", open)
	}
}

func TestLoadWithAnotherSecretKeyReportsUnreadableCredentials(t *testing.T) {
	server := upstream(t, nil)
	r, svc := newRig(t, sealed.Key("another-key-another-key-another-k"))
	r.insert(t, testKey, prometheus("secured", server.URL), Secrets{BearerToken: "token-1"})

	if err := svc.Load(reading(t.Context())); err != nil {
		t.Fatal(err)
	}

	status := health(t, r, "secured")
	if status.Healthy || status.Message != "The saved credentials cannot be read with the server's secret key; set them again" {
		t.Fatalf("status = %+v", status)
	}
}

func TestLoadRegistersAnInvalidRowAsUnavailableWithoutEchoingIt(t *testing.T) {
	r, svc := newRig(t, testKey)
	r.insert(t, testKey, Definition{UID: "broken", Type: TypePrometheus, Name: "Broken"}, Secrets{})

	if err := svc.Load(reading(t.Context())); err != nil {
		t.Fatal(err)
	}

	status := health(t, r, "broken")
	if status.Healthy || status.Message != "The saved definition cannot be used; open the datasource and save it again" {
		t.Fatalf("status = %+v", status)
	}
}

func TestReconcileAppliesChangesAndRemovals(t *testing.T) {
	server := upstream(t, nil)
	r, svc := newRig(t, testKey)
	row := r.insert(t, testKey, prometheus("added", server.URL), Secrets{})
	if err := svc.Reconcile(reading(t.Context())); err != nil {
		t.Fatal(err)
	}

	renamed := prometheus("added", server.URL)
	renamed.Name = "Renamed"
	config, _ := json.Marshal(renamed)
	if _, err := r.db.Queries().UpdateDatasource(reading(t.Context()), dbsqlc.UpdateDatasourceParams{
		Uid: row.Uid, Name: renamed.Name, Audience: string(datasource.AudienceEveryone), Config: config, SecretNames: []string{},
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reconcile(reading(t.Context())); err != nil {
		t.Fatal(err)
	}
	if in, _ := registered(t, r.registry, "added"); in.Name != "Renamed" {
		t.Fatalf("name after an edit by another server = %q", in.Name)
	}

	if _, err := r.db.Queries().DeleteDatasource(reading(t.Context()), "added"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reconcile(reading(t.Context())); err != nil {
		t.Fatal(err)
	}
	if _, ok := registered(t, r.registry, "added"); ok {
		t.Fatal("a row deleted by another server is still registered")
	}
}

func TestReconcileNeverReplacesAFileDatasource(t *testing.T) {
	server := upstream(t, nil)
	file := datasource.Instance{UID: "prom", Name: "From file", Datasource: datasource.NewUnavailable("prometheus", datasource.ErrUnavailable), Source: datasource.SourceFile}
	r, svc := newRig(t, testKey, file)
	r.insert(t, testKey, prometheus("prom", server.URL), Secrets{})

	if err := svc.Reconcile(reading(t.Context())); err != nil {
		t.Fatal(err)
	}

	in, _ := registered(t, r.registry, "prom")
	if in.Source != datasource.SourceFile || in.Name != "From file" {
		t.Fatalf("instance = %+v, the file datasource must win", in)
	}
}

func addDashboard(t *testing.T, r *rig, title, datasourceUID string) {
	t.Helper()
	logger := slog.New(slog.DiscardHandler)
	service := dashboards.New(r.db, queryproxy.NewRunner(r.registry, logger), r.access, logger)
	spec := fmt.Sprintf(`{"specVersion":"0","title":%q,"description":"","tags":[],"time":{"from":"now-15m","to":"now"},"panels":[{"id":"p","type":"timeseries","title":"p","gridPos":{"x":0,"y":0,"w":12,"h":8},"datasource":%q,"queries":[{"query":"up"}]}]}`, title, datasourceUID)
	if _, _, err := service.Create(reading(t.Context()), r.admin, pgtype.UUID{}, []byte(spec), "create"); err != nil {
		var invalidSpec *dashboards.InvalidSpecError
		if errors.As(err, &invalidSpec) {
			t.Fatalf("%v: %+v", err, invalidSpec.Issues)
		}
		t.Fatal(err)
	}
}

func TestRemovingAShadowedRowKeepsTheFileDatasourcesConcurrencyLimit(t *testing.T) {
	server := upstream(t, nil)
	file := datasource.Instance{UID: "prom", Name: "From file", Datasource: datasource.NewUnavailable("prometheus", datasource.ErrUnavailable), Source: datasource.SourceFile}
	r, svc := newRig(t, testKey, file)
	r.slots.SetTotal("prom", 2)
	r.insert(t, testKey, prometheus("prom", server.URL), Secrets{})
	if err := svc.Reconcile(reading(t.Context())); err != nil {
		t.Fatal(err)
	}

	if _, err := r.db.Queries().DeleteDatasource(reading(t.Context()), "prom"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reconcile(reading(t.Context())); err != nil {
		t.Fatal(err)
	}

	if got := r.slots.total("prom"); got != 2 {
		t.Fatalf("slot total of the file datasource = %d, want 2", got)
	}
}

func TestAPollThatReadTheRowsBeforeACreateDoesNotUndoIt(t *testing.T) {
	server := upstream(t, nil)
	r, svc := newRig(t, testKey)
	read := svc.list
	listed := make(chan struct{})
	proceed := make(chan struct{})
	svc.list = func(ctx context.Context) ([]dbsqlc.Datasource, error) {
		rows, err := read(ctx)
		close(listed)
		<-proceed
		return rows, err
	}

	polled := make(chan error, 1)
	go func() { polled <- svc.Reconcile(reading(t.Context())) }()
	<-listed

	created := make(chan error, 1)
	go func() {
		_, err := svc.Create(reading(t.Context()), r.admin, prometheus("added", server.URL), Secrets{})
		created <- err
	}()
	select {
	case err := <-created:
		if err != nil {
			t.Fatal(err)
		}
		close(proceed)
		<-polled
	case <-time.After(300 * time.Millisecond):
		close(proceed)
		<-polled
		if err := <-created; err != nil {
			t.Fatal(err)
		}
	}

	if _, ok := registered(t, r.registry, "added"); !ok {
		t.Fatal("the poll's older snapshot removed the datasource that was just added")
	}
}

func outboundBlockedMessage() string {
	message, _, _ := datasource.Reason(datasource.ErrOutboundBlocked)
	return message
}

func TestLoadRefusesAnIPAddressOnceTheAllowlistIsLocked(t *testing.T) {
	server := upstream(t, nil)
	unlocked, unlockedSvc := newRig(t, testKey)
	unlocked.insert(t, testKey, prometheus("added", server.URL), Secrets{})
	if err := unlockedSvc.Load(reading(t.Context())); err != nil {
		t.Fatal(err)
	}
	if status := health(t, unlocked, "added"); !status.Healthy {
		t.Fatalf("an IP address on an unlocked list: %+v", status)
	}

	locked, lockedSvc := newRigWith(t, rigOptions{key: testKey, lock: allowlist.Lock{Locked: true}})
	locked.insert(t, testKey, prometheus("added", server.URL), Secrets{})
	if err := lockedSvc.Load(reading(t.Context())); err != nil {
		t.Fatal(err)
	}
	if status := health(t, locked, "added"); status.Healthy || status.Message != outboundBlockedMessage() {
		t.Fatalf("an IP address on a locked list: %+v", status)
	}
}

func TestReconcileFollowsTheAllowlistForStoredDomains(t *testing.T) {
	r, svc := newRig(t, testKey)
	r.insert(t, testKey, prometheus("added", "http://prom.example.invalid:9090"), Secrets{})
	if err := svc.Reconcile(reading(t.Context())); err != nil {
		t.Fatal(err)
	}
	if status := health(t, r, "added"); status.Message != outboundBlockedMessage() {
		t.Fatalf("a domain that is not listed: %+v", status)
	}

	pattern, _ := domains.ParsePattern("prom.example.invalid")
	if _, err := r.domains.Add(reading(t.Context()), r.admin, pattern); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reconcile(reading(t.Context())); err != nil {
		t.Fatal(err)
	}
	if status := health(t, r, "added"); status.Message == outboundBlockedMessage() {
		t.Fatalf("the datasource stayed blocked after its domain was allowed: %+v", status)
	}

	if _, err := r.db.Queries().DeleteOutboundDomain(reading(t.Context()), pattern.String()); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reconcile(reading(t.Context())); err != nil {
		t.Fatal(err)
	}
	if status := health(t, r, "added"); status.Message != outboundBlockedMessage() {
		t.Fatalf("the datasource kept connecting after its domain was removed: %+v", status)
	}
}
