package grounding

import (
	"encoding/json"
	"fmt"
	"maps"
	"strings"
	"testing"
)

func issued(t *testing.T, r *Record, tool, input, content string) map[string]any {
	t.Helper()
	out := r.IssueFacts(tool, []byte(input), []byte(content), nil)
	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("IssueFacts(%s) returned invalid JSON %s: %v", tool, out, err)
	}
	return parsed
}

func TestAMeasurementGetsAFactPerValueWithItsWindowLabelsAndQuery(t *testing.T) {
	r := NewRecord(Report)
	content := `{"stat":"max","from":"2026-11-20T09:00:00Z","to":"2026-11-20T11:00:00Z","sampled":"30s","query":"max_over_time(x[2h:30s])","empty":false,"values":[{"labels":{"service":"checkout"},"value":0.5234123,"at":"2026-11-20T10:05:00Z"}]}`
	out := string(r.IssueFacts("measure_window", nil, []byte(content), nil))
	want := `{"stat":"max","from":"2026-11-20T09:00:00Z","to":"2026-11-20T11:00:00Z","sampled":"30s","query":"max_over_time(x[2h:30s])","empty":false,"values":[{"labels":{"service":"checkout"},"value":0.5234123,"at":"2026-11-20T10:05:00Z","facts":{"value":"f1"}}]}`
	if out != want {
		t.Fatalf("content =\n%s\nwant\n%s", out, want)
	}
	f, ok := r.Fact("f1")
	if !ok {
		t.Fatal("f1 is not in the ledger")
	}
	wantFact := Fact{ID: "f1", Text: "0.5234123", Value: 0.5234123, Kind: FactNumber, Of: "measure_window max values[] value",
		Labels: map[string]string{"service": "checkout"}, At: "2026-11-20T10:05:00Z", From: "2026-11-20T09:00:00Z", To: "2026-11-20T11:00:00Z", Query: "max_over_time(x[2h:30s])"}
	if fmt.Sprint(f) != fmt.Sprint(wantFact) {
		t.Errorf("fact =\n%+v\nwant\n%+v", f, wantFact)
	}
}

func TestTheSameValueReadAgainKeepsItsFactID(t *testing.T) {
	r := NewRecord(Report)
	content := `{"op":"total","from":"2026-11-20T09:00:00Z","to":"2026-11-20T11:00:00Z","query":"sum(increase(x[2h]))","values":[{"value":1284}]}`
	r.IssueFacts("measure_aggregate", nil, []byte(content), nil)
	r.IssueFacts("measure_aggregate", nil, []byte(content), nil)
	other := strings.Replace(content, "1284", "1290", 1)
	r.IssueFacts("measure_aggregate", nil, []byte(other), nil)
	if len(r.Facts) != 2 || r.Facts[0].ID != "f1" || r.Facts[1].ID != "f2" {
		t.Errorf("facts = %+v, want f1 kept and f2 for the new value", r.Facts)
	}
}

func TestTraceToolsGiveCountsDurationsAndFullTraceIDs(t *testing.T) {
	r := NewRecord(Enforce)
	content := `{"from":"2026-11-20T09:00:00Z","to":"2026-11-20T11:00:00Z","threshold":"route p50","listed_traces":40,"complete":true,"sample":false,"routes":[{"route":"GET /cart","threshold_ms":120,"slow_traces":7,"examined":40,"top_self_time":[{"service":"cart","operation":"redis GET","self_time_ms":812.4,"share":0.61,"traces":7}],"services":[{"service":"cart","traces":7}],"example":{"trace_id":"4BF92F3577B34DA6A3CE929D0E0E4736","duration_ms":1312.75}}],"note":""}`
	parsed := issued(t, r, "trace_slow_path", "{}", content)
	example := parsed["routes"].([]any)[0].(map[string]any)["example"].(map[string]any)
	ids := example["facts"].(map[string]any)
	trace, _ := r.Fact(ids["trace_id"].(string))
	duration, _ := r.Fact(ids["duration_ms"].(string))
	if trace.Kind != FactTraceID || trace.Text != "4bf92f3577b34da6a3ce929d0e0e4736" || !maps.Equal(trace.Labels, map[string]string{"route": "GET /cart"}) {
		t.Errorf("trace fact = %+v", trace)
	}
	if duration.Kind != FactDuration || duration.Unit != "ms" || duration.Value != 1312.75 || duration.Labels["trace_id"] != "4BF92F3577B34DA6A3CE929D0E0E4736" {
		t.Errorf("duration fact = %+v", duration)
	}
	self := parsed["routes"].([]any)[0].(map[string]any)["top_self_time"].([]any)[0].(map[string]any)["facts"].(map[string]any)
	share, _ := r.Fact(self["share"].(string))
	if share.Kind != FactRatio || share.Labels["operation"] != "redis GET" || share.Labels["route"] != "GET /cart" {
		t.Errorf("share fact = %+v", share)
	}
	if count := len(r.Facts); count != 10 {
		t.Errorf("facts = %d, want 10", count)
	}
}

func TestAGroupValueBecomesALabelUnderTheGroupKey(t *testing.T) {
	r := NewRecord(Report)
	parsed := issued(t, r, "trace_stats", `{"query":"{status=error}"}`, `{"step_ms":60000,"spans":10,"errors":2,"buckets":[{"start":"x","spans":4,"p99_ms":3}],"group_by":"service.name","groups":[{"value":"cart","spans":10,"errors":2,"p50_ms":1.5,"p95_ms":null,"p99_ms":9.25}]}`)
	ids := parsed["groups"].([]any)[0].(map[string]any)["facts"].(map[string]any)
	if _, ok := ids["p95_ms"]; ok {
		t.Error("a null quantile got a fact")
	}
	p99, _ := r.Fact(ids["p99_ms"].(string))
	if p99.Labels["service.name"] != "cart" || p99.Query != "{status=error}" {
		t.Errorf("p99 fact = %+v", p99)
	}
	if _, ok := parsed["buckets"].([]any)[0].(map[string]any)["facts"]; ok {
		t.Error("a time bucket got facts")
	}
}

func TestQueryRangeSeriesGiveTheirExtremesAndTheExecutedQuery(t *testing.T) {
	r := NewRecord(Report)
	issued(t, r, "query_range", `{"query":"rate(x[5m])"}`, `{"kind":"range","from":"2026-11-20T09:00:00Z","to":"2026-11-20T11:00:00Z","executed":[{"ref":"A","text":"rate(x{job=\"a\"}[5m])"}],"series":[{"name":"x","labels":{"job":"a"},"points":5,"non_null":5,"min":1,"max":4.5,"last":2}]}`)
	if len(r.Facts) != 3 || r.Facts[1].Of != "query_range series[] max" || r.Facts[1].Query != `rate(x{job="a"}[5m])` {
		t.Errorf("facts = %+v", r.Facts)
	}
}

func TestOtherToolsAndErrorsAreLeftAlone(t *testing.T) {
	r := NewRecord(Report)
	for tool, content := range map[string]string{
		"list_metrics":   `{"metrics":["x"],"total":640}`,
		"measure_window": `not json`,
		"query_range":    `{"empty":true}`,
	} {
		if got := string(r.IssueFacts(tool, nil, []byte(content), nil)); got != content {
			t.Errorf("%s: content = %s, want it unchanged", tool, got)
		}
	}
	var off *Record
	if got := string(off.IssueFacts("measure_window", nil, []byte(`{"values":[{"value":1}]}`), nil)); got != `{"values":[{"value":1}]}` {
		t.Errorf("an off record changed the content: %s", got)
	}
	if len(r.Facts) != 0 {
		t.Errorf("facts = %+v", r.Facts)
	}
}

func TestTheLedgerDropsItsOldestFactsFirst(t *testing.T) {
	r := NewRecord(Report)
	for i := range MaxFacts + 5 {
		r.IssueFacts("measure_aggregate", nil, fmt.Appendf(nil, `{"op":"total","values":[{"value":%d}]}`, i), nil)
	}
	if len(r.Facts) != MaxFacts || r.Facts[0].ID != "f6" || r.NextFact != MaxFacts+5 {
		t.Errorf("kept %d facts from %s, next %d", len(r.Facts), r.Facts[0].ID, r.NextFact)
	}
	if _, ok := r.Fact("f1"); ok {
		t.Error("the oldest fact was kept")
	}
}

func TestEveryReadRecordsTheTraceAndSpanIDsItShowed(t *testing.T) {
	r := recordOf(`{"rows":[["4bf92f3577b34da6a3ce929d0e0e4736","00f067aa0ba902b7","not-an-id"]]}`)
	if got := strings.Join(r.IDs, ","); got != "4bf92f3577b34da6a3ce929d0e0e4736,00f067aa0ba902b7" {
		t.Errorf("ids = %s", got)
	}
}
