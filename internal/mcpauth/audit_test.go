package mcpauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/audit"
)

type auditedRig struct {
	*rig
	recorder *audit.Recorder
	edge     *gin.Engine
}

func newAuditedRig(t *testing.T) *auditedRig {
	t.Helper()
	r := newRig(t)
	recorder := audit.New(audit.Config{DB: r.db.DB})
	audit.Install(recorder)
	t.Cleanup(func() { audit.Install(nil) })
	mw, err := recorder.Middleware()
	if err != nil {
		t.Fatal(err)
	}
	edge := gin.New()
	for _, path := range r.server.Paths() {
		edge.Any(path, mw, gin.WrapH(r.server))
	}
	return &auditedRig{rig: r, recorder: recorder, edge: edge}
}

func (r *auditedRig) serve(req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	r.edge.ServeHTTP(rec, req)
	return rec
}

func (r *auditedRig) form(path string, form url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return r.serve(req)
}

func (r *auditedRig) register(body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/oauth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return r.serve(req)
}

func (r *auditedRig) authorizeAt(query url.Values) *httptest.ResponseRecorder {
	return r.serve(httptest.NewRequest(http.MethodGet, "/oauth/authorize?"+query.Encode(), nil))
}

type oauthEvent struct {
	outcome, reason, actorKind, actorID, authMethod, credentialID, origin, originClient, targetKind, targetID, targetName, data string
}

func (r *auditedRig) eventsOf(t *testing.T, action string) []oauthEvent {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r.recorder.Run(ctx)
	rows, err := r.db.SQLDB(t).QueryContext(t.Context(), `SELECT outcome, coalesce(reason, ''), actor_kind, coalesce(actor_id::text, ''), coalesce(auth_method, ''), coalesce(credential_id, ''),
  origin, coalesce(origin_client, ''), coalesce(target_kind, ''), coalesce(target_id, ''), coalesce(target_name, ''), data::text
FROM audit_events WHERE action = $1 ORDER BY occurred_at, id`, action)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var events []oauthEvent
	for rows.Next() {
		var e oauthEvent
		if err := rows.Scan(&e.outcome, &e.reason, &e.actorKind, &e.actorID, &e.authMethod, &e.credentialID, &e.origin, &e.originClient, &e.targetKind, &e.targetID, &e.targetName, &e.data); err != nil {
			t.Fatal(err)
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return events
}

func (r *auditedRig) only(t *testing.T, action string) oauthEvent {
	t.Helper()
	events := r.eventsOf(t, action)
	if len(events) != 1 {
		t.Fatalf("%s events = %+v, want exactly one", action, events)
	}
	return events[0]
}

func (r *auditedRig) mentions(t *testing.T, secret string) int {
	t.Helper()
	var n int
	err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM audit_events WHERE position($1 in row_to_json(audit_events)::text) > 0`, secret).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func (r *auditedRig) grantOf(t *testing.T, clientID string) string {
	t.Helper()
	var id string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT id::text FROM oauth_grants WHERE client_id = $1`, clientID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (r *auditedRig) registered(t *testing.T, name string) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{"client_name": name, "redirect_uris": []string{callback}})
	if err != nil {
		t.Fatal(err)
	}
	rec := r.register(string(body))
	var out struct {
		ClientID string `json:"client_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || rec.Code != http.StatusCreated {
		t.Fatalf("register = %d %s: %v", rec.Code, rec.Body, err)
	}
	return out.ClientID
}

func (r *auditedRig) assertNoSecrets(t *testing.T, secrets ...string) {
	t.Helper()
	for _, secret := range secrets {
		if n := r.mentions(t, secret); n != 0 {
			t.Errorf("%d audit rows carry %q", n, secret)
		}
	}
}

func TestRegisteringAClientIsRecordedWithItsNameAndNoRedirects(t *testing.T) {
	r := newAuditedRig(t)
	clientID := r.registered(t, "claude-code")
	want := oauthEvent{outcome: "success", actorKind: "anonymous", authMethod: "none", origin: "mcp", originClient: "claude-code", targetKind: "oauth_client", targetID: clientID, targetName: "claude-code", data: "{}"}
	if got := r.only(t, "oauth.client_register"); got != want {
		t.Fatalf("event = %+v, want %+v", got, want)
	}
	r.assertNoSecrets(t, callback, "127.0.0.1:33418")
}

func TestARefusedRegistrationIsRecordedAsDeniedWithItsCode(t *testing.T) {
	r := newAuditedRig(t)
	rec := r.register(`{"client_name":"sneaky","redirect_uris":["http://example.com/cb-secret"]}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("register = %d", rec.Code)
	}
	want := oauthEvent{outcome: "denied", reason: "invalid_redirect_uri", actorKind: "anonymous", authMethod: "none", origin: "mcp", originClient: "sneaky", targetKind: "oauth_client", targetName: "sneaky", data: "{}"}
	if got := r.only(t, "oauth.client_register"); got != want {
		t.Fatalf("event = %+v, want %+v", got, want)
	}
	r.assertNoSecrets(t, "cb-secret")
}

func TestAuthorizingIsRecordedWithoutTheStateOrTheRedirect(t *testing.T) {
	r := newAuditedRig(t)
	clientID := r.registered(t, "claude-code")
	rec := r.authorizeAt(authorizeQuery(clientID, callback))
	if rec.Code != http.StatusFound || !strings.HasPrefix(rec.Header().Get("Location"), "/connect/") {
		t.Fatalf("authorize = %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	want := oauthEvent{outcome: "success", actorKind: "anonymous", authMethod: "none", origin: "mcp", originClient: "claude-code", targetKind: "oauth_client", targetID: clientID, targetName: "claude-code", data: "{}"}
	if got := r.only(t, "oauth.authorize"); got != want {
		t.Fatalf("event = %+v, want %+v", got, want)
	}
	r.assertNoSecrets(t, secretState, callback, challenge)
}

func TestRefusedAuthorizationsAreRecordedAsDenied(t *testing.T) {
	r := newAuditedRig(t)
	clientID := r.registered(t, "claude-code")

	if rec := r.authorizeAt(authorizeQuery("no-such-client", callback)); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown client = %d", rec.Code)
	}
	bad := authorizeQuery(clientID, callback)
	bad.Set("code_challenge_method", "plain")
	if rec := r.authorizeAt(bad); rec.Code != http.StatusFound {
		t.Fatalf("bad params = %d", rec.Code)
	}
	events := r.eventsOf(t, "oauth.authorize")
	if len(events) != 2 {
		t.Fatalf("events = %+v, want two", events)
	}
	if events[0].outcome != "denied" || events[0].reason != "invalid_client" || events[0].targetKind != "oauth_client" || events[0].targetID != "" {
		t.Fatalf("unknown client event = %+v", events[0])
	}
	if events[1].outcome != "denied" || events[1].reason != "invalid_request" || events[1].targetID != clientID {
		t.Fatalf("bad parameters event = %+v", events[1])
	}
	r.assertNoSecrets(t, secretState, callback, "no-such-client")
}

func TestExchangingACodeIsRecordedForThePersonWhoApprovedIt(t *testing.T) {
	r := newAuditedRig(t)
	clientID := r.registered(t, "claude-code")
	code := r.code(t, ada, clientID, callback)
	rec := r.form("/oauth/token", exchangeForm(clientID, code))
	if rec.Code != http.StatusOK {
		t.Fatalf("token = %d: %s", rec.Code, rec.Body)
	}
	var issued tokens
	if err := json.Unmarshal(rec.Body.Bytes(), &issued); err != nil {
		t.Fatal(err)
	}
	grant := r.grantOf(t, clientID)
	want := oauthEvent{outcome: "success", actorKind: "user", actorID: ada.ID, authMethod: "oauth", credentialID: grant, origin: "mcp", originClient: "claude-code", targetKind: "oauth_grant", targetID: grant, targetName: "claude-code", data: `{"grant_type": "authorization_code"}`}
	if got := r.only(t, "oauth.token"); got != want {
		t.Fatalf("event = %+v, want %+v", got, want)
	}
	r.assertNoSecrets(t, issued.Access, issued.Refresh, code, verifier, secretState, callback)
}

func TestRefreshingIsRecordedWithItsGrantTypeAndNoTokens(t *testing.T) {
	r := newAuditedRig(t)
	clientID := r.registered(t, "claude-code")
	first := r.tokens(t, exchangeForm(clientID, r.code(t, ada, clientID, callback)))
	rec := r.form("/oauth/token", refreshForm(clientID, first.Refresh))
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh = %d: %s", rec.Code, rec.Body)
	}
	var second tokens
	if err := json.Unmarshal(rec.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	events := r.eventsOf(t, "oauth.token")
	if len(events) != 2 || events[1].data != `{"grant_type": "refresh_token"}` || events[1].actorID != ada.ID || events[1].outcome != "success" {
		t.Fatalf("events = %+v, want the code exchange then the refresh", events)
	}
	r.assertNoSecrets(t, first.Access, first.Refresh, second.Access, second.Refresh)
}

func TestARefusedTokenRequestIsRecordedAsDeniedAgainstTheClient(t *testing.T) {
	r := newAuditedRig(t)
	clientID := r.registered(t, "claude-code")
	code := r.code(t, ada, clientID, callback)
	r.tokens(t, exchangeForm(clientID, code))

	rec := r.form("/oauth/token", exchangeForm(clientID, code))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("reused code = %d", rec.Code)
	}
	events := r.eventsOf(t, "oauth.token")
	if len(events) != 2 {
		t.Fatalf("events = %+v, want the success then the refusal", events)
	}
	refused := events[1]
	if refused.outcome != "denied" || refused.reason != "invalid_grant" || refused.targetKind != "oauth_client" || refused.targetID != clientID || refused.data != `{"grant_type": "authorization_code"}` {
		t.Fatalf("refusal = %+v", refused)
	}
	r.assertNoSecrets(t, code)
}

func TestRevokingATokenIsRecordedForThePersonWhoseGrantItWas(t *testing.T) {
	r := newAuditedRig(t)
	clientID := r.registered(t, "claude-code")
	issued := r.connect(t, ada, clientID)
	rec := r.form("/oauth/revoke", url.Values{"token": {issued.Access}, "client_id": {clientID}})
	if rec.Code != http.StatusOK {
		t.Fatalf("revoke = %d", rec.Code)
	}
	grant := r.grantOf(t, clientID)
	want := oauthEvent{outcome: "success", actorKind: "user", actorID: ada.ID, authMethod: "oauth", credentialID: grant, origin: "mcp", originClient: "claude-code", targetKind: "oauth_grant", targetID: grant, targetName: "claude-code", data: "{}"}
	if got := r.only(t, "oauth.revoke"); got != want {
		t.Fatalf("event = %+v, want %+v", got, want)
	}
	r.assertNoSecrets(t, issued.Access, issued.Refresh)
}

func TestRevokingAnotherClientsTokenIsRecordedAsDeniedAndChangesNothing(t *testing.T) {
	r := newAuditedRig(t)
	owner := r.registered(t, "claude-code")
	stranger := r.registered(t, "other-agent")
	issued := r.connect(t, ada, owner)
	rec := r.form("/oauth/revoke", url.Values{"token": {issued.Access}, "client_id": {stranger}})
	if rec.Code != http.StatusOK {
		t.Fatalf("revoke = %d", rec.Code)
	}
	got := r.only(t, "oauth.revoke")
	if got.outcome != "denied" || got.reason != "client_mismatch" || got.targetKind != "oauth_client" || got.targetID != stranger {
		t.Fatalf("event = %+v", got)
	}
	if reason := r.revokedReason(t, ada.ID, owner); reason != "" {
		t.Fatalf("grant revoked as %q by another client", reason)
	}
	r.assertNoSecrets(t, issued.Access, issued.Refresh)
}

func TestAnUnknownClientIdIsNeverStoredAsATarget(t *testing.T) {
	r := newAuditedRig(t)
	const junk = "client-chosen-text-"
	long := junk + strings.Repeat("x", 400) + "\u202e"
	form := exchangeForm(long, "soc_"+strings.Repeat("a", 43))
	if rec := r.form("/oauth/token", form); rec.Code != http.StatusBadRequest && rec.Code != http.StatusUnauthorized {
		t.Fatalf("token = %d: %s", rec.Code, rec.Body)
	}
	got := r.only(t, "oauth.token")
	if got.outcome != "denied" || got.targetKind != "oauth_client" || got.targetID != "" || got.targetName != "" {
		t.Fatalf("event = %+v, want a denied event with no client id", got)
	}
	r.assertNoSecrets(t, junk)
}

func TestARevokeNamingAnUnknownClientIsNeverStoredAsATarget(t *testing.T) {
	r := newAuditedRig(t)
	owner := r.registered(t, "claude-code")
	issued := r.connect(t, ada, owner)
	const junk = "caller-chosen-client-"
	rec := r.form("/oauth/revoke", url.Values{"token": {issued.Access}, "client_id": {junk + strings.Repeat("y", 300)}})
	if rec.Code != http.StatusOK {
		t.Fatalf("revoke = %d", rec.Code)
	}
	got := r.only(t, "oauth.revoke")
	if got.outcome != "denied" || got.reason != "client_mismatch" || got.targetID != "" {
		t.Fatalf("event = %+v", got)
	}
	r.assertNoSecrets(t, junk)
}

func TestARefusedOAuthCallIsNotWrittenOnTheRequestPath(t *testing.T) {
	r := newAuditedRig(t)
	r.registered(t, "claude-code")
	if rec := r.form("/oauth/token", url.Values{"grant_type": {"refresh_token"}, "refresh_token": {"sor_" + strings.Repeat("a", 43)}, "client_id": {"nobody"}}); rec.Code == http.StatusOK {
		t.Fatalf("token = %d", rec.Code)
	}
	var written int
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM audit_events WHERE action = 'oauth.token'`).Scan(&written); err != nil {
		t.Fatal(err)
	}
	if written != 0 {
		t.Fatalf("%d refusals were inserted before the response went out, want them queued for the writer", written)
	}
	if got := r.only(t, "oauth.token"); got.outcome != "denied" {
		t.Fatalf("event = %+v", got)
	}
}
