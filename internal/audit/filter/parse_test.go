package filter

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestParseClassifiesTheForm(t *testing.T) {
	for _, tc := range []struct {
		text string
		form Form
		set  bool
	}{
		{"", FormKQL, false},
		{"   ", FormKQL, false},
		{"outcome:denied", FormKQL, true},
		{"kql: outcome:denied", FormKQL, true},
		{"lucene:", FormLucene, false},
		{"lucene: outcome:denied", FormLucene, true},
		{"  LUCENE:outcome:denied", FormLucene, true},
		{"KQL:outcome:denied", FormKQL, true},
		{"  \n outcome:denied", FormKQL, true},
		{`{"query":{"term":{"outcome":"denied"}}}`, FormDSL, true},
		{"  \n" + `{"query":{"term":{"outcome":"denied"}}}`, FormDSL, true},
		{`{"size":5}`, FormDSL, false},
		{`{}`, FormDSL, false},
	} {
		q, err := Parse(tc.text)
		if err != nil {
			t.Errorf("Parse(%q): %v", tc.text, err)
			continue
		}
		if q.Form != tc.form || (q.Clause != nil) != tc.set {
			t.Errorf("Parse(%q) = form %s, clause set %v; want %s, %v", tc.text, q.Form, q.Clause != nil, tc.form, tc.set)
		}
	}
}

func TestParseKeepsSizeAndAggs(t *testing.T) {
	q := mustParse(t, `{"size":7,"query":{"match_all":{}},"aggregations":{"n":{"cardinality":{"field":"actor_id"}}}}`)
	if q.Size == nil || *q.Size != 7 || len(q.Aggs) != 1 || q.Aggs[0].Type != "cardinality" || q.Aggs[0].Field != "actor_id" {
		t.Errorf("query = %+v", q)
	}
}

func TestParseTellsAnAbsentSizeFromZero(t *testing.T) {
	if q := mustParse(t, `{"query":{"match_all":{}}}`); q.Size != nil {
		t.Errorf("absent size = %d", *q.Size)
	}
	if q := mustParse(t, `{"size":0}`); q.Size == nil || *q.Size != 0 {
		t.Errorf("size 0 = %v", q.Size)
	}
}

func TestParseErrorsCarryPositions(t *testing.T) {
	for _, tc := range []struct {
		text     string
		pos, end int
		msg      string
	}{
		{"outcome:ok", 0, 10, `bad value "ok" for outcome: use one of: success, denied, failed`},
		{"outcome:denied and status:x", 19, 27, `bad value "x" for status`},
		{"kql: outcome:denied and status:x", 24, 32, `bad value "x" for status`},
		{"unknown_f:1", 0, 11, `unknown field "unknown_f"`},
		{"actoor:alice", 0, 12, `did you mean "actor"`},
		{"status:abc", 0, 10, "a whole number is expected"},
		{"run_id:nope", 0, 11, "a UUID is expected"},
		{"client_ip:300.1.1.1", 0, 19, "an IP address or a CIDR"},
		{"occurred_at >= yesterday", 0, 24, "a time such as"},
		{"outcome >= denied", 0, 17, "range is not supported on enum fields"},
		{"target:dashboard/", 0, 17, "target is missing an id"},
		{"*foo", 0, 4, "a wildcard in free text must be at the end of a word"},
		{"outcome:denied and *foo", 19, 23, "a wildcard in free text must be at the end of a word"},
		{"outcome:", 7, 8, "expected a value"},
		{"(outcome:denied", 0, 1, ""},
		{`{"query":`, 0, 9, "invalid JSON"},
		{`{"query":{"match_all":{}}}x`, 0, 27, "unexpected text after the JSON body"},
		{`{"query":{"term":{"nope":"x"}}}`, 18, 24, `unknown field "nope"`},
		{`  {"query":{"term":{"nope":"x"}}}`, 20, 26, `unknown field "nope"`},
		{`{"query":{"term":{"outcome":"ok"}}}`, 18, 27, `bad value "ok" for outcome`},
		{`{"query":{"range":{"outcome":{"gt":"a"}}}}`, 19, 28, "range is not supported"},
		{`{"query":[]}`, 1, 8, "query must be an object"},
		{`{"size":2000}`, 1, 7, "size must be a whole number from 0 to 1000"},
		{`{"size":-1}`, 1, 7, "size must be a whole number"},
		{`{"aggs":{"a":{"avg":{"field":"outcome"}}}}`, 9, 12, "avg needs a numeric field"},
		{`{"aggs":{"a":{"terms":{"field":"nope"}}}}`, 9, 12, `unknown field "nope"`},
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

func TestParseRefusesWhatItDoesNotSupportByName(t *testing.T) {
	for _, tc := range []struct {
		text string
		msg  string
	}{
		{`{"sort":[{"occurred_at":"desc"}]}`, `unsupported key "sort"`},
		{`{"_source":["id"]}`, `unsupported key "_source"`},
		{`{"fields":["id"]}`, `unsupported key "fields"`},
		{`{"runtime_mappings":{}}`, `unsupported key "runtime_mappings"`},
		{`{"query":{"fuzzy":{"actor_name":"x"}}}`, `unsupported query clause "fuzzy"`},
		{`{"query":{"nested":{"path":"a","query":{}}}}`, `unsupported query clause "nested"`},
		{`{"query":{"regexp":{"action":"a.*"}}}`, `unsupported query clause "regexp"`},
		{`{"query":{"prefix":{"action":"a"}}}`, `unsupported query clause "prefix"`},
		{`{"query":{"term":{"outcome":"denied"},"match_all":{}}}`, "exactly one query type"},
		{`{"query":{"bool":{"should":[],"boost":2}}}`, `unsupported key "boost"`},
		{`{"query":{"bool":{"should":[{"match_all":{}}],"minimum_should_match":"75%"}}}`, "minimum_should_match"},
		{`{"query":{"multi_match":{"query":"x","fields":["actor_name^2"]}}}`, "boosts are not supported"},
		{`{"query":{"multi_match":{"query":"x","type":"cross_fields"}}}`, "best_fields and phrase"},
		{`{"query":{"query_string":{"query":"a AND b"}}}`, "operators, ? and other special characters are not supported"},
		{`{"query":{"term":{"outcome":"denied","status":1}}}`, "exactly one field"},
		{`{"query":{"wildcard":{"action":{"value":"a*","boost":1}}}}`, `unsupported key "boost"`},
		{`{"aggs":{"a":{"scripted_metric":{}}}}`, "unsupported aggregation"},
		{`{"aggs":{"a":{"histogram":{"field":"status","interval":10}}}}`, "unsupported aggregation"},
		{`{"aggs":{"a":{"date_histogram":{"field":"occurred_at","calendar_interval":"1M"}}}}`, "needs a fixed_interval"},
		{`{"aggs":{"a":{"date_histogram":{"field":"occurred_at","fixed_interval":"1h","time_zone":"Asia/Kolkata"}}}}`, "UTC"},
		{`{"aggs":{"a":{"date_histogram":{"field":"id","fixed_interval":"1h"}}}}`, "can only bucket occurred_at"},
		{`{"aggs":{"a":{"terms":{"field":"origin","script":"x"}}}}`, `unsupported key "script"`},
		{`{"aggs":{"a":{"terms":{"field":"origin","min_doc_count":0}}}}`, "min_doc_count 1"},
		{`{"aggs":{"a":{"terms":{"field":"origin","size":0}}}}`, "terms size"},
		{`{"aggs":{"a":{"terms":{"field":"target"}}}}`, "needs a field"},
		{`{"aggs":{"a":{"terms":{"field":"origin"}},"b":{"terms":{"field":"action"}}}}`, "one bucket aggregation per level"},
		{`{"aggs":{"a":{"terms":{"field":"origin"},"aggs":{"b":{"terms":{"field":"action"},"aggs":{"c":{"terms":{"field":"tool"}}}}}}}}`, "at most two terms levels"},
		{`{"aggs":{"a":{"date_histogram":{"fixed_interval":"1h"},"aggs":{"b":{"date_histogram":{"fixed_interval":"1m"}}}}}}`, "only one date_histogram"},
		{`{"aggs":{"a":{"terms":{"field":"origin"}},"m":{"max":{"field":"duration_ms"}}}}`, "inside the innermost bucket"},
		{`{"aggs":{"a":{"max":{"field":"duration_ms"},"aggs":{"b":{"max":{"field":"status"}}}}}}`, "takes no sub-aggregations"},
		{`{"aggs":{"a":{"percentiles":{"field":"duration_ms","percents":[101]}}}}`, "between 0 and 100"},
		{`{"aggs":{"a":{"terms":{"field":"origin","size":1000},"aggs":{"b":{"terms":{"field":"action","size":2}}}}}}`, "multiply"},
		{`{"aggs":{"a":{"terms":{"field":"origin"}}},"aggregations":{}}`, "not both"},
	} {
		t.Run(tc.text, func(t *testing.T) {
			_, err := Parse(tc.text)
			if err == nil || !strings.Contains(err.Error(), tc.msg) {
				t.Errorf("Parse(%q) error = %v, want it to contain %q", tc.text, err, tc.msg)
			}
		})
	}
}

func TestParseBoundsTheQuery(t *testing.T) {
	if _, err := Parse(strings.Repeat("a", maxQueryBytes+1)); err == nil {
		t.Error("an oversized query parsed")
	}
	deep := strings.Repeat(`{"bool":{"filter":`, maxClauseDepth+2) + `{"match_all":{}}` + strings.Repeat("}}", maxClauseDepth+2)
	if _, err := Parse(`{"query":` + deep + `}`); err == nil || !strings.Contains(err.Error(), "too complex") {
		t.Errorf("a deeply nested query error = %v", err)
	}
	wide := make([]string, maxClauses+1)
	for i := range wide {
		wide[i] = `{"match_all":{}}`
	}
	if _, err := Parse(`{"query":{"bool":{"filter":[` + strings.Join(wide, ",") + `]}}}`); err == nil {
		t.Error("a query with too many clauses parsed")
	}
}

func FuzzParse(f *testing.F) {
	for _, seed := range []string{
		"", "outcome:denied", "not actor:alice* and status >= 400", `"a b" c*`, "kql: x", "lucene: x", "act*:x", "lucene: a AND (b OR -c) status:[1 TO *] x~2 y^3 /r/ \\", `lucene: actor:"a b"~2`, "outcome:(a or b)",
		`{"query":{"term":{"outcome":"denied"}}}`, `{"query":{"bool":{"should":[{"match_all":{}}],"minimum_should_match":1}}}`,
		`{"aggs":{"a":{"terms":{"field":"origin"},"aggs":{"m":{"max":{"field":"duration_ms"}}}}}}`, `{"size":3`, "{", `\`, "'", "\x00",
		`{"aggs":{"h":{"date_histogram":{"fixed_interval":"288230376151711744ms"}}}}`, `{"aggs":{"h":{"date_histogram":{"fixed_interval":"9223372036854775807ms"}}}}`,
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, text string) {
		q, err := Parse(text)
		if err != nil {
			var perr *Error
			if !errors.As(err, &perr) {
				t.Fatalf("Parse(%q) returned %T, want *Error", text, err)
			}
			if perr.Pos < 0 || perr.End < perr.Pos || perr.End > len(text) {
				t.Fatalf("Parse(%q) position %d..%d is outside the text", text, perr.Pos, perr.End)
			}
			return
		}
		rows, err := q.Rows(testWindow, Page{})
		if err != nil {
			t.Fatalf("Rows of %q: %v", text, err)
		}
		assertSafeSQL(t, rows.SQL, rows.Args)
		hist, err := q.Histogram(testWindow, time.Minute)
		if err != nil {
			t.Fatalf("Histogram of %q: %v", text, err)
		}
		assertSafeSQL(t, hist.SQL, hist.Args)
		if len(q.Aggs) > 0 {
			agg, err := q.Aggregate(testWindow)
			if err != nil && !strings.Contains(err.Error(), "interval too small") {
				t.Fatalf("Aggregate of %q: %v", text, err)
			}
			if err == nil {
				assertSafeSQL(t, agg.SQL, agg.Args)
			}
		}
	})
}

func TestFieldNamesWithQuotesStayInsideTheirQuotes(t *testing.T) {
	for _, tc := range []struct {
		text string
		msg  string
	}{
		{`{"query":{"term":{"a\"b":"x"}}}`, `unknown field "a\"b"`},
		{`{"query":{"exists":{"field":"x\" OR \"1"}}}`, `unknown field "x\" OR \"1"`},
		{`{"query":{"term":{"actor\"":"x"}}}`, `unknown field "actor\"`},
		{`{"aggs":{"a":{"terms":{"field":"o\"r"}}}}`, `unknown field "o\"r"`},
		{`lucene: a\"b:x`, `unknown field "a\"b"`},
		{`lucene: _exists_:x\"y`, `unknown field "x\"y"`},
	} {
		t.Run(tc.text, func(t *testing.T) {
			_, err := Parse(tc.text)
			if err == nil || !strings.Contains(err.Error(), tc.msg) {
				t.Errorf("Parse(%q) error = %v, want it to contain %s", tc.text, err, tc.msg)
			}
		})
	}
	if _, err := (Query{}).Facets(testWindow, `x" OR "1`, 5); err == nil || !strings.Contains(err.Error(), `"x\" OR \"1"`) {
		t.Errorf("Facets error = %v", err)
	}
}
