package server

import (
	"net/http"
	"strings"
	"testing"
)

func TestWebRoutesAnswerJSONNotFoundForServerPaths(t *testing.T) {
	for _, path := range []string{"/api/nope", "/api", "/.well-known/openid-configuration", "/.well-known", "/oauth/nope", "/oauth"} {
		t.Run(path, func(t *testing.T) {
			rec := serveWeb(t, Redirects{}, http.MethodGet, path)
			if rec.Code != http.StatusNotFound || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
				t.Fatalf("GET %s = %d %q, want a JSON 404", path, rec.Code, rec.Header().Get("Content-Type"))
			}
			if !strings.Contains(rec.Body.String(), `"code":"not_found"`) {
				t.Fatalf("body %q lacks the not_found code", rec.Body.String())
			}
		})
	}
}

func TestWebRoutesStillServeTheAppForConnectAndOtherPaths(t *testing.T) {
	for _, path := range []string{"/connect/x", "/oauthish", "/settings"} {
		t.Run(path, func(t *testing.T) {
			rec := serveWeb(t, Redirects{}, http.MethodGet, path)
			if strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
				t.Fatalf("GET %s answered JSON: %s", path, rec.Body.String())
			}
			if rec.Code != http.StatusOK && rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("GET %s = %d, want the app or the not-built text", path, rec.Code)
			}
		})
	}
}
