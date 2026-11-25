package brief

import (
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/prompts"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

var sampleLedger = []revisions.Hypothesis{
	{ID: "pool", Text: "The checkout pool is exhausted", Status: "supported", Evidence: []string{"pool_in_use 50 of 50 at 10:05", "waiters 120"}},
	{ID: "deploy", Text: "The 10:00 deploy broke it", Status: "ruled_out", Evidence: []string{"no deploy annotation in the window"}},
	{ID: "db", Text: "The database is slow", Status: "open", Next: "read db latency"},
}

func TestHypothesesRenderEachOneWithItsStatusEvidenceAndNextCheck(t *testing.T) {
	want := "<hypotheses>\n" +
		"Your ledger so far; send the whole ledger with note_hypotheses when it changes, and do not repeat a read that ruled one out.\n" +
		"- pool [supported] The checkout pool is exhausted\n" +
		"  evidence: pool_in_use 50 of 50 at 10:05\n" +
		"  evidence: waiters 120\n" +
		"- deploy [ruled_out] The 10:00 deploy broke it\n" +
		"  evidence: no deploy annotation in the window\n" +
		"- db [open] The database is slow\n" +
		"  next: read db latency\n" +
		"</hypotheses>"
	if got := Hypotheses(sampleLedger); got != want {
		t.Fatalf("block:\n%s\nwant:\n%s", got, want)
	}
	if got := Hypotheses(nil); got != "" {
		t.Fatalf("an empty ledger rendered %q", got)
	}
}

func TestAFullLedgerRendersWithinTheLedgerCapPlusItsFixedText(t *testing.T) {
	var ledger []revisions.Hypothesis
	for _, id := range []string{"a", "b", "c", "d", "e", "f"} {
		ledger = append(ledger, revisions.Hypothesis{
			ID: id, Text: strings.Repeat("x", 160), Status: "open",
			Evidence: []string{strings.Repeat("y", 60), strings.Repeat("y", 60), strings.Repeat("y", 60)}, Next: strings.Repeat("z", 160),
		})
	}
	size := 0
	for _, h := range ledger {
		size += h.Size()
	}
	if size > tools.MaxLedgerBytes {
		t.Fatalf("the sample is %d bytes, over the cap", size)
	}
	if got := len(Hypotheses(ledger)); got > tools.MaxLedgerBytes+1024 {
		t.Fatalf("block is %d bytes for a %d byte ledger", got, size)
	}
}

func TestRenderWritesTheLedgerBlockWhereItWritesTheDigest(t *testing.T) {
	const digest = "<discovered>\n- prom metrics: up\n</discovered>"
	block := Hypotheses(sampleLedger)
	for name, tc := range renderCases(t) {
		task := tc.task
		task.Loop, task.Hypotheses = true, block
		got, err := Render(statuses, task, tc.views[0], Fresh, digest)
		if err != nil {
			t.Fatal(err)
		}
		appended := strings.HasSuffix(got.Context, "\n\n"+digest+"\n\n"+block)
		if appended == (name == "suggest") {
			t.Fatalf("%s: block appended = %v", name, appended)
		}
	}
}

func TestRenderSendsTheLedgerSectionOnlyInInvestigateRunsWithTheLoopOn(t *testing.T) {
	cases := renderCases(t)
	for name, tc := range cases {
		off, err := Render(statuses, tc.task, tc.views[0], Fresh, "")
		if err != nil {
			t.Fatal(err)
		}
		task := tc.task
		task.Loop = true
		on, err := Render(statuses, task, tc.views[0], Fresh, "")
		if err != nil {
			t.Fatal(err)
		}
		if name != "investigate" {
			if on != off {
				t.Fatalf("%s: the loop changed a run that is not an investigation", name)
			}
			continue
		}
		if off.System != prompts.Investigate() || on.System != prompts.InvestigateWithLedger() || off.System == on.System {
			t.Fatalf("investigate: system text off %t, on %t", off.System == prompts.Investigate(), on.System == prompts.InvestigateWithLedger())
		}
		if on.Context != off.Context || on.User != off.User {
			t.Fatal("investigate: the loop changed the context of a run with no ledger yet")
		}
		if strings.Contains(off.System, "note_hypotheses") || strings.Contains(off.Context, "<hypotheses>") {
			t.Fatal("investigate: the loop text is present with the loop off")
		}
	}
}
