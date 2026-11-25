package proxyhttp

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

type servicesSource struct {
	*fakeDatasource
	calls  atomic.Int32
	values []string
	hang   bool
	failed error
}

func (s *servicesSource) Type() string { return string(spec.Loki) }

func (s *servicesSource) Services(ctx context.Context, req datasource.ServiceRequest) (datasource.ServiceList, error) {
	s.calls.Add(1)
	if s.hang {
		<-ctx.Done()
		return datasource.ServiceList{}, ctx.Err()
	}
	if s.failed != nil {
		return datasource.ServiceList{}, s.failed
	}
	return datasource.ServiceList{Signal: "logs", Field: "service_name", Values: s.values}, nil
}

func servicesRouter(t *testing.T, source *servicesSource, role auth.Role, opts ...queryproxy.RunnerOption) *gin.Engine {
	t.Helper()
	logger := slog.New(slog.DiscardHandler)
	registry, err := datasource.NewRegistry(logger,
		datasource.Instance{UID: "loki", Datasource: source},
		datasource.Instance{UID: "prom", Datasource: &fakeDatasource{}},
	)
	if err != nil {
		t.Fatal(err)
	}
	runner := queryproxy.NewRunner(registry, logger, opts...)
	h := NewHandler(runner, &fakeSpecs{}, nil, newFakeAccess(), registry, logger)
	router := gin.New()
	api := router.Group("/api/v1")
	if role != "" {
		api.Use(func(c *gin.Context) {
			signIn(c, auth.Principal{Kind: auth.KindUser, ID: "alice", Role: role})
		})
	}
	h.Register(auth.NewRoutes(api))
	return router
}

func getServices(router *gin.Engine, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestServicesRouteAnswersTheSourceFieldWindowAndServices(t *testing.T) {
	source := &servicesSource{fakeDatasource: &fakeDatasource{}, values: []string{"checkout", "shop"}}
	rec := getServices(servicesRouter(t, source, auth.RoleViewer), "/api/v1/datasources/loki/services?signal=logs&window=15m")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var body servicesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	want := servicesResponse{Datasource: "loki", Signal: "logs", Field: "service_name", WindowSeconds: 900, Services: []string{"checkout", "shop"}}
	if body.Datasource != want.Datasource || body.Signal != want.Signal || body.Field != want.Field || body.WindowSeconds != want.WindowSeconds || len(body.Services) != 2 || body.Truncated {
		t.Fatalf("body = %+v, want %+v", body, want)
	}
}

func TestServicesRouteAnswersAViewerAndRefusesACallWithNoPrincipal(t *testing.T) {
	source := &servicesSource{fakeDatasource: &fakeDatasource{}, values: []string{"shop"}}
	if rec := getServices(servicesRouter(t, source, auth.RoleViewer), "/api/v1/datasources/loki/services"); rec.Code != http.StatusOK {
		t.Fatalf("viewer status = %d", rec.Code)
	}
	if rec := getServices(servicesRouter(t, source, ""), "/api/v1/datasources/loki/services"); rec.Code != http.StatusForbidden {
		t.Fatalf("anonymous status = %d, want 403", rec.Code)
	}
}

func TestServicesRouteRefusesWhatItCannotRead(t *testing.T) {
	source := &servicesSource{fakeDatasource: &fakeDatasource{}}
	router := servicesRouter(t, source, auth.RoleViewer)
	tests := []struct {
		path   string
		status int
		code   string
	}{
		{"/api/v1/datasources/nope/services", http.StatusNotFound, "not_found"},
		{"/api/v1/datasources/prom/services", http.StatusBadRequest, "invalid_request"},
		{"/api/v1/datasources/loki/services?signal=events", http.StatusBadRequest, "invalid_request"},
		{"/api/v1/datasources/loki/services?window=soon", http.StatusBadRequest, "invalid_request"},
		{"/api/v1/datasources/loki/services?window=10s", http.StatusBadRequest, "invalid_request"},
		{"/api/v1/datasources/loki/services?window=24h", http.StatusBadRequest, "invalid_request"},
		{"/api/v1/datasources/loki/services?bogus=1", http.StatusBadRequest, "invalid_request"},
		{"/api/v1/datasources/loki/services?window=1h&window=2h", http.StatusBadRequest, "invalid_request"},
	}
	for _, tt := range tests {
		expectError(t, getServices(router, tt.path), tt.status, tt.code)
	}
	if got := source.calls.Load(); got != 0 {
		t.Fatalf("source asked %d times for refused requests", got)
	}
}

func TestServicesRouteAnswersFromTheCacheWithinTheMinute(t *testing.T) {
	source := &servicesSource{fakeDatasource: &fakeDatasource{}, values: []string{"shop"}}
	router := servicesRouter(t, source, auth.RoleViewer)
	for range 3 {
		if rec := getServices(router, "/api/v1/datasources/loki/services"); rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
	}
	if got := source.calls.Load(); got != 1 {
		t.Fatalf("source asked %d times, want 1", got)
	}
}

func TestServicesRouteAnswers504WhenTheSourceStaysSilent(t *testing.T) {
	source := &servicesSource{fakeDatasource: &fakeDatasource{}, hang: true}
	router := servicesRouter(t, source, auth.RoleViewer, queryproxy.WithServicesTimeout(40*time.Millisecond))
	start := time.Now()
	expectError(t, getServices(router, "/api/v1/datasources/loki/services"), http.StatusGatewayTimeout, "query_timeout")
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("waited %v", elapsed)
	}
}

func TestServicesRouteAnswersAnUnreachableSourceAs502(t *testing.T) {
	source := &servicesSource{fakeDatasource: &fakeDatasource{}, failed: datasource.Errorf(datasource.ErrUnavailable, "connection failed")}
	expectError(t, getServices(servicesRouter(t, source, auth.RoleViewer), "/api/v1/datasources/loki/services"), http.StatusBadGateway, "datasource_unavailable")
}
