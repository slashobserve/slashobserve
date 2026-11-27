package server

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestReadyzFailsOnceDrainingStartsAndHealthzStaysUp(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	phases := servingPhases()
	ran := 0
	engine := gin.New()
	registerHealthRoutes(engine, slog.New(slog.DiscardHandler), []Check{{Name: "database", Run: func(context.Context) error { ran++; return nil }}}, phases.Current)

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusOK || ran != 1 {
		t.Fatalf("before draining: status %d, checks run %d, want 200 and 1", rec.Code, ran)
	}
	if got := rec.Body.String(); got != `{"status":"ok","checks":{"database":"ok"},"phase":"ready"}` {
		t.Errorf("serving body = %s", got)
	}

	phases.Enter(PhaseStopping)
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		rec = httptest.NewRecorder()
		engine.ServeHTTP(rec, httptest.NewRequest(method, "/readyz", nil))
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s /readyz while draining: status %d, want 503", method, rec.Code)
		}
	}
	rec = httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if got := rec.Body.String(); got != `{"status":"stopping","checks":{},"phase":"stopping"}` {
		t.Errorf("draining body = %s", got)
	}
	if ran != 1 {
		t.Errorf("a check ran while draining: %d runs", ran)
	}

	rec = httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("/healthz while draining: status %d, want 200", rec.Code)
	}
}

func TestReadyzKeepsThePhaseWhenACheckFails(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	registerHealthRoutes(engine, slog.New(slog.DiscardHandler), []Check{{Name: "database", Run: func(context.Context) error { return context.DeadlineExceeded }}}, servingPhases().Current)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d, want 503", rec.Code)
	}
	if got := rec.Body.String(); got != `{"status":"unavailable","checks":{"database":"error"},"phase":"ready"}` {
		t.Errorf("body = %s", got)
	}
}

func TestReadyzOfAServerWithoutPhasesSaysReady(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	registerHealthRoutes(engine, slog.New(slog.DiscardHandler), nil, nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if got := rec.Body.String(); rec.Code != http.StatusOK || got != `{"status":"ok","checks":{},"phase":"ready"}` {
		t.Errorf("status %d body %s", rec.Code, got)
	}
}

func TestStrictTransportSecurityOnlyWhenThePublicURLIsHTTPS(t *testing.T) {
	cases := []struct {
		name string
		base string
		want string
	}{
		{"https", "https://observe.example.com", strictTransportSecurity},
		{"http", "http://localhost:8080", ""},
		{"unset", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var base *url.URL
			if tc.base != "" {
				var err error
				if base, err = url.Parse(tc.base); err != nil {
					t.Fatal(err)
				}
			}
			gin.SetMode(gin.ReleaseMode)
			engine := gin.New()
			engine.Use(securityHeaders(servedOverHTTPS(base)))
			engine.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })
			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
			if got := rec.Header().Get("Strict-Transport-Security"); got != tc.want {
				t.Errorf("Strict-Transport-Security = %q, want %q", got, tc.want)
			}
			if rec.Header().Get("X-Frame-Options") != "DENY" {
				t.Error("the other security headers are missing")
			}
		})
	}
}
