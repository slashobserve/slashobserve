package clickhouse

import (
	"errors"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func TestGuardForbidden(t *testing.T) {
	for _, q := range []string{
		"INSERT INTO otel_logs (Body) VALUES ('x')",
		"SELECT 1; DROP TABLE otel_logs",
		"SELECT 1 SETTINGS max_execution_time = 1000",
		"SELECT * FROM url('http://127.0.0.1:1/x', CSV)",
		"SELECT 1 FORMAT JSON",
		"SELECT 1 INTO OUTFILE '/tmp/x'",
		"SELECT $$ ' $$; DROP TABLE otel_logs; -- '",
		"SELECT 1 # '\n; DROP TABLE otel_logs; -- '",
		"SELECT 1 /* ; */ ; SELECT 2",
		"SELECT 1 # x'\n; DROP TABLE otel_logs",
		"SELECT 1 /* /* */ ; */ ; INSERT INTO otel_logs (Body) VALUES ('x')",
		"SELECT $$ ; $$; SELECT 2",
		"SELECT 1 #! x'\n FORMAT JSON",
		"SELECT 1 # don't\n SETTINGS max_execution_time = 1",
		"SELECT 1 /* c */ SETTINGS max_execution_time = 1",
		"SELECT 1 -- c\n INTO OUTFILE '/tmp/x'",
		"SELECT 1 /* c */ FROM /* c */ url /* c */ ('http://127.0.0.1:1/x', CSV)",
		"SELECT 1;;",
		"; SELECT 1",
		"(SELECT 1)",
		"SELECT 1 // '\nSETTINGS max_result_rows=5 -- '",
		"SELECT 1 AS x SETTINGS max_result_rows=6",
		"SELECT 1 FROM url('http://x', CSV)",
		"SELECT 1 FORMAT JSON",
		"SELECT ‘'’ AS a SETTINGS max_result_rows=7 -- '",
		"SELECT * FROM “url”('http://x', CSV)",
		"SELECT * FROM \"url\"('http://x', CSV)",
		"SELECT * FROM `url`('http://x', CSV)",
		"SELECT a$$, 1 FROM t SETTINGS max_result_rows=9 -- $$",
		"SELECT * FROM URL('http://x', CSV)",
		"SELECT * FROM `\\x75rl`('http://127.0.0.1:1/x', CSV, 'a String')",
		"SELECT * FROM \"\\x75rl\"('http://127.0.0.1:1/x', CSV, 'a String')",
		"SELECT * FROM `\\x4`('http://x', CSV)",
		"SELECT * FROM \"\\xZZ\"('http://x', CSV)",
		"SELECT * FROM hudiCluster('c', 'http://x')",
		"SELECT * FROM deltaLakeCluster('c', 'http://x')",
		"SELECT * FROM icebergCluster('c', 'http://x')",
		"SELECT * FROM icebergS3Cluster('c', 'http://x')",
		"SELECT * FROM icebergAzureCluster('c', 'x')",
		"SELECT * FROM deltaLakeS3('http://x')",
		"SELECT * FROM deltaLakeAzure('x')",
		"SELECT * FROM arrowFlight('host:1', 'ds')",
		"SELECT * FROM ytsaurus('http://x', 'p', 't')",
		"SELECT * FROM paimon('http://x')",
		"SELECT * FROM paimonS3('http://x')",
		"SELECT * FROM paimonHDFS('http://x')",
		"SELECT * FROM cosn('cosn://b/k')",
		"SELECT * FROM COSN('cosn://b/k')",
		"SELECT * FROM `cosn`('cosn://b/k')",
		"SELECT * FROM \"c\\x6fsn\"('cosn://b/k')",
		"SELECT * FROM bigquery('p', 'd', 't')",
		"SELECT * FROM BigQuery('p', 'd', 't')",
		"SELECT * FROM \"bigquery\"('p', 'd', 't')",
		"SELECT * FROM `\\x62igquery`('p', 'd', 't')",
		"SELECT * FROM filesystem('/etc')",
		"SELECT * FROM FileSystem('/etc')",
		"SELECT * FROM `filesystem`('/etc')",
		"SELECT * FROM \"\\x66ilesystem\"('/etc')",
		"SELECT * FROM eval('select 1')",
		"SELECT * FROM EVAL('select 1')",
		"SELECT * FROM `eval`('select 1')",
		"SELECT * FROM \"\\x65val\"('select 1')",
		"SELECT * FROM \u201ceval\u201d('select 1')",
		"SELECT * FROM hive('thrift://x', 'db', 't', 'c Int', 'p')",
		"SELECT * FROM oss('http://x')",
		"SELECT * FROM s3Cluster('c', 'http://x')",
		"SELECT * FROM remoteSecure('x', system.one)",
		"SELECT * FROM clusterAllReplicas('c', system.one)",
	} {
		if _, err := guard(q); !errors.Is(err, datasource.ErrForbiddenQuery) {
			t.Errorf("guard(%q) err = %v, want ErrForbiddenQuery", q, err)
		}
	}
}

func TestGuardRejectsMalformed(t *testing.T) {
	for _, q := range []string{
		"", "   ", "-- only a comment", "/* c */",
		"SELECT 'abc", "SELECT 1 /* x", "SELECT $$ x", "SELECT `abc", "SELECT \"abc",
		"SELECT ‘abc", "SELECT “abc",
	} {
		if _, err := guard(q); err == nil {
			t.Errorf("guard(%q) succeeded, want a rejection", q)
		}
	}
	if _, err := guard("SELECT 'abc"); !errors.Is(err, datasource.ErrInvalidQuery) {
		t.Errorf("guard(unterminated string) err = %v, want ErrInvalidQuery", err)
	}
	if _, err := guard("  -- c\n"); !errors.Is(err, datasource.ErrForbiddenQuery) {
		t.Errorf("guard(only a comment) err = %v, want ErrForbiddenQuery", err)
	}
}

func TestGuardAllows(t *testing.T) {
	for _, q := range []string{
		"SELECT 'a;b'",
		"SELECT 1;",
		"SELECT 1 ; -- trailing",
		"SELECT 1 -- ; DROP TABLE otel_logs",
		"SELECT $$ a;b $$",
		"SELECT 'it''s', 'a\\'b'",
		"WITH x AS (SELECT 1 AS v) SELECT * FROM x",
		"select 1 as `format`",
		"SELECT 1 AS \"settings\"",
		"SELECT format('{}', 1)",
		"SELECT format ('{}', 1)",
		"SELECT * FROM system.settings",
		"SELECT * FROM system . settings",
		"SELECT t.format, t.into FROM t",
		"SELECT * FROM `system`.`settings`",
		"SELECT '‘ SETTINGS ’', ‘SETTINGS’",
		"SELECT 1 /* SETTINGS */ # SETTINGS\n -- FORMAT\n",
		"SELECT 1 // SETTINGS",
		"SELECT * FROM numbers(3)",
		"SELECT * FROM `\\url`('http://x', CSV)",
		"SELECT * FROM \"ur\\l\"('http://x', CSV)",
		"SELECT fileName FROM t",
		"SELECT hivemind(1), urlHash(2), s3count(3)",
		"\n  SELECT 1\n",
	} {
		if _, err := guard(q); err != nil {
			t.Errorf("guard(%q): %v", q, err)
		}
	}
}

func TestGuardKeepsOffsets(t *testing.T) {
	for _, q := range []string{
		"  SELECT count( FROM otel_logs",
		"\n\t-- c\n  SELECT 1 ;  ",
		" SELECT 1;",
		"SELECT 1 ;\n-- tail",
	} {
		got, err := guard(q)
		if err != nil {
			t.Fatalf("guard(%q): %v", q, err)
		}
		if !strings.HasPrefix(q, got) {
			t.Errorf("guard(%q) = %q, want a prefix of the query so offsets stay aligned", q, got)
		}
		if strings.Contains(got, ";") {
			t.Errorf("guard(%q) = %q, want the trailing semicolon removed", q, got)
		}
	}
}

func loadedTableFunctions() *tableFunctionSet {
	return newTableFunctionSet(
		[]string{"futureFn", "Loop", "numbers", "values", "view", "format", "fuzzQuery", "generateRandom"},
		map[string]bool{"format": true, "fuzzquery": true},
	)
}

func TestGuardLoadedTableFunctionsForbidden(t *testing.T) {
	tf := loadedTableFunctions()
	for _, q := range []string{
		"SELECT * FROM futureFn(1)",
		"SELECT * FROM FUTUREFN(1)",
		"SELECT * FROM `futurefn`(1)",
		"SELECT * FROM \"future\\x46n\"(1)",
		"SELECT * FROM “futurefn”(1)",
		"SELECT * FROM loop(numbers(1))",
		"SELECT format('{}', futureFn(1))",
		"SELECT * FROM format(CSV, 'a')",
		"SELECT * FROM FORMAT (CSV, 'a')",
		"SELECT 1 FROM t, format(CSV, 'a')",
		"SELECT 1 FROM t JOIN format(CSV, 'a') AS f ON 1",
		"SELECT 1 FROM t ARRAY JOIN [1] AS a, format(CSV, 'a')",
		"SELECT * FROM (SELECT 1) AS a, fuzzQuery('select 1')",
		"SELECT 1 WHERE x IN (SELECT 1 FROM fuzzQuery('select 1'))",
		"WITH 1 AS a SELECT * FROM fuzzquery('select 1')",
		"SELECT * FROM numbers(1) AS n, format(CSV, 'a')",
	} {
		if _, err := guardWith(q, tf); !errors.Is(err, datasource.ErrForbiddenQuery) {
			t.Errorf("guardWith(%q) err = %v, want ErrForbiddenQuery", q, err)
		}
	}
}

func TestGuardLoadedTableFunctionsAllowed(t *testing.T) {
	tf := loadedTableFunctions()
	for _, q := range []string{
		"SELECT * FROM numbers(3)",
		"SELECT * FROM values('a Int', (1))",
		"SELECT * FROM view(SELECT 1)",
		"SELECT * FROM generateRandom('a Int')",
		"SELECT toString(1), count() FROM otel_logs",
		"SELECT format('{}', 1) FROM otel_logs",
		"SELECT 1, format('{}', 2) FROM otel_logs WHERE format('{}', 3) = '3'",
		"SELECT * FROM otel_logs WHERE x IN (SELECT format('{}', 1))",
		"SELECT * FROM numbers(2) AS n, (SELECT format('{}', 1) AS m) AS s",
		"SELECT * FROM otel_logs ORDER BY format('{}', Body) LIMIT 1",
		"SELECT * FROM system.settings WHERE format('{}', 1) = '1'",
	} {
		if _, err := guardWith(q, tf); err != nil {
			t.Errorf("guardWith(%q): %v", q, err)
		}
	}
}

func TestGuardFallbackFormatTableFunction(t *testing.T) {
	for _, q := range []string{"SELECT * FROM format(CSV, 'a')", "SELECT * FROM t, fuzzQuery('select 1')"} {
		if _, err := guard(q); !errors.Is(err, datasource.ErrForbiddenQuery) {
			t.Errorf("guard(%q) err = %v, want ErrForbiddenQuery", q, err)
		}
	}
}
