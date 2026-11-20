package filter

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

var epoch = time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)

func TestAggregateCompilesToOneGroupByQuery(t *testing.T) {
	for _, tc := range []struct {
		name    string
		text    string
		sql     string
		args    []any
		levels  []Level
		metrics []Metric
	}{
		{
			name: "time series under a filter",
			text: `{"query":{"term":{"outcome":"denied"}},"aggs":{"t":{"date_histogram":{"field":"occurred_at","fixed_interval":"1h"}}}}`,
			sql: "WITH matched AS (SELECT date_bin(make_interval(secs => $4::double precision), occurred_at, $5::timestamptz) AS k1 FROM audit_events WHERE occurred_at >= $1 AND occurred_at < $2 AND COALESCE((outcome = $3), false))" +
				" SELECT matched.k1, count(*)::double precision AS doc_count FROM matched GROUP BY 1 ORDER BY 1 LIMIT $6",
			args:   []any{testWindow.From, testWindow.To, "denied", 3600.0, epoch, MaxAggRows + 1},
			levels: []Level{{Name: "t", Type: "date_histogram", Field: "occurred_at"}},
		},
		{
			name: "terms with metric leaves",
			text: `{"aggs":{"o":{"terms":{"field":"origin","size":3},"aggs":{"d":{"avg":{"field":"duration_ms"}},"u":{"cardinality":{"field":"actor_id"}}}}}}`,
			sql: "WITH matched AS (SELECT origin::text AS k1, duration_ms AS m1, actor_id AS m2 FROM audit_events WHERE occurred_at >= $1 AND occurred_at < $2 AND origin IS NOT NULL)," +
				" t1 AS (SELECT k1 FROM (SELECT matched.k1, row_number() OVER (ORDER BY count(*) DESC, matched.k1) AS rn FROM matched GROUP BY matched.k1) ranked WHERE rn <= $3)" +
				" SELECT matched.k1, count(*)::double precision AS doc_count, avg(matched.m1)::double precision, count(DISTINCT matched.m2)::double precision FROM matched JOIN t1 ON matched.k1 = t1.k1 GROUP BY 1 ORDER BY 1 LIMIT $4",
			args:    []any{testWindow.From, testWindow.To, 3, MaxAggRows + 1},
			levels:  []Level{{Name: "o", Type: "terms", Field: "origin"}},
			metrics: []Metric{{Name: "d", Type: "avg"}, {Name: "u", Type: "cardinality"}},
		},
		{
			name: "two terms levels ordered by key",
			text: `{"aggs":{"o":{"terms":{"field":"origin","size":3,"order":{"_key":"asc"}},"aggs":{"a":{"terms":{"field":"client_ip","size":2}}}}}}`,
			sql: "WITH matched AS (SELECT origin::text AS k1, host(client_ip) AS k2 FROM audit_events WHERE occurred_at >= $1 AND occurred_at < $2 AND origin IS NOT NULL AND client_ip IS NOT NULL)," +
				" t1 AS (SELECT k1 FROM (SELECT matched.k1, row_number() OVER (ORDER BY matched.k1 ASC) AS rn FROM matched GROUP BY matched.k1) ranked WHERE rn <= $3)," +
				" t2 AS (SELECT k1, k2 FROM (SELECT matched.k1, matched.k2, row_number() OVER (PARTITION BY matched.k1 ORDER BY count(*) DESC, matched.k2) AS rn FROM matched JOIN t1 ON matched.k1 = t1.k1 GROUP BY matched.k1, matched.k2) ranked WHERE rn <= $4)" +
				" SELECT matched.k1, matched.k2, count(*)::double precision AS doc_count FROM matched JOIN t2 ON matched.k1 = t2.k1 AND matched.k2 = t2.k2 GROUP BY 1, 2 ORDER BY 1, 2 LIMIT $5",
			args:   []any{testWindow.From, testWindow.To, 3, 2, MaxAggRows + 1},
			levels: []Level{{Name: "o", Type: "terms", Field: "origin"}, {Name: "a", Type: "terms", Field: "client_ip"}},
		},
		{
			name: "metrics only",
			text: `{"aggs":{"m":{"max":{"field":"duration_ms"}}}}`,
			sql:  "WITH matched AS (SELECT duration_ms AS m1 FROM audit_events WHERE occurred_at >= $1 AND occurred_at < $2) SELECT count(*)::double precision AS doc_count, max(matched.m1)::double precision FROM matched LIMIT $3",
			args: []any{testWindow.From, testWindow.To, MaxAggRows + 1},
			metrics: []Metric{
				{Name: "m", Type: "max"},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, err := mustParse(t, tc.text).Aggregate(testWindow)
			if err != nil {
				t.Fatal(err)
			}
			if a.SQL != tc.sql {
				t.Errorf("SQL\n got %s\nwant %s", a.SQL, tc.sql)
			}
			if !reflect.DeepEqual(a.Args, tc.args) {
				t.Errorf("args\n got %#v\nwant %#v", a.Args, tc.args)
			}
			if len(a.Levels)+len(tc.levels) > 0 && !reflect.DeepEqual(a.Levels, tc.levels) {
				t.Errorf("levels = %+v, want %+v", a.Levels, tc.levels)
			}
			if len(a.Metrics)+len(tc.metrics) > 0 && !reflect.DeepEqual(a.Metrics, tc.metrics) {
				t.Errorf("metrics = %+v, want %+v", a.Metrics, tc.metrics)
			}
			if a.RowLimit != MaxAggRows+1 {
				t.Errorf("row limit = %d", a.RowLimit)
			}
			assertSafeSQL(t, a.SQL, a.Args)
		})
	}
}

func TestAggregatePercentilesNameEveryKey(t *testing.T) {
	q := mustParse(t, `{"aggs":{"t":{"date_histogram":{"field":"occurred_at","fixed_interval":"5m"},"aggs":{"p":{"percentiles":{"field":"duration_ms","percents":[50,99.9]}}}}}}`)
	a, err := q.Aggregate(testWindow)
	if err != nil {
		t.Fatal(err)
	}
	want := []Metric{{Name: "p", Type: "percentiles", Key: "50"}, {Name: "p", Type: "percentiles", Key: "99.9"}}
	if !reflect.DeepEqual(a.Metrics, want) {
		t.Errorf("metrics = %+v, want %+v", a.Metrics, want)
	}
	if got := strings.Count(a.SQL, "percentile_cont("); got != 2 {
		t.Errorf("SQL has %d percentile_cont calls: %s", got, a.SQL)
	}
	if !a.HasTimeBucket() {
		t.Error("HasTimeBucket = false")
	}
	fraction := 99.9
	if !reflect.DeepEqual(a.Args[len(a.Args)-3:len(a.Args)-1], []any{0.5, fraction / 100}) {
		t.Errorf("percentile arguments = %#v", a.Args)
	}
}

func TestParsedAggTreeIsExposed(t *testing.T) {
	q := mustParse(t, `{"aggs":{"t":{"date_histogram":{"field":"occurred_at","fixed_interval":"30s"},"aggs":{"o":{"terms":{"field":"origin","size":4,"order":{"_count":"asc"}},"aggs":{"n":{"value_count":{"field":"id"}}}}}}}}`)
	if len(q.Aggs) != 1 {
		t.Fatalf("aggs = %+v", q.Aggs)
	}
	hist := q.Aggs[0]
	if hist.Name != "t" || hist.Type != "date_histogram" || hist.Interval != 30*time.Second || len(hist.Aggs) != 1 {
		t.Fatalf("histogram = %+v", hist)
	}
	terms := hist.Aggs[0]
	if terms.Field != "origin" || terms.Size != 4 || terms.Order != "count_asc" || terms.Aggs[0].Type != "value_count" || terms.Aggs[0].Field != "id" {
		t.Errorf("terms = %+v", terms)
	}
}

func TestHistogramCountsPerStep(t *testing.T) {
	a, err := mustParse(t, "outcome:denied").Histogram(testWindow, 90*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	wantArgs := []any{testWindow.From, testWindow.To, "denied", 90.0, epoch, MaxAggRows + 1}
	if !reflect.DeepEqual(a.Args, wantArgs) {
		t.Errorf("args = %#v, want %#v", a.Args, wantArgs)
	}
	if !strings.Contains(a.SQL, "GROUP BY 1 ORDER BY 1") || !a.HasTimeBucket() {
		t.Errorf("SQL = %s", a.SQL)
	}
	assertSafeSQL(t, a.SQL, a.Args)
}

func TestAggregateRefusesWhatItCannotRun(t *testing.T) {
	denied := mustParse(t, "outcome:denied")
	if _, err := denied.Aggregate(testWindow); err == nil {
		t.Error("a query without aggs aggregated")
	}
	if _, err := denied.Histogram(testWindow, 500*time.Microsecond); !errors.Is(err, ErrStep) {
		t.Errorf("sub-millisecond step error = %v", err)
	}
	if _, err := denied.Histogram(Window{From: testWindow.To, To: testWindow.From}, time.Minute); !errors.Is(err, ErrWindow) {
		t.Errorf("reversed window error = %v", err)
	}
	if _, err := denied.Histogram(testWindow, time.Millisecond); err == nil || !strings.Contains(err.Error(), "interval too small") {
		t.Errorf("a million buckets error = %v", err)
	}
	tiny := mustParse(t, `{"aggs":{"t":{"date_histogram":{"field":"occurred_at","fixed_interval":"1s"}}}}`)
	if _, err := tiny.Aggregate(testWindow); err == nil {
		t.Error("86400 one-second buckets aggregated")
	}
	if _, err := tiny.Aggregate(Window{}); !errors.Is(err, ErrWindow) {
		t.Errorf("empty window error = %v", err)
	}
}

func TestAggregateInputNeverReachesTheSQLText(t *testing.T) {
	for _, text := range []string{
		`{"query":{"term":{"actor_name":"zz'; DROP TABLE audit_events; --"}},"aggs":{"o":{"terms":{"field":"origin"}}}}`,
		`{"query":{"wildcard":{"action":"zz\\'%_*"}},"aggs":{"t":{"date_histogram":{"field":"occurred_at","fixed_interval":"1h"}}}}`,
		`zz'; DROP TABLE audit_events; --`,
	} {
		q, err := Parse(text)
		if err != nil {
			t.Fatalf("Parse(%q): %v", text, err)
		}
		a, err := q.Histogram(testWindow, time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		assertSafeSQL(t, a.SQL, a.Args)
		if strings.Contains(a.SQL, "zz") || strings.Contains(a.SQL, "DROP") {
			t.Errorf("SQL leaks input: %s", a.SQL)
		}
	}
}

func TestDateHistogramIntervalCannotWrapAround(t *testing.T) {
	histogram := func(interval string) string {
		return `{"aggs":{"h":{"date_histogram":{"field":"occurred_at","fixed_interval":"` + interval + `"}}}}`
	}
	for _, interval := range []string{"288230376151711744ms", "9223372036854775807ms", "153722867280912931s", "99999999999999999999d", "367d", "8785h"} {
		_, err := Parse(histogram(interval))
		if err == nil || !strings.Contains(err.Error(), "longer than 366 days") {
			t.Errorf("interval %s: error = %v", interval, err)
		}
	}
	for _, interval := range []string{"1ms", "366d", "8784h"} {
		q, err := Parse(histogram(interval))
		if err != nil || q.Aggs[0].Interval <= 0 {
			t.Errorf("interval %s: %+v %v", interval, q.Aggs, err)
		}
	}
}
