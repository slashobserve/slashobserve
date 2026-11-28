package users

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
)

func TestInviteIsOnePendingRowPerEmailAndARetryChangesOnlyTheRole(t *testing.T) {
	r := newRig(t)
	admin := r.addUser("ada", time.Now())
	caller := adminCaller(admin, "Ada")

	first := r.doJSON(http.MethodPost, "/api/v1/invites", `{"email":"New@example.com","role":"viewer"}`, caller)
	if first.Code != http.StatusCreated {
		t.Fatalf("first invite = %d %s, want 201", first.Code, first.Body)
	}
	again := r.doJSON(http.MethodPost, "/api/v1/invites", `{"email":"new@EXAMPLE.com","role":"commenter"}`, caller)
	if again.Code != http.StatusOK {
		t.Fatalf("second invite = %d %s, want 200", again.Code, again.Body)
	}
	var a, b inviteJSON
	if err := json.Unmarshal(first.Body.Bytes(), &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(again.Body.Bytes(), &b); err != nil {
		t.Fatal(err)
	}
	if a.ID != b.ID || b.Role != auth.RoleCommenter || b.Email != "New@example.com" || b.InvitedBy != "Ada" || b.SignInURL != "http://localhost:8080/login" {
		t.Fatalf("invites = %+v then %+v, want one invite now Commenter", a, b)
	}

	rec := r.do(http.MethodGet, "/api/v1/invites", caller)
	var list inviteListJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || len(list.Invites) != 1 || list.NextCursor != nil {
		t.Fatalf("list = %d %s, want one invite", rec.Code, rec.Body)
	}
}

func TestAnInviteListsWhetherSomeoneWithItsEmailHasSignedInAlready(t *testing.T) {
	r := newRig(t)
	admin := r.addUser("ada", time.Now())
	caller := adminCaller(admin, "Ada")
	for _, email := range []string{"Bo@example.com", "cy@example.com"} {
		if rec := r.doJSON(http.MethodPost, "/api/v1/invites", `{"email":"`+email+`","role":"editor"}`, caller); rec.Code != http.StatusCreated || strings.Contains(rec.Body.String(), `"member":true`) {
			t.Fatalf("invite %s = %d %s, want 201 with member false", email, rec.Code, rec.Body)
		}
	}
	r.addUser("bo", time.Now())

	rec := r.do(http.MethodGet, "/api/v1/invites", caller)
	var list inviteListJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("list = %d %s", rec.Code, rec.Body)
	}
	member := map[string]bool{}
	for _, i := range list.Invites {
		member[i.Email] = i.Member
	}
	if len(member) != 2 || !member["Bo@example.com"] || member["cy@example.com"] {
		t.Fatalf("members = %v, want Bo's invite marked and Cy's not", member)
	}
}

func TestInviteRefusesMembersOtherDomainsAndBadInput(t *testing.T) {
	r := newRig(t)
	admin := r.addUser("ada", time.Now())
	caller := adminCaller(admin, "Ada")
	for _, tc := range []struct {
		body string
		code int
		want string
	}{
		{`{"email":"ADA@example.com","role":"viewer"}`, http.StatusConflict, "already_member"},
		{`{"email":"new@other.io","role":"viewer"}`, http.StatusUnprocessableEntity, "email_domain_not_allowed"},
		{`{"email":"not-an-email","role":"viewer"}`, http.StatusBadRequest, "invalid_request"},
		{`{"email":"new@example.com","role":"owner"}`, http.StatusBadRequest, "invalid_request"},
		{`{"email":"new@example.com"}`, http.StatusBadRequest, "invalid_request"},
	} {
		rec := r.doJSON(http.MethodPost, "/api/v1/invites", tc.body, caller)
		if rec.Code != tc.code || !strings.Contains(rec.Body.String(), `"code":"`+tc.want+`"`) {
			t.Errorf("%s = %d %s, want %d %s", tc.body, rec.Code, rec.Body, tc.code, tc.want)
		}
	}
}

func TestAnAdminKeyMayInviteAndOtherRolesMayNot(t *testing.T) {
	r := newRig(t)
	key := auth.Principal{Kind: auth.KindServiceAccount, ID: "3f1c2a52-8c5e-4a53-9a3e-1d2d4c0a9b11", Name: "ci", Role: auth.RoleAdmin}
	if rec := r.doJSON(http.MethodPost, "/api/v1/invites", `{"email":"new@example.com","role":"editor"}`, key); rec.Code != http.StatusCreated {
		t.Fatalf("an Admin key's invite = %d %s, want 201", rec.Code, rec.Body)
	}
	key.Role = auth.RoleEditor
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		if rec := r.doJSON(method, "/api/v1/invites", `{}`, key); rec.Code != http.StatusForbidden {
			t.Errorf("%s /invites as an Editor key = %d, want 403", method, rec.Code)
		}
	}
}

func TestWithdrawAnInviteOnceThenNotFound(t *testing.T) {
	r := newRig(t)
	admin := r.addUser("ada", time.Now())
	caller := adminCaller(admin, "Ada")
	rec := r.doJSON(http.MethodPost, "/api/v1/invites", `{"email":"new@example.com","role":"viewer"}`, caller)
	var inv inviteJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &inv); err != nil {
		t.Fatal(err)
	}
	if rec := r.do(http.MethodDelete, "/api/v1/invites/"+inv.ID, caller); rec.Code != http.StatusNoContent {
		t.Fatalf("withdraw = %d %s, want 204", rec.Code, rec.Body)
	}
	if rec := r.do(http.MethodDelete, "/api/v1/invites/"+inv.ID, caller); rec.Code != http.StatusNotFound {
		t.Fatalf("second withdraw = %d %s, want 404", rec.Code, rec.Body)
	}
}
