package brief

import (
	"strings"
	"testing"
)

func TestRenderSendsTheTriageBlockToInvestigationsOnly(t *testing.T) {
	const block = "<triage>\nCandidates: none.\n</triage>"
	for name, tc := range renderCases(t) {
		task := tc.task
		task.Triage = block
		got, err := Render(statuses, task, tc.views[0], Fresh, "")
		if err != nil {
			t.Fatal(err)
		}
		sent := strings.Contains(got.Context, "\n\n"+block+"\n\n") && strings.Contains(got.User, block)
		if sent != (name == "investigate") {
			t.Fatalf("%s: triage block sent = %v", name, sent)
		}
	}
}
