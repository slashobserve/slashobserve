package users

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"
	"time"
)

type recorded struct {
	Type     string
	TargetID string
	Actor    string
	Data     string
}

func (r *rig) peopleActivity() []recorded {
	r.t.Helper()
	rows, err := r.db.SQLDB(r.t).QueryContext(r.t.Context(), `SELECT type, target_id::text, actor_name, data::text FROM activity WHERE target_kind = 'user' ORDER BY seq`)
	if err != nil {
		r.t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []recorded
	for rows.Next() {
		var e recorded
		if err := rows.Scan(&e.Type, &e.TargetID, &e.Actor, &e.Data); err != nil {
			r.t.Fatal(err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		r.t.Fatal(err)
	}
	return out
}

func sameJSON(t *testing.T, got, want string) bool {
	t.Helper()
	var a, b any
	if err := json.Unmarshal([]byte(got), &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(want), &b); err != nil {
		t.Fatal(err)
	}
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

func TestInvitesAreRecordedAndAnUnchangedRoleIsNot(t *testing.T) {
	r := newRig(t)
	admin := r.addUser("ada", time.Now())
	caller := adminCaller(admin, "Ada")

	var inv inviteJSON
	for _, body := range []string{
		`{"email":"New@example.com","role":"editor"}`,
		`{"email":"new@example.com","role":"editor"}`,
		`{"email":"new@example.com","role":"viewer"}`,
	} {
		rec := r.doJSON(http.MethodPost, "/api/v1/invites", body, caller)
		if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
			t.Fatalf("invite %s = %d %s", body, rec.Code, rec.Body)
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &inv); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		r.do(http.MethodDelete, "/api/v1/invites/"+inv.ID, caller)
	}
	if rec := r.doJSON(http.MethodPost, "/api/v1/invites", `{"email":"ADA@example.com","role":"viewer"}`, caller); rec.Code != http.StatusConflict {
		t.Fatalf("inviting a member = %d %s", rec.Code, rec.Body)
	}

	got := r.peopleActivity()
	want := []recorded{
		{"user.invited", inv.ID, "Ada", `{"email":"New@example.com","role":"editor"}`},
		{"user.invite_updated", inv.ID, "Ada", `{"email":"New@example.com","role":{"from":"editor","to":"viewer"}}`},
		{"user.invite_withdrawn", inv.ID, "Ada", `{"email":"New@example.com"}`},
	}
	if len(got) != len(want) {
		t.Fatalf("recorded %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i].Type != want[i].Type || got[i].TargetID != want[i].TargetID || got[i].Actor != want[i].Actor || !sameJSON(t, got[i].Data, want[i].Data) {
			t.Errorf("event %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestDeactivateReactivateAndRemoveAreRecordedOncePerChange(t *testing.T) {
	r := newRig(t)
	admin := r.addUser("ada", time.Now())
	r.makeAdmin(admin)
	bob := r.addUser("bob", time.Now())
	r.addSession(bob, "bob-session")
	r.addGrant(bob, "client-a")
	caller := adminCaller(admin, "Ada")
	path := "/api/v1/users/" + bob.String()

	for _, step := range []struct {
		method, path string
		code         int
	}{
		{http.MethodPost, path + "/reactivate", http.StatusOK},
		{http.MethodPost, path + "/deactivate", http.StatusOK},
		{http.MethodPost, path + "/deactivate", http.StatusOK},
		{http.MethodPost, path + "/reactivate", http.StatusOK},
		{http.MethodPost, path + "/reactivate", http.StatusOK},
		{http.MethodPost, path + "/deactivate", http.StatusOK},
		{http.MethodDelete, path, http.StatusNoContent},
		{http.MethodDelete, path, http.StatusNotFound},
	} {
		if rec := r.do(step.method, step.path, caller); rec.Code != step.code {
			t.Fatalf("%s %s = %d %s, want %d", step.method, step.path, rec.Code, rec.Body, step.code)
		}
	}

	got := r.peopleActivity()
	types := make([]string, len(got))
	for i, e := range got {
		types[i] = e.Type
		if e.TargetID != bob.String() || e.Actor != "Ada" {
			t.Errorf("event %d = %+v, want Bob as target and Ada as actor", i, e)
		}
	}
	wantTypes := []string{"user.deactivated", "user.reactivated", "user.deactivated", "user.removed"}
	if !slices.Equal(types, wantTypes) {
		t.Fatalf("recorded %v, want %v", types, wantTypes)
	}
	if !sameJSON(t, got[0].Data, `{"name":"bob","sessions":1,"grants":1}`) {
		t.Errorf("first deactivation data = %s", got[0].Data)
	}
	for _, i := range []int{1, 3} {
		if !sameJSON(t, got[i].Data, `{"name":"bob"}`) {
			t.Errorf("%s data = %s", got[i].Type, got[i].Data)
		}
	}
}

func TestARefusedDeactivationRecordsNothing(t *testing.T) {
	r := newRig(t)
	admin := r.addUser("ada", time.Now())
	r.makeAdmin(admin)

	if rec := r.do(http.MethodPost, "/api/v1/users/"+admin.String()+"/deactivate", adminCaller(admin, "Ada")); rec.Code != http.StatusConflict {
		t.Fatalf("deactivating yourself = %d %s, want 409", rec.Code, rec.Body)
	}
	if rec := r.do(http.MethodDelete, "/api/v1/users/"+admin.String(), adminCaller(admin, "Ada")); rec.Code != http.StatusConflict {
		t.Fatalf("removing an active person = %d %s, want 409", rec.Code, rec.Body)
	}

	if got := r.peopleActivity(); len(got) != 0 {
		t.Fatalf("recorded %+v, want nothing", got)
	}
}
