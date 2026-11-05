package grafanaconn_test

import (
	"bytes"
	"encoding/json"
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
	"github.com/slashobserve/slashobserve/internal/grafanaconn"
)

type httpRig struct {
	*rig
	engine *gin.Engine
	router routers.Router
	caller auth.Principal
}

var serviceAccount = auth.Principal{Kind: auth.KindServiceAccount, ID: "3f1c2a52-8c5e-4a53-9a3e-1d2d4c0a9b11", Name: "ci", Role: auth.RoleAdmin}

func newHTTPRig(t *testing.T, base *rig) *httpRig {
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
	h := &httpRig{rig: base, router: router, caller: base.admin}
	gin.SetMode(gin.TestMode)
	h.engine = gin.New()
	grafanaconn.NewHandler(base.svc).Register(auth.NewRoutes(h.engine.Group("/api/v1", func(c *gin.Context) {
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
	if strings.Contains(rec.Body.String(), grafanaToken) || strings.Contains(rec.Body.String(), "glsa_") {
		t.Fatalf("%s %s returned a token: %s", method, path, rec.Body)
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

func codeOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	return decode[struct {
		Code string `json:"code"`
	}](t, rec).Code
}

func expect(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status || (code != "" && codeOf(t, rec) != code) {
		t.Fatalf("answered %d %s, want %d %s", rec.Code, rec.Body, status, code)
	}
}

type idBody struct {
	ID string `json:"id"`
}

func TestHTTPConnectsTestsListsImportsAndResolves(t *testing.T) {
	h := newHTTPRig(t, newRig(t))
	threeFolders(h.grafana)
	h.grafana.add(&fakeDashboard{uid: "checkout", title: "Checkout", folder: "f-k8s", model: simpleModel("graf-prom", "prometheus"), version: 1})
	h.grafana.add(&fakeDashboard{uid: "broken", title: "Broken", model: simpleModel("graf-prom", "prometheus"), version: 1, status: 404})

	created := h.call(t, http.MethodPost, "/api/v1/grafana", `{"name":"Production","url":"`+grafanaURL+`","token":"`+grafanaToken+`"}`)
	expect(t, created, http.StatusCreated, "")
	id := decode[idBody](t, created).ID
	if created.Header().Get("Location") != "/api/v1/grafana/"+id {
		t.Fatalf("location %q", created.Header().Get("Location"))
	}
	base := "/api/v1/grafana/" + id

	expect(t, h.call(t, http.MethodGet, "/api/v1/grafana", ""), http.StatusOK, "")
	expect(t, h.call(t, http.MethodGet, base, ""), http.StatusOK, "")
	expect(t, h.call(t, http.MethodPatch, base, `{"name":"Prod"}`), http.StatusOK, "")
	tested := h.call(t, http.MethodPost, base+"/test", "")
	expect(t, tested, http.StatusOK, "")
	if !strings.Contains(tested.Body.String(), `"ok":true`) {
		t.Fatalf("test %s", tested.Body)
	}
	folders := h.call(t, http.MethodGet, base+"/folders", "")
	expect(t, folders, http.StatusOK, "")
	if !strings.Contains(folders.Body.String(), `"title":"Platform / Kubernetes"`) {
		t.Fatalf("folders %s", folders.Body)
	}
	expect(t, h.call(t, http.MethodGet, base+"/datasources", ""), http.StatusOK, "")

	started := h.call(t, http.MethodPost, base+"/imports", `{"folders":"all","mapping":{"Loki":"prom"}}`)
	expect(t, started, http.StatusAccepted, "")
	syncID := decode[idBody](t, started).ID
	if started.Header().Get("Location") != base+"/imports/"+syncID {
		t.Fatalf("location %q", started.Header().Get("Location"))
	}
	h.svc.Wait()
	progress := h.call(t, http.MethodGet, base+"/imports/"+syncID, "")
	expect(t, progress, http.StatusOK, "")
	if !strings.Contains(progress.Body.String(), `"status":"finished"`) || !strings.Contains(progress.Body.String(), `"outcome":"failed"`) {
		t.Fatalf("progress %s", progress.Body)
	}
	expect(t, h.call(t, http.MethodGet, base+"/imports/"+id, ""), http.StatusNotFound, "not_found")
	expect(t, h.call(t, http.MethodGet, base, ""), http.StatusOK, "")

	h.caller = h.viewer
	resolved := h.call(t, http.MethodGet, "/api/v1/grafana/resolve/checkout", "")
	expect(t, resolved, http.StatusOK, "")
	got := decode[struct {
		DashboardID string `json:"dashboard_id"`
		URL         string `json:"url"`
	}](t, resolved)
	if got.URL != "/dashboards/"+got.DashboardID {
		t.Fatalf("resolved %+v", got)
	}
	expect(t, h.call(t, http.MethodGet, "/api/v1/grafana/resolve/broken", ""), http.StatusNotFound, "not_found")

	h.caller = h.admin
	expect(t, h.call(t, http.MethodDelete, base, ""), http.StatusNoContent, "")
	expect(t, h.call(t, http.MethodGet, base, ""), http.StatusNotFound, "not_found")
}

func TestHTTPAdminRoutesRefuseLowerRoles(t *testing.T) {
	h := newHTTPRig(t, newRig(t))
	id := h.connect(t).ID
	h.caller = h.viewer
	base := "/api/v1/grafana/" + id
	routes := []struct{ method, path, body string }{
		{http.MethodGet, "/api/v1/grafana", ""},
		{http.MethodPost, "/api/v1/grafana", `{"name":"x","url":"` + grafanaURL + `","token":"t"}`},
		{http.MethodGet, base, ""},
		{http.MethodPatch, base, `{"name":"x"}`},
		{http.MethodDelete, base, ""},
		{http.MethodPost, base + "/test", ""},
		{http.MethodGet, base + "/folders", ""},
		{http.MethodGet, base + "/datasources", ""},
		{http.MethodPost, base + "/imports", `{"folders":"all"}`},
		{http.MethodGet, base + "/imports/x", ""},
	}
	for _, r := range routes {
		expect(t, h.call(t, r.method, r.path, r.body), http.StatusForbidden, "forbidden_role")
	}
}

func TestHTTPServiceAccountAdminsCanConnect(t *testing.T) {
	h := newHTTPRig(t, newRig(t))
	h.caller = serviceAccount
	created := h.call(t, http.MethodPost, "/api/v1/grafana", `{"name":"CI","url":"`+grafanaURL+`","token":"`+grafanaToken+`"}`)
	expect(t, created, http.StatusCreated, "")
	if !strings.Contains(created.Body.String(), `"created_by":"`+serviceAccount.ID+`"`) {
		t.Fatalf("created %s", created.Body)
	}
}

func TestHTTPRefusesBadRequests(t *testing.T) {
	h := newHTTPRig(t, newRig(t))
	base := "/api/v1/grafana/" + h.connect(t).ID
	tooMany := map[string]string{}
	for i := range 201 {
		tooMany[numbered("ds", i)] = "prom"
	}
	mapping, err := json.Marshal(map[string]any{"folders": "all", "mapping": tooMany})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct{ method, path, body string }{
		{http.MethodPost, "/api/v1/grafana", `{"name":"x","url":"` + grafanaURL + `"}`},
		{http.MethodPost, "/api/v1/grafana", `{"name":"x","url":"ftp://` + grafanaHost + `","token":"t"}`},
		{http.MethodPost, "/api/v1/grafana", `{"name":"x","url":"` + grafanaURL + `","token":"t","extra":1}`},
		{http.MethodGet, "/api/v1/grafana/not-a-uuid", ""},
		{http.MethodPatch, base, `{"url":"https://grafana2.example.com"}`},
		{http.MethodPost, base + "/imports", `{}`},
		{http.MethodPost, base + "/imports", `{"folders":[]}`},
		{http.MethodPost, base + "/imports", `{"folders":"some"}`},
		{http.MethodPost, base + "/imports", `{"folders":[""]}`},
		{http.MethodPost, base + "/imports", `{"folders":"all","mapping":{"graf-prom":""}}`},
		{http.MethodPost, base + "/imports", `{"folders":"all","mapping":{"graf-prom":"unknown"}}`},
		{http.MethodPost, base + "/imports", string(mapping)},
	}
	for _, c := range cases {
		expect(t, h.call(t, c.method, c.path, c.body), http.StatusBadRequest, "invalid_request")
	}
}

func TestHTTPNamesTheMissingSecretKey(t *testing.T) {
	h := newHTTPRig(t, newRigWith(t, rigOptions{}))
	rec := h.call(t, http.MethodPost, "/api/v1/grafana", `{"name":"x","url":"`+grafanaURL+`","token":"`+grafanaToken+`"}`)
	expect(t, rec, http.StatusConflict, "secret_key_missing")
	if !strings.Contains(rec.Body.String(), "SLASHOBSERVE_SECRET_KEY") {
		t.Fatalf("body %s", rec.Body)
	}
}

func TestHTTPReportsAGrafanaThatRefusesTheToken(t *testing.T) {
	h := newHTTPRig(t, newRig(t))
	h.grafana.token = "rotated"
	base := "/api/v1/grafana/" + h.connect(t).ID
	for _, path := range []string{base + "/folders", base + "/datasources"} {
		rec := h.call(t, http.MethodGet, path, "")
		expect(t, rec, http.StatusBadGateway, "grafana_unavailable")
		if !strings.Contains(rec.Body.String(), "Grafana refused the token (status 401).") {
			t.Fatalf("body %s", rec.Body)
		}
	}
}

func TestHTTPRefusesASecondImportWhileOneRuns(t *testing.T) {
	hold := make(chan struct{})
	h := newHTTPRig(t, newRigWith(t, rigOptions{secretKey: secretKey, hold: hold}))
	h.grafana.add(&fakeDashboard{uid: "d", title: "Dash", model: simpleModel("graf-prom", "prometheus"), version: 1})
	base := "/api/v1/grafana/" + h.connect(t).ID
	expect(t, h.call(t, http.MethodPost, base+"/imports", `{"folders":["general"]}`), http.StatusAccepted, "")
	expect(t, h.call(t, http.MethodPost, base+"/imports", `{"folders":"all"}`), http.StatusConflict, "import_running")
	close(hold)
	h.svc.Wait()
}
