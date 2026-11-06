package repairs_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/api"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/repairs"
)

type httpRig struct {
	*rig
	engine *gin.Engine
	router routers.Router
	caller auth.Principal
}

func newHTTPRig(t *testing.T, base *rig, svc *repairs.Service) *httpRig {
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
	h := &httpRig{rig: base, router: router, caller: base.editor}
	gin.SetMode(gin.TestMode)
	h.engine = gin.New()
	repairs.NewHandler(svc).Register(auth.NewRoutes(h.engine.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), h.caller))
	})))
	return h
}

func (h *httpRig) call(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.engine.ServeHTTP(rec, req)

	check := httptest.NewRequest(method, path, strings.NewReader(body))
	check.Header = req.Header.Clone()
	route, params, err := h.router.FindRoute(check)
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

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %s: %v", rec.Body, err)
	}
	return out
}

func expect(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	got := decode[struct {
		Code string `json:"code"`
	}](t, rec).Code
	if rec.Code != status || (code != "" && got != code) {
		t.Fatalf("answered %d %s, want %d %s", rec.Code, rec.Body, status, code)
	}
}

type listBody struct {
	Dashboards []struct {
		ID     string `json:"id"`
		Repair *struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"repair"`
	} `json:"dashboards"`
	NextCursor *string `json:"next_cursor"`
}

type queuedBody struct {
	Queued []struct {
		DashboardID string `json:"dashboard_id"`
		RequestID   string `json:"request_id"`
	} `json:"queued"`
	Skipped []struct {
		DashboardID string `json:"dashboard_id"`
		Reason      string `json:"reason"`
	} `json:"skipped"`
}

func TestHTTPListsRequestsAndListsAgain(t *testing.T) {
	r := newRig(t)
	h := newHTTPRig(t, r, r.svc)
	id := r.failing(t, r.shared, "Board").String()
	placeholder := r.dashboard(t, r.shared, "Placeholders")
	r.setPanels(t, placeholder, `[{"id":"p","type":"placeholder"}]`)

	before := h.call(t, http.MethodGet, "/api/v1/repairs?limit=1", "")
	expect(t, before, http.StatusOK, "")
	first := decode[listBody](t, before)
	if len(first.Dashboards) != 1 || first.NextCursor == nil {
		t.Fatalf("first page %s", before.Body)
	}
	expect(t, h.call(t, http.MethodGet, "/api/v1/repairs?limit=1&cursor="+*first.NextCursor, ""), http.StatusOK, "")

	queued := h.call(t, http.MethodPost, "/api/v1/repairs", `{"dashboard_ids":["`+id+`","`+placeholder.String()+`"]}`)
	expect(t, queued, http.StatusAccepted, "")
	body := decode[queuedBody](t, queued)
	if len(body.Queued) != 1 || body.Queued[0].DashboardID != id || len(body.Skipped) != 1 || body.Skipped[0].Reason != repairs.ReasonNothingToRepair {
		t.Fatalf("outcome %s", queued.Body)
	}
	again := decode[queuedBody](t, h.call(t, http.MethodPost, "/api/v1/repairs", `{"all":true,"folder_id":"`+r.shared.String()+`"}`))
	if len(again.Queued) != 0 || len(again.Skipped) != 2 {
		t.Fatalf("second outcome %+v", again)
	}

	after := decode[listBody](t, h.call(t, http.MethodGet, "/api/v1/repairs?folder_id="+r.shared.String(), ""))
	found := false
	for _, d := range after.Dashboards {
		if d.ID == id {
			found = d.Repair != nil && d.Repair.Status == "queued" && d.Repair.ID == body.Queued[0].RequestID
		}
	}
	if !found || after.NextCursor != nil {
		t.Fatalf("list after the request %+v", after)
	}
}

func TestHTTPViewersAreRefused(t *testing.T) {
	r := newRig(t)
	h := newHTTPRig(t, r, r.svc)
	h.caller = r.viewer
	expect(t, h.call(t, http.MethodGet, "/api/v1/repairs", ""), http.StatusForbidden, "forbidden_role")
	expect(t, h.call(t, http.MethodPost, "/api/v1/repairs", `{"all":true}`), http.StatusForbidden, "forbidden_role")
}

func TestHTTPRejectsBadRequests(t *testing.T) {
	r := newRig(t)
	h := newHTTPRig(t, r, r.svc)
	id := r.failing(t, r.shared, "Board").String()
	ids := make([]string, 101)
	for i := range ids {
		ids[i] = fmt.Sprintf(`"%08x-0000-4000-8000-000000000000"`, i)
	}
	for _, tc := range []struct{ name, method, path, body string }{
		{"empty body", http.MethodPost, "/api/v1/repairs", `{}`},
		{"both fields", http.MethodPost, "/api/v1/repairs", `{"dashboard_ids":["` + id + `"],"all":true}`},
		{"all false", http.MethodPost, "/api/v1/repairs", `{"all":false}`},
		{"no ids", http.MethodPost, "/api/v1/repairs", `{"dashboard_ids":[]}`},
		{"101 ids", http.MethodPost, "/api/v1/repairs", `{"dashboard_ids":[` + strings.Join(ids, ",") + `]}`},
		{"folder without all", http.MethodPost, "/api/v1/repairs", `{"dashboard_ids":["` + id + `"],"folder_id":"` + r.shared.String() + `"}`},
		{"not a uuid", http.MethodPost, "/api/v1/repairs", `{"dashboard_ids":["nope"]}`},
		{"folder not a uuid", http.MethodPost, "/api/v1/repairs", `{"all":true,"folder_id":"nope"}`},
		{"unknown field", http.MethodPost, "/api/v1/repairs", `{"all":true,"extra":1}`},
		{"list folder not a uuid", http.MethodGet, "/api/v1/repairs?folder_id=nope", ""},
		{"list limit", http.MethodGet, "/api/v1/repairs?limit=101", ""},
		{"list cursor", http.MethodGet, "/api/v1/repairs?cursor=!!", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expect(t, h.call(t, tc.method, tc.path, tc.body), http.StatusBadRequest, "invalid_request")
		})
	}
}

func TestHTTPUnknownOrHiddenFolderIsNotFound(t *testing.T) {
	r := newRig(t)
	h := newHTTPRig(t, r, r.svc)
	h.caller = r.other
	secret := r.folder(t, "Secret")
	r.restrict(t, secret)
	unknown := "6d2c1b9e-6a55-4f3e-9f2b-6a8d2f1e0c11"
	for _, folder := range []string{secret.String(), unknown} {
		expect(t, h.call(t, http.MethodGet, "/api/v1/repairs?folder_id="+folder, ""), http.StatusNotFound, "not_found")
		expect(t, h.call(t, http.MethodPost, "/api/v1/repairs", `{"all":true,"folder_id":"`+folder+`"}`), http.StatusNotFound, "not_found")
	}
}

func TestHTTPWithoutAnAgentIsUnavailable(t *testing.T) {
	r := newRig(t)
	h := newHTTPRig(t, r, r.noAgent)
	id := r.failing(t, r.shared, "Board").String()
	expect(t, h.call(t, http.MethodPost, "/api/v1/repairs", `{"dashboard_ids":["`+id+`"]}`), http.StatusServiceUnavailable, "agent_unavailable")
	expect(t, h.call(t, http.MethodGet, "/api/v1/repairs", ""), http.StatusOK, "")
}
