package revisions

import (
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
)

func TestClaimantKey(t *testing.T) {
	cases := []struct {
		name string
		c    Claimant
		want string
	}{
		{"built-in agent of one server", Claimant{Kind: AssignBuiltin, ID: "srv-1"}, "builtin:srv-1"},
		{"built-in agent with no server id", Claimant{Kind: AssignBuiltin}, "builtin"},
		{"connected agent", Claimant{Kind: AssignGrant, ID: "g-1", Person: "u-1"}, "grant:g-1"},
		{"service account", Claimant{Kind: AssignServiceAccount, ID: "s-1"}, "service_account:s-1"},
	}
	for _, c := range cases {
		if got := c.c.Key(); got != c.want {
			t.Errorf("%s: Key() = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestActsFor(t *testing.T) {
	user := func(id string) auth.Principal { return auth.Principal{Kind: auth.KindUser, ID: id} }
	cases := []struct {
		name      string
		c         Claimant
		requester auth.Principal
		want      bool
	}{
		{"built-in agent for anyone", Claimant{Kind: AssignBuiltin}, user("u-2"), true},
		{"service account for anyone", Claimant{Kind: AssignServiceAccount, ID: "s-1"}, user("u-2"), true},
		{"grant for its own person", Claimant{Kind: AssignGrant, ID: "g-1", Person: "u-1"}, user("u-1"), true},
		{"grant for another person", Claimant{Kind: AssignGrant, ID: "g-1", Person: "u-1"}, user("u-2"), false},
		{"grant for a service account with the person's id", Claimant{Kind: AssignGrant, ID: "g-1", Person: "u-1"}, auth.Principal{Kind: auth.KindServiceAccount, ID: "u-1"}, false},
	}
	for _, c := range cases {
		if got := actsFor(c.c, c.requester); got != c.want {
			t.Errorf("%s: actsFor = %v, want %v", c.name, got, c.want)
		}
	}
}
