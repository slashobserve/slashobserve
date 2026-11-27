package guard

import (
	"encoding/json"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
)

func writeCall(name, input string) Call {
	return Call{Name: name, Input: json.RawMessage(input)}
}

func admitWrite(s *State, name, input string) Admission {
	s.BeginTurn(false)
	return s.Admit(0, writeCall(name, input))
}

func TestWriteStateSurvivesAReload(t *testing.T) {
	s := New(tools.ModeCreate)
	admitWrite(&s, "add_block", `{"n":1}`)
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var back State
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if adm := admitWrite(&back, "add_block", `{"n":1}`); adm.Verdict != Refuse || back.Writes != 1 {
		t.Fatalf("reloaded repeat = %+v writes %d", adm, back.Writes)
	}
}
