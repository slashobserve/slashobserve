package tools

import (
	"context"
	"testing"

	"github.com/slashobserve/slashobserve/internal/annotations"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/forbidden"
)

func TestAnEditorRefusedAnAdminToolIsToldTheRoleTheToolNeeds(t *testing.T) {
	r := New(Deps{})
	editor := auth.WithPrincipal(reading(context.Background()), auth.Principal{Kind: auth.KindUser, ID: editorCaller.ID, Role: auth.RoleEditor})
	res, err := r.Call(editor, nameRunDashboardHealth, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := res.Content.(errorBody)
	if !res.IsError || body.Code != codeForbiddenRole || body.NeededRole != string(auth.RoleAdmin) {
		t.Fatalf("refusal = %#v, want forbidden_role needing admin", res.Content)
	}
}

func TestAServiceRefusalKeepsTheRoleOrAccessItNamed(t *testing.T) {
	cases := []struct {
		name         string
		err          error
		role, access string
	}{
		{"a role", forbidden.Need(annotations.ErrForbidden, forbidden.NeedsRole("editor")), "editor", ""},
		{"folder access", forbidden.Need(annotations.ErrForbidden, forbidden.NeedsAccess("edit", "Payments")), "", "edit"},
		{"nothing named", annotations.ErrForbidden, "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res, err := annotationFailure(c.err)
			body, _ := res.Content.(errorBody)
			if err != nil || body.Code != codeForbiddenRole || body.NeededRole != c.role || body.NeededAccess != c.access {
				t.Fatalf("refusal = %#v, %v", res.Content, err)
			}
		})
	}
}
