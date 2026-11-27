package dashboards

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func reasonOfPanel(t *testing.T, u Unverified, id string) UnverifiedReason {
	t.Helper()
	p, found := u.Panel(id)
	if !found {
		t.Fatalf("panel %s is not listed in %v", id, panelIDs(u))
	}
	return p.Reason
}

func TestAnUnverifiedItemSaysWhyItWasLetThrough(t *testing.T) {
	base := testDashboard(t, ``, testPanel{id: "a", title: "A", datasource: "prom", query: "slow"}, testPanel{id: "b", title: "B", datasource: "prom", query: "up"})
	query := "/panels/0/queries/0/query"

	_, timedOut := acceptUnchangedFailures(base, base, []spec.Issue{timeoutAt(query)})
	if got := reasonOfPanel(t, timedOut, "a"); got != ReasonTimeout {
		t.Errorf("an unchanged panel that timed out: %q, want timeout", got)
	}

	_, accepted := acceptEmpty(withAcceptedEmpty(t.Context()), base, []spec.Issue{issueAt(query)})
	if got := reasonOfPanel(t, accepted, "a"); got != ReasonEmpty {
		t.Errorf("a new dashboard's empty panel: %q, want empty", got)
	}

	before := Unverified{Panels: []UnverifiedPanel{{PanelID: "a", Issues: []spec.Issue{issueAt(query)}, Reason: ReasonEmpty}}}
	carried, blocking := CarryUnverified(base, before, base, []spec.Issue{issueAt(query)})
	if len(blocking) != 0 {
		t.Fatalf("blocking %v", blocking)
	}
	if got := reasonOfPanel(t, carried, "a"); got != ReasonCarried {
		t.Errorf("a failing panel the parent already listed: %q, want carried", got)
	}

	unchanged := CarryUnchanged(base, before, base, nil, nil)
	if got := reasonOfPanel(t, unchanged, "a"); got != ReasonCarried {
		t.Errorf("an unchanged panel a partial save did not run: %q, want carried", got)
	}

	if got := reasonOfPanel(t, before.because(ReasonRestored), "a"); got != ReasonRestored {
		t.Errorf("a restored list: %q, want restored", got)
	}
	if before.Panels[0].Reason != ReasonEmpty {
		t.Errorf("marking a copy changed the source list to %q", before.Panels[0].Reason)
	}
}

func TestAnItemKeptForTwoReasonsKeepsTheCarriedOne(t *testing.T) {
	issue := issueAt("/panels/0/queries")
	carried := Unverified{Panels: []UnverifiedPanel{{PanelID: "a", Issues: []spec.Issue{issue}, Reason: ReasonCarried}}}
	accepted := Unverified{Panels: []UnverifiedPanel{{PanelID: "a", Issues: []spec.Issue{issue}, Reason: ReasonEmpty}, {PanelID: "b", Issues: []spec.Issue{issue}, Reason: ReasonEmpty}}}
	merged := carried.with(accepted)
	if a, b := reasonOfPanel(t, merged, "a"), reasonOfPanel(t, merged, "b"); a != ReasonCarried || b != ReasonEmpty {
		t.Errorf("reasons %q and %q, want carried and empty", a, b)
	}
}

func TestTheReasonIsReturnedAndAStoredItemWithoutOneStillReads(t *testing.T) {
	listed := Unverified{
		Panels:    []UnverifiedPanel{{PanelID: "a", Reason: ReasonTimeout}},
		Variables: []UnverifiedVariable{{Name: "inst", Reason: ReasonImported}},
	}.withEmptyLists()
	raw, err := json.Marshal(listed)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(raw); !strings.Contains(got, `"panel_id":"a","issues":[],"reason":"timeout"`) || !strings.Contains(got, `"name":"inst","issues":[],"reason":"imported"`) {
		t.Errorf("body %s", got)
	}
	stored, err := decodeUnverified([]byte(`{"panels":[{"panel_id":"old","issues":[]}],"variables":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	again, err := json.Marshal(stored.withEmptyLists())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(again), "reason") {
		t.Errorf("a version saved before reasons were kept gained one: %s", again)
	}
}

func TestEveryReasonIsNamedInTheVerificationStates(t *testing.T) {
	if !VerificationStates.Has(VerificationUnverified) || !VerificationStates.Has(VerificationVerified) {
		t.Fatalf("states %v", VerificationStates.Names())
	}
	for _, reason := range UnverifiedReasons {
		if !strings.Contains(VerificationStates.States[0].When, "`"+string(reason)+"`") {
			t.Errorf("the unverified state does not explain %q", reason)
		}
	}
}
