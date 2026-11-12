package revisions_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/api"
	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

type documentedObserve struct {
	engine *gin.Engine
	router routers.Router
}

func newDocumentedObserve(t *testing.T, svc *revisions.Service, who auth.Principal, finding func(context.Context, pgtype.UUID) (json.RawMessage, bool, error)) *documentedObserve {
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
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	opts := revisions.ObserveOptions{
		ObservationURL: func(id string) string { return "/o/" + id },
		Finding:        finding,
	}
	revisions.NewObserveHandler(svc, opts).Register(auth.NewRoutes(engine.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), withRole(who)))
	})))
	return &documentedObserve{engine: engine, router: router}
}

func (d *documentedObserve) call(t *testing.T, method, path, body string, want int) *httptest.ResponseRecorder {
	t.Helper()
	rec := call(d.engine, method, path, body)
	if rec.Code != want {
		t.Fatalf("%s %s answered %d, want %d: %s", method, path, rec.Code, want, rec.Body)
	}
	check := httptest.NewRequest(method, path, strings.NewReader(body))
	check.Header.Set("Content-Type", "application/json")
	route, params, err := d.router.FindRoute(check)
	if err != nil {
		t.Fatalf("%s %s is not documented: %v", method, path, err)
	}
	options := &openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc, IncludeResponseStatus: true, SkipSettingDefaults: true}
	input := &openapi3filter.RequestValidationInput{Request: check, PathParams: params, Route: route, Options: options}
	if err := openapi3filter.ValidateRequest(t.Context(), input); err != nil && rec.Code < 400 {
		t.Fatalf("%s %s breaks the document but answered %d: %v", method, path, rec.Code, err)
	}
	check.Body = io.NopCloser(bytes.NewReader(nil))
	response := &openapi3filter.ResponseValidationInput{RequestValidationInput: input, Status: rec.Code, Header: rec.Header(), Options: options}
	response.SetBodyBytes(rec.Body.Bytes())
	if err := openapi3filter.ValidateResponse(t.Context(), response); err != nil {
		t.Fatalf("%s %s answered %d against the document: %v\n%s", method, path, rec.Code, err, rec.Body)
	}
	return rec
}

func requestIDOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Request struct {
			ID string `json:"id"`
		} `json:"request"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Request.ID == "" {
		t.Fatalf("no request in %s", rec.Body)
	}
	return body.Request.ID
}

func TestAssignedRunsMatchTheDocument(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	h := newDocumentedObserve(t, svc, user, nil)
	var creator, account string
	sqlDB := d.SQLDB(t)
	if err := sqlDB.QueryRow(`INSERT INTO users (oidc_issuer, oidc_subject, email) VALUES ('i', 's', 'a@example.com') RETURNING id`).Scan(&creator); err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.QueryRow(`INSERT INTO service_accounts (name, description, created_by, role) VALUES ('codex-bot', '', $1, 'editor') RETURNING id`, creator).Scan(&account); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name     string
		kind     string
		assignee string
		want     string
	}{
		{"a service account with a name", "service_account", account, `{"kind":"service_account","id":"` + account + `","name":"codex-bot"}`},
		{"a grant that no longer exists", "grant", grantA, `{"kind":"grant","id":"` + grantA + `","name":null}`},
	}
	for _, c := range cases {
		id := requestIDOf(t, h.call(t, http.MethodPost, "/api/v1/observe", `{"prompt":"checkout health"}`, http.StatusAccepted))
		assignTo(t, d, id, c.kind, c.assignee)
		var body struct {
			Request struct {
				AssignedTo json.RawMessage `json:"assigned_to"`
			} `json:"request"`
		}
		if err := json.Unmarshal(h.call(t, http.MethodGet, "/api/v1/observe/"+id, "", http.StatusOK).Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if string(body.Request.AssignedTo) != c.want {
			t.Errorf("%s: assigned_to = %s, want %s", c.name, body.Request.AssignedTo, c.want)
		}
	}
	own := requestIDOf(t, h.call(t, http.MethodPost, "/api/v1/observe", `{"prompt":"built in"}`, http.StatusAccepted))
	if body := h.call(t, http.MethodGet, "/api/v1/observe/"+own, "", http.StatusOK).Body.String(); !strings.Contains(body, `"assigned_to":null`) {
		t.Errorf("a built-in run does not show assigned_to null: %s", body)
	}
}

func TestObserveListFieldsMatchTheDocument(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	h := newDocumentedObserve(t, svc, user, nil)

	build := requestIDOf(t, h.call(t, http.MethodPost, "/api/v1/observe", `{"prompt":"checkout health"}`, http.StatusAccepted))
	investigation := requestIDOf(t, h.call(t, http.MethodPost, "/api/v1/observe", `{"prompt":"why are errors up","kind":"investigate","dashboard_id":"`+dash.String()+`","from":"now-2h","to":"now"}`, http.StatusAccepted))
	h.call(t, http.MethodPost, "/api/v1/observe", `{"prompt":"p","kind":"explain"}`, http.StatusBadRequest)

	h.call(t, http.MethodPatch, "/api/v1/observe/"+build, `{"title":"Checkout","pinned":true}`, http.StatusOK)
	h.call(t, http.MethodPatch, "/api/v1/observe/"+build, `{"title":null}`, http.StatusOK)
	h.call(t, http.MethodPatch, "/api/v1/observe/"+build, `{}`, http.StatusBadRequest)
	newDocumentedObserve(t, svc, account, nil).call(t, http.MethodPatch, "/api/v1/observe/"+investigation, `{"pinned":true}`, http.StatusForbidden)

	for _, query := range []string{"", "?q=checkout", "?pinned=true", "?source=investigate", "?dashboard_id=" + dash.String(), "?limit=1"} {
		h.call(t, http.MethodGet, "/api/v1/observe"+query, "", http.StatusOK)
	}
	h.call(t, http.MethodGet, "/api/v1/observe?pinned=maybe", "", http.StatusBadRequest)

	h.call(t, http.MethodDelete, "/api/v1/observe/"+build, "", http.StatusNoContent)
	h.call(t, http.MethodGet, "/api/v1/observe/"+build, "", http.StatusOK)
	h.call(t, http.MethodGet, "/api/v1/observe/"+investigation, "", http.StatusOK)
}

func TestEveryRequestIsStartedByAPerson(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	h := newDocumentedObserve(t, svc, user, nil)

	created := requestIDOf(t, h.call(t, http.MethodPost, "/api/v1/observe", `{"prompt":"checkout health"}`, http.StatusAccepted))
	revision := requestIDOf(t, h.call(t, http.MethodPost, "/api/v1/observe", `{"prompt":"add a panel","dashboard_id":"`+dash.String()+`"}`, http.StatusAccepted))

	var triggers struct {
		Request struct {
			Trigger string `json:"trigger"`
		} `json:"request"`
	}
	for _, id := range []string{created, revision} {
		triggers.Request.Trigger = ""
		rec := h.call(t, http.MethodGet, "/api/v1/observe/"+id, "", http.StatusOK)
		if err := json.Unmarshal(rec.Body.Bytes(), &triggers); err != nil || triggers.Request.Trigger != "person" {
			t.Fatalf("request %s trigger %q, err %v: %s", id, triggers.Request.Trigger, err, rec.Body)
		}
	}

	list := h.call(t, http.MethodGet, "/api/v1/observe", "", http.StatusOK)
	var listed struct {
		Requests []struct {
			Trigger string `json:"trigger"`
		} `json:"requests"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listed); err != nil || len(listed.Requests) != 2 {
		t.Fatalf("list %v: %s", err, list.Body)
	}
	for _, row := range listed.Requests {
		if row.Trigger != "person" {
			t.Errorf("listed trigger %q, want person", row.Trigger)
		}
	}
}

func TestAFinishedRequestReportsItsCostAsTheDocumentSays(t *testing.T) {
	cost := 0.0123
	svc, d := newService(t, reviserFunc(func(_ context.Context, req revisions.Request) (revisions.Result, error) {
		usage := &revisions.Usage{Provider: "p", Model: "m", ModelCalls: 1}
		if strings.Contains(req.Prompt, "priced") {
			usage.CostUSD = &cost
		}
		return revisions.Result{Version: 2, Usage: usage}, nil
	}))
	dash := seedDashboard(t, d, 1)
	h := newDocumentedObserve(t, svc, user, nil)
	priced := requestIDOf(t, h.call(t, http.MethodPost, "/api/v1/observe", `{"prompt":"priced run","dashboard_id":"`+dash.String()+`"}`, http.StatusAccepted))
	unpriced := requestIDOf(t, h.call(t, http.MethodPost, "/api/v1/observe", `{"prompt":"other run","dashboard_id":"`+dash.String()+`"}`, http.StatusAccepted))
	startWorker(t, svc, d, revisions.WorkerOptions{Workers: 1})

	for id, want := range map[string]*float64{priced: &cost, unpriced: nil} {
		waitFor(t, "request "+id+" to finish", func() bool {
			row, err := svc.GetByID(context.Background(), mustUUID(t, id))
			return err == nil && row.Status == string(revisions.StatusSucceeded)
		})
		var body struct {
			Request struct {
				Usage struct {
					CostUSD *float64 `json:"cost_usd"`
				} `json:"usage"`
			} `json:"request"`
		}
		rec := h.call(t, http.MethodGet, "/api/v1/observe/"+id, "", http.StatusOK)
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		got := body.Request.Usage.CostUSD
		if (got == nil) != (want == nil) || got != nil && *got != *want {
			t.Errorf("request %s usage.cost_usd = %v, want %v", id, got, want)
		}
	}
}

func storedFinding(t *testing.T, d *dbtest.Database, row dbsqlc.AgentRequest) (string, func(context.Context, pgtype.UUID) (json.RawMessage, bool, error)) {
	t.Helper()
	insertFindingFor(t, d, row.ID)
	d.Exec(t, "UPDATE agent_requests SET state = 'succeeded', status = 'succeeded', attempts = 1, started_at = now(), finished_at = now() WHERE id = $1", row.ID)
	var findingID, observationID string
	if err := d.SQLDB(t).QueryRowContext(t.Context(), "SELECT id::text, observation_id::text FROM findings WHERE agent_request_id = $1", row.ID).Scan(&findingID, &observationID); err != nil {
		t.Fatal(err)
	}
	finding := func(_ context.Context, requestID pgtype.UUID) (json.RawMessage, bool, error) {
		if requestID != row.ID {
			return nil, false, nil
		}
		now := time.Now()
		return mustMarshal(t, findings.JSONOf(findings.Finding{
			ID:             findingID,
			ObservationID:  observationID,
			AgentRequestID: row.ID.String(),
			Title:          "why",
			Markdown:       "checkout-0 errors",
			From:           now.Add(-time.Hour),
			To:             now,
			CreatedAt:      now,
			CreatedBy:      findings.Author{Kind: "user", ID: userID, Name: "Ada"},
			Actor:          actor.Actor{Kind: auth.KindUser, ID: userID, Name: "Ada"},
		}, "/o/"+observationID)), true, nil
	}
	return findingID, finding
}

type requestWithFinding struct {
	Request struct {
		Kind          string  `json:"kind"`
		URL           string  `json:"url"`
		DashboardID   *string `json:"dashboard_id"`
		ResultVersion *int    `json:"result_version"`
		Finding       *struct {
			ID string `json:"id"`
		} `json:"finding"`
	} `json:"request"`
}

func TestASucceededInvestigationShowsItsFindingAsTheDocumentSays(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	row := investigate(t, svc, revisions.InvestigateParams{})
	findingID, finding := storedFinding(t, d, row)
	h := newDocumentedObserve(t, svc, user, finding)

	rec := h.call(t, http.MethodGet, "/api/v1/observe/"+row.ID.String(), "", http.StatusOK)
	var got struct {
		Request struct {
			Kind    string `json:"kind"`
			URL     string `json:"url"`
			Finding *struct {
				ID string `json:"id"`
			} `json:"finding"`
		} `json:"request"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Request.Kind != "investigate" || got.Request.Finding == nil || got.Request.Finding.ID != findingID || got.Request.URL != "/o/"+findingID {
		t.Fatalf("GET of a succeeded investigation: %s", rec.Body)
	}

	list := h.call(t, http.MethodGet, "/api/v1/observe?source=investigate&q="+url.QueryEscape("errors"), "", http.StatusOK)
	if !strings.Contains(list.Body.String(), `"url":"/o/`+findingID+`"`) {
		t.Fatalf("the list does not link the finding: %s", list.Body)
	}
}

func TestAnAnsweredObserveRequestShowsItsFindingAsTheDocumentSays(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	row, err := svc.SubmitObserve(context.Background(), user, "what all logs are available?")
	if err != nil {
		t.Fatal(err)
	}
	findingID, finding := storedFinding(t, d, row)
	h := newDocumentedObserve(t, svc, user, finding)

	rec := h.call(t, http.MethodGet, "/api/v1/observe/"+row.ID.String(), "", http.StatusOK)
	var got requestWithFinding
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	r := got.Request
	if r.Kind != "build" || r.Finding == nil || r.Finding.ID != findingID || r.URL != "/o/"+findingID || r.DashboardID != nil || r.ResultVersion != nil {
		t.Fatalf("GET of an answered observe request: %s", rec.Body)
	}
}

func TestAnAnsweredDashboardPromptShowsItsFindingAsTheDocumentSays(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	row, err := svc.SubmitFollowUp(context.Background(), dash, nil, "", "what does this dashboard show?", user)
	if err != nil {
		t.Fatal(err)
	}
	findingID, finding := storedFinding(t, d, row)
	h := newDocumentedObserve(t, svc, user, finding)

	rec := h.call(t, http.MethodGet, "/api/v1/observe/"+row.ID.String(), "", http.StatusOK)
	var got requestWithFinding
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	r := got.Request
	if r.Finding == nil || r.Finding.ID != findingID || r.URL != "/o/"+findingID || r.DashboardID == nil || *r.DashboardID != dash.String() || r.ResultVersion != nil {
		t.Fatalf("GET of an answered dashboard prompt: %s", rec.Body)
	}
}

func mustMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestRetryAndTheRunStateFieldsMatchTheDocument(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	h := newDocumentedObserve(t, svc, user, nil)

	build := requestIDOf(t, h.call(t, http.MethodPost, "/api/v1/observe", `{"prompt":"checkout health"}`, http.StatusAccepted))
	revision := requestIDOf(t, h.call(t, http.MethodPost, "/api/v1/observe", `{"prompt":"add a panel","dashboard_id":"`+dash.String()+`"}`, http.StatusAccepted))

	queued := h.call(t, http.MethodGet, "/api/v1/observe/"+build, "", http.StatusOK)
	var shown struct {
		Request struct {
			State          string   `json:"state"`
			ResultKind     string   `json:"result_kind"`
			AllowedActions []string `json:"allowed_actions"`
			Attempt        int      `json:"attempt"`
		} `json:"request"`
	}
	if err := json.Unmarshal(queued.Body.Bytes(), &shown); err != nil || shown.Request.State != "queued" || shown.Request.ResultKind != "none" || shown.Request.Attempt != 0 {
		t.Fatalf("queued request %v: %s", err, queued.Body)
	}
	if got := strings.Join(shown.Request.AllowedActions, ","); got != "cancel,pin,archive" {
		t.Fatalf("queued allowed_actions = %s", got)
	}

	refused := h.call(t, http.MethodPost, "/api/v1/observe/"+build+"/retry", "", http.StatusConflict)
	var refusal struct {
		Code, State string
		Allowed     []string `json:"allowed_actions"`
	}
	if err := json.Unmarshal(refused.Body.Bytes(), &refusal); err != nil || refusal.Code != "invalid_transition" || refusal.State != "queued" || len(refusal.Allowed) == 0 {
		t.Fatalf("refusal %v: %s", err, refused.Body)
	}

	h.call(t, http.MethodPost, "/api/v1/observe/"+build+"/cancel", "", http.StatusOK)
	h.call(t, http.MethodPost, "/api/v1/observe/"+revision+"/cancel", "", http.StatusOK)
	retried := h.call(t, http.MethodPost, "/api/v1/observe/"+build+"/retry", "", http.StatusAccepted)
	if next := requestIDOf(t, retried); next == build || retried.Header().Get("Location") == "" {
		t.Fatalf("retry answered %s with location %q", retried.Body, retried.Header().Get("Location"))
	}
	h.call(t, http.MethodPost, "/api/v1/observe/"+revision+"/retry", "", http.StatusAccepted)

	canceled := h.call(t, http.MethodGet, "/api/v1/observe/"+build, "", http.StatusOK)
	if err := json.Unmarshal(canceled.Body.Bytes(), &shown); err != nil || shown.Request.State != "canceled" {
		t.Fatalf("canceled request %v: %s", err, canceled.Body)
	}
	h.call(t, http.MethodGet, "/api/v1/observe", "", http.StatusOK)
	h.call(t, http.MethodPost, "/api/v1/observe/"+uuidNobodyOwns+"/retry", "", http.StatusNotFound)
}

const uuidNobodyOwns = "99999999-9999-4999-8999-999999999999"
