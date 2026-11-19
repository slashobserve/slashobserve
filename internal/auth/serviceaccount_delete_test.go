package auth

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/config"
)

func (r *provisionRig) newKey(accountID string) string {
	r.t.Helper()
	rec := r.request(http.MethodPost, "/api/v1/service-accounts/"+accountID+"/keys", `{"name":"k"}`, nil)
	var body struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != http.StatusCreated {
		r.t.Fatalf("create key = %d %s", rec.Code, rec.Body)
	}
	return body.Token
}

func TestDeletingAServiceAccountStopsItsKeysAtOnce(t *testing.T) {
	r := newProvisionRig(t)
	if rec := r.request(http.MethodPost, "/api/v1/service-accounts", `{"name":"ci","role":"admin"}`, nil); rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body)
	}
	id := r.accounts()[0].ID
	key := r.newKey(id)
	if code := r.bearer(key); code != http.StatusOK {
		t.Fatalf("the new key = %d, want 200", code)
	}

	rec := r.request(http.MethodDelete, "/api/v1/service-accounts/"+id, "", http.Header{"Authorization": {"Bearer " + key}})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("an Admin key deleting its own account = %d %s, want 403", rec.Code, rec.Body)
	}

	if rec := r.request(http.MethodDelete, "/api/v1/service-accounts/"+id, "", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d %s, want 204", rec.Code, rec.Body)
	}
	if code := r.bearer(key); code != http.StatusUnauthorized {
		t.Fatalf("a key of the deleted account = %d, want 401", code)
	}
	if got := r.accounts(); len(got) != 0 {
		t.Fatalf("accounts after delete = %+v, want none", got)
	}
	if !strings.Contains(r.logs.String(), `"msg":"deleted a service account"`) {
		t.Errorf("no log line for the deletion: %s", r.logs)
	}
	if rec := r.request(http.MethodDelete, "/api/v1/service-accounts/"+id, "", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("delete twice = %d %s, want 404", rec.Code, rec.Body)
	}
}

func TestAProvisionedServiceAccountCannotBeDeleted(t *testing.T) {
	r := newProvisionRig(t)
	account, key := fileAccount(t, "editor")
	if err := r.service.EnsureProvisioned(t.Context(), []config.ServiceAccount{account}); err != nil {
		t.Fatal(err)
	}
	id := r.accounts()[0].ID
	rec := r.request(http.MethodDelete, "/api/v1/service-accounts/"+id, "", nil)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"code":"provisioned"`) {
		t.Fatalf("delete a provisioned account = %d %s, want 409 provisioned", rec.Code, rec.Body)
	}
	if code := r.bearer(key); code != http.StatusOK {
		t.Fatalf("its file key = %d, want it still working", code)
	}
}
