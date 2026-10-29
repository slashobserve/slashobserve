package prompts

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/auth"
)

const callerID = "3f1c2a52-8c5e-4a53-9a3e-1d2d4c0a9b11"

func serve(t *testing.T, kind auth.PrincipalKind, role auth.Role, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api/v1", func(c *gin.Context) {
		p := auth.Principal{Kind: kind, ID: callerID, Name: "Ada", Role: role}
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), p))
	})
	NewHandler(nil, slog.New(slog.DiscardHandler)).Register(auth.NewRoutes(api))
	req := httptest.NewRequest(method, "/api/v1/prompts"+path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

func code(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
	return body.Code
}

func TestRefusalsThatNeverReachTheDatabase(t *testing.T) {
	const id = "/0a1b2c3d-0000-4000-8000-000000000001"
	cases := []struct {
		name   string
		kind   auth.PrincipalKind
		method string
		path   string
		body   string
		status int
		code   string
	}{
		{"service account cannot save", auth.KindServiceAccount, http.MethodPost, "", `{"name":"a","body":"b"}`, http.StatusForbidden, "forbidden"},
		{"service account cannot change", auth.KindServiceAccount, http.MethodPatch, id, `{"shared":true}`, http.StatusForbidden, "forbidden"},
		{"service account cannot delete", auth.KindServiceAccount, http.MethodDelete, id, "", http.StatusForbidden, "forbidden"},
		{"invalid name on save", auth.KindUser, http.MethodPost, "", `{"name":"-bad","body":"b"}`, http.StatusUnprocessableEntity, "invalid_name"},
		{"missing name on save", auth.KindUser, http.MethodPost, "", `{"body":"b"}`, http.StatusUnprocessableEntity, "invalid_name"},
		{"missing body on save", auth.KindUser, http.MethodPost, "", `{"name":"a"}`, http.StatusUnprocessableEntity, "invalid_body"},
		{"body too long on save", auth.KindUser, http.MethodPost, "", `{"name":"a","body":"` + strings.Repeat("x", 4001) + `"}`, http.StatusUnprocessableEntity, "invalid_body"},
		{"unknown field on save", auth.KindUser, http.MethodPost, "", `{"name":"a","body":"b","owner":"x"}`, http.StatusBadRequest, "invalid_request"},
		{"invalid name on change", auth.KindUser, http.MethodPatch, id, `{"name":"semi;colon"}`, http.StatusUnprocessableEntity, "invalid_name"},
		{"empty change", auth.KindUser, http.MethodPatch, id, `{}`, http.StatusBadRequest, "invalid_request"},
		{"change with a bad id", auth.KindUser, http.MethodPatch, "/not-a-uuid", `{"shared":true}`, http.StatusBadRequest, "invalid_request"},
		{"delete with a bad id", auth.KindUser, http.MethodDelete, "/not-a-uuid", "", http.StatusBadRequest, "invalid_request"},
		{"name filter too long", auth.KindUser, http.MethodGet, "?name=" + strings.Repeat("a", 300), "", http.StatusBadRequest, "invalid_request"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := serve(t, tc.kind, auth.RoleAdmin, tc.method, tc.path, tc.body)
			if rec.Code != tc.status || code(t, rec) != tc.code {
				t.Fatalf("got %d %s, want %d %s", rec.Code, rec.Body, tc.status, tc.code)
			}
		})
	}
}

func TestNameFilterThatCannotMatchAnswersAnEmptyList(t *testing.T) {
	for _, query := range []string{"?name=", "?name=-bad", "?name=" + strings.Repeat("a", 65)} {
		rec := serve(t, auth.KindUser, auth.RoleAdmin, http.MethodGet, query, "")
		if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"prompts":[]}` {
			t.Fatalf("%s: got %d %s, want 200 with an empty list", query, rec.Code, rec.Body)
		}
	}
}

func TestMutationsNeedTheEditorRole(t *testing.T) {
	const id = "/0a1b2c3d-0000-4000-8000-000000000001"
	mutations := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"save", http.MethodPost, "", `{"name":"-bad","body":"b"}`},
		{"change", http.MethodPatch, id, `{"name":"-bad"}`},
		{"delete", http.MethodDelete, id, ""},
	}
	for _, m := range mutations {
		for _, role := range []auth.Role{auth.RoleViewer, auth.RoleCommenter} {
			t.Run(string(role)+" cannot "+m.name, func(t *testing.T) {
				rec := serve(t, auth.KindUser, role, m.method, m.path, m.body)
				if rec.Code != http.StatusForbidden || code(t, rec) != "forbidden_role" {
					t.Fatalf("got %d %s, want 403 forbidden_role", rec.Code, rec.Body)
				}
			})
		}
	}
	for _, role := range []auth.Role{auth.RoleEditor, auth.RoleAdmin} {
		t.Run(string(role)+" passes the role check on save", func(t *testing.T) {
			rec := serve(t, auth.KindUser, role, http.MethodPost, "", `{"name":"-bad","body":"b"}`)
			if rec.Code != http.StatusUnprocessableEntity || code(t, rec) != "invalid_name" {
				t.Fatalf("got %d %s, want 422 invalid_name", rec.Code, rec.Body)
			}
		})
	}
}
