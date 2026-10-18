package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/slashobserve/slashobserve/internal/logtest"
)

func TestReadinessFailureLogsOnlyCodeAndType(t *testing.T) {
	logger, logs := logtest.New()
	engine := gin.New()
	registerHealthRoutes(engine, logger, []Check{{Name: "database", Run: func(context.Context) error {
		return &pgconn.PgError{Code: "57P03", Message: `failed to connect to "user=bob host=db.internal"`}
	}}})
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d, want 503", rec.Code)
	}
	record := logs.Find(t, "readiness check failed")
	logs.AssertSafeFailure(t, record, "readiness_check_failed", "bob", "db.internal")
	if record["check"] != "database" || record["sqlstate"] != "57P03" {
		t.Errorf("record = %v, want check database and sqlstate 57P03", record)
	}
}

func TestPanicLogsTheTypeNotTheValue(t *testing.T) {
	logger, logs := logtest.New()
	engine := gin.New()
	engine.Use(recoverPanics(logger))
	engine.GET("/boom", func(*gin.Context) { panic(errors.New("query up{secret=\"s3\"} failed")) })
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/boom", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d, want 500", rec.Code)
	}
	record := logs.Find(t, "panic serving request")
	if record["panic_type"] != "*errors.errorString" {
		t.Errorf("panic_type = %v", record["panic_type"])
	}
	if _, ok := record["panic"]; ok {
		t.Errorf("record carries the panic value: %v", record)
	}
	logs.AssertAbsent(t, "s3", "secret")
}
