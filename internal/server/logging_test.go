package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/slashobserve/slashobserve/internal/clientgone"
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

type fakeUsage struct{ err error }

func (f fakeUsage) DatasourceUsage(context.Context) (map[string]int, error) { return nil, f.err }

func TestDatasourceListIgnoresAClientThatWentAway(t *testing.T) {
	cases := []struct {
		name          string
		err           error
		cancelRequest bool
		wantStatus    int
		wantError     bool
	}{
		{"request canceled", fmt.Errorf("count: %w", context.Canceled), true, clientgone.Status, false},
		{"server side cancellation", fmt.Errorf("count: %w", context.Canceled), false, http.StatusInternalServerError, true},
		{"database failure", errors.New("database exploded"), false, http.StatusInternalServerError, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logger, logs := logtest.New()
			engine := gin.New()
			registerDatasourceRoutes(engine.Group("/api/v1"), nil, fakeUsage{tc.err}, logger)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if tc.cancelRequest {
				cancel()
			}
			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/datasources", nil))
			if rec.Code != tc.wantStatus {
				t.Errorf("status %d, want %d", rec.Code, tc.wantStatus)
			}
			if got := strings.Contains(logs.String(), `"level":"ERROR"`); got != tc.wantError {
				t.Errorf("ERROR logged = %v, want %v: %s", got, tc.wantError, logs.String())
			}
		})
	}
}

func TestAccessLogRecordsAClientThatWentAwayAs499WithNoBytes(t *testing.T) {
	logger, logs := logtest.New()
	engine := gin.New()
	engine.Use(logRequests(logger))
	engine.GET("/gin", func(c *gin.Context) { clientgone.Abort(c, logger, "gin handler") })
	engine.Any("/wrapped", gin.WrapH(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientgone.WriteHeader(w, r, logger, "wrapped handler")
	})))
	contract, err := newContract(t.Context(), logger)
	if err != nil {
		t.Fatal(err)
	}
	checked := engine.Group(apiPrefix)
	checked.Use(contract.middleware)
	checked.GET("/me", func(c *gin.Context) { clientgone.Abort(c, logger, "contract checked handler") })
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	for _, path := range []string{"/gin", "/wrapped", apiPrefix + "/me"} {
		engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(canceled, http.MethodGet, path, nil))
	}
	records := 0
	for _, record := range logs.Records(t) {
		if record["msg"] != "http request" {
			continue
		}
		records++
		if record["status"] != float64(clientgone.Status) || record["bytes"] != float64(0) {
			t.Errorf("%v: status %v bytes %v, want %d and 0", record["path"], record["status"], record["bytes"], clientgone.Status)
		}
	}
	if records != 3 {
		t.Fatalf("access log records = %d, want 3", records)
	}
}
