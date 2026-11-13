package dashboards

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/auth"
)

func listFilterStatus(t *testing.T, rawQuery string) (int, ListParams) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/dashboards?"+rawQuery, nil)
	params, ok := (&Service{}).listFilters(c)
	if ok {
		return http.StatusOK, params
	}
	return rec.Code, params
}

func listPageStatus(t *testing.T, rawQuery string) (status int, limit, offset int, cursor string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/dashboards?"+rawQuery, nil)
	limit, offset, cursor, ok := listPageParams(c)
	if ok {
		return http.StatusOK, limit, offset, cursor
	}
	return rec.Code, 0, 0, ""
}

func TestListPageParams(t *testing.T) {
	tests := []struct {
		name   string
		query  string
		status int
		limit  int
		offset int
		cursor string
	}{
		{"defaults", "", http.StatusOK, 50, 0, ""},
		{"limit and offset", "limit=10&offset=20", http.StatusOK, 10, 20, ""},
		{"offset at the ceiling", "offset=100000", http.StatusOK, 50, 100000, ""},
		{"offset over the ceiling", "offset=100001", http.StatusBadRequest, 0, 0, ""},
		{"cursor with offset", "cursor=abc&offset=0", http.StatusBadRequest, 0, 0, ""},
		{"offset with an empty cursor", "offset=0&cursor=", http.StatusBadRequest, 0, 0, ""},
		{"cursor alone", "cursor=abc", http.StatusOK, 50, 0, "abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, limit, offset, cursor := listPageStatus(t, tt.query)
			if status != tt.status || limit != tt.limit || offset != tt.offset || cursor != tt.cursor {
				t.Errorf("got status %d limit %d offset %d cursor %q, want %d, %d, %d, %q", status, limit, offset, cursor, tt.status, tt.limit, tt.offset, tt.cursor)
			}
		})
	}
}

func TestListFiltersRejectBadInput(t *testing.T) {
	bad := map[string]string{
		"q invalid UTF-8":              "q=%FF",
		"q truncated UTF-8":            "q=%C3",
		"q NUL":                        "q=a%00b",
		"q whitespace only":            "q=%20%20",
		"q raw length over the limit":  "q=%20" + strings.Repeat("a", maxQueryRunes),
		"q multibyte over the limit":   "q=" + strings.Repeat("%C3%A9", maxQueryRunes+1),
		"datasource invalid UTF-8":     "datasource=%FF",
		"datasource NUL":               "datasource=%00",
		"datasource over the limit":    "datasource=" + strings.Repeat("a", maxDatasourceRunes+1),
		"tag with an uppercase letter": "tag=Demo",
		"tag with a leading dash":      "tag=-demo",
		"tag empty":                    "tag=",
		"tag over the length limit":    "tag=" + strings.Repeat("a", 41),
		"eleven tags":                  strings.Repeat("tag=a&", 10) + "tag=a",
		"one bad tag among good ones":  "tag=demo&tag=Bad",
		"problem unknown":              "problem=bogus",
		"problem empty":                "problem=",
		"problem in another case":      "problem=ANY",
		"q empty":                      "q=",
		"datasource empty":             "datasource=",
	}
	for name, raw := range bad {
		t.Run(name, func(t *testing.T) {
			if code, _ := listFilterStatus(t, raw); code != http.StatusBadRequest {
				t.Errorf("%s answered %d, want 400", raw, code)
			}
		})
	}
}

func TestListFiltersAcceptAndTrimQuery(t *testing.T) {
	tests := []struct {
		raw       string
		wantQuery string
		wantDS    string
	}{
		{"q=" + url.QueryEscape("  shop "), "shop", ""},
		{"q=" + strings.Repeat("a", maxQueryRunes), strings.Repeat("a", maxQueryRunes), ""},
		{"q=" + strings.Repeat("%C3%A9", maxQueryRunes), strings.Repeat("é", maxQueryRunes), ""},
		{"datasource=" + strings.Repeat("a", maxDatasourceRunes), "", strings.Repeat("a", maxDatasourceRunes)},
		{"", "", ""},
	}
	for _, tt := range tests {
		code, params := listFilterStatus(t, tt.raw)
		if code != http.StatusOK || params.Query != tt.wantQuery || params.Datasource != tt.wantDS {
			t.Errorf("%.40s answered %d with query %.20q and datasource %.20q", tt.raw, code, params.Query, params.Datasource)
		}
	}
}

func TestListFiltersAcceptRepeatedTags(t *testing.T) {
	tests := []struct {
		raw  string
		want []string
	}{
		{"", nil},
		{"tag=demo", []string{"demo"}},
		{"tag=demo&tag=shop.v2", []string{"demo", "shop.v2"}},
		{"tag=" + strings.Repeat("a", 40), []string{strings.Repeat("a", 40)}},
		{strings.Repeat("tag=a&", 9) + "tag=a", []string{"a", "a", "a", "a", "a", "a", "a", "a", "a", "a"}},
	}
	for _, tt := range tests {
		code, params := listFilterStatus(t, tt.raw)
		if code != http.StatusOK || !slices.Equal(params.Tags, tt.want) {
			t.Errorf("%.40s answered %d with tags %v, want %v", tt.raw, code, params.Tags, tt.want)
		}
	}
}

func TestListFiltersAcceptEveryProblem(t *testing.T) {
	for _, problem := range []string{"any", "no_data", "query_failed", "variable_failed", "duplicate_title", "unopened"} {
		if code, params := listFilterStatus(t, "problem="+problem); code != http.StatusOK || params.Problem != problem {
			t.Errorf("problem=%s answered %d with problem %q", problem, code, params.Problem)
		}
	}
	if code, params := listFilterStatus(t, ""); code != http.StatusOK || params.Problem != "" {
		t.Errorf("no problem answered %d with problem %q", code, params.Problem)
	}
}

func TestTagsJSONIsAnEmptyArrayWhenThereAreNoTags(t *testing.T) {
	raw, err := json.Marshal(newTagsJSON(nil))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "[]" {
		t.Fatalf("got %s, want []", raw)
	}
}

func TestTagsJSONCarriesNameAndCount(t *testing.T) {
	raw, err := json.Marshal(newTagsJSON([]TagCount{{Tag: "checkout", Dashboards: 3}}))
	if err != nil {
		t.Fatal(err)
	}
	if want := `[{"tag":"checkout","dashboards":3}]`; string(raw) != want {
		t.Fatalf("got %s, want %s", raw, want)
	}
}

func TestTagsRouteCoexistsWithTheIDRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	(&Service{}).Register(auth.NewRoutes(router.Group("/api/v1")))
	routes := map[string]bool{}
	for _, r := range router.Routes() {
		routes[r.Method+" "+r.Path] = true
	}
	for _, want := range []string{"GET /api/v1/dashboards/tags", "GET /api/v1/dashboards/:id"} {
		if !routes[want] {
			t.Fatalf("route %q is not registered", want)
		}
	}
}
