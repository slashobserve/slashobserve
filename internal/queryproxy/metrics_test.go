package queryproxy

import (
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

type countedQueries struct{ types []string }

func (c *countedQueries) ProxyQuery(datasourceType, _ string, _ time.Duration) {
	c.types = append(c.types, datasourceType)
}
func (c *countedQueries) ProxyCache(bool) {}

func TestQueriesOnTheBuiltInDatasourceStayOffTheOpenMetrics(t *testing.T) {
	reg, err := datasource.NewRegistry(slog.New(slog.DiscardHandler), datasource.Instance{UID: "prom", Datasource: newFake("prometheus")})
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.AddBuiltIn(datasource.Instance{UID: "audit", Datasource: newFake("opensearch")}); err != nil {
		t.Fatal(err)
	}
	counted := &countedQueries{}
	runner := NewRunner(reg, discardLogger(), WithMetrics(counted))
	d := dashboardOf([]spec.Panel{panelOf("a", spec.Table, "audit", "outcome:denied"), panelOf("p", spec.Table, "prom", "up")})
	for _, id := range []string{"a", "p"} {
		if _, err := runner.RunPanel(reading(t.Context()), Request{Dashboard: d, PanelID: id, Range: testRange()}); err != nil {
			t.Fatalf("RunPanel(%s): %v", id, err)
		}
	}
	if !slices.Equal(counted.types, []string{"prometheus"}) {
		t.Fatalf("counted query types = %v, want only the prometheus one", counted.types)
	}
}
