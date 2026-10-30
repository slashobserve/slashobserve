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

	"github.com/slashobserve/slashobserve/internal/logtest"
)

func contractEngine(t *testing.T, logs io.Writer, register func(*gin.RouterGroup)) *gin.Engine {
	t.Helper()
	return contractEngineWithLogger(t, slog.New(slog.NewTextHandler(logs, nil)), register)
}

func contractEngineWithLogger(t *testing.T, logger *slog.Logger, register func(*gin.RouterGroup)) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.ReleaseMode)
	ct, err := newContract(context.Background(), logger)
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
	if rec.Header().Get("X-Frame-Options") != "DENY" || rec.Header().Get("Content-Security-Policy") != contentSecurityPolicy {
		t.Errorf("security headers are missing: %v", rec.Header())
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Errorf("content type %q", got)
	}
	if !strings.Contains(logs.String(), contractViolation) {
		t.Errorf("violation was not logged: %s", logs.String())
	}
}

func TestDocumentedImageRoutePassesTheContractCheck(t *testing.T) {
	cases := map[string]string{
		"image/png":     "\x89PNG\r\n\x1a\n\x00\x00",
		"image/jpeg":    "\xff\xd8\xff\xe0",
		"image/gif":     "GIF89a\x01\x00",
		"image/webp":    "RIFF\x00\x00\x00\x00WEBPVP8 ",
		"image/avif":    "\x00\x00\x00\x1cftypavif\x00\x00\x00\x00",
		"image/svg+xml": `<svg xmlns="http://www.w3.org/2000/svg"/>`,
	}
	for mediaType, body := range cases {
		t.Run(mediaType, func(t *testing.T) {
			var logs bytes.Buffer
			engine := contractEngine(t, &logs, func(g *gin.RouterGroup) {
				g.GET("/assets/image", func(c *gin.Context) {
					c.Header("Cache-Control", "private, max-age=3600")
					c.Header("Content-Security-Policy", "default-src 'none'")
					c.Header("Cross-Origin-Resource-Policy", "same-origin")
					c.Data(http.StatusOK, mediaType, []byte(body))
				})
			})
			rec := serve(engine, httptest.NewRequest(http.MethodGet, "/api/v1/assets/image?url=https%3A%2F%2Fcdn.example.com%2Fa", nil))
			if rec.Code != http.StatusOK || rec.Body.String() != body {
				t.Fatalf("status %d, body %q (logs: %s)", rec.Code, rec.Body, logs.String())
			}
			if rec.Header().Get("Content-Type") != mediaType {
				t.Errorf("content type %q", rec.Header().Get("Content-Type"))
			}
		})
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

func TestDescribeWithholdsRequestErrorText(t *testing.T) {
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

func TestViolationLogDoesNotEchoResponseValues(t *testing.T) {
	logger, logs := logtest.New()
	engine := contractEngineWithLogger(t, logger, func(g *gin.RouterGroup) {
		g.GET("/me", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"id": "leak-marker-5c1d"})
		})
	})
	rec := serve(engine, httptest.NewRequest(http.MethodGet, "/api/v1/me", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d, want 500", rec.Code)
	}
	record := logs.Find(t, contractViolation)
	if reason, _ := record["reason"].(string); reason == "" {
		t.Errorf("violation has no reason: %v", record)
	}
	logs.AssertAbsent(t, "leak-marker-5c1d")
}

func TestViolationNamesTheUndocumentedStatus(t *testing.T) {
	logger, logs := logtest.New()
	engine := contractEngineWithLogger(t, logger, func(g *gin.RouterGroup) {
		g.GET("/me", func(c *gin.Context) { c.JSON(http.StatusTeapot, gin.H{"error": "x", "code": "teapot"}) })
	})
	serve(engine, httptest.NewRequest(http.MethodGet, "/api/v1/me", nil))
	record := logs.Find(t, contractViolation)
	if record["status"] != float64(http.StatusTeapot) || record["reason"] != "status is not supported" {
		t.Errorf("record = %v, want status 418 and the unsupported-status reason", record)
	}
}

func TestViolationNamesTheUndocumentedResponseField(t *testing.T) {
	logger, logs := logtest.New()
	engine := contractEngineWithLogger(t, logger, func(g *gin.RouterGroup) {
		g.GET("/me", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"zeta_extra": "value-1", "alpha_extra": "value-2"})
		})
	})
	serve(engine, httptest.NewRequest(http.MethodGet, "/api/v1/me", nil))
	record := logs.Find(t, contractViolation)
	reason, _ := record["reason"].(string)
	if record["status"] != float64(http.StatusOK) || !strings.Contains(reason, "undocumented fields alpha_extra, zeta_extra") {
		t.Errorf("record = %v, want status 200 and the sorted field names", record)
	}
	logs.AssertAbsent(t, "value-1", "value-2")
}
