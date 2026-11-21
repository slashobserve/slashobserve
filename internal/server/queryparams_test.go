package server

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
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

func TestAnUnknownQueryParameterOfANonListRouteIsReportedInAHeaderAndStillServed(t *testing.T) {
	params, err := documentedQueryParams()
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	group := engine.Group(apiPrefix)
	group.Use(warnUnknownQuery(params))
	group.GET("/dashboards/:id", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{}) })
	const id = "/api/v1/dashboards/3f2b8a52-6f2e-4a54-9a3e-0d4d3a6a9b11"
	cases := map[string]string{
		id + "?version=1":                "version",
		id:                               "",
		id + "?b=1&a=1":                  "a, b",
		id + "?x%0d%0aSet-Cookie=1&ok=1": "ok, ...",
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

func listEngine(t *testing.T) *gin.Engine {
	t.Helper()
	params, err := documentedQueryParams()
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	group := engine.Group(apiPrefix)
	group.Use(warnUnknownQuery(params))
	ok := func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{}) }
	group.GET("/dashboards", ok)
	group.GET("/observations", ok)
	group.GET("/folders", ok)
	group.GET("/observe", ok)
	return engine
}

func TestAListRouteRefusesAnUnknownQueryParameterByName(t *testing.T) {
	engine := listEngine(t)
	for target, want := range map[string]string{
		"/api/v1/dashboards?health=duplicate_title&limit=200": `unknown query parameter health; this route takes `,
		"/api/v1/observations?bogus=1":                        `unknown query parameter bogus; this route takes `,
		"/api/v1/folders?bogus=1":                             `unknown query parameter bogus`,
		"/api/v1/observe?source=observe":                      `unknown query parameter source; this route takes chat_id, `,
		"/api/v1/dashboards?folder_id=notauuid&sort=x":        `unknown query parameter folder_id, sort`,
	} {
		rec := serve(engine, httptest.NewRequest(http.MethodGet, target, nil))
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), want) || !strings.Contains(rec.Body.String(), `"code":"invalid_request"`) {
			t.Errorf("%s: %d %s, want 400 with %q", target, rec.Code, rec.Body.String(), want)
		}
	}
}

func TestAListRouteRefusesABadQueryValueByName(t *testing.T) {
	engine := listEngine(t)
	for target, want := range map[string]string{
		"/api/v1/observations?folder_id=notauuid": `query parameter \"folder_id\" must be a UUID`,
		"/api/v1/dashboards?order=oldest":         `query parameter \"order\" must be one of updated, folder, views`,
		"/api/v1/dashboards?order=;drop":          `the query string is malformed`,
		"/api/v1/dashboards?order=":               `query parameter \"order\" must be one of updated, folder, views`,
		"/api/v1/dashboards?limit=0":              `query parameter \"limit\" must be an integer from 1 to 200`,
		"/api/v1/dashboards?limit=abc":            `query parameter \"limit\" must be an integer from 1 to 200`,
		"/api/v1/dashboards?tag=Not%20A%20Tag":    `query parameter \"tag\" must be a list`,
	} {
		rec := serve(engine, httptest.NewRequest(http.MethodGet, target, nil))
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), want) {
			t.Errorf("%s: %d %s, want 400 with %q", target, rec.Code, rec.Body.String(), want)
		}
	}
}

func TestAListRouteServesDocumentedParametersAndSelections(t *testing.T) {
	engine := listEngine(t)
	for _, target := range []string{
		"/api/v1/dashboards",
		"/api/v1/dashboards?limit=200&order=views&tag=checkout&tag=prod&q=latency",
		"/api/v1/observations?folder_id=3f2b8a52-6f2e-4a54-9a3e-0d4d3a6a9b11&state=open",
	} {
		if rec := serve(engine, httptest.NewRequest(http.MethodGet, target, nil)); rec.Code != http.StatusOK {
			t.Errorf("%s: %d %s", target, rec.Code, rec.Body.String())
		}
	}
}

func sampleQueryValue(t *testing.T, ref *openapi3.SchemaRef) string {
	t.Helper()
	s := ref.Value
	switch {
	case s.Default != nil:
		return fmt.Sprint(s.Default)
	case len(s.Enum) > 0:
		return fmt.Sprint(s.Enum[0])
	case s.Type.Is("integer"), s.Type.Is("number"):
		if s.Min != nil {
			return strconv.FormatFloat(*s.Min, 'f', -1, 64)
		}
		return "1"
	case s.Type.Is("boolean"):
		return "true"
	case s.Type.Is("array"):
		return sampleQueryValue(t, s.Items)
	case s.Format == "uuid":
		return "3f2b8a52-6f2e-4a54-9a3e-0d4d3a6a9b11"
	case s.Format == "date-time":
		return "2026-01-02T03:04:05Z"
	}
	value := "a"
	for uint64(len(value)) < s.MinLength {
		value += "a"
	}
	return value
}

func TestEveryDocumentedParameterOfEveryListRouteIsAccepted(t *testing.T) {
	params, err := documentedQueryParams()
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for key, route := range params {
		if !route.list {
			continue
		}
		method, template, _ := strings.Cut(key, " ")
		for _, p := range route.parameters {
			query := url.Values{p.Name: {sampleQueryValue(t, p.Schema)}}
			for _, required := range route.parameters {
				if required.Required {
					query.Set(required.Name, sampleQueryValue(t, required.Schema))
				}
			}
			req := httptest.NewRequest(method, template+"?"+query.Encode(), nil)
			if unknown := params.unknown(method, template, query); len(unknown) > 0 {
				t.Errorf("%s: documented parameter %s is unknown", key, p.Name)
			}
			if message := route.refusal(req, nil); message != "" {
				t.Errorf("%s ?%s: %s", key, query.Encode(), message)
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatal("no list route parameters were walked")
	}
}
