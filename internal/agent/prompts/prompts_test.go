package prompts_test

import (
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/prompts"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

var promptFiles = os.DirFS(".")

var sectionHeadings = map[spec.DatasourceType]string{
	spec.Prometheus: "## Prometheus (PromQL)",
	spec.ClickHouse: "## ClickHouse (OpenTelemetry exporter tables)",
	spec.Loki:       "## Loki (LogQL)",
	spec.Tempo:      "## Tempo (TraceQL)",
	spec.Jaeger:     "## Jaeger (search keys)",

	spec.Elasticsearch: searchHeading,
}

const (
	searchHeading = "## Elasticsearch and OpenSearch (KQL, Lucene, Query DSL)"
	auditHeading  = "## The audit trail (`slashobserve-audit`)"
)

func TestGuideForNoTypesIsTheCore(t *testing.T) {
	core := prompts.GuideFor(nil)
	for _, heading := range []string{"## Prometheus", "## ClickHouse", "## Loki", "## Tempo", "## Elasticsearch"} {
		if strings.Contains(core, heading) {
			t.Errorf("the core holds %q", heading)
		}
	}
	if !strings.HasSuffix(core, "\n") || strings.HasSuffix(core, "\n\n") {
		t.Errorf("the core must end with exactly one newline")
	}
}

func TestGuideForKeepsTheFixedOrderAndIgnoresDuplicatesAndUnknownTypes(t *testing.T) {
	guide := prompts.GuideFor([]spec.DatasourceType{spec.Tempo, spec.Prometheus, spec.Tempo, "elastic"})
	prom := strings.Index(guide, sectionHeadings[spec.Prometheus])
	tempo := strings.Index(guide, sectionHeadings[spec.Tempo])
	if prom < 0 || tempo < 0 || prom > tempo {
		t.Fatalf("prometheus at %d, tempo at %d; want both, prometheus first", prom, tempo)
	}
	if strings.Count(guide, sectionHeadings[spec.Tempo]) != 1 {
		t.Errorf("the tempo section appears more than once")
	}
	for _, heading := range []string{sectionHeadings[spec.ClickHouse], sectionHeadings[spec.Loki]} {
		if strings.Contains(guide, heading) {
			t.Errorf("the guide holds %q, which no type asked for", heading)
		}
	}
	want := strings.TrimRight(prompts.GuideFor(nil), "\n") + "\n\n" + section(t, "prometheus") + "\n\n" + section(t, "tempo") + "\n"
	if guide != want {
		t.Errorf("guide is not the core, then each section after one blank line")
	}
}

func TestSpecGuideHoldsEverySection(t *testing.T) {
	all := []spec.DatasourceType{spec.Prometheus, spec.ClickHouse, spec.Loki, spec.Tempo, spec.Jaeger, spec.Elasticsearch, spec.OpenSearch}
	if prompts.SpecGuide() != prompts.GuideFor(all) {
		t.Fatal("SpecGuide is not the guide for every type")
	}
	for _, heading := range sectionHeadings {
		if !strings.Contains(prompts.SpecGuide(), heading) {
			t.Errorf("SpecGuide lacks %q", heading)
		}
	}
}

func TestSearchSectionAppearsOnceForEitherOrBothSearchTypes(t *testing.T) {
	for name, types := range map[string][]spec.DatasourceType{
		"elasticsearch": {spec.Elasticsearch},
		"opensearch":    {spec.OpenSearch},
		"both":          {spec.OpenSearch, spec.Elasticsearch},
	} {
		t.Run(name, func(t *testing.T) {
			guide := prompts.GuideFor(types)
			if got := strings.Count(guide, searchHeading); got != 1 {
				t.Fatalf("search section appears %d times, want 1", got)
			}
			want := strings.TrimRight(prompts.GuideFor(nil), "\n") + "\n\n" + section(t, "search") + "\n"
			if guide != want {
				t.Errorf("guide is not the core and the search section")
			}
		})
	}
	if strings.Contains(prompts.GuideFor([]spec.DatasourceType{spec.Loki}), searchHeading) {
		t.Errorf("a Loki-only guide holds the search section")
	}
}

func TestTheAuditSectionComesOnlyWithTheBuiltInDatasource(t *testing.T) {
	builtIn := prompts.GuideFor(prompts.GuideTypes(string(spec.OpenSearch), true))
	want := strings.TrimRight(prompts.GuideFor(nil), "\n") + "\n\n" + section(t, "search") + "\n\n" + section(t, "audit") + "\n"
	if builtIn != want {
		t.Errorf("the built-in datasource's guide is not the core, the search section and the audit section")
	}
	for name, guide := range map[string]string{
		"an OpenSearch datasource": prompts.GuideFor(prompts.GuideTypes(string(spec.OpenSearch), false)),
		"the investigate guide":    prompts.SpecGuide(),
	} {
		if strings.Contains(guide, auditHeading) {
			t.Errorf("%s holds the audit section", name)
		}
	}
}

func TestGuideFilesAreSectionsWithoutEmDashes(t *testing.T) {
	files, err := fs.Glob(promptFiles, "guide/*.md")
	if err != nil || len(files) != len(sectionHeadings)+1 {
		t.Fatalf("guide files = %v, %v; want %d", files, err, len(sectionHeadings)+1)
	}
	for _, name := range append(files, "spec_guide.md") {
		body, err := fs.ReadFile(promptFiles, name)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "—") {
			t.Errorf("%s holds an em dash", name)
		}
		if name != "spec_guide.md" && !strings.HasPrefix(string(body), "## ") {
			t.Errorf("%s does not start with a ## heading", name)
		}
	}
}

func section(t *testing.T, name string) string {
	t.Helper()
	body, err := fs.ReadFile(promptFiles, "guide/"+name+".md")
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimRight(string(body), "\n")
}
