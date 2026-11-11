package prompts

import (
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	systemTextBudget     = 23335
	fullSystemTextBudget = 28077
)

var demoTypes = []spec.DatasourceType{spec.Prometheus, spec.ClickHouse}

func TestBuildHoldsTheRulesTheGuideAndTheExampleButNotTheSchema(t *testing.T) {
	system := Build(demoTypes)
	for _, want := range []string{
		"You are SlashObserve's dashboard agent.",
		"\n\n# Authoring guide\n",
		"\n\n# Example Spec (format only)\n```json\n",
	} {
		if !strings.Contains(system, want) {
			t.Fatalf("system text lacks %q", want)
		}
	}
	if strings.Contains(system, "# ObserveSpec v0 JSON Schema") || strings.Contains(system, `"$defs"`) {
		t.Fatal("system text carries the JSON Schema")
	}
	if len(system) > systemTextBudget {
		t.Fatalf("system text is %d bytes, over the %d byte budget", len(system), systemTextBudget)
	}
	if !strings.Contains(system, "- Never create a placeholder panel. Keep existing placeholders unchanged apart from their position, and delete one only when the request asks.") {
		t.Fatal("system prompt lacks the placeholder rule")
	}
}

func TestBuildForEveryTypeStaysWithinBudget(t *testing.T) {
	system := Build([]spec.DatasourceType{spec.Prometheus, spec.ClickHouse, spec.Loki, spec.Tempo, spec.Elasticsearch, spec.OpenSearch})
	if len(system) > fullSystemTextBudget {
		t.Fatalf("system text for every datasource type is %d bytes, over the %d byte budget", len(system), fullSystemTextBudget)
	}
}

func TestBuildCarriesOnlyTheSectionsOfItsTypes(t *testing.T) {
	system := Build([]spec.DatasourceType{spec.Loki})
	if !strings.Contains(system, "## Loki (LogQL)") {
		t.Fatal("system text lacks the Loki section")
	}
	for _, heading := range []string{"## Prometheus (PromQL)", "## ClickHouse (OpenTelemetry exporter tables)", "## Tempo (TraceQL)"} {
		if strings.Contains(system, heading) {
			t.Errorf("system text for loki holds %q", heading)
		}
	}
}

func TestBothSystemTextsCarryTheScopeOnceBeforeTheGuide(t *testing.T) {
	scope := "\n\n# Scope\n\nEvery answer uses only the datasources in the context, the dashboards and findings on this server, and names you discovered in this run.\nWhen the data does not cover the question, say so in the finding instead of answering from general knowledge.\n\n# Authoring guide\n"
	for name, system := range map[string]string{"build": Build(demoTypes), "investigate": Investigate()} {
		if strings.Count(system, scope) != 1 || strings.Count(system, "Every answer uses only") != 1 {
			t.Errorf("%s system text does not carry the scope once before the guide", name)
		}
	}
}

func TestBuildStatesTheRulesTheLoopEnforces(t *testing.T) {
	system := Build(demoTypes)
	for _, want := range []string{
		"from the sixth model call without an accepted save only a save runs, and reads in the same call as a save still run",
		"the next call may run reads, such as `query_range` on a failing query, to see why",
		"Every rejection of a save lists the panels in `failing_panels`",
		"call `get_spec_guide` for the JSON Schema",
		"Set `planned_panels` once from the request",
		"names a dashboard to rebuild",
		"A third identical call is refused, and a `<note>` at the end of a tool result is an instruction from the server",
		"when `truncated` is true, pass `next_offset` to page on",
		"make its widths add up to exactly 24",
		"out of the panels you save: the server sizes a new panel",
		"## Panel type by data shape",
		"as wide as its line's recipe whatever its series count",
		"with `row` naming its row",
		"First decide what the request asks for, then do only that.",
		"Every answer keeps to the scope below.",
		"and earlier findings with `list_findings`",
		"never call `save_finding` after you saved a panel",
	} {
		if !strings.Contains(system, want) {
			t.Errorf("system text lacks %q", want)
		}
	}
	for _, gone := range []string{
		"two to eight panels unless the request asks for more",
		"after an `update_panels` rejection resend only",
		"The schema here leaves out",
		"Pick widths in a row that add up to 24",
		"Use `stat` or `table` for",
		"10 to 20 series: a full-width",
		"The only result that matters is a saved dashboard",
	} {
		if strings.Contains(system, gone) {
			t.Errorf("system text still holds %q", gone)
		}
	}
}
