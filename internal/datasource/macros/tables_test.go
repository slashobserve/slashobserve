package macros_test

import (
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource/macros"
)

func TestClickHouseTables(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  []string
	}{
		{"bare table", "SELECT count() FROM otel_logs", []string{"otel_logs"}},
		{"qualified table", "SELECT count() FROM otel.otel_logs WHERE x = 1", []string{"otel.otel_logs"}},
		{"backquoted names", "SELECT 1 FROM `otel`.`otel_logs`", []string{"otel.otel_logs"}},
		{"double quoted names", `SELECT 1 FROM "otel"."my table"`, []string{"otel.my table"}},
		{"lower case keywords", "select 1 from otel.logs as l join otel.users as u on l.id = u.id", []string{"otel.logs", "otel.users"}},
		{"join", "SELECT 1 FROM a.t LEFT JOIN b.u USING (id)", []string{"a.t", "b.u"}},
		{"comma join", "SELECT 1 FROM t, u AS x, v", []string{"t", "u", "v"}},
		{"subquery", "SELECT 1 FROM (SELECT x FROM inner_t WHERE y > 1) AS s", []string{"inner_t"}},
		{"subquery then a table", "SELECT 1 FROM (SELECT x FROM a) AS s, b", []string{"a", "b"}},
		{"table function", "SELECT * FROM numbers(10)", nil},
		{"table function next to a table", "SELECT * FROM numbers(10) AS n JOIN t ON n.number = t.id", []string{"t"}},
		{"cte name", "WITH recent AS (SELECT * FROM base WHERE x > 1) SELECT * FROM recent", []string{"base"}},
		{"several ctes", "WITH a AS (SELECT 1 FROM t1), b AS (SELECT 2 FROM t2) SELECT * FROM a JOIN b ON 1", []string{"t1", "t2"}},
		{"cte name shadows only a bare name", "WITH x AS (SELECT 1) SELECT * FROM db.x", []string{"db.x"}},
		{"scalar with alias is not a cte", "WITH 1 AS one SELECT one FROM t", []string{"t"}},
		{"union all", "SELECT 1 FROM a UNION ALL SELECT 2 FROM b.c", []string{"a", "b.c"}},
		{"parenthesised union", "(SELECT 1 FROM a) UNION ALL (SELECT 2 FROM b)", []string{"a", "b"}},
		{"repeated table once", "SELECT 1 FROM t WHERE x IN (SELECT x FROM t)", []string{"t"}},
		{"from in a string", "SELECT 'x FROM fake' AS s FROM real", []string{"real"}},
		{"from in a line comment", "SELECT 1 -- FROM fake\nFROM real", []string{"real"}},
		{"from in a block comment", "SELECT 1 /* FROM fake */ FROM real", []string{"real"}},
		{"from in a heredoc", "SELECT $$ FROM fake $$ FROM real", []string{"real"}},
		{"extract from", "SELECT extract(YEAR FROM ts) FROM t", []string{"t"}},
		{"trim from", "SELECT trim(BOTH ' ' FROM name) FROM t", []string{"t"}},
		{"array join is not a table", "SELECT x FROM t ARRAY JOIN xs AS x, ys AS y", []string{"t"}},
		{"left array join is not a table", "SELECT x FROM t LEFT ARRAY JOIN xs AS x", []string{"t"}},
		{"column named from after a dot", "SELECT t.from FROM t", []string{"t"}},
		{"where clause commas are not tables", "SELECT 1 FROM t WHERE a IN (1, 2) GROUP BY a, b", []string{"t"}},
		{"limit with two numbers", "SELECT 1 FROM t LIMIT 1, 2", []string{"t"}},
		{"three part name is ignored", "SELECT 1 FROM a.b.c", nil},
		{"final and sample", "SELECT 1 FROM t FINAL SAMPLE 0.1", []string{"t"}},
		{"no table", "SELECT 1", nil},
		{"unterminated string", "SELECT 'oops FROM t", nil},
		{"empty", "", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := macros.ClickHouseTables(tc.query)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("ClickHouseTables(%q) = %q, want %q", tc.query, got, tc.want)
			}
		})
	}
}
