package grafana

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func instantRangePanel(t *testing.T, extra string, targets ...string) (spec.Panel, PanelLine) {
	t.Helper()
	members := `"targets":[` + strings.Join(targets, ",") + `]`
	if extra != "" {
		members += "," + extra
	}
	return convertedPanel(t, panelDoc("timeseries", members))
}

func queryRefs(p spec.Panel) []string {
	refs := make([]string, len(p.Queries))
	for i := range p.Queries {
		refs[i] = p.QueryRef(i)
	}
	return refs
}

func TestInstantRangeTargetBecomesARangeAndAnInstantQuery(t *testing.T) {
	panel, line := instantRangePanel(t, "",
		`{"refId":"A","expr":"up","legendFormat":"{{job}}","instant":true,"range":true,"interval":"1m","intervalFactor":2}`)

	if got := queryRefs(panel); !slices.Equal(got, []string{"A", "B"}) {
		t.Fatalf("refs = %v, want [A B]", got)
	}
	rangeQuery, instantQuery := panel.Queries[0], panel.Queries[1]
	if rangeQuery.Instant || rangeQuery.Interval != "1m" || rangeQuery.IntervalFactor != 2 {
		t.Fatalf("range query = %+v, want the range query to keep interval and factor", rangeQuery)
	}
	if !instantQuery.Instant || instantQuery.Query != "up" || instantQuery.Legend != "{{job}}" || instantQuery.Interval != "" || instantQuery.IntervalFactor != 0 {
		t.Fatalf("instant query = %+v, want an instant copy with the same text and legend and no interval or factor", instantQuery)
	}
	if line.Status != statusImported || len(line.Paths) != 0 {
		t.Fatalf("status = %s, paths %v, want imported", line.Status, line.Paths)
	}
	if got := line.Queries[0].Rewrites; !slices.Equal(got, []string{"refId A (instant) -> B"}) {
		t.Fatalf("rewrites = %v", got)
	}
	if line.Queries[0].Ref == nil || *line.Queries[0].Ref != "A" {
		t.Fatalf("query line ref = %v, want A", line.Queries[0].Ref)
	}
}

func TestInstantWithoutRangeStaysOneInstantQuery(t *testing.T) {
	cases := []struct{ name, target string }{
		{"range absent", `{"refId":"A","expr":"up","instant":true}`},
		{"range off", `{"refId":"A","expr":"up","instant":true,"range":false}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := instantRangePanel(t, "", tc.target)
			if len(panel.Queries) != 1 || !panel.Queries[0].Instant || line.Status != statusImported {
				t.Fatalf("queries = %+v, status %s, want one instant query", panel.Queries, line.Status)
			}
		})
	}
}

func TestInstantCopyTakesTheNextFreeRef(t *testing.T) {
	panel, line := instantRangePanel(t, "",
		`{"refId":"A","expr":"up","instant":true,"range":true}`,
		`{"refId":"B","expr":"node_load1"}`)

	if got := queryRefs(panel); !slices.Equal(got, []string{"A", "C", "B"}) {
		t.Fatalf("refs = %v, want [A C B]", got)
	}
	if !panel.Queries[1].Instant || panel.Queries[2].Query != "node_load1" {
		t.Fatalf("queries = %+v", panel.Queries)
	}
	if got := line.Queries[0].Rewrites; !slices.Equal(got, []string{"refId A (instant) -> C"}) {
		t.Fatalf("rewrites = %v", got)
	}
	if line.Queries[1].Ref == nil || *line.Queries[1].Ref != "B" {
		t.Fatalf("second query line ref = %v, want B", line.Queries[1].Ref)
	}
}

func TestHiddenInstantRangeTargetHidesBothQueries(t *testing.T) {
	panel, _ := instantRangePanel(t, "", `{"refId":"A","expr":"up","instant":true,"range":true,"hide":true}`)
	if len(panel.Queries) != 2 || !panel.Queries[0].Hide || !panel.Queries[1].Hide {
		t.Fatalf("queries = %+v, want both hidden", panel.Queries)
	}
}

func TestOverrideByFrameRefIDStylesBothResultSets(t *testing.T) {
	overrides := `"fieldConfig":{"defaults":{},"overrides":[{"matcher":{"id":"byFrameRefID","options":"A"},"properties":[{"id":"unit","value":"bytes"}]}]}`
	panel, line := instantRangePanel(t, overrides, `{"refId":"A","expr":"up","instant":true,"range":true}`)

	var matched []string
	for _, o := range panel.Overrides {
		if o.Unit != "bytes" {
			t.Fatalf("override = %+v, want the unit kept", o)
		}
		matched = append(matched, o.Match.Query)
	}
	if !slices.Equal(matched, []string{"A", "B"}) || line.Status != statusImported {
		t.Fatalf("override matches = %v, status %s, paths %v, want [A B] imported", matched, line.Status, line.Paths)
	}
}

func TestFilterByRefIDKeepsTheInstantResultToo(t *testing.T) {
	transform := `"transformations":[{"id":"filterByRefId","options":{"include":"A"}}]`
	panel, _ := instantRangePanel(t, transform,
		`{"refId":"A","expr":"up","instant":true,"range":true}`,
		`{"refId":"B","expr":"node_load1"}`)

	if len(panel.Transform) != 1 || !slices.Equal(panel.Transform[0].Include, []string{"A", "C"}) {
		t.Fatalf("transform = %+v, want include [A C]", panel.Transform)
	}
}

func TestPanelAtTheQueryLimitKeepsOnlyTheRangeQuery(t *testing.T) {
	targets := []string{`{"refId":"A","expr":"up","instant":true,"range":true}`}
	for i := 1; i < maxPanelQueries; i++ {
		targets = append(targets, fmt.Sprintf(`{"refId":"Q%d","expr":"up"}`, i))
	}
	panel, line := instantRangePanel(t, "", targets...)

	if len(panel.Queries) != maxPanelQueries || panel.Queries[0].Instant {
		t.Fatalf("%d queries, first %+v, want the range query only", len(panel.Queries), panel.Queries[0])
	}
	if line.Status != statusPartial || !slices.Contains(line.Paths, "targets[0].instant") {
		t.Fatalf("status = %s, paths %v, want partial at targets[0].instant", line.Status, line.Paths)
	}
}
