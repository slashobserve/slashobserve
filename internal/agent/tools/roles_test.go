package tools

import (
	"context"
	"errors"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
)

var roleFloors = map[string]auth.Role{
	nameSaveFinding: auth.RoleEditor, nameRunDashboardHealth: auth.RoleAdmin,
	nameSearchAuditEvents: auth.RoleAdmin, nameGetAuditEvent: auth.RoleAdmin, nameListAuditFacets: auth.RoleAdmin,
	nameGetAuditHistogram: auth.RoleAdmin, nameAggregateAuditEvents: auth.RoleAdmin, nameListAuditFields: auth.RoleAdmin,
}

func TestToolRoles(t *testing.T) {
	for _, tool := range newTestRegistry(t, &fakeDS{}, nil).List() {
		want := auth.RoleViewer
		if role, ok := roleFloors[tool.Name]; ok {
			want = role
		}
		if tool.MinRole != want {
			t.Errorf("%s MinRole = %s, want %s: a tool needs the role the web app needs for the same work, and folder access decides the rest", tool.Name, tool.MinRole, want)
		}
	}
}

func TestAllowed(t *testing.T) {
	roles := []auth.Role{auth.RoleViewer, auth.RoleCommenter, auth.RoleEditor, auth.RoleAdmin}
	floors := []auth.Role{auth.RoleViewer, auth.RoleCommenter, auth.RoleEditor, auth.RoleAdmin}
	for i, role := range roles {
		for j, floor := range floors {
			tool := Tool{Name: "t", MinRole: floor}
			if got, want := allowed(auth.Principal{Role: role}, false, tool), i >= j; got != want {
				t.Errorf("allowed(%s, outside a run, %s) = %v, want %v", role, floor, got, want)
			}
			if got, want := allowed(auth.Principal{Role: role}, true, tool), i >= j; got != want {
				t.Errorf("allowed(%s, in a run, %s) = %v, want %v", role, floor, got, want)
			}
			answers := Tool{Name: "t", MinRole: floor, Answers: true}
			if !allowed(auth.Principal{Role: role}, true, answers) {
				t.Errorf("allowed(%s, in a run, answering %s) = false, want true", role, floor)
			}
			if got, want := allowed(auth.Principal{Role: role}, false, answers), i >= j; got != want {
				t.Errorf("allowed(%s, outside a run, answering %s) = %v, want %v", role, floor, got, want)
			}
		}
	}
	for _, floor := range floors {
		if allowed(auth.Principal{}, false, Tool{MinRole: floor}) {
			t.Errorf("the empty principal outside a run is allowed %s", floor)
		}
	}
}

func TestARunRefusesAnAdminToolToAnEditorAndLetsACommenterAnswer(t *testing.T) {
	r := newTestRegistry(t, &fakeDS{}, nil)
	editor := auth.WithPrincipal(reading(context.Background()), auth.Principal{Kind: auth.KindUser, ID: dashB, Name: "n", Role: auth.RoleEditor})
	res, err := r.Call(WithRun(editor, Run{Mode: ModeCreate, RequestID: dashA}), nameSearchAuditEvents, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if errorCode(t, res) != codeForbiddenRole || res.Step != "Refused "+nameSearchAuditEvents+": "+codeForbiddenRole {
		t.Fatalf("res = %#v", res)
	}
	commenter := auth.WithPrincipal(reading(context.Background()), auth.Principal{Kind: auth.KindUser, ID: dashB, Name: "n", Role: auth.RoleCommenter})
	_, err = r.Call(WithRun(commenter, Run{Mode: ModeInvestigate, RequestID: dashA}), nameSaveFinding, []byte(`{}`))
	if !errors.Is(err, errFindingsNotConfigured) {
		t.Fatalf("a commenter's ask did not reach save_finding: %v", err)
	}
}

func TestCallRefusesAToolBelowTheKeysRole(t *testing.T) {
	r := newTestRegistry(t, &fakeDS{}, nil)
	ctx := auth.WithPrincipal(reading(context.Background()), auth.Principal{Role: auth.RoleViewer})
	res, err := r.Call(ctx, nameSaveFinding, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if errorCode(t, res) != "forbidden_role" {
		t.Fatalf("res = %#v", res)
	}
	if want := "this key's role (viewer) cannot use save_finding; an admin can change the service account's role"; res.Content.(errorBody).Message != want {
		t.Fatalf("message = %q, want %q", res.Content.(errorBody).Message, want)
	}
	if want := "Refused save_finding: forbidden_role"; res.Step != want {
		t.Fatalf("step = %q, want %q", res.Step, want)
	}
}

func TestCallRefusesAPersonBelowTheToolsRole(t *testing.T) {
	r := newTestRegistry(t, &fakeDS{}, nil)
	ctx := auth.WithPrincipal(reading(context.Background()), auth.Principal{Kind: auth.KindUser, Role: auth.RoleViewer})
	res, err := r.Call(ctx, nameSaveFinding, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if errorCode(t, res) != "forbidden_role" {
		t.Fatalf("res = %#v", res)
	}
	if want := "your role (viewer) cannot use save_finding; an admin can change your role"; res.Content.(errorBody).Message != want {
		t.Fatalf("message = %q, want %q", res.Content.(errorBody).Message, want)
	}
}
