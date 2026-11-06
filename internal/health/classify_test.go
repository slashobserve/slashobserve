package health

import (
	"slices"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func healthDashboard(t *testing.T) *spec.Dashboard {
	t.Helper()
	d, err := spec.Parse([]byte(`{"specVersion":"0","title":"Checkout","time":{"from":"now-1h","to":"now"},
		"variables":[{"name":"pod","type":"query","datasource":"prom","query":"label_values(up, pod)"},{"name":"env","type":"query","datasource":"prom","query":"label_values(up, env)"}],
		"panels":[
			{"id":"rps","type":"timeseries","title":"Requests","gridPos":{"x":0,"y":0,"w":12,"h":8},"datasource":"prom","queries":[{"query":"up"},{"query":"down"}]},
			{"id":"r1","type":"row","title":"Details","gridPos":{"x":0,"y":8,"w":24,"h":1},"panels":[
				{"id":"lat","type":"timeseries","title":"Latency","gridPos":{"x":0,"y":9,"w":12,"h":8},"datasource":"prom","queries":[{"query":"lat"}]},
				{"id":"err","type":"stat","title":"Errors","gridPos":{"x":12,"y":9,"w":12,"h":8},"datasource":"prom","queries":[{"query":"err"}]}
			]},
			{"id":"p11","type":"timeseries","title":"Eleventh","gridPos":{"x":0,"y":17,"w":12,"h":8},"datasource":"prom","queries":[{"query":"x"}]}
		]}`))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestClassifyMapsEachIssueToItsPanelOrVariableAndKind(t *testing.T) {
	d := healthDashboard(t)
	cases := []struct {
		name  string
		issue spec.Issue
		want  Problem
	}{
		{"empty query is no data", spec.Issue{Path: "/panels/0/queries/0/query", Code: "query_empty", Message: "no data"},
			Problem{Kind: KindNoData, PanelID: "rps", PanelTitle: "Requests", Code: "query_empty", Message: "no data"}},
		{"failing query", spec.Issue{Path: "/panels/0/queries", Code: "query_failed", Message: "bad"},
			Problem{Kind: KindQueryFailed, PanelID: "rps", PanelTitle: "Requests", Code: "query_failed", Message: "bad"}},
		{"panel inside a row", spec.Issue{Path: "/panels/1/panels/0/queries/0/query", Code: "query_empty", Message: "m"},
			Problem{Kind: KindNoData, PanelID: "lat", PanelTitle: "Latency", Code: "query_empty", Message: "m"}},
		{"second panel inside a row", spec.Issue{Path: "/panels/1/panels/1", Code: "unknown_datasource", Message: "m"},
			Problem{Kind: KindQueryFailed, PanelID: "err", PanelTitle: "Errors", Code: "unknown_datasource", Message: "m"}},
		{"index prefix of another panel", spec.Issue{Path: "/panels/2/queries/0/query", Code: "timeout", Message: "m"},
			Problem{Kind: KindQueryFailed, PanelID: "p11", PanelTitle: "Eleventh", Code: "timeout", Message: "m"}},
		{"variable", spec.Issue{Path: "/variables/0/query", Code: "variable_empty", Message: "m"},
			Problem{Kind: KindVariableFailed, Variable: "pod", Code: "variable_empty", Message: "m"}},
		{"variable without a sub path", spec.Issue{Path: "/variables/1", Code: "variable_failed", Message: "m"},
			Problem{Kind: KindVariableFailed, Variable: "env", Code: "variable_failed", Message: "m"}},
		{"unknown variable index", spec.Issue{Path: "/variables/9", Code: "variable_failed", Message: "m"},
			Problem{Kind: KindQueryFailed, Code: "variable_failed", Message: "m"}},
		{"dashboard level pointer", spec.Issue{Path: "/time", Code: "time_range_invalid", Message: "m"},
			Problem{Kind: KindQueryFailed, Code: "time_range_invalid", Message: "m"}},
		{"unknown panel index", spec.Issue{Path: "/panels/7/queries", Code: "query_empty", Message: "m"},
			Problem{Kind: KindNoData, Code: "query_empty", Message: "m"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := classify(d, []spec.Issue{c.issue})
			if len(got) != 1 || got[0] != c.want {
				t.Errorf("classify = %+v, want %+v", got, c.want)
			}
		})
	}
}

func TestClassifyKeepsOneProblemPerPanelAndKind(t *testing.T) {
	d := healthDashboard(t)
	got := classify(d, []spec.Issue{
		{Path: "/panels/0/queries/0/query", Code: "query_empty", Message: "first"},
		{Path: "/panels/0/queries/1/query", Code: "query_empty", Message: "second"},
		{Path: "/panels/0/queries/1/query", Code: "query_failed", Message: "third"},
	})
	want := []Problem{
		{Kind: KindNoData, PanelID: "rps", PanelTitle: "Requests", Code: "query_empty", Message: "first"},
		{Kind: KindQueryFailed, PanelID: "rps", PanelTitle: "Requests", Code: "query_failed", Message: "third"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("classify = %+v, want %+v", got, want)
	}
}

func TestClassifyWithoutIssuesIsEmpty(t *testing.T) {
	if got := classify(healthDashboard(t), nil); len(got) != 0 {
		t.Errorf("classify = %+v, want none", got)
	}
}

func TestUnopenedAfterNinetyDaysSinceTheLastViewOrCreation(t *testing.T) {
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	at := func(ago time.Duration) *time.Time { v := now.Add(-ago); return &v }
	cases := []struct {
		name     string
		lastView *time.Time
		created  time.Time
		want     bool
	}{
		{"never opened and created 91 days ago", nil, now.Add(-91 * day), true},
		{"never opened and created 89 days ago", nil, now.Add(-89 * day), false},
		{"never opened and created exactly 90 days ago", nil, now.Add(-90 * day), false},
		{"opened 91 days ago", at(91 * day), now.Add(-400 * day), true},
		{"opened yesterday though created long ago", at(day), now.Add(-400 * day), false},
	}
	for _, c := range cases {
		if got := unopened(c.lastView, c.created, now); got != c.want {
			t.Errorf("%s: unopened = %v, want %v", c.name, got, c.want)
		}
	}
}
