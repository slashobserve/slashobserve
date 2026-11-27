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
	answer := s.readAnswer("measure_window", `{"datasource":"prom","stat":"max"}`, measured, &Turn{ID: "t1", Calls: 1})
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

	again := s.readAnswer("measure_window", `{"datasource":"prom","stat":"max","step":"30s"}`, measured, &Turn{ID: "t2", Calls: 1})
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
	s.readAnswer("measure_window", `{"datasource":"prom","stat":"max"}`, measured, &Turn{ID: "t1", Calls: 1})

	got := s.admitSave(`{"markdown":"Checkout p99 peaked at {{f1:ms}}."}`, &Turn{ID: "t2", Calls: 1})
	if got.answer != nil || got.unverified != nil {
		t.Fatalf("a clean save was refused or flagged: %+v %+v", got.answer, got.unverified)
	}
	if rendered, problems := got.ledger.Render("{{f1:ms:3}}"); rendered != "523 ms" || problems != nil {
		t.Errorf("the save's ledger rendered %q, %+v", rendered, problems)
	}
}

func TestAnUnknownFactIsSentBackOnceAndTheSecondSaveStillSaves(t *testing.T) {
	s := groundingScript(t, grounding.Enforce)
	s.readAnswer("measure_window", `{"datasource":"prom","stat":"max"}`, measured, &Turn{ID: "t1", Calls: 1})
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
	if len(second.unverified) != 1 || second.unverified[0].Kind != "fact" || second.unverified[0].Text != "{{f12}}" {
		t.Errorf("unverified = %+v, want the cited fact", second.unverified)
	}
	if rendered, _ := second.ledger.Render("{{f12}}"); rendered != grounding.UnreadMarker {
		t.Errorf("the unknown fact renders as %q", rendered)
	}
}

func TestReportRecordsABrokenCitationWithoutSendingItBack(t *testing.T) {
	s := groundingScript(t, grounding.Report)
	s.readAnswer("measure_window", `{"datasource":"prom","stat":"max"}`, measured, &Turn{ID: "t1", Calls: 1})
	got := s.admitSave(`{"markdown":"Checkout p99 peaked at {{f1:1}}."}`, &Turn{ID: "t2", Calls: 1})
	if got.answer != nil || len(got.unverified) != 1 || got.unverified[0].Text != "{{f1:1}}" {
		t.Errorf("report = %+v, %+v", got.answer, got.unverified)
	}
}

func TestWithGroundingOffAReadIsReturnedAsTheToolGaveIt(t *testing.T) {
	s := groundingScript(t, grounding.Off)
	if answer := s.readAnswer("measure_window", `{"datasource":"prom","stat":"max"}`, measured, &Turn{ID: "t1", Calls: 1}); string(answer.Content) != measured {
		t.Errorf("content = %s, want it unchanged", answer.Content)
	}
}
