package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/slashobserve/slashobserve/internal/audit"
)

func TestSigningOutRecordsThePersonWhoSignedOut(t *testing.T) {
	r := newAuditRig(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.Header.Set("Cookie", r.cookie)
	rec := httptest.NewRecorder()
	r.engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	r.flush()
	var kind, id, method, targetKind string
	err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT actor_kind, coalesce(actor_id::text, ''), coalesce(auth_method, ''), coalesce(target_kind, '')
FROM audit_events WHERE request_id = $1 AND action = 'auth.logout'`, rec.Header().Get(audit.RequestIDHeader)).Scan(&kind, &id, &method, &targetKind)
	if err != nil {
		t.Fatal(err)
	}
	if kind != "user" || id != r.userID.String() || method != "session" || targetKind != "session" {
		t.Fatalf("actor = %s %s via %s on %s, want user %s via session on session", kind, id, method, targetKind, r.userID.String())
	}
}
