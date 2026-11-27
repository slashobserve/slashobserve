package brief

import (
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/prompts"
	"github.com/slashobserve/slashobserve/internal/agent/route"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestFactsPickTheInvestigateVariantAndANoteForAnAnswerElsewhere(t *testing.T) {
	for name, tc := range renderCases(t) {
		plain, err := Render(statuses, tc.task, tc.views[0], Fresh, "")
		if err != nil {
			t.Fatal(err)
		}
		task := tc.task
		task.Facts = true
		got, err := Render(statuses, task, tc.views[0], Fresh, "")
		if err != nil {
			t.Fatal(err)
		}
		noted := strings.Contains(got.Context, factsNote)
		switch name {
		case "investigate":
			if got.System != investigateText(prompts.Variant{Facts: true}) || noted {
				t.Errorf("investigate: facts did not pick the facts variant alone")
			}
			task.Loop = true
			if looped, _ := Render(statuses, task, tc.views[0], Fresh, ""); looped.System != investigateText(prompts.Variant{Facts: true, Ledger: true}) {
				t.Error("investigate: facts with the loop did not pick the variant with both")
			}
		case "suggest":
			if noted || got.System != plain.System {
				t.Error("suggest: facts changed the brief")
			}
		default:
			if !noted || got.System != plain.System {
				t.Errorf("%s: noted = %t, system changed = %t", name, noted, got.System != plain.System)
			}
		}
		if strings.Contains(plain.Context, factsNote) {
			t.Errorf("%s: the note is there without facts", name)
		}
	}
}

func investigateText(v prompts.Variant) string {
	return prompts.InvestigateFor(v, []spec.DatasourceType{spec.Prometheus})
}

func TestRenderSendsThePlaybookOfTheRunsTaskType(t *testing.T) {
	tc := renderCases(t)["investigate"]
	for _, task := range route.Tasks {
		brief := tc.task
		brief.Route, brief.Facts, brief.Loop = task, true, true
		got, err := Render(statuses, brief, tc.views[0], Fresh, "")
		if err != nil {
			t.Fatal(err)
		}
		if got.System != investigateText(prompts.Variant{Task: task, Facts: true, Ledger: true}) {
			t.Errorf("%s: the system text is not its playbook's", task.Name())
		}
	}
	for _, name := range []string{"create", "revise", "suggest"} {
		c := renderCases(t)[name]
		plain, _ := Render(statuses, c.task, c.views[0], Fresh, "")
		routed := c.task
		routed.Route = route.Measure
		if got, _ := Render(statuses, routed, c.views[0], Fresh, ""); got != plain {
			t.Errorf("%s: a task type changed the brief", name)
		}
	}
}
