package spec_test

import (
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestScanQuery(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  []spec.Reference
	}{
		{"bare variable", `up{pod="$pod"}`, []spec.Reference{
			{Kind: spec.VariableRef, Name: "pod", Start: 8, End: 12},
		}},
		{"braced variable", `up{pod="${pod}"}`, []spec.Reference{
			{Kind: spec.VariableRef, Name: "pod", Start: 8, End: 14},
		}},
		{"braced variable with format", `x IN (${pod:regex})`, []spec.Reference{
			{Kind: spec.VariableRef, Name: "pod", Format: "regex", Start: 6, End: 18},
		}},
		{"rate interval", `rate(x[$__rate_interval])`, []spec.Reference{
			{Kind: spec.MacroRef, Name: "__rate_interval", Start: 7, End: 23},
		}},
		{"function macro", `WHERE $__timeFilter(Timestamp)`, []spec.Reference{
			{Kind: spec.MacroRef, Name: "__timeFilter", Args: "Timestamp", HasArgs: true, Start: 6, End: 30},
		}},
		{"nested parens", `$__timeFilter(toDateTime(ts))`, []spec.Reference{
			{Kind: spec.MacroRef, Name: "__timeFilter", Args: "toDateTime(ts)", HasArgs: true, Start: 0, End: 29},
		}},
		{"macro without args", `$__timeFilter`, []spec.Reference{
			{Kind: spec.MacroRef, Name: "__timeFilter", Start: 0, End: 13},
		}},
		{"regex capture group", `label_replace(x, "a", "${1}", "b", "(.*)")`, nil},
		{"multi-digit capture group", `replaceRegexpOne(x, 'a', '${12}')`, nil},
		{"capture group beside variable", `replaceRegexpOne(x, 'a', '${1}$pod')`, []spec.Reference{
			{Kind: spec.VariableRef, Name: "pod", Start: 30, End: 34},
		}},
		{"paren inside single quotes", `$__timeFilter(concat(ts, '('))`, []spec.Reference{
			{Kind: spec.MacroRef, Name: "__timeFilter", Args: "concat(ts, '(')", HasArgs: true, Start: 0, End: 30},
		}},
		{"paren inside escaped double quotes", `$__timeFilter(f(ts, "\")("))`, []spec.Reference{
			{Kind: spec.MacroRef, Name: "__timeFilter", Args: `f(ts, "\")(")`, HasArgs: true, Start: 0, End: 28},
		}},
		{"unknown macro with unclosed args", `$__foo(x`, []spec.Reference{
			{Kind: spec.MacroRef, Name: "__foo", Start: 0, End: 6},
		}},
		{"paren inside backticks", "$__timeFilter(concat(ts, `(`))", []spec.Reference{
			{Kind: spec.MacroRef, Name: "__timeFilter", Args: "concat(ts, `(`)", HasArgs: true, Start: 0, End: 30},
		}},
		{"backslash ends a backtick string", "$__timeFilter(f(`x\\`)) $v", []spec.Reference{
			{Kind: spec.MacroRef, Name: "__timeFilter", Args: "f(`x\\`)", HasArgs: true, Start: 0, End: 22},
			{Kind: spec.VariableRef, Name: "v", Start: 23, End: 25},
		}},
		{"apostrophe in line comment", "$__timeFilter(ts -- don't\n)", []spec.Reference{
			{Kind: spec.MacroRef, Name: "__timeFilter", Args: "ts -- don't\n", HasArgs: true, Start: 0, End: 27},
		}},
		{"apostrophe in hash comment", "$__timeFilter(ts # don't\n)", []spec.Reference{
			{Kind: spec.MacroRef, Name: "__timeFilter", Args: "ts # don't\n", HasArgs: true, Start: 0, End: 26},
		}},
		{"apostrophe and paren in block comment", "$__timeFilter(ts /* don't ) */)", []spec.Reference{
			{Kind: spec.MacroRef, Name: "__timeFilter", Args: "ts /* don't ) */", HasArgs: true, Start: 0, End: 31},
		}},
		{"apostrophe in comment after the macro", "$__timeFilter(ts) -- don't", []spec.Reference{
			{Kind: spec.MacroRef, Name: "__timeFilter", Args: "ts", HasArgs: true, Start: 0, End: 17},
		}},
		{"double negation in plain PromQL", `a --$v`, []spec.Reference{
			{Kind: spec.VariableRef, Name: "v", Start: 4, End: 6},
		}},
		{"apostrophe in slash comment", "$__timeFilter(ts // don't\n)", []spec.Reference{
			{Kind: spec.MacroRef, Name: "__timeFilter", Args: "ts // don't\n", HasArgs: true, Start: 0, End: 27},
		}},
		{"apostrophe in bang comment", "$__timeFilter(ts #! don't\n)", []spec.Reference{
			{Kind: spec.MacroRef, Name: "__timeFilter", Args: "ts #! don't\n", HasArgs: true, Start: 0, End: 27},
		}},
		{"nested block comment", "$__timeFilter(ts /* a /* b */ ) */)", []spec.Reference{
			{Kind: spec.MacroRef, Name: "__timeFilter", Args: "ts /* a /* b */ ) */", HasArgs: true, Start: 0, End: 35},
		}},
		{"hash without space is not a comment", "$__timeFilter(a #x)", []spec.Reference{
			{Kind: spec.MacroRef, Name: "__timeFilter", Args: "a #x", HasArgs: true, Start: 0, End: 19},
		}},
		{"label_replace group", `label_replace(x, "a", "$1", "b", "(.*)")`, nil},
		{"trailing dollar", `SELECT 1 $`, nil},
		{"regex anchor", `up{pod=~"x$"}`, nil},
		{"user login", `${__user.login}`, []spec.Reference{
			{Kind: spec.MacroRef, Name: "__user.login", Start: 0, End: 15},
		}},
		{"user email with format", `x = ${__user.email:sqlstring}`, []spec.Reference{
			{Kind: spec.MacroRef, Name: "__user.email", Format: "sqlstring", Start: 4, End: 29},
		}},
		{"from with a date pattern", `${__from:date:YYYY-MM-DDTHH:mm:ss}`, []spec.Reference{
			{Kind: spec.MacroRef, Name: "__from", Format: "date:YYYY-MM-DDTHH:mm:ss", Start: 0, End: 34},
		}},
		{"from with seconds", `${__from:date:seconds}`, []spec.Reference{
			{Kind: spec.MacroRef, Name: "__from", Format: "date:seconds", Start: 0, End: 22},
		}},
		{"interval in milliseconds is one name", `$__interval_ms`, []spec.Reference{
			{Kind: spec.MacroRef, Name: "__interval_ms", Start: 0, End: 14},
		}},
		{"unbraced user is only the prefix", `$__user.login`, []spec.Reference{
			{Kind: spec.MacroRef, Name: "__user", Start: 0, End: 7},
		}},
		{"two references", `a $x b ${y:csv}`, []spec.Reference{
			{Kind: spec.VariableRef, Name: "x", Start: 2, End: 4},
			{Kind: spec.VariableRef, Name: "y", Format: "csv", Start: 7, End: 15},
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := spec.ScanQuery(c.query)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got %+v, want %+v", got, c.want)
			}
		})
	}
}

func TestScanQueryErrors(t *testing.T) {
	cases := []struct {
		name    string
		query   string
		message string
	}{
		{"unterminated brace", `up{pod="${pod`, `unterminated "${" at offset 8`},
		{"empty format", `${pod:}`, `invalid reference "${pod:}" at offset 0`},
		{"digit-led name", "${1pod}", `invalid reference "${1pod}" at offset 0`},
		{"digit with format", "${2:csv}", `invalid reference "${2:csv}" at offset 0`},
		{"unknown date token", `${__from:date:Q}`, `invalid reference "${__from:date:Q}" at offset 0`},
		{"time macro with a variable format", `${__to:csv}`, `invalid reference "${__to:csv}" at offset 0`},
		{"empty date pattern", `${__from:date:}`, `invalid reference "${__from:date:}" at offset 0`},
		{"unknown user field", `${__user.bogus}`, `invalid reference "${__user.bogus}" at offset 0`},
		{"dashboard uid", `${__dashboard.uid}`, `invalid reference "${__dashboard.uid}" at offset 0`},
		{"dotted variable name", `${pod.name}`, `invalid reference "${pod.name}" at offset 0`},
		{"date pattern over 64 bytes", "${__from:date:" + strings.Repeat("Y", 65) + "}", `invalid reference`},
		{"unclosed paren", `$__timeFilter(ts`, `unclosed "(" at offset 13`},
		{"paren only inside backticks", "$__timeFilter(concat(ts, `)`)", `unclosed "(" at offset 13`},
		{"backslash and paren inside backticks", "$__timeFilter(f(`\\)`)", `unclosed "(" at offset 13`},
		{"paren only inside line comment", "$__timeFilter(ts -- )\n", `unclosed "(" at offset 13`},
		{"paren only inside block comment", "$__timeFilter(ts /* ) */", `unclosed "(" at offset 13`},
		{"paren only inside nested block comment", "$__timeFilter(ts /* /* */ ) */", `unclosed "(" at offset 13`},
		{"paren only inside quotes", `$__timeFilter(concat(ts, ')')`, `unclosed "(" at offset 13`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := spec.ScanQuery(c.query)
			if err == nil || !strings.Contains(err.Error(), c.message) {
				t.Fatalf("got error %v, want it to contain %q", err, c.message)
			}
		})
	}
}

func TestMacrosAndFormats(t *testing.T) {
	names := func(ms []spec.Macro) []string {
		var out []string
		for _, m := range ms {
			out = append(out, m.Name)
		}
		return out
	}
	globals := []string{"__interval_ms", "__range_s", "__range_ms", "__from", "__to", "__dashboard", "__user.login", "__user.email", "__user.name", "__user.id"}
	wantPrometheus := []string{"__interval", "__interval_ms", "__range", "__range_s", "__range_ms", "__rate_interval", "__from", "__to", "__dashboard", "__user.login", "__user.email", "__user.name", "__user.id"}
	if got, want := names(spec.Macros(spec.Prometheus)), wantPrometheus; !slices.Equal(got, want) {
		t.Errorf("prometheus macros = %v, want %v", got, want)
	}
	wantClickHouse := append(append([]string{"__interval"}, globals...), "__timeFilter", "__timeGroup")
	if got, want := names(spec.Macros(spec.ClickHouse)), wantClickHouse; !slices.Equal(got, want) {
		t.Errorf("clickhouse macros = %v, want %v", got, want)
	}
	if formats := spec.VariableFormats(); !slices.IsSorted(formats) || len(formats) != 7 {
		t.Errorf("VariableFormats() = %v", formats)
	}
}

func TestScanQueryPathologicalInput(t *testing.T) {
	const (
		small = 50_000
		large = 4 * small
		runs  = 3
	)
	scan := func(unit string, size int) (time.Duration, int, error) {
		query := strings.Repeat(unit, size/len(unit))
		var err error
		best := time.Duration(math.MaxInt64)
		for range runs {
			start := time.Now()
			_, err = spec.ScanQuery(query)
			best = min(best, time.Since(start))
		}
		return best, len(query), err
	}
	for _, unit := range []string{"$__x(", "$__x( /* ", "$__x( '", "$__x( `\\"} {
		smallTime, smallLen, err := scan(unit, small)
		if err == nil {
			t.Errorf("%q at %d bytes: want an error for unbounded unclosed parentheses", unit, smallLen)
		}
		largeTime, largeLen, err := scan(unit, large)
		if err == nil {
			t.Errorf("%q at %d bytes: want an error for unbounded unclosed parentheses", unit, largeLen)
		}
		if ratio := float64(largeTime) / float64(max(smallTime, time.Microsecond)); ratio > 6 {
			t.Errorf("%q: scanning %d bytes took %s and %d bytes took %s (%.1fx for 4x input, want linear or better)", unit, smallLen, smallTime, largeLen, largeTime, ratio)
		}
	}
}

func TestScanQueryManyClosedMacros(t *testing.T) {
	query := strings.Repeat("$__timeFilter(ts) ", 1000)
	refs, err := spec.ScanQuery(query)
	if err != nil || len(refs) != 1000 {
		t.Fatalf("got %d refs, err %v", len(refs), err)
	}
}
