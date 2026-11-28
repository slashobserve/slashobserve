package allowlist

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/config"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/egress/domains"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

func pattern(t *testing.T, entry string) domains.Pattern {
	t.Helper()
	p, err := domains.ParsePattern(entry)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestAddStoresTheDomainOnceAndCountsTheLimit(t *testing.T) {
	database := dbtest.New(t)
	store := New(database.DB, Lock{}, slog.New(slog.DiscardHandler))
	user, err := database.Queries().UpsertUser(t.Context(), dbsqlc.UpsertUserParams{OidcIssuer: "i", OidcSubject: "s", Email: "ada@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	ada := auth.Principal{Kind: auth.KindUser, ID: user.ID.String(), Email: "ada@example.com", Role: auth.RoleAdmin}
	key := auth.Principal{Kind: auth.KindServiceAccount, ID: "3f1c2a52-8c5e-4a53-9a3e-1d2d4c0a9b11", Name: "ci", Role: auth.RoleAdmin}

	row, err := store.Add(t.Context(), ada, pattern(t, "Grafana.Example.com"))
	if err != nil || row.Domain != "grafana.example.com" || row.CreatedByName != "ada@example.com" {
		t.Fatalf("row %+v, err %v", row, err)
	}
	if row, err := store.Add(t.Context(), key, pattern(t, "grafana.ci.example.com")); err != nil || row.CreatedByName != "ci" {
		t.Fatalf("service account add: row %+v, err %v", row, err)
	}
	if _, err := store.Add(t.Context(), ada, pattern(t, "grafana.example.com")); !errors.Is(err, ErrExists) {
		t.Fatalf("second add error = %v, want ErrExists", err)
	}
	list, err := store.Allowed(t.Context())
	if err != nil || !list.Allows("grafana.example.com") {
		t.Fatalf("allowed %v, err %v", list, err)
	}

	for i := len(list); i < config.MaxOutboundDomains; i++ {
		if _, err := store.Add(t.Context(), ada, pattern(t, "d"+string(rune('a'+i%26))+string(rune('a'+i/26))+".example.com")); err != nil {
			t.Fatalf("add %d: %v", i, err)
		}
	}
	if _, err := store.Add(t.Context(), ada, pattern(t, "one-more.example.com")); !errors.Is(err, ErrTooMany) {
		t.Fatalf("add past the limit error = %v, want ErrTooMany", err)
	}
}

func TestAddRefusesWhenConfigurationLocksTheList(t *testing.T) {
	store := New(nil, Lock{Locked: true, Domains: parseList(t, "grafana.example.com")}, slog.New(slog.DiscardHandler))
	if _, err := store.Add(t.Context(), auth.Principal{}, pattern(t, "other.example.com")); !errors.Is(err, ErrLocked) {
		t.Fatalf("error = %v, want ErrLocked", err)
	}
}

func TestUsersNameTheWebhooksAndConnectionsAnEntryAllows(t *testing.T) {
	rows := []dbsqlc.ListOutboundUsersRow{
		{Kind: "webhook", Name: "Deploys", Url: "https://Hooks.Example.com/in"},
		{Kind: "grafana_connection", Name: "Prod", Url: "http://grafana.example.com:3000"},
		{Kind: "webhook", Name: "Other", Url: "https://elsewhere.test/in"},
	}
	exact := usersOf(rows, pattern(t, "hooks.example.com"))
	if len(exact) != 1 || exact[0] != (User{Kind: "webhook", Name: "Deploys"}) {
		t.Fatalf("exact entry users = %+v", exact)
	}
	wild := usersOf(rows, pattern(t, "*.example.com"))
	if len(wild) != 2 {
		t.Fatalf("wildcard entry users = %+v", wild)
	}
	if none := usersOf(rows, pattern(t, "unused.example.com")); none == nil || len(none) != 0 {
		t.Fatalf("unused entry users = %#v, want an empty list", none)
	}
}

func TestReleaseRemovesOnlyAnUnusedConnectionEntry(t *testing.T) {
	database := dbtest.New(t)
	store := New(database.DB, Lock{}, slog.New(slog.DiscardHandler))
	by := auth.Principal{Kind: auth.KindServiceAccount, ID: "3f1c2a52-8c5e-4a53-9a3e-1d2d4c0a9b11", Name: "ci", Role: auth.RoleAdmin}
	if _, err := store.SaveOnAllowedHost(t.Context(), by, "grafana.example.com", func(*dbsqlc.Queries) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Add(t.Context(), by, pattern(t, "manual.example.com")); err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"grafana.example.com", "manual.example.com"} {
		if err := store.Release(t.Context(), by, host); err != nil {
			t.Fatal(err)
		}
	}
	list, err := store.Allowed(t.Context())
	if err != nil || list.Allows("grafana.example.com") || !list.Allows("manual.example.com") {
		t.Fatalf("allowlist %v, err %v", list, err)
	}
}

func TestReleaseWaitsForTheLockAWebhookTakesWhileItIsSaved(t *testing.T) {
	database := dbtest.New(t)
	store := New(database.DB, Lock{}, slog.New(slog.DiscardHandler))
	by := auth.Principal{Kind: auth.KindServiceAccount, ID: "3f1c2a52-8c5e-4a53-9a3e-1d2d4c0a9b11", Name: "ci", Role: auth.RoleAdmin}
	if _, err := store.SaveOnAllowedHost(t.Context(), by, "grafana.example.com", func(*dbsqlc.Queries) error { return nil }); err != nil {
		t.Fatal(err)
	}
	held := make(chan struct{})
	free := make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		finished <- database.InTx(t.Context(), func(q *dbsqlc.Queries) error {
			if err := q.LockOutboundDomains(t.Context()); err != nil {
				return err
			}
			close(held)
			<-free
			return nil
		})
	}()
	<-held
	var once sync.Once
	release := func() {
		once.Do(func() {
			close(free)
			if err := <-finished; err != nil {
				t.Error(err)
			}
		})
	}
	t.Cleanup(release)
	done := make(chan error, 1)
	go func() { done <- store.Release(t.Context(), by, "grafana.example.com") }()
	select {
	case err := <-done:
		t.Fatalf("release finished (%v) while another writer held the lock", err)
	case <-time.After(300 * time.Millisecond):
	}
	release()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func serveWithDatabase(t *testing.T, store *Store, caller auth.Principal, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), caller))
	})
	store.Register(auth.NewRoutes(api))
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(method, "/api/v1/outbound-domains"+path, strings.NewReader(body)))
	return rec
}

func userPrincipal(t *testing.T, database *dbtest.Database, email string, role auth.Role) auth.Principal {
	t.Helper()
	user, err := database.Queries().UpsertUser(t.Context(), dbsqlc.UpsertUserParams{OidcIssuer: "i", OidcSubject: email, Email: email})
	if err != nil {
		t.Fatal(err)
	}
	return auth.Principal{Kind: auth.KindUser, ID: user.ID.String(), Email: email, Role: role}
}

func TestOnlyAdminsSeeWhichWebhooksAndConnectionsUseAnEntry(t *testing.T) {
	database := dbtest.New(t)
	store := New(database.DB, Lock{}, slog.New(slog.DiscardHandler))
	admin := userPrincipal(t, database, "ada@example.com", auth.RoleAdmin)
	editor := userPrincipal(t, database, "eve@example.com", auth.RoleEditor)
	if _, err := store.Add(t.Context(), admin, pattern(t, "hooks.example.com")); err != nil {
		t.Fatal(err)
	}
	database.Exec(t, `INSERT INTO webhooks (name, url, secret_ciphertext, event_types, owner_kind, owner_id, cursor_xid, cursor_seq)
		VALUES ('pagerduty-prod-oncall', 'https://hooks.example.com/in', '\x00', ARRAY['finding.created'], 'user', $1::uuid, '0'::xid8, 0)`, admin.ID)

	type listed struct {
		Domains []struct {
			Domain string `json:"domain"`
			UsedBy []struct {
				Name string `json:"name"`
			} `json:"used_by"`
		} `json:"domains"`
	}
	read := func(caller auth.Principal) listed {
		rec := serveWithDatabase(t, store, caller, http.MethodGet, "", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("GET as %s = %d %s", caller.Role, rec.Code, rec.Body)
		}
		var got listed
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	if got := read(admin); len(got.Domains) != 1 || len(got.Domains[0].UsedBy) != 1 || got.Domains[0].UsedBy[0].Name != "pagerduty-prod-oncall" {
		t.Fatalf("admin list %+v, want the webhook named", got)
	}
	got := read(editor)
	if len(got.Domains) != 1 || got.Domains[0].UsedBy == nil || len(got.Domains[0].UsedBy) != 0 {
		t.Fatalf("editor list %+v, want the domain with an empty used_by", got)
	}
}

func TestAnAdminAllowingAConnectionsHostByHandKeepsItWhenTheConnectionGoes(t *testing.T) {
	database := dbtest.New(t)
	store := New(database.DB, Lock{}, slog.New(slog.DiscardHandler))
	admin := userPrincipal(t, database, "ada@example.com", auth.RoleAdmin)
	if _, err := store.SaveOnAllowedHost(t.Context(), admin, "grafana.example.com", func(*dbsqlc.Queries) error { return nil }); err != nil {
		t.Fatal(err)
	}
	rec := serveWithDatabase(t, store, admin, http.MethodPost, "", `{"domain":"grafana.example.com"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"from_connection":false`) {
		t.Fatalf("allowing by hand answered %d %s, want 200 with from_connection false", rec.Code, rec.Body)
	}
	if err := store.Release(t.Context(), admin, "grafana.example.com"); err != nil {
		t.Fatal(err)
	}
	if list, err := store.Allowed(t.Context()); err != nil || !list.Allows("grafana.example.com") {
		t.Fatalf("allowlist %v, err %v, want the hand-allowed host kept", list, err)
	}
	if rec := serveWithDatabase(t, store, admin, http.MethodPost, "", `{"domain":"grafana.example.com"}`); rec.Code != http.StatusConflict {
		t.Fatalf("allowing it again answered %d %s, want 409", rec.Code, rec.Body)
	}
}

func TestAddAndRemoveRecordOneAdminEventEach(t *testing.T) {
	database := dbtest.New(t)
	store := New(database.DB, Lock{}, slog.New(slog.DiscardHandler))
	ada := auth.Principal{Kind: auth.KindServiceAccount, ID: "3f1c2a52-8c5e-4a53-9a3e-1d2d4c0a9b11", Name: "ci", Role: auth.RoleAdmin}

	if _, err := store.Add(t.Context(), ada, pattern(t, "grafana.example.com")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Add(t.Context(), ada, pattern(t, "grafana.example.com")); !errors.Is(err, ErrExists) {
		t.Fatalf("second add error = %v", err)
	}
	if removed, err := store.Remove(t.Context(), ada, pattern(t, "grafana.example.com")); err != nil || !removed {
		t.Fatalf("remove = %v, %v", removed, err)
	}
	if removed, err := store.Remove(t.Context(), ada, pattern(t, "grafana.example.com")); err != nil || removed {
		t.Fatalf("second remove = %v, %v", removed, err)
	}

	got, ids := domainEvents(t, database)
	want := []string{"outbound_domain.added outbound_domain grafana.example.com", "outbound_domain.removed outbound_domain grafana.example.com"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("events %v, want %v", got, want)
	}
	if ids[0] != ids[1] || ids[0] != activity.DomainID("grafana.example.com").String() {
		t.Fatalf("target ids %v differ for one domain", ids)
	}
}

func TestAnEntryAddedAndReleasedForAConnectionIsRecorded(t *testing.T) {
	database := dbtest.New(t)
	store := New(database.DB, Lock{}, slog.New(slog.DiscardHandler))
	ada := auth.Principal{Kind: auth.KindServiceAccount, ID: "3f1c2a52-8c5e-4a53-9a3e-1d2d4c0a9b11", Name: "ci", Role: auth.RoleAdmin}

	added, err := store.SaveOnAllowedHost(t.Context(), ada, "grafana.example.com", func(*dbsqlc.Queries) error { return nil })
	if err != nil || added != "grafana.example.com" {
		t.Fatalf("save = %q, %v", added, err)
	}
	if err := store.Release(t.Context(), ada, "grafana.example.com"); err != nil {
		t.Fatal(err)
	}
	if err := store.Release(t.Context(), ada, "grafana.example.com"); err != nil {
		t.Fatal(err)
	}

	got, _ := domainEvents(t, database)
	want := []string{"outbound_domain.added outbound_domain grafana.example.com", "outbound_domain.removed outbound_domain grafana.example.com"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("events %v, want %v", got, want)
	}
}

func domainEvents(t *testing.T, database *dbtest.Database) (events, ids []string) {
	t.Helper()
	rows, err := database.SQLDB(t).QueryContext(t.Context(), `SELECT type, target_kind, data->>'domain', target_id::text FROM activity ORDER BY seq`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var typ, kind, domain, id string
		if err := rows.Scan(&typ, &kind, &domain, &id); err != nil {
			t.Fatal(err)
		}
		events = append(events, typ+" "+kind+" "+domain)
		ids = append(ids, id)
	}
	return events, ids
}
