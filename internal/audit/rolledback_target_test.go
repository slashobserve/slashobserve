package audit

import "testing"

func TestARolledBackTargetComesFromTheTransactionOnlyForTheSameAction(t *testing.T) {
	declared := Target{Kind: "dashboard", ID: "route-id"}
	written := written{action: "dashboard.update", target: Target{Kind: "dashboard", ID: "route-id", Name: "Checkout"}}
	cases := map[string]struct {
		declaredAction Action
		want           Target
	}{
		"no declared action": {"", written.target},
		"the same action":    {"dashboard.update", written.target},
		"a different action": {"dashboard.restore", declared},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := rolledBackTarget(c.declaredAction, declared, written); got != c.want {
				t.Fatalf("target = %+v, want %+v", got, c.want)
			}
		})
	}
}

func TestARolledBackTargetKeepsTheDeclaredOneWhenTheTransactionNamedNone(t *testing.T) {
	declared := Target{Kind: "dashboard", ID: "route-id"}
	got := rolledBackTarget("dashboard.update", declared, written{action: "dashboard.update"})
	if got != declared {
		t.Fatalf("target = %+v, want %+v", got, declared)
	}
}
