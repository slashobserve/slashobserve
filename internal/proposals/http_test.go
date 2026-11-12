package proposals

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
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/api"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/grantmode"
)

type documented struct {
	*rig
	engine *gin.Engine
	router routers.Router
	who    auth.Principal
	mode   grantmode.Mode
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
	routes := auth.NewRoutes(d.engine.Group("/api/v1", func(c *gin.Context) {
		ctx := auth.WithPrincipal(c.Request.Context(), d.who)
		if d.mode != "" {
			ctx = grantmode.With(ctx, d.mode)
		}
		c.Request = c.Request.WithContext(ctx)
	}))
	d.dashboards.Register(routes)
	NewHandler(d.service, d.access.RequireDashboard(folders.NeedView), slog.New(slog.DiscardHandler)).Register(routes)
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

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %s: %v", rec.Body, err)
	}
	return v
}

func codeOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	return decode[struct {
		Code string `json:"code"`
	}](t, rec).Code
}

func (d *documented) proposeOverHTTP(t *testing.T, by auth.Principal, id pgtype.UUID, base int, panels ...panel) (string, *httptest.ResponseRecorder) {
	t.Helper()
	body := fmt.Sprintf(`{"spec":%s,"summary":"proposed change","parent_version":%d,"propose_if_needed":true}`, specOf("t", panels...), base)
	rec := d.send(t, by, http.MethodPost, "/api/v1/dashboards/"+id.String()+"/versions", body, http.StatusAccepted)
	created := decode[struct {
		ProposalID string `json:"proposal_id"`
		Revision   int    `json:"revision"`
		Opened     bool   `json:"opened"`
	}](t, rec)
	if rec.Header().Get("Location") != "/api/v1/proposals/"+created.ProposalID {
		t.Fatalf("Location = %q", rec.Header().Get("Location"))
	}
	return created.ProposalID, rec
}

func TestProposalRoutesMatchTheDocument(t *testing.T) {
	d := newDocumented(t)
	id := d.createDashboard(t, steady, second)
	dashPath := "/api/v1/dashboards/" + id.String()

	pid, rec := d.proposeOverHTTP(t, commenter, id, 1, changed, second)
	if created := decode[map[string]any](t, rec); created["opened"] != true || created["revision"] != float64(1) {
		t.Fatalf("created = %v", created)
	}
	if _, rec := d.proposeOverHTTP(t, commenter, id, 1, changed, panel{id: "b", query: "node_load1"}); decode[map[string]any](t, rec)["revision"] != float64(2) {
		t.Fatalf("second save = %s", rec.Body)
	}

	list := decode[struct {
		Proposals  []SummaryJSON `json:"proposals"`
		NextCursor *string       `json:"next_cursor"`
	}](t, d.send(t, editor, http.MethodGet, dashPath+"/proposals", "", http.StatusOK))
	if len(list.Proposals) != 1 || list.Proposals[0].ID != pid || list.Proposals[0].State != "open" || list.Proposals[0].Revision != 2 || list.NextCursor != nil {
		t.Fatalf("list = %+v", list)
	}
	d.send(t, editor, http.MethodGet, dashPath+"/proposals?state=all&limit=5", "", http.StatusOK)

	got := decode[ProposalJSON](t, d.send(t, editor, http.MethodGet, "/api/v1/proposals/"+pid, "", http.StatusOK))
	if got.ID != pid || len(got.Changes) != 2 || len(got.Conflicts) != 0 || got.Spec == nil {
		t.Fatalf("proposal = %+v", got)
	}

	d.addVersion(t, id, 1, steady, second, panel{id: "c", query: "up"})
	behind := decode[ProposalJSON](t, d.send(t, editor, http.MethodGet, "/api/v1/proposals/"+pid, "", http.StatusOK))
	if !behind.Behind || behind.LatestVersion != 2 {
		t.Fatalf("behind = %+v", behind.SummaryJSON)
	}

	rec = d.send(t, editor, http.MethodPost, "/api/v1/proposals/"+pid+"/merge", `{"revision":2}`, http.StatusOK)
	merged := decode[SummaryJSON](t, rec)
	if merged.State != "merged" || merged.MergedVersion == nil || *merged.MergedVersion != 3 {
		t.Fatalf("merged = %+v", merged)
	}
	if want := dashPath + "/versions/3"; rec.Header().Get("Location") != want {
		t.Fatalf("Location = %q, want %q", rec.Header().Get("Location"), want)
	}
	version := decode[struct {
		Version struct {
			MergedProposalID *string `json:"merged_proposal_id"`
		} `json:"version"`
	}](t, d.send(t, editor, http.MethodGet, dashPath+"/versions/3", "", http.StatusOK))
	if version.Version.MergedProposalID == nil || *version.Version.MergedProposalID != pid {
		t.Fatalf("merged_proposal_id = %v", version.Version.MergedProposalID)
	}
	versions := d.send(t, editor, http.MethodGet, dashPath+"/versions", "", http.StatusOK)
	if !strings.Contains(versions.Body.String(), `"merged_proposal_id":"`+pid+`"`) {
		t.Fatalf("versions = %s", versions.Body)
	}
	d.send(t, editor, http.MethodGet, dashPath+"/proposals?state=merged", "", http.StatusOK)
}

func TestDiscardTakesAnEmptyBodyOrAnEmptyObject(t *testing.T) {
	d := newDocumented(t)
	id := d.createDashboard(t, steady)
	first, _ := d.proposeOverHTTP(t, commenter, id, 1, changed)
	second, _ := d.proposeOverHTTP(t, otherCommenter, id, 1, changed)

	rec := d.send(t, commenter, http.MethodPost, "/api/v1/proposals/"+first+"/discard", "", http.StatusOK)
	if got := decode[SummaryJSON](t, rec); got.State != "discarded" || got.ClosedBy == nil {
		t.Fatalf("discarded = %+v", got)
	}
	d.send(t, otherCommenter, http.MethodPost, "/api/v1/proposals/"+second+"/discard", `{}`, http.StatusOK)
	d.send(t, editor, http.MethodPost, "/api/v1/proposals/"+second+"/discard", `{"x":1}`, http.StatusBadRequest)
}

func TestProposalRoutesAnswerEveryDocumentedError(t *testing.T) {
	d := newDocumented(t)
	id := d.createDashboard(t, steady, second)
	dashPath := "/api/v1/dashboards/" + id.String()
	pid, _ := d.proposeOverHTTP(t, commenter, id, 1, changed, second)
	proposal := "/api/v1/proposals/" + pid
	missing := "/api/v1/proposals/99999999-9999-4999-8999-999999999999"

	if code := codeOf(t, d.send(t, viewer, http.MethodPost, dashPath+"/versions", fmt.Sprintf(`{"spec":%s,"summary":"s","parent_version":1,"propose_if_needed":true}`, specOf("t", changed)), http.StatusForbidden)); code != "forbidden_role" {
		t.Fatalf("viewer code = %s", code)
	}
	d.send(t, commenter, http.MethodPost, dashPath+"/versions", fmt.Sprintf(`{"spec":%s,"summary":"s","parent_version":1}`, specOf("t", changed)), http.StatusForbidden)

	d.send(t, editor, http.MethodGet, dashPath+"/proposals?state=weird", "", http.StatusBadRequest)
	d.send(t, editor, http.MethodGet, dashPath+"/proposals?limit=0", "", http.StatusBadRequest)
	d.send(t, editor, http.MethodGet, "/api/v1/proposals/not-a-uuid", "", http.StatusBadRequest)
	d.send(t, editor, http.MethodGet, missing, "", http.StatusNotFound)
	d.send(t, editor, http.MethodPost, missing+"/merge", `{"revision":1}`, http.StatusNotFound)
	d.send(t, editor, http.MethodPost, missing+"/discard", "", http.StatusNotFound)

	hidden := d.hiddenFolder(t)
	d.moveInto(t, id, hidden)
	d.send(t, outsider, http.MethodGet, proposal, "", http.StatusNotFound)
	d.send(t, outsider, http.MethodPost, proposal+"/merge", `{"revision":1}`, http.StatusNotFound)
	d.send(t, outsider, http.MethodPost, proposal+"/discard", "", http.StatusNotFound)
	d.send(t, outsider, http.MethodGet, dashPath+"/proposals", "", http.StatusForbidden)
	d.moveInto(t, id, d.generalFolder(t))

	d.send(t, editor, http.MethodPost, proposal+"/merge", `{}`, http.StatusBadRequest)
	d.send(t, editor, http.MethodPost, proposal+"/merge", `{"revision":0}`, http.StatusBadRequest)
	d.send(t, editor, http.MethodPost, proposal+"/merge", `{"revision":1,"choices":[{"kind":"panel","id":"a","take":"mine"}]}`, http.StatusBadRequest)
	d.send(t, editor, http.MethodPost, proposal+"/merge", `{"revision":1,"extra":true}`, http.StatusBadRequest)

	if code := codeOf(t, d.send(t, commenter, http.MethodPost, proposal+"/merge", `{"revision":1}`, http.StatusForbidden)); code != "forbidden_role" {
		t.Fatalf("commenter merge code = %s", code)
	}
	d.mode = grantmode.Propose
	if code := codeOf(t, d.send(t, editor, http.MethodPost, proposal+"/merge", `{"revision":1}`, http.StatusForbidden)); code != "propose_mode" {
		t.Fatalf("propose mode merge code = %s", code)
	}
	d.mode = ""
	if code := codeOf(t, d.send(t, otherCommenter, http.MethodPost, proposal+"/discard", "", http.StatusForbidden)); code != "forbidden_role" {
		t.Fatalf("other commenter discard code = %s", code)
	}

	d.proposeOverHTTP(t, commenter, id, 1, changed, panel{id: "b", query: "node_load1"})
	stale := decode[struct {
		Code     string `json:"code"`
		Revision int    `json:"revision"`
	}](t, d.send(t, editor, http.MethodPost, proposal+"/merge", `{"revision":1}`, http.StatusConflict))
	if stale.Code != "proposal_changed" || stale.Revision != 2 {
		t.Fatalf("stale = %+v", stale)
	}

	d.addVersion(t, id, 1, panel{id: "a", query: "node_load15"}, second)
	conflict := decode[struct {
		Code          string `json:"code"`
		LatestVersion int    `json:"latest_version"`
		Conflicts     []struct {
			ID string `json:"id"`
		} `json:"conflicts"`
	}](t, d.send(t, editor, http.MethodPost, proposal+"/merge", `{"revision":2}`, http.StatusConflict))
	if conflict.Code != "version_conflict" || conflict.LatestVersion != 2 || len(conflict.Conflicts) != 1 || conflict.Conflicts[0].ID != "a" {
		t.Fatalf("conflict = %+v", conflict)
	}
	listed := decode[ProposalJSON](t, d.send(t, editor, http.MethodGet, proposal, "", http.StatusOK))
	if len(listed.Conflicts) != 1 {
		t.Fatalf("conflicts = %+v", listed.Conflicts)
	}
	d.send(t, editor, http.MethodPost, proposal+"/merge", `{"revision":2,"choices":[{"kind":"panel","id":"a","take":"theirs"}]}`, http.StatusOK)

	closed := decode[struct {
		Code           string   `json:"code"`
		State          string   `json:"state"`
		AllowedActions []string `json:"allowed_actions"`
	}](t, d.send(t, editor, http.MethodPost, proposal+"/merge", `{"revision":2}`, http.StatusConflict))
	if closed.Code != "invalid_transition" || closed.State != "merged" || len(closed.AllowedActions) != 0 {
		t.Fatalf("closed = %+v", closed)
	}
	d.send(t, editor, http.MethodPost, proposal+"/discard", "", http.StatusConflict)
}
