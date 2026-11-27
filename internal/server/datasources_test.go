package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/logtest"
)

type unhealthyDatasource struct {
	datasource.Datasource
	err error
}

func (u unhealthyDatasource) Type() string                      { return "loki" }
func (u unhealthyDatasource) CheckHealth(context.Context) error { return u.err }
func (u unhealthyDatasource) Close() error                      { return nil }

func datasourceEngine(t *testing.T, role auth.Role, registry *datasource.Registry) *gin.Engine {
	t.Helper()
	logger, _ := logtest.New()
	engine := gin.New()
	api := engine.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), auth.Principal{Kind: auth.KindUser, ID: "u1", Role: role}))
	}, principalAccess)
	registerDatasourceRoutes(auth.NewRoutes(api), registry, fakeUsage{}, logger)
	return engine
}

func TestRecheckReturnsTheFreshStatusWithItsCause(t *testing.T) {
	registry, err := datasource.NewRegistry(nil, datasource.Instance{UID: "loki", Datasource: unhealthyDatasource{err: datasource.ErrorfStatus(datasource.ErrUnavailable, 503, "not ready")}})
	if err != nil {
		t.Fatal(err)
	}
	engine := datasourceEngine(t, auth.RoleViewer, registry)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/datasources/loki/recheck", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["healthy"] != false || got["detail"] != "The datasource answered HTTP 503" || got["source"] != "file" || got["dashboard_count"] != float64(0) {
		t.Errorf("body = %v", got)
	}
	if got["health_state"] != "failing" || got["failure_kind"] != "transient" {
		t.Errorf("health_state = %v, failure_kind = %v, want failing and transient", got["health_state"], got["failure_kind"])
	}
}

func TestRecheckOfAnUnknownDatasourceIsNotFound(t *testing.T) {
	registry, err := datasource.NewRegistry(nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, reg := range map[string]*datasource.Registry{"empty registry": registry, "no registry": nil} {
		t.Run(name, func(t *testing.T) {
			engine := datasourceEngine(t, auth.RoleViewer, reg)
			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/datasources/nope/recheck", nil))
			if rec.Code != http.StatusNotFound {
				t.Errorf("status %d: %s", rec.Code, rec.Body)
			}
		})
	}
}
