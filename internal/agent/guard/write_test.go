package guard

import (
	"encoding/json"
	"reflect"
	"strconv"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func writeCall(name, input string) Call {
	return Call{Name: name, Input: json.RawMessage(input)}
}

func admitWrite(s *State, name, input string) Admission {
	s.BeginTurn(false)
	return s.Admit(0, writeCall(name, input))
}

func TestWriteIsAdmittedAfterTheReadWindowClosed(t *testing.T) {
	s := New(tools.ModeCreate)
	s.Window.Calls = maxCallsBeforeSave
	s.Building = true
	s.BeginTurn(false)
	if read := s.Admit(0, writeCall("get_panels", `{}`)); read.Verdict != Refuse {
		t.Fatalf("read after the window = %+v", read)
	}
	write := s.Admit(1, writeCall("add_block", `{"observation_id":"o"}`))
	if write.Verdict != Dispatch || write.Phase != tools.PhaseWrite {
		t.Fatalf("write = %+v", write)
	}
}

func TestWriteCountsAsAToolCallAndAnAnsweredCall(t *testing.T) {
	s := New(tools.ModeCreate)
	s.BeginTurn(false)
	s.Admit(0, writeCall("add_block", `{"n":1}`))
	if s.Counts.ToolCalls != 1 || s.Turn.Answered != 1 || s.Writes != 1 {
		t.Fatalf("counts = %+v answered %d writes %d", s.Counts, s.Turn.Answered, s.Writes)
	}
}

func TestEightWritesRunAndTheNinthIsRefusedAsTooManyWrites(t *testing.T) {
	s := New(tools.ModeCreate)
	for i := range maxWritesPerAttempt {
		if adm := admitWrite(&s, "add_block", `{"n":`+strconv.Itoa(i)+`}`); adm.Verdict != Dispatch {
			t.Fatalf("write %d = %+v", i+1, adm)
		}
	}
	adm := admitWrite(&s, "add_block", `{"n":99}`)
	if adm.Verdict != Refuse || adm.Step != "Refused add_block: 8 writes already ran" {
		t.Fatalf("ninth write = %+v", adm)
	}
	var body map[string]string
	if err := json.Unmarshal(adm.Content, &body); err != nil || body["code"] != "too_many_writes" {
		t.Fatalf("content = %s", adm.Content)
	}
	if s.Counts.ToolCalls != maxWritesPerAttempt+1 {
		t.Fatalf("tool calls = %d", s.Counts.ToolCalls)
	}
}

func TestAnIdenticalWriteIsRefusedWhateverTheKeyOrder(t *testing.T) {
	s := New(tools.ModeCreate)
	if adm := admitWrite(&s, "update_observation", `{"observation_id":"o","title":"t"}`); adm.Verdict != Dispatch {
		t.Fatalf("first = %+v", adm)
	}
	adm := admitWrite(&s, "update_observation", `{ "title": "t", "observation_id": "o" }`)
	if adm.Verdict != Refuse || adm.Step != "Refused update_observation: the same write was sent again" {
		t.Fatalf("repeat = %+v", adm)
	}
	var body map[string]string
	if err := json.Unmarshal(adm.Content, &body); err != nil || body["code"] != "repeated_write" {
		t.Fatalf("content = %s", adm.Content)
	}
	if other := admitWrite(&s, "update_observation", `{"observation_id":"o","title":"u"}`); other.Verdict != Dispatch {
		t.Fatalf("a different input = %+v", other)
	}
	if s.Writes != 2 {
		t.Fatalf("writes = %d", s.Writes)
	}
}

func TestARefusedWriteDoesNotCountTowardTheCap(t *testing.T) {
	s := New(tools.ModeCreate)
	admitWrite(&s, "add_block", `{"n":1}`)
	admitWrite(&s, "add_block", `{"n":1}`)
	if s.Writes != 1 {
		t.Fatalf("writes = %d", s.Writes)
	}
}

func TestSettleOfAWriteNeverResetsTheRejectionCount(t *testing.T) {
	s := New(tools.ModeCreate)
	s.Rejections, s.RestrictedRefusals = 2, 1
	c := writeCall("add_block", `{"n":1}`)
	s.BeginTurn(false)
	s.Admit(0, c)
	settled, err := s.Settle(c, tools.Result{}, []byte(`{"ok":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(settled, Settled{}) || s.Rejections != 2 || s.RestrictedRefusals != 1 || s.Turn.Progress || s.Turn.Rejected {
		t.Fatalf("settled = %+v, state = %+v", settled, s)
	}
	if len(s.Reads) != 0 {
		t.Fatalf("a write was recorded as a read: %+v", s.Reads)
	}
}

func TestAWriteIsNeverReplayedOrRefusedAsARepeatedRead(t *testing.T) {
	s := New(tools.ModeCreate)
	for i := range 4 {
		s.BeginTurn(false)
		c := writeCall("add_block", `{"n":`+strconv.Itoa(i)+`}`)
		if adm := s.Admit(0, c); adm.Verdict != Dispatch {
			t.Fatalf("write %d = %+v", i, adm)
		}
		if _, err := s.Settle(c, tools.Result{}, []byte(`{}`)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTwoTurnsOfRefusedWritesStopTheRunWithTheWriteReason(t *testing.T) {
	s := New(tools.ModeCreate)
	s.Writes = maxWritesPerAttempt
	turn := func() *Stop {
		s.BeginTurn(false)
		s.Admit(0, writeCall("add_block", `{"n":1}`))
		_, stop := s.EndTurn()
		return stop
	}
	if turn() != nil {
		t.Fatal("stopped after one refused turn")
	}
	stop := turn()
	if stop == nil || stop.Failure != revisions.FailureNoProgress || stop.Step != "Stopped: the model kept writing after 8 writes" {
		t.Fatalf("stop = %+v", stop)
	}
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

func TestARestartKeepsTheWriteCountOfTheAttempt(t *testing.T) {
	s := New(tools.ModeRevise)
	admitWrite(&s, "add_block", `{"n":1}`)
	s.Restart()
	if s.Writes != 1 || !s.WriteKeys[Key("add_block", json.RawMessage(`{"n":1}`))] {
		t.Fatalf("writes = %d keys = %v", s.Writes, s.WriteKeys)
	}
}

func TestAFailedWriteFreesItsCapAndItsKeySoItCanBeRetried(t *testing.T) {
	s := New(tools.ModeCreate)
	c := writeCall("add_block", `{"n":1}`)
	s.BeginTurn(false)
	s.Admit(0, c)
	if _, err := s.Settle(c, tools.Result{IsError: true}, []byte(`{"code":"invalid_request"}`)); err != nil {
		t.Fatal(err)
	}
	if s.Writes != 0 || len(s.WriteKeys) != 0 {
		t.Fatalf("writes = %d keys = %v", s.Writes, s.WriteKeys)
	}
	if adm := admitWrite(&s, "add_block", `{"n":1}`); adm.Verdict != Dispatch {
		t.Fatalf("retry = %+v", adm)
	}
}

func TestForgetWriteLeavesAnotherWritesKeyAlone(t *testing.T) {
	s := New(tools.ModeCreate)
	admitWrite(&s, "add_block", `{"n":1}`)
	s.ForgetWrite(writeCall("add_block", `{"n":2}`))
	if s.Writes != 1 {
		t.Fatalf("writes = %d", s.Writes)
	}
}

func TestWriteKeysKeepLargeIntegersApart(t *testing.T) {
	a := writeKey(writeCall("update_block", `{"version":9007199254740993}`))
	b := writeKey(writeCall("update_block", `{"version":9007199254740992}`))
	if a == b {
		t.Fatal("two different integers made one key")
	}
	if writeKey(writeCall("update_block", `{"b":1,"a":2}`)) != writeKey(writeCall("update_block", `{"a":2,"b":1}`)) {
		t.Fatal("key order changed the key")
	}
}
