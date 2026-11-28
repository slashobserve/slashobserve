package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/elasticsearch"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const draftFixtures = "testdata/draft"

const cpuQuestionReason = "CPU usage of a pod is a resource metric, and this Loki datasource only holds log lines labeled by service_name and detected_level, with no pod or CPU data."

var draftViewer = auth.Principal{Kind: auth.KindUser, ID: "viewer-1", Role: auth.RoleViewer}

func draftRegistry(t *testing.T, provider DraftModel, fakes map[string]*fakeDS) *Registry {
	t.Helper()
	logger := slog.New(slog.DiscardHandler)
	var instances []datasource.Instance
	for uid, ds := range fakes {
		instances = append(instances, datasource.Instance{UID: uid, Name: uid, Datasource: ds})
	}
	reg, err := datasource.NewRegistry(logger, instances...)
	if err != nil {
		t.Fatal(err)
	}
	return New(Deps{Datasources: reg, Runner: queryproxy.NewRunner(reg, logger), Drafts: provider, Logger: logger})
}

func draftCatalog(labels []string, values map[string][]string) func(context.Context, datasource.DiscoverRequest) (*datasource.Catalog, error) {
	return func(_ context.Context, req datasource.DiscoverRequest) (*datasource.Catalog, error) {
		switch req.Kind {
		case datasource.DiscoverLabels:
			return &datasource.Catalog{Labels: labels}, nil
		case datasource.DiscoverTables:
			return &datasource.Catalog{Tables: []datasource.Table{{Name: "otel_logs"}, {Name: "otel_traces"}}}, nil
		case datasource.DiscoverColumns:
			return &datasource.Catalog{Columns: []datasource.Column{{Name: "Timestamp", Type: "DateTime64(9)"}, {Name: "Body", Type: "String"}, {Name: "ServiceName", Type: "String"}}}, nil
		}
		return &datasource.Catalog{Values: values[req.Label]}, nil
	}
}

func oneRow(context.Context, datasource.Query, datasource.TimeRange) ([]datasource.Frame, error) {
	return []datasource.Frame{logTableFrame()}, nil
}

func unclosedSelector(_ context.Context, query string) ([]datasource.Diagnostic, error) {
	if strings.Count(query, "{") > strings.Count(query, "}") {
		return []datasource.Diagnostic{{Severity: datasource.SeverityError, Message: "parse error: unclosed stream selector"}}, nil
	}
	return nil, nil
}

func draftFakes() map[string]*fakeDS {
	services := []string{"checkout", "payments"}
	return map[string]*fakeDS{
		"loki": {typ: "loki", lintFn: unclosedSelector, instantFn: oneRow,
			discoverFn: draftCatalog([]string{"service_name", "detected_level"}, map[string][]string{"service_name": services, "detected_level": {"error", "info"}})},
		"ch": {typ: "clickhouse", discoverFn: draftCatalog(nil, map[string][]string{"ServiceName": services}),
			instantFn: func(ctx context.Context, q datasource.Query, rng datasource.TimeRange) ([]datasource.Frame, error) {
				if strings.Contains(q.Text, "Message") {
					return nil, datasource.Errorf(datasource.ErrInvalidQuery, "unknown column Message")
				}
				return oneRow(ctx, q, rng)
			}},
		"tempo": {typ: "tempo", instantFn: oneRow,
			discoverFn: draftCatalog([]string{"resource.service.name", "span.http.status_code"}, map[string][]string{"resource.service.name": services})},
	}
}

type scriptedDraft struct {
	turns    []DraftTurn
	release  chan struct{}
	entered  chan struct{}
	mu       sync.Mutex
	requests []DraftRequest
	results  [][]DraftResult
}

func (p *scriptedDraft) Available() bool { return true }

func (p *scriptedDraft) Start(_ context.Context, req DraftRequest) (DraftSession, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.requests = append(p.requests, req)
	return &scriptedDraftSession{p: p}, nil
}

type scriptedDraftSession struct {
	p    *scriptedDraft
	next int
}

func (s *scriptedDraftSession) Next(ctx context.Context, results []DraftResult) (DraftTurn, error) {
	p := s.p
	if p.entered != nil {
		p.entered <- struct{}{}
	}
	if p.release != nil {
		select {
		case <-p.release:
		case <-ctx.Done():
			return DraftTurn{}, ctx.Err()
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.results = append(p.results, results)
	if s.next >= len(p.turns) {
		return DraftTurn{Stop: DraftStopEndTurn}, nil
	}
	turn := p.turns[s.next]
	s.next++
	return turn, nil
}

func (s *scriptedDraftSession) Close() {}

func submitTurn(input string) DraftTurn {
	return DraftTurn{Stop: DraftStopToolUse, ToolCalls: []DraftCall{{ID: "call-1", Name: nameSubmitQuery, Input: json.RawMessage(input)}}}
}

type fixtureFile struct {
	Prompt string        `json:"prompt"`
	Turns  []fixtureTurn `json:"turns"`
}

type fixtureTurn struct {
	Text      string `json:"text"`
	ToolCalls []struct {
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	} `json:"tool_calls"`
	Expect []struct {
		IsError  bool   `json:"is_error"`
		Contains string `json:"contains"`
	} `json:"expect"`
}

// fixtureModel plays testdata/draft the way the harness's replay provider plays a fixture: matched by the prompt's
// words, lowercased, one turn per call, each later turn checking the tool result it was sent.
type fixtureModel struct {
	fixtures map[string]fixtureFile
	mu       sync.Mutex
	nexts    int
}

func replayProvider(t *testing.T) *fixtureModel {
	t.Helper()
	entries, err := os.ReadDir(draftFixtures)
	if err != nil {
		t.Fatal(err)
	}
	m := &fixtureModel{fixtures: map[string]fixtureFile{}}
	for _, e := range entries {
		raw, err := fs.ReadFile(os.DirFS(draftFixtures), e.Name())
		if err != nil {
			t.Fatal(err)
		}
		var f fixtureFile
		if err := json.Unmarshal(raw, &f); err != nil {
			t.Fatal(err)
		}
		m.fixtures[fixtureKey(f.Prompt)] = f
	}
	return m
}

func fixtureKey(prompt string) string {
	return strings.Join(strings.Fields(strings.ToLower(prompt)), " ")
}

func (m *fixtureModel) Available() bool { return true }

func (m *fixtureModel) Start(_ context.Context, req DraftRequest) (DraftSession, error) {
	f, ok := m.fixtures[fixtureKey(req.Prompt)]
	return &fixtureSession{m: m, f: f, found: ok}, nil
}

func (m *fixtureModel) calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.nexts
}

type fixtureSession struct {
	m     *fixtureModel
	f     fixtureFile
	found bool
	next  int
}

func (s *fixtureSession) Next(_ context.Context, results []DraftResult) (DraftTurn, error) {
	s.m.mu.Lock()
	s.m.nexts++
	s.m.mu.Unlock()
	if !s.found {
		return DraftTurn{}, &DraftModelError{Failure: DraftFailureNoFixture}
	}
	if s.next >= len(s.f.Turns) {
		return DraftTurn{}, fmt.Errorf("fixture %q has no turn %d", s.f.Prompt, s.next+1)
	}
	turn := s.f.Turns[s.next]
	s.next++
	if turn.Expect != nil && len(turn.Expect) != len(results) {
		return DraftTurn{}, fmt.Errorf("fixture %q turn %d expected %d results, got %+v", s.f.Prompt, s.next, len(turn.Expect), results)
	}
	for i, e := range turn.Expect {
		if results[i].IsError != e.IsError || !strings.Contains(results[i].Content, e.Contains) {
			return DraftTurn{}, fmt.Errorf("fixture %q turn %d expected a result like %+v, got %+v", s.f.Prompt, s.next, e, results[i])
		}
	}
	out := DraftTurn{Stop: DraftStopEndTurn}
	for i, c := range turn.ToolCalls {
		out.Stop = DraftStopToolUse
		out.ToolCalls = append(out.ToolCalls, DraftCall{ID: fmt.Sprintf("call-%d", i+1), Name: c.Name, Input: c.Input})
	}
	return out, nil
}

func (s *fixtureSession) Close() {}

func viewerContext() context.Context {
	return auth.WithPrincipal(reading(context.Background()), draftViewer)
}

func fixtureOf(t *testing.T, file string) (prompt string, turns []fixtureTurn) {
	t.Helper()
	raw, err := fs.ReadFile(os.DirFS(draftFixtures), file)
	if err != nil {
		t.Fatal(err)
	}
	var f fixtureFile
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	return f.Prompt, f.Turns
}

func lastSubmit(t *testing.T, file string) submitInput {
	t.Helper()
	_, turns := fixtureOf(t, file)
	var in submitInput
	if err := json.Unmarshal(turns[len(turns)-1].ToolCalls[0].Input, &in); err != nil {
		t.Fatal(err)
	}
	return in
}

func TestDraftFixturesAnswerInTheirLanguage(t *testing.T) {
	cases := []struct {
		file, uid, signal, language string
	}{
		{"loki-checkout-errors.json", "loki", "", languageLogQL},
		{"loki-lint-corrected.json", "loki", "", languageLogQL},
		{"clickhouse-logs-payment.json", "ch", signalLogs, languageClickHouseSQL},
		{"clickhouse-sample-corrected.json", "ch", signalLogs, languageClickHouseSQL},
		{"clickhouse-slow-checkout-traces.json", "ch", signalTraces, languageFilters},
		{"tempo-checkout-errors.json", "tempo", "", languageFilters},
		{"tempo-status-500.json", "tempo", "", languageTraceQL},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			r := draftRegistry(t, replayProvider(t), draftFakes())
			question, _ := fixtureOf(t, c.file)
			draft, err := r.DraftQuery(viewerContext(), DraftInput{DatasourceUID: c.uid, Question: question, Signal: c.signal})
			if err != nil {
				t.Fatal(err)
			}
			want := lastSubmit(t, c.file)
			if draft.Language != c.language || draft.Query != want.Query || draft.Explanation != want.Explanation || draft.Warnings == nil || len(draft.Warnings) != 0 {
				t.Fatalf("draft %+v, want language %s and %+v", draft, c.language, want)
			}
			if (want.TraceFilters == nil) != (draft.TraceFilters == nil) || (want.TraceFilters != nil && *want.TraceFilters != *draft.TraceFilters) {
				t.Fatalf("trace filters %+v, want %+v", draft.TraceFilters, want.TraceFilters)
			}
		})
	}
}

func TestDraftMakesAtMostTwoModelCalls(t *testing.T) {
	cases := map[string]struct {
		uid, signal string
		calls       int
	}{
		"loki-checkout-errors.json":            {"loki", "", 1},
		"loki-lint-corrected.json":             {"loki", "", 2},
		"loki-lint-twice.json":                 {"loki", "", 2},
		"clickhouse-logs-payment.json":         {"ch", signalLogs, 1},
		"clickhouse-sample-corrected.json":     {"ch", signalLogs, 2},
		"clickhouse-slow-checkout-traces.json": {"ch", signalTraces, 1},
		"tempo-checkout-errors.json":           {"tempo", "", 1},
		"tempo-status-500.json":                {"tempo", "", 1},
		"cannot-translate.json":                {"loki", "", 1},
		"text-only.json":                       {"loki", "", 1},
	}
	entries, err := os.ReadDir(draftFixtures)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(cases) {
		t.Fatalf("%d fixtures, %d cases", len(entries), len(cases))
	}
	for file, c := range cases {
		t.Run(file, func(t *testing.T) {
			provider := replayProvider(t)
			r := draftRegistry(t, provider, draftFakes())
			question, _ := fixtureOf(t, file)
			_, _ = r.DraftQuery(viewerContext(), DraftInput{DatasourceUID: c.uid, Question: question, Signal: c.signal})
			if got := provider.calls(); got != c.calls {
				t.Fatalf("%d model calls, want %d", got, c.calls)
			}
		})
	}
}

func cannotTranslate(t *testing.T, err error) string {
	t.Helper()
	var cannot *CannotTranslate
	if !errors.As(err, &cannot) {
		t.Fatalf("err = %v, want *CannotTranslate", err)
	}
	return cannot.Reason
}

func TestDraftGivesUpAfterOneCorrection(t *testing.T) {
	r := draftRegistry(t, replayProvider(t), draftFakes())
	_, err := r.DraftQuery(viewerContext(), DraftInput{DatasourceUID: "loki", Question: "checkout lines with a retry"})
	reason := cannotTranslate(t, err)
	if !strings.HasPrefix(reason, stillFails) || !strings.Contains(reason, "lint check") || !strings.HasSuffix(reason, ".") {
		t.Fatalf("reason %q", reason)
	}
}

func TestDraftCorrectionCarriesTheFailureAsTheToolResult(t *testing.T) {
	provider := &scriptedDraft{turns: []DraftTurn{
		submitTurn(`{"query":"{service_name=\"checkout\"","explanation":"Checkout lines."}`),
		submitTurn(`{"query":"{service_name=\"checkout\"}","explanation":"Checkout lines."}`),
	}}
	r := draftRegistry(t, provider, draftFakes())
	draft, err := r.DraftQuery(viewerContext(), DraftInput{DatasourceUID: "loki", Question: "checkout lines"})
	if err != nil || draft.Query != `{service_name="checkout"}` {
		t.Fatalf("draft %+v, err %v", draft, err)
	}
	if len(provider.requests) != 1 || len(provider.results) != 2 || provider.results[0] != nil {
		t.Fatalf("%d sessions, results %+v", len(provider.requests), provider.results)
	}
	result := provider.results[1]
	if len(result) != 1 || result[0].CallID != "call-1" || !result[0].IsError ||
		result[0].Content != "the query fails the lint check: parse error: unclosed stream selector"+lastTurnNote {
		t.Fatalf("corrective result %+v", result)
	}
	req := provider.requests[0]
	if req.Prompt != "checkout lines" || req.System != draftSystem || len(req.Tools) != 2 || req.Tools[0].Name != nameSubmitQuery || req.Tools[1].Name != nameCannotTranslate ||
		!strings.Contains(req.Context, `values of service_name: "checkout", "payments"`) {
		t.Fatalf("request %+v", req)
	}
}

func TestDraftBrokenRuleEndsWithoutACorrection(t *testing.T) {
	cases := map[string]struct {
		uid, input, want string
	}{
		"tempo with both":         {"tempo", `{"query":"{ }","trace_filters":{"service":"checkout"},"explanation":"All traces."}`, "send either query or trace_filters, not both"},
		"clickhouse traces query": {"ch", `{"query":"SELECT 1","explanation":"One."}`, "this datasource takes trace_filters, not a query"},
		"unknown field":           {"tempo", `{"trace_filters":{},"explanation":"All.","limit":5}`, `unknown field "limit"`},
		"bad duration":            {"tempo", `{"trace_filters":{"min":"slow"},"explanation":"Slow traces."}`, "trace_filters.min must be a duration"},
		"long explanation":        {"tempo", `{"trace_filters":{},"explanation":"` + strings.Repeat("a", 301) + `"}`, "explanation must be 1 to 300 characters"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			provider := &scriptedDraft{turns: []DraftTurn{submitTurn(c.input), submitTurn(c.input)}}
			r := draftRegistry(t, provider, draftFakes())
			_, err := r.DraftQuery(viewerContext(), DraftInput{DatasourceUID: c.uid, Question: "traces", Signal: signalTraces})
			reason := cannotTranslate(t, err)
			if !strings.HasPrefix(reason, "The model's answer broke a rule: ") || !strings.Contains(reason, c.want) || len(provider.results) != 1 {
				t.Fatalf("reason %q after %d calls", reason, len(provider.results))
			}
		})
	}
}

func TestDraftTurnsWithoutAnAnswer(t *testing.T) {
	cases := map[string]struct {
		turn DraftTurn
		want string
	}{
		"refusal":      {DraftTurn{Stop: DraftStopRefusal}, reasonDeclined},
		"max tokens":   {DraftTurn{Stop: DraftStopMaxTokens}, reasonCutOff},
		"two calls":    {DraftTurn{Stop: DraftStopToolUse, ToolCalls: []DraftCall{{ID: "a", Name: nameSubmitQuery}, {ID: "b", Name: nameSubmitQuery}}}, reasonNoQuery},
		"unknown tool": {DraftTurn{Stop: DraftStopToolUse, ToolCalls: []DraftCall{{ID: "a", Name: "lint_query", Input: json.RawMessage(`{}`)}}}, reasonNoQuery},
		"empty reason": {DraftTurn{Stop: DraftStopToolUse, ToolCalls: []DraftCall{{ID: "a", Name: nameCannotTranslate, Input: json.RawMessage(`{"reason":" "}`)}}}, reasonNoQuery},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			r := draftRegistry(t, &scriptedDraft{turns: []DraftTurn{c.turn}}, draftFakes())
			_, err := r.DraftQuery(viewerContext(), DraftInput{DatasourceUID: "loki", Question: "errors"})
			if reason := cannotTranslate(t, err); reason != c.want {
				t.Fatalf("reason %q, want %q", reason, c.want)
			}
		})
	}
}

func TestDraftCannotTranslateFixtures(t *testing.T) {
	r := draftRegistry(t, replayProvider(t), draftFakes())
	cases := map[string]string{
		"cpu of the payments pod":    cpuQuestionReason,
		"hello there":                reasonNoQuery,
		"a question with no fixture": reasonNoReplay,
	}
	for question, want := range cases {
		_, err := r.DraftQuery(viewerContext(), DraftInput{DatasourceUID: "loki", Question: question})
		if reason := cannotTranslate(t, err); reason != want {
			t.Errorf("%q: reason %q, want %q", question, reason, want)
		}
	}
}

func TestDraftWarnsAboutAnUnseenService(t *testing.T) {
	provider := &scriptedDraft{turns: []DraftTurn{submitTurn(`{"trace_filters":{"service":"shipping"},"explanation":"Shipping traces."}`)}}
	r := draftRegistry(t, provider, draftFakes())
	draft, err := r.DraftQuery(viewerContext(), DraftInput{DatasourceUID: "tempo", Question: "shipping traces"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"No service named shipping was seen in this datasource; the search may be empty."}
	if draft.Language != languageFilters || !slices.Equal(draft.Warnings, want) {
		t.Fatalf("draft %+v", draft)
	}
}

func TestDraftReturnsAnEmptySampleWithAWarning(t *testing.T) {
	fakes := draftFakes()
	fakes["loki"].instantFn = func(context.Context, datasource.Query, datasource.TimeRange) ([]datasource.Frame, error) {
		return nil, nil
	}
	provider := &scriptedDraft{turns: []DraftTurn{submitTurn(`{"query":"{service_name=~\".+\"}","explanation":"Every line."}`)}}
	r := draftRegistry(t, provider, fakes)
	draft, err := r.DraftQuery(viewerContext(), DraftInput{DatasourceUID: "loki", Question: "everything"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{warnNoSampleMatches, warnEveryStream}; !slices.Equal(draft.Warnings, want) || len(provider.results) != 1 {
		t.Fatalf("warnings %q after %d calls", draft.Warnings, len(provider.results))
	}
}

func searchDraftDS(typ string) *roleFake {
	return &roleFake{
		fakeDS: &fakeDS{typ: typ, instantFn: oneRow,
			discoverFn: searchCatalog([]string{"log.level", "service.name"}, map[string]string{"log.level": "keyword", "service.name": "keyword"},
				map[string][]string{"service.name": {"checkout"}, "log.level": {"error"}}),
			lintFn: func(_ context.Context, query string) ([]datasource.Diagnostic, error) {
				switch {
				case strings.Contains(query, "serivce"):
					return []datasource.Diagnostic{{Severity: datasource.SeverityWarning, Message: "field serivce.name is not in the mapping"}}, nil
				case strings.Contains(query, "(("):
					return []datasource.Diagnostic{{Severity: datasource.SeverityError, Message: "unbalanced parenthesis"}}, nil
				}
				return nil, nil
			}},
		roles: elasticsearch.FieldRoles{Time: "@timestamp", Service: "service.name", Level: "log.level"},
	}
}

func TestDraftWritesKQLOnElasticsearchAndOpenSearch(t *testing.T) {
	cases := []struct {
		name, answer string
		warnings     []string
	}{
		{"clean kql", `service.name:checkout and log.level:error`, []string{}},
		{"unknown field", `serivce.name:checkout`, []string{"field serivce.name is not in the mapping"}},
		{"lucene prefix", `lucene:service.name:checkout`, []string{warnLucene}},
		{"every document", `*`, []string{warnEveryDocument}},
	}
	for _, typ := range []string{"elasticsearch", "opensearch"} {
		for _, c := range cases {
			t.Run(typ+" "+c.name, func(t *testing.T) {
				answer, _ := json.Marshal(map[string]string{"query": c.answer, "explanation": "Checkout errors."})
				provider := &scriptedDraft{turns: []DraftTurn{submitTurn(string(answer))}}
				r := searchRegistry(t, provider, searchDraftDS(typ))
				draft, err := r.DraftQuery(viewerContext(), DraftInput{DatasourceUID: "es", Question: "checkout errors"})
				if err != nil {
					t.Fatal(err)
				}
				if draft.Language != languageKQL || draft.Query != c.answer || draft.TraceFilters != nil || !slices.Equal(draft.Warnings, c.warnings) {
					t.Fatalf("draft %+v, want warnings %q", draft, c.warnings)
				}
				if !strings.Contains(provider.requests[0].Context, `values of service.name: "checkout"`) {
					t.Fatalf("facts:\n%s", provider.requests[0].Context)
				}
			})
		}
	}
}

func TestDraftFixesAKQLQueryThatFailsItsLint(t *testing.T) {
	provider := &scriptedDraft{turns: []DraftTurn{
		submitTurn(`{"query":"((service.name:checkout","explanation":"Checkout lines."}`),
		submitTurn(`{"query":"service.name:checkout","explanation":"Checkout lines."}`),
	}}
	r := searchRegistry(t, provider, searchDraftDS("elasticsearch"))
	draft, err := r.DraftQuery(viewerContext(), DraftInput{DatasourceUID: "es", Question: "checkout lines"})
	if err != nil || draft.Query != "service.name:checkout" {
		t.Fatalf("draft %+v, err %v", draft, err)
	}
	if got := provider.results[1][0].Content; got != "the query fails the lint check: unbalanced parenthesis"+lastTurnNote {
		t.Fatalf("corrective result %q", got)
	}
}

func TestDraftOnASearchDatasourceTakesLogsOnly(t *testing.T) {
	for _, typ := range []string{"elasticsearch", "opensearch"} {
		r := searchRegistry(t, &scriptedDraft{}, searchDraftDS(typ))
		_, err := r.DraftQuery(viewerContext(), DraftInput{DatasourceUID: "es", Question: "slow traces", Signal: signalTraces})
		want := "es is an " + searchName(spec.DatasourceType(typ)) + " datasource, which holds logs; signal must be logs"
		if !errors.Is(err, ErrDraftInvalid) || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v, want %q", typ, err, want)
		}
	}
}

func TestDraftRefusesBadInput(t *testing.T) {
	fakes := draftFakes()
	fakes["prom"] = &fakeDS{typ: "prometheus"}
	r := draftRegistry(t, &scriptedDraft{}, fakes)
	cases := map[string]struct {
		in   DraftInput
		want error
	}{
		"logs on prometheus":    {DraftInput{DatasourceUID: "prom", Question: "q", Signal: signalLogs}, ErrDraftInvalid},
		"metrics on clickhouse": {DraftInput{DatasourceUID: "ch", Question: "q", Signal: signalMetrics}, ErrDraftInvalid},
		"traces on loki":        {DraftInput{DatasourceUID: "loki", Question: "q", Signal: signalTraces}, ErrDraftInvalid},
		"logs on tempo":         {DraftInput{DatasourceUID: "tempo", Question: "q", Signal: signalLogs}, ErrDraftInvalid},
		"clickhouse signal":     {DraftInput{DatasourceUID: "ch", Question: "q"}, ErrDraftInvalid},
		"metrics on loki":       {DraftInput{DatasourceUID: "loki", Question: "q", Signal: signalMetrics}, ErrDraftInvalid},
		"unknown signal":        {DraftInput{DatasourceUID: "loki", Question: "q", Signal: "events"}, ErrDraftInvalid},
		"empty question":        {DraftInput{DatasourceUID: "loki", Question: "  "}, ErrDraftInvalid},
		"long question":         {DraftInput{DatasourceUID: "loki", Question: strings.Repeat("q", maxQuestionRunes+1)}, ErrDraftInvalid},
		"control character":     {DraftInput{DatasourceUID: "loki", Question: "q\x00"}, ErrDraftInvalid},
		"missing datasource":    {DraftInput{Question: "q"}, ErrDraftInvalid},
		"bad range":             {DraftInput{DatasourceUID: "loki", Question: "q", From: "now", To: "now-1h"}, ErrDraftInvalid},
		"unknown uid":           {DraftInput{DatasourceUID: "nope", Question: "q"}, ErrDraftNotFound},
	}
	for name, c := range cases {
		if _, err := r.DraftQuery(viewerContext(), c.in); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", name, err, c.want)
		}
	}
	if _, err := draftRegistry(t, nil, fakes).DraftQuery(viewerContext(), DraftInput{DatasourceUID: "loki", Question: "q"}); !errors.Is(err, ErrDraftUnavailable) {
		t.Errorf("no provider: err = %v", err)
	}
	if _, err := r.DraftQuery(viewerContext(), DraftInput{DatasourceUID: "loki", Question: strings.Repeat("q", maxQuestionRunes)}); errors.Is(err, ErrDraftInvalid) {
		t.Errorf("a 2000 character question is refused: %v", err)
	}
}

func TestDraftLimitsDraftsInFlight(t *testing.T) {
	provider := &scriptedDraft{release: make(chan struct{}), entered: make(chan struct{}, maxDraftsInFlight)}
	r := draftRegistry(t, provider, draftFakes())
	as := func(id string) context.Context {
		return auth.WithPrincipal(reading(context.Background()), auth.Principal{Kind: auth.KindUser, ID: id, Role: auth.RoleViewer})
	}
	ask := func(ctx context.Context) error {
		_, err := r.DraftQuery(ctx, DraftInput{DatasourceUID: "loki", Question: "errors"})
		return err
	}
	var wg sync.WaitGroup
	start := func(id string) {
		wg.Go(func() { _ = ask(as(id)) })
		<-provider.entered
	}
	for range maxDraftsPerCaller {
		start("a")
	}
	if err := ask(as("a")); !errors.Is(err, ErrDraftTooMany) {
		t.Fatalf("fourth draft by one caller: %v", err)
	}
	for i := maxDraftsPerCaller; i < maxDraftsInFlight; i++ {
		start(fmt.Sprintf("u%d", i))
	}
	if err := ask(as("b")); !errors.Is(err, ErrDraftBusy) {
		t.Fatalf("eleventh draft: %v", err)
	}
	close(provider.release)
	wg.Wait()
	go func() {
		for range provider.entered {
		}
	}()
	if err := ask(as("a")); errors.Is(err, ErrDraftTooMany) || errors.Is(err, ErrDraftBusy) {
		t.Fatalf("after release: %v", err)
	}
	close(provider.entered)
}

func TestDraftTimeoutMapsOnlyItsOwnDeadline(t *testing.T) {
	expired, cancel := context.WithTimeout(reading(context.Background()), 0)
	defer cancel()
	<-expired.Done()
	failed := errors.New("query_timeout")
	if err := timedOut(reading(context.Background()), expired, failed); !errors.Is(err, ErrDraftTimeout) {
		t.Errorf("own deadline: %v", err)
	}
	if err := timedOut(expired, expired, failed); !errors.Is(err, failed) {
		t.Errorf("caller's deadline: %v", err)
	}
	if err := timedOut(reading(context.Background()), expired, nil); err != nil {
		t.Errorf("success after the deadline: %v", err)
	}
	if err := timedOut(reading(context.Background()), reading(context.Background()), failed); !errors.Is(err, failed) {
		t.Errorf("no deadline: %v", err)
	}
}

func TestDraftProviderFailures(t *testing.T) {
	cases := []struct {
		err  error
		want error
		text string
	}{
		{&DraftModelError{Failure: DraftFailureRateLimited}, ErrDraftProvider, "rate limited"},
		{&DraftModelError{Failure: DraftFailureProviderError}, ErrDraftProvider, ""},
		{fmt.Errorf("next: %w", &DraftModelError{Failure: DraftFailureCallTimeout}), ErrDraftProvider, ""},
		{context.Canceled, context.Canceled, ""},
		{ErrDraftStopped, ErrDraftStopped, ""},
	}
	for _, c := range cases {
		got := modelFailure(c.err)
		if !errors.Is(got, c.want) || !strings.Contains(got.Error(), c.text) || strings.Contains(got.Error(), "status") {
			t.Errorf("modelFailure(%v) = %v", c.err, got)
		}
	}
	if code, _, ok := DraftFailure(modelFailure(&DraftModelError{Failure: DraftFailureError})); ok {
		t.Errorf("an unknown model failure maps to the code %s, want an internal error", code)
	}
	if code, message, _ := DraftFailure(ErrDraftStopped); code != "agent_unavailable" || message != ErrDraftStopped.Error() {
		t.Errorf("a stopped agent = %s %q", code, message)
	}
	if reason := cannotTranslate(t, modelFailure(&DraftModelError{Failure: DraftFailureNoFixture})); reason != reasonNoReplay {
		t.Errorf("no fixture reason %q", reason)
	}
}

func TestDraftFailureCodes(t *testing.T) {
	cases := []struct {
		err           error
		code, message string
	}{
		{ErrDraftUnavailable, "agent_unavailable", draftUnavailableMessage},
		{invalidDraftf("signal must be logs or traces"), codeInvalidRequest, "signal must be logs or traces"},
		{ErrDraftNotFound, "not_found", "datasource not found"},
		{ErrDraftTooMany, "too_many_requests", ErrDraftTooMany.Error()},
		{ErrDraftBusy, "agent_busy", ErrDraftBusy.Error()},
		{&DatasourceError{Code: "datasource_unavailable", Message: "connection refused"}, "datasource_unavailable", "connection refused"},
		{&CannotTranslate{Reason: "No."}, CodeCannotTranslate, "No."},
		{fmt.Errorf("%w: rate limited; try again shortly", ErrDraftProvider), "provider_error", "the model provider failed: rate limited; try again shortly"},
		{ErrDraftTimeout, "draft_timeout", ErrDraftTimeout.Error()},
	}
	for _, c := range cases {
		code, message, ok := DraftFailure(c.err)
		if !ok || code != c.code || message != c.message {
			t.Errorf("DraftFailure(%v) = %q, %q, %t", c.err, code, message, ok)
		}
	}
	if _, _, ok := DraftFailure(errors.New("boom")); ok {
		t.Error("an unknown error has a code")
	}
}

func TestDraftQueryTool(t *testing.T) {
	r := draftRegistry(t, replayProvider(t), draftFakes())
	viewer := viewerContext()
	res, err := r.Call(viewer, nameDraftQuery, []byte(`{"datasource_uid":"loki","question":"error logs from checkout","from":"now-6h","to":"now"}`))
	if err != nil {
		t.Fatal(err)
	}
	body := string(contentJSON(t, res))
	want := `{"query":"{service_name=\"checkout\", detected_level=\"error\"}","language":"logql","trace_filters":null,"explanation":"Returns log lines from the checkout service whose detected level is error, within the time range.","warnings":[]}`
	if res.IsError || res.Step != "Wrote a logql draft" || body != want {
		t.Fatalf("step %q, body %s", res.Step, body)
	}
	res, err = r.Call(viewer, nameDraftQuery, []byte(`{"datasource_uid":"ch","question":"slow checkout requests over 500 ms","signal":"traces"}`))
	if err != nil {
		t.Fatal(err)
	}
	if body := string(contentJSON(t, res)); !strings.Contains(body, `"language":"filters","trace_filters":{"service":"checkout","min":"500ms"}`) {
		t.Fatalf("filters body %s", body)
	}
	res, err = r.Call(viewer, nameDraftQuery, []byte(`{"datasource_uid":"loki","question":"cpu of the payments pod"}`))
	if err != nil {
		t.Fatal(err)
	}
	if body := string(contentJSON(t, res)); body != `{"code":"cannot_translate","message":"`+cpuQuestionReason+`"}` {
		t.Fatalf("cannot translate body %s", body)
	}
	res, err = r.Call(viewer, nameDraftQuery, []byte(`{"question":"q"}`))
	if err != nil || errorCode(t, res) != codeInvalidRequest {
		t.Fatalf("missing datasource: %+v, %v", res, err)
	}
	unavailable := draftRegistry(t, nil, draftFakes())
	res, err = unavailable.Call(viewer, nameDraftQuery, []byte(`{"datasource_uid":"loki","question":"q"}`))
	if err != nil || errorCode(t, res) != "agent_unavailable" {
		t.Fatalf("no provider: %+v, %v", res, err)
	}
}

func TestDraftQueryIsNeverOfferedToTheAgent(t *testing.T) {
	for _, tool := range New(Deps{}).List() {
		if tool.Name != nameDraftQuery {
			continue
		}
		if !tool.MCPOnly || !tool.ReadOnly || tool.MinRole != auth.RoleViewer {
			t.Fatalf("tool %+v", tool)
		}
		for _, mode := range []Mode{ModeCreate, ModeRevise, ModeSuggest, ModeInvestigate} {
			if tool.OfferedIn(mode) {
				t.Errorf("draft_query is offered in %s", mode)
			}
		}
		return
	}
	t.Fatal("draft_query is not registered")
}

func TestDraftLogsNoQuestionOrQueryText(t *testing.T) {
	var logs strings.Builder
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	reg, err := datasource.NewRegistry(logger, datasource.Instance{UID: "loki", Name: "loki", Datasource: draftFakes()["loki"]})
	if err != nil {
		t.Fatal(err)
	}
	r := New(Deps{Datasources: reg, Runner: queryproxy.NewRunner(reg, logger), Drafts: replayProvider(t), Logger: logger})
	if _, err := r.DraftQuery(viewerContext(), DraftInput{DatasourceUID: "loki", Question: "checkout lines with a timeout"}); err != nil {
		t.Fatal(err)
	}
	var line map[string]any
	for raw := range strings.SplitSeq(strings.TrimSpace(logs.String()), "\n") {
		var entry map[string]any
		if json.Unmarshal([]byte(raw), &entry) == nil && entry["msg"] == "draft_query finished" {
			line = entry
		}
	}
	if line == nil || line["model_calls"] != float64(2) || line["discovery_calls"] != float64(3) || line["outcome"] != "drafted" || line["signal"] != signalLogs {
		t.Fatalf("log line %v", line)
	}
	if text := logs.String(); strings.Contains(text, "timeout\\\"") || strings.Contains(text, "with a timeout") {
		t.Fatalf("logs carry the question or query: %s", text)
	}
}

type fakeBudget struct {
	allowed  bool
	resetsIn time.Duration
	keys     []string
}

func (f *fakeBudget) Spend(_ context.Context, key string) (bool, time.Duration, error) {
	f.keys = append(f.keys, key)
	return f.allowed, f.resetsIn, nil
}

func TestDraftRefusesAQuestionOnceTheHourlyBudgetIsSpent(t *testing.T) {
	provider := &scriptedDraft{turns: []DraftTurn{submitTurn(`{"query":"{service_name=\"checkout\"}","explanation":"Checkout lines."}`)}}
	r := draftRegistry(t, provider, draftFakes())
	budget := &fakeBudget{allowed: false, resetsIn: 17 * time.Minute}
	r.deps.DraftBudget = budget

	_, err := r.DraftQuery(viewerContext(), DraftInput{DatasourceUID: "loki", Question: "checkout lines"})

	code, message, ok := DraftFailure(err)
	if !ok || code != "too_many_requests" || !strings.Contains(message, "try again in 18 minutes") {
		t.Fatalf("code %q message %q ok %v err %v", code, message, ok, err)
	}
	if len(provider.requests) != 0 {
		t.Fatalf("the model was called %d times after the budget was spent", len(provider.requests))
	}
	if !slices.Equal(budget.keys, []string{"user:viewer-1"}) {
		t.Fatalf("budget keys %v", budget.keys)
	}
}

func TestDraftSpendsOneBudgetUnitPerQuestionAndIgnoresAnInvalidOne(t *testing.T) {
	provider := &scriptedDraft{turns: []DraftTurn{submitTurn(`{"query":"{service_name=\"checkout\"}","explanation":"Checkout lines."}`)}}
	r := draftRegistry(t, provider, draftFakes())
	budget := &fakeBudget{allowed: true}
	r.deps.DraftBudget = budget

	if _, err := r.DraftQuery(viewerContext(), DraftInput{DatasourceUID: "loki", Question: ""}); !errors.Is(err, ErrDraftInvalid) {
		t.Fatalf("err %v", err)
	}
	if len(budget.keys) != 0 {
		t.Fatalf("an invalid question spent the budget: %v", budget.keys)
	}
	if _, err := r.DraftQuery(viewerContext(), DraftInput{DatasourceUID: "loki", Question: "checkout lines"}); err != nil {
		t.Fatal(err)
	}
	if len(budget.keys) != 1 {
		t.Fatalf("budget keys %v, want one", budget.keys)
	}
}
