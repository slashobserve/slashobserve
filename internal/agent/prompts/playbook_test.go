package prompts

import (
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/route"
)

var playbookHeadings = map[route.Task]string{
	route.Measure:   "\n## Measuring a value\n",
	route.Logs:      "\n## Reading logs\n",
	route.Traces:    "\n## Reading traces\n",
	route.Incident:  "\n## Investigating an incident\n",
	route.Inventory: "\n## Listing what exists\n",
	route.Explain:   "\n## Explaining\n",
}

var everyTask = append([]route.Task{route.Full}, route.Tasks...)

func everyVariant(task route.Task) []Variant {
	return []Variant{{Task: task}, {Task: task, Facts: true}, {Task: task, Ledger: true}, {Task: task, Facts: true, Ledger: true}}
}

func TestEachTaskTypeGetsTheCoreAndOnlyItsPlaybook(t *testing.T) {
	for _, task := range everyTask {
		for _, v := range everyVariant(task) {
			text := InvestigateFor(v, allTypes)
			if !strings.HasPrefix(text, "# Investigating\n") || strings.Count(text, "End with exactly one save_finding call.") != 1 {
				t.Errorf("%+v lacks the core", v)
			}
			for other, heading := range playbookHeadings {
				if want := task == route.Full || other == task; strings.Contains(text, heading) != want {
					t.Errorf("%+v: holds %q = %v", v, heading, !want)
				}
			}
			if strings.Contains(text, "## Questions of every kind") != (task == route.Full) {
				t.Errorf("%+v: the general section is wrong", v)
			}
			if strings.Contains(text, "\n## Facts\n") != v.Facts || strings.Contains(text, "\n## Hypothesis ledger\n") != v.Ledger {
				t.Errorf("%+v: the facts or ledger section is wrong", v)
			}
			for _, rule := range enforcedRules {
				if strings.Contains(text, rule) == v.Facts {
					t.Errorf("%+v: the rule %q is wrong for its facts setting", v, rule)
				}
			}
			if !strings.Contains(text, "A `<triage>` block, when the context has one") {
				t.Errorf("%+v lacks the triage line", v)
			}
		}
	}
}

func TestAPlaybookNamesOnlyToolsItsTaskOffers(t *testing.T) {
	for task, wants := range map[route.Task][]string{
		route.Measure:   {"`measure_window`", "`measure_aggregate`"},
		route.Logs:      {"`log_patterns`", "`log_facets`", "`log_context`"},
		route.Traces:    {"`trace_error_origins`", "`trace_slow_path`", "`trace_stats`"},
		route.Incident:  {"`trace_error_origins`", "`list_annotations`", "`log_patterns`"},
		route.Inventory: {"`list_metrics`", "`search_workspace`"},
		route.Explain:   {"`get_dashboard`", "`get_observation`", "`suggested_change`"},
	} {
		body := playbookFor(task)
		for _, want := range wants {
			if !strings.Contains(body, want) {
				t.Errorf("the %s playbook lacks %s", task.Name(), want)
			}
		}
		for _, line := range strings.Split(body, "\n") {
			if n := len(line); n > 600 {
				t.Errorf("the %s playbook has a %d byte line", task.Name(), n)
			}
		}
	}
}

func TestEveryTaskTypeIsSmallerThanTheInvestigateTextBeforeTaskTypes(t *testing.T) {
	before := map[[2]bool]int{
		{false, false}: 33615,
		{true, false}:  33922,
		{false, true}:  34729,
		{true, true}:   35036,
	}
	const bodyBudget = 7000
	for _, task := range route.Tasks {
		for _, v := range everyVariant(task) {
			if got, limit := len(InvestigateFor(v, allTypes)), before[[2]bool{v.Facts, v.Ledger}]; got >= limit {
				t.Errorf("%+v is %d bytes, not under the %d of the text before task types", v, got, limit)
			}
			if got := len(InvestigateBody(Variant{Task: task})); got > bodyBudget {
				t.Errorf("the %s body is %d bytes, over %d", task.Name(), got, bodyBudget)
			}
		}
	}
}

func TestTheInvestigateGuideCarriesOnlyTheConfiguredTypes(t *testing.T) {
	text := InvestigateFor(Variant{Task: route.Measure}, demoTypes)
	if !strings.Contains(text, "## Prometheus (PromQL)") || strings.Contains(text, "## Loki (LogQL)") {
		t.Fatal("the investigate guide does not follow the configured types")
	}
	if InvestigateFor(Variant{}, allTypes) != Investigate() {
		t.Fatal("Investigate is not the full variant over every type")
	}
}
