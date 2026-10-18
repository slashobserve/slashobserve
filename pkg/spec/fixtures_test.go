package spec_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const examplesDir = "../../spec/examples"

type manifestCase struct {
	File  string         `json:"file"`
	Layer string         `json:"layer"`
	Code  spec.IssueCode `json:"code"`
	Path  string         `json:"path"`
}

func readExample(t *testing.T, rel string) []byte {
	t.Helper()
	root, err := os.OpenRoot(examplesDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	data, err := root.ReadFile(rel)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func parseIssues(t *testing.T, data []byte) []spec.Issue {
	t.Helper()
	_, err := spec.Parse(data)
	var ve *spec.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("Parse error = %v, want *spec.ValidationError", err)
	}
	return ve.Issues
}

func TestValidExamples(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(examplesDir, "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("glob examples: %v (%d files)", err, len(files))
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			first, err := spec.Parse(readExample(t, filepath.Base(f)))
			if err != nil {
				t.Fatal(err)
			}
			out, err := json.Marshal(first)
			if err != nil {
				t.Fatal(err)
			}
			second, err := spec.Parse(out)
			if err != nil {
				t.Fatalf("reparse marshaled result: %v", err)
			}
			if !reflect.DeepEqual(first, second) {
				t.Fatal("round trip changed the dashboard")
			}
		})
	}
}

func TestInvalidExamples(t *testing.T) {
	var manifest struct {
		Cases []manifestCase `json:"cases"`
	}
	if err := json.Unmarshal(readExample(t, "invalid/expected.json"), &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Cases) == 0 {
		t.Fatal("manifest has no cases")
	}
	for _, c := range manifest.Cases {
		t.Run(c.File, func(t *testing.T) {
			issues := parseIssues(t, readExample(t, filepath.Join("invalid", c.File)))
			if len(issues) != 1 {
				t.Fatalf("got %d issues, want 1: %+v", len(issues), issues)
			}
			if issues[0].Code != c.Code || issues[0].Path != c.Path {
				t.Fatalf("got %s at %q (%s), want %s at %q", issues[0].Code, issues[0].Path, issues[0].Message, c.Code, c.Path)
			}
		})
	}
}

func TestRejectedDocuments(t *testing.T) {
	minimal := string(readExample(t, "minimal.json"))
	with := func(old, repl string) []byte {
		t.Helper()
		if !strings.Contains(minimal, old) {
			t.Fatalf("minimal.json does not contain %q", old)
		}
		return []byte(strings.Replace(minimal, old, repl, 1))
	}
	cases := []struct {
		name string
		data []byte
		code spec.IssueCode
		path string
	}{
		{"too large", bytes.Repeat([]byte(" "), spec.MaxBytes+1), spec.CodeTooLarge, ""},
		{"trailing value", []byte(`{} {}`), spec.CodeMalformedJSON, ""},
		{"truncated", []byte(`{"specVersion":`), spec.CodeMalformedJSON, ""},
		{"empty", nil, spec.CodeMalformedJSON, ""},
		{"too deep", bytes.Repeat([]byte("["), 1000), spec.CodeMalformedJSON, ""},
		{"whole-number float for an integer", with(`"w": 6`, `"w": 6.0`), spec.CodeSchema, "/panels/0/gridPos/w"},
		{"impossible absolute date", with(`"from": "now-15m"`, `"from": "2026-13-45T99:99:99Z"`), spec.CodeSchema, "/time/from"},
		{"future relative end is valid", with(`"to": "now"`, `"to": "now+1h"`), "", ""},
		{"future relative start after end", with(`"from": "now-15m"`, `"from": "now+1h"`), spec.CodeTimeRangeOrder, "/time"},
		{"invalid utf-8", with(`"title": "Minimal"`, "\"title\": \"Min\xffimal\""), spec.CodeMalformedJSON, ""},
		{"variable without type", with(`"panels"`, `"variables": [{"name": "q"}], "panels"`), spec.CodeSchema, "/variables/0"},
		{"mixed absolute start with relative end is not order checked", with(`"from": "now-15m"`, `"from": "2999-01-01T00:00:00Z"`), "", ""},
		{"mixed relative start with absolute end is not order checked", with(`"to": "now"`, `"to": "2000-01-01T00:00:00Z"`), "", ""},
		{"equal plain relative range", with(`"from": "now-15m"`, `"from": "now"`), spec.CodeTimeRangeOrder, "/time"},
		{"equal offset relative range", with(`"from": "now-15m", "to": "now"`, `"from": "now-1h", "to": "now-1h"`), spec.CodeTimeRangeOrder, "/time"},
		{"equal rounded relative range is not order checked", with(`"from": "now-15m", "to": "now"`, `"from": "now-1d/d", "to": "now-1d/d"`), "", ""},
		{"equal absolute range", with(`"from": "now-15m", "to": "now"`, `"from": "2026-01-01T00:00:00Z", "to": "2026-01-01T00:00:00Z"`), spec.CodeTimeRangeOrder, "/time"},
		{"rounded relative range is not order checked", with(`"from": "now-15m"`, `"from": "now/d"`), "", ""},
		{"rounded start after plain end", with(`"from": "now-15m", "to": "now"`, `"from": "now/d", "to": "now-1h"`), "", ""},
		{"unknown unit in an override", with(`"queries":`, `"overrides": [{"match": {"field": "n"}, "unit": "reqs"}], "queries":`), spec.CodeUnknownUnit, "/panels/0/overrides/0/unit"},
		{"unknown unit in a row child", with(`"panels": [`, `"panels": [{"id": "r", "type": "row", "title": "R", "gridPos": {"x": 0, "y": 4, "w": 24, "h": 1}, "panels": [{"id": "c", "type": "stat", "title": "C", "gridPos": {"x": 0, "y": 5, "w": 6, "h": 4}, "datasource": "ch", "queries": [{"query": "up"}], "unit": "reqs"}]}, `), spec.CodeUnknownUnit, "/panels/0/panels/0/unit"},
		{"whole-number float in a row child", with(`"panels": [`, `"panels": [{"id": "r", "type": "row", "title": "R", "gridPos": {"x": 0, "y": 4, "w": 24, "h": 1}, "panels": [{"id": "c", "type": "stat", "title": "C", "gridPos": {"x": 0, "y": 5, "w": 6.0, "h": 4}, "datasource": "ch", "queries": [{"query": "up"}]}]}, `), spec.CodeSchema, "/panels/0/panels/0/gridPos/w"},
		{"float out of range in a mapping range", with(`"queries":`, `"mappings": [{"range": [1e999, null], "text": "high"}], "queries":`), spec.CodeSchema, "/panels/0/mappings/0/range/0"},
		{"a macro of another datasource type is left to the query proxy", with(`"datasource": "ch"`, `"datasource": "prom"`), "", ""},
		{"unknown macro with unclosed args", with(`$__timeFilter(Timestamp)`, `$__foo(x`), spec.CodeUnknownMacro, "/panels/0/queries/0/query"},
		{"day start after month end", with(`"from": "now-15m", "to": "now"`, `"from": "now-1d", "to": "now-1M"`), spec.CodeTimeRangeOrder, "/time"},
		{"month start before 30 days end", with(`"from": "now-15m", "to": "now"`, `"from": "now-1M", "to": "now-30d"`), "", ""},
		{"rounded day start after week end", with(`"from": "now-15m", "to": "now"`, `"from": "now/d", "to": "now-7d"`), spec.CodeTimeRangeOrder, "/time"},
		{"whole day range", with(`"from": "now-15m", "to": "now"`, `"from": "now/d", "to": "now/d"`), "", ""},
		{"future month start never before 28 day end", with(`"from": "now-15m", "to": "now"`, `"from": "now+1M", "to": "now+28d"`), spec.CodeTimeRangeOrder, "/time"},
		{"future month start possibly before 31 day end", with(`"from": "now-15m", "to": "now"`, `"from": "now+1M", "to": "now+31d"`), "", ""},
		{"function macro with an expression", with(`$__timeFilter(Timestamp)`, `$__timeFilter(toDateTime(Timestamp))`), spec.CodeInvalidReference, "/panels/0/queries/0/query"},
		{"function macro with two words", with(`$__timeFilter(Timestamp)`, `$__timeFilter(a b)`), spec.CodeInvalidReference, "/panels/0/queries/0/query"},
		{"function macro with a dotted column", with(`$__timeFilter(Timestamp)`, `$__timeFilter(logs.Timestamp)`), "", ""},
		{"calendar month against days is not order checked", with(`"from": "now-15m", "to": "now"`, `"from": "now-1M", "to": "now-30d"`), "", ""},
		{"calendar year against days is not order checked", with(`"from": "now-15m", "to": "now"`, `"from": "now-1y", "to": "now-365d"`), "", ""},
		{"calendar unit with equal days is not order checked", with(`"from": "now-15m", "to": "now"`, `"from": "now-30d", "to": "now-1M"`), "", ""},
		{"null variables", with(`"panels"`, `"variables": null, "panels"`), spec.CodeSchema, "/variables"},
		{"escaped lone high surrogate", with(`"title": "Minimal"`, `"title": "Min\ud800imal"`), spec.CodeMalformedJSON, ""},
		{"escaped lone low surrogate", with(`"title": "Minimal"`, `"title": "Min\udc00imal"`), spec.CodeMalformedJSON, ""},
		{"escaped surrogate pair is valid", with(`"title": "Minimal"`, `"title": "Min\ud83d\ude00imal"`), "", ""},
		{"escaped backslash before u is not an escape", with(`"title": "Minimal"`, `"title": "Min\\ud800imal"`), "", ""},
		{"whole units rounded the wrong way round", with(`"from": "now-15m", "to": "now"`, `"from": "now/d", "to": "now-1d/d"`), spec.CodeTimeRangeOrder, "/time"},
		{"whole weeks rounded the wrong way round", with(`"from": "now-15m", "to": "now"`, `"from": "now/w", "to": "now-1w/w"`), spec.CodeTimeRangeOrder, "/time"},
		{"whole days rounded the same", with(`"from": "now-15m", "to": "now"`, `"from": "now-1d/d", "to": "now-1d/d"`), "", ""},
		{"zero absolute time", with(`"from": "now-15m"`, `"from": "0000-01-01T00:00:00Z"`), spec.CodeSchema, "/time/from"},
		{"go zero time", with(`"from": "now-15m"`, `"from": "0001-01-01T00:00:00Z"`), spec.CodeSchema, "/time/from"},
		{"year beyond 9999", with(`"to": "now"`, `"to": "9999-12-31T23:59:59-05:00"`), spec.CodeSchema, "/time/to"},
		{"offset beyond the supported years", with(`"from": "now-15m"`, `"from": "now-10000y"`), spec.CodeSchema, "/time/from"},
		{"offset count beyond int32", with(`"from": "now-15m"`, `"from": "now-2147483648s"`), spec.CodeSchema, "/time/from"},
		{"equal month offsets", with(`"from": "now-15m", "to": "now"`, `"from": "now-1M", "to": "now-1M"`), spec.CodeTimeRangeOrder, "/time"},
		{"year equals twelve months", with(`"from": "now-15m", "to": "now"`, `"from": "now-1y", "to": "now-12M"`), spec.CodeTimeRangeOrder, "/time"},
		{"equal future month offsets", with(`"from": "now-15m", "to": "now"`, `"from": "now+1M", "to": "now+1M"`), spec.CodeTimeRangeOrder, "/time"},
		{"month rounded the wrong way round", with(`"from": "now-15m", "to": "now"`, `"from": "now/M", "to": "now-1M/M"`), spec.CodeTimeRangeOrder, "/time"},
		{"year rounded the wrong way round", with(`"from": "now-15m", "to": "now"`, `"from": "now/y", "to": "now-1y/y"`), spec.CodeTimeRangeOrder, "/time"},
		{"month start after unrounded previous month", with(`"from": "now-15m", "to": "now"`, `"from": "now/M", "to": "now-1M"`), spec.CodeTimeRangeOrder, "/time"},
		{"previous month before month", with(`"from": "now-15m", "to": "now"`, `"from": "now-1M/M", "to": "now-2M/M"`), spec.CodeTimeRangeOrder, "/time"},
		{"whole previous month", with(`"from": "now-15m", "to": "now"`, `"from": "now-1M/M", "to": "now-1M/M"`), "", ""},
		{"two whole months", with(`"from": "now-15m", "to": "now"`, `"from": "now-2M/M", "to": "now-1M/M"`), "", ""},
		{"this month so far", with(`"from": "now-15m", "to": "now"`, `"from": "now/M", "to": "now"`), "", ""},
		{"seconds count beyond int32", with(`"from": "now-15m"`, `"from": "now-9999999999s"`), spec.CodeSchema, "/time/from"},
		{"float out of range", with(`"queries":`, `"min": 1e999, "queries":`), spec.CodeSchema, "/panels/0/min"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.code == "" {
				if _, err := spec.Parse(c.data); err != nil {
					t.Fatalf("Parse: %v", err)
				}
				return
			}
			issues := parseIssues(t, c.data)
			if len(issues) != 1 || issues[0].Code != c.code || issues[0].Path != c.path {
				t.Fatalf("got %+v, want one %s issue at %q", issues, c.code, c.path)
			}
		})
	}
}

func TestValidationErrorAtRoot(t *testing.T) {
	_, err := spec.Parse([]byte(`{}`))
	if err == nil || strings.Contains(err.Error(), ": :") {
		t.Fatalf("Error() = %v, want no empty path segment", err)
	}
}

func TestVariableSchemaIssues(t *testing.T) {
	minimal := string(readExample(t, "minimal.json"))
	cases := []struct {
		name     string
		variable string
		want     []string
	}{
		{"missing type only", `{"name": "q"}`, []string{`/variables/0: missing property 'type'`}},
		{"missing name and type keeps the query problem", `{"query": 5}`, []string{
			`/variables/0: missing properties 'name', 'type'`,
			`/variables/0/query: got number, want string`,
		}},
		{"unknown field is kept beside the missing type", `{"name": "q", "bogus": 1}`, []string{
			`/variables/0: missing property 'type'`,
			`/variables/0: additional properties 'bogus' not allowed`,
		}},
		{"unknown field in every branch is kept", `{"name": "q", "bogus": 1, "query": "up"}`, []string{
			`/variables/0: missing property 'type'`,
			`/variables/0: additional properties 'bogus' not allowed`,
		}},
		{"several unknown fields are combined", `{"name": "q", "bogus": 1, "other": 2}`, []string{
			`/variables/0: missing property 'type'`,
			`/variables/0: additional properties 'bogus', 'other' not allowed`,
		}},
		{"object datasource is not a uid", `{"name": "q", "type": "query", "datasource": {"uid": "prom"}, "query": "up"}`, []string{
			`/variables/0/datasource: got object, want string`,
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data := strings.Replace(minimal, `"panels"`, `"variables": [`+c.variable+`], "panels"`, 1)
			var got []string
			for _, issue := range parseIssues(t, []byte(data)) {
				got = append(got, issue.Path+": "+issue.Message)
			}
			if !slices.Equal(got, c.want) {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestLegacyShapeRejected(t *testing.T) {
	for _, name := range upgradeGoldenNames(t) {
		t.Run(name, func(t *testing.T) {
			issues := parseIssues(t, readUpgradeFile(t, filepath.Join("legacy", name)))
			if !slices.ContainsFunc(issues, func(i spec.Issue) bool {
				return i.Code == spec.CodeSchema && i.Path == "" && strings.Contains(i.Message, "timeRange")
			}) {
				t.Fatalf("issues = %+v, want a root schema issue naming timeRange", issues)
			}
		})
	}
}

func TestPanelHelpers(t *testing.T) {
	d, err := spec.Parse([]byte(`{
  "specVersion": "0", "title": "Helpers", "time": {"from": "now-1h", "to": "now"},
  "panels": [
    {"id": "a", "type": "stat", "title": "A", "gridPos": {"x": 0, "y": 0, "w": 6, "h": 4}, "datasource": "prom",
      "queries": [{"query": "up"}, {"ref": "errors", "query": "up", "datasource": "ch"}]},
    {"id": "r", "type": "row", "title": "R", "gridPos": {"x": 0, "y": 4, "w": 24, "h": 1}, "panels": [
      {"id": "b", "type": "stat", "title": "B", "gridPos": {"x": 0, "y": 5, "w": 6, "h": 4}, "datasource": "prom", "queries": [{"query": "up"}]}
    ]},
    {"id": "c", "type": "text", "gridPos": {"x": 0, "y": 9, "w": 6, "h": 4}, "content": ""}
  ]
}`))
	if err != nil {
		t.Fatal(err)
	}
	var got [][3]string
	for _, e := range d.AllPanels() {
		row := ""
		if e.Row != nil {
			row = e.Row.ID
		}
		got = append(got, [3]string{e.Pointer, e.Panel.ID, row})
	}
	want := [][3]string{{"/panels/0", "a", ""}, {"/panels/1", "r", ""}, {"/panels/1/panels/0", "b", "r"}, {"/panels/2", "c", ""}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("AllPanels = %v, want %v", got, want)
	}
	child, ok := d.FindPanel("b")
	if !ok || child.Pointer != "/panels/1/panels/0" || child.Row == nil || child.Row.ID != "r" {
		t.Fatalf("FindPanel(b) = %+v, %v", child, ok)
	}
	if _, ok := d.FindPanel("missing"); ok {
		t.Fatal("FindPanel found a panel that does not exist")
	}
	if n := d.PanelCount(); n != 4 {
		t.Fatalf("PanelCount = %d, want 4", n)
	}
	a := d.Panels[0]
	if a.QueryRef(0) != "A" || a.QueryRef(1) != "errors" {
		t.Fatalf("QueryRef = %q, %q, want A, errors", a.QueryRef(0), a.QueryRef(1))
	}
	if a.QueryDatasource(0) != "prom" || a.QueryDatasource(1) != "ch" {
		t.Fatalf("QueryDatasource = %q, %q, want prom, ch", a.QueryDatasource(0), a.QueryDatasource(1))
	}
	if spec.DefaultQueryRef(25) != "Z" {
		t.Fatalf("DefaultQueryRef(25) = %q, want Z", spec.DefaultQueryRef(25))
	}
}

func TestSelectionJSON(t *testing.T) {
	cases := []struct {
		json string
		sel  spec.Selection
	}{
		{`"a"`, spec.Selection{Values: []string{"a"}}},
		{`["a","b"]`, spec.Selection{Values: []string{"a", "b"}, List: true}},
		{`""`, spec.Selection{Values: []string{""}}},
	}
	for _, c := range cases {
		t.Run(c.json, func(t *testing.T) {
			var got spec.Selection
			if err := json.Unmarshal([]byte(c.json), &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, c.sel) {
				t.Fatalf("Unmarshal = %+v, want %+v", got, c.sel)
			}
			out, err := json.Marshal(c.sel)
			if err != nil {
				t.Fatal(err)
			}
			if string(out) != c.json {
				t.Fatalf("Marshal = %s, want %s", out, c.json)
			}
		})
	}
	var bad spec.Selection
	if err := json.Unmarshal([]byte(`5`), &bad); err == nil {
		t.Fatal("Unmarshal of a number succeeded")
	}
}

func TestFalseBooleansSurvive(t *testing.T) {
	d, err := spec.Parse([]byte(`{
  "specVersion": "0", "title": "False", "time": {"from": "now-1h", "to": "now"},
  "panels": [
    {"id": "a", "type": "stat", "title": "A", "gridPos": {"x": 0, "y": 0, "w": 6, "h": 4}, "datasource": "prom",
      "queries": [{"query": "up"}],
      "display": {"graph": false},
      "overrides": [{"match": {"field": "Value"}, "hidden": false}]}
  ]
}`))
	if err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"graph":false`, `"hidden":false`} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("marshaled Spec lacks %s: %s", want, out)
		}
	}
}
