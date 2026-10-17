package server

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/gin-gonic/gin"
)

func contractEngine(t *testing.T, logs io.Writer, register func(*gin.RouterGroup)) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.ReleaseMode)
	ct, err := newContract(context.Background(), slog.New(slog.NewTextHandler(logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	engine.Use(securityHeaders)
	group := engine.Group(apiPrefix)
	group.Use(ct.middleware)
	register(group)
	return engine
}

func serve(engine *gin.Engine, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

func TestViolationDropsHandlerHeadersAndKeepsSecurityHeaders(t *testing.T) {
	var logs bytes.Buffer
	engine := contractEngine(t, &logs, func(g *gin.RouterGroup) {
		g.GET("/me", func(c *gin.Context) {
			c.Header("Set-Cookie", "session=leak")
			c.Header("Location", "/elsewhere")
			c.Header("WWW-Authenticate", "Bearer")
			c.Header("Cache-Control", "public")
			c.JSON(http.StatusOK, gin.H{"undocumented": true})
		})
	})
	rec := serve(engine, httptest.NewRequest(http.MethodGet, "/api/v1/me", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d, want 500", rec.Code)
	}
	for _, name := range []string{"Set-Cookie", "Location", "WWW-Authenticate", "Cache-Control"} {
		if v := rec.Header().Get(name); v != "" {
			t.Errorf("violation response leaks %s: %q", name, v)
		}
	}
	if rec.Header().Get("X-Frame-Options") != "DENY" || rec.Header().Get("Content-Security-Policy") != "frame-ancestors 'none'" {
		t.Errorf("security headers are missing: %v", rec.Header())
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Errorf("content type %q", got)
	}
	if !strings.Contains(logs.String(), contractViolation) {
		t.Errorf("violation was not logged: %s", logs.String())
	}
}

func TestValidResponseKeepsHandlerHeaders(t *testing.T) {
	var logs bytes.Buffer
	engine := contractEngine(t, &logs, func(g *gin.RouterGroup) {
		g.GET("/me", func(c *gin.Context) {
			c.Header("WWW-Authenticate", `Bearer realm="slashobserve"`)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "code": "unauthorized"})
		})
	})
	rec := serve(engine, httptest.NewRequest(http.MethodGet, "/api/v1/me", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401 (logs: %s)", rec.Code, logs.String())
	}
	if rec.Header().Get("WWW-Authenticate") == "" || rec.Header().Get("X-Frame-Options") != "DENY" {
		t.Errorf("headers lost: %v", rec.Header())
	}
}

func TestHandlerAbortedByClientDisconnectIsNotAViolation(t *testing.T) {
	var logs bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	engine := contractEngine(t, &logs, func(g *gin.RouterGroup) {
		g.GET("/me", func(c *gin.Context) {
			cancel()
			c.Abort()
		})
	})
	serve(engine, httptest.NewRequest(http.MethodGet, "/api/v1/me", nil).WithContext(ctx))
	if strings.Contains(logs.String(), contractViolation) {
		t.Fatalf("an aborted handler was logged as a violation: %s", logs.String())
	}
}

type countingBody struct{ read int }

func (b *countingBody) Read(p []byte) (int, error) {
	b.read += len(p)
	for i := range p {
		p[i] = ' '
	}
	return len(p), nil
}

func TestOversizedRequestBodyIsRefusedWithoutBuffering(t *testing.T) {
	var logs bytes.Buffer
	reached := false
	engine := contractEngine(t, &logs, func(g *gin.RouterGroup) {
		g.POST("/service-accounts", func(c *gin.Context) { reached = true })
	})
	body := &countingBody{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/service-accounts", body)
	req.Header.Set("Content-Type", "application/json")
	rec := serve(engine, req)
	if rec.Code != http.StatusRequestEntityTooLarge || reached {
		t.Fatalf("status %d reached=%v, want 413 without reaching the handler", rec.Code, reached)
	}
	if body.read > 2*maxContractBodyBytes {
		t.Fatalf("read %d bytes, want a bounded read", body.read)
	}
}

func TestDescribeFallsBackWhenReasonIsEmpty(t *testing.T) {
	err := &openapi3filter.RequestError{
		Parameter: &openapi3.Parameter{In: "query", Name: "limit"},
		Err:       errors.New("secret-value"),
	}
	got := describe(err)
	if !strings.Contains(got, "parameter query limit") || strings.HasSuffix(got, ": ") {
		t.Fatalf("describe returned %q", got)
	}
	if strings.Contains(got, "secret-value") {
		t.Fatalf("describe echoes the underlying error: %q", got)
	}
}
