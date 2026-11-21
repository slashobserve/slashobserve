package pgcheck

import (
	"context"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/slashobserve/slashobserve/internal/audit/filter"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

const schema = `
CREATE SCHEMA audit_filter_test;
SET search_path TO audit_filter_test;
CREATE SEQUENCE audit_events_seq;
CREATE TABLE audit_events (
  id             uuid        NOT NULL,
  seq            bigint      NOT NULL DEFAULT nextval('audit_events_seq'),
  xid            xid8        NOT NULL DEFAULT pg_current_xact_id(),
  occurred_at    timestamptz NOT NULL,
  category       text        NOT NULL,
  action         text        NOT NULL,
  outcome        text        NOT NULL CHECK (outcome IN ('success','denied','failed')),
  reason         text,
  status         integer,
  duration_ms    integer,
  actor_kind     text        NOT NULL CHECK (actor_kind IN ('user','service_account','anonymous','system')),
  actor_id       uuid,
  actor_name     text,
  actor_role     text,
  initiator      text        NOT NULL CHECK (initiator IN ('person','agent','system')),
  agent_id       uuid,
  agent_name     text,
  run_id         uuid,
  auth_method    text,
  credential_id  text,
  origin         text        NOT NULL,
  origin_client  text,
  tool           text,
  client_ip      inet,
  user_agent     text,
  request_id     uuid,
  trace_id       text,
  http_method    text,
  route          text,
  target_kind    text,
  target_id      text,
  target_name    text,
  folder_id      uuid,
  dashboard_id   uuid,
  observation_id uuid,
  thread_id      uuid,
  chat_id        uuid,
  datasource_uid text,
  data           jsonb       NOT NULL DEFAULT '{}',
  search         tsvector    NOT NULL,
  server_id      text        NOT NULL,
  server_version text        NOT NULL,
  PRIMARY KEY (occurred_at, id)
) PARTITION BY RANGE (occurred_at);
CREATE TABLE audit_events_p PARTITION OF audit_events FOR VALUES FROM ('2000-01-01') TO ('2100-01-01');
CREATE TABLE users (id uuid PRIMARY KEY, email text NOT NULL);
`

const runID = "5b1e0a52-0b7c-4f08-9b1f-6a1d3d6a9a10"

var base = time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)

type event struct {
	n          byte
	category   string
	action     string
	outcome    string
	reason     any
	status     any
	duration   any
	actorKind  string
	actorName  any
	initiator  string
	run        any
	origin     string
	tool       any
	ip         any
	targetKind any
	targetID   any
	datasource any
	search     string
}

var events = []event{
	{1, "content", "dashboard.update", "success", nil, 200, 12, "user", "Ishan Dubey", "person", nil, "web", nil, "10.4.1.5", "dashboard", "d1", nil, "dashboard.update Ishan Dubey Checkout health"},
	{2, "content", "dashboard.update", "denied", "forbidden_role", 403, 30, "user", "Riya", "agent", runID, "mcp", "save_dashboard", "10.5.0.1", "dashboard", "d1", nil, "dashboard.update Riya forbidden_role"},
	{3, "query", "query.run", "success", nil, 200, 200, "user", "Ishan Dubey", "agent", runID, "mcp", "query_range", "10.5.0.1", nil, nil, "prom", "query.run Ishan Dubey up"},
	{4, "auth", "auth.login_failed", "denied", "oidc_error", 401, nil, "anonymous", nil, "person", nil, "api", nil, "203.0.113.9", nil, nil, nil, "auth.login_failed"},
	{5, "content", "dashboard.view", "success", nil, 200, 8, "user", "Riya", "person", nil, "web", nil, "10.4.2.2", "dashboard", "d2", nil, "dashboard.view Riya Checkout health"},
}

func eventID(n byte) string {
	return uuid.UUID{15: n}.String()
}

func seed(ctx context.Context, t *testing.T, conn *pgx.Conn) {
	t.Helper()
	for _, e := range events {
		_, err := conn.Exec(ctx, `
INSERT INTO audit_events (id, occurred_at, category, action, outcome, reason, status, duration_ms, actor_kind, actor_name,
  initiator, run_id, origin, tool, client_ip, target_kind, target_id, datasource_uid, search, server_id, server_version)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, to_tsvector('simple', $19::text), 's1', 'test')`,
			eventID(e.n), base.Add(time.Duration(e.n)*time.Minute), e.category, e.action, e.outcome, e.reason, e.status, e.duration,
			e.actorKind, e.actorName, e.initiator, e.run, e.origin, e.tool, e.ip, e.targetKind, e.targetID, e.datasource, e.search)
		if err != nil {
			t.Fatalf("seed event %d: %v", e.n, err)
		}
	}
}

func connect(t *testing.T) (context.Context, *pgx.Conn) {
	t.Helper()
	d := dbtest.NewUnmigrated(t)
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, d.URL())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(ctx) })
	if _, err := conn.Exec(ctx, schema); err != nil {
		t.Fatalf("create the audit_events table: %v", err)
	}
	seed(ctx, t, conn)
	return ctx, conn
}

func window() filter.Window {
	return filter.Window{From: base, To: base.Add(time.Hour)}
}

func parse(t *testing.T, text string) filter.Query {
	t.Helper()
	q, err := filter.Parse(text)
	if err != nil {
		t.Fatalf("Parse(%q): %v", text, err)
	}
	return q
}

func values(ctx context.Context, t *testing.T, conn *pgx.Conn, sql string, args []any) [][]any {
	t.Helper()
	rows, err := conn.Query(ctx, sql, args...)
	if err != nil {
		t.Fatalf("run %s: %v", sql, err)
	}
	defer rows.Close()
	var out [][]any
	for rows.Next() {
		row, err := rows.Values()
		if err != nil {
			t.Fatalf("read a row: %v", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read rows: %v", err)
	}
	return out
}

func rowNumbers(ctx context.Context, t *testing.T, conn *pgx.Conn, c filter.Compiled) []int {
	t.Helper()
	var ids []int
	for _, row := range values(ctx, t, conn, c.SQL, c.Args) {
		id := row[0].([16]byte)
		ids = append(ids, int(id[15]))
	}
	return ids
}

func TestRowsRunAndSelectWhatTheQuerySays(t *testing.T) {
	ctx, conn := connect(t)
	for _, tc := range []struct {
		text string
		want []int
	}{
		{"", []int{5, 4, 3, 2, 1}},
		{"action:dashboard.*", []int{5, 2, 1}},
		{"action:(query.run or auth.login_failed)", []int{4, 3}},
		{"outcome:denied", []int{4, 2}},
		{"not actor:Riya", []int{4, 3, 1}},
		{"actor:Ishan*", []int{3, 1}},
		{"client_ip:10.4.0.0/16", []int{5, 1}},
		{"client_ip:203.0.113.9", []int{4}},
		{"run_id:" + runID, []int{3, 2}},
		{"target:dashboard/d1", []int{2, 1}},
		{"target:dashboard/d*", []int{5, 2, 1}},
		{"target:dashboard", []int{5, 2, 1}},
		{"duration_ms:(12 or 30)", []int{2, 1}},
		{"status >= 400", []int{4, 2}},
		{"status:403", []int{2}},
		{"category:con*", []int{5, 2, 1}},
		{"datasource:prom", []int{3}},
		{"reason:*", []int{4, 2}},
		{"occurred_at >= \"2026-11-01T10:03:00Z\"", []int{5, 4, 3}},
		{"Ishan", []int{3, 1}},
		{`"Ishan Dubey"`, []int{3, 1}},
		{"check*", []int{5, 1}},
		{"not Ishan", []int{5, 4, 2}},
		{"outcome:success and check*", []int{5, 1}},
		{`action:dashboard.* and "it's"`, nil},
		{`"zz'; DROP TABLE audit_events; --"`, nil},
		{`route:"x'y"`, nil},
		{`{"query":{"bool":{"filter":[{"term":{"outcome":"denied"}},{"range":{"status":{"gte":401}}}]}}}`, []int{4, 2}},
		{`{"query":{"terms":{"origin":["mcp","api"]}}}`, []int{4, 3, 2}},
		{`{"query":{"wildcard":{"action":{"value":"DASHBOARD.*","case_insensitive":true}}}}`, []int{5, 2, 1}},
		{`{"query":{"wildcard":{"action":"dashboard.v?ew"}}}`, []int{5}},
		{`{"query":{"wildcard":{"action":"DASHBOARD.*"}}}`, nil},
		{`{"query":{"exists":{"field":"tool"}}}`, []int{3, 2}},
		{`{"query":{"bool":{"must_not":[{"exists":{"field":"reason"}}]}}}`, []int{5, 3, 1}},
		{`{"query":{"match_all":{}}}`, []int{5, 4, 3, 2, 1}},
		{`{"query":{"match_none":{}}}`, nil},
		{`{"query":{"multi_match":{"query":"Riya","fields":["actor_name","agent_name"]}}}`, []int{5, 2}},
		{`{"query":{"query_string":{"query":"Checkout"}}}`, []int{5, 1}},
		{`{"query":{"term":{"client_ip":"10.5.0.0/16"}}}`, []int{3, 2}},
		{`{"query":{"bool":{"should":[{"term":{"origin":"web"}},{"term":{"outcome":"denied"}},{"term":{"category":"content"}}],"minimum_should_match":2}}}`, []int{5, 2, 1}},
		{"lucene: outcome:denied", []int{4, 2}},
		{"lucene: outcome:(denied OR failed) AND NOT actor:Riya", []int{4}},
		{"lucene: +outcome:denied origin:api", []int{4, 2}},
		{"lucene: action:dashboard.*", []int{5, 2, 1}},
		{"lucene: action:DASHBOARD.*", []int{5, 2, 1}},
		{"lucene: actor:Ishan*", []int{3, 1}},
		{"lucene: actor:Ish?n*", []int{3, 1}},
		{"lucene: act*:Riya", []int{5, 2}},
		{"lucene: *:Riya", []int{5, 2}},
		{"lucene: status:[400 TO 499]", []int{4, 2}},
		{"lucene: status:{401 TO 403]", []int{2}},
		{"lucene: status:>=401", []int{4, 2}},
		{"lucene: status:<=200", []int{5, 3, 1}},
		{"lucene: duration_ms:[10 TO 100]", []int{2, 1}},
		{"lucene: duration_ms:[100 TO *]", []int{3}},
		{"lucene: _exists_:reason", []int{4, 2}},
		{"lucene: NOT _exists_:reason", []int{5, 3, 1}},
		{"lucene: reason:*", []int{4, 2}},
		{"lucene: client_ip:10.4.0.0/16", []int{5, 1}},
		{"lucene: client_ip:203.0.113.9", []int{4}},
		{"lucene: target:dashboard/d1", []int{2, 1}},
		{"lucene: target:dashboard", []int{5, 2, 1}},
		{"lucene: run_id:" + runID, []int{3, 2}},
		{"lucene: occurred_at:[2026-11-01T10:02:00Z TO 2026-11-01T10:04:00Z]", []int{4, 3, 2}},
		{"lucene: occurred_at:[2026-11-01T10:02:00Z TO 2026-11-01T10:04:00Z}", []int{3, 2}},
		{"lucene: Ishan", []int{3, 1}},
		{`lucene: "Ishan Dubey"`, []int{3, 1}},
		{"lucene: check*", []int{5, 1}},
		{"lucene: -Ishan", []int{5, 4, 2}},
		{`lucene: route:"x'y"`, nil},
		{`lucene: "zz'; DROP TABLE audit_events; --"`, nil},
		{`lucene: action:zz\'\;\ DROP`, nil},
	} {
		t.Run(tc.text, func(t *testing.T) {
			c, err := parse(t, tc.text).Rows(window(), filter.Page{})
			if err != nil {
				t.Fatal(err)
			}
			if got := rowNumbers(ctx, t, conn, c); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("rows = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRowsPageByKeyset(t *testing.T) {
	ctx, conn := connect(t)
	q := parse(t, "")
	first, err := q.Rows(window(), filter.Page{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if got := rowNumbers(ctx, t, conn, first); !reflect.DeepEqual(got, []int{5, 4, 3}) {
		t.Fatalf("first page = %v, want the page plus one row that shows there is more", got)
	}
	cursor := &filter.Cursor{OccurredAt: base.Add(4 * time.Minute), ID: eventID(4)}
	second, err := q.Rows(window(), filter.Page{After: cursor, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if got := rowNumbers(ctx, t, conn, second); !reflect.DeepEqual(got, []int{3, 2, 1}) {
		t.Fatalf("second page = %v", got)
	}
}

func TestWindowBoundsTheRows(t *testing.T) {
	ctx, conn := connect(t)
	w := filter.Window{From: base.Add(2 * time.Minute), To: base.Add(4 * time.Minute)}
	c, err := parse(t, "").Rows(w, filter.Page{})
	if err != nil {
		t.Fatal(err)
	}
	if got := rowNumbers(ctx, t, conn, c); !reflect.DeepEqual(got, []int{3, 2}) {
		t.Fatalf("rows = %v", got)
	}
}

type aggRow struct {
	keys    []any
	count   float64
	metrics map[string]float64
}

func aggregate(ctx context.Context, t *testing.T, conn *pgx.Conn, text string) []aggRow {
	t.Helper()
	a, err := parse(t, text).Aggregate(window())
	if err != nil {
		t.Fatal(err)
	}
	return readAggregation(ctx, t, conn, a)
}

func readAggregation(ctx context.Context, t *testing.T, conn *pgx.Conn, a filter.Aggregation) []aggRow {
	t.Helper()
	var out []aggRow
	for _, row := range values(ctx, t, conn, a.SQL, a.Args) {
		levels := len(a.Levels)
		r := aggRow{keys: row[:levels], count: row[levels].(float64), metrics: map[string]float64{}}
		for i, m := range a.Metrics {
			if v, ok := row[levels+1+i].(float64); ok {
				name := m.Name
				if m.Key != "" {
					name += "/" + m.Key
				}
				r.metrics[name] = v
			}
		}
		out = append(out, r)
	}
	return out
}

func TestAggregateRunsEveryShape(t *testing.T) {
	ctx, conn := connect(t)

	byMinute := aggregate(ctx, t, conn, `{"aggs":{"t":{"date_histogram":{"field":"occurred_at","fixed_interval":"1m"}}}}`)
	if len(byMinute) != 5 || byMinute[0].count != 1 || !byMinute[0].keys[0].(time.Time).Equal(base.Add(time.Minute)) {
		t.Errorf("date_histogram = %+v", byMinute)
	}

	byOrigin := aggregate(ctx, t, conn, `{"aggs":{"o":{"terms":{"field":"origin"}}}}`)
	if len(byOrigin) != 3 || byOrigin[0].keys[0] != "api" || byOrigin[0].count != 1 || byOrigin[1].keys[0] != "mcp" || byOrigin[1].count != 2 || byOrigin[2].keys[0] != "web" {
		t.Errorf("terms on origin = %+v", byOrigin)
	}

	top := aggregate(ctx, t, conn, `{"aggs":{"o":{"terms":{"field":"origin","size":2}}}}`)
	if len(top) != 2 || top[0].keys[0] != "mcp" || top[1].keys[0] != "web" {
		t.Errorf("top 2 origins = %+v", top)
	}

	withMetrics := aggregate(ctx, t, conn, `{"aggs":{"o":{"terms":{"field":"origin"},"aggs":{"d":{"avg":{"field":"duration_ms"}},"n":{"value_count":{"field":"status"}}}}}}`)
	if len(withMetrics) != 3 || withMetrics[1].metrics["d"] != 115 || withMetrics[2].metrics["d"] != 10 || withMetrics[1].metrics["n"] != 2 {
		t.Errorf("terms with avg and value_count = %+v", withMetrics)
	}
	if _, ok := withMetrics[0].metrics["d"]; ok {
		t.Errorf("an average over no values must be null: %+v", withMetrics[0])
	}

	nested := aggregate(ctx, t, conn, `{"aggs":{"o":{"terms":{"field":"origin"},"aggs":{"a":{"terms":{"field":"actor_name","size":1}}}}}}`)
	if len(nested) != 2 || nested[0].keys[0] != "mcp" || nested[0].keys[1] != "Ishan Dubey" || nested[1].keys[0] != "web" || nested[1].keys[1] != "Ishan Dubey" {
		t.Errorf("terms in terms = %+v", nested)
	}

	overTime := aggregate(ctx, t, conn, `{"aggs":{"t":{"date_histogram":{"field":"occurred_at","fixed_interval":"1h"},"aggs":{"o":{"terms":{"field":"origin","size":1},"aggs":{"p":{"percentiles":{"field":"duration_ms","percents":[50]}}}}}}}}`)
	if len(overTime) != 1 || overTime[0].keys[1] != "mcp" || overTime[0].metrics["p/50"] != 115 {
		t.Errorf("histogram, terms and percentiles = %+v", overTime)
	}

	perBucket := aggregate(ctx, t, conn, `{"aggs":{"t":{"date_histogram":{"field":"occurred_at","fixed_interval":"2m"},"aggs":{"o":{"terms":{"field":"origin","size":1}}}}}}`)
	if len(perBucket) != 3 || perBucket[0].keys[1] != "web" || perBucket[1].keys[1] != "mcp" || perBucket[1].count != 2 || perBucket[2].keys[1] != "api" {
		t.Errorf("the top origin of each two minute bucket = %+v", perBucket)
	}

	aroundBuckets := aggregate(ctx, t, conn, `{"aggs":{"o":{"terms":{"field":"origin","size":1},"aggs":{"t":{"date_histogram":{"field":"occurred_at","fixed_interval":"2m"},"aggs":{"a":{"terms":{"field":"actor_name","size":1}}}}}}}}`)
	if len(aroundBuckets) != 1 || aroundBuckets[0].keys[0] != "mcp" || !aroundBuckets[0].keys[1].(time.Time).Equal(base.Add(2*time.Minute)) || aroundBuckets[0].keys[2] != "Ishan Dubey" {
		t.Errorf("terms, histogram and terms = %+v", aroundBuckets)
	}

	totals := aggregate(ctx, t, conn, `{"query":{"term":{"category":"content"}},"aggs":{"lo":{"min":{"field":"duration_ms"}},"hi":{"max":{"field":"duration_ms"}},"sum":{"sum":{"field":"duration_ms"}},"who":{"cardinality":{"field":"actor_name"}},"p":{"percentiles":{"field":"duration_ms","percents":[50]}}}}`)
	if len(totals) != 1 || totals[0].count != 3 || totals[0].metrics["lo"] != 8 || totals[0].metrics["hi"] != 30 || totals[0].metrics["sum"] != 50 || totals[0].metrics["who"] != 2 || totals[0].metrics["p/50"] != 12 {
		t.Errorf("metrics over content events = %+v", totals)
	}

	a, err := parse(t, "outcome:denied").Histogram(window(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	denied := readAggregation(ctx, t, conn, a)
	if len(denied) != 2 || !denied[0].keys[0].(time.Time).Equal(base.Add(2*time.Minute)) || !denied[1].keys[0].(time.Time).Equal(base.Add(4*time.Minute)) {
		t.Errorf("Histogram of denied events = %+v", denied)
	}
}

func TestFacetsRunUnderTheQuery(t *testing.T) {
	ctx, conn := connect(t)
	facet := func(text, field string) [][2]any {
		c, err := parse(t, text).Facets(window(), field, 10)
		if err != nil {
			t.Fatal(err)
		}
		var out [][2]any
		for _, row := range values(ctx, t, conn, c.SQL, c.Args) {
			out = append(out, [2]any{row[0], row[1]})
		}
		return out
	}
	if got, want := facet("", "actor"), [][2]any{{"Ishan Dubey", int64(2)}, {"Riya", int64(2)}}; !reflect.DeepEqual(got, want) {
		t.Errorf("actor facet = %v, want %v", got, want)
	}
	if got, want := facet("", "origin"), [][2]any{{"mcp", int64(2)}, {"web", int64(2)}, {"api", int64(1)}}; !reflect.DeepEqual(got, want) {
		t.Errorf("origin facet = %v, want %v", got, want)
	}
	if got, want := facet("outcome:denied", "action"), [][2]any{{"auth.login_failed", int64(1)}, {"dashboard.update", int64(1)}}; !reflect.DeepEqual(got, want) {
		t.Errorf("action facet under outcome:denied = %v, want %v", got, want)
	}
	if ips := facet("", "ip"); len(ips) != 4 || ips[0][0] != "10.5.0.1" || ips[0][1] != int64(2) {
		t.Errorf("ip facet = %v", ips)
	}
}

func TestOneQuestionInThreeFormsReturnsTheSameRows(t *testing.T) {
	ctx, conn := connect(t)
	for _, tc := range []struct {
		forms [3]string
		want  []int
	}{
		{[3]string{"outcome:denied and status:403", "lucene: outcome:denied AND status:403", `{"query":{"bool":{"filter":[{"term":{"outcome":"denied"}},{"term":{"status":403}}]}}}`}, []int{2}},
		{[3]string{"outcome:denied or outcome:failed", "lucene: outcome:denied OR outcome:failed", `{"query":{"terms":{"outcome":["denied","failed"]}}}`}, []int{4, 2}},
		{[3]string{"not actor:Riya", "lucene: NOT actor:Riya", `{"query":{"bool":{"must_not":[{"term":{"actor_name":"Riya"}}]}}}`}, []int{4, 3, 1}},
		{[3]string{"status >= 400", "lucene: status:>=400", `{"query":{"range":{"status":{"gte":400}}}}`}, []int{4, 2}},
		{[3]string{"occurred_at:2026-11-01", "lucene: occurred_at:2026-11-01", `{"query":{"term":{"occurred_at":"2026-11-01"}}}`}, []int{5, 4, 3, 2, 1}},
		{[3]string{"occurred_at:2026-11-02", "lucene: occurred_at:2026-11-02", `{"query":{"term":{"occurred_at":"2026-11-02"}}}`}, nil},
		{[3]string{"occurred_at <= 2026-11-01", "lucene: occurred_at:<=2026-11-01", `{"query":{"range":{"occurred_at":{"lte":"2026-11-01"}}}}`}, []int{5, 4, 3, 2, 1}},
		{[3]string{"occurred_at > 2026-11-01", "lucene: occurred_at:>2026-11-01", `{"query":{"range":{"occurred_at":{"gt":"2026-11-01"}}}}`}, nil},
		{[3]string{"occurred_at >= 2026-11-01", "lucene: occurred_at:>=2026-11-01", `{"query":{"range":{"occurred_at":{"gte":"2026-11-01"}}}}`}, []int{5, 4, 3, 2, 1}},
		{[3]string{"occurred_at < 2026-11-01", "lucene: occurred_at:<2026-11-01", `{"query":{"range":{"occurred_at":{"lt":"2026-11-01"}}}}`}, nil},
		{[3]string{"reason:*", "lucene: _exists_:reason", `{"query":{"exists":{"field":"reason"}}}`}, []int{4, 2}},
		{[3]string{"client_ip:10.4.0.0/16", "lucene: client_ip:10.4.0.0/16", `{"query":{"term":{"client_ip":"10.4.0.0/16"}}}`}, []int{5, 1}},
		{[3]string{"target:dashboard/d1", "lucene: target:dashboard/d1", `{"query":{"term":{"target":"dashboard/d1"}}}`}, []int{2, 1}},
		{[3]string{"Ishan", "lucene: Ishan", `{"query":{"multi_match":{"query":"Ishan"}}}`}, []int{3, 1}},
		{[3]string{"check*", "lucene: check*", `{"query":{"query_string":{"query":"check*"}}}`}, []int{5, 1}},
		{[3]string{`occurred_at >= "2026-11-01T10:03:00Z"`, `lucene: occurred_at:>="2026-11-01T10:03:00Z"`, `{"query":{"range":{"occurred_at":{"gte":"2026-11-01T10:03:00Z"}}}}`}, []int{5, 4, 3}},
	} {
		for _, text := range tc.forms {
			t.Run(text, func(t *testing.T) {
				c, err := parse(t, text).Rows(window(), filter.Page{})
				if err != nil {
					t.Fatal(err)
				}
				if got := rowNumbers(ctx, t, conn, c); !reflect.DeepEqual(got, tc.want) {
					t.Errorf("rows = %v, want %v", got, tc.want)
				}
			})
		}
	}
}

func TestRowsReadOldestFirstFromAnOffset(t *testing.T) {
	ctx, conn := connect(t)
	c, err := parse(t, "").Rows(window(), filter.Page{Limit: 2, Offset: 1, Ascending: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := rowNumbers(ctx, t, conn, c); !reflect.DeepEqual(got, []int{2, 3, 4}) {
		t.Fatalf("rows = %v, want the oldest skipped and the page plus one", got)
	}
}

func TestCountMatchesUpToTheCap(t *testing.T) {
	ctx, conn := connect(t)
	for _, tc := range []struct {
		text string
		upTo int
		want int64
	}{
		{"", 0, 5},
		{"", 3, 3},
		{"outcome:denied", 0, 2},
	} {
		c, err := parse(t, tc.text).Count(window(), tc.upTo)
		if err != nil {
			t.Fatal(err)
		}
		var n int64
		if err := conn.QueryRow(ctx, c.SQL, c.Args...).Scan(&n); err != nil || n != tc.want {
			t.Errorf("Count(%q, %d) = %d, %v; want %d", tc.text, tc.upTo, n, err, tc.want)
		}
	}
}

func TestDateHistogramBucketsStartAtLocalMidnight(t *testing.T) {
	ctx, conn := connect(t)
	rows := aggregate(ctx, t, conn, `{"aggs":{"t":{"date_histogram":{"field":"occurred_at","fixed_interval":"1d","time_zone":"Asia/Kolkata"}}}}`)
	want := time.Date(2026, 10, 31, 18, 30, 0, 0, time.UTC)
	if len(rows) != 1 || !rows[0].keys[0].(time.Time).Equal(want) || rows[0].count != 5 {
		t.Fatalf("rows = %+v, want one bucket at %s holding all five events", rows, want)
	}
}
