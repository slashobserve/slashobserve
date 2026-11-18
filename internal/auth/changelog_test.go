package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

type recordedChanges struct {
	unrecorded
	calls []string
	roles []RoleChange
}

func (r *recordedChanges) RoleChanged(_ context.Context, _ *dbsqlc.Queries, _ Principal, c RoleChange) error {
	r.calls = append(r.calls, "role_changed")
	r.roles = append(r.roles, c)
	return nil
}

func (r *recordedChanges) ServiceAccountCreated(context.Context, *dbsqlc.Queries, Principal, dbsqlc.ServiceAccount) error {
	r.calls = append(r.calls, "created")
	return nil
}

func (r *recordedChanges) ServiceAccountUpdated(context.Context, *dbsqlc.Queries, Principal, dbsqlc.ServiceAccount, dbsqlc.ServiceAccount) error {
	r.calls = append(r.calls, "updated")
	return nil
}

func (r *recordedChanges) KeyCreated(context.Context, *dbsqlc.Queries, Principal, dbsqlc.ServiceAccount, dbsqlc.CreateServiceAccountKeyRow) error {
	r.calls = append(r.calls, "key_created")
	return nil
}

func (r *recordedChanges) KeyRevoked(context.Context, *dbsqlc.Queries, Principal, dbsqlc.ServiceAccount, pgtype.UUID, string) error {
	r.calls = append(r.calls, "key_revoked")
	return nil
}

func TestServiceAccountChangesAreRecordedOncePerChange(t *testing.T) {
	r := newProvisionRig(t)
	log := &recordedChanges{}
	r.service.RecordChanges(r.db.DB, log)

	if rec := r.request(http.MethodPost, "/api/v1/service-accounts", `{"name":"ci","role":"viewer"}`, nil); rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body)
	}
	id := r.accounts()[0].ID
	for _, body := range []string{`{"role":"editor"}`, `{"takes_runs":true}`} {
		if rec := r.request(http.MethodPatch, "/api/v1/service-accounts/"+id, body, nil); rec.Code != http.StatusOK {
			t.Fatalf("patch = %d %s", rec.Code, rec.Body)
		}
	}
	rec := r.request(http.MethodPost, "/api/v1/service-accounts/"+id+"/keys", `{"name":"laptop"}`, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create key = %d %s", rec.Code, rec.Body)
	}
	var created struct {
		Key struct {
			ID string `json:"id"`
		} `json:"key"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if rec := r.request(http.MethodDelete, "/api/v1/service-accounts/"+id+"/keys/"+created.Key.ID, "", nil); rec.Code != http.StatusNoContent {
			t.Fatalf("revoke = %d %s", rec.Code, rec.Body)
		}
	}

	want := []string{"created", "updated", "updated", "key_created", "key_revoked"}
	if !slices.Equal(log.calls, want) {
		t.Fatalf("recorded %v, want %v", log.calls, want)
	}
}

func TestAFailedServiceAccountWriteRecordsNothing(t *testing.T) {
	r := newProvisionRig(t)
	log := &recordedChanges{}
	r.service.RecordChanges(r.db.DB, log)
	const missing = "99999999-9999-4999-8999-999999999999"

	for _, call := range []struct {
		method, path, body string
		status             int
	}{
		{http.MethodPatch, "/api/v1/service-accounts/" + missing, `{"role":"editor"}`, http.StatusNotFound},
		{http.MethodPost, "/api/v1/service-accounts/" + missing + "/keys", `{"name":"k"}`, http.StatusNotFound},
		{http.MethodDelete, "/api/v1/service-accounts/" + missing + "/keys/" + missing, "", http.StatusNotFound},
	} {
		if rec := r.request(call.method, call.path, call.body, nil); rec.Code != call.status {
			t.Fatalf("%s %s = %d %s, want %d", call.method, call.path, rec.Code, rec.Body, call.status)
		}
	}
	if len(log.calls) != 0 {
		t.Fatalf("recorded %v, want nothing", log.calls)
	}
}

func TestARoleChangeIsRecordedWithBeforeAndAfter(t *testing.T) {
	r := newProvisionRig(t)
	log := &recordedChanges{}
	r.service.RecordChanges(r.db.DB, log)
	other, err := r.db.Queries().UpsertUser(t.Context(), dbsqlc.UpsertUserParams{OidcIssuer: "i", OidcSubject: "other", Email: "bob@example.com", Name: "Bob"})
	if err != nil {
		t.Fatal(err)
	}
	by := Principal{Kind: KindUser, ID: "11111111-1111-4111-8111-111111111111", Name: "Ada", Role: RoleAdmin}
	set := func(role *Role) {
		t.Helper()
		if err := r.db.InTx(t.Context(), func(q *dbsqlc.Queries) error { return r.service.SetManualRole(t.Context(), q, by, other.ID, role) }); err != nil {
			t.Fatal(err)
		}
	}
	editor, viewer := RoleEditor, RoleViewer

	set(&editor)
	set(&editor)
	set(&viewer)
	set(nil)

	if len(log.roles) != 3 {
		t.Fatalf("recorded %+v, want three changes: set editor, set viewer, cleared", log.roles)
	}
	if got := log.roles[0]; got.From != RoleViewer || got.To != RoleEditor || !got.Manual || got.Name != "Bob" {
		t.Errorf("first change = %+v", got)
	}
	if got := log.roles[1]; got.From != RoleEditor || got.To != RoleViewer || !got.Manual {
		t.Errorf("second change = %+v", got)
	}
	if got := log.roles[2]; got.Manual {
		t.Errorf("clearing the override = %+v, want Manual false", got)
	}
}
