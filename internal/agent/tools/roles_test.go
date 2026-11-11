package tools

import (
	"context"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
)

var investigateRoleTools = map[string]auth.Role{nameCreateFinding: auth.RoleEditor, nameSaveFinding: auth.RoleEditor}

func TestToolRoles(t *testing.T) {
	for _, tool := range newTestRegistry(t, &fakeDS{}, nil).List() {
		want := auth.RoleViewer
		if role, ok := investigateRoleTools[tool.Name]; ok {
			want = role
		}
		if tool.MinRole != want {
			t.Errorf("%s MinRole = %s, want %s: a tool needs the role the web app needs for the same work, and folder access decides the rest", tool.Name, tool.MinRole, want)
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
	res, err := r.Call(ctx, nameCreateFinding, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if errorCode(t, res) != "forbidden_role" {
		t.Fatalf("res = %#v", res)
	}
	if want := "this key's role (viewer) cannot use create_finding; an admin can change the service account's role"; res.Content.(errorBody).Message != want {
		t.Fatalf("message = %q, want %q", res.Content.(errorBody).Message, want)
	}
	if want := "Refused create_finding: forbidden_role"; res.Step != want {
		t.Fatalf("step = %q, want %q", res.Step, want)
	}
}

func TestCallRefusesAPersonBelowTheToolsRole(t *testing.T) {
	r := newTestRegistry(t, &fakeDS{}, nil)
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{Kind: auth.KindUser, Role: auth.RoleViewer})
	res, err := r.Call(ctx, nameCreateFinding, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if errorCode(t, res) != "forbidden_role" {
		t.Fatalf("res = %#v", res)
	}
	if want := "your role (viewer) cannot use create_finding; an admin can change your role"; res.Content.(errorBody).Message != want {
		t.Fatalf("message = %q, want %q", res.Content.(errorBody).Message, want)
	}
}
