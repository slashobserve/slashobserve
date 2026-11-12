package observations_test

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
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/observations"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
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
	observations.NewHandler(d.service, slog.New(slog.DiscardHandler)).Register(auth.NewRoutes(d.engine.Group("/api/v1", func(c *gin.Context) {
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

func (d *documented) call(t *testing.T, by auth.Principal, method, path string, want int) *httptest.ResponseRecorder {
	t.Helper()
	return d.send(t, by, method, path, "", want)
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

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %s: %v", rec.Body, err)
	}
	return v
}

type envelope struct {
	Observation observations.ObservationJSON `json:"observation"`
	Folder      struct {
		ID       string `json:"id"`
		Personal bool   `json:"personal"`
	} `json:"folder"`
}

type blockEnvelope struct {
	Block observations.BlockJSON `json:"block"`
}

func (d *documented) newObservation(t *testing.T, by auth.Principal, folder string, extra string) envelope {
	t.Helper()
	rec := d.send(t, by, http.MethodPost, "/api/v1/observations", fmt.Sprintf(`{"title":"Checkout 5xx","folder_id":%q%s}`, folder, extra), http.StatusCreated)
	return decode[envelope](t, rec)
}

func (d *documented) blockPath(id string) string {
	return "/api/v1/blocks/" + id
}

func TestObservationRoutesMatchTheDocument(t *testing.T) {
	d := newDocumented(t)
	shared := d.shared.String()
	other := d.folder(t, "Postmortems").String()

	created := d.newObservation(t, editor, shared, `,"from":"2026-11-01T08:00:00Z","to":"2026-11-01T10:00:00Z","first_block":{"kind":"text","body":{"markdown":"Started after the deploy"}}`)
	o := created.Observation
	if o.State != "open" || o.URL != "/o/"+o.ID || o.ThreadID != nil || o.Blocks == nil || len(*o.Blocks) != 1 {
		t.Fatalf("created %+v", o)
	}
	base := "/api/v1/observations/" + o.ID

	got := decode[envelope](t, d.call(t, viewer, http.MethodGet, base, http.StatusOK))
	if got.Observation.Blocks == nil || len(*got.Observation.Blocks) != 1 || len(got.Observation.AllowedActions) != 0 {
		t.Fatalf("a viewer reads %+v", got.Observation)
	}

	list := decode[struct {
		Observations []observations.ObservationJSON `json:"observations"`
		NextCursor   *string                        `json:"next_cursor"`
	}](t, d.call(t, viewer, http.MethodGet, "/api/v1/observations?folder_id="+shared+"&state=open&limit=1", http.StatusOK))
	if len(list.Observations) != 1 || list.Observations[0].Blocks != nil || list.NextCursor != nil {
		t.Fatalf("list %+v", list)
	}
	d.newObservation(t, editor, shared, "")
	page := decode[struct {
		NextCursor *string `json:"next_cursor"`
	}](t, d.call(t, viewer, http.MethodGet, "/api/v1/observations?limit=1", http.StatusOK))
	if page.NextCursor == nil {
		t.Fatal("the first page has no cursor")
	}
	d.call(t, viewer, http.MethodGet, "/api/v1/observations?limit=1&cursor="+*page.NextCursor, http.StatusOK)

	d.send(t, editor, http.MethodPatch, base, `{"title":"Checkout 5xx after deploy"}`, http.StatusOK)
	d.send(t, editor, http.MethodPatch, base, `{"from":null,"to":null}`, http.StatusOK)
	d.send(t, editor, http.MethodPatch, base, `{"from":"2026-11-01T07:00:00Z","to":"2026-11-01T09:00:00Z"}`, http.StatusOK)

	text := decode[blockEnvelope](t, d.send(t, editor, http.MethodPost, base+"/blocks", `{"kind":"text","body":{"markdown":"Rollback at 09:10"},"width":"half"}`, http.StatusCreated)).Block
	panel := decode[blockEnvelope](t, d.send(t, editor, http.MethodPost, base+"/blocks", `{"kind":"panel","body":{"panel":{"type":"timeseries","title":"Rate","datasource":"prom","queries":[{"ref":"A","query":"sum(rate(http_requests_total[5m]))"}]}},"position":0}`, http.StatusCreated)).Block
	if panel.Position != 0 || text.Width != "half" || len(text.AllowedActions) == 0 {
		t.Fatalf("panel %+v text %+v", panel, text)
	}
	snapshot := decode[blockEnvelope](t, d.send(t, editor, http.MethodPost, base+"/blocks", fmt.Sprintf(`{"kind":"snapshot","from_block_id":%q}`, panel.ID), http.StatusCreated)).Block
	if snapshot.SnapshotID == nil {
		t.Fatalf("snapshot block %+v", snapshot)
	}
	frozen := decode[observations.SnapshotJSON](t, d.call(t, viewer, http.MethodGet, "/api/v1/snapshots/"+*snapshot.SnapshotID, http.StatusOK))
	if frozen.ObservationID != o.ID || len(frozen.Data) == 0 || len(frozen.Query) == 0 {
		t.Fatalf("snapshot %+v", frozen)
	}

	changed := decode[blockEnvelope](t, d.send(t, editor, http.MethodPatch, d.blockPath(text.ID), fmt.Sprintf(`{"version":%d,"body":{"markdown":"Rollback finished at 09:10"},"width":"full"}`, text.Version), http.StatusOK)).Block
	if changed.Version != text.Version+1 || changed.Width != "full" {
		t.Fatalf("changed %+v", changed)
	}

	asked := decode[blockEnvelope](t, d.send(t, commenter, http.MethodPost, base+"/blocks", `{"kind":"text","body":{"markdown":"Was the cache cold?"}}`, http.StatusCreated)).Block
	if asked.State != "requested" {
		t.Fatalf("requested block %+v", asked)
	}
	kept := decode[blockEnvelope](t, d.send(t, editor, http.MethodPost, d.blockPath(asked.ID)+"/keep", fmt.Sprintf(`{"version":%d}`, asked.Version), http.StatusOK)).Block
	if kept.State != "live" {
		t.Fatalf("kept block %+v", kept)
	}
	declined := decode[blockEnvelope](t, d.send(t, otherCommen, http.MethodPost, base+"/blocks", `{"kind":"text","body":{"markdown":"Any alert?"}}`, http.StatusCreated)).Block
	d.call(t, otherCommen, http.MethodPost, d.blockPath(declined.ID)+"/discard", http.StatusNoContent)
	d.call(t, editor, http.MethodDelete, fmt.Sprintf("%s?version=%d", d.blockPath(kept.ID), kept.Version), http.StatusNoContent)

	resolved := decode[envelope](t, d.send(t, editor, http.MethodPost, base+"/resolve", `{"summary":"A bad deploy; rolled back"}`, http.StatusOK)).Observation
	if resolved.State != "resolved" || resolved.Summary == nil || resolved.ResolvedAt == nil {
		t.Fatalf("resolved %+v", resolved)
	}
	reopened := decode[envelope](t, d.call(t, editor, http.MethodPost, base+"/reopen", http.StatusOK)).Observation
	if reopened.State != "open" {
		t.Fatalf("reopened %+v", reopened)
	}
	moved := decode[envelope](t, d.send(t, editor, http.MethodPost, base+"/move", fmt.Sprintf(`{"folder_id":%q}`, other), http.StatusOK))
	if moved.Observation.FolderID != other || moved.Folder.ID != other {
		t.Fatalf("moved %+v", moved)
	}
}

func TestObservationRoutesAnswerEveryDocumentedError(t *testing.T) {
	d := newDocumented(t)
	shared := d.shared.String()
	hidden := d.hiddenFolder(t).String()
	personal := d.personalFolder(t, editor).String()
	o := d.newObservation(t, editor, shared, "").Observation
	base := "/api/v1/observations/" + o.ID
	text := decode[blockEnvelope](t, d.send(t, editor, http.MethodPost, base+"/blocks", `{"kind":"text","body":{"markdown":"words"}}`, http.StatusCreated)).Block
	panel := decode[blockEnvelope](t, d.send(t, editor, http.MethodPost, base+"/blocks", `{"kind":"panel","body":{"panel":{"type":"timeseries","title":"Rate","datasource":"prom","queries":[{"ref":"A","query":"up"}]}}}`, http.StatusCreated)).Block
	asked := decode[blockEnvelope](t, d.send(t, commenter, http.MethodPost, base+"/blocks", `{"kind":"text","body":{"markdown":"why?"}}`, http.StatusCreated)).Block
	missing := "00000000-0000-4000-8000-000000000001"
	oversize := `{"frames":["` + strings.Repeat("x", observations.MaxSnapshotBytes) + `"]}`

	cases := []struct {
		name   string
		by     auth.Principal
		method string
		path   string
		body   string
		status int
		code   string
	}{
		{"a body that is not JSON", editor, http.MethodPost, "/api/v1/observations", `{`, 400, "invalid_request"},
		{"a folder id that is not a UUID", editor, http.MethodPost, "/api/v1/observations", `{"title":"x","folder_id":"nope"}`, 400, "invalid_request"},
		{"an id that is not a UUID", viewer, http.MethodGet, "/api/v1/observations/nope", "", 400, "invalid_request"},
		{"a state filter that is unknown", viewer, http.MethodGet, "/api/v1/observations?state=lost", "", 400, "invalid_request"},
		{"a cursor that is not valid", viewer, http.MethodGet, "/api/v1/observations?cursor=%25", "", 400, "invalid_request"},
		{"an update that changes nothing", editor, http.MethodPatch, base, `{}`, 400, "invalid_request"},
		{"a range cleared on one side", editor, http.MethodPatch, base, `{"from":null,"to":"2026-11-01T09:00:00Z"}`, 422, "invalid_range"},
		{"a block update with no version", editor, http.MethodPatch, d.blockPath(text.ID), `{"width":"half"}`, 400, "invalid_request"},
		{"a block removal with no version", editor, http.MethodDelete, d.blockPath(text.ID), "", 400, "invalid_request"},
		{"a keep with no version", editor, http.MethodPost, d.blockPath(asked.ID) + "/keep", `{}`, 400, "invalid_request"},
		{"a viewer creating", viewer, http.MethodPost, "/api/v1/observations", fmt.Sprintf(`{"title":"x","folder_id":%q}`, shared), 403, "forbidden_role"},
		{"a viewer resolving", viewer, http.MethodPost, base + "/resolve", `{"summary":"done"}`, 403, "forbidden_role"},
		{"a viewer keeping", viewer, http.MethodPost, d.blockPath(asked.ID) + "/keep", fmt.Sprintf(`{"version":%d}`, asked.Version), 403, "forbidden_role"},
		{"a commenter keeping", commenter, http.MethodPost, d.blockPath(asked.ID) + "/keep", fmt.Sprintf(`{"version":%d}`, asked.Version), 409, "invalid_transition"},
		{"an observation nobody sees", editor, http.MethodGet, "/api/v1/observations/" + missing, "", 404, "not_found"},
		{"a block nobody sees", editor, http.MethodDelete, d.blockPath(missing) + "?version=1", "", 404, "not_found"},
		{"a snapshot nobody sees", viewer, http.MethodGet, "/api/v1/snapshots/" + missing, "", 404, "not_found"},
		{"a folder nobody sees", editor, http.MethodPost, "/api/v1/observations", fmt.Sprintf(`{"title":"x","folder_id":%q}`, hidden), 404, "not_found"},
		{"a move into a folder nobody sees", editor, http.MethodPost, base + "/move", fmt.Sprintf(`{"folder_id":%q}`, hidden), 404, "not_found"},
		{"a move into My Dashboards", editor, http.MethodPost, base + "/move", fmt.Sprintf(`{"folder_id":%q}`, personal), 409, "personal_folder"},
		{"a reopen of an open observation", editor, http.MethodPost, base + "/reopen", "", 409, "invalid_transition"},
		{"a keep of a live block", editor, http.MethodPost, d.blockPath(text.ID) + "/keep", fmt.Sprintf(`{"version":%d}`, text.Version), 409, "invalid_transition"},
		{"a discard of a live block", editor, http.MethodPost, d.blockPath(text.ID) + "/discard", "", 409, "invalid_transition"},
		{"a stale block update", editor, http.MethodPatch, d.blockPath(text.ID), `{"version":99,"width":"half"}`, 409, "version_conflict"},
		{"a stale block removal", editor, http.MethodDelete, d.blockPath(text.ID) + "?version=99", "", 409, "version_conflict"},
		{"a stale keep", editor, http.MethodPost, d.blockPath(asked.ID) + "/keep", `{"version":99}`, 409, "version_conflict"},
		{"a title that is empty", editor, http.MethodPost, "/api/v1/observations", fmt.Sprintf(`{"title":"  ","folder_id":%q}`, shared), 422, "invalid_title"},
		{"a range that is backwards", editor, http.MethodPatch, base, `{"from":"2026-11-01T10:00:00Z","to":"2026-11-01T08:00:00Z"}`, 422, "invalid_range"},
		{"a resolve with no summary", editor, http.MethodPost, base + "/resolve", `{"summary":""}`, 422, "invalid_summary"},
		{"a block of an unknown kind", editor, http.MethodPost, base + "/blocks", `{"kind":"chart","body":{}}`, 422, "invalid_block"},
		{"a text block with no text", editor, http.MethodPost, base + "/blocks", `{"kind":"text","body":{"markdown":""}}`, 422, "invalid_block"},
		{"a panel that fails the Spec", editor, http.MethodPost, base + "/blocks", `{"kind":"panel","body":{"panel":{"type":"timeseries","queries":[]}}}`, 422, "invalid_panel"},
		{"a run block added by hand", editor, http.MethodPost, base + "/blocks", `{"kind":"run","body":{}}`, 422, "kind_not_addable"},
		{"a snapshot of a text block", editor, http.MethodPost, base + "/blocks", fmt.Sprintf(`{"kind":"snapshot","from_block_id":%q}`, text.ID), 422, "invalid_block"},
		{"a body change on a snapshot", editor, http.MethodPatch, d.blockPath(text.ID), fmt.Sprintf(`{"version":%d,"body":{"panel":{}}}`, text.Version), 422, "invalid_block"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := d.send(t, c.by, c.method, c.path, c.body, c.status)
			if got := errorCode(t, rec); got != c.code {
				t.Errorf("code %q, want %q: %s", got, c.code, rec.Body)
			}
		})
	}

	conflict := d.send(t, editor, http.MethodPatch, d.blockPath(text.ID), `{"version":99,"width":"half"}`, http.StatusConflict)
	if current := decode[struct {
		Current observations.BlockJSON `json:"current"`
	}](t, conflict).Current; current.ID != text.ID || current.Version != text.Version {
		t.Errorf("the conflict names %+v", current)
	}
	refusal := decode[struct {
		Allowed []string `json:"allowed_actions"`
	}](t, d.call(t, editor, http.MethodPost, base+"/reopen", http.StatusConflict))
	if len(refusal.Allowed) == 0 {
		t.Error("the refusal names no allowed action")
	}

	for range observations.MaxPanelBlocks - 1 {
		d.send(t, editor, http.MethodPost, base+"/blocks", `{"kind":"panel","body":{"panel":{"type":"stat","title":"Up","datasource":"prom","queries":[{"ref":"A","query":"up"}]}}}`, http.StatusCreated)
	}
	if rec := d.send(t, editor, http.MethodPost, base+"/blocks", `{"kind":"panel","body":{"panel":{"type":"stat","title":"Up","datasource":"prom","queries":[{"ref":"A","query":"up"}]}}}`, http.StatusUnprocessableEntity); errorCode(t, rec) != "too_many_panels" {
		t.Errorf("past the panel limit: %s", rec.Body)
	}

	snapshotOf := fmt.Sprintf(`{"kind":"snapshot","from_block_id":%q}`, panel.ID)
	d.freezer.data = json.RawMessage(oversize)
	if rec := d.send(t, editor, http.MethodPost, base+"/blocks", snapshotOf, http.StatusUnprocessableEntity); errorCode(t, rec) != "snapshot_too_large" {
		t.Errorf("oversize snapshot: %s", rec.Body)
	}
	d.freezer.data = nil

	failures := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"a datasource that is busy", queryproxy.ErrUpstreamBusy, 429, "too_many_requests"},
		{"a query that is invalid", fmt.Errorf("check: %w", datasource.ErrInvalidQuery), 422, "query_invalid"},
		{"a datasource that is down", fmt.Errorf("check: %w", datasource.ErrUnavailable), 502, "datasource_unavailable"},
		{"a query that times out", fmt.Errorf("check: %w", datasource.ErrTimeout), 504, "query_timeout"},
	}
	for _, c := range failures {
		t.Run(c.name, func(t *testing.T) {
			d.freezer.err = c.err
			defer func() { d.freezer.err = nil }()
			rec := d.send(t, editor, http.MethodPost, base+"/blocks", snapshotOf, c.status)
			if got := errorCode(t, rec); got != c.code {
				t.Errorf("code %q, want %q", got, c.code)
			}
		})
	}
	d.freezer.err = fmt.Errorf("boom")
	if rec := d.send(t, editor, http.MethodPost, base+"/blocks", snapshotOf, http.StatusInternalServerError); errorCode(t, rec) != "internal" || strings.Contains(rec.Body.String(), "boom") {
		t.Errorf("an unknown failure: %s", rec.Body)
	}
	d.freezer.err = nil

}
