package users

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/opaquetoken"
)

func decodeTeam(t *testing.T, code int, rec *httptest.ResponseRecorder) teamJSON {
	t.Helper()
	if rec.Code != code {
		t.Fatalf("status = %d, want %d, body %s", rec.Code, code, rec.Body)
	}
	var team teamJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &team); err != nil {
		t.Fatal(err)
	}
	return team
}

func (r *rig) createTeam(name string, caller auth.Principal) teamJSON {
	r.t.Helper()
	rec := r.doJSON(http.MethodPost, "/api/v1/teams", `{"name":"`+name+`"}`, caller)
	return decodeTeam(r.t, http.StatusCreated, rec)
}

func (r *rig) principalGroups(id pgtype.UUID) []string {
	r.t.Helper()
	row, err := r.db.Queries().GetUserPrincipal(r.t.Context(), id)
	if err != nil {
		r.t.Fatal(err)
	}
	return row.Groups
}

func memberIDs(team teamJSON) []string {
	ids := make([]string, 0, len(team.Members))
	for _, m := range team.Members {
		ids = append(ids, m.ID)
	}
	return ids
}

func TestATeamNameJoinsTheGroupsOfItsMembers(t *testing.T) {
	r := newRig(t)
	ada := r.addUser("ada", time.Now())
	bob := r.addUser("bob", time.Now())
	r.db.Exec(t, "UPDATE users SET groups = ARRAY['idp-devs'] WHERE id = $1", bob)
	r.addSession(bob, "bob-1")
	admin := adminCaller(ada, "Ada")
	sre := r.createTeam("sre", admin)
	if sre.CreatedBy != "Ada" || len(sre.Members) != 0 {
		t.Fatalf("new team = %+v, want created by Ada with no members", sre)
	}

	rec := r.do(http.MethodPut, "/api/v1/teams/"+sre.ID+"/members/"+bob.String(), admin)
	if got := memberIDs(decodeTeam(t, http.StatusOK, rec)); !slices.Equal(got, []string{bob.String()}) {
		t.Fatalf("members = %v, want bob", got)
	}
	r.do(http.MethodPut, "/api/v1/teams/"+sre.ID+"/members/"+bob.String(), admin)
	if got := r.principalGroups(bob); !slices.Equal(got, []string{"idp-devs", "sre"}) {
		t.Fatalf("principal groups = %v, want the IdP group and the team", got)
	}
	session, err := r.db.Queries().GetSessionUser(t.Context(), opaquetoken.Hash("bob-1"))
	if err != nil || !slices.Contains(session.Groups, "sre") {
		t.Fatalf("session groups = %v, %v; want sre", session.Groups, err)
	}
	var folder pgtype.UUID
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), "INSERT INTO folders (title, created_by_kind, created_by_id, created_by_name) VALUES ('F', 'user', $1, 'Ada') RETURNING id", ada).Scan(&folder); err != nil {
		t.Fatal(err)
	}
	r.db.Exec(t, "INSERT INTO folder_permissions (folder_id, group_name, level) VALUES ($1, 'sre', 'view')", folder)
	perms, err := r.db.Queries().ListFolderPermissions(t.Context(), folder)
	if err != nil || len(perms) != 1 || perms[0].MemberCount != 1 {
		t.Fatalf("folder grants = %+v, %v; want sre counted with one member", perms, err)
	}

	rec = r.do(http.MethodDelete, "/api/v1/teams/"+sre.ID+"/members/"+bob.String(), admin)
	if got := decodeTeam(t, http.StatusOK, rec); len(got.Members) != 0 {
		t.Fatalf("members after removal = %+v, want none", got.Members)
	}
	if got := r.principalGroups(bob); !slices.Equal(got, []string{"idp-devs"}) {
		t.Fatalf("principal groups after removal = %v, want only the IdP group", got)
	}
}

func TestDeletingATeamOrAPersonEndsTheMembership(t *testing.T) {
	r := newRig(t)
	ada := r.addUser("ada", time.Now())
	bob := r.addUser("bob", time.Now())
	admin := adminCaller(ada, "Ada")
	sre := r.createTeam("sre", admin)
	ops := r.createTeam("ops", admin)
	for _, team := range []teamJSON{sre, ops} {
		r.do(http.MethodPut, "/api/v1/teams/"+team.ID+"/members/"+bob.String(), admin)
	}

	if rec := r.do(http.MethodDelete, "/api/v1/teams/"+sre.ID, admin); rec.Code != http.StatusNoContent {
		t.Fatalf("delete team = %d %s, want 204", rec.Code, rec.Body)
	}
	if got := r.principalGroups(bob); !slices.Equal(got, []string{"ops"}) {
		t.Fatalf("groups after deleting sre = %v, want ops", got)
	}
	if rec := r.do(http.MethodDelete, "/api/v1/teams/"+sre.ID, admin); rec.Code != http.StatusNotFound {
		t.Fatalf("delete twice = %d, want 404", rec.Code)
	}

	r.db.Exec(t, "DELETE FROM users WHERE id = $1", bob)
	var list struct {
		Teams []teamJSON `json:"teams"`
	}
	rec := r.do(http.MethodGet, "/api/v1/teams", admin)
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("list = %d %s", rec.Code, rec.Body)
	}
	if len(list.Teams) != 1 || list.Teams[0].Name != "ops" || len(list.Teams[0].Members) != 0 {
		t.Fatalf("teams = %+v, want ops with no members", list.Teams)
	}
}

func TestTeamRequestsAreChecked(t *testing.T) {
	r := newRig(t)
	ada := r.addUser("ada", time.Now())
	admin := adminCaller(ada, "Ada")
	sre := r.createTeam("sre", admin)
	for _, tc := range []struct {
		method, path, body, code string
		status                   int
	}{
		{http.MethodPost, "/api/v1/teams", `{"name":"SRE"}`, "team_exists", http.StatusConflict},
		{http.MethodPost, "/api/v1/teams", `{"name":"  "}`, "invalid_request", http.StatusBadRequest},
		{http.MethodPost, "/api/v1/teams", `{"name":"a\u0007b"}`, "invalid_request", http.StatusBadRequest},
		{http.MethodPost, "/api/v1/teams", `{"name":"` + strings.Repeat("x", 101) + `"}`, "invalid_request", http.StatusBadRequest},
		{http.MethodPut, "/api/v1/teams/" + sre.ID + "/members/" + nilID, "", "not_found", http.StatusNotFound},
		{http.MethodPut, "/api/v1/teams/" + nilID + "/members/" + ada.String(), "", "not_found", http.StatusNotFound},
		{http.MethodDelete, "/api/v1/teams/" + nilID + "/members/" + ada.String(), "", "not_found", http.StatusNotFound},
	} {
		rec := r.doJSON(tc.method, tc.path, tc.body, admin)
		if rec.Code != tc.status || !strings.Contains(rec.Body.String(), `"code":"`+tc.code+`"`) {
			t.Errorf("%s %s %s = %d %s, want %d %s", tc.method, tc.path, tc.body, rec.Code, rec.Body, tc.status, tc.code)
		}
	}
}

func TestAnAdminKeyManagesTeamsAndAnEditorCannot(t *testing.T) {
	r := newRig(t)
	key := auth.Principal{Kind: auth.KindServiceAccount, ID: nilID, Name: "ci", Role: auth.RoleAdmin}
	if team := r.createTeam("sre", key); team.CreatedBy != "ci" {
		t.Fatalf("created by = %q, want the key's account", team.CreatedBy)
	}
	editor := auth.Principal{Kind: auth.KindUser, ID: nilID, Role: auth.RoleEditor}
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		if rec := r.doJSON(method, "/api/v1/teams", `{"name":"x"}`, editor); rec.Code != http.StatusForbidden {
			t.Errorf("%s /teams as an Editor = %d, want 403", method, rec.Code)
		}
	}
}
