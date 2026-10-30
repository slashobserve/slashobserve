package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/forbidden"
)

func routesEngine(role Role, withPrincipal bool) (Routes, *gin.Engine) {
	engine := gin.New()
	group := engine.Group("/api/v1", func(c *gin.Context) {
		if withPrincipal {
			c.Request = c.Request.WithContext(WithPrincipal(c.Request.Context(), Principal{Kind: KindUser, ID: "u1", Role: role}))
		}
		c.Next()
	})
	return NewRoutes(group), engine
}

func serve(engine *gin.Engine, method, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func TestRoutesAllowEveryRoleAtOrAboveTheFloor(t *testing.T) {
	order := []Role{RoleViewer, RoleCommenter, RoleEditor, RoleAdmin}
	ok := func(c *gin.Context) { c.Status(http.StatusOK) }
	for i, have := range order {
		for j, floor := range order {
			routes, engine := routesEngine(have, true)
			routes.GET("/thing", floor, ok)
			rec := serve(engine, http.MethodGet, "/api/v1/thing")
			want := http.StatusForbidden
			if i >= j {
				want = http.StatusOK
			}
			if rec.Code != want {
				t.Errorf("%s on a %s route: status %d, want %d", have, floor, rec.Code, want)
			}
			if want == http.StatusForbidden {
				var body map[string]string
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
					t.Fatalf("decode body: %v", err)
				}
				if body["code"] != forbidden.Code || body["error"] != forbidden.RoleMessage {
					t.Errorf("%s on a %s route: body %v", have, floor, body)
				}
			}
		}
	}
}

func TestRoutesRefuseARequestWithoutAPrincipal(t *testing.T) {
	routes, engine := routesEngine("", false)
	routes.GET("/thing", RoleViewer, func(c *gin.Context) { c.Status(http.StatusOK) })
	rec := serve(engine, http.MethodGet, "/api/v1/thing")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d, want 403", rec.Code)
	}
}

func TestRoutesRefuseAPrincipalWithoutAKnownRole(t *testing.T) {
	routes, engine := routesEngine("owner", true)
	routes.GET("/thing", RoleViewer, func(c *gin.Context) { c.Status(http.StatusOK) })
	if rec := serve(engine, http.MethodGet, "/api/v1/thing"); rec.Code != http.StatusForbidden {
		t.Fatalf("status %d, want 403", rec.Code)
	}
}

func TestRoutesGuardEveryMethodAndSubgroup(t *testing.T) {
	routes, engine := routesEngine(RoleViewer, true)
	ok := func(c *gin.Context) { c.Status(http.StatusOK) }
	sub := routes.Group("/sub")
	sub.GET("/a", RoleEditor, ok)
	sub.POST("/a", RoleEditor, ok)
	sub.PUT("/a", RoleEditor, ok)
	sub.PATCH("/a", RoleEditor, ok)
	sub.DELETE("/a", RoleEditor, ok)
	sub.GET("/open", RoleViewer, ok)
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		if rec := serve(engine, method, "/api/v1/sub/a"); rec.Code != http.StatusForbidden {
			t.Errorf("%s: status %d, want 403", method, rec.Code)
		}
	}
	if rec := serve(engine, http.MethodGet, "/api/v1/sub/open"); rec.Code != http.StatusOK {
		t.Errorf("open route: status %d, want 200", rec.Code)
	}
}

func TestRoutesRunTheRoleCheckBeforeTheRouteHandlers(t *testing.T) {
	routes, engine := routesEngine(RoleViewer, true)
	ran := false
	routes.POST("/thing", RoleAdmin, func(c *gin.Context) { ran = true })
	serve(engine, http.MethodPost, "/api/v1/thing")
	if ran {
		t.Fatal("handler ran for a role below the floor")
	}
}
