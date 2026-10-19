package dashboards

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
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

func TestListFiltersRejectBadInput(t *testing.T) {
	bad := map[string]string{
		"q invalid UTF-8":             "q=%FF",
		"q truncated UTF-8":           "q=%C3",
		"q NUL":                       "q=a%00b",
		"q whitespace only":           "q=%20%20",
		"q raw length over the limit": "q=%20" + strings.Repeat("a", maxQueryRunes),
		"q multibyte over the limit":  "q=" + strings.Repeat("%C3%A9", maxQueryRunes+1),
		"datasource invalid UTF-8":    "datasource=%FF",
		"datasource NUL":              "datasource=%00",
		"datasource over the limit":   "datasource=" + strings.Repeat("a", maxDatasourceRunes+1),
		"q empty":                     "q=",
		"datasource empty":            "datasource=",
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
