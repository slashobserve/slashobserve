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
