package auth

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(dbtest.Main(m))
}

type sessionRig struct {
	t      *testing.T
	db     *dbtest.Database
	engine *gin.Engine
	userID pgtype.UUID
	cookie string
	login  string
}

func newSessionRig(t *testing.T) *sessionRig {
	t.Helper()
	return newSessionRigWithLogger(t, slog.New(slog.DiscardHandler))
}

func newSessionRigWithLogger(t *testing.T, logger *slog.Logger) *sessionRig {
	t.Helper()
	return newSessionRigWithIssuer(t, logger, "http://issuer.example/dex")
}

func newSessionRigWithIssuer(t *testing.T, logger *slog.Logger, issuer string) *sessionRig {
	t.Helper()
	database := dbtest.New(t)
	s, err := New(Config{PublicURL: mustURL(t, "http://localhost:8080"), IssuerURL: issuer}, database.Queries(), logger)
	if err != nil {
		t.Fatal(err)
	}
	user, err := database.Queries().UpsertUser(t.Context(), dbsqlc.UpsertUserParams{OidcIssuer: "i", OidcSubject: "s", Email: "ada@example.com", Name: "Ada"})
	if err != nil {
		t.Fatal(err)
	}
	token, hash, err := newToken("sos_")
	if err != nil {
		t.Fatal(err)
	}
	err = database.Queries().CreateSession(t.Context(), dbsqlc.CreateSessionParams{
		TokenHash: hash, UserID: user.ID, ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	database.Exec(t, "UPDATE users SET role = 'admin', last_seen_at = now() - interval '1 hour' WHERE id = $1", user.ID)

	engine := gin.New()
	authenticated := s.Mount(engine.Group("/api/v1"))
	authenticated.POST("/ping", RoleViewer, func(c *gin.Context) { c.Status(http.StatusNoContent) })
	authenticated.GET("/ping", RoleViewer, func(c *gin.Context) { c.Status(http.StatusNoContent) })
	return &sessionRig{t: t, db: database, engine: engine, userID: user.ID, cookie: s.sessionCookie + "=" + token, login: s.loginCookie}
}

func (r *sessionRig) request(method, path, origin string) int {
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Cookie", r.cookie)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	rec := httptest.NewRecorder()
	r.engine.ServeHTTP(rec, req)
	return rec.Code
}

func (r *sessionRig) idleFor() time.Duration {
	r.t.Helper()
	rows, err := r.db.Queries().ListUsers(r.t.Context(), dbsqlc.ListUsersParams{RowLimit: 10})
	if err != nil || len(rows) != 1 {
		r.t.Fatalf("list users: %v (%d rows)", err, len(rows))
	}
	return time.Since(rows[0].LastSeenAt.Time)
}

func TestRejectedCrossOriginRequestDoesNotMarkThePersonOnline(t *testing.T) {
	r := newSessionRig(t)
	if got := r.request(http.MethodPost, "/api/v1/ping", "http://evil.example"); got != http.StatusForbidden {
		t.Fatalf("cross-origin POST = %d, want 403", got)
	}
	if idle := r.idleFor(); idle < 30*time.Minute {
		t.Fatalf("a rejected request touched last_seen_at (idle %s)", idle)
	}
	if got := r.request(http.MethodPost, "/api/v1/ping", "http://localhost:8080"); got != http.StatusNoContent {
		t.Fatalf("same-origin POST = %d, want 204", got)
	}
	if idle := r.idleFor(); idle > time.Minute {
		t.Fatalf("an accepted request left last_seen_at stale (idle %s)", idle)
	}
}

func TestSessionRequestsTouchLastSeenAtMostOncePerInterval(t *testing.T) {
	r := newSessionRig(t)
	if got := r.request(http.MethodGet, "/api/v1/me", ""); got != http.StatusOK {
		t.Fatalf("GET /me = %d, want 200", got)
	}
	first := r.idleFor()
	if first > time.Minute {
		t.Fatalf("last_seen_at was not touched (idle %s)", first)
	}
	r.db.Exec(t, "UPDATE users SET last_seen_at = now() - interval '30 seconds' WHERE id = $1", r.userID)
	r.request(http.MethodGet, "/api/v1/me", "")
	if idle := r.idleFor(); idle < 25*time.Second {
		t.Fatalf("a touch inside the interval moved last_seen_at (idle %s)", idle)
	}
}

func TestValidFollowsTheSessionWithoutTouchingIt(t *testing.T) {
	r := newSessionRig(t)
	s, err := New(Config{PublicURL: mustURL(t, "http://localhost:8080"), IssuerURL: "http://issuer.example/dex"}, r.db.Queries(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Cookie", r.cookie)
	if ok, err := s.Valid(req); err != nil || !ok {
		t.Fatalf("Valid = %t, %v, want true", ok, err)
	}
	if idle := r.idleFor(); idle < 30*time.Minute {
		t.Fatalf("Valid touched last_seen_at (idle %s)", idle)
	}
	if _, err := r.db.Queries().DeleteUserSessions(t.Context(), r.userID); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.Valid(req); err != nil || ok {
		t.Fatalf("Valid after sign out = %t, %v, want false", ok, err)
	}
	if ok, err := s.Valid(httptest.NewRequest(http.MethodGet, "/", nil)); err != nil || ok {
		t.Fatalf("Valid without credentials = %t, %v, want false", ok, err)
	}
}
