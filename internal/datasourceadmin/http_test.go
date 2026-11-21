package datasourceadmin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
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
	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/audit/audittest"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
)

type httpRig struct {
	*rig
	engine *gin.Engine
	router routers.Router
	caller auth.Principal
	flush  func()
}

func newHTTPRig(t *testing.T, file ...datasource.Instance) *httpRig {
	t.Helper()
	return buildHTTPRig(t, false, file...)
}

func newAuditedHTTPRig(t *testing.T, file ...datasource.Instance) *httpRig {
	t.Helper()
	return buildHTTPRig(t, true, file...)
}

func buildHTTPRig(t *testing.T, audited bool, file ...datasource.Instance) *httpRig {
	t.Helper()
	base, svc := newRig(t, testKey, file...)
	openapi3.DefineStringFormatValidator("uuid", openapi3.NewRegexpFormatValidator(openapi3.FormatOfStringForUUIDOfRFC9562))
	doc, err := api.Load(reading(t.Context()))
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
	var middleware []gin.HandlerFunc
	if audited {
		var mw gin.HandlerFunc
		mw, h.flush = audittest.Middleware(t, base.database)
		middleware = append(middleware, mw)
	}
	middleware = append(middleware, func(c *gin.Context) {
		auth.Authenticated(c.Request.Context(), h.caller, audit.AuthSession, "")
		c.Request = c.Request.WithContext(datasource.WithAccess(auth.WithPrincipal(c.Request.Context(), h.caller), datasource.Access{By: h.caller}))
	})
	NewHandler(svc, slog.New(slog.DiscardHandler)).Register(auth.NewRoutes(h.engine.Group("/api/v1", middleware...)))
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
	if err := openapi3filter.ValidateRequest(reading(t.Context()), input); err != nil && rec.Code < 400 {
		t.Fatalf("%s %s breaks the document but answered %d: %v", method, path, rec.Code, err)
	}
	check.Body = io.NopCloser(bytes.NewReader(nil))
	response := &openapi3filter.ResponseValidationInput{RequestValidationInput: input, Status: rec.Code, Header: rec.Header(), Options: options}
	response.SetBodyBytes(rec.Body.Bytes())
	if err := openapi3filter.ValidateResponse(reading(t.Context()), response); err != nil {
		t.Fatalf("%s %s answered %d against the document: %v\n%s", method, path, rec.Code, err, rec.Body)
	}
	return rec
}

func expectCode(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	var body struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != status || body.Code != code {
		t.Fatalf("answered %d %q, want %d %q: %s", rec.Code, body.Code, status, code, rec.Body)
	}
}

func createBody(uid, address string) string {
	return fmt.Sprintf(`{"uid":%q,"type":"prometheus","name":"Metrics","address":%q,"secrets":{"bearer_token":"token-1","headers":{"X-Org":"org-7"}}}`, uid, address)
}

func TestAnAdminAddsReadsChangesTestsAndRemovesADatasourceOverHTTP(t *testing.T) {
	var auth string
	server := upstream(t, &auth)
	h := newHTTPRig(t)

	created := h.call(t, http.MethodPost, "/api/v1/datasources", createBody("added", server.URL))
	if created.Code != http.StatusCreated {
		t.Fatalf("create answered %d: %s", created.Code, created.Body)
	}
	for _, secret := range []string{"token-1", "org-7"} {
		if strings.Contains(created.Body.String(), secret) {
			t.Fatalf("the create response carries %q", secret)
		}
	}

	got := h.call(t, http.MethodGet, "/api/v1/datasources/added/config", "")
	var record struct {
		Address    string   `json:"address"`
		Source     string   `json:"source"`
		SecretsSet []string `json:"secrets_set"`
	}
	if err := json.Unmarshal(got.Body.Bytes(), &record); err != nil || record.Address != server.URL || record.Source != "app" || len(record.SecretsSet) != 2 {
		t.Fatalf("config = %s, %v", got.Body, err)
	}
	if strings.Contains(got.Body.String(), "token-1") || strings.Contains(got.Body.String(), "org-7") {
		t.Fatal("the config response carries a credential")
	}

	patched := h.call(t, http.MethodPatch, "/api/v1/datasources/added", `{"name":"Renamed","limits":{"max_rows":50},"clear_secrets":["header:X-Org"]}`)
	if patched.Code != http.StatusOK || !strings.Contains(patched.Body.String(), `"Renamed"`) || !strings.Contains(patched.Body.String(), `"address":"`+server.URL+`"`) || strings.Contains(patched.Body.String(), "header:X-Org") {
		t.Fatalf("patch answered %d: %s", patched.Code, patched.Body)
	}

	tested := h.call(t, http.MethodPost, "/api/v1/datasources/test?base=added", `{"name":"Draft"}`)
	if tested.Code != http.StatusOK || !strings.Contains(tested.Body.String(), `"healthy":true`) || auth != "Bearer token-1" {
		t.Fatalf("test answered %d: %s, Authorization = %q", tested.Code, tested.Body, auth)
	}

	removed := h.call(t, http.MethodDelete, "/api/v1/datasources/added", "")
	if removed.Code != http.StatusNoContent {
		t.Fatalf("delete answered %d: %s", removed.Code, removed.Body)
	}
	expectCode(t, h.call(t, http.MethodGet, "/api/v1/datasources/added/config", ""), http.StatusNotFound, "not_found")
}

func TestHTTPErrorsCarryStableCodes(t *testing.T) {
	server := upstream(t, nil)
	file := datasource.Instance{UID: "fromfile", Datasource: datasource.NewUnavailable("prometheus", datasource.ErrUnavailable), Source: datasource.SourceFile}
	builtIn := datasource.Instance{UID: "audit", Datasource: datasource.NewUnavailable("opensearch", datasource.ErrUnavailable), Source: datasource.SourceBuiltin, Audience: datasource.AudienceAdmins}
	h := newHTTPRig(t, file, builtIn)
	h.call(t, http.MethodPost, "/api/v1/datasources", createBody("added", server.URL))

	for _, method := range []string{http.MethodPatch, http.MethodDelete} {
		rec := h.call(t, method, "/api/v1/datasources/audit", `{"name":"x"}`)
		expectCode(t, rec, http.StatusConflict, "datasource_read_only")
		if !strings.Contains(rec.Body.String(), "built into the server") {
			t.Errorf("%s of the built-in datasource said %s", method, rec.Body)
		}
	}
	expectCode(t, h.call(t, http.MethodGet, "/api/v1/datasources/audit/config", ""), http.StatusConflict, "datasource_read_only")
	expectCode(t, h.call(t, http.MethodPost, "/api/v1/datasources", createBody("audit", server.URL)), http.StatusConflict, "uid_taken")
	expectCode(t, h.call(t, http.MethodPost, "/api/v1/datasources", createBody("added", server.URL)), http.StatusConflict, "uid_taken")
	expectCode(t, h.call(t, http.MethodPost, "/api/v1/datasources", createBody("fromfile", server.URL)), http.StatusConflict, "uid_taken")
	expectCode(t, h.call(t, http.MethodPost, "/api/v1/datasources", `{"uid":"x","type":"prometheus"}`), http.StatusUnprocessableEntity, "invalid_datasource")
	expectCode(t, h.call(t, http.MethodPost, "/api/v1/datasources", createBody("link", "http://169.254.169.254")), http.StatusUnprocessableEntity, "invalid_datasource")
	expectCode(t, h.call(t, http.MethodPatch, "/api/v1/datasources/fromfile", `{"name":"x"}`), http.StatusConflict, "datasource_read_only")
	expectCode(t, h.call(t, http.MethodDelete, "/api/v1/datasources/fromfile", ""), http.StatusConflict, "datasource_read_only")
	expectCode(t, h.call(t, http.MethodGet, "/api/v1/datasources/fromfile/config", ""), http.StatusConflict, "datasource_read_only")
	expectCode(t, h.call(t, http.MethodPatch, "/api/v1/datasources/nothing", `{"name":"x"}`), http.StatusNotFound, "not_found")
	expectCode(t, h.call(t, http.MethodPatch, "/api/v1/datasources/added", `{"type":"loki"}`), http.StatusUnprocessableEntity, "invalid_datasource")
	expectCode(t, h.call(t, http.MethodPatch, "/api/v1/datasources/added", `{"unknown":1}`), http.StatusBadRequest, "invalid_request")
}

func TestHTTPDeleteListsWhatUsesTheDatasource(t *testing.T) {
	server := upstream(t, nil)
	h := newHTTPRig(t)
	h.call(t, http.MethodPost, "/api/v1/datasources", createBody("added", server.URL))
	addDashboard(t, h.rig, "Checkout", "added")

	rec := h.call(t, http.MethodDelete, "/api/v1/datasources/added", "")
	expectCode(t, rec, http.StatusConflict, "datasource_in_use")
	var body struct {
		UsedBy []struct{ Kind, Title string } `json:"used_by"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || len(body.UsedBy) != 1 || body.UsedBy[0].Title != "Checkout" {
		t.Fatalf("body = %s, %v", rec.Body, err)
	}
}

func TestOnlyAnAdminReachesTheManagementRoutes(t *testing.T) {
	h := newHTTPRig(t)
	routes := []struct{ method, path, body string }{
		{http.MethodPost, "/api/v1/datasources", createBody("x", "http://x:9090")},
		{http.MethodPost, "/api/v1/datasources/test", createBody("x", "http://x:9090")},
		{http.MethodGet, "/api/v1/datasources/x/config", ""},
		{http.MethodPatch, "/api/v1/datasources/x", `{"name":"y"}`},
		{http.MethodDelete, "/api/v1/datasources/x", ""},
	}
	for _, role := range []auth.Role{auth.RoleEditor, auth.RoleCommenter, auth.RoleViewer} {
		h.caller = auth.Principal{Kind: auth.KindUser, ID: h.admin.ID, Name: "lower", Role: role}
		for _, route := range routes {
			rec := h.call(t, route.method, route.path, route.body)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("%s %s as %s answered %d, want 403", route.method, route.path, role, rec.Code)
			}
		}
	}
}
