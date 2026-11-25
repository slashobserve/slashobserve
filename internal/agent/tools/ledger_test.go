package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func ledgerRegistry() *Registry { return New(Deps{LedgerTool: true}) }

func callLedger(t *testing.T, mode Mode, input string) Result {
	t.Helper()
	ctx := WithRun(auth.WithPrincipal(context.Background(), auth.Principal{Kind: auth.KindUser, Role: auth.RoleViewer}), Run{Mode: mode, RequestID: otherID})
	res, err := ledgerRegistry().Call(ctx, nameNoteHypotheses, json.RawMessage(input))
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestTheLedgerToolIsRegisteredOnlyWhenTheLoopIsOn(t *testing.T) {
	for _, tool := range New(Deps{}).List() {
		if tool.Name == nameNoteHypotheses {
			t.Fatal("note_hypotheses is registered with the loop off")
		}
	}
	var found *Tool
	for _, tool := range ledgerRegistry().List() {
		if tool.Name == nameNoteHypotheses {
			found = &tool
		}
	}
	if found == nil {
		t.Fatal("note_hypotheses is not registered with the loop on")
	}
	if !found.AgentOnly || !found.ReadOnly || found.Answers || found.MCPOnly {
		t.Errorf("flags = %+v", *found)
	}
	for _, mode := range []Mode{ModeCreate, ModeRevise, ModeSuggest} {
		if found.OfferedIn(mode) {
			t.Errorf("offered in %s runs", mode)
		}
	}
	if !found.OfferedIn(ModeInvestigate) {
		t.Error("not offered in investigate runs")
	}
	if got := Phase(nameNoteHypotheses); got != PhaseLedger {
		t.Errorf("phase = %q", got)
	}
}

func TestTurningTheLoopOnAddsOnlyTheLedgerTool(t *testing.T) {
	off := map[string]Tool{}
	for _, tool := range New(Deps{}).List() {
		off[tool.Name] = tool
	}
	for _, tool := range ledgerRegistry().List() {
		before, ok := off[tool.Name]
		if !ok {
			if tool.Name != nameNoteHypotheses {
				t.Errorf("the loop added %s", tool.Name)
			}
			continue
		}
		if string(before.AgentInputSchema) != string(tool.AgentInputSchema) || before.AgentDescription != tool.AgentDescription || before.Description != tool.Description {
			t.Errorf("the loop changed %s", tool.Name)
		}
	}
	if len(ledgerRegistry().List()) != len(off)+1 {
		t.Errorf("tool counts differ by more than one")
	}
}

func TestTheLedgerToolIsRefusedOutsideAnInvestigation(t *testing.T) {
	res := callLedger(t, ModeCreate, `{"hypotheses":[{"id":"h1","text":"a","status":"open"}]}`)
	if !res.IsError || !strings.Contains(toJSON(t, res.Content), codeNotInThisRun) || res.Ledger != nil {
		t.Fatalf("result = %+v", res)
	}
}

func TestTheLedgerToolOutsideARunIsRefusedWithItsOwnHint(t *testing.T) {
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{Kind: auth.KindUser, Role: auth.RoleViewer})
	res, err := ledgerRegistry().Call(ctx, nameNoteHypotheses, json.RawMessage(`{"hypotheses":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	text := toJSON(t, res.Content)
	if !res.IsError || !strings.Contains(text, "note_hypotheses runs inside the agent; outside an investigate run, keep the hypotheses in your own notes") {
		t.Fatalf("result = %s", text)
	}
}

func toJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestAValidLedgerIsReturnedWithItsStepAndCounts(t *testing.T) {
	res := callLedger(t, ModeInvestigate, `{"hypotheses":[
		{"id":"pool","text":"The  checkout\npool is exhausted","status":"supported","evidence":["pool_in_use 50 of 50 at 10:05"],"next_check":"ignored"},
		{"id":"deploy","text":"The 10:00 deploy broke it","status":"ruled_out","evidence":["no deploy annotation in the window"]},
		{"id":"db","text":"The database is slow","status":"open","next_check":"read db latency"}]}`)
	if res.IsError {
		t.Fatalf("result = %s", toJSON(t, res.Content))
	}
	if res.Step != "3 hypotheses: 1 supported, 1 ruled out, 1 open" {
		t.Errorf("step = %q", res.Step)
	}
	want := []revisions.Hypothesis{
		{ID: "pool", Text: "The checkout pool is exhausted", Status: "supported", Evidence: []string{"pool_in_use 50 of 50 at 10:05"}},
		{ID: "deploy", Text: "The 10:00 deploy broke it", Status: "ruled_out", Evidence: []string{"no deploy annotation in the window"}},
		{ID: "db", Text: "The database is slow", Status: "open", Next: "read db latency"},
	}
	if toJSON(t, res.Ledger) != toJSON(t, want) {
		t.Errorf("ledger = %s", toJSON(t, res.Ledger))
	}
	if got := toJSON(t, res.Content); got != `{"hypotheses":3,"open":1,"supported":1,"ruled_out":1}` {
		t.Errorf("content = %s", got)
	}
}

func TestALedgerThatBreaksARuleIsRejectedWithTheReason(t *testing.T) {
	long := func(n int) string { return strings.Repeat("x", n) }
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"no hypotheses", `{"hypotheses":[]}`, "1 to 6 hypotheses"},
		{"seven", ledgerOf(7, `"text":"a","status":"open"`), "1 to 6 hypotheses"},
		{"a duplicate id", `{"hypotheses":[{"id":"h1","text":"a","status":"open"},{"id":"h1","text":"b","status":"open"}]}`, "used twice"},
		{"an id with a space", `{"hypotheses":[{"id":"h 1","text":"a","status":"open"}]}`, "id must be"},
		{"an id too long", `{"hypotheses":[{"id":"` + long(17) + `","text":"a","status":"open"}]}`, "id must be"},
		{"no text", `{"hypotheses":[{"id":"h1","text":" ","status":"open"}]}`, "text must be"},
		{"text too long", `{"hypotheses":[{"id":"h1","text":"` + long(161) + `","status":"open"}]}`, "text must be"},
		{"an unknown status", `{"hypotheses":[{"id":"h1","text":"a","status":"maybe"}]}`, "status must be"},
		{"supported with no evidence", `{"hypotheses":[{"id":"h1","text":"a","status":"supported"}]}`, "no evidence line"},
		{"ruled out with no evidence", `{"hypotheses":[{"id":"h1","text":"a","status":"ruled_out","evidence":[]}]}`, "no evidence line"},
		{"four evidence lines", `{"hypotheses":[{"id":"h1","text":"a","status":"open","evidence":["a","b","c","d"]}]}`, "at most 3 lines"},
		{"an empty evidence line", `{"hypotheses":[{"id":"h1","text":"a","status":"open","evidence":[" "]}]}`, "evidence[0]"},
		{"an evidence line too long", `{"hypotheses":[{"id":"h1","text":"a","status":"open","evidence":["` + long(201) + `"]}]}`, "evidence[0]"},
		{"a next check too long", `{"hypotheses":[{"id":"h1","text":"a","status":"open","next_check":"` + long(161) + `"}]}`, "next_check"},
		{"an unknown field", `{"hypotheses":[{"id":"h1","text":"a","status":"open"}],"extra":1}`, "extra"},
		{"over the size limit", ledgerOf(6, `"text":"`+long(160)+`","status":"open","evidence":["`+long(200)+`","`+long(200)+`","`+long(200)+`"],"next_check":"`+long(160)+`"`), "byte limit"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := callLedger(t, ModeInvestigate, tc.input)
			if !res.IsError || res.Ledger != nil {
				t.Fatalf("accepted: %s", toJSON(t, res.Content))
			}
			if body := toJSON(t, res.Content); !strings.Contains(body, tc.want) {
				t.Fatalf("message %s lacks %q", body, tc.want)
			}
		})
	}
}

func ledgerOf(n int, fields string) string {
	items := make([]string, n)
	for i := range items {
		items[i] = `{"id":"h` + string(rune('a'+i)) + `",` + fields + `}`
	}
	return `{"hypotheses":[` + strings.Join(items, ",") + `]}`
}

func TestAFullLedgerWithinTheSizeLimitIsAccepted(t *testing.T) {
	res := callLedger(t, ModeInvestigate, ledgerOf(6, `"text":"`+strings.Repeat("x", 100)+`","status":"supported","evidence":["`+strings.Repeat("y", 100)+`"]`))
	if res.IsError || len(res.Ledger) != 6 {
		t.Fatalf("result = %s", toJSON(t, res.Content))
	}
}

func TestTheLedgerToolTextStaysWithinTheAgentCap(t *testing.T) {
	for _, tool := range ledgerRegistry().List() {
		if tool.Name != nameNoteHypotheses {
			continue
		}
		offered := tool.ForAgent()
		if size := len(offered.Description) + len(offered.InputSchema); size > agentToolTextCap {
			t.Fatalf("agent tool text is %d bytes, over %d", size, agentToolTextCap)
		}
		if !json.Valid(offered.InputSchema) {
			t.Fatal("schema is not valid JSON")
		}
	}
}
