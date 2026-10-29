package allowlist

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/egress/domains"
)

func parseList(t *testing.T, entries ...string) domains.List {
	t.Helper()
	list := domains.List{}
	for _, e := range entries {
		p, err := domains.ParsePattern(e)
		if err != nil {
			t.Fatal(err)
		}
		list = append(list, p)
	}
	return list
}

func serve(t *testing.T, lock Lock, kind auth.PrincipalKind, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api/v1", func(c *gin.Context) {
		p := auth.Principal{Kind: kind, ID: "3f1c2a52-8c5e-4a53-9a3e-1d2d4c0a9b11", Name: "Ada"}
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), p))
	})
	New(nil, lock, slog.New(slog.DiscardHandler)).Register(api)
	req := httptest.NewRequest(method, "/api/v1/outbound-domains"+path, strings.NewReader(body))
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

func TestLockedListAnswersFromTheConfiguredEntriesSorted(t *testing.T) {
	lock := Lock{Locked: true, Domains: parseList(t, "b.example.org", "*.a.example.com", "a.example.com")}
	rec := serve(t, lock, auth.KindUser, http.MethodGet, "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var body struct {
		ManagedBy string `json:"managed_by"`
		Domains   []struct {
			Domain  string  `json:"domain"`
			AddedAt *string `json:"added_at"`
			AddedBy *string `json:"added_by"`
		} `json:"domains"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.ManagedBy != "config" {
		t.Fatalf("managed_by = %q, want config", body.ManagedBy)
	}
	want := []string{"*.a.example.com", "a.example.com", "b.example.org"}
	if len(body.Domains) != len(want) {
		t.Fatalf("domains = %+v, want %v", body.Domains, want)
	}
	for i, d := range body.Domains {
		if d.Domain != want[i] || d.AddedAt != nil || d.AddedBy != nil {
			t.Fatalf("domains[%d] = %+v, want %q with null added_at and added_by", i, d, want[i])
		}
	}
}

func TestLockedEmptyListIsAnEmptyArray(t *testing.T) {
	rec := serve(t, Lock{Locked: true}, auth.KindUser, http.MethodGet, "", "")
	if !strings.Contains(rec.Body.String(), `"domains":[]`) {
		t.Fatalf("body = %s, want an empty domains array", rec.Body)
	}
}

func TestLockedRefusesChangesBeforeTouchingTheDatabase(t *testing.T) {
	lock := Lock{Locked: true, Domains: parseList(t, "example.com")}
	for _, tt := range []struct{ method, path, body string }{
		{http.MethodPost, "", `{"domain":"example.org"}`},
		{http.MethodDelete, "/example.com", ""},
	} {
		rec := serve(t, lock, auth.KindUser, tt.method, tt.path, tt.body)
		if rec.Code != http.StatusConflict || code(t, rec) != "managed_by_config" {
			t.Fatalf("%s: status = %d, body %s, want 409 managed_by_config", tt.method, rec.Code, rec.Body)
		}
	}
}

func TestAddRefusesInvalidDomainsAndLargeBodies(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantCode int
		wantBody string
	}{
		{"a url", `{"domain":"http://x"}`, http.StatusUnprocessableEntity, "invalid_domain"},
		{"an address", `{"domain":"10.0.0.1"}`, http.StatusUnprocessableEntity, "invalid_domain"},
		{"a missing domain", `{}`, http.StatusUnprocessableEntity, "invalid_domain"},
		{"a body over 4 KiB", `{"domain":"` + strings.Repeat("a", 5000) + `.com"}`, http.StatusRequestEntityTooLarge, "too_large"},
		{"an unknown field", `{"domain":"example.com","x":1}`, http.StatusBadRequest, "invalid_request"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := serve(t, Lock{}, auth.KindUser, http.MethodPost, "", tt.body)
			if rec.Code != tt.wantCode || code(t, rec) != tt.wantBody {
				t.Fatalf("status = %d, body %s, want %d %s", rec.Code, rec.Body, tt.wantCode, tt.wantBody)
			}
		})
	}
}

func TestRemoveRefusesAnUnparsablePathValue(t *testing.T) {
	rec := serve(t, Lock{}, auth.KindUser, http.MethodDelete, "/bad%20host", "")
	if rec.Code != http.StatusUnprocessableEntity || code(t, rec) != "invalid_domain" {
		t.Fatalf("status = %d, body %s, want 422 invalid_domain", rec.Code, rec.Body)
	}
}

func TestServiceAccountKeysAreForbidden(t *testing.T) {
	for _, lock := range []Lock{{}, {Locked: true}} {
		for _, tt := range []struct{ method, path, body string }{
			{http.MethodGet, "", ""},
			{http.MethodPost, "", `{"domain":"example.com"}`},
			{http.MethodDelete, "/example.com", ""},
		} {
			rec := serve(t, lock, auth.KindServiceAccount, tt.method, tt.path, tt.body)
			if rec.Code != http.StatusForbidden || code(t, rec) != "forbidden" {
				t.Fatalf("%s locked=%v: status = %d, body %s, want 403 forbidden", tt.method, lock.Locked, rec.Code, rec.Body)
			}
		}
	}
}

func TestAllowedInLockedModeIsTheConfiguredList(t *testing.T) {
	list := parseList(t, "example.com")
	got, err := New(nil, Lock{Locked: true, Domains: list}, slog.New(slog.DiscardHandler)).Allowed(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !got.Allows("example.com") || got.Allows("example.org") {
		t.Fatalf("Allowed = %v, want only example.com", got)
	}
}
