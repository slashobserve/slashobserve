package main

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestUsagePinsAreValidatedAgainstTheCounterNames(t *testing.T) {
	cases := map[string]struct {
		pins string
		want string
	}{
		"every counter":  {"{build_saves: 3, renewals: 1, compactions: 1, nudges: 0, retries: 0, save_now_refusals: 1, stalled_turns: 1, one_panel_refusals: 1}", ""},
		"unknown key":    {"{panels: 3}", `expect.usage key "panels" must be one of`},
		"negative value": {"{nudges: -1}", "expect.usage.nudges must not be negative"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := loadScenarios(writeScenario(t, scenarioHeader+"expect:\n  datasources: [prometheus]\n  usage: "+tc.pins+"\n"))
			switch {
			case tc.want == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
				t.Fatalf("error %v does not contain %q", err, tc.want)
			}
		})
	}
}

func TestUsageCheckComparesEveryPinnedCounterExactly(t *testing.T) {
	usage := &Usage{BuildSaves: 3, Renewals: 2, Compactions: 2, Nudges: 1, Retries: 4, SaveNowRefusals: 1, StalledTurns: 2, OnePanelRefusals: 1}
	cases := map[string]struct {
		pins   map[string]int
		usage  *Usage
		want   bool
		detail string
	}{
		"all match": {
			pins:  map[string]int{"build_saves": 3, "renewals": 2, "compactions": 2, "nudges": 1, "retries": 4, "save_now_refusals": 1, "stalled_turns": 2, "one_panel_refusals": 1},
			usage: usage, want: true,
		},
		"zero is a real pin": {pins: map[string]int{"compactions": 0}, usage: usage, detail: "compactions 2, expected 0"},
		"too low":            {pins: map[string]int{"build_saves": 4}, usage: usage, detail: "build_saves 3, expected 4"},
		"two wrong ones": {
			pins: map[string]int{"nudges": 0, "renewals": 1}, usage: usage,
			detail: "nudges 1, expected 0; renewals 2, expected 1",
		},
		"no usage reported": {pins: map[string]int{"nudges": 1}, detail: "no usage"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := checkUsage(Expect{Usage: tc.pins}, observation{request: okRequest(func(req *AgentRequest) { req.Usage = tc.usage })})
			if r.Passed != tc.want {
				t.Fatalf("passed %t, want %t (%s)", r.Passed, tc.want, r.Detail)
			}
			if !strings.Contains(r.Detail, tc.detail) {
				t.Fatalf("detail %q does not contain %q", r.Detail, tc.detail)
			}
		})
	}
}

func TestRunChecksAddsTheUsageCheckOnlyWhenPinned(t *testing.T) {
	sc := Scenario{ID: "s", Expect: Expect{FailureCode: "no_progress"}}
	obs := observation{request: okRequest(failedWith("no_progress"), func(req *AgentRequest) { req.Usage = &Usage{StalledTurns: 2} })}
	if got := runChecks(sc, obs); len(got) != 1 {
		t.Fatalf("got %d checks without a pin, want 1", len(got))
	}
	sc.Expect.Usage = map[string]int{"stalled_turns": 2}
	got := runChecks(sc, obs)
	if len(got) != 2 || got[1].Name != checkNameUsage || !got[1].Passed {
		t.Fatalf("got %+v, want the failure code check then a passing usage check", got)
	}
}

func TestRenewalScenarioPinsWhatTheGeneratedFixtureCauses(t *testing.T) {
	scenarios, err := loadScenarios(filepath.Join("..", "scenarios"))
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(scenarios, func(sc Scenario) bool { return sc.ID == "build-nef-renewal" })
	if i < 0 {
		t.Fatal("the renewal scenario is missing")
	}
	sc := scenarios[i]
	fixture, err := renewalFixture()
	if err != nil {
		t.Fatal(err)
	}
	if fixture.Prompt != sc.Prompt {
		t.Fatalf("the fixture prompt %q differs from the scenario prompt %q", fixture.Prompt, sc.Prompt)
	}
	if len(fixture.Turns) != sc.Expect.MinPanels || len(fixture.Turns) != sc.Expect.MaxPanels {
		t.Fatalf("the fixture saves %d panels, the scenario expects %d to %d", len(fixture.Turns), sc.Expect.MinPanels, sc.Expect.MaxPanels)
	}
	for _, resultBytes := range []int{0, 300, 800} {
		if got := renewalsOf(fixture, resultBytes); got != sc.Expect.Usage["renewals"] || got != sc.Expect.Usage["compactions"] {
			t.Errorf("with %d result bytes per save the fixture causes %d renewals, the scenario pins %d renewals and %d compactions",
				resultBytes, got, sc.Expect.Usage["renewals"], sc.Expect.Usage["compactions"])
		}
	}
}

func renewalsOf(fixture *replayFixture, resultBytes int) int {
	const renewBytes = 256 << 10
	renewals, session := 0, 0
	for _, turn := range fixture.Turns {
		for _, call := range turn.ToolCalls {
			session += len(call.Input) + resultBytes
		}
		if session > renewBytes {
			renewals++
			session = 0
		}
	}
	return renewals
}
