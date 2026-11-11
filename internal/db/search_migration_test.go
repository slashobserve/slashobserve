package db_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

const (
	searchVersion = 33

	searchDashboard  = "00000000-0000-4000-8000-0000000006a1"
	searchFinding    = "00000000-0000-4000-8000-0000000006b1"
	searchAnnotation = "00000000-0000-4000-8000-0000000006b2"
	searchThread     = "00000000-0000-4000-8000-0000000006c1"
	searchUser       = "00000000-0000-4000-8000-0000000006d1"
)

type searchTable struct {
	name    string
	index   string
	vector  string
	from    string
	explain string
}

var searchTables = []searchTable{
	{"dashboard_versions", "dashboard_versions_search_idx", "dashboard_search_vector(spec)", "dashboard_versions", `EXPLAIN SELECT 1 FROM dashboard_versions WHERE dashboard_search_vector(spec) @@ search_query('zebra', NULL)`},
	{"findings", "findings_search_idx", "finding_search_vector(title, markdown, evidence)", "findings", `EXPLAIN SELECT 1 FROM findings WHERE finding_search_vector(title, markdown, evidence) @@ search_query('zebra', NULL)`},
	{"annotations", "annotations_search_idx", "annotation_search_vector(text, tags)", "annotations", `EXPLAIN SELECT 1 FROM annotations WHERE annotation_search_vector(text, tags) @@ search_query('zebra', NULL)`},
	{"thread_messages", "thread_messages_search_idx", "message_search_vector(body)", "thread_messages", `EXPLAIN SELECT 1 FROM thread_messages WHERE message_search_vector(body) @@ search_query('zebra', NULL)`},
	{"saved_searches", "saved_searches_search_idx", "saved_search_search_vector(title, query, filters)", "saved_searches", `EXPLAIN SELECT 1 FROM saved_searches WHERE saved_search_search_vector(title, query, filters) @@ search_query('zebra', NULL)`},
}

const searchSpecV1 = `{"title":"Checkout","tags":["payments"],"panels":[{"id":"a","title":"walrus panel","queries":[{"query":"up"}]}]}`

const searchSpecV2 = `{"title":"Checkout","description":"orders overview","tags":["payments"],"panels":[
  {"id":"r","type":"row","title":"Row","panels":[{"id":"b","title":"inner","description":"zebra crossing","queries":[{"query":"sum(http_requests_total)"}]}]},
  {"id":"c","title":"latency","queries":[{"query":"histogram_quantile(0.9, rate(request_duration_seconds_bucket[5m]))"}]}
]}`

func seedSearchFixtures(t *testing.T, d *dbtest.Database) {
	t.Helper()
	d.Exec(t, `INSERT INTO users (id, oidc_issuer, oidc_subject, email, name) VALUES ($1::uuid, 'https://idp', 'ada', 'ada@example.com', 'Ada')`, searchUser)
	seedDashboard(t, d, searchDashboard, 2)
	seedVersion(t, d, searchDashboard, 1, searchSpecV1)
	seedVersion(t, d, searchDashboard, 2, searchSpecV2)
	d.Exec(t, `INSERT INTO findings (id, dashboard_id, title, markdown, evidence, time_from, time_to, created_by_kind, created_by_id, created_by_name)
VALUES ($1::uuid, $2::uuid, 'Elevated latency', 'Checkout errors rose after the deploy', $3::jsonb, now() - interval '1 hour', now(), 'user', $4::uuid, 'Ada')`,
		searchFinding, searchDashboard,
		`[{"id":"e","title":"saturation evidence","description":"pool exhausted","queries":[{"query":"db_pool_in_use"}]}]`, searchUser)
	d.Exec(t, `INSERT INTO annotations (id, dashboard_id, time_from, text, tags) VALUES ($1::uuid, $2::uuid, now(), E'Deploy of release 42\nrolled back by hand', ARRAY['rollout','canary'])`,
		searchAnnotation, searchDashboard)
	d.Exec(t, `INSERT INTO threads (id, annotation_id) VALUES ($1::uuid, $2::uuid)`, searchThread, searchAnnotation)
	d.Exec(t, `INSERT INTO thread_messages (id, thread_id, body, author_kind, author_id, author_name)
VALUES ($1::uuid, $1::uuid, $2, 'user', $3::uuid, 'Ada')`,
		searchThread, "please look @[Grace Hopper](user:11111111-1111-4111-8111-111111111111) at this", searchUser)
	d.Exec(t, `INSERT INTO saved_searches (kind, title, datasource_uid, query, filters, time_from, time_to, folder_id, created_by_kind, created_by_id, created_by_name)
VALUES ('logs', 'Payment failures', 'loki', '{app="billing"} |= "declined"', '{"columns":["level"]}', 'now-1h', 'now', general_folder_id(), 'user', $1::uuid, 'Ada'),
       ('traces', 'Slow checkouts', 'tempo', NULL, '{"service":"cartservice","span":"charge"}', 'now-1h', 'now', general_folder_id(), 'user', $1::uuid, 'Ada')`, searchUser)
}

func migrateToSearch(t *testing.T) *dbtest.Database {
	t.Helper()
	d := dbtest.NewUnmigrated(t)
	if err := d.MigrateUpTo(context.Background(), searchVersion-1); err != nil {
		t.Fatal(err)
	}
	seedSearchFixtures(t, d)
	if err := d.MigrateUpTo(context.Background(), searchVersion); err != nil {
		t.Fatal(err)
	}
	return d
}

func searchMatches(t *testing.T, sqlDB *sql.DB, table searchTable, where, words string, prefix any) int {
	t.Helper()
	var n int
	query := `SELECT count(*) FROM ` + table.from + ` WHERE ` + where + ` AND ` + table.vector + ` @@ search_query($1, $2)`
	if err := sqlDB.QueryRow(query, words, prefix).Scan(&n); err != nil {
		t.Fatalf("%s %q: %v", table.name, words, err)
	}
	return n
}

func TestSearchVectorsMatchTheSeededWords(t *testing.T) {
	d := migrateToSearch(t)
	sqlDB := d.SQLDB(t)
	dashboards, findings, annotations, messages, saved := searchTables[0], searchTables[1], searchTables[2], searchTables[3], searchTables[4]
	latest := "version = 2"
	cases := []struct {
		name   string
		table  searchTable
		where  string
		words  string
		prefix any
		want   int
	}{
		{"nested panel description", dashboards, latest, "zebra", nil, 1},
		{"title", dashboards, latest, "checkout", nil, 1},
		{"tag", dashboards, latest, "payments", nil, 1},
		{"query text as a phrase", dashboards, latest, `"http_requests_total"`, nil, 1},
		{"query text by one of its words", dashboards, latest, "requests", nil, 1},
		{"query text of a top level panel", dashboards, latest, "request_duration_seconds_bucket", nil, 1},
		{"a word only in the old version", dashboards, latest, "walrus", nil, 0},
		{"the old version still holds it", dashboards, "version = 1", "walrus", nil, 1},
		{"every word must match", dashboards, latest, "zebra walrus", nil, 0},
		{"or joins words", dashboards, latest, "zebra or walrus", nil, 1},
		{"minus excludes", dashboards, latest, "checkout -zebra", nil, 0},
		{"prefix of the last word", dashboards, latest, "chec", "chec", 1},
		{"prefix after an exact word", dashboards, latest, "orders chec", "chec", 1},
		{"no prefix means no partial match", dashboards, latest, "chec", nil, 0},
		{"a stop word matches exactly", findings, "true", "after the deploy", nil, 1},
		{"finding title", findings, "true", "latency", nil, 1},
		{"finding stem", findings, "true", "errors", nil, 1},
		{"finding stem of the plural", findings, "true", "error", nil, 1},
		{"finding evidence description", findings, "true", "exhausted", nil, 1},
		{"finding evidence query", findings, "true", "db_pool_in_use", nil, 1},
		{"annotation first line", annotations, "true", "release", nil, 1},
		{"annotation body", annotations, "true", "hand", nil, 1},
		{"annotation tag", annotations, "true", "canary", nil, 1},
		{"comment mention shows the name", messages, "true", "grace hopper", nil, 1},
		{"comment mention hides the id", messages, "true", "11111111", nil, 0},
		{"comment body", messages, "true", "look", nil, 1},
		{"saved search title", saved, "true", "failures", nil, 1},
		{"saved search query", saved, "true", "declined", nil, 1},
		{"traces service filter", saved, "true", "cartservice", nil, 1},
		{"traces span filter", saved, "true", "charge", nil, 1},
		{"logs columns are not indexed", saved, "true", "level", nil, 0},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := searchMatches(t, sqlDB, tt.table, tt.where, tt.words, tt.prefix); got != tt.want {
				t.Fatalf("%q matched %d rows of %s, want %d", tt.words, got, tt.table.name, tt.want)
			}
		})
	}
}

func TestSearchQueryEdges(t *testing.T) {
	d := migrateToSearch(t)
	sqlDB := d.SQLDB(t)
	var empty, stopWordsOnly bool
	if err := sqlDB.QueryRow(`SELECT search_query('the', NULL) = ''::tsquery, numnode(search_query('the', NULL)) = 0`).Scan(&empty, &stopWordsOnly); err != nil {
		t.Fatal(err)
	}
	if empty || stopWordsOnly {
		t.Fatalf("a stop word query is empty: equal to empty %v, no nodes %v", empty, stopWordsOnly)
	}
	var wrapsAround bool
	if err := sqlDB.QueryRow(`SELECT search_vector('title', '', 'beta alpha', 'code') @@ search_query('"alpha beta"', NULL)`).Scan(&wrapsAround); err != nil {
		t.Fatal(err)
	}
	if wrapsAround {
		t.Fatal("a phrase matched the last word of a part followed by its first")
	}
	var spansParts bool
	if err := sqlDB.QueryRow(`SELECT search_vector('alpha', '', 'beta', '') @@ search_query('"alpha beta"', NULL)`).Scan(&spansParts); err != nil {
		t.Fatal(err)
	}
	if spansParts {
		t.Fatal("a phrase matched across the title and the body")
	}
	var text string
	if err := sqlDB.QueryRow(`SELECT search_query('chec', 'chec')::text`).Scan(&text); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "chec':*") {
		t.Fatalf("prefix-only query = %s, want a prefix term", text)
	}
}

func TestSearchRecencyOnlyOrdersNearEquals(t *testing.T) {
	d := migrateToSearch(t)
	var fresh, month, ancient, future float64
	if err := d.SQLDB(t).QueryRow(`SELECT
  search_recency(now(), now()),
  search_recency(now() - interval '30 days', now()),
  search_recency(now() - interval '3650 days', now()),
  search_recency(now() + interval '1 day', now())`).Scan(&fresh, &month, &ancient, &future); err != nil {
		t.Fatal(err)
	}
	if fresh != 1.5 || month != 1.25 || ancient < 1 || ancient > 1.0001 || future != 1.5 {
		t.Fatalf("recency fresh=%v month=%v ancient=%v future=%v", fresh, month, ancient, future)
	}
}

func TestSearchIndexesServeTheirVectorExpressions(t *testing.T) {
	d := migrateToSearch(t)
	sqlDB := d.SQLDB(t)
	for _, table := range searchTables {
		t.Run(table.name, func(t *testing.T) {
			tx, err := sqlDB.BeginTx(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback() }()
			if _, err := tx.Exec(`SET LOCAL enable_seqscan = off`); err != nil {
				t.Fatal(err)
			}
			rows, err := tx.Query(table.explain)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = rows.Close() }()
			var plan strings.Builder
			for rows.Next() {
				var line string
				if err := rows.Scan(&line); err != nil {
					t.Fatal(err)
				}
				plan.WriteString(line + "\n")
			}
			if !strings.Contains(plan.String(), table.index) {
				t.Fatalf("plan does not name %s:\n%s", table.index, plan.String())
			}
		})
	}
}

func TestSearchIndexesRebuildWithAnEmptySearchPath(t *testing.T) {
	d := migrateToSearch(t)
	conn, err := d.SQLDB(t).Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	ctx := context.Background()
	if _, err := conn.ExecContext(ctx, `SET search_path = ''`); err != nil {
		t.Fatal(err)
	}
	for _, table := range searchTables {
		if _, err := conn.ExecContext(ctx, `REINDEX TABLE public.`+table.name); err != nil {
			t.Fatalf("reindex %s: %v", table.name, err)
		}
	}
}

func TestSearchDown(t *testing.T) {
	ctx := context.Background()
	d := migrateToSearch(t)
	if err := d.MigrateDownTo(ctx, searchVersion-1); err != nil {
		t.Fatalf("down: %v", err)
	}
	sqlDB := d.SQLDB(t)
	for _, table := range searchTables {
		var exists bool
		if err := sqlDB.QueryRow(`SELECT to_regclass($1) IS NOT NULL`, table.index).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if exists {
			t.Fatalf("%s still exists after the down migration", table.index)
		}
	}
	var leftover int
	if err := sqlDB.QueryRow(`SELECT count(*) FROM pg_proc WHERE proname ~ '(^search_|_search_|_search$)'`).Scan(&leftover); err != nil {
		t.Fatal(err)
	}
	if leftover != 0 {
		t.Fatalf("%d search functions remain after the down migration", leftover)
	}
	if err := d.MigrateUpTo(ctx, searchVersion); err != nil {
		t.Fatalf("up again: %v", err)
	}
}
