package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
)

func TestToolRoles(t *testing.T) {
	for _, tool := range newTestRegistry(t, &fakeDS{}, nil).List() {
		switch tool.MinRole {
		case auth.RoleViewer, auth.RoleCommenter, auth.RoleEditor:
		default:
			t.Errorf("%s MinRole = %q, want Viewer, Commenter or Editor", tool.Name, tool.MinRole)
		}
		if !tool.ReadOnly && tool.MinRole == auth.RoleViewer {
			t.Errorf("%s writes but stops at Viewer; writes need at least Commenter", tool.Name)
		}
		if tool.MinRole == auth.RoleCommenter && !strings.HasSuffix(tool.Name, "_comment") {
			t.Errorf("%s stops at Commenter; only comment tools stop at Commenter", tool.Name)
		}
	}
}

func TestAllowed(t *testing.T) {
	roles := []auth.Role{auth.RoleViewer, auth.RoleCommenter, auth.RoleEditor, auth.RoleAdmin}
	floors := []auth.Role{auth.RoleViewer, auth.RoleCommenter, auth.RoleEditor}
	for i, role := range roles {
		for j, floor := range floors {
			if got, want := allowed(auth.Principal{Role: role}, false, floor), i >= j; got != want {
				t.Errorf("allowed(%s, outside a run, %s) = %v, want %v", role, floor, got, want)
			}
			if !allowed(auth.Principal{Role: role}, true, floor) {
				t.Errorf("allowed(%s, in a run, %s) = false, want true", role, floor)
			}
		}
	}
	for _, floor := range floors {
		if allowed(auth.Principal{}, false, floor) {
			t.Errorf("the empty principal outside a run is allowed %s", floor)
		}
	}
}

func TestCallRefusesAToolBelowTheKeysRole(t *testing.T) {
	r := newTestRegistry(t, &fakeDS{}, nil)
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{Role: auth.RoleViewer})
	res, err := r.Call(ctx, nameCreateDashboard, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if errorCode(t, res) != "forbidden_role" {
		t.Fatalf("res = %#v", res)
	}
	if want := "this key's role (viewer) cannot use create_dashboard; an admin can change the service account's role"; res.Content.(errorBody).Message != want {
		t.Fatalf("message = %q, want %q", res.Content.(errorBody).Message, want)
	}
	if want := "Refused create_dashboard: forbidden_role"; res.Step != want {
		t.Fatalf("step = %q, want %q", res.Step, want)
	}
}
