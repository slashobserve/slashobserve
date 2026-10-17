package spec_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
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
