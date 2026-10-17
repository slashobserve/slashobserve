package macros_test

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/macros"
)

var (
	rangeFrom = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	testRange = datasource.TimeRange{From: rangeFrom, To: rangeFrom.Add(6 * time.Hour)}
)

func val(name string, values ...string) datasource.Variable {
	return datasource.Variable{Name: name, Values: values}
}

func all(name string, values ...string) datasource.Variable {
	return datasource.Variable{Name: name, Values: values, All: true}
}

func envWith(vs ...datasource.Variable) macros.Env {
	return macros.Env{Range: testRange, Interval: 30 * time.Second, ScrapeInterval: 15 * time.Second, Variables: vs}
}

type expandCase struct {
	name  string
	d     macros.Dialect
	query string
	env   macros.Env
	want  string
	err   bool
}

func runExpandCases(t *testing.T, cases []expandCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := macros.Expand(tc.d, tc.query, tc.env)
			if tc.err {
				if err == nil {
					t.Fatalf("Expand(%q) = %q, want error", tc.query, got.Text)
				}
				if !errors.Is(err, datasource.ErrInvalidQuery) {
					t.Fatalf("error %v is not ErrInvalidQuery", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Expand(%q): %v", tc.query, err)
			}
			if got.Text != tc.want {
				t.Fatalf("Expand(%q)\n got: %s\nwant: %s", tc.query, got.Text, tc.want)
			}
		})
	}
}

func TestPromQLVariables(t *testing.T) {
	bt := "`"
	runExpandCases(t, []expandCase{
		{name: "regex multi escapes metachar and quote", d: macros.PromQL, query: `up{pod=~"$pod"}`, env: envWith(val("pod", "a.b", `c"d`)), want: `up{pod=~"(a\\.b|c\"d)"}`},
		{name: "regex single", d: macros.PromQL, query: `up{pod=~"$pod"}`, env: envWith(val("pod", "a.b")), want: `up{pod=~"a\\.b"}`},
		{name: "regex all", d: macros.PromQL, query: `up{pod=~"$pod"}`, env: envWith(all("pod", "a", "b")), want: `up{pod=~".*"}`},
		{name: "regex all with no options", d: macros.PromQL, query: `up{pod=~"$pod"}`, env: envWith(all("pod")), want: `up{pod=~".*"}`},
		{name: "negative regex multi", d: macros.PromQL, query: `up{pod!~"$pod"}`, env: envWith(val("pod", "a", "b")), want: `up{pod!~"(a|b)"}`},
		{name: "regex with spaces around operator", d: macros.PromQL, query: `up{pod =~ "$pod"}`, env: envWith(val("pod", "a+b")), want: `up{pod =~ "a\\+b"}`},
		{name: "regex alternation and pipe stay literal", d: macros.PromQL, query: `up{pod=~"$pod"}`, env: envWith(val("pod", "a|b")), want: `up{pod=~"a\\|b"}`},
		{name: "single quoted regex string", d: macros.PromQL, query: `up{pod=~'$pod'}`, env: envWith(val("pod", "it's")), want: `up{pod=~'it\'s'}`},
		{name: "backtick regex string is verbatim", d: macros.PromQL, query: "up{pod=~" + bt + "$pod" + bt + "}", env: envWith(val("pod", "a.b")), want: "up{pod=~" + bt + `a\.b` + bt + "}"},
		{name: "backtick in backtick string", d: macros.PromQL, query: "up{pod=~" + bt + "$pod" + bt + "}", env: envWith(val("pod", "a"+bt+"b")), err: true},
		{name: "equality escapes backslash and quote", d: macros.PromQL, query: `up{pod="$pod"}`, env: envWith(val("pod", `a\b"c`)), want: `up{pod="a\\b\"c"}`},
		{name: "equality unicode", d: macros.PromQL, query: `up{pod="$pod"}`, env: envWith(val("pod", "héllo✓名前")), want: `up{pod="héllo✓名前"}`},
		{name: "not equal is raw", d: macros.PromQL, query: `up{pod!="$pod"}`, env: envWith(val("pod", "x")), want: `up{pod!="x"}`},
		{name: "non matcher string is raw", d: macros.PromQL, query: `label_replace(up, "dst", "$v", "src", "(.*)")`, env: envWith(val("v", "x")), want: `label_replace(up, "dst", "x", "src", "(.*)")`},
		{name: "raw with two values", d: macros.PromQL, query: `up{pod="$pod"}`, env: envWith(val("pod", "x", "y")), err: true},
		{name: "raw with all", d: macros.PromQL, query: `up{pod="$pod"}`, env: envWith(all("pod", "x")), err: true},
		{name: "explicit csv", d: macros.PromQL, query: `up{pod="${pod:csv}"}`, env: envWith(val("pod", "a", "b")), want: `up{pod="a,b"}`},
		{name: "explicit pipe", d: macros.PromQL, query: `up{pod=~"${pod:pipe}"}`, env: envWith(val("pod", "a", "b")), want: `up{pod=~"a|b"}`},
		{name: "explicit regex in equality string", d: macros.PromQL, query: `up{pod="${pod:regex}"}`, env: envWith(val("pod", "a.b", "c")), want: `up{pod="(a\\.b|c)"}`},
		{name: "explicit regex does not leave single quotes unescaped", d: macros.PromQL, query: `up{pod='${pod:regex}'}`, env: envWith(val("pod", "a'b")), want: `up{pod='a\'b'}`},
		{name: "doublequote in code", d: macros.PromQL, query: `foo{a=${v:doublequote}}`, env: envWith(val("v", `x"y`)), want: `foo{a="x\"y"}`},
		{name: "doublequote multi in code", d: macros.PromQL, query: `foo{a=${v:doublequote}}`, env: envWith(val("v", "x", "y")), want: `foo{a="x","y"}`},
		{name: "singlequote in code", d: macros.PromQL, query: `foo{a=${v:singlequote}}`, env: envWith(val("v", "it's")), want: `foo{a='it\'s'}`},
		{name: "code identifier", d: macros.PromQL, query: `sum by ($label) (up)`, env: envWith(val("label", "pod")), want: `sum by (pod) (up)`},
		{name: "code identifier csv", d: macros.PromQL, query: `sum by (${label:csv}) (up)`, env: envWith(val("label", "pod", "node")), want: `sum by (pod,node) (up)`},
		{name: "code duration", d: macros.PromQL, query: `rate(x[$w])`, env: envWith(val("w", "5m")), want: `rate(x[5m])`},
		{name: "code compound duration", d: macros.PromQL, query: `rate(x[$w])`, env: envWith(val("w", "1h30m")), want: `rate(x[1h30m])`},
		{name: "code number", d: macros.PromQL, query: `up > $n`, env: envWith(val("n", "0.5")), want: `up > 0.5`},
		{name: "code injection in identifier", d: macros.PromQL, query: `sum by ($label) (up)`, env: envWith(val("label", "pod) or vector(1")), err: true},
		{name: "code invalid duration", d: macros.PromQL, query: `rate(x[$w])`, env: envWith(val("w", "5x")), err: true},
		{name: "code number then operator", d: macros.PromQL, query: `up > $n`, env: envWith(val("n", "1 or vector(1)")), err: true},
		{name: "code regex is refused", d: macros.PromQL, query: `up{a=${v:regex}}`, env: envWith(val("v", "x")), err: true},
		{name: "code raw with all", d: macros.PromQL, query: `sum by ($label) (up)`, env: envWith(all("label", "pod")), err: true},
		{name: "hash inside string is not a comment", d: macros.PromQL, query: `up{a="#$v"}`, env: envWith(val("v", "x")), want: `up{a="#x"}`},
		{name: "reference in comment is verbatim", d: macros.PromQL, query: "up # $nope ${x:csv} $__bogus\n", env: envWith(), want: "up # $nope ${x:csv} $__bogus\n"},
		{name: "reference after comment line is expanded", d: macros.PromQL, query: "# note\nup{a=\"$v\"}", env: envWith(val("v", "x")), want: "# note\nup{a=\"x\"}"},
		{name: "dollar digit is literal", d: macros.PromQL, query: `label_replace(up, "d", "$1", "s", "(.*)")`, env: envWith(), want: `label_replace(up, "d", "$1", "s", "(.*)")`},
		{name: "backslash before reference", d: macros.PromQL, query: `up{a="\$v"}`, env: envWith(val("v", "x")), err: true},
		{name: "escaped backslash before reference", d: macros.PromQL, query: `up{a="\\$v"}`, env: envWith(val("v", "x")), want: `up{a="\\x"}`},
		{name: "reference repeated", d: macros.PromQL, query: `up{a="$v"} or up{b="$v"}`, env: envWith(val("v", "x")), want: `up{a="x"} or up{b="x"}`},
		{name: "value that looks like a reference is not rescanned", d: macros.PromQL, query: `up{a="$v"}`, env: envWith(val("v", "$w"), val("w", "boom")), want: `up{a="$w"}`},
		{name: "unknown variable", d: macros.PromQL, query: `up{a="$v"}`, env: envWith(), err: true},
		{name: "no value selected", d: macros.PromQL, query: `up{a="$v"}`, env: envWith(val("v")), err: true},
		{name: "unknown format", d: macros.PromQL, query: `up{a="${v:bogus}"}`, env: envWith(val("v", "x")), err: true},
		{name: "csv with no values", d: macros.PromQL, query: `up{a="${v:csv}"}`, env: envWith(all("v")), err: true},
		{name: "newline in value", d: macros.PromQL, query: `up{a="$v"}`, env: envWith(val("v", "a\nb")), err: true},
		{name: "tab in value", d: macros.PromQL, query: `up{a="$v"}`, env: envWith(val("v", "a\tb")), err: true},
		{name: "nul in value", d: macros.PromQL, query: `up{a="$v"}`, env: envWith(val("v", "a\x00b")), err: true},
		{name: "del in value", d: macros.PromQL, query: `up{a="$v"}`, env: envWith(val("v", "a\x7fb")), err: true},
		{name: "control byte in one of many values", d: macros.PromQL, query: `up{a=~"$v"}`, env: envWith(val("v", "ok", "bad\r")), err: true},
		{name: "unterminated brace reference", d: macros.PromQL, query: `up{a="${v"}`, env: envWith(val("v", "x")), err: true},
		{name: "reference crosses string boundary", d: macros.PromQL, query: `up{a=$__bogus("x)"}`, env: envWith(), err: true},
	})
}

func TestClickHouseVariables(t *testing.T) {
	runExpandCases(t, []expandCase{
		{name: "code default sqlstring single", d: macros.ClickHouse, query: `SELECT 1 WHERE svc IN ($svc)`, env: envWith(val("svc", "it's")), want: `SELECT 1 WHERE svc IN ('it''s')`},
		{name: "code default sqlstring multi", d: macros.ClickHouse, query: `SELECT 1 WHERE svc IN ($svc)`, env: envWith(val("svc", "a", "b")), want: `SELECT 1 WHERE svc IN ('a','b')`},
		{name: "code sqlstring backslash", d: macros.ClickHouse, query: `SELECT 1 WHERE svc IN ($svc)`, env: envWith(val("svc", `b\c`)), want: `SELECT 1 WHERE svc IN ('b\\c')`},
		{name: "code sqlstring all", d: macros.ClickHouse, query: `SELECT 1 WHERE svc IN ($svc)`, env: envWith(all("svc", "a", "b")), want: `SELECT 1 WHERE svc IN ('a','b')`},
		{name: "code sqlstring all with no options", d: macros.ClickHouse, query: `SELECT 1 WHERE svc IN ($svc)`, env: envWith(all("svc")), err: true},
		{name: "code sqlstring unicode", d: macros.ClickHouse, query: `SELECT 1 WHERE svc IN ($svc)`, env: envWith(val("svc", "héllo✓名前")), want: `SELECT 1 WHERE svc IN ('héllo✓名前')`},
		{name: "code singlequote", d: macros.ClickHouse, query: `SELECT 1 WHERE svc IN (${svc:singlequote})`, env: envWith(val("svc", "it's", "b")), want: `SELECT 1 WHERE svc IN ('it\'s','b')`},
		{name: "code doublequote", d: macros.ClickHouse, query: `SELECT ${c:doublequote}`, env: envWith(val("c", `a"b`)), want: `SELECT "a\"b"`},
		{name: "code csv identifiers", d: macros.ClickHouse, query: `SELECT ${cols:csv} FROM t`, env: envWith(val("cols", "a", "b")), want: `SELECT a,b FROM t`},
		{name: "code raw identifier", d: macros.ClickHouse, query: `SELECT 1 ORDER BY ${col:raw}`, env: envWith(val("col", "ts")), want: `SELECT 1 ORDER BY ts`},
		{name: "code raw number", d: macros.ClickHouse, query: `SELECT 1 LIMIT ${n:raw}`, env: envWith(val("n", "10")), want: `SELECT 1 LIMIT 10`},
		{name: "code raw injection", d: macros.ClickHouse, query: `SELECT 1 ORDER BY ${col:raw}`, env: envWith(val("col", "ts DESC; DROP TABLE x")), err: true},
		{name: "code csv with space", d: macros.ClickHouse, query: `SELECT ${cols:csv} FROM t`, env: envWith(val("cols", "a b")), err: true},
		{name: "code csv with comma", d: macros.ClickHouse, query: `SELECT ${cols:csv} FROM t`, env: envWith(val("cols", "a,b")), err: true},
		{name: "code pipe unsafe", d: macros.ClickHouse, query: `SELECT ${cols:pipe} FROM t`, env: envWith(val("cols", "a-1")), err: true},
		{name: "code regex refused", d: macros.ClickHouse, query: `SELECT 1 WHERE x = ${v:regex}`, env: envWith(val("v", "a")), err: true},
		{name: "string default raw escapes quote", d: macros.ClickHouse, query: `SELECT 1 WHERE svc = '$svc'`, env: envWith(val("svc", "x' OR 1=1 --")), want: `SELECT 1 WHERE svc = 'x\' OR 1=1 --'`},
		{name: "string escapes backslash", d: macros.ClickHouse, query: `SELECT 1 WHERE svc = '$svc'`, env: envWith(val("svc", `a\`)), want: `SELECT 1 WHERE svc = 'a\\'`},
		{name: "string backslash quote", d: macros.ClickHouse, query: `SELECT 1 WHERE svc = '$svc'`, env: envWith(val("svc", `\' OR 1=1`)), want: `SELECT 1 WHERE svc = '\\\' OR 1=1'`},
		{name: "string raw with two values", d: macros.ClickHouse, query: `SELECT 1 WHERE svc = '$svc'`, env: envWith(val("svc", "a", "b")), err: true},
		{name: "string csv", d: macros.ClickHouse, query: `SELECT 1 WHERE svc = '${svc:csv}'`, env: envWith(val("svc", "a", "b")), want: `SELECT 1 WHERE svc = 'a,b'`},
		{name: "string explicit sqlstring is escaped again", d: macros.ClickHouse, query: `SELECT 1 WHERE svc = '${svc:sqlstring}'`, env: envWith(val("svc", "a")), want: `SELECT 1 WHERE svc = '\'a\''`},
		{name: "doubled quote keeps string open", d: macros.ClickHouse, query: `SELECT 1 WHERE a = 'it''s $svc'`, env: envWith(val("svc", "x")), want: `SELECT 1 WHERE a = 'it''s x'`},
		{name: "escaped quote keeps string open", d: macros.ClickHouse, query: `SELECT 1 WHERE a = 'a\' $svc'`, env: envWith(val("svc", "x")), want: `SELECT 1 WHERE a = 'a\' x'`},
		{name: "dashes inside string are not a comment", d: macros.ClickHouse, query: `SELECT 1 WHERE a = '--$svc'`, env: envWith(val("svc", "x")), want: `SELECT 1 WHERE a = '--x'`},
		{name: "line comment is verbatim", d: macros.ClickHouse, query: "-- $nope ${x:csv}\nSELECT 1", env: envWith(), want: "-- $nope ${x:csv}\nSELECT 1"},
		{name: "line comment with quote ends at newline", d: macros.ClickHouse, query: "SELECT 1 -- it's\n WHERE b IN ($svc)", env: envWith(val("svc", "x")), want: "SELECT 1 -- it's\n WHERE b IN ('x')"},
		{name: "block comment is verbatim", d: macros.ClickHouse, query: "/* $nope $__bogus */ SELECT 1", env: envWith(), want: "/* $nope $__bogus */ SELECT 1"},
		{name: "after block comment is expanded", d: macros.ClickHouse, query: "/* c */ SELECT 1 WHERE a IN ($v)", env: envWith(val("v", "x")), want: "/* c */ SELECT 1 WHERE a IN ('x')"},
		{name: "double quoted identifier refused", d: macros.ClickHouse, query: `SELECT "$c" FROM t`, env: envWith(val("c", "a")), err: true},
		{name: "backtick identifier refused", d: macros.ClickHouse, query: "SELECT `$c` FROM t", env: envWith(val("c", "a")), err: true},
		{name: "nul byte", d: macros.ClickHouse, query: `SELECT 1 WHERE a IN ($v)`, env: envWith(val("v", "a\x00b")), err: true},
		{name: "newline", d: macros.ClickHouse, query: `SELECT 1 WHERE a = '$v'`, env: envWith(val("v", "a\nb")), err: true},
		{name: "carriage return", d: macros.ClickHouse, query: `SELECT 1 WHERE a IN ($v)`, env: envWith(val("v", "a\rb")), err: true},
		{name: "unknown variable", d: macros.ClickHouse, query: `SELECT 1 WHERE a IN ($v)`, env: envWith(), err: true},
		{name: "no value selected", d: macros.ClickHouse, query: `SELECT 1 WHERE a IN ($v)`, env: envWith(val("v")), err: true},
		{name: "backslash before reference", d: macros.ClickHouse, query: `SELECT 1 WHERE a = '\$v'`, env: envWith(val("v", "x")), err: true},
	})
}

func TestPromQLMacros(t *testing.T) {
	env := func(interval, scrape time.Duration) macros.Env {
		return macros.Env{Range: testRange, Interval: interval, ScrapeInterval: scrape}
	}
	runExpandCases(t, []expandCase{
		{name: "interval", d: macros.PromQL, query: `rate(x[$__interval])`, env: env(30*time.Second, 15*time.Second), want: `rate(x[30s])`},
		{name: "interval braced", d: macros.PromQL, query: `rate(x[${__interval}])`, env: env(30*time.Second, 15*time.Second), want: `rate(x[30s])`},
		{name: "interval in string", d: macros.PromQL, query: `up{a="$__interval"}`, env: env(30*time.Second, 15*time.Second), want: `up{a="30s"}`},
		{name: "interval milliseconds", d: macros.PromQL, query: `rate(x[$__interval])`, env: env(1500*time.Millisecond, 0), want: `rate(x[1500ms])`},
		{name: "interval truncates to milliseconds", d: macros.PromQL, query: `rate(x[$__interval])`, env: env(2500*time.Microsecond, 0), want: `rate(x[2ms])`},
		{name: "interval minimum", d: macros.PromQL, query: `rate(x[$__interval])`, env: env(500*time.Microsecond, 0), want: `rate(x[1ms])`},
		{name: "rate interval floor is four scrapes", d: macros.PromQL, query: `rate(x[$__rate_interval])`, env: env(30*time.Second, 15*time.Second), want: `rate(x[60s])`},
		{name: "rate interval interval plus scrape", d: macros.PromQL, query: `rate(x[$__rate_interval])`, env: env(2*time.Minute, 15*time.Second), want: `rate(x[135s])`},
		{name: "rate interval large scrape", d: macros.PromQL, query: `rate(x[$__rate_interval])`, env: env(30*time.Second, time.Minute), want: `rate(x[240s])`},
		{name: "rate interval default scrape", d: macros.PromQL, query: `rate(x[$__rate_interval])`, env: env(30*time.Second, 0), want: `rate(x[60s])`},
		{name: "range", d: macros.PromQL, query: `increase(x[$__range])`, env: env(30*time.Second, 0), want: `increase(x[21600s])`},
		{name: "macro in comment is verbatim", d: macros.PromQL, query: "up # $__bogus\n", env: env(30*time.Second, 0), want: "up # $__bogus\n"},
		{name: "unknown macro", d: macros.PromQL, query: `rate(x[$__bogus])`, env: env(30*time.Second, 0), err: true},
		{name: "clickhouse macro refused", d: macros.PromQL, query: `x[$__timeFilter(ts)]`, env: env(30*time.Second, 0), err: true},
		{name: "value macro with args", d: macros.PromQL, query: `rate(x[$__interval(1)])`, env: env(30*time.Second, 0), err: true},
		{name: "macro with format", d: macros.PromQL, query: `rate(x[${__interval:csv}])`, env: env(30*time.Second, 0), err: true},
		{name: "zero interval", d: macros.PromQL, query: `up`, env: env(0, 0), err: true},
		{name: "negative interval", d: macros.PromQL, query: `up`, env: env(-time.Second, 0), err: true},
		{name: "zero time range", d: macros.PromQL, query: `up`, env: macros.Env{Interval: time.Second}, err: true},
		{name: "reversed time range", d: macros.PromQL, query: `up`, env: macros.Env{Interval: time.Second, Range: datasource.TimeRange{From: testRange.To, To: testRange.From}}, err: true},
	})
}

func TestClickHouseMacros(t *testing.T) {
	env := func(interval time.Duration) macros.Env {
		return macros.Env{Range: testRange, Interval: interval}
	}
	from, to := testRange.From.UnixMilli(), testRange.To.UnixMilli()
	filter := func(col string) string {
		return fmt.Sprintf("(%[1]s >= fromUnixTimestamp64Milli(toInt64(%[2]d), 'UTC') AND %[1]s <= fromUnixTimestamp64Milli(toInt64(%[3]d), 'UTC'))", col, from, to)
	}
	runExpandCases(t, []expandCase{
		{name: "interval seconds", d: macros.ClickHouse, query: `SELECT $__interval`, env: env(30 * time.Second), want: `SELECT 30`},
		{name: "interval rounds up", d: macros.ClickHouse, query: `SELECT $__interval`, env: env(1500 * time.Millisecond), want: `SELECT 2`},
		{name: "interval minimum one", d: macros.ClickHouse, query: `SELECT $__interval`, env: env(100 * time.Millisecond), want: `SELECT 1`},
		{name: "interval in string", d: macros.ClickHouse, query: `SELECT '$__interval'`, env: env(30 * time.Second), want: `SELECT '30'`},
		{name: "time filter", d: macros.ClickHouse, query: `SELECT 1 FROM t WHERE $__timeFilter(Timestamp)`, env: env(time.Minute), want: `SELECT 1 FROM t WHERE ` + filter("Timestamp")},
		{name: "time filter with expression", d: macros.ClickHouse, query: `SELECT 1 FROM t WHERE $__timeFilter(toDateTime(ts))`, env: env(time.Minute), want: `SELECT 1 FROM t WHERE ` + filter("toDateTime(ts)")},
		{name: "time filter trims args", d: macros.ClickHouse, query: `SELECT 1 FROM t WHERE $__timeFilter( ts )`, env: env(time.Minute), want: `SELECT 1 FROM t WHERE ` + filter("ts")},
		{name: "time group", d: macros.ClickHouse, query: `SELECT $__timeGroup(ts) FROM t GROUP BY 1`, env: env(30 * time.Second), want: `SELECT toStartOfInterval(ts, INTERVAL 30 SECOND) FROM t GROUP BY 1`},
		{name: "time group rounds interval up", d: macros.ClickHouse, query: `SELECT $__timeGroup(ts)`, env: env(90500 * time.Millisecond), want: `SELECT toStartOfInterval(ts, INTERVAL 91 SECOND)`},
		{name: "macro in comment is verbatim", d: macros.ClickHouse, query: "-- $__timeFilter()\nSELECT 1", env: env(time.Minute), want: "-- $__timeFilter()\nSELECT 1"},
		{name: "prometheus macro refused", d: macros.ClickHouse, query: `SELECT $__range`, env: env(time.Minute), err: true},
		{name: "rate interval refused", d: macros.ClickHouse, query: `SELECT $__rate_interval`, env: env(time.Minute), err: true},
		{name: "unknown macro", d: macros.ClickHouse, query: `SELECT $__bogus`, env: env(time.Minute), err: true},
		{name: "function macro without args", d: macros.ClickHouse, query: `SELECT 1 WHERE $__timeFilter`, env: env(time.Minute), err: true},
		{name: "function macro braced without args", d: macros.ClickHouse, query: `SELECT 1 WHERE ${__timeFilter}`, env: env(time.Minute), err: true},
		{name: "function macro empty args", d: macros.ClickHouse, query: `SELECT 1 WHERE $__timeFilter()`, env: env(time.Minute), err: true},
		{name: "function macro blank args", d: macros.ClickHouse, query: `SELECT 1 WHERE $__timeFilter(  )`, env: env(time.Minute), err: true},
		{name: "function macro semicolon in args", d: macros.ClickHouse, query: `SELECT 1 WHERE $__timeFilter(ts; DROP TABLE x)`, env: env(time.Minute), err: true},
		{name: "function macro dollar in args", d: macros.ClickHouse, query: `SELECT 1 WHERE $__timeFilter($col)`, env: env(time.Minute), err: true},
		{name: "function macro in string", d: macros.ClickHouse, query: `SELECT '$__timeFilter(ts)'`, env: env(time.Minute), err: true},
		{name: "function macro in quoted identifier", d: macros.ClickHouse, query: `SELECT "$__timeGroup(ts)"`, env: env(time.Minute), err: true},
		{name: "value macro in quoted identifier", d: macros.ClickHouse, query: `SELECT "$__interval"`, env: env(time.Minute), err: true},
		{name: "value macro with args", d: macros.ClickHouse, query: `SELECT $__interval(1)`, env: env(time.Minute), err: true},
		{name: "macro with format", d: macros.ClickHouse, query: `SELECT ${__interval:raw}`, env: env(time.Minute), err: true},
		{name: "unbalanced macro args", d: macros.ClickHouse, query: `SELECT $__timeFilter(ts`, env: env(time.Minute), err: true},
		{name: "macro args break out of string", d: macros.ClickHouse, query: `SELECT $__timeFilter(a') FROM t WHERE x = 'y'`, env: env(time.Minute), err: true},
	})
}

func TestMacrosAndVariablesTogether(t *testing.T) {
	from, to := testRange.From.UnixMilli(), testRange.To.UnixMilli()
	e := envWith(val("svc", "it's"))
	got, err := macros.Expand(macros.ClickHouse, `SELECT $__timeGroup(ts), count() FROM t WHERE $__timeFilter(ts) AND svc IN ($svc) GROUP BY 1`, e)
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("SELECT toStartOfInterval(ts, INTERVAL 30 SECOND), count() FROM t WHERE (ts >= fromUnixTimestamp64Milli(toInt64(%[1]d), 'UTC') AND ts <= fromUnixTimestamp64Milli(toInt64(%[2]d), 'UTC')) AND svc IN ('it''s') GROUP BY 1", from, to)
	if got.Text != want {
		t.Fatalf("got %s\nwant %s", got.Text, want)
	}
}

func TestErrorsNeverContainValues(t *testing.T) {
	const secret = "s3cr3t-token"
	cases := []expandCase{
		{d: macros.PromQL, query: `up{a="$v"}`, env: envWith(val("v", secret, "other"))},
		{d: macros.PromQL, query: `up{a="$v"}`, env: envWith(val("v", secret+"\n"))},
		{d: macros.PromQL, query: `sum by ($v) (up)`, env: envWith(val("v", secret))},
		{d: macros.PromQL, query: `sum by (${v:regex}) (up)`, env: envWith(val("v", secret))},
		{d: macros.PromQL, query: "up{a=~`$v`}", env: envWith(val("v", secret+"`"))},
		{d: macros.ClickHouse, query: `SELECT ${v:raw}`, env: envWith(val("v", secret+" x"))},
		{d: macros.ClickHouse, query: `SELECT "$v"`, env: envWith(val("v", secret))},
		{d: macros.ClickHouse, query: `SELECT '$v'`, env: envWith(all("v", secret, "b"))},
		{d: macros.ClickHouse, query: `SELECT ${v:bogus}`, env: envWith(val("v", secret))},
	}
	for i, tc := range cases {
		_, err := macros.Expand(tc.d, tc.query, tc.env)
		if err == nil {
			t.Fatalf("case %d: expected error", i)
		}
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("case %d: error leaks a value: %v", i, err)
		}
		if !strings.Contains(err.Error(), "offset") {
			t.Fatalf("case %d: error does not name an offset: %v", i, err)
		}
	}
}

var injectionCorpus = []string{
	`"} or vector(1) #`,
	`"}) or vector(1) #`,
	`'; DROP TABLE x; --`,
	`\' OR 1=1`,
	`' OR 1=1 --`,
	`x') UNION SELECT 1 --`,
	`\`,
	`\\`,
	`x\`,
	`"`,
	`'`,
	`''`,
	`'''`,
	`\"`,
	`\'`,
	`a\'b`,
	`}`,
	`)`,
	`*/ DROP TABLE x /*`,
	`/*`,
	`#`,
	`--`,
	`.*`,
	`a|b`,
	`(?i)x`,
	`$__interval`,
	`${v}`,
	`$(x)`,
	"`",
	"back`tick",
	"héllo ✓ 名前\"",
	`😀'`,
	``,
}

type literalCase struct {
	name   string
	d      macros.Dialect
	prefix string
	suffix string
	list   bool
}

func scanLiteral(d macros.Dialect, s string, open int) int {
	quote := s[open]
	for j := open + 1; j < len(s); {
		switch {
		case s[j] == '\\' && quote != '`':
			j += 2
		case s[j] == quote && d == macros.ClickHouse && j+1 < len(s) && s[j+1] == quote:
			j += 2
		case s[j] == quote:
			return j + 1
		default:
			j++
		}
	}
	return len(s)
}

func literalEnd(d macros.Dialect, s string, from int, list bool) int {
	pos := from
	for {
		if pos >= len(s) {
			return len(s)
		}
		pos = scanLiteral(d, s, pos)
		if list && pos < len(s) && s[pos] == ',' && pos+1 < len(s) {
			pos++
			continue
		}
		return pos
	}
}

func TestInjectionCorpusStaysInsideLiteral(t *testing.T) {
	bt := "`"
	cases := []literalCase{
		{name: "promql equality double", d: macros.PromQL, prefix: `up{pod="`, suffix: `"}`},
		{name: "promql equality single", d: macros.PromQL, prefix: `up{pod='`, suffix: `'}`},
		{name: "promql regex double", d: macros.PromQL, prefix: `up{pod=~"`, suffix: `"}`},
		{name: "promql regex single", d: macros.PromQL, prefix: `up{pod=~'`, suffix: `'}`},
		{name: "promql negative regex", d: macros.PromQL, prefix: `up{pod!~"`, suffix: `"} # tail`},
		{name: "promql backtick", d: macros.PromQL, prefix: "up{pod=~" + bt, suffix: bt + "}"},
		{name: "promql doublequote list", d: macros.PromQL, prefix: `up{pod=~`, suffix: `}`, list: true},
		{name: "clickhouse string", d: macros.ClickHouse, prefix: `SELECT 1 WHERE svc = '`, suffix: `' AND x = 1`},
		{name: "clickhouse in list", d: macros.ClickHouse, prefix: `SELECT 1 WHERE svc IN (`, suffix: `)`, list: true},
	}
	formats := map[string]string{
		"promql doublequote list": "doublequote",
		"clickhouse in list":      "sqlstring",
	}
	for _, tc := range cases {
		for _, single := range injectionCorpus {
			for _, values := range [][]string{{single}, {single, "ok"}, {"ok", single}} {
				multi := len(values) > 1
				ref := "$v"
				if f := formats[tc.name]; f != "" {
					ref = "${v:" + f + "}"
				}
				query := tc.prefix + ref + tc.suffix
				if multi && (tc.name == "promql equality double" || tc.name == "promql equality single" || tc.name == "clickhouse string") {
					continue
				}
				got, err := macros.Expand(tc.d, query, envWith(val("v", values...)))
				if err != nil {
					if !errors.Is(err, datasource.ErrInvalidQuery) {
						t.Fatalf("%s %q: error is not ErrInvalidQuery: %v", tc.name, values, err)
					}
					if tc.name != "promql backtick" || !strings.Contains(strings.Join(values, ""), bt) {
						t.Fatalf("%s %q: unexpected error: %v", tc.name, values, err)
					}
					continue
				}
				if tc.name == "promql backtick" && strings.Contains(strings.Join(values, ""), bt) {
					t.Fatalf("%s %q: backtick value was accepted: %s", tc.name, values, got.Text)
				}
				out := got.Text
				if !strings.HasPrefix(out, tc.prefix) || !strings.HasSuffix(out, tc.suffix) {
					t.Fatalf("%s %q: template text changed: %s", tc.name, values, out)
				}
				start, wantEnd := len(tc.prefix)-1, len(out)-len(tc.suffix)+1
				if tc.list {
					start, wantEnd = len(tc.prefix), len(out)-len(tc.suffix)
				}
				if end := literalEnd(tc.d, out, start, tc.list); end != wantEnd {
					t.Fatalf("%s %q: value escapes its literal (literal ends at %d, want %d): %s", tc.name, values, end, wantEnd, out)
				}
			}
		}
	}
}

func TestInjectionCorpusRegexMatchesExactlyTheValues(t *testing.T) {
	for _, first := range injectionCorpus {
		for _, values := range [][]string{{first}, {first, "second.value"}} {
			if first == "" {
				continue
			}
			got, err := macros.Expand(macros.PromQL, `up{pod=~"$v"}`, envWith(val("v", values...)))
			if err != nil {
				t.Fatalf("%q: %v", values, err)
			}
			literal := strings.TrimSuffix(strings.TrimPrefix(got.Text, `up{pod=~`), `}`)
			pattern, err := strconv.Unquote(literal)
			if err != nil {
				t.Fatalf("%q: output %s is not a valid string literal: %v", values, literal, err)
			}
			re, err := regexp.Compile("^(?:" + pattern + ")$")
			if err != nil {
				t.Fatalf("%q: pattern %q does not compile: %v", values, pattern, err)
			}
			for _, v := range values {
				if !re.MatchString(v) {
					t.Fatalf("%q: pattern %q does not match %q", values, pattern, v)
				}
				for _, other := range []string{v + "x", "x" + v, ""} {
					matchesAValue := false
					for _, w := range values {
						matchesAValue = matchesAValue || other == w
					}
					if re.MatchString(other) && !matchesAValue {
						t.Fatalf("%q: pattern %q also matches %q", values, pattern, other)
					}
				}
			}
		}
	}
}

func TestValuesAreNeverRescanned(t *testing.T) {
	for _, d := range []macros.Dialect{macros.PromQL, macros.ClickHouse} {
		query := map[macros.Dialect]string{macros.PromQL: `up{a="$v"}`, macros.ClickHouse: `SELECT '$v'`}[d]
		got, err := macros.Expand(d, query, envWith(val("v", "$__interval ${w} $w"), val("w", "boom")))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(got.Text, "$__interval ${w} $w") || strings.Contains(got.Text, "boom") {
			t.Fatalf("value was rescanned: %s", got.Text)
		}
	}
}

func TestLocate(t *testing.T) {
	query := `up{pod=~"$pod", job="$job"} / $__interval`
	got, err := macros.Expand(macros.PromQL, query, envWith(val("pod", "a.b", "c"), val("job", "x")))
	if err != nil {
		t.Fatal(err)
	}
	podText := `(a\\.b|c)`
	if !strings.Contains(got.Text, podText) {
		t.Fatalf("unexpected expansion: %s", got.Text)
	}
	podStart, podEnd := strings.Index(query, "$pod"), strings.Index(query, "$pod")+len("$pod")
	jobStart, jobEnd := strings.Index(query, "$job"), strings.Index(query, "$job")+len("$job")
	macroStart, macroEnd := strings.Index(query, "$__interval"), len(query)
	expPod := strings.Index(got.Text, podText)
	expJobEq := strings.Index(got.Text, `job=`)
	expMacro := strings.Index(got.Text, "30s")

	cases := []struct {
		name                 string
		start, end           int
		wantStart, wantEnd   int
		wantInVariable       bool
		skipInVariableAssert bool
	}{
		{name: "whole variable expansion", start: expPod, end: expPod + len(podText), wantStart: podStart, wantEnd: podEnd, wantInVariable: true},
		{name: "inside variable expansion", start: expPod + 2, end: expPod + 4, wantStart: podStart, wantEnd: podEnd, wantInVariable: true},
		{name: "text before any expansion", start: 0, end: 2, wantStart: 0, wantEnd: 2},
		{name: "text between expansions", start: expJobEq, end: expJobEq + 3, wantStart: strings.Index(query, "job="), wantEnd: strings.Index(query, "job=") + 3},
		{name: "from plain text into variable", start: 0, end: expPod + 3, wantStart: 0, wantEnd: podEnd, wantInVariable: true},
		{name: "across two variables", start: expPod + 1, end: expJobEq + 5, wantStart: podStart, wantEnd: strings.Index(query, "job=") + 5, wantInVariable: true},
		{name: "second variable", start: strings.Index(got.Text, `"x"`) + 1, end: strings.Index(got.Text, `"x"`) + 2, wantStart: jobStart, wantEnd: jobEnd, wantInVariable: true},
		{name: "macro is not a variable", start: expMacro, end: expMacro + 3, wantStart: macroStart, wantEnd: macroEnd},
		{name: "empty range at plain text", start: 2, end: 2, wantStart: 2, wantEnd: 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, e, inVar := got.Locate(tc.start, tc.end)
			if s != tc.wantStart || e != tc.wantEnd || inVar != tc.wantInVariable {
				t.Fatalf("Locate(%d, %d) = (%d, %d, %v), want (%d, %d, %v)", tc.start, tc.end, s, e, inVar, tc.wantStart, tc.wantEnd, tc.wantInVariable)
			}
		})
	}

	t.Run("tail after last expansion", func(t *testing.T) {
		plain, err := macros.Expand(macros.PromQL, `up{a="$v"} + 1`, envWith(val("v", "longer-than-ref")))
		if err != nil {
			t.Fatal(err)
		}
		idx := strings.Index(plain.Text, "+ 1")
		s, e, inVar := plain.Locate(idx, idx+3)
		if s != strings.Index(`up{a="$v"} + 1`, "+ 1") || e != len(`up{a="$v"} + 1`) || inVar {
			t.Fatalf("Locate = (%d, %d, %v)", s, e, inVar)
		}
	})
}

func TestExpandForLint(t *testing.T) {
	t.Run("promql", func(t *testing.T) {
		got, err := macros.ExpandForLint(macros.PromQL, `sum by (${l:csv}) (rate(x{pod=~"$pod", job="$job"}[$__rate_interval])) / $__range`)
		if err != nil {
			t.Fatal(err)
		}
		want := `sum by (x) (rate(x{pod=~"x", job="x"}[75s])) / 3600s`
		if got.Text != want {
			t.Fatalf("got %s\nwant %s", got.Text, want)
		}
		start := strings.Index(got.Text, `job="x"`) + len(`job="`)
		s, e, inVar := got.Locate(start, start+1)
		if !inVar || s != strings.Index(`sum by (${l:csv}) (rate(x{pod=~"$pod", job="$job"}[$__rate_interval])) / $__range`, "$job") || e-s != len("$job") {
			t.Fatalf("Locate = (%d, %d, %v)", s, e, inVar)
		}
	})
	t.Run("clickhouse", func(t *testing.T) {
		got, err := macros.ExpandForLint(macros.ClickHouse, `SELECT $__timeGroup(ts), count() FROM t WHERE $__timeFilter(ts) AND svc IN ($svc) AND n = '$n'`)
		if err != nil {
			t.Fatal(err)
		}
		for _, frag := range []string{"toStartOfInterval(ts, INTERVAL 60 SECOND)", "ts >= fromUnixTimestamp64Milli", "svc IN ('x')", "n = 'x'"} {
			if !strings.Contains(got.Text, frag) {
				t.Fatalf("missing %q in %s", frag, got.Text)
			}
		}
	})
	t.Run("repeated variable", func(t *testing.T) {
		got, err := macros.ExpandForLint(macros.PromQL, `up{a="$v", b="$v"}`)
		if err != nil || got.Text != `up{a="x", b="x"}` {
			t.Fatalf("got %q, %v", got.Text, err)
		}
	})
	t.Run("variable in comment needs no value", func(t *testing.T) {
		got, err := macros.ExpandForLint(macros.PromQL, "up # $v\n")
		if err != nil || got.Text != "up # $v\n" {
			t.Fatalf("got %q, %v", got.Text, err)
		}
	})
	for _, tc := range []struct {
		name  string
		d     macros.Dialect
		query string
	}{
		{"unknown macro", macros.PromQL, `rate(x[$__bogus])`},
		{"scan error", macros.PromQL, `up{a="${v"}`},
		{"unterminated macro args", macros.ClickHouse, `SELECT $__timeFilter(ts`},
		{"hostile template", macros.PromQL, `sum by ($v) (up{a=${v:regex}})`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := macros.ExpandForLint(tc.d, tc.query)
			if !errors.Is(err, datasource.ErrInvalidQuery) {
				t.Fatalf("err = %v, want ErrInvalidQuery", err)
			}
		})
	}
}

func TestUnknownDialect(t *testing.T) {
	_, err := macros.Expand(0, "up", envWith())
	if !errors.Is(err, datasource.ErrInvalidQuery) {
		t.Fatalf("err = %v", err)
	}
}
