package mcpauth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestFlowIssuesTokensThatAuthenticateAsThePerson(t *testing.T) {
	r := newRig(t)
	clientID := r.registerClient(t, "claude-code")

	got := r.connect(t, ada, clientID)
	if got.Type != "Bearer" || got.Expires != 3600 || !strings.HasPrefix(got.Access, "soa_") || !strings.HasPrefix(got.Refresh, "sor_") {
		t.Fatalf("tokens %+v", got)
	}
	p, name, ok := r.authenticate(t, got.Access)
	if !ok || p.ID != ada.ID || p.Kind != "user" || p.Name != "Ada" || name != "claude-code" {
		t.Fatalf("Authenticate = %+v %q %v", p, name, ok)
	}

	logs := r.logs.String()
	for _, secret := range []string{got.Access, got.Refresh, secretState, callback, "claude-code", challenge, verifier} {
		if strings.Contains(logs, secret) {
			t.Errorf("logs contain %q", secret)
		}
	}
	for _, stored := range []string{got.Access, got.Refresh} {
		var n int
		if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM oauth_tokens WHERE position($1 in encode(token_hash, 'escape')) > 0`, stored).Scan(&n); err != nil || n != 0 {
			t.Fatalf("a token is stored in the clear: %d, %v", n, err)
		}
	}
}

func TestAuthenticateRereadsTheRoleOnEveryRequest(t *testing.T) {
	r := newRig(t)
	got := r.connect(t, ada, r.registerClient(t, "claude-code"))
	r.db.Exec(t, `UPDATE users SET role = 'admin', role_source = 'manual' WHERE id = $1`, ada.ID)
	p, _, ok := r.authenticate(t, got.Access)
	if !ok || p.Role != "admin" {
		t.Fatalf("Authenticate = %+v %v, want the new role", p, ok)
	}
}

func TestAuthenticateIgnoresCredentialsThatAreNotAccessTokens(t *testing.T) {
	r := newRig(t)
	got := r.connect(t, ada, r.registerClient(t, "claude-code"))
	for name, header := range map[string]string{
		"refresh token":  "Bearer " + got.Refresh,
		"service key":    "Bearer sok_" + strings.Repeat("a", 43),
		"short":          "Bearer soa_short",
		"no scheme":      got.Access,
		"basic":          "Basic " + got.Access,
		"empty":          "",
		"lowercase body": "Bearer " + strings.ToLower(got.Access),
	} {
		_, _, ok, err := r.server.Authenticate(t.Context(), header)
		if err != nil || ok {
			t.Errorf("%s: Authenticate = %v, %v", name, ok, err)
		}
	}
	if _, _, ok, _ := r.server.Authenticate(t.Context(), "bearer "+got.Access); !ok {
		t.Error("the Bearer scheme is case-insensitive")
	}
}

func TestOAuthAccessTokenIsRefusedOutsideMCP(t *testing.T) {
	r := newRig(t)
	got := r.connect(t, ada, r.registerClient(t, "claude-code"))
	engine := gin.New()
	r.people.Mount(engine.Group("/api/v1"))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+got.Access)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET /api/v1/me with an OAuth access token = %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRefreshRotatesAndOldAccessKeepsWorking(t *testing.T) {
	r := newRig(t)
	clientID := r.registerClient(t, "claude-code")
	first := r.connect(t, ada, clientID)

	second := r.tokens(t, refreshForm(clientID, first.Refresh))
	if second.Access == first.Access || second.Refresh == first.Refresh {
		t.Fatal("refresh returned the same tokens")
	}
	for _, access := range []string{first.Access, second.Access} {
		if _, _, ok := r.authenticate(t, access); !ok {
			t.Fatalf("access token %s stopped working after a refresh", access[:6])
		}
	}
	var rotated bool
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT rotated_at IS NOT NULL FROM oauth_tokens WHERE token_hash = $1`, hashOf(first.Refresh)).Scan(&rotated); err != nil || !rotated {
		t.Fatalf("old refresh token rotated = %v, err %v", rotated, err)
	}
}

func TestRefreshReuseWithinTheLeewayOnlyFails(t *testing.T) {
	r := newRig(t)
	clientID := r.registerClient(t, "claude-code")
	first := r.connect(t, ada, clientID)
	second := r.tokens(t, refreshForm(clientID, first.Refresh))

	r.tokenError(t, refreshForm(clientID, first.Refresh), http.StatusBadRequest, "invalid_grant")
	if _, _, ok := r.authenticate(t, second.Access); !ok {
		t.Fatal("a refresh inside the leeway disconnected the person")
	}
	if reason := r.revokedReason(t, ada.ID, clientID); reason != "" {
		t.Fatalf("grant revoked as %q inside the leeway", reason)
	}
	r.tokens(t, refreshForm(clientID, second.Refresh))
}

func TestRefreshReuseAfterTheLeewayRevokesTheGrant(t *testing.T) {
	r := newRig(t)
	clientID := r.registerClient(t, "claude-code")
	first := r.connect(t, ada, clientID)
	second := r.tokens(t, refreshForm(clientID, first.Refresh))
	r.db.Exec(t, `UPDATE oauth_tokens SET rotated_at = now() - interval '1 minute' WHERE token_hash = $1`, hashOf(first.Refresh))

	r.tokenError(t, refreshForm(clientID, first.Refresh), http.StatusBadRequest, "invalid_grant")
	if _, _, ok := r.authenticate(t, second.Access); ok {
		t.Fatal("the new access token still works after refresh reuse")
	}
	if _, _, ok := r.authenticate(t, first.Access); ok {
		t.Fatal("the old access token still works after refresh reuse")
	}
	r.tokenError(t, refreshForm(clientID, second.Refresh), http.StatusBadRequest, "invalid_grant")
	if reason := r.revokedReason(t, ada.ID, clientID); reason != "refresh_reuse" {
		t.Fatalf("revoked reason %q, want refresh_reuse", reason)
	}
	if !strings.Contains(r.logs.String(), "oauth refresh token reused; grant revoked") {
		t.Error("reuse was not logged")
	}
}

func TestRefreshChecksClientAndResource(t *testing.T) {
	r := newRig(t)
	clientID, otherID := r.registerClient(t, "claude-code"), r.registerClient(t, "other")
	got := r.connect(t, ada, clientID)
	r.tokenError(t, refreshForm(otherID, got.Refresh), http.StatusBadRequest, "invalid_grant")
	form := refreshForm(clientID, got.Refresh)
	form.Set("resource", "http://elsewhere.test/mcp")
	r.tokenError(t, form, http.StatusBadRequest, "invalid_target")
	r.tokens(t, refreshForm(clientID, got.Refresh))
}

func TestRefreshTokenOfAnExpiredGrantFails(t *testing.T) {
	r := newRig(t)
	clientID := r.registerClient(t, "claude-code")
	got := r.connect(t, ada, clientID)
	r.db.Exec(t, `UPDATE oauth_tokens SET expires_at = now() - interval '1 second' WHERE token_hash = $1`, hashOf(got.Refresh))
	r.tokenError(t, refreshForm(clientID, got.Refresh), http.StatusBadRequest, "invalid_grant")
}

func TestRefreshExpiryNeverPassesTheGrantLimit(t *testing.T) {
	r := newRig(t)
	clientID := r.registerClient(t, "claude-code")
	got := r.connect(t, ada, clientID)
	r.db.Exec(t, `UPDATE oauth_grants SET created_at = now() - interval '89 days 23 hours' WHERE user_id = $1`, ada.ID)
	next := r.tokens(t, refreshForm(clientID, got.Refresh))
	if next.Expires > 3600 || next.Expires < 3000 {
		t.Fatalf("expires_in %d", next.Expires)
	}
	var withinLimit bool
	err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT expires_at <= created_at + interval '90 days' FROM oauth_grants WHERE user_id = $1`, ada.ID).Scan(&withinLimit)
	if err != nil || !withinLimit {
		t.Fatalf("grant expiry within the 90 day limit = %v, err %v", withinLimit, err)
	}
	r.db.Exec(t, `UPDATE oauth_grants SET created_at = now() - interval '91 days' WHERE user_id = $1`, ada.ID)
	r.tokenError(t, refreshForm(clientID, next.Refresh), http.StatusBadRequest, "invalid_grant")
}

func TestCodeReuseRevokesTheGrantItProduced(t *testing.T) {
	r := newRig(t)
	clientID := r.registerClient(t, "claude-code")
	code := r.code(t, ada, clientID, callback)
	got := r.tokens(t, exchangeForm(clientID, code))

	r.tokenError(t, exchangeForm(clientID, code), http.StatusBadRequest, "invalid_grant")
	if _, _, ok := r.authenticate(t, got.Access); ok {
		t.Fatal("the access token works after its code was reused")
	}
	if reason := r.revokedReason(t, ada.ID, clientID); reason != "code_reuse" {
		t.Fatalf("revoked reason %q, want code_reuse", reason)
	}
	if strings.Contains(r.logs.String(), code) {
		t.Error("the code was logged")
	}
}

func TestCodeExchangeRefusesWhatDoesNotMatch(t *testing.T) {
	r := newRig(t)
	clientID, otherID := r.registerClient(t, "claude-code"), r.registerClient(t, "other")
	cases := []struct {
		name   string
		change func(url.Values)
		status int
		code   string
	}{
		{"wrong verifier", func(f url.Values) { f.Set("code_verifier", strings.Repeat("a", 43)) }, 400, "invalid_grant"},
		{"short verifier", func(f url.Values) { f.Set("code_verifier", "short") }, 400, "invalid_grant"},
		{"wrong redirect uri", func(f url.Values) { f.Set("redirect_uri", "http://127.0.0.1:40000/callback") }, 400, "invalid_grant"},
		{"another client", func(f url.Values) { f.Set("client_id", otherID) }, 400, "invalid_grant"},
		{"another resource", func(f url.Values) { f.Set("resource", "http://elsewhere.test/mcp") }, 400, "invalid_target"},
		{"unknown code", func(f url.Values) { f.Set("code", "soc_"+strings.Repeat("a", 43)) }, 400, "invalid_grant"},
		{"malformed code", func(f url.Values) { f.Set("code", "nope") }, 400, "invalid_grant"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code := r.code(t, ada, clientID, callback)
			form := exchangeForm(clientID, code)
			c.change(form)
			r.tokenError(t, form, c.status, c.code)
			r.tokens(t, exchangeForm(clientID, code))
		})
	}
}

func TestExpiredCodeIsRefused(t *testing.T) {
	r := newRig(t)
	clientID := r.registerClient(t, "claude-code")
	code := r.code(t, ada, clientID, callback)
	r.db.Exec(t, `UPDATE oauth_codes SET expires_at = now() - interval '1 second'`)
	r.tokenError(t, exchangeForm(clientID, code), http.StatusBadRequest, "invalid_grant")
}

func TestTokenEndpointRefusesBadRequests(t *testing.T) {
	r := newRig(t)
	clientID := r.registerClient(t, "claude-code")
	code := r.code(t, ada, clientID, callback)

	secret := exchangeForm(clientID, code)
	secret.Set("client_secret", "s3cret")
	r.tokenError(t, secret, http.StatusUnauthorized, "invalid_client")

	req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(exchangeForm(clientID, code).Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Basic Zm9vOmJhcg==")
	rec := httptest.NewRecorder()
	r.server.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "invalid_client") {
		t.Fatalf("Authorization header = %d: %s", rec.Code, rec.Body.String())
	}

	asJSON := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(`{"grant_type":"authorization_code"}`))
	asJSON.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	r.server.ServeHTTP(rec, asJSON)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_request") {
		t.Fatalf("JSON body = %d: %s", rec.Code, rec.Body.String())
	}

	duplicate := exchangeForm(clientID, code)
	duplicate.Add("redirect_uri", "http://127.0.0.1:1/other")
	r.tokenError(t, duplicate, http.StatusBadRequest, "invalid_request")

	r.tokenError(t, url.Values{"grant_type": {"password"}, "client_id": {clientID}}, http.StatusBadRequest, "unsupported_grant_type")
	r.tokenError(t, url.Values{"grant_type": {"authorization_code"}}, http.StatusBadRequest, "invalid_request")

	get := httptest.NewRecorder()
	r.server.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/oauth/token", nil))
	if get.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET /oauth/token = %d", get.Code)
	}
	options := httptest.NewRecorder()
	r.server.ServeHTTP(options, httptest.NewRequest(http.MethodOptions, "/oauth/token", nil))
	if options.Code != http.StatusNoContent || options.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("OPTIONS /oauth/token = %d %v", options.Code, options.Header())
	}
}

func TestTokenEndpointRateLimitsPerClient(t *testing.T) {
	r := newRig(t)
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {"nope"}, "client_id": {"c1"}}
	for range tokenPerClientPerMinute {
		r.oauth(http.MethodPost, "/oauth/token", form)
	}
	rec := r.oauth(http.MethodPost, "/oauth/token", form)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("request past the per-client limit = %d %v", rec.Code, rec.Header())
	}
	form.Set("client_id", "c2")
	if rec := r.oauth(http.MethodPost, "/oauth/token", form); rec.Code == http.StatusTooManyRequests {
		t.Fatal("another client was limited")
	}
}

func TestAuthorizeRefusesWithoutRedirectingWhenTheClientIsNotTrusted(t *testing.T) {
	r := newRig(t)
	clientID := r.registerClient(t, "claude-code")
	cases := map[string]url.Values{
		"unknown client":          authorizeQuery("nope", callback),
		"missing client":          authorizeQuery("", callback),
		"unregistered redirect":   authorizeQuery(clientID, "http://127.0.0.1:33418/elsewhere"),
		"other host redirect":     authorizeQuery(clientID, "https://evil.example/cb"),
		"localhost for 127.0.0.1": authorizeQuery(clientID, "http://localhost:33418/callback"),
	}
	dup := authorizeQuery(clientID, callback)
	dup.Add("redirect_uri", "https://evil.example/cb")
	cases["duplicate redirect"] = dup
	for name, query := range cases {
		t.Run(name, func(t *testing.T) {
			rec := r.authorize(query)
			if rec.Code != http.StatusBadRequest || rec.Header().Get("Location") != "" {
				t.Fatalf("authorize = %d to %q", rec.Code, rec.Header().Get("Location"))
			}
			if !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain") || !strings.Contains(rec.Body.String(), "Start connecting again from your agent.") {
				t.Fatalf("body %q", rec.Body.String())
			}
		})
	}
}

func TestAuthorizeAnswersFailuresByRedirectingToTheClient(t *testing.T) {
	r := newRig(t)
	clientID := r.registerClient(t, "claude-code")
	cases := []struct {
		name   string
		change func(url.Values)
		code   string
	}{
		{"implicit", func(q url.Values) { q.Set("response_type", "token") }, "unsupported_response_type"},
		{"no challenge", func(q url.Values) { q.Del("code_challenge") }, "invalid_request"},
		{"short challenge", func(q url.Values) { q.Set("code_challenge", "abc") }, "invalid_request"},
		{"plain method", func(q url.Values) { q.Set("code_challenge_method", "plain") }, "invalid_request"},
		{"no method", func(q url.Values) { q.Del("code_challenge_method") }, "invalid_request"},
		{"another resource", func(q url.Values) { q.Set("resource", "http://elsewhere.test/mcp") }, "invalid_target"},
		{"one wrong resource of two", func(q url.Values) { q.Add("resource", "http://elsewhere.test/mcp") }, "invalid_target"},
		{"long state", func(q url.Values) { q.Set("state", strings.Repeat("s", maxStateBytes+1)) }, "invalid_request"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			query := authorizeQuery(clientID, callback)
			c.change(query)
			rec := r.authorize(query)
			if rec.Code != http.StatusFound {
				t.Fatalf("authorize = %d: %s", rec.Code, rec.Body.String())
			}
			location, err := url.Parse(rec.Header().Get("Location"))
			if err != nil || location.Host != "127.0.0.1:33418" {
				t.Fatalf("redirected to %q", rec.Header().Get("Location"))
			}
			got := location.Query()
			if got.Get("error") != c.code || got.Get("error_description") == "" || got.Get("iss") != testPublicURL {
				t.Fatalf("redirect query %v, want error %s and iss", got, c.code)
			}
			if c.name != "long state" && got.Get("state") != secretState {
				t.Fatalf("state %q not echoed", got.Get("state"))
			}
			if c.name == "long state" && got.Get("state") != "" {
				t.Fatal("an oversized state was echoed")
			}
		})
	}
}

func TestAuthorizeAcceptsAnotherLoopbackPortAndAnAbsentResource(t *testing.T) {
	r := newRig(t)
	clientID := r.registerClient(t, "claude-code", "http://localhost/callback")
	query := authorizeQuery(clientID, "http://localhost:51234/callback")
	query.Del("resource")
	rec := r.authorize(query)
	if rec.Code != http.StatusFound || !strings.HasPrefix(rec.Header().Get("Location"), "/connect/") {
		t.Fatalf("authorize = %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("authorize redirect is cacheable")
	}
	var stored string
	err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT redirect_uri FROM oauth_requests`).Scan(&stored)
	if err != nil || stored != "http://localhost:51234/callback" {
		t.Fatalf("stored redirect %q, err %v", stored, err)
	}
}

func TestAuthorizeUsesTheOnlyRegisteredRedirectWhenAbsent(t *testing.T) {
	r := newRig(t)
	clientID := r.registerClient(t, "claude-code")
	query := authorizeQuery(clientID, "")
	query.Del("redirect_uri")
	if rec := r.authorize(query); rec.Code != http.StatusFound || !strings.HasPrefix(rec.Header().Get("Location"), "/connect/") {
		t.Fatalf("authorize = %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	two := r.registerClient(t, "two", callback, "https://claude.ai/api/mcp/auth_callback")
	query.Set("client_id", two)
	if rec := r.authorize(query); rec.Code != http.StatusBadRequest {
		t.Fatalf("authorize without a redirect for two registered = %d", rec.Code)
	}
}

func TestAuthorizeCapsPendingRequests(t *testing.T) {
	r := newRig(t)
	clientID := r.registerClient(t, "claude-code")
	r.db.Exec(t, `INSERT INTO oauth_requests (client_id, redirect_uri, code_challenge, resource, expires_at)
SELECT $1, 'http://localhost/cb', $2, 'r', now() + interval '5 minutes' FROM generate_series(1, $3::int)`, clientID, challenge, maxPendingRequests)
	rec := r.authorize(authorizeQuery(clientID, callback))
	location, err := url.Parse(rec.Header().Get("Location"))
	if rec.Code != http.StatusFound || err != nil || location.Query().Get("error") != "temporarily_unavailable" {
		t.Fatalf("authorize at the cap = %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	r.db.Exec(t, `UPDATE oauth_requests SET expires_at = now() - interval '1 second'`)
	if rec := r.authorize(authorizeQuery(clientID, callback)); !strings.HasPrefix(rec.Header().Get("Location"), "/connect/") {
		t.Fatalf("authorize after the requests expired = %d to %q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestConsentShowsWhatThePersonApproves(t *testing.T) {
	r := newRig(t)
	clientID := r.registerClient(t, "claude-code")
	id := r.pendingRequest(t, clientID, callback)

	rec := r.api(ada, http.MethodGet, "/oauth/requests/"+id)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET request = %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`"name":"Claude Code"`, `"redirect_host":"127.0.0.1:33418"`, `"local":true`, `"replaces_grant":false`, `"email":"ada@example.com"`, `"role":"viewer"`, `"id":"` + id + `"`} {
		if !strings.Contains(body, want) {
			t.Errorf("body lacks %s: %s", want, body)
		}
	}

	r.connect(t, ada, clientID)
	again := r.pendingRequest(t, clientID, callback)
	if got := r.api(ada, http.MethodGet, "/oauth/requests/"+again).Body.String(); !strings.Contains(got, `"replaces_grant":true`) {
		t.Errorf("a reconnecting person is not told it replaces a grant: %s", got)
	}
	if got := r.api(bob, http.MethodGet, "/oauth/requests/"+again).Body.String(); !strings.Contains(got, `"replaces_grant":false`) {
		t.Errorf("another person is told it replaces a grant: %s", got)
	}
}

func TestConsentShowsAnUnnamedClientAsNull(t *testing.T) {
	r := newRig(t)
	clientID := r.registerClient(t, "")
	body := r.api(ada, http.MethodGet, "/oauth/requests/"+r.pendingRequest(t, clientID, callback)).Body.String()
	if !strings.Contains(body, `"name":null`) {
		t.Fatalf("body %s", body)
	}
}

func TestConsentRoutesAreForPeopleOnly(t *testing.T) {
	r := newRig(t)
	id := r.pendingRequest(t, r.registerClient(t, "claude-code"), callback)
	for _, call := range []struct{ method, path string }{
		{http.MethodGet, "/oauth/requests/" + id},
		{http.MethodPost, "/oauth/requests/" + id + "/approve"},
		{http.MethodPost, "/oauth/requests/" + id + "/deny"},
	} {
		if rec := r.api(bot, call.method, call.path); rec.Code != http.StatusForbidden {
			t.Errorf("%s %s as a key = %d", call.method, call.path, rec.Code)
		}
	}
	if rec := r.api(ada, http.MethodGet, "/oauth/requests/"+id); rec.Code != http.StatusOK {
		t.Fatalf("the request did not survive the refused calls: %d", rec.Code)
	}
}

func TestConsentRequestNotFound(t *testing.T) {
	r := newRig(t)
	id := r.pendingRequest(t, r.registerClient(t, "claude-code"), callback)
	for name, path := range map[string]string{
		"unknown":   "/oauth/requests/00000000-0000-4000-8000-000000000000",
		"malformed": "/oauth/requests/x",
	} {
		for _, suffix := range []string{"", "/approve", "/deny"} {
			method := http.MethodGet
			if suffix != "" {
				method = http.MethodPost
			}
			if rec := r.api(ada, method, path+suffix); rec.Code != http.StatusNotFound {
				t.Errorf("%s %s%s = %d", method, name, suffix, rec.Code)
			}
		}
	}
	r.db.Exec(t, `UPDATE oauth_requests SET expires_at = now() - interval '1 second'`)
	if rec := r.api(ada, http.MethodGet, "/oauth/requests/"+id); rec.Code != http.StatusNotFound {
		t.Errorf("an expired request = %d", rec.Code)
	}
	if rec := r.api(ada, http.MethodPost, "/oauth/requests/"+id+"/approve"); rec.Code != http.StatusNotFound {
		t.Errorf("approving an expired request = %d", rec.Code)
	}
}

func TestApprovalIsSingleUse(t *testing.T) {
	r := newRig(t)
	id := r.pendingRequest(t, r.registerClient(t, "claude-code"), callback)
	r.approve(t, ada, id)
	if rec := r.api(ada, http.MethodPost, "/oauth/requests/"+id+"/approve"); rec.Code != http.StatusNotFound {
		t.Fatalf("second approval = %d", rec.Code)
	}
}

func TestDenyRedirectsWithAccessDenied(t *testing.T) {
	r := newRig(t)
	id := r.pendingRequest(t, r.registerClient(t, "claude-code"), callback)
	rec := r.api(ada, http.MethodPost, "/oauth/requests/"+id+"/deny")
	if rec.Code != http.StatusOK {
		t.Fatalf("deny = %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		RedirectTo string `json:"redirect_to"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	q := redirectQuery(t, out.RedirectTo)
	if q.Get("error") != "access_denied" || q.Get("state") != secretState || q.Get("iss") != testPublicURL || q.Get("code") != "" {
		t.Fatalf("deny redirect query %v", q)
	}
	if rec := r.api(ada, http.MethodGet, "/oauth/requests/"+id); rec.Code != http.StatusNotFound {
		t.Fatalf("a denied request is still pending: %d", rec.Code)
	}
	var codes int
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM oauth_codes`).Scan(&codes); err != nil || codes != 0 {
		t.Fatalf("%d codes after a denial, err %v", codes, err)
	}
}

func TestTwentyFirstApprovalIsRefusedAndKeepsTheRequest(t *testing.T) {
	r := newRig(t)
	r.db.Exec(t, `WITH c AS (INSERT INTO oauth_clients (id, redirect_uris) SELECT 'bulk-' || i, ARRAY['http://localhost/cb'] FROM generate_series(1, $2::int) i RETURNING id)
INSERT INTO oauth_grants (client_id, user_id, resource, expires_at) SELECT id, $1, 'r', now() + interval '1 day' FROM c`, ada.ID, maxLiveGrantsPerPerson)
	clientID := r.registerClient(t, "claude-code")
	id := r.pendingRequest(t, clientID, callback)

	rec := r.api(ada, http.MethodPost, "/oauth/requests/"+id+"/approve")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"code":"too_many_grants"`) || !strings.Contains(rec.Body.String(), "Settings") {
		t.Fatalf("21st approval = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := r.api(ada, http.MethodGet, "/oauth/requests/"+id); rec.Code != http.StatusOK {
		t.Fatalf("the refused request was consumed: %d", rec.Code)
	}
	if rec := r.api(bob, http.MethodPost, "/oauth/requests/"+id+"/approve"); rec.Code != http.StatusOK {
		t.Fatalf("another person at no cap = %d: %s", rec.Code, rec.Body.String())
	}
}

func TestReconnectingAtTheCapReplacesTheOldGrant(t *testing.T) {
	r := newRig(t)
	clientID := r.registerClient(t, "claude-code")
	old := r.connect(t, ada, clientID)
	r.db.Exec(t, `WITH c AS (INSERT INTO oauth_clients (id, redirect_uris) SELECT 'bulk-' || i, ARRAY['http://localhost/cb'] FROM generate_series(1, $2::int) i RETURNING id)
INSERT INTO oauth_grants (client_id, user_id, resource, expires_at) SELECT id, $1, 'r', now() + interval '1 day' FROM c`, ada.ID, maxLiveGrantsPerPerson-1)

	fresh := r.connect(t, ada, clientID)
	if _, _, ok := r.authenticate(t, old.Access); ok {
		t.Fatal("the replaced grant's access token still works")
	}
	if _, _, ok := r.authenticate(t, fresh.Access); !ok {
		t.Fatal("the new grant's access token does not work")
	}
	r.tokenError(t, refreshForm(clientID, old.Refresh), http.StatusBadRequest, "invalid_grant")
	var live int
	err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM oauth_grants WHERE user_id = $1 AND client_id = $2 AND revoked_at IS NULL`, ada.ID, clientID).Scan(&live)
	if err != nil || live != 1 {
		t.Fatalf("%d live grants for the client, err %v", live, err)
	}
}

func TestRevokeEndsTheGrantOnlyForTheClientThatOwnsIt(t *testing.T) {
	r := newRig(t)
	clientID, otherID := r.registerClient(t, "claude-code"), r.registerClient(t, "other")
	got := r.connect(t, ada, clientID)

	rec := r.oauth(http.MethodPost, "/oauth/revoke", url.Values{"token": {got.Refresh}, "client_id": {otherID}})
	if rec.Code != http.StatusOK || rec.Body.Len() != 0 {
		t.Fatalf("revoke by another client = %d %q", rec.Code, rec.Body.String())
	}
	if _, _, ok := r.authenticate(t, got.Access); !ok {
		t.Fatal("another client revoked the grant")
	}

	for _, token := range []string{"nope", "sor_" + strings.Repeat("a", 43)} {
		if rec := r.oauth(http.MethodPost, "/oauth/revoke", url.Values{"token": {token}, "client_id": {clientID}}); rec.Code != http.StatusOK || rec.Body.Len() != 0 {
			t.Fatalf("revoking an unknown token = %d %q", rec.Code, rec.Body.String())
		}
	}

	rec = r.oauth(http.MethodPost, "/oauth/revoke", url.Values{"token": {got.Refresh}, "client_id": {clientID}})
	if rec.Code != http.StatusOK || rec.Body.Len() != 0 {
		t.Fatalf("revoke = %d %q", rec.Code, rec.Body.String())
	}
	if _, _, ok := r.authenticate(t, got.Access); ok {
		t.Fatal("the access token works after the client revoked its grant")
	}
	r.tokenError(t, refreshForm(clientID, got.Refresh), http.StatusBadRequest, "invalid_grant")
	if reason := r.revokedReason(t, ada.ID, clientID); reason != "client" {
		t.Fatalf("revoked reason %q, want client", reason)
	}
}

func TestRevokeWithTheAccessTokenWorksToo(t *testing.T) {
	r := newRig(t)
	clientID := r.registerClient(t, "claude-code")
	got := r.connect(t, ada, clientID)
	r.oauth(http.MethodPost, "/oauth/revoke", url.Values{"token": {got.Access}, "client_id": {clientID}})
	if _, _, ok := r.authenticate(t, got.Access); ok {
		t.Fatal("the access token works after it was revoked")
	}
}

func TestDeletingThePersonDisconnectsTheirAgents(t *testing.T) {
	r := newRig(t)
	got := r.connect(t, ada, r.registerClient(t, "claude-code"))
	r.db.Exec(t, `DELETE FROM users WHERE id = $1`, ada.ID)
	if _, _, ok := r.authenticate(t, got.Access); ok {
		t.Fatal("a deleted person's access token still works")
	}
}

func TestGrantForAnotherResourceDoesNotAuthenticate(t *testing.T) {
	r := newRig(t)
	got := r.connect(t, ada, r.registerClient(t, "claude-code"))
	r.db.Exec(t, `UPDATE oauth_grants SET resource = 'http://old.example/mcp'`)
	if _, _, ok := r.authenticate(t, got.Access); ok {
		t.Fatal("a grant bound to another resource authenticated")
	}
}

func TestExpiredGrantAndExpiredAccessTokenDoNotAuthenticate(t *testing.T) {
	r := newRig(t)
	got := r.connect(t, ada, r.registerClient(t, "claude-code"))
	r.db.Exec(t, `UPDATE oauth_tokens SET expires_at = now() - interval '1 second' WHERE kind = 'access'`)
	if _, _, ok := r.authenticate(t, got.Access); ok {
		t.Fatal("an expired access token authenticated")
	}
	r.db.Exec(t, `UPDATE oauth_tokens SET expires_at = now() + interval '1 hour' WHERE kind = 'access'`)
	r.db.Exec(t, `UPDATE oauth_grants SET expires_at = now() - interval '1 second'`)
	if _, _, ok := r.authenticate(t, got.Access); ok {
		t.Fatal("an access token of an expired grant authenticated")
	}
}

func TestAuthenticateRecordsLastUse(t *testing.T) {
	r := newRig(t)
	got := r.connect(t, ada, r.registerClient(t, "claude-code"))
	r.authenticate(t, got.Access)
	var used bool
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT last_used_at IS NOT NULL FROM oauth_grants`).Scan(&used); err != nil || !used {
		t.Fatalf("last_used_at set = %v, err %v", used, err)
	}
}

func TestPruneRemovesOnlyWhatIsDead(t *testing.T) {
	r := newRig(t)
	clientID := r.registerClient(t, "claude-code")
	live := r.connect(t, ada, clientID)
	r.db.Exec(t, `INSERT INTO oauth_clients (id, redirect_uris, registered_at) VALUES ('stale', ARRAY['http://localhost/cb'], now() - interval '2 days'), ('fresh', ARRAY['http://localhost/cb'], now())`)
	r.db.Exec(t, `INSERT INTO oauth_codes (code_hash, client_id, user_id, redirect_uri, code_challenge, resource, expires_at) VALUES (sha256('old'), $1, $2, 'r', 'c', 'r', now() - interval '20 minutes'), (sha256('new'), $1, $2, 'r', 'c', 'r', now() + interval '1 minute')`, clientID, bob.ID)
	if err := PruneJob(r.db.DB)(t.Context()); err != nil {
		t.Fatal(err)
	}
	count := func(query string) int {
		var n int
		if err := r.db.SQLDB(t).QueryRowContext(t.Context(), query).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count(`SELECT count(*) FROM oauth_clients WHERE id IN ('stale')`); n != 0 {
		t.Errorf("stale unused client kept")
	}
	if n := count(`SELECT count(*) FROM oauth_clients WHERE id IN ('fresh', '` + clientID + `')`); n != 2 {
		t.Errorf("fresh or used clients pruned, %d left", n)
	}
	if n := count(`SELECT count(*) FROM oauth_codes WHERE code_hash = sha256('old')`); n != 0 {
		t.Error("old code kept")
	}
	if n := count(`SELECT count(*) FROM oauth_codes WHERE code_hash = sha256('new')`); n != 1 {
		t.Error("live code pruned")
	}
	if _, _, ok := r.authenticate(t, live.Access); !ok {
		t.Error("prune disconnected a live grant")
	}
}

func TestConcurrentRefreshesOfOneTokenIssueOnce(t *testing.T) {
	r := newRig(t)
	clientID := r.registerClient(t, "claude-code")
	got := r.connect(t, ada, clientID)
	const callers = 6
	codes := make(chan int, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Go(func() {
			codes <- r.oauth(http.MethodPost, "/oauth/token", refreshForm(clientID, got.Refresh)).Code
		})
	}
	wg.Wait()
	close(codes)
	var ok, refused int
	for code := range codes {
		switch code {
		case http.StatusOK:
			ok++
		case http.StatusBadRequest:
			refused++
		}
	}
	if ok != 1 || refused != callers-1 {
		t.Fatalf("%d refreshes succeeded and %d were refused, want 1 and %d", ok, refused, callers-1)
	}
	if reason := r.revokedReason(t, ada.ID, clientID); reason != "" {
		t.Fatalf("concurrent refreshes revoked the grant as %q", reason)
	}
}
