package auth

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/opaquetoken"
)

type auditRig struct {
	db       *dbtest.Database
	engine   *gin.Engine
	recorder *audit.Recorder
	userID   pgtype.UUID
	cookie   string
}

func newAuditRig(t *testing.T) auditRig {
	t.Helper()
	database := dbtest.New(t)
	s, err := New(Config{PublicURL: mustURL(t, "http://localhost:8080"), IssuerURL: "http://issuer.example/dex"}, database.Queries(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	user, err := database.Queries().UpsertUser(t.Context(), dbsqlc.UpsertUserParams{OidcIssuer: "i", OidcSubject: "s", Email: "ada@example.com", Name: "Ada"})
	if err != nil {
		t.Fatal(err)
	}
	token, hash, err := opaquetoken.New("sos_")
	if err != nil {
		t.Fatal(err)
	}
	err = database.Queries().CreateSession(t.Context(), dbsqlc.CreateSessionParams{TokenHash: hash, UserID: user.ID, ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	recorder := audit.New(audit.Config{DB: database.DB})
	mw, err := recorder.Middleware()
	if err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	authenticated := s.Mount(engine.Group("/api/v1", mw))
	authenticated.POST("/dashboards/:id/versions", RoleViewer, func(c *gin.Context) { c.Status(http.StatusCreated) })
	return auditRig{db: database, engine: engine, recorder: recorder, userID: user.ID, cookie: s.sessionCookie + "=" + token}
}

func (r auditRig) post(t *testing.T, cookie, origin string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dashboards/55555555-5555-4555-8555-555555555555/versions", nil)
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	rec := httptest.NewRecorder()
	r.engine.ServeHTTP(rec, req)
	return rec.Code, rec.Header().Get(audit.RequestIDHeader)
}

func (r auditRig) flush() {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r.recorder.Run(ctx)
}

type refusal struct {
	action, outcome, reason, actorKind, actorID, authMethod string
	status                                                  int
}

func (r auditRig) refusalOf(t *testing.T, request string) refusal {
	t.Helper()
	var e refusal
	err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT action, outcome, coalesce(reason, ''), actor_kind, coalesce(actor_id::text, ''), coalesce(auth_method, ''), coalesce(status, 0)
FROM audit_events WHERE request_id = $1`, request).Scan(&e.action, &e.outcome, &e.reason, &e.actorKind, &e.actorID, &e.authMethod, &e.status)
	if err != nil {
		t.Fatalf("event of request %s: %v", request, err)
	}
	return e
}

func TestAnUnauthenticatedRequestRecordsADeniedEvent(t *testing.T) {
	r := newAuditRig(t)
	status, request := r.post(t, "", "")
	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", status)
	}
	r.flush()
	want := refusal{action: "dashboard.update", outcome: "denied", reason: "unauthenticated", actorKind: "anonymous", authMethod: "none", status: 401}
	if got := r.refusalOf(t, request); got != want {
		t.Fatalf("event = %+v, want %+v", got, want)
	}
}

func TestACrossOriginRejectionRecordsADeniedEventForThePerson(t *testing.T) {
	r := newAuditRig(t)
	status, request := r.post(t, r.cookie, "https://elsewhere.example")
	if status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", status)
	}
	r.flush()
	want := refusal{action: "dashboard.update", outcome: "denied", reason: "csrf_rejected", actorKind: "user", actorID: r.userID.String(), authMethod: "session", status: 403}
	if got := r.refusalOf(t, request); got != want {
		t.Fatalf("event = %+v, want %+v", got, want)
	}
}

func TestASignedInRequestCarriesThePersonIntoItsEvent(t *testing.T) {
	r := newAuditRig(t)
	status, request := r.post(t, r.cookie, "")
	if status != http.StatusCreated {
		t.Fatalf("status = %d, want 201", status)
	}
	r.flush()
	want := refusal{action: "dashboard.update", outcome: "success", actorKind: "user", actorID: r.userID.String(), authMethod: "session", status: 201}
	if got := r.refusalOf(t, request); got != want {
		t.Fatalf("event = %+v, want %+v", got, want)
	}
}
