package spec_test

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const sourcePanelJSON = `{"id":"src","type":"timeseries","title":"Source","gridPos":{"x":0,"y":0,"w":6,"h":4},"datasource":"prom","queries":[{"query":"up"}]}`

func reusePanel(id string, y int, extra, queries string) string {
	if queries == "" {
		queries = `{"source":{"panel":"src"}}`
	}
	return `{"id":"` + id + `","type":"table","title":"Reuse","gridPos":{"x":6,"y":` + strconv.Itoa(y) + `,"w":6,"h":4}` + extra + `,"queries":[` + queries + `]}`
}

func TestDashboardSourceParses(t *testing.T) {
	cases := []struct {
		name   string
		panels string
		want   spec.DashboardSource
	}{
		{"source panel only", sourcePanelJSON + "," + reusePanel("dep", 0, "", ""), spec.DashboardSource{Panel: "src"}},
		{"with transforms", sourcePanelJSON + "," + reusePanel("dep", 0, "", `{"ref":"R","source":{"panel":"src","withTransforms":true}}`), spec.DashboardSource{Panel: "src", WithTransforms: true}},
		{"with ad hoc filters", sourcePanelJSON + "," + reusePanel("dep", 0, "", `{"ref":"R","source":{"panel":"src","adHocFilters":true}}`), spec.DashboardSource{Panel: "src", AdHocFilters: true}},
		{"hidden", sourcePanelJSON + "," + reusePanel("dep", 0, "", `{"hide":true,"source":{"panel":"src"}}`), spec.DashboardSource{Panel: "src"}},
		{"source after the panel that reuses it", reusePanel("dep", 0, "", "") + "," + sourcePanelJSON, spec.DashboardSource{Panel: "src"}},
		{"transforms of its own", sourcePanelJSON + "," + reusePanel("dep", 0, `,"transform":[{"type":"sortBy","field":"Time"}]`, ""), spec.DashboardSource{Panel: "src"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, err := spec.Parse(dsDashboard("", tc.panels))
			if err != nil {
				t.Fatalf("Parse error = %v, want none", err)
			}
			dep, ok := d.FindPanel("dep")
			if !ok {
				t.Fatal("dependent panel missing")
			}
			got, isSource := dep.Panel.DashboardSource()
			if !isSource || got != tc.want {
				t.Fatalf("DashboardSource() = %+v, %v; want %+v, true", got, isSource, tc.want)
			}
			if src, _ := d.FindPanel("src"); src.Panel.Queries[0].IsDashboardSource() {
				t.Fatal("the source panel reports a dashboard source")
			}
		})
	}
}

func rowOf(id string, repeat string, y int, children ...string) string {
	head := `{"id":"` + id + `","type":"row","title":"Row"`
	if repeat != "" {
		head += `,"repeat":"` + repeat + `"`
	}
	return head + `,"gridPos":{"x":0,"y":` + strconv.Itoa(y) + `,"w":24,"h":1},"panels":[` + strings.Join(children, ",") + `]}`
}

func TestDashboardSourceAcrossRows(t *testing.T) {
	source := `{"id":"src","type":"timeseries","title":"Source","gridPos":{"x":0,"y":1,"w":6,"h":4},"datasource":"prom","queries":[{"query":"up"}]}`
	dependent := `{"id":"dep","type":"table","title":"Reuse","gridPos":{"x":0,"y":21,"w":6,"h":4},"queries":[{"source":{"panel":"src"}}]}`
	d, err := spec.Parse(dsDashboard("", rowOf("r1", "", 0, source)+","+rowOf("r2", "", 20, dependent)))
	if err != nil {
		t.Fatalf("Parse error = %v, want none", err)
	}
	if _, ok := d.FindPanel("dep"); !ok {
		t.Fatal("dependent panel missing")
	}

	variable := `{"name":"env","type":"custom","options":["a","b"],"default":"a"}`
	issues := parseIssues(t, dsDashboard(variable, rowOf("r1", "env", 0, source)+","+rowOf("r2", "", 20, dependent)))
	want := spec.Issue{Path: "/panels/1/panels/0/queries/0/source/panel", Code: spec.CodeInvalidReference}
	if len(issues) != 1 || issues[0].Path != want.Path || issues[0].Code != want.Code {
		t.Fatalf("issues = %+v, want %+v", issues, want)
	}
}

func TestDashboardSourceSemanticIssues(t *testing.T) {
	variable := `{"name":"env","type":"custom","options":["a","b"],"default":"a"}`
	cases := []struct {
		name      string
		variables string
		panels    string
		want      []spec.Issue
	}{
		{
			name:   "unknown source panel",
			panels: sourcePanelJSON + "," + reusePanel("dep", 0, "", `{"source":{"panel":"nope"}}`),
			want:   []spec.Issue{{Path: "/panels/1/queries/0/source/panel", Code: spec.CodeInvalidReference}},
		},
		{
			name:   "source is a text panel",
			panels: `{"id":"src","type":"text","title":"Notes","gridPos":{"x":0,"y":0,"w":6,"h":4},"content":"hi"},` + reusePanel("dep", 0, "", ""),
			want:   []spec.Issue{{Path: "/panels/1/queries/0/source/panel", Code: spec.CodeInvalidReference}},
		},
		{
			name:   "source is a row",
			panels: `{"id":"src","type":"row","title":"Row","gridPos":{"x":0,"y":0,"w":24,"h":1}},` + reusePanel("dep", 1, "", ""),
			want:   []spec.Issue{{Path: "/panels/1/queries/0/source/panel", Code: spec.CodeInvalidReference}},
		},
		{
			name:   "a panel cannot reuse itself",
			panels: sourcePanelJSON + "," + reusePanel("dep", 0, "", `{"source":{"panel":"dep"}}`),
			want:   []spec.Issue{{Path: "/panels/1/queries/0/source/panel", Code: spec.CodeInvalidReference}},
		},
		{
			name:   "no chaining",
			panels: sourcePanelJSON + "," + reusePanel("mid", 0, "", "") + "," + reusePanel("end", 4, "", `{"source":{"panel":"mid"}}`),
			want:   []spec.Issue{{Path: "/panels/2/queries/0/source/panel", Code: spec.CodeInvalidReference}},
		},
		{
			name:   "two panels reusing each other",
			panels: reusePanel("a", 0, "", `{"source":{"panel":"b"}}`) + "," + reusePanel("b", 4, "", `{"source":{"panel":"a"}}`),
			want: []spec.Issue{
				{Path: "/panels/0/queries/0/source/panel", Code: spec.CodeInvalidReference},
				{Path: "/panels/1/queries/0/source/panel", Code: spec.CodeInvalidReference},
			},
		},
		{
			name:      "a source that repeats",
			variables: variable,
			panels:    `{"id":"src","type":"timeseries","title":"Source","repeat":"env","gridPos":{"x":0,"y":0,"w":6,"h":4},"datasource":"prom","queries":[{"query":"up"}]},` + reusePanel("dep", 0, "", ""),
			want:      []spec.Issue{{Path: "/panels/1/queries/0/source/panel", Code: spec.CodeInvalidReference}},
		},
		{
			name:   "another query next to the source",
			panels: sourcePanelJSON + "," + reusePanel("dep", 0, `,"datasource":"prom"`, `{"source":{"panel":"src"}},{"ref":"B","query":"up"}`),
			want: []spec.Issue{
				{Path: "/panels/1/queries/0", Code: spec.CodeInvalidReference},
				{Path: "/panels/1/datasource", Code: spec.CodeInvalidReference},
			},
		},
		{
			name:   "own datasource",
			panels: sourcePanelJSON + "," + reusePanel("dep", 0, `,"datasource":"prom"`, ""),
			want:   []spec.Issue{{Path: "/panels/1/datasource", Code: spec.CodeInvalidReference}},
		},
		{
			name:   "own interval, time override and maxDataPoints",
			panels: sourcePanelJSON + "," + reusePanel("dep", 0, `,"interval":"1m","timeFrom":"now-1h","timeShift":"1h","maxDataPoints":100`, ""),
			want: []spec.Issue{
				{Path: "/panels/1/interval", Code: spec.CodeInvalidReference},
				{Path: "/panels/1/timeFrom", Code: spec.CodeInvalidReference},
				{Path: "/panels/1/timeShift", Code: spec.CodeInvalidReference},
				{Path: "/panels/1/maxDataPoints", Code: spec.CodeInvalidReference},
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

func TestOverridesOfAReusingPanelMatchTheSourcesRefs(t *testing.T) {
	source := `{"id":"src","type":"timeseries","title":"Source","gridPos":{"x":0,"y":0,"w":6,"h":4},"datasource":"prom","queries":[{"ref":"S","query":"up"}]}`
	withOverride := func(ref string) []byte {
		dependent := reusePanel("dep", 0, `,"overrides":[{"match":{"query":"`+ref+`"},"color":"critical"}]`, `{"ref":"R","source":{"panel":"src"}}`)
		return dsDashboard("", source+","+dependent)
	}

	if _, err := spec.Parse(withOverride("S")); err != nil {
		t.Fatalf("an override on the source's ref: Parse error = %v, want none", err)
	}

	issues := parseIssues(t, withOverride("R"))
	want := spec.Issue{Path: "/panels/1/overrides/0/match/query", Code: spec.CodeUnknownQueryRef}
	if len(issues) != 1 || issues[0].Path != want.Path || issues[0].Code != want.Code {
		t.Fatalf("an override on the reusing query's own ref: issues = %+v, want %+v", issues, want)
	}
}

func TestDashboardSourceSchemaRejections(t *testing.T) {
	cases := []struct {
		name   string
		panels string
		path   string
	}{
		{"extra field on the source", sourcePanelJSON + "," + reusePanel("dep", 0, "", `{"source":{"panel":"src","extra":1}}`), "/panels/1/queries/0/source"},
		{"extra field on the query", sourcePanelJSON + "," + reusePanel("dep", 0, "", `{"source":{"panel":"src"},"legend":"x"}`), "/panels/1/queries/0"},
		{"source without a panel", sourcePanelJSON + "," + reusePanel("dep", 0, "", `{"source":{"withTransforms":true}}`), "/panels/1/queries/0/source"},
		{"panel id with a bad shape", sourcePanelJSON + "," + reusePanel("dep", 0, "", `{"source":{"panel":"1 bad"}}`), "/panels/1/queries/0/source/panel"},
		{"transforms that is not a boolean", sourcePanelJSON + "," + reusePanel("dep", 0, "", `{"source":{"panel":"src","withTransforms":"yes"}}`), "/panels/1/queries/0/source/withTransforms"},
		{"query text next to the source", sourcePanelJSON + "," + reusePanel("dep", 0, "", `{"query":"up","source":{"panel":"src"}}`), "/panels/1/queries/0"},
		{"expression next to the source", sourcePanelJSON + "," + reusePanel("dep", 0, "", `{"ref":"E","source":{"panel":"src"},"expression":{"type":"math","math":"1"}}`), "/panels/1/queries/0"},
		{"a datasource panel without a datasource", reusePanel("dep", 0, "", `{"query":"up"}`) + "," + sourcePanelJSON, "/panels/0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			issues := parseIssues(t, dsDashboard("", tc.panels))
			if !slices.ContainsFunc(issues, func(is spec.Issue) bool { return is.Code == spec.CodeSchema && is.Path == tc.path }) {
				t.Fatalf("issues = %+v, want a schema issue at %s", issues, tc.path)
			}
		})
	}
}
