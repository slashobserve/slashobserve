package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/logtest"
)

type headerAuth struct{}

func (headerAuth) Mount(api *gin.RouterGroup) auth.Routes {
	return auth.NewRoutes(api.Group("", func(c *gin.Context) {
		if c.GetHeader("X-Session") == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "sign in", "code": "unauthenticated"})
			return
		}
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), auth.Principal{Kind: auth.KindUser, ID: "u1", Role: auth.RoleViewer}))
		c.Next()
	}))
}

type publicEcho struct{}

func (publicEcho) RegisterPublic(g *gin.RouterGroup) {
	g.GET("/:token", func(c *gin.Context) {
		_, signedIn := auth.FromContext(c.Request.Context())
		c.JSON(http.StatusOK, gin.H{"signed_in": signedIn})
	})
}

func (publicEcho) Register(api auth.Routes) {
	api.GET("/secret", auth.RoleViewer, func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{}) })
}

func TestPublicRoutesNeedNoSession(t *testing.T) {
	logger, logs := logtest.New()
	handler, err := New(Options{Logger: logger, Auth: headerAuth{}, API: []Registrar{publicEcho{}}, Public: []PublicRegistrar{publicEcho{}}})
	if err != nil {
		t.Fatal(err)
	}
	const token = "pub_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/public/"+token, nil))
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"signed_in":false}` {
		t.Fatalf("public route: %d %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	if got := rec.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("X-Frame-Options = %q, want DENY", got)
	}
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/secret", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("authenticated route without a session: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/p/"+token, nil))
	logs.AssertAbsent(t, token)
	if !strings.Contains(logs.String(), `"/api/v1/public/:token"`) || !strings.Contains(logs.String(), `"/p/:token"`) {
		t.Fatalf("request log does not name the redacted paths: %s", logs.String())
	}
}

func TestLoggedPathHidesPublicTokens(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/api/v1/public/pub_abc", "/api/v1/public/:token"},
		{"/api/v1/public/pub_abc/panels/p1/data", "/api/v1/public/:token/panels/p1/data"},
		{"/api/v1/public/pub_abc/variables/env/values", "/api/v1/public/:token/variables/env/values"},
		{"/api/v1/public/", "/api/v1/public/"},
		{"/p/pub_abc", "/p/:token"},
		{"/p/pub_abc/", "/p/:token/"},
		{"/prompts", "/prompts"},
		{"/api/v1/dashboards/d1", "/api/v1/dashboards/d1"},
		{"/api/v1/publicity", "/api/v1/publicity"},
	}
	for _, tc := range cases {
		if got := loggedPath(tc.in); got != tc.want {
			t.Errorf("loggedPath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
