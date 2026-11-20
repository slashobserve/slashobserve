package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/logtest"
)

func TestTheRequestLogNamesTheRequestThePrincipalAndTheRoute(t *testing.T) {
	logger, logs := logtest.New()
	recordRequests, err := auditMiddleware(audit.New(audit.Config{}))
	if err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	engine.Use(logRequests(logger))
	api := engine.Group(apiPrefix, recordRequests...)
	signedIn := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	api.GET("/dashboards", func(c *gin.Context) {
		audit.Update(c.Request.Context(), func(s *audit.Scope) {
			s.Actor = audit.Actor{Kind: audit.ActorUser, ID: uuid.NullUUID{UUID: signedIn, Valid: true}}
		})
		c.Status(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/dashboards", nil))
	requestID := rec.Header().Get(audit.RequestIDHeader)
	if _, err := uuid.Parse(requestID); err != nil {
		t.Fatalf("%s = %q, want a UUID", audit.RequestIDHeader, requestID)
	}
	record := logs.Find(t, "http request")
	if record["request_id"] != requestID || record["route"] != "/api/v1/dashboards" || record["principal"] != "user:"+signedIn.String() {
		t.Fatalf("record = %v, want request_id %s, the route and the principal", record, requestID)
	}
}

func TestARequestOutsideTheAPILogsItsRouteOnly(t *testing.T) {
	logger, logs := logtest.New()
	engine := gin.New()
	engine.Use(logRequests(logger))
	engine.GET("/readyz", func(c *gin.Context) { c.Status(http.StatusOK) })
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	record := logs.Find(t, "http request")
	if _, ok := record["request_id"]; ok || record["route"] != "/readyz" {
		t.Fatalf("a request outside the API logged %v, want the route and no request id", record)
	}
	if rec.Header().Get(audit.RequestIDHeader) != "" {
		t.Fatalf("a request outside the API carries %s", audit.RequestIDHeader)
	}
}
