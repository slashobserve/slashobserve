package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

const redirectID = "0b6f9c1e-7a52-4c1b-9d3e-2f8a6b4c5d10"

func serveWeb(t *testing.T, redirects Redirects, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	engine := gin.New()
	registerWebRoutes(engine, redirects)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}

func TestRedirectOldAddressesToShortForms(t *testing.T) {
	cases := []struct {
		name, from, to string
	}{
		{"dashboard", "/dashboards/" + redirectID, "/d/" + redirectID},
		{"dashboard history", "/dashboards/" + redirectID + "/versions", "/d/" + redirectID + "/versions"},
		{"dashboard query kept", "/dashboards/" + redirectID + "?from=now-6h&var-pod=a&var-pod=b", "/d/" + redirectID + "?from=now-6h&var-pod=a&var-pod=b"},
		{"capital hex id", "/dashboards/0B6F9C1E-7A52-4C1B-9D3E-2F8A6B4C5D10", "/d/0B6F9C1E-7A52-4C1B-9D3E-2F8A6B4C5D10"},
		{"folder", "/folders/" + redirectID, "/f/" + redirectID},
		{"chat", "/observe/" + redirectID, "/c/" + redirectID},
		{"chat query kept", "/observe/" + redirectID + "?panel=history", "/c/" + redirectID + "?panel=history"},
		{"build", "/builds/" + redirectID, "/b/" + redirectID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, method := range []string{http.MethodGet, http.MethodHead} {
				rec := serveWeb(t, Redirects{}, method, tc.from)
				if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != tc.to {
					t.Fatalf("%s %s = %d to %q, want 301 to %q", method, tc.from, rec.Code, rec.Header().Get("Location"), tc.to)
				}
			}
		})
	}
}

func TestRedirectLeavesOtherPathsToTheWebApp(t *testing.T) {
	cases := []struct{ name, method, path string }{
		{"dashboards list", http.MethodGet, "/dashboards"},
		{"by title", http.MethodGet, "/dashboards/by-title?title=Shop"},
		{"not an id", http.MethodGet, "/dashboards/does-not-exist"},
		{"dotted segment", http.MethodGet, "/dashboards/v1.2"},
		{"uuid without dashes", http.MethodGet, "/dashboards/0b6f9c1e7a524c1b9d3e2f8a6b4c5d10"},
		{"folder subpath", http.MethodGet, "/folders/" + redirectID + "/permissions"},
		{"chat subpath", http.MethodGet, "/observe/" + redirectID + "/extra"},
		{"new form", http.MethodGet, "/d/" + redirectID + "/shop"},
		{"non-id new form without a resolver", http.MethodGet, "/d/grafana-uid"},
		{"post", http.MethodPost, "/dashboards/" + redirectID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := serveWeb(t, Redirects{}, tc.method, tc.path)
			if rec.Code == http.StatusMovedPermanently || rec.Code == http.StatusFound || rec.Header().Get("Location") != "" {
				t.Fatalf("%s %s redirected: %d to %q", tc.method, tc.path, rec.Code, rec.Header().Get("Location"))
			}
		})
	}
}

func TestRedirectGrafanaUIDToTheImportedDashboard(t *testing.T) {
	var asked []string
	redirects := Redirects{GrafanaDashboard: func(r *http.Request, uid string) (string, bool) {
		asked = append(asked, uid)
		if uid == "known uid" && r.Header.Get("Cookie") == "session=ok" {
			return "/d/" + redirectID, true
		}
		return "", false
	}}
	cases := []struct{ name, from, to string }{
		{"slug and query kept", "/d/known%20uid/checkout-health?orgId=1&var-pod=a", "/d/" + redirectID + "/checkout-health?orgId=1&var-pod=a"},
		{"no slug", "/d/known%20uid", "/d/" + redirectID},
		{"trailing slash", "/d/known%20uid/", "/d/" + redirectID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine := gin.New()
			registerWebRoutes(engine, redirects)
			req := httptest.NewRequest(http.MethodGet, tc.from, nil)
			req.Header.Set("Cookie", "session=ok")
			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, req)
			if rec.Code != http.StatusFound || rec.Header().Get("Location") != tc.to {
				t.Fatalf("%s = %d to %q, want 302 to %q", tc.from, rec.Code, rec.Header().Get("Location"), tc.to)
			}
			if got := rec.Header().Get("Cache-Control"); got != "no-store" {
				t.Fatalf("Cache-Control %q, want no-store because the answer depends on the caller", got)
			}
		})
	}
	asked = nil
	for _, path := range []string{"/d/known%20uid", "/d/unknown", "/d/" + redirectID} {
		rec := serveWeb(t, redirects, http.MethodGet, path)
		if rec.Header().Get("Location") != "" || rec.Code == http.StatusFound {
			t.Fatalf("%s without the session redirected: %d to %q", path, rec.Code, rec.Header().Get("Location"))
		}
	}
	if len(asked) != 2 || asked[0] != "known uid" || asked[1] != "unknown" {
		t.Fatalf("resolver asked %v, want the two non-id dashboard paths only", asked)
	}
}
