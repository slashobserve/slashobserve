package grounding

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/findings"
)

func factOf(t *testing.T, r *Record, holder any, key string) Fact {
	t.Helper()
	ids, _ := holder.(map[string]any)["facts"].(map[string]any)
	id, _ := ids[key].(string)
	f, ok := r.Fact(id)
	if !ok {
		t.Fatalf("no fact for %s in %v", key, holder)
	}
	return f
}

const facetsResult = `{"sampled":true,"lines_sampled":2000,"fields":[` +
	`{"name":"service_name","kind":"label","exact":true,"unit":"bytes","cardinality":2,"values":[{"value":"cart","count":524288},{"value":"ad","count":1024}],"truncated":false},` +
	`{"name":"level","kind":"field","exact":false,"cardinality":2,"values":[{"value":"error","count":37}],"truncated":false},` +
	`{"name":"status","kind":"column","exact":true,"values":[{"value":"503","count":412}],"truncated":false}],"truncated":false}`

func TestLogFacetsGiveACountPerValueMarkedSampledWhenTheFieldIsNotExact(t *testing.T) {
	r := NewRecord(Enforce)
	parsed := issued(t, r, "log_facets", `{"query":"{service_name=~\".+\"}"}`, facetsResult)
	fields := parsed["fields"].([]any)
	value := func(field, i int) any { return fields[field].(map[string]any)["values"].([]any)[i] }

	bytes := factOf(t, r, value(0, 0), "count")
	if bytes.Kind != FactCount || bytes.Unit != "bytes" || bytes.Sampled || bytes.Value != 524288 ||
		!maps.Equal(bytes.Labels, map[string]string{"name": "service_name", "value": "cart"}) || bytes.Of != "log_facets fields[].values[] count" {
		t.Errorf("index label fact %+v", bytes)
	}
	errors := factOf(t, r, value(1, 0), "count")
	if !errors.Sampled || errors.Unit != "" || errors.Value != 37 || errors.Labels["value"] != "error" {
		t.Errorf("sampled field fact %+v", errors)
	}
	if status := factOf(t, r, value(2, 0), "count"); status.Sampled || status.Value != 412 {
		t.Errorf("exact column fact %+v", status)
	}
	if lines := factOf(t, r, parsed, "lines_sampled"); lines.Sampled || lines.Value != 2000 || lines.Kind != FactCount {
		t.Errorf("lines sampled fact %+v", lines)
	}
	if signalOf(errors) != findings.SignalLogs {
		t.Errorf("a facet count is classed %s", signalOf(errors))
	}
}

func TestLogPatternsGiveACountPerPatternSampledOnlyWhenTheServerClusteredASample(t *testing.T) {
	long := strings.Repeat("x", 80)
	for _, c := range []struct {
		source  string
		sampled bool
	}{{"loki", false}, {"elasticsearch", false}, {"server", true}} {
		r := NewRecord(Report)
		content := fmt.Sprintf(`{"source":%q,"lines_sampled":2000,"lines_matched":2000,"trend_step_ms":120000,"patterns":[`+
			`{"pattern":"connection refused <_>","count":1200,"sample":"connection refused 10.0.0.1","first":"2026-11-20T10:05:00Z","last":"2026-11-20T10:59:00Z","level":"error","trend":[0,600,600]},`+
			`{"pattern":"%s <_>","count":800,"sample":"s","first":"2026-11-20T10:05:00Z","last":"2026-11-20T10:59:00Z","level":"","trend":[0,800]}],"truncated":false}`, c.source, long)
		parsed := issued(t, r, "log_patterns", `{"query":"{service_name=\"cart\"}"}`, content)
		patterns := parsed["patterns"].([]any)
		refused := factOf(t, r, patterns[0], "count")
		if refused.Sampled != c.sampled || refused.Value != 1200 || refused.Kind != FactCount ||
			!maps.Equal(refused.Labels, map[string]string{"pattern": "connection refused <_>", "level": "error"}) {
			t.Errorf("%s: pattern fact %+v", c.source, refused)
		}
		if clipped := factOf(t, r, patterns[1], "count"); clipped.Labels["pattern"] != long[:maxFactPatternText] {
			t.Errorf("%s: a long pattern's label is %q", c.source, clipped.Labels["pattern"])
		}
		if matched := factOf(t, r, parsed, "lines_matched"); matched.Sampled != c.sampled {
			t.Errorf("%s: lines matched fact %+v", c.source, matched)
		}
		if _, ok := parsed["facts"].(map[string]any)["lines_sampled"]; ok {
			t.Errorf("%s: the sample size of patterns got a fact", c.source)
		}
		for _, f := range r.Facts {
			if f.Value == 120000 || f.Value == 600 {
				t.Errorf("%s: a trend or its step became fact %+v", c.source, f)
			}
		}
	}
}

func TestAMeasurementOnLokiIsTheLogsSignal(t *testing.T) {
	r := NewRecord(Report)
	loki := `{"op":"total","store":"loki","from":"2026-11-20T09:00:00Z","to":"2026-11-20T11:00:00Z","query":"sum(count_over_time({service_name=\"cart\"} |= \"error\" [7200s]))","empty":false,"values":[{"value":412}]}`
	prom := `{"op":"total","store":"prometheus","from":"2026-11-20T09:00:00Z","to":"2026-11-20T11:00:00Z","query":"sum(increase(errors_total[7200s]))","empty":false,"values":[{"value":980}]}`
	lokiFact := factOf(t, r, issued(t, r, "measure_aggregate", "{}", loki)["values"].([]any)[0], "value")
	promFact := factOf(t, r, issued(t, r, "measure_aggregate", "{}", prom)["values"].([]any)[0], "value")
	if signalOf(lokiFact) != findings.SignalLogs || signalOf(promFact) != findings.SignalMetrics {
		t.Errorf("signals %s and %s, want logs and metrics", signalOf(lokiFact), signalOf(promFact))
	}
	if lokiFact.Sampled {
		t.Error("a Loki total is marked sampled")
	}
}

func TestLogFactsAddTheLogsSignalToACause(t *testing.T) {
	r := causeRecord(t)
	issued(t, r, "log_patterns", `{}`, `{"source":"loki","lines_sampled":0,"lines_matched":1200,"patterns":[{"pattern":"connection refused <_>","count":1200,"level":"error"}]}`)
	id := r.Facts[len(r.Facts)-1].ID
	input := fmt.Sprintf(`{"markdown":"m","causes":[{"component":{"kind":"service","name":"cart"},"kind":"dependency","mechanism":"m","evidence":{"facts":["f1",%q]}}]}`, id)
	got := r.CauseSignals(json.RawMessage(input))
	if len(got) != 1 || !slices.Equal(got[0].Kinds, []string{findings.SignalMetrics, findings.SignalLogs}) {
		t.Fatalf("signals %+v", got)
	}
	if score := findings.Score(got[0]); score != 0.5 {
		t.Errorf("score %.2f, want 0.5 for metrics, logs and the component", score)
	}
}

func TestASampledCountRendersAsALowerBound(t *testing.T) {
	sampled := Fact{ID: "f4", Text: "1250", Value: 1250, Kind: FactCount, Sampled: true}
	bytes := Fact{ID: "f5", Text: "524288", Value: 524288, Kind: FactCount, Unit: "bytes"}
	cases := []struct {
		fact    Fact
		options []string
		want    string
		err     string
	}{
		{sampled, nil, "at least 1250", ""},
		{sampled, []string{"bare"}, "at least 1250", ""},
		{sampled, []string{"3"}, "at least 1250", ""},
		{Fact{ID: "f6", Text: "1251", Value: 1251, Kind: FactCount, Sampled: true}, []string{"3"}, "at least 1250", ""},
		{Fact{ID: "f7", Text: "1249", Value: 1249, Kind: FactCount, Sampled: true}, []string{"3"}, "", "{{f7:3}} would show at least 1250, above the 1249 counted in a sample"},
		{sampled, []string{"%"}, "", "f4 is a count, so it cannot be shown as a percent"},
		{bytes, nil, "524288 bytes", ""},
		{bytes, []string{"ms"}, "", "f5 is a count in bytes, so it cannot be shown in ms"},
	}
	for _, c := range cases {
		got, err := Format(c.fact, c.options)
		switch {
		case c.err != "" && (err == nil || !strings.Contains(err.Error(), c.err)):
			t.Errorf("%s %v: err %v, want %q", c.fact.ID, c.options, err, c.err)
		case c.err == "" && (err != nil || got != c.want):
			t.Errorf("%s %v: %q, %v, want %q", c.fact.ID, c.options, got, err, c.want)
		}
	}
	ledger := &Ledger{Facts: []Fact{sampled}}
	if text, problems := ledger.Render("The error pattern appeared {{f4}} times."); text != "The error pattern appeared at least 1250 times." || problems != nil {
		t.Errorf("rendered %q, %+v", text, problems)
	}
}
