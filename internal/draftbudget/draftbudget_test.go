package draftbudget

import (
	"os"
	"testing"

	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

func TestMain(m *testing.M) { os.Exit(dbtest.Main(m)) }

func TestSpendAllowsTheLimitThenRefusesWithTheTimeLeft(t *testing.T) {
	store := New(dbtest.New(t).DB)
	for i := range MaxPerWindow {
		allowed, _, err := store.Spend(t.Context(), "user:a")
		if err != nil || !allowed {
			t.Fatalf("spend %d: allowed %v err %v", i+1, allowed, err)
		}
	}

	allowed, resetsIn, err := store.Spend(t.Context(), "user:a")

	if err != nil || allowed || resetsIn <= 0 || resetsIn > Window {
		t.Fatalf("after the limit: allowed %v resetsIn %v err %v", allowed, resetsIn, err)
	}
}

func TestSpendCountsEachPersonSeparately(t *testing.T) {
	store := New(dbtest.New(t).DB)
	for range MaxPerWindow {
		if _, _, err := store.Spend(t.Context(), "user:a"); err != nil {
			t.Fatal(err)
		}
	}

	allowed, _, err := store.Spend(t.Context(), "service_account:b")

	if err != nil || !allowed {
		t.Fatalf("another caller: allowed %v err %v", allowed, err)
	}
}

func TestSpendDropsTheCallersRowsFromEarlierWindows(t *testing.T) {
	database := dbtest.New(t)
	database.Exec(t, "INSERT INTO draft_query_spend (principal_key, window_start, calls) VALUES ('user:a', now() - interval '3 hours', 5)")

	if _, _, err := New(database.DB).Spend(t.Context(), "user:a"); err != nil {
		t.Fatal(err)
	}

	var rows int
	if err := database.SQLDB(t).QueryRowContext(t.Context(), "SELECT count(*) FROM draft_query_spend").Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("rows %d err %v, want 1", rows, err)
	}
}
