package spec_test

import (
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const resolutionVariable = `{"name":"resolution","type":"interval","options":["15s","1m"]}`

func intervalPanel(panelInterval, queryExtras string) string {
	head := statPanelHead + `,"datasource":"prom"`
	if panelInterval != "" {
		head += `,"interval":"` + panelInterval + `"`
	}
	return head + `,"queries":[{"query":"up"` + queryExtras + `}]}`
}

func intervalRow(child string) string {
	return `{"id":"r1","type":"row","title":"Row","gridPos":{"x":0,"y":0,"w":24,"h":1},"panels":[` +
		`{"id":"c1","type":"stat","title":"Up","gridPos":{"x":0,"y":1,"w":6,"h":4},"datasource":"prom",` + child + `}]}`
}

func TestIntervalVariableName(t *testing.T) {
	cases := []struct {
		ref      string
		wantName string
		wantOK   bool
	}{
		{"${resolution}", "resolution", true},
		{"${Step_2}", "Step_2", true},
		{"30s", "", false},
		{"$resolution", "", false},
		{"${resolution:text}", "", false},
		{"${1step}", "", false},
		{"${}", "", false},
		{"", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.ref, func(t *testing.T) {
			name, ok := spec.IntervalVariableName(tc.ref)
			if name != tc.wantName || ok != tc.wantOK {
				t.Fatalf("IntervalVariableName(%q) = %q, %v; want %q, %v", tc.ref, name, ok, tc.wantName, tc.wantOK)
			}
		})
	}
}

func TestIntervalRefAcceptedWhereAllowed(t *testing.T) {
	cases := []struct {
		name   string
		panels string
	}{
		{"panel", intervalPanel("${resolution}", "")},
		{"query", intervalPanel("", `,"interval":"${resolution}"`)},
		{"query with a factor", intervalPanel("", `,"interval":"${resolution}","intervalFactor":2`)},
		{"literal duration with a factor", intervalPanel("", `,"interval":"30s","intervalFactor":10`)},
		{"row child", intervalRow(`"interval":"${resolution}","queries":[{"query":"up"}]`)},
		{"row child query", intervalRow(`"queries":[{"query":"up","interval":"${resolution}"}]`)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := spec.Parse(dsDashboard(resolutionVariable, tc.panels)); err != nil {
				t.Fatalf("Parse error = %v, want none", err)
			}
		})
	}
}

func TestIntervalRefSemanticIssues(t *testing.T) {
	cases := []struct {
		name      string
		variables string
		panels    string
		want      []spec.Issue
	}{
		{
			name:   "undeclared name on a panel",
			panels: intervalPanel("${resolution}", ""),
			want:   []spec.Issue{{Path: "/panels/0/interval", Code: spec.CodeUnknownVariable}},
		},
		{
			name:      "undeclared name on a query",
			variables: resolutionVariable,
			panels:    intervalPanel("", `,"interval":"${other}"`),
			want:      []spec.Issue{{Path: "/panels/0/queries/0/interval", Code: spec.CodeUnknownVariable}},
		},
		{
			name:      "custom variable named on a panel",
			variables: `{"name":"resolution","type":"custom","options":["15s"]}`,
			panels:    intervalPanel("${resolution}", ""),
			want:      []spec.Issue{{Path: "/panels/0/interval", Code: spec.CodeInvalidReference}},
		},
		{
			name:      "datasource variable named on a query",
			variables: dsVariable,
			panels:    intervalPanel("", `,"interval":"${ds}"`),
			want:      []spec.Issue{{Path: "/panels/0/queries/0/interval", Code: spec.CodeInvalidReference}},
		},
		{
			name:   "undeclared name on a row child",
			panels: intervalRow(`"queries":[{"query":"up","interval":"${resolution}"}]`),
			want:   []spec.Issue{{Path: "/panels/0/panels/0/queries/0/interval", Code: spec.CodeUnknownVariable}},
		},
		{
			name:   "panel and query both undeclared",
			panels: intervalPanel("${a}", `,"interval":"${b}"`),
			want: []spec.Issue{
				{Path: "/panels/0/interval", Code: spec.CodeUnknownVariable},
				{Path: "/panels/0/queries/0/interval", Code: spec.CodeUnknownVariable},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			issues := parseIssues(t, dsDashboard(tc.variables, tc.panels))
			got := make([]spec.Issue, len(issues))
			for i, is := range issues {
				got[i] = spec.Issue{Path: is.Path, Code: is.Code}
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("issues = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestIntervalRefSchemaRejections(t *testing.T) {
	cases := []struct {
		name     string
		panels   string
		wantPath string
	}{
		{"dollar form on a panel", intervalPanel("$resolution", ""), "/panels/0/interval"},
		{"format suffix on a query", intervalPanel("", `,"interval":"${resolution:text}"`), "/panels/0/queries/0/interval"},
		{"factor one", intervalPanel("", `,"intervalFactor":1`), "/panels/0/queries/0/intervalFactor"},
		{"factor eleven", intervalPanel("", `,"intervalFactor":11`), "/panels/0/queries/0/intervalFactor"},
		{"factor not an integer", intervalPanel("", `,"intervalFactor":2.5`), "/panels/0/queries/0/intervalFactor"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			issues := parseIssues(t, dsDashboard(resolutionVariable, tc.panels))
			if len(issues) != 1 || issues[0].Code != spec.CodeSchema || issues[0].Path != tc.wantPath {
				t.Fatalf("issues = %+v, want one %s at %s", issues, spec.CodeSchema, tc.wantPath)
			}
		})
	}
}
