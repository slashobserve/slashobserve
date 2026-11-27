package runsession

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/grounding"
	"github.com/slashobserve/slashobserve/internal/agent/guard"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
)

func (s *script) readAnswer(name, input, content string, turn *Turn) Answer {
	s.t.Helper()
	ctx := context.Background()
	call := guard.Call{Name: name, Input: json.RawMessage(input)}
	admitted, err := s.sess.admit(ctx, call, turn)
	if err != nil || admitted.answer != nil {
		s.t.Fatalf("admit %s: %+v, %v", name, admitted.answer, err)
	}
	answer, err := s.sess.settle(ctx, call, admitted, dispatched{res: tools.Result{Content: json.RawMessage(content), Step: "Read " + name}, content: []byte(content)})
	if err != nil {
		s.t.Fatalf("settle %s: %v", name, err)
	}
	return answer
}

const measured = `{"stat":"max","from":"2026-11-20T09:00:00Z","to":"2026-11-20T11:00:00Z","sampled":"30s","query":"max_over_time(p99[2h:30s])","empty":false,"values":[{"labels":{"service":"checkout"},"value":0.5234123,"at":"2026-11-20T10:05:00Z"}]}`

func TestAReadShowsItsFactIDsAndTheSessionKeepsThemAcrossAStore(t *testing.T) {
	s := groundingScript(t, grounding.Report)
	answer := s.readAnswer("measure_window", `{"datasource":"prom","query":"max(request_duration_seconds)","stat":"max"}`, measured, &Turn{ID: "t1", Calls: 1})
	if !strings.Contains(string(answer.Content), `"value":0.5234123,"at":"2026-11-20T10:05:00Z","facts":{"value":"f1"}`) {
		t.Fatalf("the result lacks its fact id: %s", answer.Content)
	}
	raw, err := json.Marshal(s.sess.state)
	if err != nil {
		t.Fatal(err)
	}
	var stored State
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	if f, ok := stored.Grounding.Fact("f1"); !ok || f.Value != 0.5234123 || f.Query != "max_over_time(p99[2h:30s])" {
		t.Errorf("stored fact = %+v, %t", f, ok)
	}

	again := s.readAnswer("measure_window", `{"datasource":"prom","query":"max(request_duration_seconds)","stat":"max","step":"30s"}`, measured, &Turn{ID: "t2", Calls: 1})
	if !strings.Contains(string(again.Content), `"facts":{"value":"f1"}`) {
		t.Errorf("the same value read again got another id: %s", again.Content)
	}
}

func sendBackMessage(t *testing.T, a admitted) string {
	t.Helper()
	if a.answer == nil || !a.answer.IsError {
		t.Fatal("the save was admitted, want it sent back")
	}
	var refusal struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(a.answer.Content, &refusal); err != nil {
		t.Fatal(err)
	}
	return refusal.Message
}

func TestACleanCitationIsAdmittedWithTheLedgerForTheSave(t *testing.T) {
	s := groundingScript(t, grounding.Enforce)
	s.readAnswer("measure_window", `{"datasource":"prom","query":"max(request_duration_seconds)","stat":"max"}`, measured, &Turn{ID: "t1", Calls: 1})

	got := s.admitSave(`{"markdown":"Checkout p99 peaked at {{f1:ms}}."}`, &Turn{ID: "t2", Calls: 1})
	if got.answer != nil || got.grounding.unverified != nil {
		t.Fatalf("a clean save was refused or flagged: %+v %+v", got.answer, got.grounding.unverified)
	}
	if rendered, problems := got.grounding.ledger.Render("{{f1:ms:3}}"); rendered != "523 ms" || problems != nil {
		t.Errorf("the save's ledger rendered %q, %+v", rendered, problems)
	}
}

func TestAnUnknownFactIsSentBackOnceAndTheSecondSaveStillSaves(t *testing.T) {
	s := groundingScript(t, grounding.Enforce)
	s.readAnswer("measure_window", `{"datasource":"prom","query":"max(request_duration_seconds)","stat":"max"}`, measured, &Turn{ID: "t1", Calls: 1})
	const save = `{"markdown":"Checkout p99 peaked at {{f12}}."}`

	message := sendBackMessage(t, s.admitSave(save, &Turn{ID: "t2", Calls: 1}))
	if !strings.Contains(message, `cited fact "{{f12}}": f12 is not a fact this run read`) {
		t.Errorf("the send-back does not name the id:\n%s", message)
	}
	if s.sess.state.Guard.Rejections != 0 {
		t.Errorf("rejections = %d, want the send-back not counted", s.sess.state.Guard.Rejections)
	}

	second := s.admitSave(save, &Turn{ID: "t3", Calls: 1})
	if second.answer != nil {
		t.Fatalf("the second save was refused: %s", second.answer.Content)
	}
	if len(second.grounding.unverified) != 1 || second.grounding.unverified[0].Kind != "fact" || second.grounding.unverified[0].Text != "{{f12}}" {
		t.Errorf("unverified = %+v, want the cited fact", second.grounding.unverified)
	}
	if rendered, _ := second.grounding.ledger.Render("{{f12}}"); rendered != grounding.UnreadMarker {
		t.Errorf("the unknown fact renders as %q", rendered)
	}
}

func TestTwoAnswersForOneQuantityAreSentBack(t *testing.T) {
	s := groundingScript(t, grounding.Enforce)
	s.readAnswer("measure_window", `{"datasource":"prom","query":"max(request_duration_seconds)","stat":"max","step":"30s"}`, measured, &Turn{ID: "t1", Calls: 1})
	finer := strings.Replace(strings.Replace(measured, "0.5234123", "0.61", 1), "[2h:30s]", "[2h:15s]", 1)
	s.readAnswer("measure_window", `{"datasource":"prom","query":"max(request_duration_seconds)","stat":"max","step":"15s"}`, finer, &Turn{ID: "t2", Calls: 1})
	const save = `{"markdown":"Checkout p99 peaked at {{f1:ms}}.","answers":[{"quantity":"peak p99","fact":"f1"},{"quantity":"peak p99","fact":"f2"}]}`

	message := sendBackMessage(t, s.admitSave(save, &Turn{ID: "t3", Calls: 1}))
	if !strings.Contains(message, `"answers: peak p99 = f1 or f2": one quantity takes one fact`) {
		t.Errorf("the send-back does not name the conflict:\n%s", message)
	}
	clean := s.admitSave(`{"markdown":"Checkout p99 peaked at {{f2:ms}}.","answers":[{"quantity":"peak p99","fact":"f2"}]}`, &Turn{ID: "t4", Calls: 1})
	if clean.answer != nil || clean.grounding.unverified != nil {
		t.Errorf("the settled save = %+v, %+v", clean.answer, clean.grounding.unverified)
	}
}

const slowPath = `{"from":"2026-11-20T09:00:00Z","to":"2026-11-20T11:00:00Z","threshold":"route p50","listed_traces":40,"complete":true,"sample":false,"routes":[{"route":"GET /cart","threshold_ms":120,"slow_traces":7,"examined":40,"top_self_time":[],"services":[],"example":{"trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","duration_ms":1312.75}}],"note":""}`

func TestATypedFigureNoReadGaveIsSentBackWithTheFactItRounds(t *testing.T) {
	s := groundingScript(t, grounding.Enforce)
	s.readAnswer("measure_window", `{"datasource":"prom","query":"max(request_duration_seconds)","stat":"max"}`, measured, &Turn{ID: "t1", Calls: 1})
	save := `{"markdown":"Checkout p99 peaked at 0.5 s, and 2,950 orders failed over the last 6 hours on HTTP 503."}`

	message := sendBackMessage(t, s.admitSave(save, &Turn{ID: "t2", Calls: 1}))
	for _, want := range []string{
		`figure "0.5 s": more than 1% from f1, 0.5234123: cite {{f1}}`,
		`figure "2,950": read it, correct it, or remove it`,
	} {
		if !strings.Contains(message, want) {
			t.Errorf("the send-back lacks %q:\n%s", want, message)
		}
	}
	if strings.Contains(message, `"6 hours"`) || strings.Contains(message, `"503"`) {
		t.Errorf("the send-back names a window or a status code:\n%s", message)
	}
}

func TestACutTraceIDIsSentBackWithTheWholeID(t *testing.T) {
	s := groundingScript(t, grounding.Enforce)
	s.readAnswer("trace_slow_path", `{"datasource":"tempo","route":"GET /cart"}`, slowPath, &Turn{ID: "t1", Calls: 1})

	message := sendBackMessage(t, s.admitSave(`{"markdown":"GET /cart had 7 slow traces, such as 4bf92f35, which took {{f8}}."}`, &Turn{ID: "t2", Calls: 1}))
	if !strings.Contains(message, `trace id "4bf92f35": an id cut short: write the whole id 4bf92f3577b34da6a3ce929d0e0e4736`) {
		t.Errorf("the send-back does not name the cut id:\n%s", message)
	}
	ids := s.sess.state.Grounding.Facts
	trace, duration := ids[len(ids)-2], ids[len(ids)-1]
	clean := s.admitSave(`{"markdown":"GET /cart had {{f3}} slow traces, such as {{`+trace.ID+`}}, which took {{`+duration.ID+`:s}}.","answers":[{"quantity":"example trace","fact":"`+trace.ID+`"},{"quantity":"its duration","fact":"`+duration.ID+`"}]}`, &Turn{ID: "t3", Calls: 1})
	if clean.answer != nil || clean.grounding.unverified != nil {
		t.Errorf("the repaired save = %+v, %+v", clean.answer, clean.grounding.unverified)
	}
	if rendered, _ := clean.grounding.ledger.Render("{{" + duration.ID + ":s}} in {{" + trace.ID + "}}"); rendered != "1.31275 s in 4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("rendered = %q", rendered)
	}
}

func TestReportRecordsABrokenCitationWithoutSendingItBack(t *testing.T) {
	s := groundingScript(t, grounding.Report)
	s.readAnswer("measure_window", `{"datasource":"prom","query":"max(request_duration_seconds)","stat":"max"}`, measured, &Turn{ID: "t1", Calls: 1})
	got := s.admitSave(`{"markdown":"Checkout p99 peaked at {{f1:1}}."}`, &Turn{ID: "t2", Calls: 1})
	if got.answer != nil || len(got.grounding.unverified) != 1 || got.grounding.unverified[0].Text != "{{f1:1}}" {
		t.Errorf("report = %+v, %+v", got.answer, got.grounding.unverified)
	}
}

func TestASampledLogFacetCountIsCitedAsALowerBound(t *testing.T) {
	s := groundingScript(t, grounding.Enforce)
	const facets = `{"sampled":true,"lines_sampled":2000,"fields":[{"name":"level","kind":"field","exact":false,"values":[{"value":"error","count":37}],"truncated":false}],"truncated":false}`
	answer := s.readAnswer("log_facets", `{"datasource":"loki","query":"{service_name=\"cart\"}"}`, facets, &Turn{ID: "t1", Calls: 1})
	if !strings.Contains(string(answer.Content), `{"value":"error","count":37,"facts":{"count":"f2"}}`) {
		t.Fatalf("the facet count lacks its fact id: %s", answer.Content)
	}
	got := s.admitSave(`{"markdown":"Cart logged {{f2}} errors."}`, &Turn{ID: "t2", Calls: 1})
	if got.answer != nil || got.grounding.unverified != nil {
		t.Fatalf("the citation was refused or flagged: %+v %+v", got.answer, got.grounding.unverified)
	}
	if rendered, _ := got.grounding.ledger.Render("Cart logged {{f2}} errors."); rendered != "Cart logged at least 37 errors." {
		t.Errorf("rendered %q", rendered)
	}
}

func TestWithGroundingOffAReadIsReturnedAsTheToolGaveIt(t *testing.T) {
	s := groundingScript(t, grounding.Off)
	if answer := s.readAnswer("measure_window", `{"datasource":"prom","query":"max(request_duration_seconds)","stat":"max"}`, measured, &Turn{ID: "t1", Calls: 1}); string(answer.Content) != measured {
		t.Errorf("content = %s, want it unchanged", answer.Content)
	}
}
