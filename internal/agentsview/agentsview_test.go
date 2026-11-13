package agentsview

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/api"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

var (
	ada  = auth.Principal{Kind: auth.KindUser, ID: "11111111-1111-4111-8111-111111111111", Name: "Ada", Role: auth.RoleViewer}
	bob  = auth.Principal{Kind: auth.KindUser, ID: "22222222-2222-4222-8222-222222222222", Name: "Bob", Role: auth.RoleViewer}
	root = auth.Principal{Kind: auth.KindUser, ID: "44444444-4444-4444-8444-444444444444", Name: "Root", Role: auth.RoleAdmin}
	bot  = auth.Principal{Kind: auth.KindServiceAccount, ID: "33333333-3333-4333-8333-333333333333", Name: "ci", Role: auth.RoleAdmin}
)

type overview struct {
	ObserveAgent struct {
		Enabled   bool    `json:"enabled"`
		Provider  *string `json:"provider"`
		Model     *string `json:"model"`
		Runtime   string  `json:"runtime"`
		Scope     string  `json:"scope"`
		Last7Days stats   `json:"last_7_days"`
	} `json:"observe_agent"`
	Connected []struct {
		GrantID     string  `json:"grant_id"`
		ClientName  string  `json:"client_name"`
		Mode        string  `json:"mode"`
		Presence    string  `json:"presence"`
		LastSeenAt  *string `json:"last_seen_at"`
		ConnectedAt string  `json:"connected_at"`
		Writes7d    int     `json:"writes_7d"`
		Last7Days   stats   `json:"last_7_days"`
	} `json:"connected"`
	ServiceAccounts []struct {
		ID         string  `json:"id"`
		Name       string  `json:"name"`
		Presence   string  `json:"presence"`
		LastSeenAt *string `json:"last_seen_at"`
		Last7Days  *stats  `json:"last_7_days"`
	} `json:"service_accounts"`
}

type stats struct {
	Runs                   int      `json:"runs"`
	Failed                 int      `json:"failed"`
	CostUSD                *float64 `json:"cost_usd"`
	MedianQueueWaitSeconds *float64 `json:"median_queue_wait_seconds"`
}

type rig struct {
	db     *dbtest.Database
	engine *gin.Engine
}

func newRig(t *testing.T, observe Observe) *rig {
	t.Helper()
	d := dbtest.New(t)
	d.Exec(t, `INSERT INTO users (id, oidc_issuer, oidc_subject, email, name) VALUES ($1, 'https://idp', 'ada', 'ada@example.com', 'Ada'), ($2, 'https://idp', 'bob', 'bob@example.com', 'Bob'), ($3, 'https://idp', 'root', 'root@example.com', 'Root')`, ada.ID, bob.ID, root.ID)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	principals := map[string]auth.Principal{ada.ID: ada, bob.ID: bob, root.ID: root, bot.ID: bot}
	api := engine.Group("/api/v1", func(c *gin.Context) {
		if p, ok := principals[c.GetHeader("X-Test-Principal")]; ok {
			c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), p))
		}
	})
	NewHandler(d.DB, observe, slog.New(slog.DiscardHandler)).Register(auth.NewRoutes(api))
	return &rig{db: d, engine: engine}
}

func (r *rig) get(by auth.Principal) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	req.Header.Set("X-Test-Principal", by.ID)
	rec := httptest.NewRecorder()
	r.engine.ServeHTTP(rec, req)
	return rec
}

func (r *rig) overview(t *testing.T, by auth.Principal) overview {
	t.Helper()
	rec := r.get(by)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /agents as %s = %d: %s", by.Name, rec.Code, rec.Body.String())
	}
	var out overview
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func (r *rig) grant(t *testing.T, user auth.Principal, client string) string {
	t.Helper()
	var name any
	if client != "" {
		name = client
	}
	var clientID, grantID string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `INSERT INTO oauth_clients (name, redirect_uris) VALUES ($1, ARRAY['http://127.0.0.1:33418/callback']) RETURNING id`, name).Scan(&clientID); err != nil {
		t.Fatal(err)
	}
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `INSERT INTO oauth_grants (client_id, user_id, resource, expires_at) VALUES ($1, $2::uuid, 'https://x/mcp', now() + interval '1 day') RETURNING id::text`, clientID, user.ID).Scan(&grantID); err != nil {
		t.Fatal(err)
	}
	return grantID
}

func (r *rig) serviceAccount(t *testing.T, name string, takesRuns bool) string {
	t.Helper()
	var id string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `INSERT INTO service_accounts (name, role, takes_runs) VALUES ($1, 'editor', $2) RETURNING id::text`, name, takesRuns).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

type run struct {
	kind, assignedID string
	by               auth.Principal
	state            string
	cost             *float64
	createdDaysAgo   int
	waitSeconds      int
}

func (r *rig) run(t *testing.T, in run) {
	t.Helper()
	status, failure := in.state, any(nil)
	switch in.state {
	case "canceled":
		status, failure = "failed", "canceled"
	case "failed":
		failure = "internal"
	}
	var usage any
	if in.cost != nil {
		raw, err := json.Marshal(map[string]any{"cost_usd": *in.cost})
		if err != nil {
			t.Fatal(err)
		}
		usage = string(raw)
	}
	r.db.Exec(t, `INSERT INTO agent_requests (source, prompt, requested_by_kind, requested_by_id, requested_by_name, status, state, failure_code, run_usage, assigned_kind, assigned_id, created_at, started_at)
		SELECT 'observe', 'p', 'user', $1::uuid, 'x', $2, $3, $4, $5::jsonb, $6, nullif($7, '')::uuid, at, CASE WHEN $2 <> 'queued' THEN at + make_interval(secs => $9) END
		FROM (SELECT now() - make_interval(days => $8) AS at) t`,
		in.by.ID, status, in.state, failure, usage, in.kind, in.assignedID, in.createdDaysAgo, in.waitSeconds)
}

func (r *rig) activity(t *testing.T, by auth.Principal, origin, client string, daysAgo int) {
	t.Helper()
	var clientArg any
	if client != "" {
		clientArg = client
	}
	r.db.Exec(t, `INSERT INTO activity (type, target_kind, target_id, actor_kind, actor_id, actor_name, origin, origin_client, created_at)
		VALUES ('dashboard.updated', 'dashboard', gen_random_uuid(), 'user', $1::uuid, 'x', $2, $3, now() - make_interval(days => $4))`, by.ID, origin, clientArg, daysAgo)
}

func price(v float64) *float64 { return &v }

func TestObserveAgentReportsTheConfiguredRuntime(t *testing.T) {
	for _, runtime := range []string{"go", "sidecar"} {
		r := newRig(t, Observe{Enabled: true, Runtime: runtime})
		if got := r.overview(t, ada).ObserveAgent.Runtime; got != runtime {
			t.Fatalf("runtime = %q, want the configured %q", got, runtime)
		}
	}
}

func TestAdminSeesWorkspaceScopeAndServiceAccountStats(t *testing.T) {
	r := newRig(t, Observe{Enabled: true, Provider: "anthropic", Model: "claude-sonnet-5-5", Runtime: "sidecar"})
	account := r.serviceAccount(t, "nightly", true)
	r.run(t, run{kind: "builtin", by: ada, state: "succeeded", cost: price(0.25), waitSeconds: 4})
	r.run(t, run{kind: "builtin", by: bob, state: "failed", cost: price(0.5), waitSeconds: 8})
	r.run(t, run{kind: "service_account", assignedID: account, by: ada, state: "succeeded", cost: price(1), waitSeconds: 2})

	got := r.overview(t, root)
	if o := got.ObserveAgent; !o.Enabled || o.Scope != "workspace" || o.Runtime != "sidecar" || o.Provider == nil || *o.Provider != "anthropic" || o.Model == nil || *o.Model != "claude-sonnet-5-5" {
		t.Fatalf("observe agent = %+v, want enabled anthropic claude-sonnet-5-5 in workspace scope on the sidecar runtime", o)
	}
	s := got.ObserveAgent.Last7Days
	if s.Runs != 2 || s.Failed != 1 || s.CostUSD == nil || *s.CostUSD != 0.75 || s.MedianQueueWaitSeconds == nil || *s.MedianQueueWaitSeconds != 6 {
		t.Fatalf("workspace stats = %+v, want 2 runs, 1 failed, 0.75 USD and a median wait of 6 seconds", s)
	}
	if len(got.ServiceAccounts) != 1 || got.ServiceAccounts[0].ID != account || got.ServiceAccounts[0].Name != "nightly" {
		t.Fatalf("service accounts = %+v, want nightly with its id", got.ServiceAccounts)
	}
	if st := got.ServiceAccounts[0].Last7Days; st == nil || st.Runs != 1 || *st.CostUSD != 1 {
		t.Fatalf("service account stats = %+v, want one run costing 1 USD", st)
	}
}

func TestMemberSeesOwnRunsOwnGrantsAndServiceAccountsWithoutStats(t *testing.T) {
	r := newRig(t, Observe{Enabled: true, Provider: "anthropic", Model: "claude-sonnet-5-5"})
	adaGrant := r.grant(t, ada, "claude-code")
	r.grant(t, bob, "cursor")
	account := r.serviceAccount(t, "nightly", true)
	r.serviceAccount(t, "not-a-worker", false)
	r.run(t, run{kind: "builtin", by: ada, state: "succeeded", cost: price(0.25)})
	r.run(t, run{kind: "builtin", by: bob, state: "succeeded", cost: price(0.5)})
	r.run(t, run{kind: "grant", assignedID: adaGrant, by: ada, state: "succeeded"})
	r.run(t, run{kind: "service_account", assignedID: account, by: bob, state: "succeeded"})

	got := r.overview(t, ada)
	if got.ObserveAgent.Scope != "you" || got.ObserveAgent.Last7Days.Runs != 1 || *got.ObserveAgent.Last7Days.CostUSD != 0.25 {
		t.Fatalf("observe agent = %+v, want scope you and only Ada's run", got.ObserveAgent)
	}
	if len(got.Connected) != 1 || got.Connected[0].GrantID != adaGrant || got.Connected[0].ClientName != "Claude Code" || got.Connected[0].Mode != "direct" || got.Connected[0].Last7Days.Runs != 1 {
		t.Fatalf("connected = %+v, want only Ada's grant with its id, client and one run", got.Connected)
	}
	if len(got.ServiceAccounts) != 1 || got.ServiceAccounts[0].ID != account || got.ServiceAccounts[0].Last7Days != nil {
		t.Fatalf("service accounts = %+v, want the run-taking account with null stats", got.ServiceAccounts)
	}
}

func TestRevokedAndExpiredGrantsAreAbsent(t *testing.T) {
	r := newRig(t, Observe{})
	revoked := r.grant(t, ada, "old")
	expired := r.grant(t, ada, "older")
	live := r.grant(t, ada, "live")
	r.db.Exec(t, `UPDATE oauth_grants SET revoked_at = now(), revoked_reason = 'person' WHERE id = $1::uuid`, revoked)
	r.db.Exec(t, `UPDATE oauth_grants SET expires_at = now() - interval '1 minute' WHERE id = $1::uuid`, expired)

	got := r.overview(t, ada)
	if len(got.Connected) != 1 || got.Connected[0].GrantID != live {
		t.Fatalf("connected = %+v, want only the live grant", got.Connected)
	}
	if o := got.ObserveAgent; o.Enabled || o.Provider != nil || o.Model != nil {
		t.Fatalf("observe agent = %+v, want disabled with null provider and model", o)
	}
}

func TestACostIsNullWhenAFinishedRunHasNoPrice(t *testing.T) {
	r := newRig(t, Observe{Enabled: true})
	r.run(t, run{kind: "builtin", by: ada, state: "succeeded", cost: price(0.25)})
	r.run(t, run{kind: "builtin", by: ada, state: "queued"})
	if s := r.overview(t, ada).ObserveAgent.Last7Days; s.Runs != 2 || s.CostUSD == nil || *s.CostUSD != 0.25 {
		t.Fatalf("stats with a priced and a queued run = %+v, want 0.25 USD", s)
	}

	r.run(t, run{kind: "builtin", by: ada, state: "failed"})
	if s := r.overview(t, ada).ObserveAgent.Last7Days; s.Runs != 3 || s.Failed != 1 || s.CostUSD != nil {
		t.Fatalf("stats with an unpriced finished run = %+v, want a null cost", s)
	}
}

func TestStatsCoverSevenDaysAndLeaveCanceledRunsOutOfFailed(t *testing.T) {
	r := newRig(t, Observe{Enabled: true})
	r.run(t, run{kind: "builtin", by: ada, state: "succeeded", cost: price(1), createdDaysAgo: 8})
	r.run(t, run{kind: "builtin", by: ada, state: "canceled", cost: price(0)})
	if s := r.overview(t, ada).ObserveAgent.Last7Days; s.Runs != 1 || s.Failed != 0 {
		t.Fatalf("stats = %+v, want the 8 day old run left out and the canceled run not failed", s)
	}
	none := r.overview(t, bob).ObserveAgent.Last7Days
	if none.Runs != 0 || none.MedianQueueWaitSeconds != nil {
		t.Fatalf("stats without runs = %+v, want zero runs and a null median", none)
	}
}

func TestWritesCountOnlyThatClientsMCPRowsOfTheCaller(t *testing.T) {
	r := newRig(t, Observe{})
	r.grant(t, ada, "claude-code")
	r.activity(t, ada, "mcp", "claude-code", 1)
	r.activity(t, ada, "mcp", "claude-code", 6)
	r.activity(t, ada, "mcp", "claude-code", 8)
	r.activity(t, ada, "mcp", "cursor", 1)
	r.activity(t, ada, "web", "", 1)
	r.activity(t, bob, "mcp", "claude-code", 1)

	got := r.overview(t, ada)
	if len(got.Connected) != 1 || got.Connected[0].Writes7d != 2 {
		t.Fatalf("connected = %+v, want 2 writes: two recent claude-code rows of Ada", got.Connected)
	}
}

func TestAnAnonymousClientIsNamedMCPAndCountsRowsWithoutAClient(t *testing.T) {
	r := newRig(t, Observe{})
	r.grant(t, ada, "")
	r.activity(t, ada, "mcp", "", 1)
	r.activity(t, ada, "mcp", "cursor", 1)

	got := r.overview(t, ada)
	if len(got.Connected) != 1 || got.Connected[0].ClientName != "MCP" || got.Connected[0].Writes7d != 1 {
		t.Fatalf("connected = %+v, want an MCP client with the one row that has no client", got.Connected)
	}
}

func TestPresenceFollowsTheAgentWorkerAndHeldRuns(t *testing.T) {
	r := newRig(t, Observe{})
	grant := r.grant(t, ada, "claude-code")
	account := r.serviceAccount(t, "nightly", true)
	presences := func() (string, string, *string) {
		got := r.overview(t, ada)
		return got.Connected[0].Presence, got.ServiceAccounts[0].Presence, got.Connected[0].LastSeenAt
	}
	if g, s, seen := presences(); g != "offline" || s != "offline" || seen != nil {
		t.Fatalf("fresh agents presence = %q, %q seen %v, want offline and never seen", g, s, seen)
	}

	r.db.Exec(t, `INSERT INTO agent_workers (id, principal_kind, principal_id, seen_at) VALUES ('grant:' || $1, 'grant', $1::uuid, now()), ('service_account:' || $2, 'service_account', $2::uuid, now() - interval '5 minutes')`, grant, account)
	if g, s, seen := presences(); g != "listening" || s != "offline" || seen == nil {
		t.Fatalf("after the grant asked for work presence = %q, %q seen %v, want listening and offline", g, s, seen)
	}

	r.db.Exec(t, `INSERT INTO agent_requests (source, prompt, requested_by_kind, requested_by_id, requested_by_name, status, state, lease_until, assigned_kind, assigned_id, claimed_by)
		VALUES ('observe', 'p', 'user', $1::uuid, 'Ada', 'running', 'running', now() + interval '1 minute', 'service_account', $2::uuid, 'service_account:' || $2)`, ada.ID, account)
	if _, s, _ := presences(); s != "working" {
		t.Fatalf("a service account holding a run has presence %q, want working", s)
	}
}

func TestKeysAreRefusedWithUserSessionRequired(t *testing.T) {
	r := newRig(t, Observe{})
	rec := r.get(bot)
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusForbidden || body.Code != "user_session_required" {
		t.Fatalf("a key got %d %s, want 403 user_session_required", rec.Code, rec.Body.String())
	}
}

func TestTheResponseNamesOnlyDocumentedFields(t *testing.T) {
	r := newRig(t, Observe{Enabled: true, Provider: "anthropic", Model: "m"})
	r.grant(t, ada, "claude-code")
	r.serviceAccount(t, "nightly", true)
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(r.get(root).Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	if want := []string{"connected", "observe_agent", "service_accounts"}; !slices.Equal(keys, want) {
		t.Fatalf("top level keys = %v, want %v", keys, want)
	}
}

func (r *rig) validate(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	openapi3.DefineStringFormatValidator("uuid", openapi3.NewRegexpFormatValidator(openapi3.FormatOfStringForUUIDOfRFC9562))
	doc, err := api.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	router, err := legacy.NewRouter(doc)
	if err != nil {
		t.Fatal(err)
	}
	check := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	route, params, err := router.FindRoute(check)
	if err != nil {
		t.Fatalf("GET /agents is not documented: %v", err)
	}
	options := &openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc, IncludeResponseStatus: true, SkipSettingDefaults: true}
	input := &openapi3filter.RequestValidationInput{Request: check, PathParams: params, Route: route, Options: options}
	response := &openapi3filter.ResponseValidationInput{RequestValidationInput: input, Status: rec.Code, Header: rec.Header(), Options: options}
	response.SetBodyBytes(rec.Body.Bytes())
	if err := openapi3filter.ValidateResponse(t.Context(), response); err != nil {
		t.Fatalf("GET /agents answered %d against the document: %v\n%s", rec.Code, err, rec.Body)
	}
}

func TestTheResponsesMatchTheDocument(t *testing.T) {
	r := newRig(t, Observe{Enabled: true, Provider: "anthropic", Model: "claude-sonnet-5-5", Runtime: "sidecar"})
	grant := r.grant(t, root, "claude-code")
	account := r.serviceAccount(t, "nightly", true)
	r.run(t, run{kind: "builtin", by: root, state: "succeeded", cost: price(0.5), waitSeconds: 3})
	r.run(t, run{kind: "grant", assignedID: grant, by: root, state: "failed"})
	r.run(t, run{kind: "service_account", assignedID: account, by: root, state: "succeeded", cost: price(1)})
	for _, by := range []auth.Principal{root, ada, bot} {
		r.validate(t, r.get(by))
	}
}
