package filter

import (
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestLuceneParsesEveryConstruct(t *testing.T) {
	for _, tc := range []struct {
		text   string
		clause string
	}{
		{"lucene:", `null`},
		{"lucene:   ", `null`},
		{"lucene: outcome:denied", `{"match":{"outcome":"denied"}}`},
		{`lucene: outcome:"denied"`, `{"match_phrase":{"outcome":"denied"}}`},
		{"lucene: checkout", `{"multi_match":{"query":"checkout"}}`},
		{`lucene: "dashboard update"`, `{"multi_match":{"query":"dashboard update","type":"phrase"}}`},
		{"lucene: check*", `{"query_string":{"query":"check*"}}`},
		{`lucene: check\*`, `{"multi_match":{"query":"check*"}}`},
		{`lucene: a\(b*`, `{"query_string":{"query":"a\\(b*"}}`},
		{"lucene: *", `{"query_string":{"query":"*"}}`},
		{"lucene: outcome:denied AND status:403", `{"bool":{"must":[{"match":{"outcome":"denied"}},{"match":{"status":"403"}}]}}`},
		{"lucene: outcome:denied && status:403", `{"bool":{"must":[{"match":{"outcome":"denied"}},{"match":{"status":"403"}}]}}`},
		{"lucene: outcome:denied OR outcome:failed", `{"bool":{"should":[{"match":{"outcome":"denied"}},{"match":{"outcome":"failed"}}]}}`},
		{"lucene: outcome:denied||outcome:failed", `{"bool":{"should":[{"match":{"outcome":"denied"}},{"match":{"outcome":"failed"}}]}}`},
		{"lucene: outcome:denied failed", `{"bool":{"should":[{"match":{"outcome":"denied"}},{"multi_match":{"query":"failed"}}]}}`},
		{"lucene: a AND b OR c", `{"bool":{"should":[{"bool":{"must":[{"multi_match":{"query":"a"}},{"multi_match":{"query":"b"}}]}},{"multi_match":{"query":"c"}}]}}`},
		{"lucene: NOT outcome:denied", `{"bool":{"must_not":[{"match":{"outcome":"denied"}}]}}`},
		{"lucene: !outcome:denied", `{"bool":{"must_not":[{"match":{"outcome":"denied"}}]}}`},
		{"lucene: -outcome:denied", `{"bool":{"must_not":[{"match":{"outcome":"denied"}}]}}`},
		{"lucene: +outcome:denied", `{"match":{"outcome":"denied"}}`},
		{"lucene: +outcome:denied -actor:Riya", `{"bool":{"must":[{"match":{"outcome":"denied"}}],"must_not":[{"match":{"actor":"Riya"}}]}}`},
		{"lucene: +outcome:denied status:403", `{"bool":{"must":[{"match":{"outcome":"denied"}}],"should":[{"match":{"status":"403"}}]}}`},
		{"lucene: outcome:denied AND NOT actor:Riya", `{"bool":{"must":[{"match":{"outcome":"denied"}}],"must_not":[{"match":{"actor":"Riya"}}]}}`},
		{"lucene: outcome:denied && !actor:Riya || status:401", `{"bool":{"should":[{"bool":{"must":[{"match":{"outcome":"denied"}}],"must_not":[{"match":{"actor":"Riya"}}]}},{"match":{"status":"401"}}]}}`},
		{"lucene: outcome:(denied OR failed)", `{"bool":{"should":[{"match":{"outcome":"denied"}},{"match":{"outcome":"failed"}}]}}`},
		{"lucene: outcome:(denied failed)", `{"bool":{"should":[{"match":{"outcome":"denied"}},{"match":{"outcome":"failed"}}]}}`},
		{"lucene: outcome:(+denied -failed)", `{"bool":{"must":[{"match":{"outcome":"denied"}}],"must_not":[{"match":{"outcome":"failed"}}]}}`},
		{`lucene: actor:(Riya OR "Ishan Dubey" OR tool:x)`, `{"bool":{"should":[{"match":{"actor":"Riya"}},{"match_phrase":{"actor":"Ishan Dubey"}},{"match":{"tool":"x"}}]}}`},
		{"lucene: (outcome:denied OR outcome:failed) AND status:>=400", `{"bool":{"must":[{"bool":{"should":[{"match":{"outcome":"denied"}},{"match":{"outcome":"failed"}}]}},{"range":{"status":{"gte":"400"}}}]}}`},
		{"lucene: status:[400 TO 499]", `{"range":{"status":{"gte":"400","lte":"499"}}}`},
		{"lucene: status:{400 TO 499}", `{"range":{"status":{"gt":"400","lt":"499"}}}`},
		{"lucene: status:[400 TO 499}", `{"range":{"status":{"gte":"400","lt":"499"}}}`},
		{"lucene: status:{400 TO 499]", `{"range":{"status":{"gt":"400","lte":"499"}}}`},
		{"lucene: status:[400 TO *]", `{"range":{"status":{"gte":"400"}}}`},
		{"lucene: status:{* TO 499}", `{"range":{"status":{"lt":"499"}}}`},
		{"lucene: status:[* TO *]", `{"exists":{"field":"status"}}`},
		{"lucene: status:>400", `{"range":{"status":{"gt":"400"}}}`},
		{"lucene: status:>=400", `{"range":{"status":{"gte":"400"}}}`},
		{"lucene: status:<400", `{"range":{"status":{"lt":"400"}}}`},
		{"lucene: status:<=400", `{"range":{"status":{"lte":"400"}}}`},
		{"lucene: occurred_at:[2026-11-01T10:00:00Z TO 2026-11-02]", `{"range":{"occurred_at":{"gte":"2026-11-01T10:00:00Z","lte":"2026-11-02"}}}`},
		{`lucene: occurred_at:>"2026-11-01 10:00:00"`, `{"range":{"occurred_at":{"gt":"2026-11-01 10:00:00"}}}`},
		{`lucene: occurred_at:["2026-11-01" TO "2026-11-02"}`, `{"range":{"occurred_at":{"gte":"2026-11-01","lt":"2026-11-02"}}}`},
		{"lucene: actor:(a OR b) AND NOT status:(401 403)", `{"bool":{"must":[{"bool":{"should":[{"match":{"actor":"a"}},{"match":{"actor":"b"}}]}}],"must_not":[{"bool":{"should":[{"match":{"status":"401"}},{"match":{"status":"403"}}]}}]}}`},
		{"lucene: actor:Ish*", `{"wildcard":{"actor":{"value":"Ish*"}}}`},
		{"lucene: actor:I?han*", `{"wildcard":{"actor":{"value":"I?han*"}}}`},
		{`lucene: action:dashboard\.*`, `{"wildcard":{"action":{"value":"dashboard.*"}}}`},
		{`lucene: action:a\*b*`, `{"wildcard":{"action":{"value":"a\\*b*"}}}`},
		{`lucene: action:a\*b`, `{"match":{"action":"a*b"}}`},
		{"lucene: _exists_:reason", `{"exists":{"field":"reason"}}`},
		{"lucene: reason:*", `{"exists":{"field":"reason"}}`},
		{"lucene: NOT reason:*", `{"bool":{"must_not":[{"exists":{"field":"reason"}}]}}`},
		{"lucene: client_ip:10.4.0.0/16", `{"match":{"client_ip":"10.4.0.0/16"}}`},
		{`lucene: client_ip:"::1"`, `{"match_phrase":{"client_ip":"::1"}}`},
		{"lucene: client_ip:::1", `{"match":{"client_ip":"::1"}}`},
		{"lucene: target:dashboard/d1", `{"match":{"target":"dashboard/d1"}}`},
		{`lucene: action:a\:b`, `{"match":{"action":"a:b"}}`},
		{"lucene: status:-1", `{"match":{"status":"-1"}}`},
		{"lucene: act*:x", `{"multi_match":{"fields":["act*"],"lenient":true,"query":"x"}}`},
		{"lucene: act*:x*", `{"query_string":{"analyze_wildcard":true,"fields":["act*"],"lenient":true,"query":"x*"}}`},
		{`lucene: act*:"x y"`, `{"multi_match":{"fields":["act*"],"lenient":true,"query":"x y","type":"phrase"}}`},
		{"lucene: *:*", `{"match_all":{}}`},
		{"lucene:\n  outcome:denied\n  AND\n  status:403", `{"bool":{"must":[{"match":{"outcome":"denied"}},{"match":{"status":"403"}}]}}`},
	} {
		t.Run(tc.text, func(t *testing.T) {
			q, err := Parse(tc.text)
			if err != nil {
				t.Fatal(err)
			}
			if q.Form != FormLucene || len(q.Aggs) != 0 {
				t.Errorf("query = %+v", q)
			}
			got, _ := json.Marshal(q.Clause)
			if string(got) != tc.clause {
				t.Errorf("clause\n got %s\nwant %s", got, tc.clause)
			}
		})
	}
}

func TestLuceneRefusalsAndSyntaxErrorsCarryPositions(t *testing.T) {
	for _, tc := range []struct {
		text     string
		pos, end int
		msg      string
	}{
		{"lucene: actor:Riya~", 18, 19, "fuzzy searches (term~n)"},
		{"lucene: actor:Riya~2", 18, 20, "fuzzy searches (term~n)"},
		{"lucene: Riya~0.8 AND x", 12, 16, "fuzzy searches (term~n)"},
		{`lucene: "a b"~3`, 13, 15, `proximity searches ("a b"~n)`},
		{`lucene: actor:"a b"~3`, 19, 21, "proximity searches"},
		{"lucene: actor:Riya^2", 18, 20, "boosts (^n)"},
		{"lucene: Riya^1.5", 12, 16, "boosts (^n)"},
		{"lucene: (a b)^2", 13, 15, "boosts (^n)"},
		{`lucene: "a b"^2`, 13, 15, "boosts (^n)"},
		{"lucene: status:[1 TO 2]^2", 23, 25, "boosts (^n)"},
		{"lucene: actor:/R.*/", 14, 19, "regular expressions (/.../)"},
		{"lucene: /abc/ AND x", 8, 13, "regular expressions (/.../)"},
		{`lucene: /a\/b/`, 8, 14, "regular expressions (/.../)"},
		{"  lucene:  /abc", 11, 15, "regular expressions (/.../)"},
		{"lucene: (", 8, 9, "missing ) for this ("},
		{"lucene: (a OR b", 8, 9, "missing ) for this ("},
		{"lucene: a)", 9, 10, "unmatched )"},
		{"lucene: )", 8, 9, "unmatched )"},
		{"lucene: ()", 8, 10, "empty group"},
		{"lucene: a AND", 10, 13, "AND needs a clause after it"},
		{"lucene: a && ", 10, 12, "&& needs a clause after it"},
		{"lucene: a OR", 10, 12, "OR needs a clause after it"},
		{"lucene: AND a", 8, 11, "AND needs a clause before it"},
		{"lucene: a OR OR b", 13, 15, "OR needs a clause before it"},
		{"lucene: NOT", 8, 11, "NOT needs a clause after it"},
		{"lucene: -", 8, 9, "a - must touch the clause"},
		{"lucene: + a", 8, 9, "a + must touch the clause"},
		{"lucene: actor:", 13, 14, "expected a value after the :"},
		{"lucene: actor: x", 13, 14, "expected a value after the :"},
		{`lucene: "abc`, 8, 12, "missing the closing quote"},
		{`lucene: actor:"abc`, 14, 18, "missing the closing quote"},
		{"lucene: status:[1 TO 2", 15, 22, "a range is written"},
		{"lucene: status:[1 2]", 15, 18, "a range is written"},
		{"lucene: [1 TO 2]", 8, 9, "a range needs a field"},
		{"lucene: status:>", 15, 16, "a comparison needs a value"},
		{"lucene: status:>4*", 16, 18, "cannot use wildcards"},
		{`lucene: a\`, 9, 10, "a backslash needs a character"},
		{"lucene: :x", 8, 9, `unexpected ':'`},
		{"lucene: ^2", 8, 9, `unexpected '^'`},
		{"lucene: ac?:x", 8, 11, "a field name can use * but not ?"},
		{"lucene: outcome:ok", 8, 18, `bad value "ok" for outcome`},
		{"lucene: outcome:denied AND status:x", 27, 35, `bad value "x" for status`},
		{"lucene: unknown_f:1", 8, 19, `unknown field "unknown_f"`},
		{"lucene: actoor:x", 8, 16, `did you mean "actor"`},
		{"lucene: status:[a TO b]", 8, 23, "a whole number is expected"},
		{"lucene: outcome:[a TO b]", 8, 24, "range is not supported on enum fields"},
		{"lucene: client_ip:300.1.1.1", 8, 27, "an IP address or a CIDR"},
		{"lucene: _exists_:nope", 8, 21, `unknown field "nope"`},
		{"lucene: *foo", 8, 12, "a wildcard in free text must be at the end of a word"},
		{"lucene: ok AND a?c", 15, 18, "operators, ? and other special characters are not supported"},
	} {
		t.Run(tc.text, func(t *testing.T) {
			_, err := Parse(tc.text)
			var perr *Error
			if !errors.As(err, &perr) {
				t.Fatalf("Parse(%q) error = %v, want *Error", tc.text, err)
			}
			if perr.Pos != tc.pos || perr.End != tc.end {
				t.Errorf("position = %d..%d, want %d..%d (%s)", perr.Pos, perr.End, tc.pos, tc.end, perr.Msg)
			}
			if !strings.Contains(perr.Msg, tc.msg) {
				t.Errorf("message %q does not contain %q", perr.Msg, tc.msg)
			}
			if want := "at byte " + strconv.Itoa(perr.Pos) + ": "; !strings.HasPrefix(perr.Error(), want) {
				t.Errorf("Error() = %q, want prefix %q", perr.Error(), want)
			}
		})
	}
}

func TestLuceneQueriesCompileToParameterisedWhereClauses(t *testing.T) {
	for _, tc := range []struct {
		text  string
		where string
		args  []any
	}{
		{"lucene:", "", []any{}},
		{"lucene: outcome:denied AND NOT actor:Riya", "(COALESCE((outcome = $3), false) AND NOT COALESCE((actor_name = $4), false))", []any{"denied", "Riya"}},
		{"lucene: NOT actor:Riya", "(NOT COALESCE((actor_name = $3), false))", []any{"Riya"}},
		{"lucene: outcome:(denied OR failed)", "((COALESCE((outcome = $3), false) OR COALESCE((outcome = $4), false)))", []any{"denied", "failed"}},
		{"lucene: status:[400 TO 499}", "COALESCE((status >= $3 AND status < $4), false)", []any{int32(400), int32(499)}},
		{"lucene: duration_ms:<=12", "COALESCE((duration_ms <= $3), false)", []any{int32(12)}},
		{"lucene: client_ip:10.4.7.9/16", "COALESCE((client_ip <<= $3::inet), false)", []any{"10.4.0.0/16"}},
		{"lucene: target:dashboard/d1", "COALESCE((target_kind = $3 AND target_id = $4), false)", []any{"dashboard", "d1"}},
		{"lucene: target:dashboard", "COALESCE((target_kind = $3), false)", []any{"dashboard"}},
		{"lucene: actor:Ish*", `COALESCE(actor_name LIKE $3 ESCAPE '\', false)`, []any{"Ish%"}},
		{"lucene: actor:i?han", `COALESCE(actor_name LIKE $3 ESCAPE '\', false)`, []any{"i_han"}},
		{`lucene: actor:i\?han`, "COALESCE((actor_name = $3), false)", []any{"i?han"}},
		{"lucene: actor:a_b?c%*", `COALESCE(actor_name LIKE $3 ESCAPE '\', false)`, []any{`a\_b_c\%%`}},
		{"lucene: _exists_:reason", "(reason IS NOT NULL)", []any{}},
		{"lucene: NOT reason:*", "(NOT (reason IS NOT NULL))", []any{}},
		{"lucene: act*:x", "(COALESCE((action = $3), false) OR COALESCE((actor_name = $4), false) OR COALESCE((actor_role = $5), false))", []any{"x", "x", "x"}},
		{"lucene: checkout", "(search @@ plainto_tsquery('simple', $3))", []any{"checkout"}},
		{`lucene: "dashboard update"`, "(search @@ phraseto_tsquery('simple', $3))", []any{"dashboard update"}},
		{"lucene: check*", "(search @@ to_tsquery('simple', $3))", []any{"'check':*"}},
		{"lucene: *:*", "true", []any{}},
		{"lucene: occurred_at:[2026-11-01T10:00:00Z TO *]", "COALESCE((occurred_at >= $3), false)", []any{testWindow.From.Add(10 * 3600e9)}},
	} {
		t.Run(tc.text, func(t *testing.T) {
			where, args := whereAndArgs(t, tc.text)
			if where != tc.where {
				t.Errorf("where\n got %s\nwant %s", where, tc.where)
			}
			if !reflect.DeepEqual(args, tc.args) {
				t.Errorf("args\n got %#v\nwant %#v", args, tc.args)
			}
		})
	}
}

func TestOneQuestionInThreeFormsCompilesAlike(t *testing.T) {
	for _, forms := range [][3]string{
		{"outcome:denied and status:403", "lucene: outcome:denied AND status:403", `{"query":{"bool":{"filter":[{"term":{"outcome":"denied"}},{"term":{"status":403}}]}}}`},
		{"outcome:denied or outcome:failed", "lucene: outcome:denied OR outcome:failed", `{"query":{"bool":{"should":[{"term":{"outcome":"denied"}},{"term":{"outcome":"failed"}}],"minimum_should_match":1}}}`},
		{"not actor:Riya", "lucene: NOT actor:Riya", `{"query":{"bool":{"must_not":[{"term":{"actor":"Riya"}}]}}}`},
		{"not actor:Riya", "lucene: -actor:Riya", `{"query":{"bool":{"must_not":[{"match":{"actor":"Riya"}}]}}}`},
		{"status >= 400", "lucene: status:>=400", `{"query":{"range":{"status":{"gte":400}}}}`},
		{"occurred_at < 2026-11-01", "lucene: occurred_at:<2026-11-01", `{"query":{"range":{"occurred_at":{"lt":"2026-11-01"}}}}`},
		{"reason:*", "lucene: _exists_:reason", `{"query":{"exists":{"field":"reason"}}}`},
		{"reason:*", "lucene: reason:*", `{"query":{"exists":{"field":"reason"}}}`},
		{"client_ip:10.4.0.0/16", "lucene: client_ip:10.4.0.0/16", `{"query":{"term":{"client_ip":"10.4.0.0/16"}}}`},
		{"target:dashboard/d1", "lucene: target:dashboard/d1", `{"query":{"term":{"target":"dashboard/d1"}}}`},
		{"checkout", "lucene: checkout", `{"query":{"multi_match":{"query":"checkout"}}}`},
		{`"dashboard update"`, `lucene: "dashboard update"`, `{"query":{"multi_match":{"query":"dashboard update","type":"phrase"}}}`},
		{"check*", "lucene: check*", `{"query":{"query_string":{"query":"check*"}}}`},
		{`actor:"Ishan Dubey"`, `lucene: actor:"Ishan Dubey"`, `{"query":{"match_phrase":{"actor":"Ishan Dubey"}}}`},
		{"actor:Riya and status:403", "lucene: actor:Riya AND status:403", `{"query":{"bool":{"filter":[{"term":{"actor":"Riya"}},{"term":{"status":403}}]}}}`},
	} {
		t.Run(forms[1], func(t *testing.T) {
			wantWhere, wantArgs := whereAndArgs(t, forms[0])
			for _, text := range forms[1:] {
				where, args := whereAndArgs(t, text)
				if where != wantWhere || !reflect.DeepEqual(args, wantArgs) {
					t.Errorf("%s\n got %s %#v\nwant %s %#v (from %s)", text, where, args, wantWhere, wantArgs, forms[0])
				}
			}
		})
	}
}
