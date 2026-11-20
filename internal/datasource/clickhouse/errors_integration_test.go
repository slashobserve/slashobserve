package clickhouse_test

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func TestQueryAuthorErrorsKeepTheirMessage(t *testing.T) {
	ctx := context.Background()
	t.Run("unknown aggregate function", func(t *testing.T) {
		_, err := roDS(t).QueryInstant(ctx, query("SELECT avgg(1)"), lastHour())
		if !errors.Is(err, datasource.ErrInvalidQuery) || strings.Contains(err.Error(), "ClickHouse error") || !strings.Contains(err.Error(), "avgg") {
			t.Fatalf("err = %v, want the real message naming avgg", err)
		}
	})
	t.Run("throwIf message is scrubbed of addresses", func(t *testing.T) {
		_, err := roDS(t).QueryInstant(ctx, query("SELECT throwIf(1, 'backend 10.1.2.3:9000 down')"), lastHour())
		if !errors.Is(err, datasource.ErrInvalidQuery) || regexp.MustCompile(`10\.1\.2\.3`).MatchString(err.Error()) {
			t.Fatalf("err = %v, want ErrInvalidQuery without the address", err)
		}
	})
	t.Run("throwIf without an address keeps its message", func(t *testing.T) {
		_, err := roDS(t).QueryInstant(ctx, query("SELECT throwIf(1, 'dashboard guard tripped')"), lastHour())
		if !errors.Is(err, datasource.ErrInvalidQuery) || !strings.Contains(err.Error(), "dashboard guard tripped") {
			t.Fatalf("err = %v, want the throwIf message", err)
		}
	})
}

func TestProfileMaximumBelowDriverSlack(t *testing.T) {
	if testsSkipped {
		t.Skip("integration tests need Docker")
	}
	ctx := context.Background()
	admin, err := rawConn("admin", adminPass)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	const user = "slashobserve_lowmax"
	for _, stmt := range []string{
		"CREATE SETTINGS PROFILE IF NOT EXISTS slashobserve_lowmax_profile SETTINGS readonly = 2 CONST, max_execution_time = 5 MIN 1 MAX 8",
		"CREATE USER IF NOT EXISTS " + user + " IDENTIFIED WITH sha256_password BY '" + roPass + "' SETTINGS PROFILE 'slashobserve_lowmax_profile'",
		"GRANT SELECT ON otel.* TO " + user,
	} {
		if err := admin.Exec(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_ = admin.Exec(ctx, "DROP USER IF EXISTS "+user)
		_ = admin.Exec(ctx, "DROP SETTINGS PROFILE IF EXISTS slashobserve_lowmax_profile")
	})
	ds := newDS(t, user, roPass, datasource.Limits{})
	if err := ds.CheckHealth(ctx); err != nil {
		t.Fatalf("CheckHealth: %v", err)
	}
	if _, err := ds.QueryInstant(ctx, query("SELECT 1"), lastHour()); err != nil {
		t.Fatalf("QueryInstant: %v", err)
	}
}

func TestASyntaxErrorNamesThePlaceInThePersonsOwnQuery(t *testing.T) {
	ctx := context.Background()
	text := "SELECT ServiceName, count() FROM otel_traces\nWHERE $__timeFilter(Timestamp) GROUP"
	_, _, paged := roDS(t).QueryPage(ctx, datasource.PageRequest{Query: datasource.Query{Text: text}, Range: lastHour(), Page: 1, PageSize: 50})
	_, plain := roDS(t).QueryInstant(ctx, datasource.Query{Text: text, Explore: true}, lastHour())
	for name, err := range map[string]error{"paged": paged, "instant": plain} {
		if !errors.Is(err, datasource.ErrInvalidQuery) {
			t.Fatalf("%s: err = %v, want an invalid query", name, err)
		}
		message := err.Error()
		if !regexp.MustCompile(`Syntax error: failed at (line 2, column \d+ of the query|the end of the query \(line 2, column 37\))`).MatchString(message) {
			t.Errorf("%s: err = %v, want the place in the person's text", name, err)
		}
		for _, leak := range []string{"position", "ORDER BY ALL", "{limit", "fromUnixTimestamp64Milli"} {
			if strings.Contains(message, leak) {
				t.Errorf("%s: err = %v, names %q", name, err, leak)
			}
		}
	}
}
