package runsession

import (
	"encoding/json"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/guard"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/llm/pricing"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func TestTurnValidityRejectsKeysTheServerCannotTrust(t *testing.T) {
	long := strings.Repeat("x", maxTurnIDBytes+1)
	cases := []struct {
		name string
		turn *Turn
		ok   bool
	}{
		{"none", nil, false},
		{"a turn", &Turn{ID: "t", Calls: 3, Saves: 1}, true},
		{"the largest turn", &Turn{ID: strings.Repeat("x", maxTurnIDBytes), Calls: maxTurnCalls, Saves: maxTurnCalls}, true},
		{"no id", &Turn{Calls: 1}, false},
		{"id too long", &Turn{ID: long, Calls: 1}, false},
		{"no calls", &Turn{ID: "t"}, false},
		{"too many calls", &Turn{ID: "t", Calls: maxTurnCalls + 1}, false},
		{"negative saves", &Turn{ID: "t", Calls: 1, Saves: -1}, false},
		{"more saves than calls", &Turn{ID: "t", Calls: 1, Saves: 2}, false},
	}
	for _, tc := range cases {
		if got := tc.turn.valid(); (got != nil) != tc.ok {
			t.Errorf("%s: valid = %v, want %v", tc.name, got != nil, tc.ok)
		}
	}
}

func TestOpenTurnIsJoinedOnlyByAKeyWithItsID(t *testing.T) {
	open := OpenTurn{Open: true, ID: "t1"}
	if !open.joinedBy(&Turn{ID: "t1"}) || open.joinedBy(&Turn{ID: "t2"}) || open.joinedBy(nil) {
		t.Fatal("joinedBy does not match on the id")
	}
	if (OpenTurn{Open: true}).joinedBy(&Turn{ID: ""}) || (OpenTurn{ID: "t1"}).joinedBy(&Turn{ID: "t1"}) {
		t.Fatal("a keyless or closed turn was joined")
	}
}

func TestAnswerTextAppendsEachNoteAfterTheContent(t *testing.T) {
	a := Answer{Content: []byte(`{"ok":true}`), Notes: []string{"one", "two"}}
	if got, want := a.Text(), "{\"ok\":true}\n\n<note>one</note>\n\n<note>two</note>"; got != want {
		t.Fatalf("text = %q, want %q", got, want)
	}
	if got := (Answer{Content: []byte(`{}`)}).Text(); got != "{}" {
		t.Fatalf("text = %q", got)
	}
	a.Text()
	if string(a.Content) != `{"ok":true}` {
		t.Fatalf("Text changed the content: %s", a.Content)
	}
}

func TestMetaNamesTheSettledStateAndHidesTheBuildOfAnEndedRun(t *testing.T) {
	build := &tools.BuildProgress{Saves: 1, Panels: 1, PlannedPanels: 2}
	cases := []struct {
		name      string
		state     State
		restarted bool
		settled   string
		build     bool
	}{
		{"nothing settled", State{Build: build}, false, "", true},
		{"saved", State{Build: build, Settled: Settled{Kind: settledSaved}}, false, "saved", true},
		{"restarted", State{}, true, "restarted", false},
		{"ended", State{Build: build, Settled: Settled{Kind: settledEnded}}, false, "ended", false},
	}
	for _, tc := range cases {
		got := tc.state.meta(tc.restarted)
		gotSettled := ""
		if got.Settled != nil {
			gotSettled = *got.Settled
		}
		if gotSettled != tc.settled || (got.Build != nil) != tc.build {
			t.Errorf("%s: meta = %+v", tc.name, got)
		}
	}
}

func TestStoredDigestKeepsTheNewestEntriesWithinItsCap(t *testing.T) {
	var s State
	name := strings.Repeat("n", 1000)
	for i := range 30 {
		s.addDiscovery(tools.Discovery{Datasource: "prom", Of: strconv.Itoa(i), Names: []string{name}})
	}
	total := 0
	for _, e := range s.Digest.Entries {
		total += len(e.Datasource) + len(e.Of) + len(e.Names[0])
	}
	if total > maxStoredDigestBytes || len(s.Digest.Entries) == 0 || len(s.Digest.Entries) >= 30 {
		t.Fatalf("kept %d entries, %d bytes", len(s.Digest.Entries), total)
	}
	if last := s.Digest.Entries[len(s.Digest.Entries)-1]; last.Of != strconv.Itoa(29) {
		t.Fatalf("the newest entry was dropped: %+v", last)
	}
}

func TestNoteModelKeepsTheLargestOfEachCumulativeCount(t *testing.T) {
	var s State
	s.noteModel(revisions.Usage{Provider: "p", Model: "m", InputTokens: 100, ModelCalls: 3, CostUSD: floatPtr(0.5)})
	s.noteModel(revisions.Usage{InputTokens: 40, OutputTokens: 9, ModelCalls: 1, CostUSD: floatPtr(0.2)})
	s.noteModel(revisions.Usage{InputTokens: -5, CostUSD: floatPtr(-1)})
	got := s.Usage
	if got.Provider != "p" || got.Model != "m" || got.InputTokens != 100 || got.OutputTokens != 9 || got.ModelCalls != 3 || got.CostUSD == nil || *got.CostUSD != 0.5 {
		t.Fatalf("usage = %+v", got)
	}
}

func TestStoredUsageIsNilForARunThatDidNothingAndCarriesGuardCounts(t *testing.T) {
	var s State
	if s.storedUsage(pricing.Table{}) != nil {
		t.Fatal("an idle run stored usage")
	}
	s.Guard.Counts.ToolCalls, s.Guard.Counts.Repairs, s.Guard.Counts.BuildSaves = 4, 1, 2
	got := s.storedUsage(pricing.Table{})
	if got == nil || got.ToolCalls != 4 || got.RepairRounds != 1 || got.BuildSaves != 2 {
		t.Fatalf("usage = %+v", got)
	}
}

func TestOverCostLimitNeedsALimitAndACost(t *testing.T) {
	cost := func(v float64) revisions.Usage { return revisions.Usage{CostUSD: &v} }
	cases := []struct {
		name  string
		limit float64
		usage revisions.Usage
		want  bool
	}{
		{"no limit", 0, cost(100), false},
		{"under", 1, cost(0.99), false},
		{"at the limit", 1, cost(1), true},
		{"over", 1, cost(2), true},
		{"no cost known", 1, revisions.Usage{InputTokens: 1 << 40}, false},
	}
	for _, tc := range cases {
		s := &Service{maxRunCost: tc.limit}
		if got := s.overCostLimit(tc.usage); got != tc.want {
			t.Errorf("%s: over = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestSessionStateRoundTripsThroughJSON(t *testing.T) {
	var s State
	s.Guard = newGuardForTest()
	s.Settled = Settled{Kind: settledSaved, Version: 3}
	s.Turn = OpenTurn{Seq: 4, ID: "t", Open: true, Calls: 2, Admitted: 1}
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var back State
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	again, _ := json.Marshal(back)
	if string(again) != string(raw) {
		t.Fatalf("round trip changed the state:\n%s\n%s", raw, again)
	}
}

func TestTheRunSessionDoesNotImportTheAgentLoop(t *testing.T) {
	if testing.Short() {
		t.Skip("go list is slow")
	}
	out, err := exec.Command("go", "list", "-deps", ".").CombinedOutput()
	if err != nil {
		t.Fatalf("go list: %v\n%s", err, out)
	}
	for _, dep := range strings.Fields(string(out)) {
		if strings.HasSuffix(dep, "/internal/agent") {
			t.Fatalf("the run session depends on the agent loop: %s", dep)
		}
	}
}

func newGuardForTest() guard.State {
	g := guard.New(tools.ModeRevise)
	g.BeginSession(true, 2)
	g.BeginTurn(false)
	return g
}
