package findings_test

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
	"github.com/slashobserve/slashobserve/internal/annotations"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
	"github.com/slashobserve/slashobserve/internal/observations"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
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
	r := newRig(t)
	logger := slog.New(slog.DiscardHandler)
	registry, err := datasource.NewRegistry(logger)
	if err != nil {
		t.Fatal(err)
	}
	access := folders.NewAccess(r.db.DB)
	dash := dashboards.New(r.db.DB, queryproxy.NewRunner(registry, logger), access, logger)
	r.service, err = findings.New(r.db.DB, access, annotations.New(r.db.DB, access), dash, observations.New(r.db.DB, access, nil, nil, logger), logger)
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	d := &documented{rig: r, engine: gin.New(), router: router, who: editor}
	findings.NewHandler(r.service, func(f findings.Finding) string { return "/o/" + f.ObservationID }, logger).Register(auth.NewRoutes(d.engine.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), d.who))
	})))
	return d
}

func (d *documented) call(t *testing.T, by auth.Principal, method, path string, want int) *httptest.ResponseRecorder {
	t.Helper()
	return d.send(t, by, method, path, "", want)
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

func panelOf(t *testing.T, raw string) spec.Panel {
	t.Helper()
	var p spec.Panel
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatal(err)
	}
	return p
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

func TestFindingReadsMatchTheDocument(t *testing.T) {
	d := newDocumented(t)
	shared := d.save(t, editor, findings.SaveInput{DashboardID: d.open})
	private := d.save(t, editor, findings.SaveInput{})
	hidden := d.dashboard(t, "Payroll")
	foldertest.Move(t, d.db, hidden, foldertest.Restricted(t, d.db, "Payroll", payrollGroup))

	page := d.call(t, editor, http.MethodGet, "/api/v1/findings?limit=1", http.StatusOK)
	var list struct {
		Findings   []findings.FindingJSON `json:"findings"`
		NextCursor *string                `json:"next_cursor"`
	}
	if err := json.Unmarshal(page.Body.Bytes(), &list); err != nil || len(list.Findings) != 1 || list.NextCursor == nil {
		t.Fatalf("first page %s", page.Body)
	}
	d.call(t, editor, http.MethodGet, "/api/v1/findings?limit=1&cursor="+*list.NextCursor, http.StatusOK)
	d.call(t, viewer, http.MethodGet, "/api/v1/findings?dashboard_id="+d.open.String(), http.StatusOK)

	get := d.call(t, viewer, http.MethodGet, "/api/v1/findings/"+shared.ID, http.StatusOK)
	if !strings.Contains(get.Body.String(), `"url":"/o/`+shared.ObservationID+`"`) || !strings.Contains(get.Body.String(), `"observation_id":"`+shared.ObservationID+`"`) {
		t.Fatalf("finding without its observation: %s", get.Body)
	}
	if want := `</api/v1/observations/` + shared.ObservationID + `>; rel="successor-version"`; get.Header().Get("Link") != want {
		t.Fatalf("Link %q, want %q", get.Header().Get("Link"), want)
	}
	var shown struct {
		Finding struct {
			Actor struct {
				Label string `json:"label"`
			} `json:"actor"`
		} `json:"finding"`
	}
	if err := json.Unmarshal(get.Body.Bytes(), &shown); err != nil || shown.Finding.Actor.Label != editor.Name {
		t.Fatalf("actor label %q, want %q: %s, %v", shown.Finding.Actor.Label, editor.Name, get.Body, err)
	}

	refusals := []struct {
		name string
		by   auth.Principal
		path string
		want int
		code string
	}{
		{"a dashboard id that is not a UUID", editor, "/api/v1/findings?dashboard_id=nope", http.StatusBadRequest, "invalid_request"},
		{"a limit over 100", editor, "/api/v1/findings?limit=101", http.StatusBadRequest, "invalid_request"},
		{"a cursor that is not one", editor, "/api/v1/findings?cursor=nope", http.StatusBadRequest, "invalid_request"},
		{"a dashboard the caller cannot view", viewer, "/api/v1/findings?dashboard_id=" + hidden.String(), http.StatusNotFound, "not_found"},
		{"a finding id that is not a UUID", editor, "/api/v1/findings/nope", http.StatusBadRequest, "invalid_request"},
		{"another editor's private finding", otherEditor, "/api/v1/findings/" + private.ID, http.StatusNotFound, "not_found"},
	}
	for _, c := range refusals {
		t.Run(c.name, func(t *testing.T) {
			if code := errorCode(t, d.call(t, c.by, http.MethodGet, c.path, c.want)); code != c.code {
				t.Fatalf("code %q, want %q", code, c.code)
			}
		})
	}
}

func TestPinningAFindingMatchesTheDocument(t *testing.T) {
	d := newDocumented(t)
	shared := d.save(t, editor, findings.SaveInput{DashboardID: d.open})
	private := d.save(t, editor, findings.SaveInput{})

	if code := errorCode(t, d.call(t, viewer, http.MethodPost, "/api/v1/findings/"+shared.ID+"/pin", http.StatusForbidden)); code != "forbidden_role" {
		t.Fatalf("a viewer pinning got %q", code)
	}
	created := d.call(t, editor, http.MethodPost, "/api/v1/findings/"+shared.ID+"/pin", http.StatusCreated)
	var a struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &a); err != nil {
		t.Fatal(err)
	}
	if created.Header().Get("Location") != "/api/v1/annotations/"+a.ID {
		t.Fatalf("Location %q for annotation %s", created.Header().Get("Location"), a.ID)
	}
	if want := "Checkout errors come from checkout-0 /findings/" + shared.ID; a.Text != want {
		t.Fatalf("pin text %q, want %q", a.Text, want)
	}
	again := d.call(t, editor, http.MethodPost, "/api/v1/findings/"+shared.ID+"/pin", http.StatusOK)
	if !strings.Contains(again.Body.String(), `"id":"`+a.ID+`"`) {
		t.Fatalf("a second pin made another annotation: %s", again.Body)
	}
	if code := errorCode(t, d.call(t, editor, http.MethodPost, "/api/v1/findings/"+private.ID+"/pin", http.StatusConflict)); code != "finding_has_no_dashboard" {
		t.Fatalf("pinning a finding without a dashboard got %q", code)
	}
	d.call(t, otherEditor, http.MethodPost, "/api/v1/findings/"+private.ID+"/pin", http.StatusNotFound)
}

func TestSavingAFindingAsADashboardMatchesTheDocument(t *testing.T) {
	d := newDocumented(t)
	notes := d.save(t, editor, findings.SaveInput{Evidence: []spec.Panel{panelOf(t, `{"id":"evidence-1","type":"text","title":"Notes","gridPos":{"x":0,"y":0,"w":12,"h":8},"content":"checkout-0"}`)}})
	broken := d.save(t, editor, findings.SaveInput{Evidence: []spec.Panel{panelOf(t, `{"id":"evidence-1","type":"timeseries","title":"Errors","gridPos":{"x":0,"y":0,"w":12,"h":8},"datasource":"gone","queries":[{"query":"up"}]}`)}})
	empty := d.save(t, editor, findings.SaveInput{Evidence: []spec.Panel{}})

	saved := d.call(t, editor, http.MethodPost, "/api/v1/findings/"+notes.ID+"/dashboard", http.StatusCreated)
	if !strings.HasPrefix(saved.Header().Get("Location"), "/api/v1/dashboards/") {
		t.Fatalf("Location %q", saved.Header().Get("Location"))
	}
	if code := errorCode(t, d.call(t, editor, http.MethodPost, "/api/v1/findings/"+empty.ID+"/dashboard", http.StatusConflict)); code != "finding_has_no_evidence" {
		t.Fatalf("saving a finding without evidence got %q", code)
	}
	d.call(t, editor, http.MethodPost, "/api/v1/findings/"+broken.ID+"/dashboard", http.StatusUnprocessableEntity)
	d.call(t, otherEditor, http.MethodPost, "/api/v1/findings/"+notes.ID+"/dashboard", http.StatusNotFound)
}

func TestEveryFindingRouteAnswersTheDeprecationHeader(t *testing.T) {
	d := newDocumented(t)
	onDashboard := d.save(t, editor, findings.SaveInput{DashboardID: d.open})
	id := onDashboard.ID
	routes := []struct {
		name         string
		method, path string
		body         string
		want         int
	}{
		{"list", http.MethodGet, "/api/v1/findings", "", http.StatusOK},
		{"get", http.MethodGet, "/api/v1/findings/" + id, "", http.StatusOK},
		{"get of a missing finding", http.MethodGet, "/api/v1/findings/99999999-9999-4999-8999-999999999999", "", http.StatusNotFound},
		{"pin", http.MethodPost, "/api/v1/findings/" + id + "/pin", "", http.StatusCreated},
		{"save as a dashboard", http.MethodPost, "/api/v1/findings/" + id + "/dashboard", "", http.StatusConflict},
	}
	for _, c := range routes {
		t.Run(c.name, func(t *testing.T) {
			rec := d.send(t, editor, c.method, c.path, c.body, c.want)
			if got := rec.Header().Get("Deprecation"); got != "@1794355200" {
				t.Fatalf("Deprecation %q, want @1794355200", got)
			}
		})
	}
}

func TestTheFindingMoveRouteIsGone(t *testing.T) {
	d := newDocumented(t)
	f := d.save(t, editor, findings.SaveInput{})
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/findings/"+f.ID+"/move", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	d.engine.ServeHTTP(rec, request)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("POST /findings/{id}/move answered %d, want 404", rec.Code)
	}
}
