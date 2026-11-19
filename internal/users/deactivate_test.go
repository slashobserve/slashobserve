package users

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/opaquetoken"
)

func decodePerson(t *testing.T, code int, rec *httptest.ResponseRecorder) personJSON {
	t.Helper()
	if rec.Code != code {
		t.Fatalf("status = %d, want %d, body %s", rec.Code, code, rec.Body)
	}
	var p personJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func (r *rig) doJSON(method, path, body string, caller auth.Principal) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-Kind", string(caller.Kind))
	req.Header.Set("X-Test-Id", caller.ID)
	req.Header.Set("X-Test-Role", string(caller.Role))
	req.Header.Set("X-Test-Name", caller.Name)
	rec := httptest.NewRecorder()
	r.engine.ServeHTTP(rec, req)
	return rec
}

func adminCaller(id pgtype.UUID, name string) auth.Principal {
	return auth.Principal{Kind: auth.KindUser, ID: id.String(), Name: name, Role: auth.RoleAdmin}
}

func (r *rig) makeAdmin(id pgtype.UUID) {
	r.t.Helper()
	r.db.Exec(r.t, "UPDATE users SET role = 'admin' WHERE id = $1", id)
}

func (r *rig) principalGone(id pgtype.UUID) bool {
	r.t.Helper()
	_, err := r.db.Queries().GetUserPrincipal(r.t.Context(), id)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		r.t.Fatal(err)
	}
	return errors.Is(err, pgx.ErrNoRows)
}

func TestDeactivateEndsSessionsAndAgentsAndKeepsThePerson(t *testing.T) {
	r := newRig(t)
	admin := r.addUser("ada", time.Now())
	r.makeAdmin(admin)
	bob := r.addUser("bob", time.Now())
	r.addSession(bob, "bob-1")
	r.addGrant(bob, "client-a")

	p := decodePerson(t, http.StatusOK, r.do(http.MethodPost, "/api/v1/users/"+bob.String()+"/deactivate", adminCaller(admin, "Ada")))

	if p.DeactivatedAt == nil || p.DeactivatedBy == nil || *p.DeactivatedBy != "Ada" {
		t.Fatalf("person = %+v, want deactivated by Ada", p)
	}
	if p.SessionCount != 0 {
		t.Errorf("session_count = %d, want 0", p.SessionCount)
	}
	if got := r.grantState(bob, "client-a"); !got.revoked || got.reason != "admin" {
		t.Errorf("grant = %+v, want revoked by an admin", got)
	}
	if !r.principalGone(bob) {
		t.Error("a deactivated person still resolves as a principal")
	}
	r.addSession(bob, "bob-2")
	if _, err := r.db.Queries().GetSessionUser(t.Context(), opaquetoken.Hash("bob-2")); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("a session of a deactivated person resolves: %v", err)
	}
}

func TestDeactivateDropsCodesAnAgentHasNotExchangedYet(t *testing.T) {
	r := newRig(t)
	admin := r.addUser("ada", time.Now())
	r.makeAdmin(admin)
	bob := r.addUser("bob", time.Now())
	r.addGrant(bob, "client-a")
	code := opaquetoken.Hash("bob-code")
	r.db.Exec(t, "INSERT INTO oauth_codes (code_hash, client_id, user_id, redirect_uri, code_challenge, resource, expires_at) VALUES ($1, 'client-a', $2, 'http://127.0.0.1:1/cb', 'challenge', 'http://localhost:8080/mcp', now() + interval '1 minute')", code, bob)

	decodePerson(t, http.StatusOK, r.do(http.MethodPost, "/api/v1/users/"+bob.String()+"/deactivate", adminCaller(admin, "Ada")))
	decodePerson(t, http.StatusOK, r.do(http.MethodPost, "/api/v1/users/"+bob.String()+"/reactivate", adminCaller(admin, "Ada")))

	err := r.db.InTx(t.Context(), func(q *dbsqlc.Queries) error {
		_, err := q.GetOAuthCode(t.Context(), code)
		return err
	})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("a code Bob approved before deactivation can still be exchanged after reactivation: %v", err)
	}
}

func TestKeysADeactivatedPersonCreatedKeepWorking(t *testing.T) {
	r := newRig(t)
	admin := r.addUser("ada", time.Now())
	r.makeAdmin(admin)
	bob := r.addUser("bob", time.Now())
	r.makeAdmin(bob)
	ctx := t.Context()
	account, err := r.db.Queries().CreateServiceAccount(ctx, dbsqlc.CreateServiceAccountParams{Name: "ci", CreatedBy: bob, Role: "editor"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.db.Queries().CreateServiceAccountKey(ctx, dbsqlc.CreateServiceAccountKeyParams{
		ServiceAccountID: account.ID, Name: "deploy", DisplayPrefix: "so_test", TokenHash: opaquetoken.Hash("bob-key"), CreatedBy: bob,
	}); err != nil {
		t.Fatal(err)
	}

	decodePerson(t, http.StatusOK, r.do(http.MethodPost, "/api/v1/users/"+bob.String()+"/deactivate", adminCaller(admin, "Ada")))

	key, err := r.db.Queries().GetKeyPrincipal(ctx, opaquetoken.Hash("bob-key"))
	if err != nil {
		t.Fatalf("a key Bob created stopped working when Bob was deactivated: %v", err)
	}
	if key.ServiceAccountID != account.ID {
		t.Fatalf("key resolves to %v, want the ci account %v", key.ServiceAccountID, account.ID)
	}
}

func TestDeactivateRefusesYourselfAndTheLastActiveAdmin(t *testing.T) {
	r := newRig(t)
	ada := r.addUser("ada", time.Now())
	bob := r.addUser("bob", time.Now())
	r.makeAdmin(ada)
	r.makeAdmin(bob)

	rec := r.do(http.MethodPost, "/api/v1/users/"+ada.String()+"/deactivate", adminCaller(ada, "Ada"))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"code":"cannot_deactivate_self"`) {
		t.Fatalf("deactivating yourself = %d %s, want 409 cannot_deactivate_self", rec.Code, rec.Body)
	}
	decodePerson(t, http.StatusOK, r.do(http.MethodPost, "/api/v1/users/"+bob.String()+"/deactivate", adminCaller(ada, "Ada")))

	cy := r.addUser("cy", time.Now())
	rec = r.do(http.MethodPost, "/api/v1/users/"+ada.String()+"/deactivate", adminCaller(cy, "Cy"))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"code":"last_admin"`) {
		t.Fatalf("deactivating the last active Admin = %d %s, want 409 last_admin", rec.Code, rec.Body)
	}
}

func TestLoweringTheLastActiveAdminIsRefusedWhileAnotherAdminIsDeactivated(t *testing.T) {
	r := newRig(t)
	r.addUser("first", time.Now().Add(-time.Hour))
	ada := r.addUser("ada", time.Now())
	bob := r.addUser("bob", time.Now())
	r.makeAdmin(ada)
	r.makeAdmin(bob)
	decodePerson(t, http.StatusOK, r.do(http.MethodPost, "/api/v1/users/"+bob.String()+"/deactivate", adminCaller(ada, "Ada")))

	rec := r.doJSON(http.MethodPatch, "/api/v1/users/"+ada.String(), `{"role":"editor"}`, adminCaller(ada, "Ada"))

	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"code":"last_admin"`) {
		t.Fatalf("lowering the last active Admin = %d %s, want 409 last_admin", rec.Code, rec.Body)
	}
}

func TestDeactivateTwiceAnswersTheSamePersonAndReactivateRestores(t *testing.T) {
	r := newRig(t)
	admin := r.addUser("ada", time.Now())
	r.makeAdmin(admin)
	bob := r.addUser("bob", time.Now())
	path := "/api/v1/users/" + bob.String()

	first := decodePerson(t, http.StatusOK, r.do(http.MethodPost, path+"/deactivate", adminCaller(admin, "Ada")))
	second := decodePerson(t, http.StatusOK, r.do(http.MethodPost, path+"/deactivate", adminCaller(admin, "Someone else")))
	if *first.DeactivatedAt != *second.DeactivatedAt || *second.DeactivatedBy != "Ada" {
		t.Fatalf("second deactivate changed the record: %+v then %+v", first, second)
	}

	back := decodePerson(t, http.StatusOK, r.do(http.MethodPost, path+"/reactivate", adminCaller(admin, "Ada")))
	if back.DeactivatedAt != nil || back.DeactivatedBy != nil {
		t.Fatalf("reactivated person = %+v, want no deactivation", back)
	}
	if r.principalGone(bob) {
		t.Error("a reactivated person does not resolve as a principal")
	}
	decodePerson(t, http.StatusOK, r.do(http.MethodPost, path+"/reactivate", adminCaller(admin, "Ada")))
}

func TestDeactivateAndReactivateAnswerNotFoundForAnUnknownPerson(t *testing.T) {
	r := newRig(t)
	admin := r.addUser("ada", time.Now())
	r.makeAdmin(admin)
	for _, action := range []string{"deactivate", "reactivate"} {
		rec := r.do(http.MethodPost, "/api/v1/users/"+nilID+"/"+action, adminCaller(admin, "Ada"))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s an unknown person = %d %s, want 404", action, rec.Code, rec.Body)
		}
	}
}
