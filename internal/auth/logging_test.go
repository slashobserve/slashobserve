package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/oauth2"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/logtest"
)

const leakedDetail = `password authentication failed for user "bob" at db.internal`

type failingDB struct{ err error }

func (f failingDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, f.err
}

func (f failingDB) Query(context.Context, string, ...any) (pgx.Rows, error) { return nil, f.err }

func (f failingDB) QueryRow(context.Context, string, ...any) pgx.Row { return failingRow(f) }

type failingRow struct{ err error }

func (r failingRow) Scan(...any) error { return r.err }

func newLoggingService(t *testing.T, issuer string) (*gin.Engine, *gin.RouterGroup, *logtest.Capture, *Service) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	logger, logs := logtest.New()
	public, err := url.Parse("http://localhost:8080")
	if err != nil {
		t.Fatal(err)
	}
	queries := dbsqlc.New(failingDB{&pgconn.PgError{Code: "28P01", Message: leakedDetail}})
	svc, err := New(Config{PublicURL: public, IssuerURL: issuer, ClientID: "id", ClientSecret: "client-secret-value"}, queries, logger)
	if err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	authenticated := svc.Mount(engine.Group("/api/v1"))
	authenticated.GET("/ping", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	return engine, authenticated, logs, svc
}

func TestSessionLookupFailureLogsOnlyCodeAndType(t *testing.T) {
	engine, _, logs, svc := newLoggingService(t, "http://issuer.invalid")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	req.Header.Set("Cookie", svc.sessionCookie+"=token")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d, want 500", rec.Code)
	}
	record := logs.Find(t, "resolve session")
	logs.AssertSafeFailure(t, record, "session_lookup_failed", leakedDetail, "bob", "db.internal")
	if record["sqlstate"] != "28P01" {
		t.Errorf("sqlstate = %v, want 28P01", record["sqlstate"])
	}
}

func TestBearerKeyLookupFailureLogsOnlyCodeAndType(t *testing.T) {
	engine, _, logs, _ := newLoggingService(t, "http://issuer.invalid")
	token := keyPrefix + strings.Repeat("a", keyLength-len(keyPrefix))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ping", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d, want 500", rec.Code)
	}
	logs.AssertSafeFailure(t, logs.Find(t, "resolve bearer key"), "key_lookup_failed", leakedDetail, token)
}

func TestOIDCDiscoveryFailureDoesNotLogTheProviderResponse(t *testing.T) {
	const providerBody = "upstream-secret-body-31337"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, providerBody, http.StatusInternalServerError)
	}))
	t.Cleanup(upstream.Close)
	engine, _, logs, _ := newLoggingService(t, upstream.URL)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/login", nil))
	if rec.Code != http.StatusFound {
		t.Fatalf("status %d, want 302", rec.Code)
	}
	logs.AssertSafeFailure(t, logs.Find(t, "oidc discovery failed"), "oidc_unavailable", providerBody, upstream.URL)
}

func TestRefusedSignInLogsCodeAndTypeOnly(t *testing.T) {
	_, _, logs, svc := newLoggingService(t, "http://issuer.invalid")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/callback", nil)
	cause := &pgconn.PgError{Code: "08006", Message: leakedDetail}
	svc.refuse(c, &loginFailure{code: "exchange_failed", err: cause})
	record := logs.Find(t, "sign-in failed")
	logs.AssertSafeFailure(t, record, "exchange_failed", leakedDetail)
	if record["sqlstate"] != "08006" {
		t.Errorf("sqlstate = %v, want 08006", record["sqlstate"])
	}
}

func TestCodeExchangeFailureLogsTheOAuthErrorCodeOnly(t *testing.T) {
	cases := []struct {
		name      string
		retrieve  *oauth2.RetrieveError
		wantError any
	}{
		{"standard code", &oauth2.RetrieveError{ErrorCode: "invalid_grant", ErrorDescription: "leaked-description", Body: []byte("leaked-body")}, "invalid_grant"},
		{"free text is not a code", &oauth2.RetrieveError{ErrorCode: "code leaked-body here"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, logs, svc := newLoggingService(t, "http://issuer.invalid")
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/callback", nil)
			svc.refuse(c, &loginFailure{code: "exchange_failed", err: fmt.Errorf("exchange: %w", tc.retrieve)})
			record := logs.Find(t, "sign-in failed")
			logs.AssertSafeFailure(t, record, "exchange_failed", "leaked-description", "leaked-body")
			if record["oauth_error"] != tc.wantError {
				t.Errorf("oauth_error = %v, want %v", record["oauth_error"], tc.wantError)
			}
		})
	}
}
