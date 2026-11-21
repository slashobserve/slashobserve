package auth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/opaquetoken"
)

type storedRefusal struct {
	action, outcome, reason, actorKind, authMethod, targetKind, targetName, data string
}

func (r auditRig) storedRefusal(t *testing.T, request string) storedRefusal {
	t.Helper()
	r.flush()
	var e storedRefusal
	err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT action, outcome, coalesce(reason, ''), actor_kind, coalesce(auth_method, ''), coalesce(target_kind, ''), coalesce(target_name, ''), data::text
FROM audit_events WHERE request_id = $1`, request).Scan(&e.action, &e.outcome, &e.reason, &e.actorKind, &e.authMethod, &e.targetKind, &e.targetName, &e.data)
	if err != nil {
		t.Fatalf("event of request %s: %v", request, err)
	}
	return e
}

func (r auditRig) mentions(t *testing.T, secret string) int {
	t.Helper()
	var n int
	err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM audit_events WHERE position($1 in row_to_json(audit_events)::text) > 0`, secret).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func (r auditRig) postWithBearer(t *testing.T, header string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dashboards/55555555-5555-4555-8555-555555555555/versions", nil)
	req.Header.Set("Authorization", header)
	rec := httptest.NewRecorder()
	r.engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	return rec.Header().Get(audit.RequestIDHeader)
}

func TestAnUnknownKeyIsRecordedByItsDisplayPrefixAlone(t *testing.T) {
	r := newAuditRig(t)
	token, _, err := opaquetoken.New("sok_")
	if err != nil {
		t.Fatal(err)
	}
	got := r.storedRefusal(t, r.postWithBearer(t, "Bearer "+token))
	want := storedRefusal{action: "auth.credential_rejected", outcome: "denied", reason: "unknown_credential", actorKind: "anonymous", authMethod: "api_key", targetKind: "credential", targetName: token[:12], data: `{"count": 1}`}
	if got != want {
		t.Fatalf("event = %+v, want %+v", got, want)
	}
	if n := r.mentions(t, token); n != 0 {
		t.Fatalf("%d audit rows carry the rejected key", n)
	}
}

func TestAnUnknownOAuthTokenIsRecordedAsAnOAuthCredential(t *testing.T) {
	r := newAuditRig(t)
	token, _, err := opaquetoken.New("soa_")
	if err != nil {
		t.Fatal(err)
	}
	got := r.storedRefusal(t, r.postWithBearer(t, "Bearer "+token))
	if got.authMethod != "oauth" || got.reason != "unknown_credential" || got.targetName != token[:12] {
		t.Fatalf("event = %+v", got)
	}
}

func TestAMalformedBearerIsRecordedWithoutAnyOfItsText(t *testing.T) {
	r := newAuditRig(t)
	const junk = "sok_not-a-real-key-shape-but-secret-looking"
	got := r.storedRefusal(t, r.postWithBearer(t, "Bearer "+junk))
	want := storedRefusal{action: "auth.credential_rejected", outcome: "denied", reason: "malformed_credential", actorKind: "anonymous", authMethod: "api_key", data: `{"count": 1}`}
	if got != want {
		t.Fatalf("event = %+v, want %+v", got, want)
	}
	if n := r.mentions(t, junk[:20]); n != 0 {
		t.Fatalf("%d audit rows carry the rejected text", n)
	}
}

func TestACrossOriginSignOutIsRecordedAsRefusedForThePerson(t *testing.T) {
	r := newAuditRig(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.Header.Set("Origin", "https://elsewhere.example")
	req.Header.Set("Cookie", r.cookie)
	rec := httptest.NewRecorder()
	r.engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	got := r.storedRefusal(t, rec.Header().Get(audit.RequestIDHeader))
	if got.action != "auth.csrf_rejected" || got.outcome != "denied" || got.reason != "csrf_rejected" || got.data != `{"count": 1}` {
		t.Fatalf("event = %+v", got)
	}
}

func TestTheFirstRefusalOfAKeyIsRecordedAtOnceAndTheRepeatsAreOneLaterEventWithTheirCount(t *testing.T) {
	r := newAuditRig(t)
	token, _, err := opaquetoken.New("sok_")
	if err != nil {
		t.Fatal(err)
	}
	var first string
	for i := range 5 {
		request := r.postWithBearer(t, "Bearer "+token)
		if i == 0 {
			first = request
		}
	}
	if got := r.storedRefusal(t, first); got.data != `{"count": 1}` {
		t.Fatalf("data of the first refusal = %s, want count 1", got.data)
	}
	var rows, later int
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*), count(*) FILTER (WHERE data = '{"count": 4}') FROM audit_events WHERE action = 'auth.credential_rejected'`).Scan(&rows, &later); err != nil || rows != 2 || later != 1 {
		t.Fatalf("rows = %d with %d later row of count 4, %v, want 2 and 1", rows, later, err)
	}
}

func TestAnUnreachableIdentityProviderIsRecordedAsARefusedSignInStart(t *testing.T) {
	r := newCallbackRig(t, "ada@example.com", nil)
	r.issuer.server.Close()
	rec := httptest.NewRecorder()
	r.engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/login", nil))
	if rec.Code != http.StatusFound || !strings.Contains(rec.Header().Get("Location"), "error=unavailable") {
		t.Fatalf("login answered %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	got := r.eventsOf(t, rec.Header().Get(audit.RequestIDHeader))
	want := signInEvent{action: "auth.login_failed", outcome: "failed", reason: "unavailable", actorKind: "anonymous", authMethod: "none", data: `{"count": 1}`}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("events = %+v, want %+v", got, want)
	}
}

func TestAProviderErrorIsRecordedWithItsCodeAlone(t *testing.T) {
	r := newCallbackRig(t, "ada@example.com", nil)
	rec := httptest.NewRecorder()
	r.engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/login", nil))
	location, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	state := location.Query().Get("state")
	query := url.Values{"error": {"access_denied"}, "error_description": {"secret words"}, "state": {state}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/callback?"+query.Encode(), nil)
	req.Header.Set("Cookie", r.service.loginCookie+"="+state)
	rec = httptest.NewRecorder()
	r.engine.ServeHTTP(rec, req)
	got := r.eventsOf(t, rec.Header().Get(audit.RequestIDHeader))
	if len(got) != 1 || got[0].reason != "access_denied" || got[0].data != `{"count": 1, "provider_error": "access_denied"}` {
		t.Fatalf("events = %+v", got)
	}
	var mentions int
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM audit_events WHERE row_to_json(audit_events)::text LIKE '%secret words%'`).Scan(&mentions); err != nil || mentions != 0 {
		t.Fatalf("%d audit rows carry the provider's description, %v", mentions, err)
	}
}
