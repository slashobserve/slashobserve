package dashboards

import (
	"testing"

	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func timeoutAt(path string) spec.Issue {
	return spec.Issue{Path: path, Code: queryproxy.CodeQueryTimeout, Message: "timed out"}
}

func TestAcceptTimeoutsTurnsAnUnchangedPanelsTimeoutIntoUnverified(t *testing.T) {
	base := testDashboard(t, ``, testPanel{id: "a", title: "A", datasource: "prom", query: "slow"}, testPanel{id: "b", title: "B", datasource: "prom", query: "up"})
	blocking, accepted := acceptTimeouts(base, base, []spec.Issue{timeoutAt("/panels/0/queries/0/query")})
	if len(blocking) != 0 {
		t.Fatalf("blocking %v, want none", blocking)
	}
	if ids := panelIDs(accepted); len(ids) != 1 || ids[0] != "a" {
		t.Fatalf("accepted %v, want panel a", ids)
	}
}

func TestAcceptTimeoutsKeepsBlockingWhatItMustNotAccept(t *testing.T) {
	base := testDashboard(t, ``, testPanel{id: "a", title: "A", datasource: "prom", query: "slow"})
	edited := testDashboard(t, ``, testPanel{id: "a", title: "A", datasource: "prom", query: "slower"})
	timeout := timeoutAt("/panels/0/queries/0/query")
	mixed := []spec.Issue{timeout, issueAt("/panels/0/queries")}
	cases := []struct {
		name   string
		prev   *spec.Dashboard
		next   *spec.Dashboard
		issues []spec.Issue
	}{
		{"a panel whose query changed", base, edited, []spec.Issue{timeout}},
		{"a panel with another failure", base, base, mixed},
		{"no earlier version", nil, base, []spec.Issue{timeout}},
		{"an unattributed timeout", base, base, []spec.Issue{timeoutAt("/time")}},
	}
	for _, c := range cases {
		blocking, accepted := acceptTimeouts(c.prev, c.next, c.issues)
		if !accepted.Empty() || len(blocking) != len(c.issues) {
			t.Errorf("%s: accepted %v, blocking %v, want nothing accepted", c.name, panelIDs(accepted), blocking)
		}
	}
}

func TestAcceptTimeoutsLeavesOtherPanelsIssuesBlocking(t *testing.T) {
	base := testDashboard(t, ``, testPanel{id: "a", title: "A", datasource: "prom", query: "slow"}, testPanel{id: "b", title: "B", datasource: "prom", query: "bad"})
	empty := issueAt("/panels/1/queries")
	blocking, accepted := acceptTimeouts(base, base, []spec.Issue{timeoutAt("/panels/0/queries/0/query"), empty})
	if len(blocking) != 1 || blocking[0] != empty {
		t.Fatalf("blocking %v, want only the other panel's issue", blocking)
	}
	if ids := panelIDs(accepted); len(ids) != 1 || ids[0] != "a" {
		t.Fatalf("accepted %v, want panel a", ids)
	}
}

const instVariable = `{"name":"inst","type":"query","datasource":"prom","query":"label_values(vq, instance)"}`

func variableUnresolvedAt(path string) spec.Issue {
	return spec.Issue{Path: path, Code: queryproxy.CodeVariableUnresolved, Message: "variable \"inst\" could not be resolved"}
}

func TestAcceptTimeoutsAcceptsAnUnchangedSlowVariableAndItsDependents(t *testing.T) {
	base := testDashboard(t, instVariable, testPanel{id: "a", title: "A", datasource: "prom", query: `up{instance="$inst"}`}, testPanel{id: "b", title: "B", datasource: "prom", query: "up"})
	blocking, accepted := acceptTimeouts(base, base, []spec.Issue{timeoutAt("/variables/0"), variableUnresolvedAt("/panels/0/queries/0/query")})
	if len(blocking) != 0 {
		t.Fatalf("blocking %v, want none", blocking)
	}
	if len(accepted.Variables) != 1 || accepted.Variables[0].Name != "inst" {
		t.Fatalf("accepted variables %v, want inst", accepted.Variables)
	}
	if ids := panelIDs(accepted); len(ids) != 1 || ids[0] != "a" {
		t.Fatalf("accepted panels %v, want panel a", ids)
	}
}

func TestAcceptTimeoutsKeepsBlockingWhatASlowVariableDoesNotExplain(t *testing.T) {
	base := testDashboard(t, instVariable, testPanel{id: "a", title: "A", datasource: "prom", query: `up{instance="$inst"}`})
	edited := testDashboard(t, `{"name":"inst","type":"query","datasource":"prom","query":"label_values(vq2, instance)"}`, testPanel{id: "a", title: "A", datasource: "prom", query: `up{instance="$inst"}`})
	unresolved := variableUnresolvedAt("/panels/0/queries/0/query")
	cases := []struct {
		name   string
		prev   *spec.Dashboard
		next   *spec.Dashboard
		issues []spec.Issue
	}{
		{"a variable that changed", base, edited, []spec.Issue{timeoutAt("/variables/0")}},
		{"a variable with another failure", base, base, []spec.Issue{{Path: "/variables/0", Code: queryproxy.CodeVariableEmpty, Message: "empty"}}},
		{"a dependent panel without a slow variable", base, base, []spec.Issue{unresolved}},
	}
	for _, c := range cases {
		blocking, accepted := acceptTimeouts(c.prev, c.next, c.issues)
		if !accepted.Empty() || len(blocking) != len(c.issues) {
			t.Errorf("%s: accepted %v, blocking %v, want nothing accepted", c.name, accepted, blocking)
		}
	}
}

func TestAcceptTimeoutsKeepsAPanelBlockingThatDoesNotReachTheSlowVariable(t *testing.T) {
	base := testDashboard(t, instVariable, testPanel{id: "a", title: "A", datasource: "prom", query: "up"})
	unresolved := variableUnresolvedAt("/panels/0/queries/0/query")
	blocking, accepted := acceptTimeouts(base, base, []spec.Issue{timeoutAt("/variables/0"), unresolved})
	if len(blocking) != 1 || blocking[0] != unresolved || len(accepted.Variables) != 1 || len(accepted.Panels) != 0 {
		t.Fatalf("blocking %v, accepted %v, want only the panel issue blocking", blocking, accepted)
	}
}

func TestUnverifiedWithMergesTheIssuesOfAPanelAndAVariableBothSidesAccepted(t *testing.T) {
	empty := issueAt("/panels/0/queries")
	timeout := timeoutAt("/panels/0/queries/0/query")
	a := Unverified{Panels: []UnverifiedPanel{{PanelID: "a", Issues: []spec.Issue{empty}}}, Variables: []UnverifiedVariable{{Name: "v", Issues: []spec.Issue{timeoutAt("/variables/0")}}}}
	b := Unverified{Panels: []UnverifiedPanel{{PanelID: "a", Issues: []spec.Issue{timeout, empty}}, {PanelID: "b", Issues: []spec.Issue{timeout}}}, Variables: []UnverifiedVariable{{Name: "v", Issues: []spec.Issue{variableUnresolvedAt("/variables/0")}}, {Name: "w", Issues: []spec.Issue{timeoutAt("/variables/1")}}}}
	got := a.with(b)
	if len(got.Panels) != 2 || len(got.Panels[0].Issues) != 2 || got.Panels[0].Issues[0] != empty || got.Panels[0].Issues[1] != timeout {
		t.Fatalf("panels %v, want a with its empty and timeout issues once each, then b", got.Panels)
	}
	if len(got.Variables) != 2 || len(got.Variables[0].Issues) != 2 {
		t.Fatalf("variables %v, want v with both issues, then w", got.Variables)
	}
	if len(a.Panels[0].Issues) != 1 || len(a.Variables[0].Issues) != 1 {
		t.Fatalf("with changed its receiver: %v", a)
	}
}
