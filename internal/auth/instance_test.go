package auth

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func instanceBody(t *testing.T, cfg Config) (map[string]any, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	s, err := New(cfg, nil, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	engine.GET("/instance", s.instance)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/instance", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body, rec.Body.String()
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestInstanceServesOnlyPublicSettings(t *testing.T) {
	issuer := url.URL{Scheme: "https", User: url.UserPassword("svc-user", "svc-pass"), Host: "login.example.com", Path: "/dex"}
	body, raw := instanceBody(t, Config{
		PublicURL:           mustURL(t, "https://observe.example.com"),
		IssuerURL:           issuer.String(),
		InternalURL:         mustURL(t, "http://dex-internal.cluster.local:5556"),
		ClientID:            "client-id-value",
		ClientSecret:        "client-secret-value",
		SessionTTL:          36 * time.Hour,
		AllowedEmailDomains: []string{"example.com", "example.org"},
		ProviderName:        "Acme SSO",
	})

	want := map[string]any{
		"public_url":            "https://observe.example.com",
		"mcp_url":               "https://observe.example.com/mcp",
		"oidc_issuer_url":       "https://login.example.com/dex",
		"oidc_provider_name":    "Acme SSO",
		"allowed_email_domains": []any{"example.com", "example.org"},
		"any_email_domain":      false,
		"session_ttl_seconds":   float64(36 * 3600),
	}
	if len(body) != len(want) {
		t.Fatalf("fields = %v, want exactly %d", body, len(want))
	}
	for key, value := range want {
		got, ok := body[key]
		if !ok || !equalJSON(got, value) {
			t.Errorf("%s = %v, want %v", key, got, value)
		}
	}
	for _, leaked := range []string{"client-secret-value", "client-id-value", "dex-internal", "cluster.local", "svc-user", "svc-pass"} {
		if strings.Contains(raw, leaked) {
			t.Errorf("response leaks %q: %s", leaked, raw)
		}
	}
}

func equalJSON(a, b any) bool {
	if as, ok := a.([]any); ok {
		bs, ok := b.([]any)
		return ok && slices.Equal(as, bs)
	}
	return a == b
}

func TestInstanceWithAnyDomainListsNoDomains(t *testing.T) {
	body, _ := instanceBody(t, Config{
		PublicURL:           mustURL(t, "http://localhost:8080"),
		IssuerURL:           "http://localhost:5556/dex",
		SessionTTL:          time.Hour,
		AllowedEmailDomains: []string{"*"},
		AllowAnyEmailDomain: true,
	})
	domains, ok := body["allowed_email_domains"].([]any)
	if !ok || len(domains) != 0 || body["any_email_domain"] != true || body["oidc_provider_name"] != "" {
		t.Fatalf("body = %v, want an empty domain list, any_email_domain true and an empty provider name", body)
	}
}
