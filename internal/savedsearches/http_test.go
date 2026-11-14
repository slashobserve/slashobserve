package savedsearches_test

import (
	"bytes"
	"encoding/json"
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
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
	"github.com/slashobserve/slashobserve/internal/savedsearches"
)

type documented struct {
	rig
	engine *gin.Engine
	router routers.Router
	who    auth.Principal
}

func newDocumented(t *testing.T) *documented {
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
	d := &documented{rig: newRig(t), engine: gin.New(), router: router, who: editor}
	savedsearches.NewHandler(d.service, slog.New(slog.DiscardHandler)).Register(auth.NewRoutes(d.engine.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), d.who))
	})))
	return d
}

func (d *documented) send(t *testing.T, by auth.Principal, method, path, body string, want int) *httptest.ResponseRecorder {
	t.Helper()
	d.who = by
	request := func() *http.Request {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		return r
	}
	rec := httptest.NewRecorder()
	d.engine.ServeHTTP(rec, request())
	if rec.Code != want {
		t.Fatalf("%s %s answered %d, want %d: %s", method, path, rec.Code, want, rec.Body)
	}
	check := request()
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

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Code == "" {
		t.Fatalf("no error code in %s", rec.Body)
	}
	return body.Code
}

func idOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.ID == "" {
		t.Fatalf("no id in %s", rec.Body)
	}
	return body.ID
}

func TestSavedSearchRoutesMatchTheDocument(t *testing.T) {
	d := newDocumented(t)
	restricted := foldertest.Restricted(t, d.db, "Payroll", payrollGroup)
	shared := d.folder(t, "Shared")
	const logsBody = `{"kind":"logs","title":"Errors","datasource_uid":"loki","query":"{app=\"x\"}","filters":{"columns":["level"]},"from":"now-15m","to":"now"}`

	created := d.send(t, editor, http.MethodPost, "/api/v1/saved-searches", logsBody, http.StatusCreated)
	id := idOf(t, created)
	traces := d.send(t, editor, http.MethodPost, "/api/v1/saved-searches", `{"kind":"traces","title":"Slow","datasource_uid":"tempo","filters":{"service":"checkout","min":"250ms","errors":true},"folder_id":"`+shared.String()+`"}`, http.StatusCreated)
	if !strings.Contains(traces.Body.String(), `"query":null`) || !strings.Contains(traces.Body.String(), `"url":"/traces/tempo?`) {
		t.Fatalf("trace search %s", traces.Body)
	}

	page := d.send(t, editor, http.MethodGet, "/api/v1/saved-searches?limit=1", "", http.StatusOK)
	var list struct {
		SavedSearches []savedsearches.SavedSearchJSON `json:"saved_searches"`
		NextCursor    *string                         `json:"next_cursor"`
	}
	if err := json.Unmarshal(page.Body.Bytes(), &list); err != nil || len(list.SavedSearches) != 1 || list.NextCursor == nil {
		t.Fatalf("first page %s", page.Body)
	}
	d.send(t, editor, http.MethodGet, "/api/v1/saved-searches?limit=1&cursor="+*list.NextCursor, "", http.StatusOK)
	d.send(t, editor, http.MethodGet, "/api/v1/saved-searches?kind=logs&folder_id="+shared.String(), "", http.StatusOK)
	d.send(t, viewer, http.MethodGet, "/api/v1/saved-searches", "", http.StatusOK)

	renamed := d.send(t, editor, http.MethodPatch, "/api/v1/saved-searches/"+id, `{"title":"Checkout errors"}`, http.StatusOK)
	if !strings.Contains(renamed.Body.String(), `"title":"Checkout errors"`) {
		t.Fatalf("rename %s", renamed.Body)
	}
	d.send(t, editor, http.MethodPatch, "/api/v1/saved-searches/"+id, `{"folder_id":"`+shared.String()+`"}`, http.StatusOK)
	d.send(t, editor, http.MethodDelete, "/api/v1/saved-searches/"+id, "", http.StatusNoContent)

	refusals := []struct {
		name, method, path, body string
		by                       auth.Principal
		want                     int
		code                     string
	}{
		{"a list limit over 100", http.MethodGet, "/api/v1/saved-searches?limit=101", "", editor, http.StatusBadRequest, "invalid_request"},
		{"a list kind that is not one", http.MethodGet, "/api/v1/saved-searches?kind=metrics", "", editor, http.StatusBadRequest, "invalid_request"},
		{"a list folder that is not a UUID", http.MethodGet, "/api/v1/saved-searches?folder_id=nope", "", editor, http.StatusBadRequest, "invalid_request"},
		{"a list cursor that is not one", http.MethodGet, "/api/v1/saved-searches?cursor=nope", "", editor, http.StatusBadRequest, "invalid_request"},
		{"a list folder the caller cannot view", http.MethodGet, "/api/v1/saved-searches?folder_id=" + restricted.String(), "", editor, http.StatusNotFound, "not_found"},
		{"a create body that is not JSON", http.MethodPost, "/api/v1/saved-searches", `{`, editor, http.StatusBadRequest, "invalid_request"},
		{"a create folder that is not a UUID", http.MethodPost, "/api/v1/saved-searches", `{"kind":"logs","title":"x","datasource_uid":"loki","query":"q","folder_id":"nope"}`, editor, http.StatusBadRequest, "invalid_request"},
		{"a create in a folder the caller cannot view", http.MethodPost, "/api/v1/saved-searches", `{"kind":"logs","title":"x","datasource_uid":"loki","query":"q","folder_id":"` + restricted.String() + `"}`, editor, http.StatusNotFound, "not_found"},
		{"a create in a folder where the caller cannot create", http.MethodPost, "/api/v1/saved-searches", `{"kind":"logs","title":"x","datasource_uid":"loki","query":"q","folder_id":"` + d.general(t).String() + `"}`, viewer, http.StatusForbidden, "forbidden_role"},
		{"a create with a field that breaks a rule", http.MethodPost, "/api/v1/saved-searches", `{"kind":"logs","title":"","datasource_uid":"loki","query":"q"}`, editor, http.StatusUnprocessableEntity, "invalid_saved_search"},
		{"a create with an unknown filter key", http.MethodPost, "/api/v1/saved-searches", `{"kind":"logs","title":"x","datasource_uid":"loki","query":"q","filters":{"colums":[]}}`, editor, http.StatusUnprocessableEntity, "invalid_saved_search"},
		{"a create on an unknown datasource", http.MethodPost, "/api/v1/saved-searches", `{"kind":"logs","title":"x","datasource_uid":"nowhere","query":"q"}`, editor, http.StatusUnprocessableEntity, "unknown_datasource"},
		{"a create on a datasource of the wrong kind", http.MethodPost, "/api/v1/saved-searches", `{"kind":"logs","title":"x","datasource_uid":"tempo","query":"q"}`, editor, http.StatusUnprocessableEntity, "unsupported_datasource"},
		{"an update that is not a UUID", http.MethodPatch, "/api/v1/saved-searches/nope", `{"title":"x"}`, editor, http.StatusBadRequest, "invalid_request"},
		{"an update with nothing in it", http.MethodPatch, "/api/v1/saved-searches/" + idOf(t, traces), `{}`, editor, http.StatusBadRequest, "invalid_request"},
		{"an update of another person's search", http.MethodPatch, "/api/v1/saved-searches/" + id, `{"title":"x"}`, otherEditor, http.StatusNotFound, "not_found"},
		{"an update by someone who can only view", http.MethodPatch, "/api/v1/saved-searches/" + idOf(t, traces), `{"title":"x"}`, viewer, http.StatusForbidden, "forbidden_role"},
		{"an update with an empty title", http.MethodPatch, "/api/v1/saved-searches/" + idOf(t, traces), `{"title":" "}`, editor, http.StatusUnprocessableEntity, "invalid_saved_search"},
		{"a move into My Dashboards", http.MethodPatch, "/api/v1/saved-searches/" + idOf(t, traces), `{"folder_id":"` + d.personal(t, editor).String() + `"}`, editor, http.StatusConflict, "personal_folder"},
		{"a move into a folder that does not exist", http.MethodPatch, "/api/v1/saved-searches/" + idOf(t, traces), `{"folder_id":"5f3c1a52-8a0e-4b8c-9a55-0a6f3e4b7d10"}`, editor, http.StatusNotFound, "not_found"},
		{"a delete that is not a UUID", http.MethodDelete, "/api/v1/saved-searches/nope", "", editor, http.StatusBadRequest, "invalid_request"},
		{"a delete of a search that is gone", http.MethodDelete, "/api/v1/saved-searches/" + id, "", editor, http.StatusNotFound, "not_found"},
		{"a delete by someone who can only view", http.MethodDelete, "/api/v1/saved-searches/" + idOf(t, traces), "", viewer, http.StatusForbidden, "forbidden_role"},
	}
	for _, c := range refusals {
		t.Run(c.name, func(t *testing.T) {
			if code := errorCode(t, d.send(t, c.by, c.method, c.path, c.body, c.want)); code != c.code {
				t.Fatalf("code %q, want %q", code, c.code)
			}
		})
	}
}
