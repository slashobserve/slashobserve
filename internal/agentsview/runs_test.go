package agentsview

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"

	"github.com/slashobserve/slashobserve/api"
	"github.com/slashobserve/slashobserve/internal/auth"
)

type runItem struct {
	ID          string   `json:"id"`
	Source      string   `json:"source"`
	Title       string   `json:"title"`
	State       string   `json:"state"`
	ResultKind  string   `json:"result_kind"`
	FailureCode *string  `json:"failure_code"`
	CostUSD     *float64 `json:"cost_usd"`
	RequestedBy struct {
		Kind string `json:"kind"`
		Name string `json:"name"`
	} `json:"requested_by"`
	Openable  bool `json:"openable"`
	Dashboard *struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	} `json:"dashboard"`
	Observation *struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	} `json:"observation"`
}

type runPage struct {
	Scope      string    `json:"scope"`
	Items      []runItem `json:"items"`
	NextCursor *string   `json:"next_cursor"`
}

type seededRun struct {
	kind, assignedID string
	by               auth.Principal
	source           string
	title            string
	prompt           string
	dashboardID      string
	minutesAgo       int
}

func (r *rig) seed(t *testing.T, in seededRun) string {
	t.Helper()
	source := in.source
	if source == "" {
		source = "observe"
	}
	prompt := in.prompt
	if prompt == "" {
		prompt = "p"
	}
	var id string
	err := r.db.SQLDB(t).QueryRowContext(t.Context(), `INSERT INTO agent_requests (source, title, prompt, requested_by_kind, requested_by_id, requested_by_name, status, state, assigned_kind, assigned_id, dashboard_id, base_version, created_at)
		VALUES ($1, nullif($2, ''), $3, 'user', $4::uuid, $5, 'queued', 'queued', $6, nullif($7, '')::uuid, nullif($8, '')::uuid, CASE WHEN $8 <> '' THEN 1 END, now() - make_interval(mins => $9))
		RETURNING id::text`,
		source, in.title, prompt, in.by.ID, in.by.Name, in.kind, in.assignedID, in.dashboardID, in.minutesAgo).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (r *rig) getRuns(by auth.Principal, query url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/runs?"+query.Encode(), nil)
	req.Header.Set("X-Test-Principal", by.ID)
	rec := httptest.NewRecorder()
	r.engine.ServeHTTP(rec, req)
	return rec
}

func (r *rig) runs(t *testing.T, by auth.Principal, query url.Values) runPage {
	t.Helper()
	rec := r.getRuns(by, query)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /agents/runs?%s as %s = %d: %s", query.Encode(), by.Name, rec.Code, rec.Body.String())
	}
	var out runPage
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func ids(page runPage) []string {
	out := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		out = append(out, item.ID)
	}
	return out
}

func kindQuery(kind, id string) url.Values {
	q := url.Values{"kind": {kind}}
	if id != "" {
		q.Set("id", id)
	}
	return q
}

func TestRunTitlePrefersTheTitleThenThePromptThenTheSource(t *testing.T) {
	long := strings.Repeat("é", 130)
	tests := []struct {
		name, title, prompt, source, want string
	}{
		{"title wins", "Checkout latency", "something else", "prompt", "Checkout latency"},
		{"first prompt line", "", "Why is checkout slow?\nSecond line", "prompt", "Why is checkout slow?"},
		{"leading blank lines are skipped by trimming", "", "\n  \n  Hello  \nworld", "prompt", "Hello"},
		{"cut to 120 runes", "", long, "prompt", strings.Repeat("é", 120)},
		{"blank prompt falls back to the source", "", " \n ", "investigate", "investigate"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := runTitle(tc.title, tc.prompt, tc.source); got != tc.want {
				t.Fatalf("runTitle(%q, %q, %q) = %q, want %q", tc.title, tc.prompt, tc.source, got, tc.want)
			}
		})
	}
}

func TestBuiltInRunsAreScopedToTheRequesterUnlessAdmin(t *testing.T) {
	r := newRig(t, Observe{Enabled: true})
	adaRun := r.seed(t, seededRun{kind: "builtin", by: ada, minutesAgo: 2})
	bobRun := r.seed(t, seededRun{kind: "builtin", by: bob, minutesAgo: 1})

	mine := r.runs(t, ada, kindQuery("builtin", ""))
	if mine.Scope != "you" || !slices.Equal(ids(mine), []string{adaRun}) || !mine.Items[0].Openable {
		t.Fatalf("Ada's page = %+v, want only her own openable run with scope you", mine)
	}
	all := r.runs(t, root, kindQuery("builtin", ""))
	if all.Scope != "workspace" || !slices.Equal(ids(all), []string{bobRun, adaRun}) {
		t.Fatalf("Root's page = %+v, want Bob's then Ada's run with scope workspace", all)
	}
	for _, item := range all.Items {
		if item.Openable {
			t.Fatalf("Root can open %s, which Root did not request", item.ID)
		}
	}
	if all.Items[0].RequestedBy.Name != "Bob" || all.Items[0].RequestedBy.Kind != "user" {
		t.Fatalf("requested_by = %+v, want user Bob", all.Items[0].RequestedBy)
	}
}

func TestAGrantListsItsRunsOnlyForItsOwner(t *testing.T) {
	r := newRig(t, Observe{})
	grant := r.grant(t, ada, "claude-code")
	run := r.seed(t, seededRun{kind: "grant", assignedID: grant, by: ada})
	r.seed(t, seededRun{kind: "builtin", by: ada})
	r.db.Exec(t, `UPDATE oauth_grants SET revoked_at = now(), revoked_reason = 'person' WHERE id = $1::uuid`, grant)

	if got := r.runs(t, ada, kindQuery("grant", grant)); got.Scope != "you" || !slices.Equal(ids(got), []string{run}) {
		t.Fatalf("Ada's revoked grant page = %+v, want its one run with scope you", got)
	}
	for _, by := range []auth.Principal{bob, root} {
		if rec := r.getRuns(by, kindQuery("grant", grant)); rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"not_found"`) {
			t.Fatalf("%s asking for Ada's grant got %d %s, want 404 not_found", by.Name, rec.Code, rec.Body.String())
		}
	}
}

func TestServiceAccountRunsFollowTheRequesterUnlessAdmin(t *testing.T) {
	r := newRig(t, Observe{})
	account := r.serviceAccount(t, "nightly", true)
	adaRun := r.seed(t, seededRun{kind: "service_account", assignedID: account, by: ada, minutesAgo: 2})
	bobRun := r.seed(t, seededRun{kind: "service_account", assignedID: account, by: bob, minutesAgo: 1})
	other := r.serviceAccount(t, "other", true)
	r.seed(t, seededRun{kind: "service_account", assignedID: other, by: ada})

	if got := r.runs(t, root, kindQuery("service_account", account)); got.Scope != "workspace" || !slices.Equal(ids(got), []string{bobRun, adaRun}) {
		t.Fatalf("Root's page = %+v, want both runs of the account with scope workspace", got)
	}
	if got := r.runs(t, ada, kindQuery("service_account", account)); got.Scope != "you" || !slices.Equal(ids(got), []string{adaRun}) {
		t.Fatalf("Ada's page = %+v, want only her run with scope you", got)
	}
	if rec := r.getRuns(root, kindQuery("service_account", "99999999-9999-4999-8999-999999999999")); rec.Code != http.StatusNotFound {
		t.Fatalf("an unknown account got %d %s, want 404", rec.Code, rec.Body.String())
	}
}

func TestADashboardInAFolderTheCallerCannotSeeComesBackNull(t *testing.T) {
	r := newRig(t, Observe{})
	var secret, hiddenDashboard, openDashboard string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `INSERT INTO folders (title) VALUES ('Secret') RETURNING id::text`).Scan(&secret); err != nil {
		t.Fatal(err)
	}
	r.db.Exec(t, `INSERT INTO folder_permissions (folder_id, group_name, level) VALUES ($1::uuid, 'secret-team', 'view')`, secret)
	for dest, args := range map[*string][2]string{&hiddenDashboard: {"Payroll", secret}, &openDashboard: {"Checkout", ""}} {
		if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `INSERT INTO dashboards (title, latest_version, folder_id, created_by_kind, created_by_id, created_by_name, created_via)
			VALUES ($1, 1, coalesce(nullif($2, '')::uuid, general_folder_id()), 'user', gen_random_uuid(), 'Seed', 'direct') RETURNING id::text`, args[0], args[1]).Scan(dest); err != nil {
			t.Fatal(err)
		}
	}
	hidden := r.seed(t, seededRun{kind: "builtin", by: ada, source: "prompt", dashboardID: hiddenDashboard, minutesAgo: 2})
	open := r.seed(t, seededRun{kind: "builtin", by: ada, source: "prompt", dashboardID: openDashboard, minutesAgo: 1})

	page := r.runs(t, ada, kindQuery("builtin", ""))
	if !slices.Equal(ids(page), []string{open, hidden}) {
		t.Fatalf("ids = %v, want both runs", ids(page))
	}
	if d := page.Items[0].Dashboard; d == nil || d.ID != openDashboard || d.Title != "Checkout" {
		t.Fatalf("visible dashboard = %+v, want Checkout", d)
	}
	if page.Items[1].Dashboard != nil {
		t.Fatalf("hidden dashboard = %+v, want null", page.Items[1].Dashboard)
	}
	if strings.Contains(r.getRuns(ada, kindQuery("builtin", "")).Body.String(), hiddenDashboard) {
		t.Fatal("the response names the hidden dashboard's id")
	}
}

func TestSuggestionRunsNeverAppear(t *testing.T) {
	r := newRig(t, Observe{})
	r.db.Exec(t, `INSERT INTO dashboards (id, title, latest_version, created_by_kind, created_by_id, created_by_name, created_via)
		VALUES ('55555555-5555-4555-8555-555555555555', 'Checkout', 1, 'user', gen_random_uuid(), 'Seed', 'direct')`)
	r.db.Exec(t, `INSERT INTO agent_requests (source, prompt, requested_by_kind, requested_by_id, requested_by_name, status, state, assigned_kind, dashboard_id, base_version, panel_id)
		VALUES ('suggest', 'p', 'user', $1::uuid, 'Ada', 'queued', 'queued', 'builtin', '55555555-5555-4555-8555-555555555555', 1, 'p1')`, ada.ID)
	kept := r.seed(t, seededRun{kind: "builtin", by: ada})

	if got := r.runs(t, ada, kindQuery("builtin", "")); !slices.Equal(ids(got), []string{kept}) {
		t.Fatalf("ids = %v, want only the non-suggest run %s", ids(got), kept)
	}
}

func TestRunsPageWithACursor(t *testing.T) {
	r := newRig(t, Observe{})
	newest := r.seed(t, seededRun{kind: "builtin", by: ada, minutesAgo: 1})
	middle := r.seed(t, seededRun{kind: "builtin", by: ada, minutesAgo: 2})
	oldest := r.seed(t, seededRun{kind: "builtin", by: ada, minutesAgo: 3})

	query := kindQuery("builtin", "")
	query.Set("limit", "2")
	first := r.runs(t, ada, query)
	if !slices.Equal(ids(first), []string{newest, middle}) || first.NextCursor == nil {
		t.Fatalf("first page = %+v, want two runs and a cursor", first)
	}
	query.Set("cursor", *first.NextCursor)
	second := r.runs(t, ada, query)
	if !slices.Equal(ids(second), []string{oldest}) || second.NextCursor != nil {
		t.Fatalf("second page = %+v, want the oldest run and no cursor", second)
	}
}

func TestInvalidRunQueriesAreRefused(t *testing.T) {
	r := newRig(t, Observe{})
	const someID = "99999999-9999-4999-8999-999999999999"
	tests := []struct {
		name  string
		query url.Values
	}{
		{"no kind", url.Values{}},
		{"unknown kind", kindQuery("other", "")},
		{"builtin with an id", kindQuery("builtin", someID)},
		{"grant without an id", kindQuery("grant", "")},
		{"service account without an id", kindQuery("service_account", "")},
		{"id that is not a uuid", kindQuery("grant", "nope")},
		{"limit zero", url.Values{"kind": {"builtin"}, "limit": {"0"}}},
		{"limit above the ceiling", url.Values{"kind": {"builtin"}, "limit": {"51"}}},
		{"cursor that does not decode", url.Values{"kind": {"builtin"}, "cursor": {"!!"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := r.getRuns(ada, tc.query)
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"invalid_request"`) {
				t.Fatalf("got %d %s, want 400 invalid_request", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestKeysCannotListRuns(t *testing.T) {
	r := newRig(t, Observe{})
	rec := r.getRuns(bot, kindQuery("builtin", ""))
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"user_session_required"`) {
		t.Fatalf("a key got %d %s, want 403 user_session_required", rec.Code, rec.Body.String())
	}
}

func TestTheRunResponsesMatchTheDocument(t *testing.T) {
	r := newRig(t, Observe{})
	openapi3.DefineStringFormatValidator("uuid", openapi3.NewRegexpFormatValidator(openapi3.FormatOfStringForUUIDOfRFC9562))
	doc, err := api.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	router, err := legacy.NewRouter(doc)
	if err != nil {
		t.Fatal(err)
	}
	grant := r.grant(t, ada, "claude-code")
	r.seed(t, seededRun{kind: "builtin", by: ada, title: "Titled", minutesAgo: 2})
	r.seed(t, seededRun{kind: "grant", assignedID: grant, by: ada})
	r.db.Exec(t, `UPDATE agent_requests SET run_usage = '{"cost_usd": 0.5}', started_at = created_at, failure_code = 'internal', status = 'failed', state = 'failed' WHERE title = 'Titled'`)

	cases := []struct {
		by    auth.Principal
		query url.Values
	}{
		{ada, kindQuery("builtin", "")},
		{root, kindQuery("builtin", "")},
		{ada, kindQuery("grant", grant)},
		{bob, kindQuery("grant", grant)},
		{ada, kindQuery("grant", "")},
		{bot, kindQuery("builtin", "")},
	}
	for _, tc := range cases {
		rec := r.getRuns(tc.by, tc.query)
		check := httptest.NewRequest(http.MethodGet, "/api/v1/agents/runs?"+tc.query.Encode(), nil)
		route, params, err := router.FindRoute(check)
		if err != nil {
			t.Fatalf("GET /agents/runs is not documented: %v", err)
		}
		options := &openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc, IncludeResponseStatus: true, SkipSettingDefaults: true}
		input := &openapi3filter.RequestValidationInput{Request: check, PathParams: params, Route: route, Options: options}
		response := &openapi3filter.ResponseValidationInput{RequestValidationInput: input, Status: rec.Code, Header: rec.Header(), Options: options}
		response.SetBodyBytes(rec.Body.Bytes())
		if err := openapi3filter.ValidateResponse(t.Context(), response); err != nil {
			t.Fatal(fmt.Errorf("%s %s answered %d against the document: %w\n%s", tc.by.Name, tc.query.Encode(), rec.Code, err, rec.Body))
		}
	}
}
