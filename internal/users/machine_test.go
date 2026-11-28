package users

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

func TestPersonMachineIsWellFormed(t *testing.T) {
	if err := PersonMachine.Check(); err != nil {
		t.Fatal(err)
	}
}

func TestAPersonCarriesItsStateAndTheActionsTheCallerMayTake(t *testing.T) {
	ada := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	bob := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	admin := auth.Principal{Kind: auth.KindUser, ID: ada.String(), Role: auth.RoleAdmin}
	key := auth.Principal{Kind: auth.KindServiceAccount, ID: ada.String(), Role: auth.RoleAdmin}
	deactivated := pgtype.Timestamptz{Time: time.Unix(1, 0), Valid: true}
	cases := []struct {
		name   string
		row    dbsqlc.ListUsersRow
		caller auth.Principal
		state  State
		want   []Action
	}{
		{"an active person", dbsqlc.ListUsersRow{ID: bob, RoleSource: "default"}, admin, StateActive, []Action{ActionSetRole, ActionSignOut, ActionDeactivate}},
		{"a locked role", dbsqlc.ListUsersRow{ID: bob, RoleSource: "group"}, admin, StateActive, []Action{ActionSignOut, ActionDeactivate}},
		{"yourself", dbsqlc.ListUsersRow{ID: ada, RoleSource: "manual"}, admin, StateActive, []Action{ActionSetRole, ActionSignOut}},
		{"a deactivated person", dbsqlc.ListUsersRow{ID: bob, RoleSource: "default", DeactivatedAt: deactivated}, admin, StateDeactivated, []Action{ActionSetRole, ActionReactivate, ActionRemove}},
		{"a deactivated person with a locked role", dbsqlc.ListUsersRow{ID: bob, RoleSource: "first_user", DeactivatedAt: deactivated}, admin, StateDeactivated, []Action{ActionReactivate, ActionRemove}},
		{"read with a key, which cannot change people", dbsqlc.ListUsersRow{ID: bob, RoleSource: "default"}, key, StateActive, []Action{}},
	}
	for _, c := range cases {
		p := newPersonJSON(c.row, c.caller)
		if p.State != c.state || !slices.Equal(p.AllowedActions, c.want) {
			t.Errorf("%s: state %q actions %v, want %q %v", c.name, p.State, p.AllowedActions, c.state, c.want)
		}
		raw, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		var wire map[string]json.RawMessage
		if err := json.Unmarshal(raw, &wire); err != nil {
			t.Fatal(err)
		}
		if string(wire["allowed_actions"]) == "null" || wire["state"] == nil {
			t.Errorf("%s: wire state %s allowed_actions %s", c.name, wire["state"], wire["allowed_actions"])
		}
	}
}

func decodeRefusal(t *testing.T, code string, rec *httptest.ResponseRecorder) refusalJSON {
	t.Helper()
	var body refusalJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusConflict || body.Code != code {
		t.Fatalf("refusal = %d %s, want 409 %s", rec.Code, rec.Body, code)
	}
	return body
}

func TestARefusalNamesThePersonsStateAndWhatTheCallerMayStillDo(t *testing.T) {
	r := newRig(t)
	r.addUser("first", time.Now().Add(-2*time.Hour))
	ada := r.addUser("ada", time.Now().Add(-time.Hour))
	r.makeAdmin(ada)
	bob := r.addUser("bob", time.Now())
	r.db.Exec(t, "UPDATE users SET role_source = 'group' WHERE id = $1", bob)
	admin := adminCaller(ada, "Ada")

	self := decodeRefusal(t, "cannot_deactivate_self", r.do(http.MethodPost, "/api/v1/users/"+ada.String()+"/deactivate", admin))
	if self.State != StateActive || slices.Contains(self.AllowedActions, ActionDeactivate) || !slices.Contains(self.AllowedActions, ActionSignOut) {
		t.Errorf("deactivating yourself = %+v, want active without deactivate", self)
	}
	active := decodeRefusal(t, "not_deactivated", r.do(http.MethodDelete, "/api/v1/users/"+bob.String(), admin))
	if active.State != StateActive || !slices.Equal(active.AllowedActions, []Action{ActionSignOut, ActionDeactivate}) {
		t.Errorf("removing an active person = %+v, want active with sign_out and deactivate", active)
	}
	locked := decodeRefusal(t, "role_locked", r.doJSON(http.MethodPatch, "/api/v1/users/"+bob.String(), `{"role":"editor"}`, admin))
	if locked.State != StateActive || slices.Contains(locked.AllowedActions, ActionSetRole) {
		t.Errorf("changing a locked role = %+v, want active without set_role", locked)
	}
	last := decodeRefusal(t, "last_admin", r.doJSON(http.MethodPatch, "/api/v1/users/"+ada.String(), `{"role":"viewer"}`, admin))
	if last.State != StateActive || last.Error == "" {
		t.Errorf("lowering the last admin = %+v, want the state and the message", last)
	}
}
