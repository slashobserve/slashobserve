package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestDecodeInput(t *testing.T) {
	type sample struct {
		Datasource string            `json:"datasource"`
		Limit      *int              `json:"limit"`
		Count      int               `json:"count"`
		Items      []json.RawMessage `json:"items"`
	}
	tests := []struct {
		name    string
		input   string
		wantErr string
		check   func(t *testing.T, s sample)
	}{
		{"empty input", ``, "", nil},
		{"null input", `null`, "", nil},
		{"exact keys", `{"datasource":"a","limit":5}`, "", func(t *testing.T, s sample) {
			if s.Datasource != "a" || *s.Limit != 5 {
				t.Fatalf("decoded %+v", s)
			}
		}},
		{"upper case key", `{"DATASOURCE":"a"}`, `unknown field "DATASOURCE"`, nil},
		{"mixed case key", `{"Datasource":"a"}`, `unknown field "Datasource"`, nil},
		{"unknown key", `{"nope":1}`, `unknown field "nope"`, nil},
		{"unknown next to valid", `{"datasource":"a","extra":true}`, `unknown field "extra"`, nil},
		{"integral float", `{"limit":200.0}`, "", func(t *testing.T, s sample) {
			if *s.Limit != 200 {
				t.Fatalf("limit = %d", *s.Limit)
			}
		}},
		{"integral exponent", `{"limit":2e2}`, "", func(t *testing.T, s sample) {
			if *s.Limit != 200 {
				t.Fatalf("limit = %d", *s.Limit)
			}
		}},
		{"integral float on plain int", `{"count":3.0}`, "", func(t *testing.T, s sample) {
			if s.Count != 3 {
				t.Fatalf("count = %d", s.Count)
			}
		}},
		{"negative integral float", `{"count":-3.0}`, "", func(t *testing.T, s sample) {
			if s.Count != -3 {
				t.Fatalf("count = %d", s.Count)
			}
		}},
		{"fractional", `{"limit":10.5}`, `field "limit" must be an integer`, nil},
		{"fractional exponent", `{"limit":1e-1}`, `field "limit" must be an integer`, nil},
		{"huge float", `{"limit":1e30}`, `field "limit" must be an integer`, nil},
		{"string for integer", `{"limit":"5"}`, `field "limit" must be an integer, not a string`, nil},
		{"number for string field", `{"datasource":5}`, `field "datasource" must be a string, not a number`, nil},
		{"float for string field", `{"datasource":1.5}`, `field "datasource" must be a string, not a number`, nil},
		{"object for array field", `{"items":{"id":"p1"}}`, `field "items" must be an array, not an object`, nil},
		{"string for array field", `{"items":"p1"}`, `field "items" must be an array, not a string`, nil},
		{"array", `[]`, "input must be a JSON object", nil},
		{"scalar", `7`, "input must be a JSON object", nil},
		{"trailing value", `{} {}`, "input must be a single JSON object", nil},
		{"truncated", `{"datasource"`, "input is not valid JSON", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s sample
			res, ok := decodeInput(json.RawMessage(tt.input), &s)
			if tt.wantErr == "" {
				if !ok {
					t.Fatalf("rejected: %#v", res.Content)
				}
				if tt.check != nil {
					tt.check(t, s)
				}
				return
			}
			if ok {
				t.Fatalf("accepted %+v", s)
			}
			if errorCode(t, res) != codeInvalidRequest {
				t.Fatalf("content = %#v", res.Content)
			}
			if got := res.Content.(errorBody).Message; got != tt.wantErr {
				t.Fatalf("message = %q, want %q", got, tt.wantErr)
			}
		})
	}
}

func TestDecodeInputEmbeddedAndRawFields(t *testing.T) {
	var in rangeInput
	if res, ok := decodeInput(json.RawMessage(`{"datasource":"a","query":"q","from":"now-1h","max_data_points":100.0}`), &in); !ok {
		t.Fatalf("rejected: %#v", res.Content)
	}
	if in.Datasource != "a" || in.From != "now-1h" || *in.MaxDataPoints != 100 {
		t.Fatalf("decoded %+v", in)
	}
	if _, ok := decodeInput(json.RawMessage(`{"Query":"q"}`), &in); ok {
		t.Fatal("case-mismatched embedded key accepted")
	}

	rawSpec := `{ "title": "a <b> & c",   "panels": [] }`
	var save createInput
	if res, ok := decodeInput(json.RawMessage(`{"summary":"s","spec":`+rawSpec+`}`), &save); !ok {
		t.Fatalf("rejected: %#v", res.Content)
	}
	if string(save.Spec) != rawSpec {
		t.Fatalf("spec bytes changed: %q", save.Spec)
	}
}

func TestCallRejectsLooseInputWithStep(t *testing.T) {
	r := newTestRegistry(t, &fakeDS{}, nil)
	tests := []struct{ name, tool, input string }{
		{"case mismatch", nameLintQuery, `{"DATASOURCE":"fake","query":"up"}`},
		{"unknown field", nameLintQuery, `{"datasource":"fake","query":"up","x":1}`},
		{"fractional integer", nameQueryRange, `{"datasource":"fake","query":"up","max_data_points":20.5}`},
		{"empty-object tool with a key", nameListDatasources, `{"x":1}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := call(t, r, tt.tool, tt.input)
			if err != nil {
				t.Fatal(err)
			}
			if errorCode(t, res) != codeInvalidRequest {
				t.Fatalf("content = %#v", res.Content)
			}
			if !strings.HasPrefix(res.Step, "Rejected "+tt.tool+" input: ") || res.Step == "Rejected "+tt.tool+" input: invalid_request" {
				t.Fatalf("step = %q, want it to carry the message", res.Step)
			}
		})
	}
}

func TestUnknownFieldNamesTheFieldsOfTheCallersSurface(t *testing.T) {
	r := newTestRegistry(t, &fakeDS{}, nil)
	principal := auth.WithPrincipal(context.Background(), auth.Principal{Role: auth.RoleAdmin})
	createRun := WithRun(principal, Run{Mode: ModeCreate, Build: true, RequestID: dashA})
	message := func(t *testing.T, ctx context.Context, tool, input string) string {
		t.Helper()
		res, err := r.Call(ctx, tool, []byte(input))
		if err != nil {
			t.Fatal(err)
		}
		if errorCode(t, res) != codeInvalidRequest {
			t.Fatalf("content = %#v", res.Content)
		}
		return res.Content.(errorBody).Message
	}

	if got, want := message(t, WithRun(principal, Run{}), nameQueryInstant, `{"datasource":"fake","query":"up","format":"table"}`),
		`unknown field "format"; query_instant takes dashboard_title, datasource, from, query, to and variables`; got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
	if got, want := message(t, principal, nameListDatasources, `{"x":1}`), `unknown field "x"; list_datasources takes no fields`; got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
	outOfRun := message(t, principal, nameUpdatePanels, `{"bogus":1}`)
	for _, field := range []string{"dashboard_id", "parent_version"} {
		if !strings.Contains(outOfRun, field) {
			t.Errorf("MCP message lacks %s: %s", field, outOfRun)
		}
	}
	if strings.Contains(outOfRun, "finish") {
		t.Errorf("MCP message offers finish: %s", outOfRun)
	}
	inRun := message(t, createRun, nameUpdatePanels, `{"bogus":1}`)
	for _, field := range []string{"finish", "planned_panels", "row"} {
		if !strings.Contains(inRun, field) {
			t.Errorf("in-run message lacks %s: %s", field, inRun)
		}
	}
	if strings.Contains(inRun, "dashboard_id") {
		t.Errorf("in-run message offers dashboard_id: %s", inRun)
	}
}

func TestAnInvalidRequestStepCarriesItsClippedMessage(t *testing.T) {
	r := newTestRegistry(t, &fakeDS{}, nil)
	tests := []struct {
		name   string
		result Result
		want   string
	}{
		{"a short message", invalid("limit must be between 1 and 500"), "Rejected list_metrics input: limit must be between 1 and 500"},
		{"a long message is clipped", invalid("%s", strings.Repeat("x", 300)), "Rejected list_metrics input: " + strings.Repeat("x", maxStepReasonBytes) + "..."},
		{"no message", toolError(codeInvalidRequest, ""), "Rejected list_metrics input: invalid_request"},
		{"another code", toolError(codeNotFound, "gone"), "Could not run list_metrics: not_found"},
		{"a step the handler set", Result{Content: errorBody{Code: codeInvalidRequest, Message: "m"}, IsError: true, Step: "Own step"}, "Own step"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r.entries[nameListMetrics] = entry{handle: func(context.Context, json.RawMessage) (Result, error) { return tt.result, nil }}
			res, err := call(t, r, nameListMetrics, `{}`)
			if err != nil || res.Step != tt.want {
				t.Fatalf("step = %q err = %v, want %q", res.Step, err, tt.want)
			}
		})
	}
}

func TestPageCutsByBytesAndSaysWhereToGoOn(t *testing.T) {
	items := make([]string, 100)
	for i := range items {
		items[i] = fmt.Sprintf("%03d%s", i, strings.Repeat("x", 97))
	}
	kept, tr := page(items, 5, false, 1000)
	if len(kept) == 0 || kept[0] != items[5] {
		t.Fatalf("kept %d items starting %q, want a page starting at item 5", len(kept), kept[0])
	}
	if !tr.Truncated || tr.TruncatedBy != "bytes" || tr.NextOffset == nil || *tr.NextOffset != 5+len(kept) {
		t.Fatalf("truncation = %+v, want bytes and next_offset %d", tr, 5+len(kept))
	}
	if last, _ := page(items, 5+len(kept), false, 1<<20); len(last) != 100-5-len(kept) {
		t.Fatalf("the next page held %d items, want the rest", len(last))
	}
}

func TestPageReportsAnUpstreamLimitCut(t *testing.T) {
	items := []string{"a", "b", "c", "d"}
	kept, tr := page(items, 1, true, 1<<20)
	if !slices.Equal(kept, []string{"b", "c", "d"}) {
		t.Fatalf("kept = %v", kept)
	}
	if !tr.Truncated || tr.TruncatedBy != "limit" || tr.NextOffset == nil || *tr.NextOffset != 4 {
		t.Fatalf("truncation = %+v, want limit and next_offset 4", tr)
	}
	if _, tr := page(items, 0, false, 1<<20); tr != (truncation{}) {
		t.Fatalf("an uncut page reports %+v", tr)
	}
}

func TestPageGivesNoNextOffsetPastTheNamesLimit(t *testing.T) {
	items := make([]string, 500)
	kept, tr := page(items, 450, true, 1<<20)
	if len(kept) != 50 || !tr.Truncated || tr.TruncatedBy != "limit" || tr.NextOffset != nil {
		t.Fatalf("kept %d, truncation = %+v, want limit with no next_offset", len(kept), tr)
	}
	if kept, tr := page(items[:10], 20, false, 1<<20); len(kept) != 0 || tr != (truncation{}) {
		t.Fatalf("an offset past the end gave %d items and %+v", len(kept), tr)
	}
}

func TestUnknownToolStepNeverEchoesTheName(t *testing.T) {
	r := newTestRegistry(t, &fakeDS{}, nil)
	res, err := call(t, r, "secret-looking-name", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if errorCode(t, res) != codeUnknownTool || strings.Contains(res.Step, "secret") || res.Step == "" {
		t.Fatalf("res = %#v", res)
	}
}

func TestNonInputErrorsUseFailedStep(t *testing.T) {
	r := newTestRegistry(t, &fakeDS{}, nil)
	res, err := call(t, r, nameLintQuery, `{"datasource":"nope","query":"up"}`)
	if err != nil || res.Step != "Could not run lint_query: unknown_datasource" {
		t.Fatalf("res = %#v err = %v", res, err)
	}
	for _, code := range []string{codeNotFound, codeTooLarge, "query_timeout"} {
		r.entries[nameGetDashboard] = entry{handle: func(context.Context, json.RawMessage) (Result, error) {
			return toolError(code, "model-visible detail"), nil
		}}
		res, err := call(t, r, nameGetDashboard, `{}`)
		if err != nil || res.Step != "Could not run get_dashboard: "+code {
			t.Fatalf("%s: res = %#v err = %v", code, res, err)
		}
	}
}

func TestLintQueryReportsAnUnavailableDatasource(t *testing.T) {
	ds := &fakeDS{lintFn: func(context.Context, string) ([]datasource.Diagnostic, error) {
		return nil, datasource.Errorf(datasource.ErrUnavailable, "connection failed")
	}}
	r := newTestRegistry(t, ds, nil)
	res, err := call(t, r, nameLintQuery, `{"datasource":"fake","query":"up"}`)
	if err != nil || !res.IsError || res.Step != "Could not lint a query on fake: datasource_unavailable" {
		t.Fatalf("res = %#v err = %v", res, err)
	}
}

func TestToolErrorsKeepSpecificSteps(t *testing.T) {
	ds := &fakeDS{instantFn: func(context.Context, datasource.Query, datasource.TimeRange) ([]datasource.Frame, error) {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "bad")
	}}
	r := newTestRegistry(t, ds, nil)
	res, err := call(t, r, nameQueryInstant, `{"datasource":"fake","query":"up"}`)
	if err != nil || res.Step != "Ran a query on fake: query_invalid" {
		t.Fatalf("res = %#v err = %v", res, err)
	}
}

func TestCallRecoversPanics(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	r := newTestRegistry(t, &fakeDS{}, logger)
	r.entries[nameLintQuery] = entry{handle: func(context.Context, json.RawMessage) (Result, error) {
		panic("boom " + "hunter2")
	}}
	res, err := call(t, r, nameLintQuery, `{"query":"super-secret-query"}`)
	if err == nil || res.Content != nil || res.IsError {
		t.Fatalf("res = %#v err = %v", res, err)
	}
	if !errors.Is(err, ErrInvariant) {
		t.Fatalf("a panic is not an invariant: %v", err)
	}
	if FailureCode(err) != FailurePanicked {
		t.Fatalf("failure code = %q", FailureCode(err))
	}
	logged := logs.String()
	if !strings.Contains(logged, nameLintQuery) {
		t.Fatalf("log lacks the tool name: %s", logged)
	}
	if strings.Contains(logged, "stack") || strings.Contains(logged, "goroutine") {
		t.Fatalf("a panic must log its type only, not a stack: %s", logged)
	}
	for _, leaked := range []string{"super-secret-query", "hunter2"} {
		if strings.Contains(logged, leaked) || strings.Contains(err.Error(), leaked) {
			t.Fatalf("%q leaked: %s / %v", leaked, logged, err)
		}
	}
}

func TestADatasourcePanicReachesTheModelAsAToolError(t *testing.T) {
	ds := &fakeDS{instantFn: func(context.Context, datasource.Query, datasource.TimeRange) ([]datasource.Frame, error) {
		panic("driver bug")
	}}
	r := newTestRegistry(t, ds, nil)
	res, err := call(t, r, nameQueryInstant, `{"datasource":"fake","query":"up"}`)
	if err != nil || errorCode(t, res) != FailureToolFailed {
		t.Fatalf("res = %#v err = %v", res, err)
	}
	if msg := res.Content.(errorBody).Message; strings.Contains(msg, "driver bug") {
		t.Fatalf("message leaks the panic: %q", msg)
	}
}

func TestAHandlerToolErrorReachesTheModelWithSafeText(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"plain error", errors.New("boom at 10.0.0.5 password=hunter2"), "lint_query failed: the datasource did not answer"},
		{"errno", fmt.Errorf("dial tcp 10.0.0.5:9090: %w", syscall.ECONNREFUSED), "lint_query failed: errno connection refused"},
		{"wrapped twice", fmt.Errorf("outer: %w", fmt.Errorf("SELECT secret: %w", errors.New("hunter2"))), "lint_query failed: the datasource did not answer"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			r := newTestRegistry(t, &fakeDS{}, slog.New(slog.NewJSONHandler(&logs, nil)))
			r.entries[nameLintQuery] = entry{handle: func(context.Context, json.RawMessage) (Result, error) { return Result{}, tt.err }}
			ctx := WithRun(auth.WithPrincipal(context.Background(), auth.Principal{Role: auth.RoleAdmin}), Run{Mode: ModeCreate, RequestID: dashA})
			res, err := r.Call(ctx, nameLintQuery, []byte(`{"query":"super-secret-query"}`))
			if err != nil {
				t.Fatalf("err = %v, want a model-visible result", err)
			}
			if errorCode(t, res) != FailureToolFailed || res.Content.(errorBody).Message != tt.want {
				t.Fatalf("content = %#v, want message %q", res.Content, tt.want)
			}
			if res.Step != "Could not run lint_query: tool_failed" {
				t.Fatalf("step = %q", res.Step)
			}
			var line map[string]any
			if err := json.Unmarshal(logs.Bytes(), &line); err != nil {
				t.Fatalf("want one log line, got %s", logs.String())
			}
			if line["msg"] != "agent tool failed" || line["tool"] != nameLintQuery || line["error_code"] != FailureToolFailed || line["agent_request_id"] != dashA {
				t.Fatalf("log = %v", line)
			}
			for _, leaked := range []string{"super-secret-query", "hunter2", "10.0.0.5", "SELECT"} {
				if strings.Contains(logs.String(), leaked) || strings.Contains(res.Content.(errorBody).Message, leaked) {
					t.Fatalf("%q leaked: %s / %#v", leaked, logs.String(), res.Content)
				}
			}
		})
	}
}

func TestRunEndingHandlerErrorsAreInvariants(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	tests := []struct {
		name string
		ctx  context.Context
		err  error
	}{
		{"wrapped cancellation", context.Background(), fmt.Errorf("query: %w", context.Canceled)},
		{"wrapped deadline", context.Background(), fmt.Errorf("query: %w", context.DeadlineExceeded)},
		{"build lost", context.Background(), fmt.Errorf("save: %w", dashboards.ErrBuildLost)},
		{"invalid run scope", context.Background(), fmt.Errorf("%w: no request id", errInvalidRun)},
		{"missing dependency", context.Background(), errCommentsNotConfigured},
		{"any error once the run is canceled", canceled, errors.New("boom")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestRegistry(t, &fakeDS{}, nil)
			r.entries[nameLintQuery] = entry{handle: func(context.Context, json.RawMessage) (Result, error) { return Result{}, tt.err }}
			res, err := r.Call(auth.WithPrincipal(tt.ctx, auth.Principal{Role: auth.RoleAdmin}), nameLintQuery, []byte(`{}`))
			if !errors.Is(err, ErrInvariant) || !errors.Is(err, tt.err) || res.Content != nil || res.IsError {
				t.Fatalf("res = %#v err = %v, want an invariant wrapping the cause", res, err)
			}
		})
	}
}

func TestARunWithoutAPrincipalIsAnInvariant(t *testing.T) {
	r := newTestRegistry(t, &fakeDS{}, nil)
	res, err := r.Call(WithRun(context.Background(), Run{Mode: ModeCreate}), nameListDatasources, []byte(`{}`))
	if !errors.Is(err, ErrInvariant) || res.Content != nil {
		t.Fatalf("res = %#v err = %v", res, err)
	}
}

func TestCallResultCaps(t *testing.T) {
	big := func(n int) handler {
		return func(context.Context, json.RawMessage) (Result, error) {
			return success("ok", map[string]string{"data": strings.Repeat("a", n)}), nil
		}
	}
	r := newTestRegistry(t, &fakeDS{}, nil)

	r.entries[nameListMetrics] = entry{handle: big(maxContentBytes - 20)}
	if _, err := call(t, r, nameListMetrics, `{}`); err != nil {
		t.Fatalf("result under the cap rejected: %v", err)
	}
	r.entries[nameListMetrics] = entry{handle: big(maxContentBytes)}
	if res, err := call(t, r, nameListMetrics, `{}`); err != nil || errorCode(t, res) != FailureResultTooLarge {
		t.Fatalf("oversized result accepted or misclassified: res = %#v err = %v", res, err)
	}

	if MaxModelSpecBytes != 128<<10 {
		t.Fatalf("get_dashboard cap = %d, want 128 KiB", MaxModelSpecBytes)
	}
	r.entries[nameGetDashboard] = entry{handle: big(maxContentBytes * 4)}
	if _, err := call(t, r, nameGetDashboard, `{}`); err != nil {
		t.Fatalf("get_dashboard result above 16 KiB rejected: %v", err)
	}
	r.entries[nameGetDashboard] = entry{handle: big(MaxModelSpecBytes - 20)}
	if _, err := call(t, r, nameGetDashboard, `{}`); err != nil {
		t.Fatalf("get_dashboard result at its cap rejected: %v", err)
	}
	r.entries[nameGetDashboard] = entry{handle: big(MaxModelSpecBytes)}
	if res, err := call(t, r, nameGetDashboard, `{}`); err != nil || errorCode(t, res) != FailureResultTooLarge {
		t.Fatalf("get_dashboard result over its cap accepted: res = %#v err = %v", res, err)
	}
}

func TestAResultOverItsLimitIsTooLargeWithANarrowingHint(t *testing.T) {
	tests := []struct {
		tool  string
		size  int
		limit int
		hint  string
	}{
		{nameListMetrics, 20 << 10, maxContentBytes, "pass search, or a smaller limit and the next_offset as offset"},
		{nameQueryRange, 20 << 10, maxContentBytes, "shorten from and to, lower max_data_points, or reduce the series with a label selector or an aggregation"},
		{nameGetDashboard, MaxModelSpecBytes + 1, MaxModelSpecBytes, "use get_panels with the panel ids you need"},
		{nameLintQuery, 20 << 10, maxContentBytes, "narrow the request"},
	}
	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			var logs bytes.Buffer
			r := newTestRegistry(t, &fakeDS{}, slog.New(slog.NewJSONHandler(&logs, nil)))
			r.entries[tt.tool] = entry{handle: func(context.Context, json.RawMessage) (Result, error) {
				return success("ok", map[string]string{"data": strings.Repeat("a", tt.size)}), nil
			}}
			res, err := call(t, r, tt.tool, `{}`)
			if err != nil {
				t.Fatalf("err = %v, want a model-visible result", err)
			}
			if errorCode(t, res) != FailureResultTooLarge || res.Step != "Result too large: "+tt.tool {
				t.Fatalf("res = %#v", res)
			}
			msg := res.Content.(errorBody).Message
			wantSize := fmt.Sprintf("%d bytes, over the %d byte limit", tt.size+len(`{"data":""}`), tt.limit)
			if !strings.Contains(msg, wantSize) || !strings.HasSuffix(msg, "; "+tt.hint) {
				t.Fatalf("message = %q, want %q and the hint %q", msg, wantSize, tt.hint)
			}
			if !strings.Contains(logs.String(), "agent tool result too large") {
				t.Fatalf("log lacks the too-large line: %s", logs.String())
			}
		})
	}
}

func TestASaveResultOverItsLimitIsTrimmedNeverAnError(t *testing.T) {
	panels := func(n int) []panelOut {
		out := make([]panelOut, n)
		for i := range out {
			out[i] = panelOut{ID: fmt.Sprintf("p%d", i), Title: strings.Repeat("t", 200), Type: spec.Timeseries}
		}
		return out
	}
	ids := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = fmt.Sprintf("unverified-%d-%s", i, strings.Repeat("u", 40))
		}
		return out
	}
	tests := []struct {
		name    string
		tool    string
		limit   int
		content any
	}{
		{"create with too many panels", nameCreateDashboard, maxContentBytes, savedOut{DashboardID: "d1", Version: 1, URL: "http://x/d/d1", Panels: panels(200), UnverifiedPanels: ids(3)}},
		{"update_panels with too many unverified panels", nameUpdatePanels, maxSuggestResult, savedOut{DashboardID: "d1", Version: 9, URL: "http://x/d/d1", Panels: panels(400), UnverifiedPanels: ids(2000)}},
		{"build save with too many changed panels", nameUpdateDashboard, maxSuggestResult, buildOut{Build: buildStateOut{Saves: 2, Panels: 400}, ChangedPanels: panels(400), Layout: &layoutOut{Rows: []string{"r1"}}, Message: "saved"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if jsonSize(tt.content) <= tt.limit {
				t.Fatalf("fixture is %d bytes, not over the %d byte limit", jsonSize(tt.content), tt.limit)
			}
			r := newTestRegistry(t, &fakeDS{}, nil)
			r.entries[tt.tool] = entry{handle: func(context.Context, json.RawMessage) (Result, error) {
				return success("Saved", tt.content), nil
			}}
			res, err := call(t, r, tt.tool, `{}`)
			if err != nil || res.IsError {
				t.Fatalf("res = %#v err = %v, want a trimmed success", res, err)
			}
			if size := jsonSize(res.Content); size > tt.limit {
				t.Fatalf("trimmed result is %d bytes, over the %d byte limit", size, tt.limit)
			}
			switch got := res.Content.(type) {
			case savedOut:
				want := tt.content.(savedOut)
				if !got.Truncated || got.DashboardID != want.DashboardID || got.Version != want.Version || got.URL != want.URL {
					t.Fatalf("got %+v, want the saved version kept and marked truncated", got)
				}
				if len(got.UnverifiedPanels) == 0 || !slices.Equal(got.UnverifiedPanels, want.UnverifiedPanels[:len(got.UnverifiedPanels)]) {
					t.Fatalf("unverified panels = %v, want a prefix of the saved list", got.UnverifiedPanels)
				}
				if len(got.Panels) >= len(want.Panels) || !slices.Equal(got.Panels, want.Panels[:len(got.Panels)]) {
					t.Fatalf("panels = %d, want a shorter prefix of %d", len(got.Panels), len(want.Panels))
				}
			case buildOut:
				want := tt.content.(buildOut)
				if got.Build != want.Build || got.Message != want.Message || len(got.ChangedPanels) == 0 || len(got.ChangedPanels) >= len(want.ChangedPanels) {
					t.Fatalf("got %+v, want the build state kept and fewer changed panels", got)
				}
			default:
				t.Fatalf("content is %T", res.Content)
			}
		})
	}
}

func TestListSchemas(t *testing.T) {
	wantNames := []string{
		"create_annotation", "create_dashboard", "create_finding", "draft_query", "get_dashboard", "get_panels", "get_schema", "get_spec_guide", "import_grafana_dashboard", "lint_query",
		"list_annotations", "list_comments", "list_dashboard_health", "list_dashboards", "list_datasources", "list_findings", "list_folders", "list_label_values", "list_labels", "list_metrics", "log_facets", "logs_for_trace",
		"query_instant", "query_range", "reply_to_comment", "resolve_comment", "sample_rows", "save_finding", "update_dashboard", "update_panels",
	}
	first := New(Deps{}).List()
	var names []string
	for _, tool := range first {
		names = append(names, tool.Name)
	}
	if !slices.Equal(names, wantNames) || !slices.IsSorted(names) {
		t.Fatalf("names = %v", names)
	}

	second := New(Deps{}).List()
	for i, tool := range first {
		if tool.Description == "" || strings.ContainsRune(tool.Description, 0x2014) {
			t.Errorf("%s: bad description %q", tool.Name, tool.Description)
		}
		if !bytes.Equal(tool.InputSchema, second[i].InputSchema) || tool.Description != second[i].Description {
			t.Errorf("%s: not byte-stable across registries", tool.Name)
		}
		checkSchema(t, tool)
	}

	descriptions := map[string]string{}
	for _, tool := range first {
		descriptions[tool.Name] = tool.Description
	}
	if !strings.HasSuffix(descriptions[nameCreateDashboard], " Never include a placeholder panel.") {
		t.Errorf("create_dashboard description = %q", descriptions[nameCreateDashboard])
	}
	if !strings.HasSuffix(descriptions[nameUpdateDashboard], " Placeholder panels may be moved or removed, never added or edited.") {
		t.Errorf("update_dashboard description = %q", descriptions[nameUpdateDashboard])
	}

	list := New(Deps{}).List()
	list[0].InputSchema[0] = 'X'
	if again := New(Deps{}).List(); again[0].InputSchema[0] != '{' {
		t.Fatal("List exposes the registry's schema bytes")
	}
}

var numericKeywords = []string{"minimum", "maximum", "minLength", "maxLength", "minItems", "maxItems", "maxProperties"}

func checkSchema(t *testing.T, tool Tool) {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(tool.InputSchema))
	dec.UseNumber()
	var schema map[string]any
	if err := dec.Decode(&schema); err != nil {
		t.Errorf("%s: schema is not valid JSON: %v", tool.Name, err)
		return
	}
	if schema["type"] != "object" || schema["additionalProperties"] != false {
		t.Errorf("%s: root must be a closed object: %v", tool.Name, schema)
	}
	props, _ := schema["properties"].(map[string]any)
	if props == nil {
		t.Errorf("%s: no properties", tool.Name)
	}
	if required, ok := schema["required"].([]any); ok {
		for _, name := range required {
			if _, ok := props[name.(string)]; !ok {
				t.Errorf("%s: required %q is not a property", tool.Name, name)
			}
		}
	}
	walkSchema(t, tool.Name, schema)
}

func walkSchema(t *testing.T, tool string, node any) {
	t.Helper()
	switch v := node.(type) {
	case map[string]any:
		for _, kw := range numericKeywords {
			if value, ok := v[kw]; ok {
				if _, isNumber := value.(json.Number); !isNumber {
					t.Errorf("%s: %s must be a JSON number, got %T", tool, kw, value)
				}
			}
		}
		for _, child := range v {
			walkSchema(t, tool, child)
		}
	case []any:
		for _, child := range v {
			walkSchema(t, tool, child)
		}
	}
}

func TestSpecGuideFitsWithMargin(t *testing.T) {
	r := newTestRegistry(t, &fakeDS{}, nil)
	res, err := call(t, r, nameGetSpecGuide, `{}`)
	if err != nil || res.IsError {
		t.Fatalf("res = %#v err = %v", res, err)
	}
	body, err := json.Marshal(res.Content)
	if err != nil {
		t.Fatal(err)
	}
	if limit := resultLimits[nameGetSpecGuide] * 3 / 4; len(body) > limit {
		t.Fatalf("guide result is %d bytes, over 75%% of the cap (%d)", len(body), limit)
	}
	var out struct {
		Guide   string          `json:"guide"`
		Schema  json.RawMessage `json:"schema"`
		Example json.RawMessage `json:"example"`
	}
	if err := json.Unmarshal(body, &out); err != nil || out.Guide == "" || len(out.Schema) == 0 || len(out.Example) == 0 {
		t.Fatalf("incomplete guide: %v", err)
	}
}
