package mcpauth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"slices"
	"testing"
)

const testPublicURL = "http://observe.test:8080"

func metadataServer(t *testing.T, public string) *Server {
	t.Helper()
	base, err := url.Parse(public)
	if err != nil {
		t.Fatal(err)
	}
	return New(Config{PublicURL: base}, nil, nil)
}

func getJSON(t *testing.T, s *Server, path string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	var body map[string]any
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s is not JSON: %v", path, err)
		}
	}
	return rec, body
}

func TestAuthorizationServerMetadata(t *testing.T) {
	for _, public := range []string{testPublicURL, testPublicURL + "/"} {
		s := metadataServer(t, public)
		rec, body := getJSON(t, s, "/.well-known/oauth-authorization-server")
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d", rec.Code)
		}
		want := map[string]any{
			"issuer":                                         testPublicURL,
			"authorization_endpoint":                         testPublicURL + "/oauth/authorize",
			"token_endpoint":                                 testPublicURL + "/oauth/token",
			"registration_endpoint":                          testPublicURL + "/oauth/register",
			"revocation_endpoint":                            testPublicURL + "/oauth/revoke",
			"response_types_supported":                       []any{"code"},
			"grant_types_supported":                          []any{"authorization_code", "refresh_token"},
			"code_challenge_methods_supported":               []any{"S256"},
			"token_endpoint_auth_methods_supported":          []any{"none"},
			"revocation_endpoint_auth_methods_supported":     []any{"none"},
			"authorization_response_iss_parameter_supported": true,
		}
		if !reflect.DeepEqual(body, want) {
			t.Errorf("metadata for %q = %v, want %v", public, body, want)
		}
		if rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("Access-Control-Allow-Origin") != "*" {
			t.Errorf("headers %v lack no-store or CORS", rec.Header())
		}
	}
}

func TestProtectedResourceMetadataOnBothPaths(t *testing.T) {
	s := metadataServer(t, testPublicURL)
	for _, path := range []string{"/.well-known/oauth-protected-resource/mcp", "/.well-known/oauth-protected-resource"} {
		rec, body := getJSON(t, s, path)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status %d", path, rec.Code)
		}
		if body["resource"] != testPublicURL+"/mcp" {
			t.Errorf("%s resource = %v", path, body["resource"])
		}
		if !reflect.DeepEqual(body["authorization_servers"], []any{testPublicURL}) {
			t.Errorf("%s authorization_servers = %v", path, body["authorization_servers"])
		}
		if _, scoped := body["scopes_supported"]; scoped {
			t.Errorf("%s advertises scopes", path)
		}
	}
}

func TestPathsAndResourceMetadataURL(t *testing.T) {
	s := metadataServer(t, testPublicURL)
	want := []string{
		"/.well-known/oauth-protected-resource/mcp",
		"/.well-known/oauth-protected-resource",
		"/.well-known/oauth-authorization-server",
		"/oauth/authorize",
		"/oauth/token",
		"/oauth/register",
		"/oauth/revoke",
	}
	if got := s.Paths(); !slices.Equal(got, want) {
		t.Errorf("Paths() = %v, want %v", got, want)
	}
	if got := s.ResourceMetadataURL(); got != testPublicURL+"/.well-known/oauth-protected-resource/mcp" {
		t.Errorf("ResourceMetadataURL() = %q", got)
	}
}
