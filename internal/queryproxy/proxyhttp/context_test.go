package proxyhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const contextAnchor = "1793983689914935344"

var contextAnchorTime = time.Unix(0, 1793983689914935344).UTC()

type lineSource struct {
	*typedDatasource
	onLines func(req datasource.LinesRequest) ([]datasource.Frame, error)

	mu    sync.Mutex
	lines []datasource.LinesRequest
}

func (d *lineSource) Lines(_ context.Context, req datasource.LinesRequest) ([]datasource.Frame, error) {
	d.mu.Lock()
	d.lines = append(d.lines, req)
	d.mu.Unlock()
	if d.onLines == nil {
		return nil, nil
	}
	return d.onLines(req)
}

func (d *lineSource) side(direction datasource.LineDirection) datasource.LinesRequest {
	d.mu.Lock()
	defer d.mu.Unlock()
	i := slices.IndexFunc(d.lines, func(r datasource.LinesRequest) bool { return r.Direction == direction })
	if i < 0 {
		return datasource.LinesRequest{}
	}
	return d.lines[i]
}

func (d *lineSource) calls() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.lines)
}

func newContextHarness(t *testing.T) (*harness, *lineSource) {
	t.Helper()
	hn := newHarness(t)
	logs := &lineSource{typedDatasource: &typedDatasource{fakeDatasource: &fakeDatasource{}, kind: spec.Loki}}
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

func (hn *harness) logContext(uid string, body map[string]any) *httptest.ResponseRecorder {
	hn.t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		hn.t.Fatalf("marshal log-context body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/datasources/"+uid+"/log-context", bytes.NewReader(encoded))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Kind", string(auth.KindUser))
	return hn.serve(req, "alice")
}

func contextLines(truncated bool, offsets ...int64) []datasource.Frame {
	f := datasource.Frame{Labels: map[string]string{"service_name": "checkout"}, Fields: []datasource.Field{
		{Name: "Time", Type: datasource.FieldTime},
		{Name: "Body", Type: datasource.FieldString},
		{Name: "TimeNs", Type: datasource.FieldString},
	}}
	for _, o := range offsets {
		at := contextAnchorTime.Add(time.Duration(o))
		f.Fields[0].Values = append(f.Fields[0].Values, at)
		f.Fields[1].Values = append(f.Fields[1].Values, "line "+strconv.FormatInt(o, 10))
		f.Fields[2].Values = append(f.Fields[2].Values, strconv.FormatInt(at.UnixNano(), 10))
	}
	if truncated {
		f.Meta = &datasource.FrameMeta{Truncated: true}
	}
	return []datasource.Frame{f}
}

type contextBody struct {
	Anchor string `json:"anchor"`
	Before struct {
		Frames    []json.RawMessage `json:"frames"`
		Truncated bool              `json:"truncated"`
	} `json:"before"`
	After struct {
		Frames    []json.RawMessage `json:"frames"`
		Truncated bool              `json:"truncated"`
	} `json:"after"`
}

func decodeContext(t *testing.T, rec *httptest.ResponseRecorder) contextBody {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	var body contextBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
	return body
}

func TestLogContextReadsBothSidesAroundTheAnchor(t *testing.T) {
	hn, logs := newContextHarness(t)
	logs.onLines = func(req datasource.LinesRequest) ([]datasource.Frame, error) {
		if req.Direction == datasource.Backward {
			return contextLines(false, -1, -2), nil
		}
		return contextLines(true, 1, 2, 3), nil
	}
	rec := hn.logContext("logs", map[string]any{"query": `{service_name="checkout"}`, "anchor": contextAnchor, "before": 2, "after": 3, "window_ms": 60000})
	body := decodeContext(t, rec)
	if body.Anchor != contextAnchor || len(body.Before.Frames) != 1 || len(body.After.Frames) != 1 {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if body.Before.Truncated || !body.After.Truncated {
		t.Fatalf("truncated = %v/%v, want false/true", body.Before.Truncated, body.After.Truncated)
	}
	if strings.Contains(rec.Body.String(), `"ref":"B"`) || !strings.Contains(rec.Body.String(), `"ref":"A"`) {
		t.Fatalf("frames must carry ref A: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"`+strconv.FormatInt(contextAnchorTime.Add(3).UnixNano(), 10)+`"`) {
		t.Fatalf("TimeNs strings must reach the wire: %s", rec.Body.String())
	}
	before, after := logs.side(datasource.Backward), logs.side(datasource.Forward)
	if !before.Range.To.Equal(contextAnchorTime) || !before.Range.From.Equal(contextAnchorTime.Add(-time.Minute)) || before.Limit != 2 {
		t.Fatalf("before = %+v, want range ending at the anchor with limit 2", before)
	}
	if !after.Range.From.Equal(contextAnchorTime.Add(time.Nanosecond)) || !after.Range.To.Equal(contextAnchorTime.Add(time.Minute)) || after.Limit != 3 {
		t.Fatalf("after = %+v, want range starting one nanosecond after the anchor with limit 3", after)
	}
	for _, req := range []datasource.LinesRequest{before, after} {
		if req.Query.Text != `{service_name="checkout"}` || req.Query.Limits != queryproxy.DraftLimits || req.Query.User.ID != "alice" {
			t.Fatalf("request = %+v, want the query under DraftLimits as alice", req)
		}
	}
	if hn.heldSlots() != 0 {
		t.Fatalf("%d slots still held", hn.heldSlots())
	}
}

func TestLogContextDefaultsToFiftyLinesAndOneHourAndSkipsAnEmptySide(t *testing.T) {
	hn, logs := newContextHarness(t)
	body := decodeContext(t, hn.logContext("logs", map[string]any{"query": "{a=\"b\"}", "anchor": contextAnchor}))
	if len(body.Before.Frames) != 0 || len(body.After.Frames) != 0 || body.Before.Truncated || body.After.Truncated {
		t.Fatalf("body = %+v, want two empty sides", body)
	}
	before, after := logs.side(datasource.Backward), logs.side(datasource.Forward)
	if before.Limit != defaultContextLines || after.Limit != defaultContextLines {
		t.Fatalf("limits = %d/%d, want %d", before.Limit, after.Limit, defaultContextLines)
	}
	if !before.Range.From.Equal(contextAnchorTime.Add(-time.Hour)) || !after.Range.To.Equal(contextAnchorTime.Add(time.Hour)) {
		t.Fatalf("window = %v..%v, want one hour each side", before.Range.From, after.Range.To)
	}

	hn, logs = newContextHarness(t)
	body = decodeContext(t, hn.logContext("logs", map[string]any{"query": "{a=\"b\"}", "anchor": contextAnchor, "before": 0}))
	if logs.calls() != 1 || logs.side(datasource.Backward).Direction != "" {
		t.Fatalf("calls = %d, want only the after side", logs.calls())
	}
	if !strings.Contains(hn.logContext("logs", map[string]any{"query": "{a=\"b\"}", "anchor": contextAnchor, "before": 0, "after": 0}).Body.String(), `"before":{"frames":[],"truncated":false}`) {
		t.Fatal("an empty side must serialize as an empty frame list")
	}
}

func TestLogContextReportsADatasourceErrorWithoutARef(t *testing.T) {
	hn, logs := newContextHarness(t)
	logs.onLines = func(req datasource.LinesRequest) ([]datasource.Frame, error) {
		if req.Direction == datasource.Forward {
			return nil, datasource.Errorf(datasource.ErrInvalidQuery, "parse error")
		}
		return contextLines(false, -1), nil
	}
	rec := hn.logContext("logs", map[string]any{"query": "{a=\"b\"}", "anchor": contextAnchor})
	expectError(t, rec, http.StatusUnprocessableEntity, "query_invalid")
	if strings.Contains(rec.Body.String(), `"ref"`) {
		t.Fatalf("error carries a ref: %s", rec.Body.String())
	}
}

func TestLogContextRejectsInvalidRequests(t *testing.T) {
	hn, logs := newContextHarness(t)
	valid := map[string]any{"query": "{a=\"b\"}", "anchor": contextAnchor}
	with := func(changes map[string]any) map[string]any {
		body := map[string]any{}
		for k, v := range valid {
			body[k] = v
		}
		for k, v := range changes {
			body[k] = v
		}
		return body
	}
	tests := []struct {
		name   string
		uid    string
		body   map[string]any
		status int
		code   string
	}{
		{"missing query", "logs", map[string]any{"anchor": contextAnchor}, http.StatusBadRequest, "invalid_request"},
		{"query too long", "logs", with(map[string]any{"query": strings.Repeat("x", maxLintQueryBytes+1)}), http.StatusBadRequest, "query_too_long"},
		{"missing anchor", "logs", map[string]any{"query": "{a=\"b\"}"}, http.StatusBadRequest, "invalid_request"},
		{"anchor not a number", "logs", with(map[string]any{"anchor": "2026-11-06T12:00:00Z"}), http.StatusBadRequest, "invalid_request"},
		{"before negative", "logs", with(map[string]any{"before": -1}), http.StatusBadRequest, "invalid_request"},
		{"after over 500", "logs", with(map[string]any{"after": 501}), http.StatusBadRequest, "invalid_request"},
		{"window below one second", "logs", with(map[string]any{"window_ms": 999}), http.StatusBadRequest, "invalid_request"},
		{"window over one day", "logs", with(map[string]any{"window_ms": 86400001}), http.StatusBadRequest, "invalid_request"},
		{"unknown field", "logs", with(map[string]any{"from": "now-1h"}), http.StatusBadRequest, "invalid_request"},
		{"unknown datasource", "nope", valid, http.StatusNotFound, "not_found"},
		{"not a line source", "prom", valid, http.StatusBadRequest, "invalid_request"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expectError(t, hn.logContext(tt.uid, tt.body), tt.status, tt.code)
		})
	}
	if logs.calls() != 0 {
		t.Fatalf("%d Lines calls for refused requests", logs.calls())
	}
	if got := decodeError(t, hn.logContext("prom", valid)).Error; got != notLineSource {
		t.Fatalf("message = %q, want %q", got, notLineSource)
	}
}
