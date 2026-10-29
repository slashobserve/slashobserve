package tools

import (
	"context"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
)

func TestToolRoles(t *testing.T) {
	want := map[string]auth.Role{
		nameListDashboards:  auth.RoleViewer,
		nameGetDashboard:    auth.RoleViewer,
		nameGetSpecGuide:    auth.RoleViewer,
		nameListComments:    auth.RoleViewer,
		nameReplyToComment:  auth.RoleCommenter,
		nameResolveComment:  auth.RoleCommenter,
		nameListDatasources: auth.RoleEditor,
		nameListMetrics:     auth.RoleEditor,
		nameListLabels:      auth.RoleEditor,
		nameListLabelValues: auth.RoleEditor,
		nameGetSchema:       auth.RoleEditor,
		nameSampleRows:      auth.RoleEditor,
		nameLintQuery:       auth.RoleEditor,
		nameQueryRange:      auth.RoleEditor,
		nameQueryInstant:    auth.RoleEditor,
		nameCreateDashboard: auth.RoleEditor,
		nameUpdateDashboard: auth.RoleEditor,
		nameImportGrafana:   auth.RoleEditor,
	}
	listed := newTestRegistry(t, &fakeDS{}, nil).List()
	if len(listed) != len(want) {
		t.Fatalf("registry has %d tools, table has %d", len(listed), len(want))
	}
	for _, tool := range listed {
		role, ok := want[tool.Name]
		if !ok {
			t.Errorf("%s is not in the role table", tool.Name)
			continue
		}
		if tool.MinRole != role {
			t.Errorf("%s MinRole = %q, want %q", tool.Name, tool.MinRole, role)
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
