package proxyhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/observations"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

type fakePanels struct {
	mu     sync.Mutex
	spec   *spec.Dashboard
	folder folders.Folder
	err    error
	calls  int
	drafts []draftCall
}

type draftCall struct {
	observation, block pgtype.UUID
	panel              json.RawMessage
}

func (f *fakePanels) DraftPanelData(_ context.Context, _ auth.Principal, observationID, blockID pgtype.UUID, panel json.RawMessage) (*spec.Dashboard, folders.Folder, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.drafts = append(f.drafts, draftCall{observation: observationID, block: blockID, panel: panel})
	if f.err != nil {
		return nil, folders.Folder{}, f.err
	}
	var draft struct {
		Queries []struct {
			Query string `json:"query"`
		} `json:"queries"`
	}
	if err := json.Unmarshal(panel, &draft); err != nil || len(draft.Queries) == 0 {
		return nil, folders.Folder{}, &observations.InvalidInputError{Code: observations.CodeInvalidPanel, Message: "the panel is invalid"}
	}
	d := f.spec
	if d == nil {
		d = blockDashboard(draft.Queries[0].Query)
	} else {
		copied := *d
		copied.Panels = slices.Clone(d.Panels)
		copied.Panels[0].Queries = []spec.Query{{Query: draft.Queries[0].Query}}
		d = &copied
	}
	return d, f.folder, nil
}

func (f *fakePanels) draftCalls() []draftCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.drafts)
}

func blockPreview(router *gin.Engine, observation string, body any, kind auth.PrincipalKind, role auth.Role) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/observations/"+observation+"/blocks/preview", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Kind", string(kind))
	req.Header.Set("X-Role", string(role))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func draftBody(query string, extra map[string]any) map[string]any {
	body := map[string]any{
		"panel": map[string]any{"type": "timeseries", "title": "5xx", "datasource": "prom", "queries": []map[string]any{{"query": query}}},
		"from":  "1760000000123",
		"to":    "1760021600456",
	}
	for k, v := range extra {
		body[k] = v
	}
	return body
}

func (f *fakePanels) PanelData(context.Context, auth.Principal, pgtype.UUID) (*spec.Dashboard, folders.Folder, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.spec, f.folder, f.err
}

func (f *fakePanels) lookups() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func blockDashboard(query string) *spec.Dashboard {
	return &spec.Dashboard{
		SpecVersion: spec.Version,
		Title:       "Checkout 5xx",
		Time:        spec.TimeRange{From: "2025-10-09T08:00:00Z", To: "2025-10-09T14:00:00Z"},
		Panels: []spec.Panel{
			{ID: observations.BlockPanelID, Type: spec.Timeseries, Title: "5xx", Datasource: "prom", GridPos: spec.GridPos{W: 12, H: 8}, Queries: []spec.Query{{Query: query}}},
		},
	}
}

func (hn *harness) observationRouter(p ObservationPanels) *gin.Engine {
	router := gin.New()
	api := router.Group("/api/v1", func(c *gin.Context) {
		role := auth.Role(c.GetHeader("X-Role"))
		if role == "" {
			role = auth.RoleViewer
		}
		principal := auth.Principal{Kind: auth.PrincipalKind(c.GetHeader("X-Kind")), ID: "alice", Email: "alice@example.com", Name: "alice", Role: role}
		signIn(c, principal)
	})
	NewObservationData(hn.h, p).Register(auth.NewRoutes(api))
	return router
}

func TestObservationBlockDataIsForUserSessions(t *testing.T) {
	hn := newHarness(t)
	f := &fakePanels{spec: blockDashboard("up")}
	router := hn.observationRouter(f)
	path := "/api/v1/blocks/" + hn.id.String() + "/data?" + window
	for _, kind := range []auth.PrincipalKind{auth.KindServiceAccount, ""} {
		expectError(t, findingGet(router, path, kind), http.StatusForbidden, "user_session_required")
	}
	if f.lookups() != 0 || hn.heldSlots() != 0 {
		t.Fatalf("refused calls read the block %d times and hold %d slots", f.lookups(), hn.heldSlots())
	}
	for range 2 {
		if body := decodeData(t, findingGet(router, path, auth.KindUser)); body.Meta.Cached || len(body.Frames) == 0 {
			t.Fatalf("block data %+v", body.Meta)
		}
	}
	if got := hn.ds.lastQuery().Text; got != "up" {
		t.Errorf("ran %q", got)
	}
}

func TestObservationBlockDataDefaultsToTheBlocksRange(t *testing.T) {
	hn := newHarness(t)
	router := hn.observationRouter(&fakePanels{spec: blockDashboard("up")})
	decodeData(t, findingGet(router, "/api/v1/blocks/"+hn.id.String()+"/data", auth.KindUser))
	want := time.Date(2025, 10, 9, 8, 0, 0, 0, time.UTC)
	hn.ds.mu.Lock()
	got := hn.ds.rangeRange
	hn.ds.mu.Unlock()
	if d := got.From.Sub(want); d < -time.Minute || d > time.Minute || got.To.Sub(got.From) < 5*time.Hour {
		t.Errorf("ran %v to %v", got.From, got.To)
	}
}

func TestObservationBlockDataRefusals(t *testing.T) {
	hn := newHarness(t)
	hidden := hn.observationRouter(&fakePanels{err: observations.ErrNotFound})
	visible := hn.observationRouter(&fakePanels{spec: blockDashboard("up")})
	cases := []struct {
		name   string
		router *gin.Engine
		path   string
		status int
		code   string
	}{
		{"an invisible or non-panel block", hidden, "/api/v1/blocks/" + hn.id.String() + "/data?" + window, http.StatusNotFound, "not_found"},
		{"a bad id", visible, "/api/v1/blocks/nope/data?" + window, http.StatusBadRequest, "invalid_request"},
		{"query text", visible, "/api/v1/blocks/" + hn.id.String() + "/data?query=up&" + window, http.StatusForbidden, "query_not_in_spec"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			expectError(t, findingGet(c.router, c.path, auth.KindUser), c.status, c.code)
		})
	}
	if hn.heldSlots() != 0 {
		t.Fatalf("%d slots held after the refusals", hn.heldSlots())
	}
}

func TestObservationBlockDataRunsWithTheVariablesBakedIntoTheBlock(t *testing.T) {
	hn := newHarness(t)
	d := blockDashboard(`sum(rate(errors{service=~"$service"}[5m]))`)
	d.Variables = []spec.Variable{{
		Name:    "service",
		Type:    spec.CustomVariable,
		Multi:   true,
		Options: []spec.VariableOption{{Text: "checkout", Value: "checkout"}, {Text: "cart", Value: "cart"}},
		Default: &spec.Selection{Values: []string{"checkout", "cart"}, List: true},
	}}
	router := hn.observationRouter(&fakePanels{spec: d})
	decodeData(t, findingGet(router, "/api/v1/blocks/"+hn.id.String()+"/data?"+window, auth.KindUser))
	query := hn.ds.lastQuery()
	if query.Text != `sum(rate(errors{service=~"$service"}[5m]))` || len(query.Variables) != 1 {
		t.Fatalf("ran %+v", query)
	}
	if v := query.Variables[0]; v.Name != "service" || !slices.Equal(v.Values, []string{"checkout", "cart"}) {
		t.Errorf("variable %+v", v)
	}
}

func TestAnAdminReadsAPanelBlockInASharedFolderAsEveryoneDoes(t *testing.T) {
	hn, audit := adminsOnlyHarness(t)
	f := &fakePanels{spec: &spec.Dashboard{
		SpecVersion: spec.Version,
		Title:       "Checkout 5xx",
		Time:        spec.TimeRange{From: "2025-10-09T08:00:00Z", To: "2025-10-09T14:00:00Z"},
		Variables:   []spec.Variable{{Name: "ds", Type: spec.DatasourceVariable, DatasourceType: spec.Prometheus}},
		Panels: []spec.Panel{
			{ID: observations.BlockPanelID, Type: spec.Table, Title: "picked", Datasource: "${ds}", GridPos: spec.GridPos{W: 12, H: 8}, Queries: []spec.Query{{Query: "count(up)"}}},
		},
	}}
	router := hn.observationRouter(f)
	path := "/api/v1/blocks/" + hn.id.String() + "/data?" + window
	read := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("X-Kind", string(auth.KindUser))
		req.Header.Set("X-Role", string(auth.RoleAdmin))
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	if rec := read(); rec.Code != http.StatusOK {
		t.Fatalf("shared folder: status %d %s", rec.Code, rec.Body)
	}
	if ranges, instants, _, _ := audit.counts(); ranges+instants != 0 {
		t.Fatal("an Admin's read of a panel block in a shared folder reached the admins only datasource")
	}
	f.folder.AdminsOnly = true
	if rec := read(); rec.Code != http.StatusOK {
		t.Fatalf("admins only folder: status %d %s", rec.Code, rec.Body)
	}
	if ranges, instants, _, _ := audit.counts(); ranges+instants == 0 {
		t.Fatal("an Admin's read of a panel block in an Admins-only folder never reached the admins only datasource")
	}
}

func TestObservationBlockPreviewIsForUserSessions(t *testing.T) {
	hn := newHarness(t)
	f := &fakePanels{}
	router := hn.observationRouter(f)
	for _, kind := range []auth.PrincipalKind{auth.KindServiceAccount, ""} {
		expectError(t, blockPreview(router, hn.id.String(), draftBody("up", nil), kind, auth.RoleAdmin), http.StatusForbidden, "user_session_required")
	}
	if f.lookups() != 0 || hn.heldSlots() != 0 {
		t.Fatalf("refused calls read the observation %d times and hold %d slots", f.lookups(), hn.heldSlots())
	}
	for range 2 {
		if body := decodeData(t, blockPreview(router, hn.id.String(), draftBody("rate(errors[5m])", nil), auth.KindUser, auth.RoleViewer)); body.Meta.Cached || len(body.Frames) == 0 {
			t.Fatalf("draft data %+v", body.Meta)
		}
	}
	if got := hn.ds.lastQuery().Text; got != "rate(errors[5m])" {
		t.Errorf("ran %q", got)
	}
	if hn.heldSlots() != 0 {
		t.Fatalf("%d slots held after the previews", hn.heldSlots())
	}
}

func TestObservationBlockPreviewPassesTheObservationBlockAndDraft(t *testing.T) {
	hn := newHarness(t)
	f := &fakePanels{}
	router := hn.observationRouter(f)
	block := pgtype.UUID{Bytes: [16]byte{9}, Valid: true}
	decodeData(t, blockPreview(router, hn.id.String(), draftBody("up", map[string]any{"block_id": block.String()}), auth.KindUser, auth.RoleViewer))
	decodeData(t, blockPreview(router, hn.id.String(), draftBody("up", nil), auth.KindUser, auth.RoleViewer))
	calls := f.draftCalls()
	if len(calls) != 2 {
		t.Fatalf("%d draft lookups, want 2", len(calls))
	}
	if calls[0].observation != hn.id || calls[0].block != block {
		t.Errorf("edit draft looked up %v / %v", calls[0].observation, calls[0].block)
	}
	if calls[1].block.Valid {
		t.Errorf("a new query named block %v", calls[1].block)
	}
	if !bytes.Contains(calls[0].panel, []byte(`"query":"up"`)) {
		t.Errorf("draft panel %s", calls[0].panel)
	}
}

func TestObservationBlockPreviewRefusals(t *testing.T) {
	hn := newHarness(t)
	visible := hn.observationRouter(&fakePanels{})
	cases := []struct {
		name        string
		router      *gin.Engine
		observation string
		body        any
		status      int
		code        string
	}{
		{"an invisible observation or block", hn.observationRouter(&fakePanels{err: observations.ErrNotFound}), hn.id.String(), draftBody("up", nil), http.StatusNotFound, "not_found"},
		{"a viewer who can neither edit nor comment", hn.observationRouter(&fakePanels{err: folders.ErrForbidden}), hn.id.String(), draftBody("up", nil), http.StatusForbidden, "forbidden_role"},
		{"an invalid draft panel", visible, hn.id.String(), map[string]any{"panel": map[string]any{"type": "timeseries"}}, http.StatusUnprocessableEntity, observations.CodeInvalidPanel},
		{"a bad observation id", visible, "nope", draftBody("up", nil), http.StatusBadRequest, "invalid_request"},
		{"a bad block id", visible, hn.id.String(), draftBody("up", map[string]any{"block_id": "nope"}), http.StatusBadRequest, "invalid_request"},
		{"a panel that is not an object", visible, hn.id.String(), map[string]any{"panel": "up"}, http.StatusBadRequest, "invalid_request"},
		{"an unknown field", visible, hn.id.String(), draftBody("up", map[string]any{"query": "up"}), http.StatusBadRequest, "invalid_request"},
		{"an empty variable", visible, hn.id.String(), draftBody("up", map[string]any{"variables": map[string][]string{"service": {}}}), http.StatusBadRequest, "invalid_request"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			expectError(t, blockPreview(c.router, c.observation, c.body, auth.KindUser, auth.RoleViewer), c.status, c.code)
		})
	}
	if hn.heldSlots() != 0 {
		t.Fatalf("%d slots held after the refusals", hn.heldSlots())
	}
}

func TestAnAdminPreviewsAPanelBlockInASharedFolderAsEveryoneDoes(t *testing.T) {
	hn, audit := adminsOnlyHarness(t)
	f := &fakePanels{spec: &spec.Dashboard{
		SpecVersion: spec.Version,
		Title:       "Checkout 5xx",
		Time:        spec.TimeRange{From: "2025-10-09T08:00:00Z", To: "2025-10-09T14:00:00Z"},
		Variables:   []spec.Variable{{Name: "ds", Type: spec.DatasourceVariable, DatasourceType: spec.Prometheus}},
		Panels: []spec.Panel{
			{ID: observations.BlockPanelID, Type: spec.Table, Title: "picked", Datasource: "${ds}", GridPos: spec.GridPos{W: 12, H: 8}, Queries: []spec.Query{{Query: "count(up)"}}},
		},
	}}
	router := hn.observationRouter(f)
	preview := func() *httptest.ResponseRecorder {
		return blockPreview(router, hn.id.String(), draftBody("count(up)", nil), auth.KindUser, auth.RoleAdmin)
	}
	if rec := preview(); rec.Code != http.StatusOK {
		t.Fatalf("shared folder: status %d %s", rec.Code, rec.Body)
	}
	if ranges, instants, _, _ := audit.counts(); ranges+instants != 0 {
		t.Fatal("an Admin's draft of a panel block in a shared folder reached the admins only datasource")
	}
	f.folder.AdminsOnly = true
	if rec := preview(); rec.Code != http.StatusOK {
		t.Fatalf("admins only folder: status %d %s", rec.Code, rec.Body)
	}
	if ranges, instants, _, _ := audit.counts(); ranges+instants == 0 {
		t.Fatal("an Admin's draft of a panel block in an Admins-only folder never reached the admins only datasource")
	}
}
