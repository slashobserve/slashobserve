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

func TestWithGroundingOffAReadIsReturnedAsTheToolGaveIt(t *testing.T) {
	s := groundingScript(t, grounding.Off)
	if answer := s.readAnswer("measure_window", `{"datasource":"prom","stat":"max"}`, measured, &Turn{ID: "t1", Calls: 1}); string(answer.Content) != measured {
		t.Errorf("content = %s, want it unchanged", answer.Content)
	}
}
