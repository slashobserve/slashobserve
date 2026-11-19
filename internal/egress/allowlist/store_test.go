package allowlist

import (
	"errors"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

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
	if _, err := store.AddForConnection(t.Context(), by, pattern(t, "grafana.example.com")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Add(t.Context(), by, pattern(t, "manual.example.com")); err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"grafana.example.com", "manual.example.com"} {
		if err := store.Release(t.Context(), host); err != nil {
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
	if _, err := store.AddForConnection(t.Context(), by, pattern(t, "grafana.example.com")); err != nil {
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
	go func() { done <- store.Release(t.Context(), "grafana.example.com") }()
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
