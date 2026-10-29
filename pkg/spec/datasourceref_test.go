package spec_test

import (
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	dsVariable    = `{"name":"ds","type":"datasource","datasourceType":"prometheus"}`
	statPanelHead = `{"id":"p1","type":"stat","title":"Up","gridPos":{"x":0,"y":0,"w":6,"h":4}`
)

func dsPanel(datasource, queryDatasource string) string {
	query := `{"query":"up"}`
	if queryDatasource != "" {
		query = `{"query":"up","datasource":"` + queryDatasource + `"}`
	}
	return statPanelHead + `,"datasource":"` + datasource + `","queries":[` + query + `]}`
}

func dsDashboard(variables, panels string) []byte {
	doc := `{"specVersion":"0","title":"Datasource refs","time":{"from":"now-15m","to":"now"},`
	if variables != "" {
		doc += `"variables":[` + variables + `],`
	}
	return []byte(doc + `"panels":[` + panels + `]}`)
}

func TestDatasourceVariableName(t *testing.T) {
	cases := []struct {
		ref      string
		wantName string
		wantOK   bool
	}{
		{"${ds}", "ds", true},
		{"${Prom_2}", "Prom_2", true},
		{"prom", "", false},
		{"$ds", "", false},
		{"${ds:text}", "", false},
		{"${1ds}", "", false},
		{"${}", "", false},
		{"${ds", "", false},
		{" ${ds}", "", false},
		{"", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.ref, func(t *testing.T) {
			name, ok := spec.DatasourceVariableName(tc.ref)
			if name != tc.wantName || ok != tc.wantOK {
				t.Fatalf("DatasourceVariableName(%q) = %q, %v; want %q, %v", tc.ref, name, ok, tc.wantName, tc.wantOK)
			}
		})
	}
}

func TestDatasourceRefAcceptedWhereAllowed(t *testing.T) {
	cases := []struct {
		name      string
		variables string
		panels    string
	}{
		{"panel", dsVariable, dsPanel("${ds}", "")},
		{"query override", dsVariable, dsPanel("prom", "${ds}")},
		{
			"query variable after the datasource variable",
			dsVariable + `,{"name":"job","type":"query","datasource":"${ds}","query":"label_values(up, job)"}`,
			dsPanel("prom", ""),
		},
		{
			"adhoc variable after the datasource variable",
			dsVariable + `,{"name":"filters","type":"adhoc","datasource":"${ds}"}`,
			dsPanel("prom", ""),
		},
		{
			"row child",
			dsVariable,
			`{"id":"r1","type":"row","title":"Row","gridPos":{"x":0,"y":0,"w":24,"h":1},"panels":[` +
				`{"id":"c1","type":"stat","title":"Up","gridPos":{"x":0,"y":1,"w":6,"h":4},"datasource":"${ds}","queries":[{"query":"up"}]}]}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := spec.Parse(dsDashboard(tc.variables, tc.panels)); err != nil {
				t.Fatalf("Parse error = %v, want none", err)
			}
		})
	}
}

func TestDatasourceRefSemanticIssues(t *testing.T) {
	cases := []struct {
		name      string
		variables string
		panels    string
		want      []spec.Issue
	}{
		{
			name:   "undeclared name on a panel",
			panels: dsPanel("${ds}", ""),
			want:   []spec.Issue{{Path: "/panels/0/datasource", Code: spec.CodeUnknownVariable}},
		},
		{
			name:      "undeclared name on a query override",
			variables: dsVariable,
			panels:    dsPanel("prom", "${other}"),
			want:      []spec.Issue{{Path: "/panels/0/queries/0/datasource", Code: spec.CodeUnknownVariable}},
		},
		{
			name: "undeclared name on a row child",
			panels: `{"id":"r1","type":"row","title":"Row","gridPos":{"x":0,"y":0,"w":24,"h":1},"panels":[` +
				`{"id":"c1","type":"stat","title":"Up","gridPos":{"x":0,"y":1,"w":6,"h":4},"datasource":"${ds}","queries":[{"query":"up"}]}]}`,
			want: []spec.Issue{{Path: "/panels/0/panels/0/datasource", Code: spec.CodeUnknownVariable}},
		},
		{
			name:      "custom variable named",
			variables: `{"name":"env","type":"custom","options":["prod"]}`,
			panels:    dsPanel("${env}", ""),
			want:      []spec.Issue{{Path: "/panels/0/datasource", Code: spec.CodeInvalidReference}},
		},
		{
			name:      "query variable before the datasource variable",
			variables: `{"name":"job","type":"query","datasource":"${ds}","query":"label_values(up, job)"},` + dsVariable,
			panels:    dsPanel("prom", ""),
			want:      []spec.Issue{{Path: "/variables/0/datasource", Code: spec.CodeVariableOrder}},
		},
		{
			name:      "adhoc variable before the datasource variable",
			variables: `{"name":"filters","type":"adhoc","datasource":"${ds}"},` + dsVariable,
			panels:    dsPanel("prom", ""),
			want:      []spec.Issue{{Path: "/variables/0/datasource", Code: spec.CodeVariableOrder}},
		},
		{
			name:      "query variable naming itself",
			variables: `{"name":"self","type":"query","datasource":"${self}","query":"label_values(up, job)"}`,
			panels:    dsPanel("prom", ""),
			want:      []spec.Issue{{Path: "/variables/0/datasource", Code: spec.CodeInvalidReference}},
		},
		{
			name:      "panel and query override both undeclared",
			variables: dsVariable,
			panels:    dsPanel("${a}", "${b}"),
			want: []spec.Issue{
				{Path: "/panels/0/datasource", Code: spec.CodeUnknownVariable},
				{Path: "/panels/0/queries/0/datasource", Code: spec.CodeUnknownVariable},
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

func TestDatasourceRefSchemaRejections(t *testing.T) {
	cases := []struct {
		name      string
		variables string
		panels    string
		wantPath  string
	}{
		{"format suffix on a panel", dsVariable, dsPanel("${ds:text}", ""), "/panels/0/datasource"},
		{"dollar form on a panel", dsVariable, dsPanel("$ds", ""), "/panels/0/datasource"},
		{"unclosed brace on a panel", dsVariable, dsPanel("${ds", ""), "/panels/0/datasource"},
		{
			"reference as a datasource variable default",
			`{"name":"ds","type":"datasource","datasourceType":"prometheus","default":"${ds}"}`,
			dsPanel("prom", ""),
			"/variables/0/default",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			issues := parseIssues(t, dsDashboard(tc.variables, tc.panels))
			if len(issues) != 1 || issues[0].Code != spec.CodeSchema || issues[0].Path != tc.wantPath {
				t.Fatalf("issues = %+v, want one %s at %s", issues, spec.CodeSchema, tc.wantPath)
			}
		})
	}
}
