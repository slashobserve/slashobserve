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
		{"impossible absolute date", with(`"from": "now-15m"`, `"from": "2026-13-45T99:99:99Z"`), spec.CodeSchema, "/timeRange/from"},
		{"future relative end is valid", with(`"to": "now"`, `"to": "now+1h"`), "", ""},
		{"future relative start after end", with(`"from": "now-15m"`, `"from": "now+1h"`), spec.CodeTimeRangeOrder, "/timeRange"},
		{"invalid utf-8", with(`"title": "Minimal"`, "\"title\": \"Min\xffimal\""), spec.CodeMalformedJSON, ""},
		{"variable without type", with(`"panels"`, `"variables": [{"name": "q"}], "panels"`), spec.CodeSchema, "/variables/0"},
		{"mixed absolute start with relative end is not order checked", with(`"from": "now-15m"`, `"from": "2999-01-01T00:00:00Z"`), "", ""},
		{"mixed relative start with absolute end is not order checked", with(`"to": "now"`, `"to": "2000-01-01T00:00:00Z"`), "", ""},
		{"equal plain relative range", with(`"from": "now-15m"`, `"from": "now"`), spec.CodeTimeRangeOrder, "/timeRange"},
		{"equal offset relative range", with(`"from": "now-15m", "to": "now"`, `"from": "now-1h", "to": "now-1h"`), spec.CodeTimeRangeOrder, "/timeRange"},
		{"equal rounded relative range is not order checked", with(`"from": "now-15m", "to": "now"`, `"from": "now-1d/d", "to": "now-1d/d"`), "", ""},
		{"equal absolute range", with(`"from": "now-15m", "to": "now"`, `"from": "2026-01-01T00:00:00Z", "to": "2026-01-01T00:00:00Z"`), spec.CodeTimeRangeOrder, "/timeRange"},
		{"rounded relative range is not order checked", with(`"from": "now-15m"`, `"from": "now/d"`), "", ""},
		{"rounded start after plain end", with(`"from": "now-15m", "to": "now"`, `"from": "now/d", "to": "now-1h"`), "", ""},
		{"unknown macro with unclosed args", with(`$__timeFilter(Timestamp)`, `$__foo(x`), spec.CodeUnknownMacro, "/panels/0/query"},
		{"day start after month end", with(`"from": "now-15m", "to": "now"`, `"from": "now-1d", "to": "now-1M"`), spec.CodeTimeRangeOrder, "/timeRange"},
		{"month start before 30 days end", with(`"from": "now-15m", "to": "now"`, `"from": "now-1M", "to": "now-30d"`), "", ""},
		{"rounded day start after week end", with(`"from": "now-15m", "to": "now"`, `"from": "now/d", "to": "now-7d"`), spec.CodeTimeRangeOrder, "/timeRange"},
		{"whole day range", with(`"from": "now-15m", "to": "now"`, `"from": "now/d", "to": "now/d"`), "", ""},
		{"future month start never before 28 day end", with(`"from": "now-15m", "to": "now"`, `"from": "now+1M", "to": "now+28d"`), spec.CodeTimeRangeOrder, "/timeRange"},
		{"future month start possibly before 31 day end", with(`"from": "now-15m", "to": "now"`, `"from": "now+1M", "to": "now+31d"`), "", ""},
		{"function macro with an expression", with(`$__timeFilter(Timestamp)`, `$__timeFilter(toDateTime(Timestamp))`), spec.CodeInvalidReference, "/panels/0/query"},
		{"function macro with two words", with(`$__timeFilter(Timestamp)`, `$__timeFilter(a b)`), spec.CodeInvalidReference, "/panels/0/query"},
		{"function macro with a dotted column", with(`$__timeFilter(Timestamp)`, `$__timeFilter(logs.Timestamp)`), "", ""},
		{"calendar month against days is not order checked", with(`"from": "now-15m", "to": "now"`, `"from": "now-1M", "to": "now-30d"`), "", ""},
		{"calendar year against days is not order checked", with(`"from": "now-15m", "to": "now"`, `"from": "now-1y", "to": "now-365d"`), "", ""},
		{"calendar unit with equal days is not order checked", with(`"from": "now-15m", "to": "now"`, `"from": "now-30d", "to": "now-1M"`), "", ""},
		{"null variables", with(`"panels"`, `"variables": null, "panels"`), spec.CodeSchema, "/variables"},
		{"escaped lone high surrogate", with(`"title": "Minimal"`, `"title": "Min\ud800imal"`), spec.CodeMalformedJSON, ""},
		{"escaped lone low surrogate", with(`"title": "Minimal"`, `"title": "Min\udc00imal"`), spec.CodeMalformedJSON, ""},
		{"escaped surrogate pair is valid", with(`"title": "Minimal"`, `"title": "Min\ud83d\ude00imal"`), "", ""},
		{"escaped backslash before u is not an escape", with(`"title": "Minimal"`, `"title": "Min\\ud800imal"`), "", ""},
		{"whole units rounded the wrong way round", with(`"from": "now-15m", "to": "now"`, `"from": "now/d", "to": "now-1d/d"`), spec.CodeTimeRangeOrder, "/timeRange"},
		{"whole weeks rounded the wrong way round", with(`"from": "now-15m", "to": "now"`, `"from": "now/w", "to": "now-1w/w"`), spec.CodeTimeRangeOrder, "/timeRange"},
		{"whole days rounded the same", with(`"from": "now-15m", "to": "now"`, `"from": "now-1d/d", "to": "now-1d/d"`), "", ""},
		{"zero absolute time", with(`"from": "now-15m"`, `"from": "0000-01-01T00:00:00Z"`), spec.CodeSchema, "/timeRange/from"},
		{"go zero time", with(`"from": "now-15m"`, `"from": "0001-01-01T00:00:00Z"`), spec.CodeSchema, "/timeRange/from"},
		{"year beyond 9999", with(`"to": "now"`, `"to": "9999-12-31T23:59:59-05:00"`), spec.CodeSchema, "/timeRange/to"},
		{"offset beyond the supported years", with(`"from": "now-15m"`, `"from": "now-10000y"`), spec.CodeSchema, "/timeRange/from"},
		{"offset count beyond int32", with(`"from": "now-15m"`, `"from": "now-2147483648s"`), spec.CodeSchema, "/timeRange/from"},
		{"equal month offsets", with(`"from": "now-15m", "to": "now"`, `"from": "now-1M", "to": "now-1M"`), spec.CodeTimeRangeOrder, "/timeRange"},
		{"year equals twelve months", with(`"from": "now-15m", "to": "now"`, `"from": "now-1y", "to": "now-12M"`), spec.CodeTimeRangeOrder, "/timeRange"},
		{"equal future month offsets", with(`"from": "now-15m", "to": "now"`, `"from": "now+1M", "to": "now+1M"`), spec.CodeTimeRangeOrder, "/timeRange"},
		{"month rounded the wrong way round", with(`"from": "now-15m", "to": "now"`, `"from": "now/M", "to": "now-1M/M"`), spec.CodeTimeRangeOrder, "/timeRange"},
		{"year rounded the wrong way round", with(`"from": "now-15m", "to": "now"`, `"from": "now/y", "to": "now-1y/y"`), spec.CodeTimeRangeOrder, "/timeRange"},
		{"month start after unrounded previous month", with(`"from": "now-15m", "to": "now"`, `"from": "now/M", "to": "now-1M"`), spec.CodeTimeRangeOrder, "/timeRange"},
		{"previous month before month", with(`"from": "now-15m", "to": "now"`, `"from": "now-1M/M", "to": "now-2M/M"`), spec.CodeTimeRangeOrder, "/timeRange"},
		{"whole previous month", with(`"from": "now-15m", "to": "now"`, `"from": "now-1M/M", "to": "now-1M/M"`), "", ""},
		{"two whole months", with(`"from": "now-15m", "to": "now"`, `"from": "now-2M/M", "to": "now-1M/M"`), "", ""},
		{"this month so far", with(`"from": "now-15m", "to": "now"`, `"from": "now/M", "to": "now"`), "", ""},
		{"seconds count beyond int32", with(`"from": "now-15m"`, `"from": "now-9999999999s"`), spec.CodeSchema, "/timeRange/from"},
		{"float out of range", with(`"query":`, `"min": 1e999, "query":`), spec.CodeSchema, "/panels/0/min"},
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
		{"nested missing type is not a missing variable type", `{"name": "q", "type": "query", "datasource": {"uid": "prom"}, "query": "up"}`, []string{
			`/variables/0/datasource: missing property 'type'`,
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
