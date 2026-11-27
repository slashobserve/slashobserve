package brief

import (
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/prompts"
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
			if got.System != prompts.InvestigateWithFacts(false) || noted {
				t.Errorf("investigate: facts did not pick the facts variant alone")
			}
			task.Loop = true
			if looped, _ := Render(statuses, task, tc.views[0], Fresh, ""); looped.System != prompts.InvestigateWithFacts(true) {
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
