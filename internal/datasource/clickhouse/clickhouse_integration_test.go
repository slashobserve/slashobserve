package clickhouse_test

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	ch "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/testcontainers/testcontainers-go"
	tcclickhouse "github.com/testcontainers/testcontainers-go/modules/clickhouse"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/clickhouse"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	image       = "clickhouse/clickhouse-server:26.8.15.10@sha256:9716a352d25f538bd19d868b001141e7a60e4d035bc9b823658c6a071660aa61"
	roUser      = "slashobserve_ro"
	ro1User     = "slashobserve_ro1"
	ipUser      = "slashobserve_ip"
	capUser     = "slashobserve_cap"
	urlUser     = "slashobserve_url"
	roleUser    = "slashobserve_role"
	readURLUser = "slashobserve_read_url"
	readAllUser = "slashobserve_read_all"
	writeS3User = "slashobserve_write_s3"
	tempUser    = "slashobserve_temp"
	filteredURL = "slashobserve_filtered_url"
	adminDB     = "otel"
)

//go:embed testdata/otel_schema.sql
var schemaSQL string

//go:embed testdata/readonly_user.sql
var readonlyUserSQL string

//go:embed testdata/dead_cluster.xml
var deadClusterXML string

var (
	host         string
	adminPass    string
	roPass       string
	testsSkipped bool
)

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	flag.Parse()
	if testing.Short() {
		testsSkipped = true
		return m.Run()
	}
	ctx := context.Background()
	adminPass = randomHex()
	roPass = randomHex()
	container, err := tcclickhouse.Run(ctx, image,
		tcclickhouse.WithUsername("admin"),
		tcclickhouse.WithPassword(adminPass),
		tcclickhouse.WithDatabase(adminDB),
		testcontainers.WithEnv(map[string]string{"CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT": "1"}),
		testcontainers.WithFiles(testcontainers.ContainerFile{
			Reader:            strings.NewReader(deadClusterXML),
			ContainerFilePath: "/etc/clickhouse-server/config.d/dead_cluster.xml",
			FileMode:          0o644,
		}),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "start clickhouse:", err)
		return 1
	}
	defer func() { _ = testcontainers.TerminateContainer(container) }()
	host, err = container.ConnectionHost(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "clickhouse host:", err)
		return 1
	}
	if err := provision(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "provision clickhouse:", err)
		return 1
	}
	return m.Run()
}

func extraUsersSQL(pass string) string {
	return `CREATE USER IF NOT EXISTS ` + ro1User + ` IDENTIFIED WITH sha256_password BY '` + pass + `' SETTINGS readonly = 1;
GRANT SELECT ON otel.* TO ` + ro1User + `;
CREATE USER IF NOT EXISTS ` + capUser + ` IDENTIFIED WITH sha256_password BY '` + pass + `' SETTINGS readonly = 2 CONST, max_execution_time = 5 MIN 1 MAX 8;
GRANT SELECT ON otel.* TO ` + capUser + `;
CREATE USER IF NOT EXISTS ` + urlUser + ` IDENTIFIED WITH sha256_password BY '` + pass + `' SETTINGS readonly = 2;
GRANT SELECT ON otel.* TO ` + urlUser + `;
GRANT URL ON *.* TO ` + urlUser + `;
CREATE USER IF NOT EXISTS ` + readURLUser + ` IDENTIFIED WITH sha256_password BY '` + pass + `' SETTINGS readonly = 2;
GRANT SELECT ON otel.* TO ` + readURLUser + `;
GRANT READ ON URL TO ` + readURLUser + `;
CREATE USER IF NOT EXISTS ` + readAllUser + ` IDENTIFIED WITH sha256_password BY '` + pass + `' SETTINGS readonly = 2;
GRANT SELECT ON otel.* TO ` + readAllUser + `;
GRANT READ ON * TO ` + readAllUser + `;
CREATE USER IF NOT EXISTS ` + writeS3User + ` IDENTIFIED WITH sha256_password BY '` + pass + `' SETTINGS readonly = 2;
GRANT SELECT ON otel.* TO ` + writeS3User + `;
GRANT WRITE ON S3 TO ` + writeS3User + `;
CREATE USER IF NOT EXISTS ` + tempUser + ` IDENTIFIED WITH sha256_password BY '` + pass + `' SETTINGS readonly = 2;
GRANT SELECT ON otel.* TO ` + tempUser + `;
GRANT CREATE TEMPORARY TABLE ON *.* TO ` + tempUser + `;
CREATE USER IF NOT EXISTS ` + filteredURL + ` IDENTIFIED WITH sha256_password BY '` + pass + `' SETTINGS readonly = 2;
GRANT SELECT ON otel.* TO ` + filteredURL + `;
GRANT READ ON URL('http://127.0.0.1:1/.*') TO ` + filteredURL + `;
GRANT CREATE TEMPORARY TABLE ON *.* TO ` + filteredURL + `;
CREATE ROLE IF NOT EXISTS slashobserve_s3_role;
GRANT S3 ON *.* TO slashobserve_s3_role;
CREATE USER IF NOT EXISTS ` + roleUser + ` IDENTIFIED WITH sha256_password BY '` + pass + `' SETTINGS readonly = 2;
GRANT SELECT ON otel.* TO ` + roleUser + `;
GRANT slashobserve_s3_role TO ` + roleUser + `;
CREATE USER IF NOT EXISTS ` + ipUser + ` IDENTIFIED WITH sha256_password BY '` + pass + `' HOST IP '203.0.113.5';
CREATE TABLE IF NOT EXISTS otel.dead_shard (dummy UInt8) ENGINE = Distributed(dead_cluster, system, one);
`
}

func randomHex() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func rawConn(user, pass string) (driver.Conn, error) {
	return ch.Open(&ch.Options{
		Protocol: ch.Native,
		Addr:     []string{host},
		Auth:     ch.Auth{Database: adminDB, Username: user, Password: pass},
	})
}

func statements(script string) []string {
	var out []string
	for _, s := range strings.Split(script, ";\n") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func provision(ctx context.Context) error {
	admin, err := rawConn("admin", adminPass)
	if err != nil {
		return err
	}
	defer func() { _ = admin.Close() }()
	script := schemaSQL + "\n" + strings.ReplaceAll(readonlyUserSQL, "__PASSWORD__", roPass)
	script += "\n" + extraUsersSQL(roPass)
	for _, stmt := range statements(script) {
		if err := admin.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("%w in %q", err, stmt)
		}
	}
	fixtures := []string{
		`INSERT INTO otel.otel_logs (Timestamp, ServiceName, SeverityText, Body)
		 SELECT now64(9) - toIntervalMinute(m), s, sev, body FROM (
		   SELECT arrayJoin([
		     (1, 'checkout', 'ERROR', 'payment declined'),
		     (2, 'checkout', 'ERROR', 'payment declined'),
		     (3, 'checkout', 'ERROR', 'timeout'),
		     (180, 'checkout', 'ERROR', 'old'),
		     (1, 'cart', 'INFO', 'added'),
		     (2, 'cart', 'INFO', 'added'),
		     (3, 'cart', 'INFO', 'added')
		   ]) AS t, t.1 AS m, t.2 AS s, t.3 AS sev, t.4 AS body)`,
		`INSERT INTO otel.otel_metrics_gauge
		 SELECT map('service.name', 'checkout'), 'checkout', 'http.server.active_requests', 'Active requests', '{request}',
		        map('pod', pod), now64(9) - toIntervalSecond(number * 10), toFloat64(number % 7)
		 FROM numbers(60) ARRAY JOIN ['a', 'b'] AS pod`,
		`INSERT INTO otel.otel_metrics_sum
		 SELECT map('service.name', 'checkout'), 'checkout', 'http.server.request.count', 'Requests', '{request}',
		        map('pod', pod), now64(9) - toIntervalSecond(number * 10), toFloat64(number), true
		 FROM numbers(60) ARRAY JOIN ['a', 'b'] AS pod`,
		`INSERT INTO otel.otel_traces
		 SELECT now64(9) - toIntervalSecond(number * 30), hex(number), hex(number + 100), 'GET /cart', 'checkout',
		        1000000 * (number + 1), 'STATUS_CODE_OK', map('http.method', 'GET'), map('service.name', 'checkout')
		 FROM numbers(5)`,
	}
	for _, stmt := range fixtures {
		if err := admin.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("%w in %q", err, stmt)
		}
	}
	return nil
}

func newDS(t *testing.T, user, pass string, limits datasource.Limits) *clickhouse.Datasource {
	t.Helper()
	if testsSkipped {
		t.Skip("integration tests need Docker")
	}
	ds, err := clickhouse.New(clickhouse.Options{
		Addr:     host,
		Database: adminDB,
		Username: user,
		Password: datasource.NewSecret(pass),
		Limits:   limits,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ds.Close() })
	return ds
}

func roDS(t *testing.T) *clickhouse.Datasource {
	return newDS(t, roUser, roPass, datasource.Limits{})
}

func lastHour() datasource.TimeRange {
	now := time.Now()
	return datasource.TimeRange{From: now.Add(-time.Hour), To: now.Add(time.Minute)}
}

func query(text string, vars ...datasource.Variable) datasource.Query {
	return datasource.Query{Text: text, Variables: vars}
}

func column(t *testing.T, f datasource.Frame, name string) datasource.Field {
	t.Helper()
	for _, fld := range f.Fields {
		if fld.Name == name {
			return fld
		}
	}
	t.Fatalf("frame has no field %q: %+v", name, f.Fields)
	return datasource.Field{}
}

func instantRows(t *testing.T, ds *clickhouse.Datasource, q datasource.Query) int {
	t.Helper()
	frames, err := ds.QueryInstant(context.Background(), q, lastHour())
	if err != nil {
		t.Fatalf("QueryInstant(%q): %v", q.Text, err)
	}
	if len(frames) != 1 {
		t.Fatalf("frames = %d, want 1", len(frames))
	}
	return frames[0].Rows()
}

const (
	panelQuery = "SELECT Body AS message, count() AS n FROM otel_logs WHERE ServiceName = 'checkout' AND SeverityText = 'ERROR' AND $__timeFilter(Timestamp) GROUP BY message ORDER BY n DESC LIMIT 20"
	rangeQuery = "SELECT $__timeGroup(TimeUnix) AS t, Attributes['pod'] AS pod, avg(Value) AS v FROM otel_metrics_gauge WHERE MetricName = 'http.server.active_requests' AND $__timeFilter(TimeUnix) GROUP BY t, pod ORDER BY t"
)

func TestHealth(t *testing.T) {
	ctx := context.Background()
	t.Run("read-only user is healthy", func(t *testing.T) {
		if err := roDS(t).CheckHealth(ctx); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("writable user is unhealthy", func(t *testing.T) {
		err := newDS(t, "admin", adminPass, datasource.Limits{}).CheckHealth(ctx)
		if err == nil || !strings.Contains(err.Error(), "not read-only") {
			t.Fatalf("err = %v, want not read-only", err)
		}
		if !errors.Is(err, datasource.ErrForbiddenQuery) {
			t.Fatalf("err = %v, want ErrForbiddenQuery", err)
		}
		if _, err := newDS(t, "admin", adminPass, datasource.Limits{}).QueryInstant(ctx, query("SELECT 1"), lastHour()); !errors.Is(err, datasource.ErrForbiddenQuery) {
			t.Fatalf("QueryInstant err = %v, want ErrForbiddenQuery", err)
		}
	})
	t.Run("readonly=1 user is refused with a specific error", func(t *testing.T) {
		err := newDS(t, ro1User, roPass, datasource.Limits{}).CheckHealth(ctx)
		if !errors.Is(err, datasource.ErrForbiddenQuery) || !strings.Contains(err.Error(), "readonly=2") {
			t.Fatalf("err = %v, want ErrForbiddenQuery mentioning readonly=2", err)
		}
	})
	t.Run("wrong password leaks nothing", func(t *testing.T) {
		err := newDS(t, roUser, "wrong-"+roPass, datasource.Limits{}).CheckHealth(ctx)
		if !errors.Is(err, datasource.ErrUnavailable) {
			t.Fatalf("err = %v, want ErrUnavailable", err)
		}
		hostname, port, _ := strings.Cut(host, ":")
		for _, secret := range []string{hostname, port, roUser, roPass, "wrong-"} {
			if strings.Contains(err.Error(), secret) {
				t.Fatalf("error %q leaks %q", err, secret)
			}
		}
	})
}

func TestPanelQuery(t *testing.T) {
	frames, err := roDS(t).QueryInstant(context.Background(), query(panelQuery), lastHour())
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 1 || frames[0].Rows() != 2 {
		t.Fatalf("frames = %+v, want one frame with 2 rows", frames)
	}
	message, n := column(t, frames[0], "message"), column(t, frames[0], "n")
	if message.Values[0] != "payment declined" || n.Values[0] != float64(2) {
		t.Fatalf("first row = %v/%v, want payment declined/2", message.Values[0], n.Values[0])
	}
}

func TestRangeQuery(t *testing.T) {
	ds := roDS(t)
	ctx := context.Background()
	frames, err := ds.QueryRange(ctx, query(rangeQuery), lastHour(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 2 {
		t.Fatalf("frames = %d, want 2", len(frames))
	}
	var pods []string
	for _, f := range frames {
		pods = append(pods, f.Labels["pod"])
		if len(f.Fields) != 2 || f.Fields[0].Name != "Time" || f.Fields[0].Type != datasource.FieldTime || f.Fields[1].Name != "v" {
			t.Fatalf("fields = %+v, want Time then v", f.Fields)
		}
	}
	if !slices.Equal(pods, []string{"a", "b"}) && !slices.Equal(pods, []string{"b", "a"}) {
		t.Fatalf("pods = %v", pods)
	}

	interval, err := ds.QueryRange(ctx, query("SELECT $__interval AS i"), lastHour(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if got := interval[0].Fields[0].Values[0]; got != float64(60) {
		t.Fatalf("$__interval = %v, want 60", got)
	}

	used := panelQuery + rangeQuery + "SELECT $__interval AS i"
	for _, m := range spec.Macros(spec.ClickHouse) {
		if !strings.Contains(used, "$"+m.Name) {
			t.Errorf("macro %s is not exercised by any query", m.Name)
		}
	}
}

func TestGuardAndGrants(t *testing.T) {
	ctx := context.Background()
	ds := roDS(t)
	for _, text := range []string{
		"INSERT INTO otel_logs (Body) VALUES ('x')",
		"SELECT 1; DROP TABLE otel_logs",
		"SELECT 1 SETTINGS max_execution_time = 1000",
		"SELECT * FROM url('http://127.0.0.1:1/x', CSV)",
		"SELECT 1 FORMAT JSON",
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
		"SELECT 1 // '\nSETTINGS max_result_rows=5 -- '",
		"SELECT 1 AS x\u00a0SETTINGS max_result_rows=6",
		"SELECT * FROM\u00a0url('http://127.0.0.1:1/x', CSV)",
		"SELECT \u2018'\u2019 AS a SETTINGS max_result_rows=7 -- '",
		"SELECT * FROM \u201curl\u201d('http://127.0.0.1:1/x', CSV)",
		"SELECT * FROM cosn('cosn://b/k')",
		"SELECT * FROM bigquery('p', 'd', 't')",
		"SELECT * FROM filesystem('/etc')",
		"SELECT * FROM eval('SELECT 1')",
		"SELECT * FROM loop(numbers(1))",
		"SELECT * FROM mergeTreeIndex('otel', 'otel_logs')",
		"SELECT * FROM format(CSV, 'a,b')",
		"SELECT * FROM `\\x75rl`('http://127.0.0.1:1/x', CSV, 'a String')",
		"SELECT * FROM \"\\x75rl\"('http://127.0.0.1:1/x', CSV, 'a String')",
	} {
		if _, err := ds.QueryInstant(ctx, query(text), lastHour()); !errors.Is(err, datasource.ErrForbiddenQuery) {
			t.Errorf("QueryInstant(%q) err = %v, want ErrForbiddenQuery", text, err)
		}
	}
	for _, text := range []string{"SELECT 'abc", "SELECT 1 /* x", "SELECT $$ x", "SELECT `abc", "SELECT \"abc"} {
		if _, err := ds.QueryInstant(ctx, query(text), lastHour()); err == nil {
			t.Errorf("QueryInstant(%q) succeeded, want a rejection", text)
		}
	}
	for _, text := range []string{
		"SELECT 'a;b'", "SELECT 1;", "SELECT 1 -- ; DROP TABLE otel_logs", "SELECT $$ a;b $$",
		"WITH x AS (SELECT 1 AS v) SELECT * FROM x", "SELECT 1 AS `format`",
		"SELECT format('{}', 1)", "SELECT name FROM system.settings LIMIT 1",
	} {
		if _, err := ds.QueryInstant(ctx, query(text), lastHour()); err != nil {
			t.Errorf("QueryInstant(%q): %v", text, err)
		}
	}

	t.Run("server refuses what the guard would", func(t *testing.T) {
		conn, err := rawConn(roUser, roPass)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = conn.Close() }()
		for _, text := range []string{
			"INSERT INTO otel_logs (Body) VALUES ('x')",
			"SELECT * FROM url('http://127.0.0.1:1/x', CSV)",
			"SELECT 1 SETTINGS max_execution_time = 100000",
			"SELECT 1 SETTINGS readonly = 0",
			"SELECT 1 SETTINGS max_execution_time = 0",
		} {
			rows, err := conn.Query(ctx, text)
			if err == nil {
				for rows.Next() {
				}
				err = rows.Err()
				_ = rows.Close()
			}
			if err == nil {
				t.Errorf("raw %q succeeded for the read-only user", text)
			}
		}
	})
}

func TestSourceGrantsRefused(t *testing.T) {
	if testsSkipped {
		t.Skip("integration tests need Docker")
	}
	ctx := context.Background()
	conn, err := rawConn(filteredURL, roPass)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	var readsURL uint8
	if err := conn.QueryRow(ctx, "CHECK GRANT READ ON URL").Scan(&readsURL); err != nil || readsURL != 0 {
		t.Fatalf("CHECK GRANT READ ON URL for a filtered grant = %d, %v; want 0, which only the CREATE TEMPORARY TABLE probe can catch", readsURL, err)
	}
	for _, tc := range []struct{ user, grant string }{
		{urlUser, "URL"}, {roleUser, "S3"}, {readURLUser, "READ ON URL"}, {readAllUser, "READ ON"},
		{writeS3User, "WRITE ON S3"}, {tempUser, "CREATE TEMPORARY TABLE"},
		{filteredURL, "CREATE TEMPORARY TABLE"},
	} {
		ds := newDS(t, tc.user, roPass, datasource.Limits{})
		err := ds.CheckHealth(ctx)
		if !errors.Is(err, datasource.ErrForbiddenQuery) || !strings.Contains(err.Error(), tc.grant) {
			t.Errorf("CheckHealth(%s) err = %v, want ErrForbiddenQuery naming %s", tc.user, err, tc.grant)
		}
		if _, err := ds.QueryInstant(ctx, query("SELECT 1"), lastHour()); !errors.Is(err, datasource.ErrForbiddenQuery) {
			t.Errorf("QueryInstant(%s) err = %v, want ErrForbiddenQuery", tc.user, err)
		}
		if got := ds.Lint(ctx, "SELECT 1"); len(got) != 1 || got[0].Severity != datasource.SeverityWarning {
			t.Errorf("Lint(%s) = %+v, want one warning", tc.user, got)
		}
	}
	if err := roDS(t).CheckHealth(ctx); err != nil {
		t.Errorf("CheckHealth(shipped read-only user): %v", err)
	}
}

func TestLimits(t *testing.T) {
	ctx := context.Background()
	t.Run("max rows", func(t *testing.T) {
		ds := newDS(t, roUser, roPass, datasource.Limits{MaxRows: 5})
		_, err := ds.QueryInstant(ctx, query("SELECT * FROM otel_metrics_gauge"), lastHour())
		if !errors.Is(err, datasource.ErrLimitExceeded) {
			t.Fatalf("err = %v, want ErrLimitExceeded", err)
		}
	})
	t.Run("timeout", func(t *testing.T) {
		ds := roDS(t)
		q := query("SELECT sleep(3)")
		q.Limits = datasource.Limits{Timeout: time.Second}
		start := time.Now()
		_, err := ds.QueryInstant(ctx, q, lastHour())
		if !errors.Is(err, datasource.ErrTimeout) {
			t.Fatalf("err = %v, want ErrTimeout", err)
		}
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Fatalf("took %s", elapsed)
		}
	})
	t.Run("max series", func(t *testing.T) {
		ds := newDS(t, roUser, roPass, datasource.Limits{MaxSeries: 1})
		_, err := ds.QueryRange(ctx, query(rangeQuery), lastHour(), time.Minute)
		if !errors.Is(err, datasource.ErrLimitExceeded) {
			t.Fatalf("err = %v, want ErrLimitExceeded", err)
		}
	})
	t.Run("max points", func(t *testing.T) {
		ds := newDS(t, roUser, roPass, datasource.Limits{MaxPoints: 3})
		_, err := ds.QueryRange(ctx, query(rangeQuery), lastHour(), time.Minute)
		if !errors.Is(err, datasource.ErrLimitExceeded) {
			t.Fatalf("err = %v, want ErrLimitExceeded", err)
		}
	})
	t.Run("a looser query limit never raises the configured one", func(t *testing.T) {
		ds := newDS(t, roUser, roPass, datasource.Limits{MaxRows: 5})
		q := query("SELECT * FROM otel_metrics_gauge")
		q.Limits = datasource.Limits{MaxRows: 100000}
		if _, err := ds.QueryInstant(ctx, q, lastHour()); !errors.Is(err, datasource.ErrLimitExceeded) {
			t.Fatalf("err = %v, want ErrLimitExceeded", err)
		}
	})
}

func TestVariables(t *testing.T) {
	ds := roDS(t)
	svc := func(values ...string) datasource.Variable { return datasource.Variable{Name: "svc", Values: values} }
	const inQuery = "SELECT Body FROM otel_logs WHERE ServiceName IN ($svc)"
	if n := instantRows(t, ds, query(inQuery, svc("checkout"))); n != 4 {
		t.Errorf("checkout rows = %d, want 4", n)
	}
	if n := instantRows(t, ds, query(inQuery, svc("x') OR 1=1 --"))); n != 0 {
		t.Errorf("injection rows = %d, want 0", n)
	}
	if n := instantRows(t, ds, query("SELECT Body FROM otel_logs WHERE ServiceName = '$svc'", svc("checkout' OR '1'='1"))); n != 0 {
		t.Errorf("quoted injection rows = %d, want 0", n)
	}
	all := datasource.Variable{Name: "svc", Values: []string{"checkout", "cart"}, All: true}
	if n := instantRows(t, ds, query(inQuery, all)); n != 7 {
		t.Errorf("All rows = %d, want 7", n)
	}
	_, err := ds.QueryInstant(context.Background(), query("SELECT ${svc:raw}", svc("a b")), lastHour())
	if !errors.Is(err, datasource.ErrInvalidQuery) {
		t.Errorf("raw err = %v, want ErrInvalidQuery", err)
	}
}

func TestLint(t *testing.T) {
	ds := roDS(t)
	ctx := context.Background()
	for _, q := range []string{
		"SELECT 1",
		"SELECT count() FROM otel_logs WHERE $__timeFilter(Timestamp)",
		"SELECT Body FROM otel_logs WHERE ServiceName IN ($svc)",
	} {
		if got := ds.Lint(ctx, q); got == nil || len(got) != 0 {
			t.Errorf("Lint(%q) = %+v, want empty", q, got)
		}
	}
	if got := ds.Lint(ctx, "SELEC 1"); len(got) != 1 || got[0].Severity != datasource.SeverityError {
		t.Errorf("Lint(SELEC 1) = %+v, want one error", got)
	}
	got := ds.Lint(ctx, "SELECT count( FROM otel_logs")
	if len(got) != 1 || got[0].Severity != datasource.SeverityError || got[0].Start <= 0 {
		t.Errorf("Lint(count() = %+v, want one error with Start > 0", got)
	}
	got = ds.Lint(ctx, "SELECT nope FROM otel_logs")
	if len(got) != 1 || got[0].Severity != datasource.SeverityError || !strings.Contains(got[0].Message, "nope") {
		t.Errorf("Lint(nope) = %+v, want one error mentioning nope", got)
	}
	got = ds.Lint(ctx, "SELECT * FROM missing_table")
	if len(got) != 1 || got[0].Severity != datasource.SeverityError {
		t.Errorf("Lint(missing_table) = %+v, want one error", got)
	}
	got = ds.Lint(ctx, "SELECT 1 SETTINGS max_execution_time = 1")
	if len(got) != 1 || got[0].Severity != datasource.SeverityError {
		t.Errorf("Lint(SETTINGS) = %+v, want one error", got)
	}
	for _, q := range []string{"SELECT 1 // '\nSETTINGS max_result_rows=5 -- '", "SELECT 1 AS x\u00a0SETTINGS max_result_rows=6"} {
		if got := ds.Lint(ctx, q); len(got) != 1 || got[0].Severity != datasource.SeverityError || !strings.Contains(got[0].Message, "SETTINGS") {
			t.Errorf("Lint(%q) = %+v, want one SETTINGS error", q, got)
		}
	}
	for _, q := range []string{"SELECT format('{}', 1)", "SELECT name FROM system.settings LIMIT 1"} {
		if got := ds.Lint(ctx, q); len(got) != 0 {
			t.Errorf("Lint(%q) = %+v, want empty", q, got)
		}
	}
	if got := newDS(t, capUser, roPass, datasource.Limits{}).Lint(ctx, "SELECT 1"); len(got) != 0 {
		t.Errorf("Lint under a profile maximum of 8s = %+v, want empty", got)
	}
	const padding = "   \n  "
	got = ds.Lint(ctx, padding+"SELECT count( FROM otel_logs")
	if want := len(padding) + len("SELECT count"); len(got) != 1 || got[0].Start != want {
		t.Errorf("Lint(padded count() = %+v, want one error starting at %d", got, want)
	}
	got = ds.Lint(ctx, "SELECT ServiceName FROM otel_logs WHERE ServiceName IN ($svc) AND")
	if len(got) != 1 {
		t.Errorf("Lint(trailing AND) = %+v, want one diagnostic", got)
	}
}

func TestDiscover(t *testing.T) {
	ds := roDS(t)
	ctx := context.Background()
	discover := func(req datasource.DiscoverRequest) *datasource.Catalog {
		t.Helper()
		cat, err := ds.Discover(ctx, req)
		if err != nil {
			t.Fatalf("Discover(%+v): %v", req, err)
		}
		return cat
	}

	tables := map[string]string{}
	for _, tb := range discover(datasource.DiscoverRequest{Kind: datasource.DiscoverTables}).Tables {
		tables[tb.Name] = tb.Kind
	}
	for name, kind := range map[string]string{"otel_logs": "logs", "otel_metrics_gauge": "metrics_gauge", "otel_traces": "traces", "otel_metrics_sum": "metrics_sum"} {
		if tables[name] != kind {
			t.Errorf("table %s kind = %q, want %q (all: %v)", name, tables[name], kind, tables)
		}
	}

	columns := discover(datasource.DiscoverRequest{Kind: datasource.DiscoverColumns, Table: "otel_logs"}).Columns
	if !slices.Contains(columns, datasource.Column{Name: "Timestamp", Type: "DateTime64(9)"}) {
		t.Errorf("columns = %+v, want Timestamp DateTime64(9)", columns)
	}

	metrics := discover(datasource.DiscoverRequest{Kind: datasource.DiscoverMetrics, Search: "active"}).Metrics
	want := datasource.Metric{Name: "http.server.active_requests", Type: "gauge", Help: "Active requests", Unit: "{request}", Table: "otel_metrics_gauge"}
	if len(metrics) != 1 || metrics[0] != want {
		t.Errorf("metrics = %+v, want [%+v]", metrics, want)
	}
	if all := discover(datasource.DiscoverRequest{Kind: datasource.DiscoverMetrics}).Metrics; len(all) != 2 || all[0].Name > all[1].Name {
		t.Errorf("metrics = %+v, want two sorted by name", all)
	}

	labels := discover(datasource.DiscoverRequest{Kind: datasource.DiscoverLabels, Table: "otel_metrics_gauge", Metric: "http.server.active_requests"}).Labels
	for _, l := range []string{"Attributes['pod']", "ResourceAttributes['service.name']", "ServiceName"} {
		if !slices.Contains(labels, l) {
			t.Errorf("labels = %v, want %s", labels, l)
		}
	}
	if none := discover(datasource.DiscoverRequest{Kind: datasource.DiscoverLabels, Table: "otel_metrics_gauge", Metric: "nope"}).Labels; slices.Contains(none, "Attributes['pod']") {
		t.Errorf("labels for unknown metric = %v", none)
	}

	values := discover(datasource.DiscoverRequest{Kind: datasource.DiscoverLabelValues, Table: "otel_metrics_gauge", Label: "Attributes['pod']"}).Values
	if !slices.Equal(values, []string{"a", "b"}) {
		t.Errorf("values = %v, want [a b]", values)
	}
	values = discover(datasource.DiscoverRequest{Kind: datasource.DiscoverLabelValues, Table: "otel_logs", Label: "ServiceName", Search: "CHECK"}).Values
	if !slices.Equal(values, []string{"checkout"}) {
		t.Errorf("searched values = %v, want [checkout]", values)
	}
	values = discover(datasource.DiscoverRequest{Kind: datasource.DiscoverLabelValues, Table: "otel_logs", Label: "ServiceName", Search: "x') OR 1=1 --"}).Values
	if len(values) != 0 {
		t.Errorf("injected search values = %v, want none", values)
	}

	sample := discover(datasource.DiscoverRequest{Kind: datasource.DiscoverSampleRows, Table: "otel_logs"}).Sample
	if sample == nil || sample.Rows() == 0 || sample.Rows() > 5 || column(t, *sample, "Timestamp").Type != datasource.FieldTime {
		t.Errorf("sample = %+v", sample)
	}

	_, err := ds.Discover(ctx, datasource.DiscoverRequest{Kind: datasource.DiscoverLabelValues, Table: "otel_metrics_gauge", Label: "Attributes['x') OR 1=1 --']"})
	if !errors.Is(err, datasource.ErrInvalidQuery) {
		t.Errorf("injected label err = %v, want ErrInvalidQuery", err)
	}
	_, err = ds.Discover(ctx, datasource.DiscoverRequest{Kind: datasource.DiscoverColumns, Table: "missing"})
	if !errors.Is(err, datasource.ErrNotFound) {
		t.Errorf("missing table err = %v, want ErrNotFound", err)
	}
	_, err = ds.Discover(ctx, datasource.DiscoverRequest{Kind: datasource.DiscoverSampleRows, Table: "missing"})
	if !errors.Is(err, datasource.ErrNotFound) {
		t.Errorf("missing sample table err = %v, want ErrNotFound", err)
	}
}

func TestVariableValues(t *testing.T) {
	ds := roDS(t)
	ctx := context.Background()
	values, err := ds.VariableValues(ctx, query("SELECT DISTINCT ServiceName FROM otel_logs ORDER BY 1"), lastHour())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(values, []string{"cart", "checkout"}) {
		t.Errorf("values = %v, want [cart checkout]", values)
	}
	if _, err := ds.VariableValues(ctx, query("INSERT INTO otel_logs (Body) VALUES ('x')"), lastHour()); !errors.Is(err, datasource.ErrForbiddenQuery) {
		t.Errorf("INSERT err = %v, want ErrForbiddenQuery", err)
	}
}

func TestColumnTyping(t *testing.T) {
	frames, err := roDS(t).QueryInstant(context.Background(), query(`SELECT
		toUInt64(1) AS a, toDecimal64(1.5, 2) AS b, [1, 2] AS c, map('a', 1) AS d, true AS e,
		generateUUIDv4() AS f, toIPv4('1.2.3.4') AS g, toDate('2026-01-01') AS h,
		CAST(NULL AS Nullable(Float64)) AS i, nan AS j`), lastHour())
	if err != nil {
		t.Fatal(err)
	}
	types := map[string]datasource.FieldType{
		"a": datasource.FieldNumber, "b": datasource.FieldNumber, "c": datasource.FieldOther, "d": datasource.FieldOther,
		"e": datasource.FieldBool, "f": datasource.FieldString, "g": datasource.FieldString, "h": datasource.FieldTime,
		"i": datasource.FieldNumber, "j": datasource.FieldNumber,
	}
	for name, want := range types {
		if got := column(t, frames[0], name).Type; got != want {
			t.Errorf("column %s type = %s, want %s", name, got, want)
		}
	}
	values := func(name string) any { return column(t, frames[0], name).Values[0] }
	if values("a") != float64(1) || values("b") != 1.5 || values("c") != "[1,2]" || values("d") != `{"a":1}` || values("e") != true || values("g") != "1.2.3.4" {
		t.Errorf("unexpected values: %+v", frames[0].Fields)
	}
	if values("i") != nil || values("j") != nil {
		t.Errorf("null and nan must be nil, got %v and %v", values("i"), values("j"))
	}
	if got := values("h"); got != time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) {
		t.Errorf("date = %v", got)
	}
}

func TestColumnTypingJSONArrays(t *testing.T) {
	frames, err := roDS(t).QueryInstant(context.Background(), query(`SELECT
		[toUUID('61f0c404-5cb3-11e7-907b-a6006ad3dba0')] AS uuids,
		[toIPv4('1.2.3.4')] AS ip4s,
		[toIPv6('2001:db8::1')] AS ip6s,
		[toInt128('170141183460469231731687303715884105727')] AS int128s,
		[toUInt256('115792089237316195423570985008687907853269984665640564039457584007913129639935')] AS uint256s,
		map('k', toUUID('61f0c404-5cb3-11e7-907b-a6006ad3dba0')) AS uuidmap,
		[toUInt8(1), toUInt8(2)] AS bytes`), lastHour())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"uuids":    `["61f0c404-5cb3-11e7-907b-a6006ad3dba0"]`,
		"ip4s":     `["1.2.3.4"]`,
		"ip6s":     `["2001:db8::1"]`,
		"int128s":  `[170141183460469231731687303715884105727]`,
		"uint256s": `[115792089237316195423570985008687907853269984665640564039457584007913129639935]`,
		"uuidmap":  `{"k":"61f0c404-5cb3-11e7-907b-a6006ad3dba0"}`,
		"bytes":    `[1,2]`,
	}
	for name, w := range want {
		if got := column(t, frames[0], name).Values[0]; got != w {
			t.Errorf("%s = %v, want %s", name, got, w)
		}
	}
}

func TestDiscoverMetricsWithManyTables(t *testing.T) {
	if testsSkipped {
		t.Skip("integration tests need Docker")
	}
	ctx := context.Background()
	admin, err := rawConn("admin", adminPass)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	var created []string
	t.Cleanup(func() {
		for _, name := range created {
			_ = admin.Exec(ctx, "DROP TABLE IF EXISTS otel."+name)
		}
	})
	create := func(name, columns string) {
		t.Helper()
		if err := admin.Exec(ctx, "CREATE TABLE otel."+name+" ("+columns+") ENGINE = MergeTree ORDER BY tuple()"); err != nil {
			t.Fatal(err)
		}
		created = append(created, name)
	}
	for i := range 12 {
		create(fmt.Sprintf("many_tables_%02d", i), "x UInt8")
	}
	create("otel_metrics_histogram", "x UInt8")

	ds := newDS(t, roUser, roPass, datasource.Limits{MaxRows: 5})
	cat, err := ds.Discover(ctx, datasource.DiscoverRequest{Kind: datasource.DiscoverMetrics})
	if err != nil {
		t.Fatal(err)
	}
	if len(cat.Metrics) != 2 {
		t.Fatalf("metrics = %+v, want the two standard metrics", cat.Metrics)
	}
}

func TestErrorMapping(t *testing.T) {
	ctx := context.Background()
	ipPattern := regexp.MustCompile(`\d+\.\d+\.\d+\.\d+`)
	t.Run("timeout above the profile maximum still runs", func(t *testing.T) {
		ds := newDS(t, roUser, roPass, datasource.Limits{Timeout: 400 * time.Second})
		if _, err := ds.QueryInstant(ctx, query("SELECT 1"), lastHour()); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("timeout message reports the effective deadline", func(t *testing.T) {
		ds := roDS(t)
		if _, err := ds.QueryInstant(ctx, query("SELECT 1"), lastHour()); err != nil {
			t.Fatal(err)
		}
		parent, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
		defer cancel()
		_, err := ds.QueryInstant(parent, query("SELECT sleep(3)"), lastHour())
		if !errors.Is(err, datasource.ErrTimeout) || !strings.Contains(err.Error(), "after 1.5s") {
			t.Fatalf("err = %v, want ErrTimeout naming the 1.5s deadline", err)
		}
	})
	t.Run("unreachable shard leaks no address", func(t *testing.T) {
		_, err := roDS(t).QueryInstant(ctx, query("SELECT * FROM dead_shard"), lastHour())
		if !errors.Is(err, datasource.ErrUnavailable) || ipPattern.MatchString(err.Error()) {
			t.Fatalf("err = %v, want ErrUnavailable without an address", err)
		}
	})
	t.Run("disallowed client ip is an authentication failure without an address", func(t *testing.T) {
		err := newDS(t, ipUser, roPass, datasource.Limits{}).CheckHealth(ctx)
		if !errors.Is(err, datasource.ErrUnavailable) || ipPattern.MatchString(err.Error()) {
			t.Fatalf("err = %v, want ErrUnavailable without an address", err)
		}
	})
	t.Run("unknown exception codes are generic", func(t *testing.T) {
		_, err := roDS(t).QueryInstant(ctx, query("SELECT throwIf(1, 'backend 10.1.2.3:9000 down')"), lastHour())
		if !errors.Is(err, datasource.ErrInvalidQuery) || strings.Contains(err.Error(), "10.1.2.3") || !strings.Contains(err.Error(), "ClickHouse error") {
			t.Fatalf("err = %v, want generic ErrInvalidQuery", err)
		}
	})
	t.Run("query mistakes keep their message", func(t *testing.T) {
		_, err := roDS(t).QueryInstant(ctx, query("SELECT nope_column FROM otel_logs"), lastHour())
		if !errors.Is(err, datasource.ErrInvalidQuery) || !strings.Contains(err.Error(), "nope_column") {
			t.Fatalf("err = %v, want ErrInvalidQuery naming nope_column", err)
		}
	})
	t.Run("unreadable column type is not a connection failure", func(t *testing.T) {
		_, err := roDS(t).QueryInstant(ctx, query("SELECT sumState(toUInt8(1)) AS x"), lastHour())
		if !errors.Is(err, datasource.ErrInvalidQuery) || errors.Is(err, datasource.ErrUnavailable) {
			t.Fatalf("err = %v, want ErrInvalidQuery", err)
		}
	})
	t.Run("unreachable server is unavailable", func(t *testing.T) {
		ds, err := clickhouse.New(clickhouse.Options{Addr: "127.0.0.1:1", Username: roUser, Password: datasource.NewSecret(roPass)})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = ds.Close() })
		if err := ds.CheckHealth(ctx); !errors.Is(err, datasource.ErrUnavailable) {
			t.Fatalf("err = %v, want ErrUnavailable", err)
		}
	})
}
