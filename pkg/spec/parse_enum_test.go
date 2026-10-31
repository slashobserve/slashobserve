package spec_test

import (
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestEnumOnlyAnyOfGivesOneIssue(t *testing.T) {
	minimal := string(readExample(t, "minimal.json"))
	with := func(old, repl string) []byte {
		t.Helper()
		if !strings.Contains(minimal, old) {
			t.Fatalf("minimal.json does not contain %q", old)
		}
		return []byte(strings.Replace(minimal, old, repl, 1))
	}
	timeSeries := func(display string) []byte {
		return with(`"type": "stat",`, `"type": "timeseries", "display": `+display+`,`)
	}
	rejected := []struct {
		name     string
		data     []byte
		path     string
		contains string
	}{
		{"unknown stat reducer", with(`"queries":`, `"display": {"reduce": "p100"}, "queries":`), "/panels/0/display/reduce", "lastNotNull"},
		{"unknown legend value", timeSeries(`{"legendValues": ["nope"]}`), "/panels/0/display/legendValues/0", "lastNotNull"},
		{"unknown unit", with(`"queries":`, `"unit": "reqs", "queries":`), "/panels/0/unit", "unknown unit"},
	}
	for _, c := range rejected {
		t.Run(c.name, func(t *testing.T) {
			issues := parseIssues(t, c.data)
			if len(issues) != 1 || issues[0].Path != c.path || !strings.Contains(issues[0].Message, c.contains) {
				t.Fatalf("got %+v, want one issue at %q mentioning %q", issues, c.path, c.contains)
			}
		})
	}
	accepted := []struct {
		name string
		data []byte
	}{
		{"p99 reducer", with(`"queries":`, `"display": {"reduce": "p99"}, "queries":`)},
		{"allIsNull legend value", timeSeries(`{"legendValues": ["allIsNull"]}`)},
	}
	for _, c := range accepted {
		t.Run(c.name, func(t *testing.T) {
			if _, err := spec.Parse(c.data); err != nil {
				t.Fatalf("Parse: %v", err)
			}
		})
	}
}
