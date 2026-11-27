package prompts

import (
	"strings"
	"testing"
)

func TestTheFactsVariantTradesTheFigureRulesForTheFactsSection(t *testing.T) {
	base := Investigate()
	for _, rule := range enforcedRules {
		if strings.Count(base, "\n"+rule) != 1 {
			t.Fatalf("the investigate prompt lacks the rule %q that the facts variant drops", rule)
		}
	}
	for name, with := range map[string]string{"facts": InvestigateWithFacts(false), "facts and ledger": InvestigateWithFacts(true)} {
		for _, rule := range enforcedRules {
			if strings.Contains(with, rule) {
				t.Errorf("%s: the variant keeps %q", name, rule)
			}
		}
		for _, want := range []string{"\n\n## Facts\n", "`{{f3}}`", "`answers`", "within 1%", "\n\n" + strings.TrimRight(Scope(), "\n") + "\n\n# Authoring guide\n"} {
			if !strings.Contains(with, want) {
				t.Errorf("%s: the variant lacks %q", name, want)
			}
		}
		if growth := len(with) - len(base); name == "facts" && growth > 600 {
			t.Errorf("the facts variant is %d bytes longer than the investigate prompt, over 600", growth)
		}
	}
	if strings.Contains(base, "## Facts") || strings.Contains(InvestigateWithLedger(), "## Facts") {
		t.Error("a prompt without facts carries the facts section")
	}
	both := InvestigateWithFacts(true)
	if facts, ledger := strings.Index(both, "\n\n## Facts\n"), strings.Index(both, "\n\n## Hypothesis ledger\n"); facts < 0 || ledger < facts {
		t.Error("the ledger section does not follow the facts section")
	}
	if strings.Contains(InvestigateWithFacts(false), "note_hypotheses") {
		t.Error("the facts variant without the loop carries the ledger section")
	}
}
