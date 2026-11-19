package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestUnknownQueryNamesComeFromTheDocument(t *testing.T) {
	params, err := documentedQueryParams()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		method string
		path   string
		query  map[string][]string
		want   []string
	}{
		{"documented names", "GET", "/api/v1/search", map[string][]string{"q": {"a"}, "kind": {"dashboard"}, "limit": {"5"}}, nil},
		{"a misspelt filter", "GET", "/api/v1/dashboards", map[string][]string{"health": {"duplicate_title"}, "problem": {"x"}, "limit": {"5"}}, []string{"health"}},
		{"names are sorted", "GET", "/api/v1/dashboards", map[string][]string{"zeta": {"1"}, "alpha": {"1"}}, []string{"alpha", "zeta"}},
		{"a selection is never unknown", "GET", "/api/v1/dashboards/{id}/panels/{panelId}/data", map[string][]string{"var-pod": {"a"}}, nil},
		{"an undocumented route is not judged", "GET", "/api/v1/nope", map[string][]string{"x": {"1"}}, nil},
	}
	for _, tc := range cases {
		got := params.unknown(tc.method, tc.path, tc.query)
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestAnUnknownQueryParameterIsReportedInAHeaderAndStillServed(t *testing.T) {
	params, err := documentedQueryParams()
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	group := engine.Group(apiPrefix)
	group.Use(warnUnknownQuery(params))
	group.GET("/dashboards", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{}) })
	cases := map[string]string{
		"/api/v1/dashboards?limit=5":                  "",
		"/api/v1/dashboards?health=x&limit=5":         "health",
		"/api/v1/dashboards?b=1&a=1":                  "a, b",
		"/api/v1/dashboards?x%0d%0aSet-Cookie=1&ok=1": "ok, ...",
	}
	for target, want := range cases {
		rec := serve(engine, httptest.NewRequest(http.MethodGet, target, nil))
		if rec.Code != http.StatusOK || rec.Header().Get(unknownQueryHeader) != want {
			t.Errorf("%s: %d %q, want %q", target, rec.Code, rec.Header().Get(unknownQueryHeader), want)
		}
	}
}

func TestTheContractCheckRefusesToAcceptAnUndocumentedQueryParameter(t *testing.T) {
	var logs bytes.Buffer
	engine := contractEngine(t, &logs, func(g *gin.RouterGroup) {
		g.POST("/auth/logout", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	})
	rec := serve(engine, httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout?surprise=1", nil))
	if rec.Code != http.StatusInternalServerError || !strings.Contains(logs.String(), "a query parameter the document does not list") {
		t.Fatalf("status %d, logs %s", rec.Code, logs.String())
	}
	logs.Reset()
	if rec := serve(engine, httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)); rec.Code != http.StatusNoContent {
		t.Fatalf("status %d, logs %s", rec.Code, logs.String())
	}
}
