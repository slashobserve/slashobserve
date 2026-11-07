package proxyhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	exploreFrom = "1760000000123"
	exploreTo   = "1760000060456"
)

type instantCall struct {
	query datasource.Query
	rng   datasource.TimeRange
}

type recordingDatasource struct {
	*fakeDatasource
	mu    sync.Mutex
	calls []instantCall
}

func (r *recordingDatasource) QueryInstant(ctx context.Context, q datasource.Query, rng datasource.TimeRange) ([]datasource.Frame, error) {
	r.mu.Lock()
	r.calls = append(r.calls, instantCall{query: q, rng: rng})
	r.mu.Unlock()
	return r.fakeDatasource.QueryInstant(ctx, q, rng)
}

func (r *recordingDatasource) instants() []instantCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]instantCall(nil), r.calls...)
}

func newExploreHarness(t *testing.T) (*harness, *recordingDatasource) {
	t.Helper()
	hn := newHarness(t)
	recorder := &recordingDatasource{fakeDatasource: hn.ds}
	registry, err := datasource.NewRegistry(hn.h.logger, datasource.Instance{UID: "prom", Datasource: recorder})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	hn.runner = queryproxy.NewRunner(registry, hn.h.logger)
	hn.h.runner, hn.h.registry = hn.runner, registry
	return hn, recorder
}

func (hn *harness) explore(kind auth.PrincipalKind, uid string, body map[string]any) *httptest.ResponseRecorder {
	hn.t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		hn.t.Fatalf("marshal explore body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/datasources/"+uid+"/query", bytes.NewReader(encoded))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Kind", string(kind))
	return hn.serve(req, "alice")
}

func instantBody(query string) map[string]any {
	return map[string]any{"query": query, "kind": "instant", "from": exploreFrom, "to": exploreTo}
}

func TestExploreRunsAnInstantQueryOverTheExactRangeWithDraftLimits(t *testing.T) {
	hn, recorder := newExploreHarness(t)
	hn.ds.onInstant = func(context.Context, datasource.TimeRange) ([]datasource.Frame, error) {
		return logFrames(), nil
	}
	body := decodeData(t, hn.explore(auth.KindUser, "prom", instantBody(`{service_name="checkout"}`)))
	if body.Meta.Kind != queryproxy.KindInstant {
		t.Fatalf("kind = %q, want instant", body.Meta.Kind)
	}
	if body.Meta.Truncated || body.Meta.Partial || body.Meta.RowsTotal != 0 {
		t.Fatalf("meta = %+v, want neither truncated nor partial and no rows_total", body.Meta)
	}
	if body.Meta.Rows != 3 || body.Meta.Oldest != "2026-11-06T11:59:58.5Z" || body.Meta.Newest != "2026-11-06T12:00:00.000000001Z" {
		t.Fatalf("meta = %+v, want 3 rows between 11:59:58.5 and 12:00:00.000000001", body.Meta)
	}
	calls := recorder.instants()
	if len(calls) != 1 {
		t.Fatalf("%d instant calls, want 1", len(calls))
	}
	got := calls[0]
	if got.query.Text != `{service_name="checkout"}` {
		t.Fatalf("query text = %q", got.query.Text)
	}
	if got.query.Limits != queryproxy.DraftLimits {
		t.Fatalf("limits = %+v, want %+v", got.query.Limits, queryproxy.DraftLimits)
	}
	if got.rng.From.UnixMilli() != 1760000000123 || got.rng.To.UnixMilli() != 1760000060456 {
		t.Fatalf("range = %d to %d, want the exact bounds", got.rng.From.UnixMilli(), got.rng.To.UnixMilli())
	}
	if ranges, _, _, _ := hn.ds.counts(); ranges != 0 {
		t.Fatalf("%d range calls for an instant query", ranges)
	}
}

func TestExploreLimitLowersOnlyTheRowLimit(t *testing.T) {
	hn, recorder := newExploreHarness(t)
	body := instantBody("up")
	body["limit"] = 50
	decodeData(t, hn.explore(auth.KindUser, "prom", body))
	want := queryproxy.DraftLimits
	want.MaxRows = 50
	if got := recorder.instants()[0].query.Limits; got != want {
		t.Fatalf("limits = %+v, want %+v", got, want)
	}
}

func TestExploreRunsARangeQueryWithAStepOverAnAlignedRange(t *testing.T) {
	hn, _ := newExploreHarness(t)
	body := decodeData(t, hn.explore(auth.KindUser, "prom", map[string]any{
		"query": "rate(http_requests_total[5m])", "kind": "range", "from": "1760000000123", "to": "1760021600456", "max_data_points": 500,
	}))
	if body.Meta.Kind != queryproxy.KindRange || body.Meta.StepMS <= 0 {
		t.Fatalf("meta = %+v, want a range with a step", body.Meta)
	}
	step := hn.ds.stepFor("rate(http_requests_total[5m])")
	if step <= 0 {
		t.Fatalf("step = %v, want a positive step", step)
	}
	if step.Milliseconds() != body.Meta.StepMS {
		t.Fatalf("upstream step %v, response step %d ms", step, body.Meta.StepMS)
	}
	hn.ds.mu.Lock()
	rng := hn.ds.rangeRange
	hn.ds.mu.Unlock()
	if rng.From.UnixMilli()%step.Milliseconds() != 0 || rng.To.UnixMilli()%step.Milliseconds() != 0 {
		t.Fatalf("range %v to %v is not aligned to %v", rng.From, rng.To, step)
	}
	if ranges, instants, _, _ := hn.ds.counts(); ranges != 1 || instants != 0 {
		t.Fatalf("calls = %d range, %d instant; want 1 range", ranges, instants)
	}
}

func TestExploreIsNeverCachedOrCoalesced(t *testing.T) {
	hn, recorder := newExploreHarness(t)
	for i := range 2 {
		if body := decodeData(t, hn.explore(auth.KindUser, "prom", instantBody("up"))); body.Meta.Cached {
			t.Fatalf("request %d reports cached", i+1)
		}
	}
	if calls := len(recorder.instants()); calls != 2 {
		t.Fatalf("%d upstream calls for 2 identical requests, want 2", calls)
	}
}

func TestExploreAcceptsUsersAndServiceAccountKeys(t *testing.T) {
	hn, _ := newExploreHarness(t)
	for _, kind := range []auth.PrincipalKind{auth.KindUser, auth.KindServiceAccount} {
		if rec := hn.explore(kind, "prom", instantBody("up")); rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, body %s", kind, rec.Code, rec.Body.String())
		}
	}
}

func TestExploreAnswersNotFoundForAnUnknownDatasource(t *testing.T) {
	hn, recorder := newExploreHarness(t)
	expectError(t, hn.explore(auth.KindUser, "missing", instantBody("up")), http.StatusNotFound, "not_found")
	if calls := len(recorder.instants()); calls != 0 {
		t.Fatalf("%d upstream calls for an unknown datasource", calls)
	}
	if held := hn.heldSlots(); held != 0 {
		t.Fatalf("%d slots still held", held)
	}
}

func TestExploreRejectsInvalidRequests(t *testing.T) {
	hn, recorder := newExploreHarness(t)
	with := func(edit func(map[string]any)) map[string]any {
		body := instantBody("up")
		edit(body)
		return body
	}
	tests := []struct {
		name string
		body map[string]any
		code string
	}{
		{"missing query", with(func(b map[string]any) { delete(b, "query") }), "invalid_request"},
		{"empty query", with(func(b map[string]any) { b["query"] = "" }), "invalid_request"},
		{"query over 64 KiB", with(func(b map[string]any) { b["query"] = strings.Repeat("x", 64<<10+1) }), "query_too_long"},
		{"unknown kind", with(func(b map[string]any) { b["kind"] = "table" }), "invalid_request"},
		{"missing to", with(func(b map[string]any) { delete(b, "to") }), "invalid_request"},
		{"from after to", with(func(b map[string]any) { b["from"], b["to"] = exploreTo, exploreFrom }), "invalid_request"},
		{"limit zero", with(func(b map[string]any) { b["limit"] = 0 }), "invalid_request"},
		{"limit over the row limit", with(func(b map[string]any) { b["limit"] = 1001 }), "invalid_request"},
		{"max_data_points on an instant query", with(func(b map[string]any) { b["max_data_points"] = 100 }), "invalid_request"},
		{"unknown field", with(func(b map[string]any) { b["expr"] = "up" }), "invalid_request"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expectError(t, hn.explore(auth.KindUser, "prom", tt.body), http.StatusBadRequest, tt.code)
		})
	}
	if calls := len(recorder.instants()); calls != 0 {
		t.Fatalf("%d upstream calls for invalid requests", calls)
	}
}

func logFrames() []datasource.Frame {
	at := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	return []datasource.Frame{
		{Labels: map[string]string{"service_name": "checkout"}, Fields: []datasource.Field{
			{Name: "Time", Type: datasource.FieldTime, Values: []any{at.Add(time.Nanosecond), at.Add(-1500 * time.Millisecond)}},
			{Name: "Body", Type: datasource.FieldString, Values: []any{"retrying", "payment failed"}},
		}},
		{Labels: map[string]string{"service_name": "frontend"}, Fields: []datasource.Field{
			{Name: "Time", Type: datasource.FieldTime, Values: []any{at.Add(-time.Second)}},
			{Name: "Body", Type: datasource.FieldString, Values: []any{"slow"}},
		}},
	}
}

func TestExploreCountsNoRowsAndNoEdgesForSeries(t *testing.T) {
	hn, _ := newExploreHarness(t)
	body := decodeData(t, hn.explore(auth.KindUser, "prom", instantBody("up")))
	if body.Meta.Rows != 0 {
		t.Fatalf("rows = %d for series data, want 0", body.Meta.Rows)
	}
	if body.Meta.Oldest != "2025-10-09T08:53:20Z" || body.Meta.Newest != body.Meta.Oldest {
		t.Fatalf("edges = %q to %q, want the one sample time", body.Meta.Oldest, body.Meta.Newest)
	}
	hn.ds.onInstant = func(context.Context, datasource.TimeRange) ([]datasource.Frame, error) {
		return []datasource.Frame{{Fields: []datasource.Field{{Name: "name", Type: datasource.FieldString, Values: []any{"a"}}}}}, nil
	}
	body = decodeData(t, hn.explore(auth.KindUser, "prom", instantBody("names")))
	if body.Meta.Rows != 1 || body.Meta.Oldest != "" || body.Meta.Newest != "" {
		t.Fatalf("meta = %+v, want 1 row and no edges without a time field", body.Meta)
	}
}

func TestExploreReportsTruncationFromTheDatasource(t *testing.T) {
	hn, _ := newExploreHarness(t)
	hn.ds.onInstant = func(context.Context, datasource.TimeRange) ([]datasource.Frame, error) {
		frames := logFrames()
		frames[1].Meta = &datasource.FrameMeta{Truncated: true, Partial: true, RowsTotal: 120}
		return frames, nil
	}
	body := decodeData(t, hn.explore(auth.KindUser, "prom", instantBody(`{service_name="checkout"}`)))
	if !body.Meta.Truncated || !body.Meta.Partial || body.Meta.RowsTotal != 120 || body.Meta.Rows != 3 {
		t.Fatalf("meta = %+v, want truncated, partial, rows_total 120 and 3 rows", body.Meta)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(hn.explore(auth.KindUser, "prom", instantBody("up")).Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw["frames"]), "meta") {
		t.Fatalf("frames carry per-frame meta on the wire: %s", raw["frames"])
	}
}

func TestExploreReportsAQueryErrorWithoutARef(t *testing.T) {
	hn, _ := newExploreHarness(t)
	hn.ds.onInstant = func(context.Context, datasource.TimeRange) ([]datasource.Frame, error) {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "parse error at line 1")
	}
	rec := hn.explore(auth.KindUser, "prom", instantBody("{"))
	expectError(t, rec, http.StatusUnprocessableEntity, "query_invalid")
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if _, hasRef := body["ref"]; hasRef || !strings.Contains(body["error"].(string), "parse error at line 1") {
		t.Fatalf("error = %v, want the datasource message and no ref", body)
	}
}

func TestExploreAnswersTooManyRequestsWhenEverySlotIsHeld(t *testing.T) {
	hn, recorder := newExploreHarness(t)
	holdPrincipalSlots(t, hn, "alice")
	rec := hn.explore(auth.KindUser, "prom", instantBody("up"))
	expectError(t, rec, http.StatusTooManyRequests, "too_many_requests")
	if got := rec.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q, want 1", got)
	}
	if calls := len(recorder.instants()); calls != 0 {
		t.Fatalf("%d upstream calls while every slot is held", calls)
	}
}

func TestExploreLogsAnUpstreamFailureWithoutTheQueryText(t *testing.T) {
	hn, _ := newExploreHarness(t)
	hn.ds.onInstant = func(context.Context, datasource.TimeRange) ([]datasource.Frame, error) {
		return nil, datasource.Errorf(datasource.ErrUnavailable, "connection refused")
	}
	text := `{service_name="secret-checkout-query"}`
	expectError(t, hn.explore(auth.KindUser, "prom", instantBody(text)), http.StatusBadGateway, "datasource_unavailable")
	logs := hn.logs.String()
	if n := strings.Count(logs, "explore query failed"); n != 1 {
		t.Fatalf("%d explore failure lines, want 1:\n%s", n, logs)
	}
	if strings.Contains(logs, "secret-checkout-query") {
		t.Fatalf("the log holds the query text:\n%s", logs)
	}
}

func TestExploreMetaCarriesJobsAndElapsed(t *testing.T) {
	hn, _ := newExploreHarness(t)
	hn.ds.onInstant = func(context.Context, datasource.TimeRange) ([]datasource.Frame, error) {
		time.Sleep(2 * time.Millisecond)
		frames := logFrames()
		frames[0].Meta = &datasource.FrameMeta{JobsDone: 1, JobsTotal: 6, InspectedBytes: 10, Partial: true}
		return frames, nil
	}
	body := decodeData(t, hn.explore(auth.KindUser, "prom", instantBody(`{}`)))
	meta := body.Meta
	if meta.JobsDone != 1 || meta.JobsTotal != 6 || meta.InspectedBytes != 10 || !meta.Partial || meta.Bytes != 0 {
		t.Fatalf("meta = %+v, want 1 of 6 jobs, 10 inspected bytes, partial and no payload size", meta)
	}
	if meta.ElapsedMS < 2 {
		t.Fatalf("elapsed_ms = %d, want at least the time the datasource took", meta.ElapsedMS)
	}
	hn.ds.onInstant = nil
	var raw struct {
		Meta map[string]json.RawMessage `json:"meta"`
	}
	if err := json.Unmarshal(hn.explore(auth.KindUser, "prom", instantBody("up")).Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"jobs_done", "jobs_total", "inspected_bytes", "bytes"} {
		if _, ok := raw.Meta[key]; ok {
			t.Fatalf("meta.%s = %s on a series query, want it absent", key, raw.Meta[key])
		}
	}
}

type pageSource struct {
	*typedDatasource
	byOffset bool
	onPage   func(req datasource.PageRequest) ([]datasource.Frame, datasource.PageInfo, error)

	pageMu sync.Mutex
	pages  []datasource.PageRequest
}

func (d *pageSource) PagesByOffset() bool { return d.byOffset }

func (d *pageSource) QueryPage(_ context.Context, req datasource.PageRequest) ([]datasource.Frame, datasource.PageInfo, error) {
	d.pageMu.Lock()
	d.pages = append(d.pages, req)
	d.pageMu.Unlock()
	if d.onPage != nil {
		return d.onPage(req)
	}
	return logFrames(), datasource.PageInfo{Page: req.Page, PageSize: req.PageSize, Total: 300, TotalRelation: datasource.TotalAtLeast, HasNext: true, Next: &datasource.PageCursor{Page: req.Page + 1, EndNs: 5, Seen: []string{"fp"}}}, nil
}

func (d *pageSource) pageRequests() []datasource.PageRequest {
	d.pageMu.Lock()
	defer d.pageMu.Unlock()
	return append([]datasource.PageRequest(nil), d.pages...)
}

func newPageHarness(t *testing.T) (*harness, *pageSource) {
	t.Helper()
	hn := newHarness(t)
	logs := &pageSource{typedDatasource: &typedDatasource{fakeDatasource: &fakeDatasource{}, kind: spec.Loki}}
	registry, err := datasource.NewRegistry(hn.h.logger,
		datasource.Instance{UID: "prom", Datasource: hn.ds},
		datasource.Instance{UID: "logs", Datasource: logs},
	)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	hn.runner = queryproxy.NewRunner(registry, hn.h.logger)
	hn.h.runner, hn.h.registry = hn.runner, registry
	return hn, logs
}

const pagedQuery = `{service_name="checkout"}`

func pagedBody(size int) map[string]any {
	body := instantBody(pagedQuery)
	body["page_size"] = size
	return body
}

func rawMeta(t *testing.T, rec *httptest.ResponseRecorder) map[string]json.RawMessage {
	t.Helper()
	var raw struct {
		Meta map[string]json.RawMessage `json:"meta"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	return raw.Meta
}

func rawPage(t *testing.T, rec *httptest.ResponseRecorder) map[string]json.RawMessage {
	t.Helper()
	var page map[string]json.RawMessage
	if err := json.Unmarshal(rawMeta(t, rec)["page"], &page); err != nil {
		t.Fatal(err)
	}
	return page
}

func TestExplorePagesWithMetaPage(t *testing.T) {
	hn, logs := newPageHarness(t)
	rec := hn.explore(auth.KindUser, "logs", pagedBody(100))
	body := decodeData(t, rec)
	page := body.Meta.Page
	if page == nil {
		t.Fatalf("meta = %+v, want a page", body.Meta)
	}
	if page.Page != 1 || page.PageSize != 100 || page.Total == nil || *page.Total != 300 || page.TotalRelation != "gte" || !page.HasNext || page.HasPrev || page.Next == "" || page.Prev != "" {
		t.Fatalf("page = %+v, want page 1 of 100 with at least 300 rows, a next cursor and no prev", page)
	}
	if body.Meta.Truncated || body.Meta.RowsTotal != 0 || body.Meta.Rows != 3 || body.Meta.Kind != queryproxy.KindInstant || body.Meta.Cached {
		t.Fatalf("meta = %+v, want 3 rows, not truncated, no rows_total", body.Meta)
	}
	if body.Meta.Oldest != "2026-11-06T11:59:58.5Z" || body.Meta.Newest != "2026-11-06T12:00:00.000000001Z" {
		t.Fatalf("edges = %q to %q", body.Meta.Oldest, body.Meta.Newest)
	}
	cursor, err := datasource.DecodeCursor(page.Next, datasource.CursorScope("logs", pagedQuery))
	if err != nil {
		t.Fatal(err)
	}
	if cursor.Page != 2 || cursor.EndNs != 5 || cursor.FromNs != 1760000000123000000 || cursor.ToNs != 1760000060456000000 {
		t.Fatalf("next = %+v, want page 2 pinned to the request range", cursor)
	}
	reqs := logs.pageRequests()
	if len(reqs) != 1 || reqs[0].Page != 1 || reqs[0].PageSize != 100 || reqs[0].Cursor != nil || reqs[0].Query.Text != pagedQuery || reqs[0].Query.Limits != queryproxy.DraftLimits {
		t.Fatalf("page requests = %+v, want one for page 1 of 100 under draft limits", reqs)
	}
	meta, pageRaw := rawMeta(t, rec), rawPage(t, rec)
	for _, key := range []string{"prev", "window"} {
		if _, ok := pageRaw[key]; ok {
			t.Fatalf("meta.page.%s = %s on page 1, want it absent", key, pageRaw[key])
		}
	}
	if _, ok := meta["rows_total"]; ok {
		t.Fatalf("meta.rows_total = %s with paging on, want it absent", meta["rows_total"])
	}
}

func TestExplorePageTwoWithCursor(t *testing.T) {
	hn, logs := newPageHarness(t)
	cursor := datasource.EncodeCursor(datasource.PageCursor{Scope: datasource.CursorScope("logs", pagedQuery), Page: 2, FromNs: 1760000000000000000, ToNs: 1760000060000000000, EndNs: 9, Seen: []string{"a"}})
	body := pagedBody(100)
	body["cursor"] = cursor
	body["page"] = 2
	data := decodeData(t, hn.explore(auth.KindUser, "logs", body))
	page := data.Meta.Page
	if page == nil || page.Page != 2 || !page.HasPrev || page.Prev != cursor || !page.HasNext || page.Next == "" {
		t.Fatalf("page = %+v, want page 2 with the cursor echoed as prev", page)
	}
	if data.Meta.From != "2025-10-09T08:53:20Z" || data.Meta.To != "2025-10-09T08:54:20Z" {
		t.Fatalf("range = %q to %q, want the range pinned by the cursor", data.Meta.From, data.Meta.To)
	}
	reqs := logs.pageRequests()
	if len(reqs) != 1 || reqs[0].Page != 2 || reqs[0].Cursor == nil || reqs[0].Cursor.EndNs != 9 || reqs[0].Range.From.UnixNano() != 1760000000000000000 {
		t.Fatalf("page requests = %+v, want page 2 over the cursor's range", reqs)
	}
}

func TestExplorePageMetaCarriesTheWindowOnlyWhenSet(t *testing.T) {
	hn, logs := newPageHarness(t)
	for _, window := range []int{0, 10000} {
		logs.onPage = func(req datasource.PageRequest) ([]datasource.Frame, datasource.PageInfo, error) {
			return logFrames(), datasource.PageInfo{Page: 1, PageSize: 100, Total: 3, TotalRelation: datasource.TotalExact, Window: window}, nil
		}
		rec := hn.explore(auth.KindUser, "logs", pagedBody(100))
		got, present := rawPage(t, rec)["window"]
		if present != (window > 0) || (present && string(got) != "10000") {
			t.Fatalf("window = %s (present %v) for %d", got, present, window)
		}
		if page := decodeData(t, rec).Meta.Page; page.Window != window || page.HasNext || page.Next != "" {
			t.Fatalf("page = %+v, want window %d and no next", page, window)
		}
	}
}

func TestExplorePageWithUnknownTotalLeavesTotalOut(t *testing.T) {
	hn, logs := newPageHarness(t)
	logs.onPage = func(req datasource.PageRequest) ([]datasource.Frame, datasource.PageInfo, error) {
		frames := logFrames()
		frames[0].Meta = &datasource.FrameMeta{Partial: true, JobsDone: 1, JobsTotal: 4}
		return frames, datasource.PageInfo{Page: 1, PageSize: 100, TotalRelation: datasource.TotalUnknown}, nil
	}
	rec := hn.explore(auth.KindUser, "logs", pagedBody(100))
	body := decodeData(t, rec)
	if body.Meta.Page.Total != nil || body.Meta.Page.TotalRelation != "unknown" || !body.Meta.Partial || body.Meta.JobsDone != 1 {
		t.Fatalf("meta = %+v page %+v, want an unknown total and the partial meta", body.Meta, body.Meta.Page)
	}
	if total, ok := rawPage(t, rec)["total"]; ok {
		t.Fatalf("meta.page.total = %s, want it absent", total)
	}
}

func TestExploreRejectsBadPaging(t *testing.T) {
	hn, logs := newPageHarness(t)
	with := func(edit func(map[string]any)) map[string]any {
		body := pagedBody(100)
		edit(body)
		return body
	}
	tests := []struct {
		name    string
		uid     string
		body    map[string]any
		message string
	}{
		{"page_size on a range query", "logs", with(func(b map[string]any) { b["kind"] = "range" }), "paging applies to instant queries"},
		{"page_size with limit", "logs", with(func(b map[string]any) { b["limit"] = 10 }), "limit and page_size cannot both be given"},
		{"page without page_size", "logs", with(func(b map[string]any) { delete(b, "page_size"); b["page"] = 2 }), "page and cursor need page_size"},
		{"cursor without page_size", "logs", with(func(b map[string]any) { delete(b, "page_size"); b["cursor"] = "x" }), "page and cursor need page_size"},
		{"page_size below 50", "logs", with(func(b map[string]any) { b["page_size"] = 49 }), "page_size must be from 50 to 500"},
		{"page_size above 500", "logs", with(func(b map[string]any) { b["page_size"] = 501 }), "page_size must be from 50 to 500"},
		{"page zero", "logs", with(func(b map[string]any) { b["page"] = 0 }), "page must be at least 1"},
		{"cursor over 2 KiB", "logs", with(func(b map[string]any) { b["cursor"] = strings.Repeat("x", 2049) }), "cursor must be at most 2 KiB"},
		{"page 2 without a cursor on a cursor store", "logs", with(func(b map[string]any) { b["page"] = 2 }), "this datasource pages by cursor; pass the cursor of the previous page"},
		{"a cursor of another query", "logs", with(func(b map[string]any) {
			b["cursor"] = datasource.EncodeCursor(datasource.PageCursor{Scope: datasource.CursorScope("logs", "other"), Page: 2})
		}), "cursor does not belong to this query"},
		{"an unreadable cursor", "logs", with(func(b map[string]any) { b["cursor"] = "nope" }), "cursor is not readable"},
		{"a datasource that does not page", "prom", pagedBody(100), "paging reads logs and trace datasources"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := hn.explore(auth.KindUser, tt.uid, tt.body)
			expectError(t, rec, http.StatusBadRequest, "invalid_request")
			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body["error"] != tt.message {
				t.Fatalf("error = %q, want %q", body["error"], tt.message)
			}
		})
	}
	if n := len(logs.pageRequests()); n != 0 {
		t.Fatalf("%d pages ran for invalid requests", n)
	}
	if held := hn.heldSlots(); held != 0 {
		t.Fatalf("%d slots still held", held)
	}
}

func TestExplorePageMapsDatasourceErrors(t *testing.T) {
	hn, logs := newPageHarness(t)
	logs.onPage = func(req datasource.PageRequest) ([]datasource.Frame, datasource.PageInfo, error) {
		return nil, datasource.PageInfo{}, datasource.Errorf(datasource.ErrLimitExceeded, "%s", datasource.PageTimestampShared)
	}
	rec := hn.explore(auth.KindUser, "logs", pagedBody(100))
	expectError(t, rec, http.StatusUnprocessableEntity, "limit_exceeded")
	if !strings.Contains(rec.Body.String(), "more than page_size rows share one timestamp") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}
