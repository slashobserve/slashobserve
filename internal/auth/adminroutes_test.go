package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func (r *sessionRig) serve(method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Cookie", r.cookie)
	req.Header.Set("Origin", "http://localhost:8080")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.engine.ServeHTTP(rec, req)
	return rec
}

func TestOnlyAdminsCanUseTheServiceAccountAndKeyRoutes(t *testing.T) {
	const account = "/api/v1/service-accounts/22222222-2222-4222-8222-222222222222"
	routes := []struct{ method, path, body string }{
		{http.MethodPost, "/api/v1/service-accounts", `{"name":"ci"}`},
		{http.MethodGet, "/api/v1/service-accounts", ""},
		{http.MethodPatch, account, `{"role":"viewer"}`},
		{http.MethodPost, account + "/keys", `{"name":"laptop"}`},
		{http.MethodGet, account + "/keys", ""},
		{http.MethodDelete, account + "/keys/33333333-3333-4333-8333-333333333333", ""},
	}
	for _, role := range []string{"editor", "commenter", "viewer"} {
		t.Run(role, func(t *testing.T) {
			r := newSessionRig(t)
			r.db.Exec(t, "UPDATE users SET role = $1 WHERE id = $2", role, r.userID)
			for _, tc := range routes {
				rec := r.serve(tc.method, tc.path, tc.body)
				if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"code":"forbidden_role"`) {
					t.Errorf("%s %s = %d %s, want 403 forbidden_role", tc.method, tc.path, rec.Code, rec.Body)
				}
			}
			r.db.Exec(t, "UPDATE users SET role = 'admin' WHERE id = $1", r.userID)
			rec := r.serve(http.MethodGet, "/api/v1/service-accounts", "")
			if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), `"name":"ci"`) {
				t.Fatalf("an Admin lists %d %s, want 200 and no account the refused POST made", rec.Code, rec.Body)
			}
		})
	}
}
