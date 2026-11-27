package prompts

import (
	"strings"
	"testing"
)

func TestInvestigateHasNoLedgerSectionUntilTheLoopAsksForIt(t *testing.T) {
	base := Investigate()
	if strings.Contains(base, "note_hypotheses") || strings.Contains(base, "Hypothesis ledger") || strings.Contains(base, "<hypotheses>") {
		t.Fatal("the investigate prompt carries the ledger section with the loop off")
	}
	if !strings.HasPrefix(base, "# Investigating\n") || !strings.Contains(base, "\n\n"+strings.TrimRight(Scope(), "\n")+"\n\n# Authoring guide\n") {
		t.Fatal("the investigate prompt lost its head or its scope and guide")
	}
}

func TestTheLedgerSectionIsShortAndSitsBeforeTheScope(t *testing.T) {
	with, base := InvestigateWithLedger(), Investigate()
	before, after, found := strings.Cut(with, "\n\n## Hypothesis ledger\n")
	if !found || before != strings.TrimRight(strings.Split(base, "\n\n"+strings.TrimRight(Scope(), "\n"))[0], "\n") {
		t.Fatal("the section does not follow the unchanged investigate text")
	}
	section, tail, found := strings.Cut(after, "\n\n"+strings.TrimRight(Scope(), "\n"))
	if !found || !strings.HasSuffix(base, tail) {
		t.Fatal("the scope and guide after the section differ from the investigate prompt")
	}
	if lines := strings.Count(strings.TrimRight(section, "\n"), "\n") + 1; lines > 10 {
		t.Fatalf("the section has %d lines, over 10", lines)
	}
	for _, want := range []string{"note_hypotheses", "ruled_out", "supported", "next_check", "<hypotheses>", "save_finding", "close every one with its evidence and a `closing`", "`transient` (gone on a later read, citing both reads)"} {
		if !strings.Contains(section, want) {
			t.Errorf("the section lacks %q", want)
		}
	}
}
