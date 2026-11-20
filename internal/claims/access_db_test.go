package claims

import (
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
)

func TestABuiltInRunActsWithItsRequestersCurrentRoleOnEveryCall(t *testing.T) {
	f := newFixture(t)
	f.principals.role.Store(auth.RoleAdmin)
	f.queueBuiltin(t)
	offer := f.claim(t, builtinClaimer)
	roleNow := func() auth.Role {
		t.Helper()
		req, ok := f.authenticate(t, "Bearer "+offer.Session.Token)
		if !ok {
			t.Fatal("the run token was refused")
		}
		return req.RequestedBy.Role
	}
	if got := roleNow(); got != auth.RoleAdmin {
		t.Fatalf("role on the first call = %s", got)
	}
	f.principals.role.Store(auth.RoleViewer)
	if got := roleNow(); got != auth.RoleViewer {
		t.Fatalf("role on the call after the requester was demoted = %s, want viewer", got)
	}
}
