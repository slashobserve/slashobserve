package grounding

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/findings"
)

func causeRecord(t *testing.T) *Record {
	t.Helper()
	r := NewRecord(Enforce)
	r.AddRead([]byte(`{"series":[{"labels":{"service":"cart","feature_flag":"cartFailure"},"values":[[1,"2"]]}]}`), []string{"checkout"},
		[]Range{{From: time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC), To: time.Date(2026, 11, 20, 11, 0, 0, 0, time.UTC)}})
	r.IssueFacts("measure_window", nil, []byte(`{"stat":"max","from":"2026-11-20T09:00:00Z","to":"2026-11-20T11:00:00Z","query":"rate(errors[5m])","values":[{"labels":{"service":"cart"},"value":0.52,"at":"2026-11-20T10:05:00Z"}]}`), nil)
	r.IssueFacts("trace_error_origins", nil, []byte(`{"failing_traces":40,"groups":[{"service":"cart","traces":38,"share":0.95,"example_trace_id":"4bf92f3577b34da6a3ce929d0e0e4736"}]}`), nil)
	return r
}

func TestCauseSignalsComeFromTheLedgerTheNamesAndTheRangesTheRunRead(t *testing.T) {
	r := causeRecord(t)
	input := `{"markdown":"m","ruled_out":["The database: latency held at {{f1}}."],"causes":[
		{"component":{"kind":"service","name":"cart"},"kind":"config_or_flag","mechanism":"m","onset":"2026-11-20T10:03:00Z",
		 "trigger":{"kind":"feature_flag","key":"cartFailure","value":"on"},"evidence":{"facts":["f1","f9"],"trace_ids":["4BF92F3577B34DA6A3CE929D0E0E4736"]}},
		{"component":{"kind":"service","name":"payments"},"kind":"unknown","mechanism":"m","onset":"2026-11-21T10:00:00Z","evidence":{}}]}`
	got := r.CauseSignals(json.RawMessage(input))
	want := []findings.Signals{
		{Kinds: []string{findings.SignalMetrics, findings.SignalTraces}, Component: true, OnsetRead: true, OnsetFact: true, TriggerRead: true, RuledOut: true},
		{RuledOut: true},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("signals\n%+v\nwant\n%+v", got, want)
	}
	if findings.Score(got[0]) != 0.9 || findings.Score(got[1]) != 0.05 {
		t.Errorf("scores %.2f and %.2f", findings.Score(got[0]), findings.Score(got[1]))
	}
}

func TestALegacyRootCauseIsScoredWithTheFactsTheFindingCites(t *testing.T) {
	r := causeRecord(t)
	input := `{"markdown":"Cart errors peaked at {{f1}}.","onset":"2026-11-20T10:05:00Z","root_cause":{"component":"cart","kind":"deploy","summary":"A deploy broke cart."}}`
	got := r.CauseSignals(json.RawMessage(input))
	if len(got) != 1 || !slices.Equal(got[0].Kinds, []string{findings.SignalMetrics}) || !got[0].Component || !got[0].OnsetFact {
		t.Fatalf("signals %+v", got)
	}
	if r.causeProblems(json.RawMessage(input)) != nil {
		t.Error("a legacy root cause is checked twice")
	}
}

func TestNoRecordGivesNoSignals(t *testing.T) {
	var r *Record
	if r.CauseSignals(json.RawMessage(`{"causes":[{}]}`)) != nil {
		t.Error("an off record gave signals")
	}
}

func TestEveryCauseIsCheckedLikeTheRootCause(t *testing.T) {
	r := causeRecord(t)
	input := `{"markdown":"m","causes":[
		{"component":{"kind":"service","name":"cart"},"kind":"deploy","mechanism":"m","onset":"2026-11-20T10:03:00Z","evidence":{"facts":["f1"]}},
		{"component":{"kind":"service","name":"inventory"},"kind":"config_or_flag","mechanism":"Broken by {{f77}}.","onset":"2026-11-22T10:00:00Z",
		 "trigger":{"kind":"config_value","key":"MAX_POOL"},"evidence":{"facts":["f9"],"trace_ids":["aaaaaaaaaaaaaaaa"]}}]}`
	got := r.Review(json.RawMessage(input), "")
	var kinds []string
	for _, p := range got {
		kinds = append(kinds, p.Kind+" "+p.Text)
	}
	want := []string{
		"fact {{f77}}",
		"component inventory",
		"trigger MAX_POOL",
		"onset 2026-11-22T10:00:00Z",
		"fact causes[1].evidence.facts: f9",
		"trace_id aaaaaaaaaaaaaaaa",
	}
	if !slices.Equal(kinds, want) {
		t.Errorf("problems\n%q\nwant\n%q", kinds, want)
	}
	if text := SendBackProblems(got); !strings.Contains(text, `- trigger key, not a name this run saw, "MAX_POOL": read where it is set`) {
		t.Errorf("send back %s", text)
	}
}
