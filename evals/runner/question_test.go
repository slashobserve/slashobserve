package main

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/llm"
	"github.com/slashobserve/slashobserve/internal/llm/replay"
)

var questionScenarios = []string{"question-what-logs-exist", "question-shop-request-rate", "question-explain-dashboard", "question-out-of-scope"}

func TestQuestionScenariosLoadAsLiveAndReplayQuestions(t *testing.T) {
	for _, id := range questionScenarios {
		sc := shippedScenario(t, id)
		if sc.Kind != kindQuestion || !slices.Equal(sc.Suites, []string{suiteLive, suiteReplay}) || sc.DoneWhen || sc.Category != "" || sc.Seed != nil {
			t.Errorf("%s loaded as %+v", id, sc)
		}
	}
}

func TestQuestionExpectationsAreValidated(t *testing.T) {
	question := func(edit func(*Scenario)) Scenario {
		sc := Scenario{ID: "probe", Prompt: "what logs exist", Kind: kindQuestion, Suites: []string{suiteReplay}}
		edit(&sc)
		applyDefaults(&sc)
		return sc
	}
	cases := []struct {
		name string
		sc   Scenario
		want string
	}{
		{"no expectation", question(func(*Scenario) {}), ""},
		{"mentions", question(func(sc *Scenario) { sc.Expect.FindingMentions = []string{"checkout"} }), ""},
		{"a seed", question(func(sc *Scenario) { sc.Seed = &Seed{Corpus: "a.json"} }), "a question scenario does not take a seed"},
		{"a panel count", question(func(sc *Scenario) { sc.Expect.MinPanels = 2 }), "a question scenario does not take min_panels"},
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

func TestCheckAnswered(t *testing.T) {
	version, dashboard := 1, "d1"
	finding := &Finding{Markdown: "Loki holds checkout logs."}
	cases := []struct {
		name   string
		req    AgentRequest
		passed bool
		detail string
	}{
		{"an answer", AgentRequest{Status: "succeeded", Finding: finding}, true, ""},
		{"a failed request", AgentRequest{Status: "failed"}, false, "request failed"},
		{"a dashboard instead", AgentRequest{Status: "succeeded", ResultVersion: &version, DashboardID: &dashboard}, false, "the run built a dashboard instead of answering"},
		{"no finding", AgentRequest{Status: "succeeded"}, false, "the request has no finding"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := checkAnswered(observation{request: c.req})
			if got.Name != "answered" || got.Passed != c.passed || (c.detail != "" && got.Detail != c.detail) {
				t.Fatalf("check = %+v", got)
			}
		})
	}
}

func TestQuestionFixturesLoadInTheReplayProviderAndEndWithSaveFinding(t *testing.T) {
	provider, err := replay.New(filepath.Join("..", "fixtures", "llm"), corpusTokenCap, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range questionScenarios {
		session, err := provider.Start(context.Background(), llm.Request{Prompt: shippedScenario(t, id).Prompt})
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		var results []llm.ToolResult
		var last llm.Turn
		for range 3 {
			turn, err := session.Next(context.Background(), results)
			if err != nil {
				t.Fatalf("%s: %v", id, err)
			}
			if len(turn.ToolCalls) == 0 {
				break
			}
			last, results = turn, nil
			for _, c := range turn.ToolCalls {
				results = append(results, llm.ToolResult{CallID: c.ID, Content: "{}"})
			}
		}
		if len(last.ToolCalls) != 1 || last.ToolCalls[0].Name != "save_finding" {
			t.Fatalf("%s last turn = %+v", id, last)
		}
	}
}
