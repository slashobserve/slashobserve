package spec_test

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestApplyVariableRegex(t *testing.T) {
	cases := []struct {
		name   string
		regex  string
		values []string
		want   []string
	}{
		{"plain pattern is anchored", "web-.*", []string{"web-1", "api-web-1", "web-2"}, []string{"web-1", "web-2"}},
		{"no group keeps the value", "/web-.*/", []string{"web-1", "api-1"}, []string{"web-1"}},
		{"first capture group replaces the value", "/^web-(.*)$/", []string{"web-1", "api-1", "web-2"}, []string{"1", "2"}},
		{"named value group wins over the first group", "/^(\\w+)-(?<value>\\d+)$/", []string{"web-1", "api-2"}, []string{"1", "2"}},
		{"named text group is the fallback", "/^(\\w+)-(?<text>\\d+)$/", []string{"web-1"}, []string{"1"}},
		{"duplicates keep the first", "/^(\\w+)-\\d+$/", []string{"web-1", "web-2", "api-1"}, []string{"web", "api"}},
		{"case insensitive flag", "/WEB/i", []string{"web", "api"}, []string{"web"}},
		{"unanchored slash pattern", "/eb/", []string{"web", "api"}, []string{"web"}},
		{"the sticky flag is accepted and ignored", "/web/y", []string{"web", "api"}, []string{"web"}},
		{"global flag takes every match", "/(\\d+)/g", []string{"a1b22", "c3"}, []string{"1", "22", "3"}},
		{"no global flag takes the first match", "/(\\d+)/", []string{"a1b22", "c3"}, []string{"1", "3"}},
		{"multiline flag anchors lines", "/^b$/m", []string{"a\nb\nc", "bc"}, []string{"a\nb\nc"}},
		{"dot all flag", "/a.b/s", []string{"a\nb", "ab"}, []string{"a\nb"}},
		{"an unmatched named group adds nothing", "/(?<value>a)|b/", []string{"a", "b"}, []string{"a"}},
		{"an unmatched first group adds nothing", "/(a)|b/", []string{"a", "b"}, []string{"a"}},
		{"a matched empty group still counts", "/^(a*)b$/", []string{"b", "aab"}, []string{"", "aa"}},
		{"nothing matches", "nope", []string{"a"}, []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			re, err := spec.CompileVariableRegex(tc.regex)
			if err != nil {
				t.Fatal(err)
			}
			if got := spec.ApplyVariableRegex(re, tc.values); !slices.Equal(got, tc.want) {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCompileVariableRegexRejects(t *testing.T) {
	for _, in := range []string{"/a(/", "a(", "/a", "/", "/a/x", "/(?=a)b/", "[a-"} {
		if _, err := spec.CompileVariableRegex(in); err == nil {
			t.Errorf("CompileVariableRegex(%q) compiled, want an error", in)
		}
	}
}

func TestSortVariableValues(t *testing.T) {
	cases := []struct {
		order  string
		values []string
		want   []string
	}{
		{"alpha", []string{"b", "C", "a"}, []string{"C", "a", "b"}},
		{"alphaDesc", []string{"b", "C", "a"}, []string{"b", "a", "C"}},
		{"alpha", []string{"\uff5e", "\U0001F600", "a"}, []string{"a", "\U0001F600", "\uff5e"}},
		{"alpha", []string{"ab", "a", "abc", ""}, []string{"", "a", "ab", "abc"}},
		{"alpha", []string{"\U0001F601", "\U0001F600", "\U0001F600a"}, []string{"\U0001F600", "\U0001F600a", "\U0001F601"}},
		{"alphaDesc", []string{"a", "ab"}, []string{"ab", "a"}},
		{"alphaDesc", []string{"a", "\U0001F600", "\uff5e"}, []string{"\uff5e", "\U0001F600", "a"}},
		{"num", []string{"10", "9", "x", "2.5", "a"}, []string{"x", "a", "2.5", "9", "10"}},
		{"numDesc", []string{"10", "9", "x", "2.5", "a"}, []string{"10", "9", "2.5", "a", "x"}},
		{"num", []string{"pod-10", "pod-9", "node"}, []string{"node", "pod-9", "pod-10"}},
		{"numDesc", []string{"pod-10", "pod-9", "node", "app"}, []string{"pod-10", "pod-9", "app", "node"}},
		{"numDesc", []string{"b1", "a1", "c0"}, []string{"a1", "b1", "c0"}},
		{"num", []string{"b1", "a1", "c0"}, []string{"c0", "b1", "a1"}},
		{"num", []string{"9" + strings.Repeat("9", 400), "5", "x"}, []string{"x", "5", "9" + strings.Repeat("9", 400)}},
		{"numDesc", []string{"5", "9" + strings.Repeat("9", 400), "x"}, []string{"9" + strings.Repeat("9", 400), "5", "x"}},
		{"num", []string{"v1.10", "v1.9"}, []string{"v1.10", "v1.9"}},
		{"numDesc", []string{"v1.10", "v1.9"}, []string{"v1.9", "v1.10"}},
		{"num", []string{"-3", "2", "x"}, []string{"x", "2", "-3"}},
		{"", []string{"b", "a"}, []string{"b", "a"}},
	}
	for _, tc := range cases {
		input := slices.Clone(tc.values)
		if got := spec.SortVariableValues(input, tc.order); !slices.Equal(got, tc.want) {
			t.Errorf("sort %q of %q = %q, want %q", tc.order, tc.values, got, tc.want)
		}
		if !slices.Equal(input, tc.values) {
			t.Errorf("sort %q changed its input", tc.order)
		}
	}
}

func TestSortVariableValuesSharedCases(t *testing.T) {
	var cases []struct {
		Order  string   `json:"order"`
		Values []string `json:"values"`
		Want   []string `json:"want"`
	}
	if err := json.Unmarshal(readExample(t, "variables/sort-cases.json"), &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 15 {
		t.Fatalf("%d cases, want at least 15", len(cases))
	}
	for _, tc := range cases {
		input := slices.Clone(tc.Values)
		if got := spec.SortVariableValues(input, tc.Order); !slices.Equal(got, tc.Want) {
			t.Errorf("sort %q of %q = %q, want %q", tc.Order, tc.Values, got, tc.Want)
		}
		if !slices.Equal(input, tc.Values) {
			t.Errorf("sort %q changed its input", tc.Order)
		}
	}
}

func TestSortVariableValuesGivesOneOrderForEveryInputOrder(t *testing.T) {
	values := []string{"b", "A", "a", "B", "007", "7", "08", "a10", "a9"}
	for _, order := range []string{"alphaCaseInsensitive", "alphaCaseInsensitiveDesc", "natural", "naturalDesc"} {
		want := spec.SortVariableValues(values, order)
		reversed := slices.Clone(values)
		slices.Reverse(reversed)
		if got := spec.SortVariableValues(reversed, order); !slices.Equal(got, want) {
			t.Errorf("sort %q of the reversed input = %q, want %q", order, got, want)
		}
	}
}

func TestVariableOptionsExample(t *testing.T) {
	d, err := spec.Parse(readExample(t, "variables/options.json"))
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]spec.Variable{}
	for _, v := range d.Variables {
		byName[v.Name] = v
	}
	if !byName["pod"].AllowCustomValue || byName["pod"].Sort != "natural" {
		t.Errorf("pod = %+v, want natural sort and allowCustomValue", byName["pod"])
	}
	if byName["job"].AllowCustomValue || byName["job"].Sort != "alphaCaseInsensitiveDesc" {
		t.Errorf("job = %+v, want alphaCaseInsensitiveDesc and no allowCustomValue", byName["job"])
	}
	if !byName["env"].AllowCustomValue {
		t.Errorf("env = %+v, want allowCustomValue", byName["env"])
	}
	want := []spec.AdhocFilter{
		{Key: "namespace", Operator: "=", Value: "checkout"},
		{Key: "pod", Operator: "!~", Value: "canary-.*"},
	}
	if got := byName["filters"].Default; got == nil || !slices.Equal(got.Filters, want) {
		t.Errorf("filters default = %+v, want %+v", got, want)
	}
}

func TestAdhocDefaultSchema(t *testing.T) {
	minimal := string(readExample(t, "minimal.json"))
	with := func(variable string) []byte {
		return []byte(strings.Replace(minimal, `"panels"`, fmt.Sprintf(`"variables": [%s], "panels"`, variable), 1))
	}
	filter := func(op string) string {
		return fmt.Sprintf(`{"key":"job","operator":%q,"value":"api"}`, op)
	}
	cases := []struct {
		name     string
		variable string
		wantPath string
	}{
		{"every accepted operator", `{"name":"f","type":"adhoc","datasource":"prom","default":[` + strings.Join([]string{filter("="), filter("!="), filter("=~"), filter("!~")}, ",") + `]}`, ""},
		{"empty value", `{"name":"f","type":"adhoc","datasource":"prom","default":[{"key":"job","operator":"=","value":""}]}`, ""},
		{"twenty filters", `{"name":"f","type":"adhoc","datasource":"prom","default":[` + strings.TrimSuffix(strings.Repeat(filter("=")+",", 20), ",") + `]}`, ""},
		{"twenty-one filters", `{"name":"f","type":"adhoc","datasource":"prom","default":[` + strings.TrimSuffix(strings.Repeat(filter("=")+",", 21), ",") + `]}`, "/variables/0/default"},
		{"no filters", `{"name":"f","type":"adhoc","datasource":"prom","default":[]}`, "/variables/0/default"},
		{"less than", `{"name":"f","type":"adhoc","datasource":"prom","default":[` + filter("<") + `]}`, "/variables/0/default/0/operator"},
		{"empty key", `{"name":"f","type":"adhoc","datasource":"prom","default":[{"key":"","operator":"=","value":"x"}]}`, "/variables/0/default/0/key"},
		{"value over 500 characters", `{"name":"f","type":"adhoc","datasource":"prom","default":[{"key":"job","operator":"=","value":"` + strings.Repeat("x", 501) + `"}]}`, "/variables/0/default/0/value"},
		{"unknown filter field", `{"name":"f","type":"adhoc","datasource":"prom","default":[{"key":"job","operator":"=","value":"x","extra":1}]}`, "/variables/0/default/0"},
		{"string default", `{"name":"f","type":"adhoc","datasource":"prom","default":"job|=|x"}`, "/variables/0/default"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := with(tc.variable)
			if tc.wantPath == "" {
				d, err := spec.Parse(data)
				if err != nil {
					t.Fatalf("Parse: %v", err)
				}
				out, err := json.Marshal(d)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := spec.Parse(out); err != nil {
					t.Fatalf("reparse marshaled result: %v", err)
				}
				return
			}
			issues := parseIssues(t, data)
			if !slices.ContainsFunc(issues, func(i spec.Issue) bool { return i.Path == tc.wantPath }) {
				t.Fatalf("issues = %+v, want one at %q", issues, tc.wantPath)
			}
		})
	}
}

func TestAllowCustomValueSchema(t *testing.T) {
	minimal := string(readExample(t, "minimal.json"))
	with := func(variable string) []byte {
		return []byte(strings.Replace(minimal, `"panels"`, fmt.Sprintf(`"variables": [%s], "panels"`, variable), 1))
	}
	cases := []struct {
		name     string
		variable string
		wantPath string
	}{
		{"query", `{"name":"v","type":"query","datasource":"prom","query":"label_values(up, job)","allowCustomValue":true}`, ""},
		{"custom", `{"name":"v","type":"custom","options":["a"],"allowCustomValue":true}`, ""},
		{"not a boolean", `{"name":"v","type":"custom","options":["a"],"allowCustomValue":"yes"}`, "/variables/0/allowCustomValue"},
		{"interval has none", `{"name":"v","type":"interval","options":["1m"],"allowCustomValue":true}`, "/variables/0"},
		{"textbox has none", `{"name":"v","type":"textbox","allowCustomValue":true}`, "/variables/0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.wantPath == "" {
				if _, err := spec.Parse(with(tc.variable)); err != nil {
					t.Fatalf("Parse: %v", err)
				}
				return
			}
			issues := parseIssues(t, with(tc.variable))
			if !slices.ContainsFunc(issues, func(i spec.Issue) bool { return i.Path == tc.wantPath }) {
				t.Fatalf("issues = %+v, want one at %q", issues, tc.wantPath)
			}
		})
	}
}

func TestVariableDefinitionIssues(t *testing.T) {
	minimal := string(readExample(t, "minimal.json"))
	with := func(variables string) []byte {
		return []byte(strings.Replace(minimal, `"panels"`, fmt.Sprintf(`"variables": [%s], "panels"`, variables), 1))
	}
	cases := []struct {
		name      string
		variables string
		code      spec.IssueCode
		path      string
	}{
		{"interval default not an option", `{"name":"iv","type":"interval","options":["1m","5m"],"default":"10m"}`, spec.CodeInvalidDefault, "/variables/0/default"},
		{"interval default an option", `{"name":"iv","type":"interval","options":["1m","5m"],"default":"5m"}`, "", ""},
		{"all default with includeAll", `{"name":"v","type":"custom","options":["a"],"includeAll":true,"default":"$__all"}`, "", ""},
		{"multi array default", `{"name":"v","type":"custom","options":["a","b"],"multi":true,"default":["a","b"]}`, "", ""},
		{"query default is free", `{"name":"v","type":"query","datasource":"prom","query":"label_values(up, job)","default":"anything"}`, "", ""},
		{"query array default without multi", `{"name":"v","type":"query","datasource":"prom","query":"label_values(up, job)","default":["a"]}`, spec.CodeInvalidDefault, "/variables/0/default"},
		{"datasource regex does not compile", `{"name":"ds","type":"datasource","datasourceType":"prometheus","regex":"/a(/"}`, spec.CodeInvalidRegex, "/variables/0/regex"},
		{"adhoc from a variable query", `{"name":"f","type":"adhoc","datasource":"prom"},{"name":"v","type":"query","datasource":"prom","query":"label_values(up{x=\"$f\"}, job)"}`, spec.CodeInvalidReference, "/variables/1/query"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := spec.Parse(with(tc.variables))
			if tc.code == "" {
				if err != nil {
					t.Fatalf("Parse: %v", err)
				}
				return
			}
			issues := parseIssues(t, with(tc.variables))
			if len(issues) != 1 || issues[0].Code != tc.code || issues[0].Path != tc.path {
				t.Fatalf("issues = %+v, want one %s at %q", issues, tc.code, tc.path)
			}
		})
	}
}
