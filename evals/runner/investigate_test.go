package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/llm"
	"github.com/slashobserve/slashobserve/internal/llm/replay"
)

var investigateScenarios = []string{"investigate-errors-fault", "investigate-latency-fault", "investigate-queue-fault"}

func shippedScenario(t *testing.T, id string) Scenario {
	t.Helper()
	all, err := loadScenarios(filepath.Join("..", "scenarios"))
	if err != nil {
		t.Fatal(err)
	}
	for _, sc := range all {
		if sc.ID == id {
			return sc
		}
	}
	t.Fatalf("no scenario %s", id)
	return Scenario{}
}

func TestInvestigateScenariosLoadAsReplayInvestigations(t *testing.T) {
	for _, id := range investigateScenarios {
		sc := shippedScenario(t, id)
		if sc.Kind != kindInvestigate || len(sc.Suites) != 1 || sc.Suites[0] != suiteReplay || sc.Fault != nil || sc.Expect.EvidenceMin != 1 || len(sc.Expect.FindingMentions) != 2 {
			t.Errorf("%s loaded as %+v", id, sc)
		}
	}
}

func TestInvestigateExpectationsAreValidated(t *testing.T) {
	investigation := func(edit func(*Scenario)) Scenario {
		sc := Scenario{ID: "probe", Prompt: "why", Kind: kindInvestigate, Suites: []string{suiteReplay}, Expect: Expect{FindingMentions: []string{"checkout"}}}
		edit(&sc)
		applyDefaults(&sc)
		return sc
	}
	cases := []struct {
		name string
		sc   Scenario
		want string
	}{
		{"mentions alone", investigation(func(*Scenario) {}), ""},
		{"evidence alone", investigation(func(sc *Scenario) { sc.Expect = Expect{EvidenceMin: 2} }), ""},
		{"neither expectation", investigation(func(sc *Scenario) { sc.Expect = Expect{} }), "must expect finding_mentions or evidence_min"},
		{"a seed", investigation(func(sc *Scenario) { sc.Seed = &Seed{Corpus: "a.json"} }), "does not take a seed"},
		{"a panel count", investigation(func(sc *Scenario) { sc.Expect.MaxPanels = 3 }), "does not take min_panels"},
		{"metrics", investigation(func(sc *Scenario) { sc.Expect.Metrics = []string{"up"} }), "does not take min_panels"},
		{"a blank mention", investigation(func(sc *Scenario) { sc.Expect.FindingMentions = []string{" "} }), "finding_mentions must not contain an empty entry"},
		{"five evidence panels", investigation(func(sc *Scenario) { sc.Expect.EvidenceMin = 5 }), "evidence_min must be between 0 and 4"},
		{"mentions on a create scenario", investigation(func(sc *Scenario) {
			sc.Kind, sc.Category, sc.Expect.Datasources = kindCreate, categoryPrometheus, []string{"prometheus"}
		}), "for kinds investigate and question only"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateScenario(c.sc)
			switch {
			case c.want == "" && err != nil:
				t.Fatalf("refused: %v", err)
			case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
				t.Fatalf("error %v, want %q", err, c.want)
			}
		})
	}
}

func TestFindingChecks(t *testing.T) {
	ex := Expect{FindingMentions: []string{"checkout-0", "Errors"}, EvidenceMin: 1}
	answered := func(markdown string, evidence int) observation {
		return observation{request: AgentRequest{Finding: &Finding{Markdown: markdown, Evidence: make([]json.RawMessage, evidence)}}}
	}
	cases := []struct {
		name               string
		obs                observation
		mentions, evidence bool
		detail             string
	}{
		{"every mention in any case", answered("The ERRORS come from Checkout-0.", 1), true, true, ""},
		{"a missing mention", answered("The errors come from one pod.", 1), false, true, `does not mention "checkout-0"`},
		{"evidence below the minimum", answered("checkout-0 errors", 0), true, false, "has 0 evidence panels, expected at least 1"},
		{"no finding", observation{}, false, false, "has no finding"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mentions, evidence := checkFindingMentions(ex, c.obs), checkEvidence(ex, c.obs)
			if mentions.Passed != c.mentions || evidence.Passed != c.evidence {
				t.Fatalf("mentions %+v, evidence %+v", mentions, evidence)
			}
			if c.detail != "" && !strings.Contains(mentions.Detail+evidence.Detail, c.detail) {
				t.Fatalf("details %q and %q do not say %q", mentions.Detail, evidence.Detail, c.detail)
			}
		})
	}
}

func TestInvestigateFixturesLoadInTheReplayProviderAndReadDataFirst(t *testing.T) {
	provider, err := replay.New(filepath.Join("..", "fixtures", "llm"), corpusTokenCap, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range investigateScenarios {
		session, err := provider.Start(context.Background(), llm.Request{Prompt: shippedScenario(t, id).Prompt})
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		turn, err := session.Next(context.Background(), nil)
		if err != nil || len(turn.ToolCalls) != 1 || turn.ToolCalls[0].Name != "query_range" {
			t.Fatalf("%s first turn = %+v, %v", id, turn, err)
		}
	}
}
