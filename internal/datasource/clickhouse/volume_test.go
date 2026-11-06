package clickhouse

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/logvolume"
)

const webLogsTemplate = "SELECT Timestamp, SeverityText, Body, ServiceName, TraceId, SpanId, LogAttributes FROM otel_logs WHERE $__timeFilter(Timestamp) ORDER BY Timestamp DESC LIMIT 1000"

const volumeFold = "multiIf(" +
	"lower(trimBoth(toString(`SeverityText`))) IN ('err', 'eror', 'error', 'emerg', 'fatal', 'alert', 'crit', 'critical', 'panic'), 'error', " +
	"lower(trimBoth(toString(`SeverityText`))) IN ('warn', 'warning'), 'warning', " +
	"lower(trimBoth(toString(`SeverityText`))) IN ('info', 'information', 'informational', 'notice'), 'info', 'other')"

func wrapped(inner string, seconds string) string {
	return "SELECT toStartOfInterval(`Timestamp`, INTERVAL " + seconds + " SECOND) AS Time, " + volumeFold +
		" AS Level, count() AS Count\nFROM (\n" + inner + "\n)\nGROUP BY Time, Level\nORDER BY Time"
}

func TestVolumeQueryWrapsTheWebLogsTemplate(t *testing.T) {
	got, err := VolumeQuery(webLogsTemplate, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	inner := "SELECT Timestamp, SeverityText, Body, ServiceName, TraceId, SpanId, LogAttributes FROM otel_logs WHERE $__timeFilter(Timestamp) "
	if want := wrapped(inner, "60"); got != want {
		t.Fatalf("query =\n%s\nwant\n%s", got, want)
	}
}

func TestVolumeQueryStepIsWholeSecondsAtLeastOne(t *testing.T) {
	for step, seconds := range map[time.Duration]string{500 * time.Millisecond: "1", 90 * time.Second: "90", 2500 * time.Millisecond: "2"} {
		got, err := VolumeQuery("SELECT 1", step)
		if err != nil || got != wrapped("SELECT 1", seconds) {
			t.Fatalf("step %v: query = %q, %v", step, got, err)
		}
	}
}

func TestVolumeQueryCutsTheTopLevelOrderAndLimit(t *testing.T) {
	for q, inner := range map[string]string{
		"SELECT * FROM otel_logs LIMIT 10 OFFSET 5":                          "SELECT * FROM otel_logs ",
		"SELECT * FROM otel_logs LIMIT 5, 10":                                "SELECT * FROM otel_logs ",
		"SELECT * FROM otel_logs;":                                           "SELECT * FROM otel_logs",
		"SELECT * FROM otel_logs ORDER BY Timestamp LIMIT 3;":                "SELECT * FROM otel_logs ",
		"SELECT * FROM otel_logs order by Timestamp desc":                    "SELECT * FROM otel_logs ",
		"SELECT * FROM (SELECT * FROM otel_logs ORDER BY Timestamp LIMIT 3)": "SELECT * FROM (SELECT * FROM otel_logs ORDER BY Timestamp LIMIT 3)",
		"SELECT * FROM otel_logs WHERE Body IN (SELECT Body FROM t LIMIT 3)": "SELECT * FROM otel_logs WHERE Body IN (SELECT Body FROM t LIMIT 3)",
		"SELECT 'ORDER BY x LIMIT 1' AS s, `limit` FROM otel_logs":           "SELECT 'ORDER BY x LIMIT 1' AS s, `limit` FROM otel_logs",
		"SELECT * FROM otel_logs -- ORDER BY Timestamp LIMIT 1":              "SELECT * FROM otel_logs -- ORDER BY Timestamp LIMIT 1",
		"SELECT * FROM otel_logs /* LIMIT 1 */ WHERE 1 ORDER BY Timestamp":   "SELECT * FROM otel_logs /* LIMIT 1 */ WHERE 1 ",
		"SELECT t.order, t.limit FROM otel_logs AS t":                        "SELECT t.order, t.limit FROM otel_logs AS t",
		"SELECT * FROM otel_logs ORDER BY Timestamp DESC LIMIT 1000 -- note": "SELECT * FROM otel_logs ",
	} {
		got, err := VolumeQuery(q, time.Minute)
		if err != nil {
			t.Fatalf("VolumeQuery(%q): %v", q, err)
		}
		if want := wrapped(inner, "60"); got != want {
			t.Errorf("VolumeQuery(%q) =\n%s\nwant\n%s", q, got, want)
		}
	}
}

func TestVolumeQueryKeepsATrailingCommentOffTheClosingParen(t *testing.T) {
	got, err := VolumeQuery("SELECT * FROM otel_logs -- note", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "-- note\n)\nGROUP BY") {
		t.Fatalf("query = %q", got)
	}
}

func TestVolumeQueryRefusesSetOperatorsAndLimitBy(t *testing.T) {
	for _, q := range []string{
		"SELECT * FROM a UNION ALL SELECT * FROM b",
		"SELECT * FROM a EXCEPT SELECT * FROM b",
		"SELECT * FROM a INTERSECT SELECT * FROM b",
		"SELECT * FROM otel_logs LIMIT 1 BY ServiceName",
		"SELECT * FROM otel_logs ORDER BY Timestamp LIMIT 1 BY ServiceName LIMIT 10",
	} {
		_, err := VolumeQuery(q, time.Minute)
		if !errors.Is(err, datasource.ErrInvalidQuery) || !strings.HasSuffix(err.Error(), volumeNeedsSelect) {
			t.Errorf("VolumeQuery(%q) err = %v, want %q", q, err, volumeNeedsSelect)
		}
	}
	if _, err := VolumeQuery("SELECT * FROM (SELECT 1 UNION ALL SELECT 2)", time.Minute); err != nil {
		t.Fatalf("a UNION inside a subquery was refused: %v", err)
	}
}

func TestVolumeQueryReturnsTokenizeErrors(t *testing.T) {
	if _, err := VolumeQuery("SELECT 'open", time.Minute); err == nil {
		t.Fatal("an unterminated string was accepted")
	}
}

func TestVolumeFoldNamesEveryAliasOnce(t *testing.T) {
	fold := volumeLevelFold()
	if fold != volumeFold {
		t.Fatalf("fold =\n%s\nwant\n%s", fold, volumeFold)
	}
	for _, kind := range logvolume.Kinds {
		for _, w := range logvolume.Aliases(kind) {
			want := 1
			if w == string(kind) {
				want++
			}
			if n := strings.Count(fold, "'"+w+"'"); n != want {
				t.Errorf("alias %q appears %d times, want %d", w, n, want)
			}
		}
	}
}

func TestGuardAcceptsTheVolumeQuery(t *testing.T) {
	q, err := VolumeQuery(strings.Replace(webLogsTemplate, "$__timeFilter(Timestamp)", "Timestamp >= now() - INTERVAL 1 HOUR", 1), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := guard(q); err != nil {
		t.Fatalf("guard refused the volume query: %v", err)
	}
}
