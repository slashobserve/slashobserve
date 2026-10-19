package macros

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

var hostileValues = [][]string{
	{"a'b"}, {"\\"}, {"x--"}, {"' OR 1=1 -- "}, {"$t$"}, {"*/"}, {"‘"}, {"’ OR 1 --"}, {"a", "b'"},
	{"1"}, {"e5"}, {"\\'"}, {"''"}, {"a\\"}, {"ab"}, {"/*"}, {"−−"}, {"f"}, {"0b"}, {"1e"}, {"\"\""}, {"`"}, {"# "}, {"é"},
}

func fuzzEnv(values ...string) Env {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return Env{
		Range:          datasource.TimeRange{From: from, To: from.Add(time.Hour)},
		Interval:       30 * time.Second,
		ScrapeInterval: 15 * time.Second,
		Variables:      []datasource.Variable{{Name: "v", Values: values}, {Name: "w", Values: values}},
		Dashboard:      strings.Join(values, ","),
		User:           datasource.QueryUser{ID: "u", Login: strings.Join(values, ","), Email: strings.Join(values, ","), Name: strings.Join(values, ",")},
	}
}

func templateKinds(d Dialect, query string, e Expansion) ([]int, bool) {
	units, err := expandedUnits(d, e.Text)
	if err != nil {
		return nil, false
	}
	kinds := make([]int, len(e.Text))
	for _, u := range units {
		k := 0
		if u.container {
			k = u.kind<<8 | int(u.quote)
		}
		for i := u.start; i < u.end; i++ {
			kinds[i] = k
		}
	}
	out := make([]int, 0, len(query))
	pos := 0
	for _, s := range e.spans {
		out = append(out, kinds[pos:s.expStart]...)
		pos = s.expEnd
	}
	return append(out, kinds[pos:]...), true
}

func checkNoDesync(t *testing.T, d Dialect, template string, hostile []string) {
	t.Helper()
	base, err := Expand(d, template, fuzzEnv("a"))
	if err != nil {
		return
	}
	baseKinds, ok := templateKinds(d, template, base)
	if !ok {
		t.Fatalf("expanded text of %q does not tokenize: %q", template, base.Text)
	}
	for _, values := range append([][]string{hostile}, hostileValues...) {
		e, err := Expand(d, template, fuzzEnv(values...))
		if err != nil {
			continue
		}
		kinds, ok := templateKinds(d, template, e)
		if !ok {
			t.Fatalf("expanded text of %q with %q does not tokenize: %q", template, values, e.Text)
		}
		if !slices.Equal(baseKinds, kinds) {
			t.Fatalf("template bytes change tokenization\ntemplate: %q\nvalues:   %q\nbenign:   %q\nhostile:  %q", template, values, base.Text, e.Text)
		}
	}
}

var clickHouseFuzzSeeds = []string{
	"SELECT ${v:raw}e-- x'\n, '$w'",
	"SELECT ${v:raw}0b$$ x' $$, '$w'",
	"SELECT ${v:raw}1e-- x'\n, '$w'",
	"SELECT $v'abc', $w",
	"SELECT x$v, $w",
	"SELECT a${v:raw} FROM t WHERE x = '$w'",
	"SELECT 1 FROM t # don't\nWHERE id = $v",
	"SELECT 1 FROM t /* /* */ it's */ WHERE id = $v",
	"SELECT $$ it's $$ AS a, id FROM t WHERE id = $v",
	"SELECT 1 #! x'\n, $v",
	"SELECT 1 // c\r'\n, $v",
	"SELECT $a$b, '$w'",
	"SELECT ${v:csv}, '$w' -- $v",
	"SELECT $__timeFilter(ts) AND a = '$v' AND b IN ($w)",
	"SELECT ‘it's’, $v",
	"SELECT `a``b`, \"$w\", $v",
	"SELECT ${v}${w}",
	"SELECT $v1e",
	"SELECT $__dashboard, '${__user.login}', ${__user.email:raw}e-- x'\n, '$w'",
	"SELECT ${__dashboard:raw}0b$$ x' $$, '${__user.name}'",
	"SELECT $__interval_ms, $__range_s, $__range_ms, ${__from:date:seconds}, $__to, '${__from:date:YYYY-MM-DD}'",
	"SELECT ${__user.id:csv} FROM t WHERE a = '$__dashboard' AND b IN ($w)",
	"SELECT 1 /* ${__user.login} */ WHERE x = ${__dashboard}",
	"SELECT 1 WHERE $__timeFilter_ms(ts) AND $__dateFilter(d) AND $__dt(d, ts) AND x = '$v'",
	"SELECT $__fromTime, $__toTime, $__fromTime_ms, $__toTime_ms, $__interval_s, '$w'",
	"SELECT $__timeInterval(ts), $__timeInterval_ms(ts), $w FROM t",
	"SELECT 1 WHERE $__conditionalAll(a IN (${v:singlequote}) AND b = '$w', $v) AND c = '$w'",
	"SELECT 1 WHERE $__conditionalAll(a = ${v:raw}e-- x'\n, $w)",
	"SELECT 1 WHERE $__conditionalAll(a = 'x' /* ${w} */ OR b = $$ ${v} $$, $v)",
	"SELECT 1 WHERE $__conditionalAll(a = $$ x ) $$ AND b = ${v}, $w) -- '$v",
	"SELECT 1 WHERE $__conditionalAll(a = ${v:csv} -- ) '\n, $w)\nAND b = '$v'",
	"SELECT 1 WHERE $__conditionalAll(a = '${v:sqlstring}', ${w})",
}

var conditionalAllFuzzSeeds = []string{
	"a IN (${v:singlequote}) AND b = '$w'",
	" a = '$v' ",
	"a = $v",
	"a = ${v:raw}",
	"a = ${v:raw}e-- x'\n",
	"a = 1 -- x\n",
	"a = 1 /* $w */",
	"a = 1 /* $w",
	"a = 'x, y' AND b = \"$w\"",
	"a = '$v' OR b = \"$v\"",
	"has(['a','b'], c) AND d IN ($w)",
	"m = {'a': 1, 'b': 2} AND n = '$w'",
	"$__timeFilter(ts) AND a = '$v'",
	"$__dt(d, ts) AND a IN (${w:singlequote})",
	"a > $__fromTime AND b < $__interval_s",
	"a = ${__dashboard:raw} AND b = '${__user.login}'",
	"a = $$ x ) $$",
	"a = $$ it's $$ AND b = $w",
	"a = ${v:csv}",
	"a = `b` AND \"c\" = 1",
	"a = 'it\\'s' AND b = $v",
	"a = 1 # c\n",
	"a = 1 #! c\nAND b = $w",
	"a = ‘x’ AND b = $v",
	"f(a, b) = 1",
	"a = 1, b = 2",
	"$__conditionalAll(a = 1, $w)",
	"(a = 1) OR (b = $w",
	"a = 1) OR (b = $w",
	"",
}

func FuzzConditionalAllMatchesParenthesisedCondition(f *testing.F) {
	for _, seed := range conditionalAllFuzzSeeds {
		for _, hostile := range []string{"a", "1", "' OR 1=1 --", "$t$", "a\\", "*/"} {
			f.Add(seed, hostile)
		}
	}
	const head = "SELECT 1 WHERE "
	f.Fuzz(func(t *testing.T, condition, hostile string) {
		macro := "$__conditionalAll(" + condition + ", $v)"
		conditional := head + macro + " AND z = 1"
		refs, err := spec.ScanQuery(conditional)
		if err != nil || len(refs) == 0 || refs[0].Start != len(head) || refs[0].End != len(head)+len(macro) {
			t.Skip("the condition closes the macro itself")
		}
		parenthesised := head + "(" + strings.TrimSpace(condition) + ") AND z = 1"
		for _, values := range append([][]string{{hostile}}, hostileValues...) {
			got, err := Expand(ClickHouse, conditional, fuzzEnv(values...))
			if err != nil {
				continue
			}
			want, err := Expand(ClickHouse, parenthesised, fuzzEnv(values...))
			if err != nil {
				t.Fatalf("conditional all expands but its parenthesised condition does not\ncondition: %q\nvalues:    %q\nerror:     %v\nexpansion: %q", condition, values, err, got.Text)
			}
			if got.Text != want.Text {
				t.Fatalf("conditional all differs from the parenthesised condition\ncondition:     %q\nvalues:        %q\nconditional:   %q\nparenthesised: %q", condition, values, got.Text, want.Text)
			}
		}
		checkNoDesync(t, ClickHouse, parenthesised, []string{hostile})
	})
}

func FuzzExpandClickHouse(f *testing.F) {
	for _, seed := range clickHouseFuzzSeeds {
		for _, hostile := range []string{"a", "1", "' OR 1=1 --", "$t$"} {
			f.Add(seed, hostile)
		}
	}
	f.Fuzz(func(t *testing.T, template, hostile string) {
		checkNoDesync(t, ClickHouse, template, []string{hostile})
	})
}

var promQLFuzzSeeds = []string{
	`up{a="$v"}`,
	`up{a=~"$v"} # c` + "\r" + `"$w"`,
	"up # c\r`\n{a=\"$v\"}`",
	`sum by ($v) (up{a='$w'})`,
	`up{a=${v:doublequote}, b="$w"}`,
	`rate(x[$__rate_interval]) # "$v"`,
	"up{a=~`$v`, b=\"$w\"}",
	`up{a=~"${v:pipe}", b="$w"}`,
	`label_replace(up, "d", "$v", "s", "(.*)")`,
	`up{a="$__dashboard", b=~"${__user.login}"}`,
	`up{a=${__dashboard:doublequote}, b="$w"}`,
	`sum by (${__user.id}) (rate(x[$__interval_ms]))`,
	`up # "$__dashboard"` + "\r" + `{a="${__user.email}"}`,
	`up{a=~'${__user.name:regex}', t="${__from:date:YYYY-MM-DDTHH:mm}"}`,
}

func FuzzExpandPromQL(f *testing.F) {
	for _, seed := range promQLFuzzSeeds {
		for _, hostile := range []string{"a", "1", `" or vector(1) #`, "`"} {
			f.Add(seed, hostile)
		}
	}
	f.Fuzz(func(t *testing.T, template, hostile string) {
		checkNoDesync(t, PromQL, template, []string{hostile})
	})
}

func TestConditionalAllFuzzSeedsExpandToSomething(t *testing.T) {
	expanded := 0
	for _, seed := range conditionalAllFuzzSeeds {
		if _, err := Expand(ClickHouse, "$__conditionalAll("+seed+", $v)", fuzzEnv("a")); err == nil {
			expanded++
		}
	}
	if expanded < 15 {
		t.Fatalf("only %d conditional all fuzz seeds expand with a benign value; the oracle would be vacuous", expanded)
	}
}

func TestFuzzSeedsExpandToSomething(t *testing.T) {
	expanded := 0
	for _, seed := range clickHouseFuzzSeeds {
		if _, err := Expand(ClickHouse, seed, fuzzEnv("a")); err == nil {
			expanded++
		}
	}
	for _, seed := range promQLFuzzSeeds {
		if _, err := Expand(PromQL, seed, fuzzEnv("a")); err == nil {
			expanded++
		}
	}
	if expanded < 10 {
		t.Fatalf("only %d fuzz seeds expand with a benign value; the oracle would be vacuous", expanded)
	}
}
