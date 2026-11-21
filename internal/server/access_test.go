package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/logtest"
)

type accessProbe struct{}

type probed struct {
	Set    bool   `json:"set"`
	By     string `json:"by"`
	Role   string `json:"role"`
	System bool   `json:"system"`
}

func probe(c *gin.Context) {
	a, ok := datasource.AccessFrom(c.Request.Context())
	c.JSON(http.StatusOK, probed{Set: ok, By: a.By.ID, Role: string(a.By.Role), System: a.System})
}

func (accessProbe) Register(api auth.Routes) { api.GET("/probe", auth.RoleViewer, probe) }

func (accessProbe) RegisterPublic(g *gin.RouterGroup) { g.GET("/:token", probe) }

func probeAs(t *testing.T, handler http.Handler, path string, session bool) probed {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
	if session {
		req.Header.Set("X-Session", "s")
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s answered %d: %s", path, rec.Code, rec.Body)
	}
	var got probed
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestEveryAPIRouteReadsDatasourcesAsItsCallerAndAPublicRouteAsNobody(t *testing.T) {
	logger, _ := logtest.New()
	handler, err := New(Options{Logger: logger, Auth: headerAuth{}, API: []Registrar{accessProbe{}}, Public: []PublicRegistrar{accessProbe{}}})
	if err != nil {
		t.Fatal(err)
	}
	if got := probeAs(t, handler, "/api/v1/probe", true); !got.Set || got.By != "u1" || got.Role != string(auth.RoleViewer) || got.System {
		t.Fatalf("signed-in route access = %+v", got)
	}
	if got := probeAs(t, handler, "/api/v1/public/pub_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", false); got.Set {
		t.Fatalf("a public route inherited datasource access %+v; its handler must set its own", got)
	}
}
