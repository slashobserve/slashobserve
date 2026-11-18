package allowlist

import (
	"errors"
	"log/slog"
	"os"
	"testing"

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

	rows, err := database.SQLDB(t).QueryContext(t.Context(), `SELECT type, target_kind, data->>'domain', target_id::text FROM activity ORDER BY seq`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var got []string
	var ids []string
	for rows.Next() {
		var typ, kind, domain, id string
		if err := rows.Scan(&typ, &kind, &domain, &id); err != nil {
			t.Fatal(err)
		}
		got = append(got, typ+" "+kind+" "+domain)
		ids = append(ids, id)
	}
	want := []string{"outbound_domain.added outbound_domain grafana.example.com", "outbound_domain.removed outbound_domain grafana.example.com"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("events %v, want %v", got, want)
	}
	if ids[0] != ids[1] || ids[0] != activity.DomainID("grafana.example.com").String() {
		t.Fatalf("target ids %v differ for one domain", ids)
	}
}
