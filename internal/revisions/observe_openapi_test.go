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
	"github.com/slashobserve/slashobserve/internal/auth"
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
		FindingURL: func(id string) string { return "/findings/" + id },
		Finding:    finding,
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

func TestASucceededInvestigationShowsItsFindingAsTheDocumentSays(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	row := investigate(t, svc, revisions.InvestigateParams{})
	insertFindingFor(t, d, row.ID)
	d.Exec(t, "UPDATE agent_requests SET status = 'succeeded', attempts = 1, started_at = now(), finished_at = now() WHERE id = $1", row.ID)
	var findingID string
	if err := d.SQLDB(t).QueryRowContext(t.Context(), "SELECT id::text FROM findings WHERE agent_request_id = $1", row.ID).Scan(&findingID); err != nil {
		t.Fatal(err)
	}
	finding := func(_ context.Context, requestID pgtype.UUID) (json.RawMessage, bool, error) {
		if requestID != row.ID {
			return nil, false, nil
		}
		now := time.Now()
		return mustMarshal(t, findings.JSONOf(findings.Finding{
			ID:             findingID,
			AgentRequestID: row.ID.String(),
			Title:          "why",
			Markdown:       "checkout-0 errors",
			From:           now.Add(-time.Hour),
			To:             now,
			CreatedAt:      now,
			CreatedBy:      findings.Author{Kind: "user", ID: userID, Name: "Ada"},
		}, "/findings/"+findingID)), true, nil
	}
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
	if got.Request.Kind != "investigate" || got.Request.Finding == nil || got.Request.Finding.ID != findingID || got.Request.URL != "/findings/"+findingID {
		t.Fatalf("GET of a succeeded investigation: %s", rec.Body)
	}

	list := h.call(t, http.MethodGet, "/api/v1/observe?source=investigate&q="+url.QueryEscape("errors"), "", http.StatusOK)
	if !strings.Contains(list.Body.String(), `"url":"/findings/`+findingID+`"`) {
		t.Fatalf("the list does not link the finding: %s", list.Body)
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
