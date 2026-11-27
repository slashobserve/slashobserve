package runsession

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/agent/grounding"
	"github.com/slashobserve/slashobserve/internal/agent/guard"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	groundedFinding   = `{"title":"Checkout latency","markdown":"Checkout p99 rose to 523 ms, up from 287 ms.","root_cause":{"component":"checkout","kind":"dependency","summary":"s"},"onset":"2026-11-20T10:05:00Z","evidence":[{"id":"p99"}]}`
	ungroundedFinding = `{"title":"Checkout latency","markdown":"Checkout p99 rose to 9.3 s and 2,950 orders failed.","root_cause":{"component":"cartservice","kind":"dependency","summary":"s"},"onset":"2026-11-20T10:05:00Z","evidence":[{"id":"p99"}]}`
)

func groundingScript(t *testing.T, mode grounding.Mode) *script {
	t.Helper()
	s := newScript(t, false)
	s.sess.state.Grounding = grounding.NewRecord(mode)
	if s.sess.state.Grounding.Enforced() {
		s.sess.state.Guard.WatchGrounding()
	}
	return s
}

func (s *script) queryLatency(turn *Turn) {
	s.t.Helper()
	ctx := context.Background()
	call := guard.Call{Name: "query_range", Input: json.RawMessage(`{"datasource":"prom","query":"p99"}`)}
	admitted, err := s.sess.admit(ctx, call, turn)
	if err != nil || admitted.answer != nil {
		s.t.Fatalf("admit query_range: %+v, %v", admitted.answer, err)
	}
	content := []byte(`{"series":[{"labels":{"service_name":"checkout"},"min":0.287,"max":0.5234}]}`)
	from := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	done := dispatched{
		res:     tools.Result{Content: json.RawMessage(content), Step: "Ran 1 query on prom"},
		content: content,
		ranges:  []datasource.TimeRange{{From: from, To: from.Add(2 * time.Hour)}},
	}
	if _, err := s.sess.settle(ctx, call, admitted, done); err != nil {
		s.t.Fatalf("settle query_range: %v", err)
	}
}

func (s *script) admitSave(input string, turn *Turn) admitted {
	s.t.Helper()
	admitted, err := s.sess.admit(context.Background(), guard.Call{Name: "save_finding", Input: json.RawMessage(input)}, turn)
	if err != nil {
		s.t.Fatalf("admit save_finding: %v", err)
	}
	return admitted
}

var ungroundedItems = []findings.Unverified{
	{Kind: findings.UnverifiedFigure, Text: "9.3 s"},
	{Kind: findings.UnverifiedFigure, Text: "2,950"},
	{Kind: findings.UnverifiedComponent, Text: "cartservice"},
}

func TestReportStoresTheUnverifiedItemsAndAdmitsTheSave(t *testing.T) {
	s := groundingScript(t, grounding.Report)
	s.queryLatency(&Turn{ID: "t1", Calls: 1})

	got := s.admitSave(ungroundedFinding, &Turn{ID: "t2", Calls: 1})
	if got.answer != nil {
		t.Fatalf("report sent the finding back: %s", got.answer.Content)
	}
	if !slices.Equal(got.unverified, ungroundedItems) {
		t.Errorf("unverified = %+v\nwant %+v", got.unverified, ungroundedItems)
	}
}

func TestEnforceSendsBackOnceAndAcceptsARepairedFinding(t *testing.T) {
	s := groundingScript(t, grounding.Enforce)
	s.queryLatency(&Turn{ID: "t1", Calls: 1})

	first := s.admitSave(ungroundedFinding, &Turn{ID: "t2", Calls: 1})
	if first.answer == nil || !first.answer.IsError {
		t.Fatal("enforce admitted an ungrounded finding")
	}
	var refusal struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(first.answer.Content, &refusal); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`figure "9.3 s": read it, correct it, or remove it`, `"cartservice": read it, correct it, or remove it`} {
		if !strings.Contains(refusal.Message, want) {
			t.Errorf("the send-back lacks %q:\n%s", want, refusal.Message)
		}
	}
	if s.sess.state.Guard.Rejections != 0 {
		t.Errorf("rejections = %d, want the send-back not counted", s.sess.state.Guard.Rejections)
	}

	s.queryLatency(&Turn{ID: "t3", Calls: 1})
	second := s.admitSave(groundedFinding, &Turn{ID: "t4", Calls: 1})
	if second.answer != nil {
		t.Fatalf("the repaired finding was refused: %s", second.answer.Content)
	}
	if len(second.unverified) != 0 {
		t.Errorf("unverified = %+v, want none", second.unverified)
	}
}

func TestEnforceAcceptsTheSecondSaveAndKeepsItsUnverifiedItems(t *testing.T) {
	s := groundingScript(t, grounding.Enforce)
	s.queryLatency(&Turn{ID: "t1", Calls: 1})

	if first := s.admitSave(ungroundedFinding, &Turn{ID: "t2", Calls: 1}); first.answer == nil {
		t.Fatal("enforce admitted an ungrounded finding")
	}
	second := s.admitSave(ungroundedFinding, &Turn{ID: "t3", Calls: 1})
	if second.answer != nil {
		t.Fatalf("the second save was refused: %s", second.answer.Content)
	}
	if !slices.Equal(second.unverified, ungroundedItems) {
		t.Errorf("unverified = %+v\nwant %+v", second.unverified, ungroundedItems)
	}
	if s.sess.state.Guard.Rejections != 0 || s.sess.ended {
		t.Errorf("rejections = %d, ended = %t, want neither", s.sess.state.Guard.Rejections, s.sess.ended)
	}
}

func TestWithGroundingOffNothingIsRecordedOrChecked(t *testing.T) {
	s := groundingScript(t, grounding.Off)
	s.queryLatency(&Turn{ID: "t1", Calls: 1})

	got := s.admitSave(ungroundedFinding, &Turn{ID: "t2", Calls: 1})
	if got.answer != nil || got.unverified != nil {
		t.Fatalf("off changed the save: %+v, %+v", got.answer, got.unverified)
	}
	raw, err := json.Marshal(s.sess.state)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "grounding") {
		t.Errorf("the stored state carries grounding: %s", raw)
	}
}

func TestOnlyAnInvestigateRunStartsWithAGroundingRecord(t *testing.T) {
	ask := revisions.Request{ID: "run", Attempt: 1, Intent: revisions.IntentAsk, Prompt: "why is checkout slow", Investigation: true}
	suggest := revisions.Request{ID: "run", Attempt: 1, Intent: revisions.IntentSuggest, Prompt: "add a panel", BaseVersion: 1}
	cases := []struct {
		name    string
		mode    grounding.Mode
		req     revisions.Request
		record  bool
		watched bool
	}{
		{"an investigation in report", grounding.Report, ask, true, false},
		{"an investigation in enforce", grounding.Enforce, ask, true, true},
		{"an investigation with grounding off", grounding.Off, ask, false, false},
		{"a suggestion in enforce", grounding.Enforce, suggest, false, false},
	}
	for _, c := range cases {
		state, err := (&Service{findingGrounding: c.mode}).initial(context.Background(), c.req, pgtype.UUID{})
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if (state.Grounding != nil) != c.record || (state.Guard.Grounding != nil) != c.watched {
			t.Errorf("%s: record = %t, watched = %t, want %t and %t", c.name, state.Grounding != nil, state.Guard.Grounding != nil, c.record, c.watched)
		}
	}
}

func (s *script) readOf(name, content string, discovered *tools.Discovery) {
	s.t.Helper()
	s.run(name, `{"q":"`+name+`"}`, func() tools.Result {
		return tools.Result{Content: json.RawMessage(content), Step: "Read " + name, Discovered: discovered}
	}, &Turn{ID: name, Calls: 1})
}

func TestAMCPToolResultFeedsTheRecordButTheSourceListDoesNot(t *testing.T) {
	s := groundingScript(t, grounding.Report)
	s.readOf("list_mcp_sources", `{"sources":[{"name":"code","tools":3,"revision":7700}]}`, nil)
	s.readOf("call_mcp_tool", `{"matches":[{"file":"checkout.go","line":4821}]}`, nil)

	got := s.sess.state.Grounding.Check(json.RawMessage(`{"markdown":"The match is at line 4,821, not 7,700."}`))
	want := []findings.Unverified{{Kind: findings.UnverifiedFigure, Text: "7,700"}}
	if !slices.Equal(got, want) {
		t.Errorf("unverified = %+v\nwant %+v", got, want)
	}
}

func TestOnlyDatasourceReadsFeedTheRecord(t *testing.T) {
	s := groundingScript(t, grounding.Report)
	s.readOf("get_observation", `{"blocks":[{"text":"p99 reached 9.3 s for 2,950 orders"}],"value":4321}`, nil)
	s.readOf("list_observations", `{"observations":[{"title":"Checkout at 7,700 ms"}]}`, nil)
	s.readOf("get_trace", `{"duration_ms":812.4,"services":[{"name":"checkout","spans":40}]}`, nil)
	s.readOf("list_metrics", `{"metrics":["pool_in_use"],"total":640}`, &tools.Discovery{Datasource: "prom", Of: "metrics", Names: []string{"pool_in_use"}})

	got := s.sess.state.Grounding.Check(json.RawMessage(`{"markdown":"Traces took 812 ms over 640 metrics, not 9.3 s, 2,950, 4,321 or 7,700 ms."}`))
	want := []findings.Unverified{
		{Kind: findings.UnverifiedFigure, Text: "9.3 s"},
		{Kind: findings.UnverifiedFigure, Text: "2,950"},
		{Kind: findings.UnverifiedFigure, Text: "4,321"},
		{Kind: findings.UnverifiedFigure, Text: "7,700 ms"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("unverified = %+v\nwant %+v", got, want)
	}
}
