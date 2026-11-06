package provision

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/specfiles"
	"github.com/slashobserve/slashobserve/internal/taste"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

var referenceDir = filepath.Join("..", "..", "deploy", "compose", "dashboards")

var referenceFolders = map[string]string{
	"Demo/shop-overview.json":     "Demo",
	"Demo/node-health.json":       "Demo",
	"SlashObserve/internals.json": "SlashObserve",
}

var referenceDatasources = []string{"prom", "ch"}

func TestReferenceDashboardsLandInTheirFolders(t *testing.T) {
	files, err := specfiles.Scan(referenceDir)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]string{}
	for _, f := range files {
		found[f.Path] = f.Folder
	}
	for path, folder := range referenceFolders {
		got, ok := found[path]
		if !ok {
			t.Errorf("%s is missing from %s", path, referenceDir)
			continue
		}
		if got != folder {
			t.Errorf("%s lands in folder %q, want %q", path, got, folder)
		}
	}
}

func TestReferenceDashboardsFollowTheLayoutRules(t *testing.T) {
	files, err := specfiles.Scan(referenceDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatalf("no dashboard file in %s", referenceDir)
	}
	dir := os.DirFS(referenceDir)
	for _, f := range files {
		t.Run(f.Path, func(t *testing.T) {
			data, err := fs.ReadFile(dir, f.Path)
			if err != nil {
				t.Fatal(err)
			}
			d, err := spec.Parse(data)
			if err != nil {
				t.Fatal(err)
			}
			for _, j := range taste.Judge(d) {
				for _, failure := range j.Failed {
					t.Errorf("%s: %s", j.Rule, failure)
				}
			}
			for _, problem := range datasourceProblems(d) {
				t.Error(problem)
			}
		})
	}
}

func datasourceProblems(d *spec.Dashboard) []string {
	var problems []string
	for _, entry := range d.AllPanels() {
		p := entry.Panel
		var datasources []string
		if p.Datasource != "" {
			datasources = append(datasources, p.Datasource)
		}
		for i := range p.Queries {
			datasources = append(datasources, p.QueryDatasource(i))
		}
		slices.Sort(datasources)
		for _, ds := range slices.Compact(datasources) {
			if !slices.Contains(referenceDatasources, ds) {
				problems = append(problems, fmt.Sprintf("%s: datasource %s is not one of %s", p.ID, ds, strings.Join(referenceDatasources, ", ")))
			}
		}
	}
	return problems
}

func TestDatasourceProblemsNamesAPanelOffTheReferenceDatasources(t *testing.T) {
	d, err := spec.Parse([]byte(`{ "specVersion": "0", "title": "Base", "time": { "from": "now-1h", "to": "now" }, "panels": [
  { "id": "p_a", "gridPos": { "x": 0, "y": 0, "w": 12, "h": 4 }, "type": "stat", "title": "A", "datasource": "prom", "queries": [{ "query": "sum(up)" }] },
  { "id": "p_b", "gridPos": { "x": 12, "y": 0, "w": 12, "h": 4 }, "type": "stat", "title": "B", "datasource": "loki", "queries": [{ "query": "sum(count_over_time({job=\"x\"}[5m]))" }] }
] }`))
	if err != nil {
		t.Fatal(err)
	}
	if got := datasourceProblems(d); !slices.Equal(got, []string{"p_b: datasource loki is not one of prom, ch"}) {
		t.Fatalf("problems = %q", got)
	}
}
