package datasourceadmin

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
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
	registry *datasource.Registry
	slots    *slotTotals
}

func newRig(t *testing.T, key sealed.Key, file ...datasource.Instance) (*rig, *Service) {
	t.Helper()
	database := dbtest.New(t)
	registry, err := datasource.NewRegistry(nil, file...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = registry.Close() })
	r := &rig{db: database.DB, registry: registry, slots: &slotTotals{}}
	return r, New(Deps{DB: database.DB, Registry: registry, Slots: r.slots, Key: key})
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
	row, err := r.db.Queries().InsertDatasource(t.Context(), dbsqlc.InsertDatasourceParams{
		ID: id, Uid: def.UID, Type: def.Type, Name: def.Name, Config: config,
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

func newID() pgtype.UUID {
	var id pgtype.UUID
	_, _ = rand.Read(id.Bytes[:])
	id.Bytes[6] = id.Bytes[6]&0x0f | 0x40
	id.Bytes[8] = id.Bytes[8]&0x3f | 0x80
	id.Valid = true
	return id
}

func upstream(t *testing.T, seenAuth *string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if seenAuth != nil {
			*seenAuth = r.Header.Get("Authorization")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
	}))
	t.Cleanup(server.Close)
	return server
}

func health(t *testing.T, r *rig, uid string) datasource.Status {
	t.Helper()
	status, ok := r.registry.Recheck(context.Background(), uid)
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

	if err := svc.Load(t.Context()); err != nil {
		t.Fatal(err)
	}

	in, ok := r.registry.Get("added")
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

	if err := svc.Load(t.Context()); err != nil {
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

	if err := svc.Load(t.Context()); err != nil {
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

	if err := svc.Load(t.Context()); err != nil {
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
	if err := svc.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}

	renamed := prometheus("added", server.URL)
	renamed.Name = "Renamed"
	config, _ := json.Marshal(renamed)
	if _, err := r.db.Queries().UpdateDatasource(t.Context(), dbsqlc.UpdateDatasourceParams{
		Uid: row.Uid, Name: renamed.Name, Config: config, SecretNames: []string{},
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	if in, _ := r.registry.Get("added"); in.Name != "Renamed" {
		t.Fatalf("name after an edit by another server = %q", in.Name)
	}

	if _, err := r.db.Queries().DeleteDatasource(t.Context(), "added"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.registry.Get("added"); ok {
		t.Fatal("a row deleted by another server is still registered")
	}
}

func TestReconcileNeverReplacesAFileDatasource(t *testing.T) {
	server := upstream(t, nil)
	file := datasource.Instance{UID: "prom", Name: "From file", Datasource: datasource.NewUnavailable("prometheus", datasource.ErrUnavailable), Source: datasource.SourceFile}
	r, svc := newRig(t, testKey, file)
	r.insert(t, testKey, prometheus("prom", server.URL), Secrets{})

	if err := svc.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}

	in, _ := r.registry.Get("prom")
	if in.Source != datasource.SourceFile || in.Name != "From file" {
		t.Fatalf("instance = %+v, the file datasource must win", in)
	}
}
