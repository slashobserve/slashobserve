package triage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/datasource"
)

type transcriptCall struct {
	Tool    string          `json:"tool"`
	Input   json.RawMessage `json:"input"`
	Content string          `json:"content"`
	IsError bool            `json:"is_error,omitempty"`
}

type sweepCase struct {
	Name     string           `json:"name"`
	Sources  []sourceJSON     `json:"sources"`
	End      time.Time        `json:"end"`
	Question *windowJSON      `json:"question,omitempty"`
	Calls    []transcriptCall `json:"calls"`
	Want     struct {
		Block string `json:"block"`
		Leads []Lead `json:"leads"`
		Calls int    `json:"calls"`
	} `json:"want"`
}

type sourceJSON struct {
	UID  string `json:"uid"`
	Type string `json:"type"`
}

func sourcesOfJSON(in []sourceJSON) []Source {
	var out []Source
	for _, s := range in {
		out = append(out, Source(s))
	}
	return out
}

type answer struct {
	content any
	isError bool
}

type generator struct {
	call  func(name string, input map[string]any) answer
	frame func(uid, query string, rng datasource.TimeRange, points int) []datasource.Frame
}

func canonical(input any) json.RawMessage {
	raw, _ := json.Marshal(input)
	var v any
	_ = json.Unmarshal(raw, &v)
	out, _ := json.Marshal(v)
	return out
}

type transcriptTools struct {
	gen *generator

	mu       sync.Mutex
	recorded map[string]transcriptCall
	replay   map[string]transcriptCall
	missing  []string
}

func keyOf(tool string, input json.RawMessage) string { return tool + " " + string(input) }

func (tt *transcriptTools) answerOf(tool string, input json.RawMessage, record func() transcriptCall) (transcriptCall, bool) {
	tt.mu.Lock()
	defer tt.mu.Unlock()
	key := keyOf(tool, input)
	if tt.gen == nil {
		c, ok := tt.replay[key]
		if !ok {
			tt.missing = append(tt.missing, key)
		}
		return c, ok
	}
	c := record()
	tt.recorded[key] = c
	return c, true
}

func (tt *transcriptTools) Call(_ context.Context, name string, input json.RawMessage) (tools.Result, error) {
	in := canonical(json.RawMessage(input))
	c, ok := tt.answerOf(name, in, func() transcriptCall {
		var fields map[string]any
		_ = json.Unmarshal(input, &fields)
		a := tt.gen.call(name, fields)
		raw, _ := json.Marshal(a.content)
		return transcriptCall{Tool: name, Input: in, Content: string(raw), IsError: a.isError}
	})
	if !ok {
		return tools.Result{}, fmt.Errorf("no answer for %s", name)
	}
	return tools.Result{Content: json.RawMessage(c.Content), IsError: c.IsError}, nil
}

type seriesJSON struct {
	Labels map[string]string `json:"labels,omitempty"`
	Points [][2]float64      `json:"points"`
}

func (tt *transcriptTools) SweepRange(_ context.Context, uid, query string, rng datasource.TimeRange, points int) ([]datasource.Frame, error) {
	in := canonical(map[string]any{"datasource": uid, "query": query, "from": stampOf(rng.From), "to": stampOf(rng.To), "max_data_points": points})
	c, ok := tt.answerOf("read_series", in, func() transcriptCall {
		out := struct {
			From   string       `json:"from"`
			To     string       `json:"to"`
			Series []seriesJSON `json:"series"`
		}{From: stampOf(rng.From), To: stampOf(rng.To), Series: []seriesJSON{}}
		for _, f := range tt.gen.frame(uid, query, rng, points) {
			times := f.Fields[0].Values
			for _, field := range f.Fields[1:] {
				s := seriesJSON{Labels: f.Labels, Points: [][2]float64{}}
				for i, v := range field.Values {
					x, _ := v.(float64)
					if math.IsNaN(x) {
						continue
					}
					s.Points = append(s.Points, [2]float64{float64(times[i].(time.Time).Unix()), x})
				}
				out.Series = append(out.Series, s)
			}
		}
		raw, _ := json.Marshal(out)
		return transcriptCall{Tool: "read_series", Input: in, Content: string(raw)}
	})
	if !ok {
		return nil, fmt.Errorf("no answer for read_series")
	}
	var out struct {
		Series []seriesJSON `json:"series"`
	}
	if err := json.Unmarshal([]byte(c.Content), &out); err != nil {
		return nil, err
	}
	var frames []datasource.Frame
	for _, s := range out.Series {
		var times, nums []any
		for _, p := range s.Points {
			times = append(times, time.Unix(int64(p[0]), 0).UTC())
			nums = append(nums, p[1])
		}
		frames = append(frames, datasource.Frame{Labels: s.Labels, Fields: []datasource.Field{
			{Name: "Time", Type: datasource.FieldTime, Values: times},
			{Name: "Value", Type: datasource.FieldNumber, Values: nums},
		}})
	}
	return frames, nil
}

var sweepTiming = regexp.MustCompile(`Sweep: (\d+) reads in [^\n]*\.`)

func maskTiming(block string) string {
	return sweepTiming.ReplaceAllString(block, "Sweep: $1 reads in TIME.")
}

func steps(rng datasource.TimeRange, step time.Duration, value func(time.Time) float64) ([]any, []any) {
	var times, nums []any
	for t := rng.From; !t.After(rng.To); t = t.Add(step) {
		times = append(times, t)
		nums = append(nums, value(t))
	}
	return times, nums
}

func frameOf(labels map[string]string, rng datasource.TimeRange, points int, value func(time.Time) float64) datasource.Frame {
	step := rng.To.Sub(rng.From) / time.Duration(points)
	times, nums := steps(rng, step, value)
	return datasource.Frame{Labels: labels, Fields: []datasource.Field{
		{Name: "Time", Type: datasource.FieldTime, Values: times},
		{Name: "Value", Type: datasource.FieldNumber, Values: nums},
	}}
}

func listed(names ...string) map[string]any {
	return map[string]any{"sources": []map[string]any{{"field": "service_name", "services": names}}}
}

func metricList(search string, names ...string) map[string]any {
	var out []map[string]string
	for _, n := range names {
		if strings.Contains(n, search) {
			out = append(out, map[string]string{"name": n})
		}
	}
	return map[string]any{"metrics": out}
}

func unavailable(message string) answer {
	return answer{content: map[string]string{"code": "datasource_unavailable", "message": message}, isError: true}
}

func stampText(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func spanMetricsGenerator() *generator {
	faultAt := end.Add(-20 * time.Minute)
	return &generator{
		call: func(name string, in map[string]any) answer {
			switch name {
			case "list_services":
				if in["datasource"] == "loki" {
					return unavailable("connection refused")
				}
				return answer{content: listed("payment", "cart")}
			case "list_metrics":
				search, _ := in["search"].(string)
				return answer{content: metricList(search, "traces_span_metrics_calls_total", "traces_span_metrics_duration_milliseconds_bucket", "process_cpu_seconds_total")}
			case "list_annotations":
				return answer{content: map[string]any{"annotations": []map[string]any{
					{"time_from": stampText(faultAt.Add(-3 * time.Minute)), "text": "rolled out payment v2", "tags": []string{"deploy", "payment"}},
					{"time_from": "not a time", "text": "x"},
				}}}
			}
			return answer{content: map[string]string{"code": "unknown_tool"}, isError: true}
		},
		frame: func(_, q string, rng datasource.TimeRange, points int) []datasource.Frame {
			var out []datasource.Frame
			for _, svc := range []string{"cart", "payment"} {
				out = append(out, frameOf(map[string]string{"service_name": svc}, rng, points, func(t time.Time) float64 {
					switch {
					case strings.Contains(q, "histogram_quantile") && svc == "payment" && !t.Before(faultAt):
						return 950.5
					case strings.Contains(q, "histogram_quantile"):
						return 40
					case strings.Contains(q, "STATUS_CODE_ERROR") && svc == "payment" && !t.Before(faultAt):
						return 3
					case strings.Contains(q, "STATUS_CODE_ERROR") && svc == "cart":
						return 0.2
					case strings.Contains(q, "STATUS_CODE_ERROR"):
						return 0
					case svc == "cart" && t.Equal(rng.From):
						return math.NaN()
					}
					return 10
				}))
			}
			return out
		},
	}
}

func traceFallbackGenerator() *generator {
	faulty := map[string]bool{"payment": true, "checkout": true, "frontend": true}
	return &generator{
		call: func(name string, in map[string]any) answer {
			query, _ := in["query"].(string)
			switch name {
			case "list_services":
				switch in["datasource"] {
				case "tempo":
					return answer{content: map[string]any{"sources": []map[string]any{{"field": "resource.service.name", "services": []string{"payment", "frontend", "checkout"}}}}}
				case "loki":
					return answer{content: listed("checkout", "frontend", "payment")}
				}
				return answer{content: map[string]any{"sources": []any{}}}
			case "trace_error_origins":
				return answer{content: map[string]any{"groups": []map[string]any{
					{"service": "payment", "operation": "Charge", "status_message": "card declined: \"invalid token\"", "traces": 30, "share": 0.8, "example_trace_id": "4bf92f3577b34da6a3ce929d0e0e4736", "relayed_by": []map[string]any{{"service": "checkout"}, {"service": "frontend"}}},
				}}}
			case "log_patterns":
				return answer{content: map[string]any{"patterns": []map[string]any{
					{"pattern": "charge failed <_>", "level": "error", "trend": []int{0, 0, 0, 0, 5, 9}},
					{"pattern": "steady <_>", "level": "info", "trend": []int{5, 5, 5, 5, 5, 5}},
				}}}
			case "list_annotations":
				return answer{content: map[string]any{"annotations": []any{}}}
			case "trace_stats":
				from, _ := time.Parse(time.RFC3339, in["from"].(string))
				to, _ := time.Parse(time.RFC3339, in["to"].(string))
				buckets, _ := in["buckets"].(float64)
				groupBy, _ := in["group_by"].(string)
				if strings.Contains(query, "feature_flag") {
					if groupBy == "event.feature_flag.key" {
						return answer{content: map[string]any{"groups": []map[string]any{{"value": "paymentFailure", "spans": 40}, {"value": "", "spans": 9}}}}
					}
					if strings.Contains(groupBy, "variant") && !strings.Contains(query, "variant") {
						if groupBy == "event.feature_flag.result.variant" {
							return answer{content: map[string]any{"groups": []any{}}}
						}
						return answer{content: map[string]any{"groups": []map[string]any{{"value": "on", "spans": 30}, {"value": "off", "spans": 10}}}}
					}
					var out []map[string]any
					step := to.Sub(from) / time.Duration(buckets)
					for at := from; at.Before(to); at = at.Add(step) {
						spans := 0
						if strings.Contains(query, `"on"`) && !at.Before(end.Add(-22*time.Minute)) {
							spans = 3
						}
						if strings.Contains(query, `"off"`) {
							spans = 1
						}
						out = append(out, map[string]any{"start": stampText(at), "spans": spans, "errors": 0})
					}
					return answer{content: map[string]any{"buckets": out}}
				}
				if groupBy == traceServiceKey {
					var groups []map[string]any
					for _, svc := range []string{"checkout", "frontend", "payment", "quote"} {
						g := map[string]any{"value": svc, "spans": 1000, "errors": 0, "p95_ms": 40.5}
						if to.Equal(end) && faulty[svc] && to.Sub(from) == time.Hour {
							g["errors"] = 300
						}
						if svc == "quote" {
							g["p95_ms"] = nil
						}
						groups = append(groups, g)
					}
					return answer{content: map[string]any{"groups": groups}}
				}
				var out []map[string]any
				step := to.Sub(from) / time.Duration(buckets)
				onset := end.Add(-20 * time.Minute)
				if strings.Contains(query, "frontend") || strings.Contains(query, "checkout") {
					onset = end.Add(-16 * time.Minute)
				}
				for at := from; at.Before(to); at = at.Add(step) {
					b := map[string]any{"start": stampText(at), "spans": 100, "errors": 0, "p95_ms": 30}
					if !at.Before(onset) {
						b["errors"] = 40
					}
					out = append(out, b)
				}
				return answer{content: map[string]any{"buckets": out}}
			}
			return answer{content: map[string]string{"code": "unknown_tool"}, isError: true}
		},
		frame: func(_, q string, rng datasource.TimeRange, points int) []datasource.Frame {
			switch {
			case strings.Contains(q, "feature_flag_key"):
				onset := end.Add(-22 * time.Minute)
				return []datasource.Frame{frameOf(map[string]string{"feature_flag_key": "paymentFailure", "feature_flag_result_variant": "on"}, rng, points, func(t time.Time) float64 {
					if t.Before(onset) {
						return 0
					}
					return 3
				})}
			case strings.Contains(q, "deploy"):
				return []datasource.Frame{frameOf(map[string]string{"service_name": "payment"}, rng, points, func(t time.Time) float64 {
					if t.After(end.Add(-50*time.Minute)) && t.Before(end.Add(-25*time.Minute)) {
						return 2
					}
					return 0
				})}
			}
			return nil
		},
	}
}

func failedErrorReadGenerator() *generator {
	return &generator{
		call: func(name string, in map[string]any) answer {
			switch name {
			case "list_services":
				return answer{content: listed("cart")}
			case "list_metrics":
				search, _ := in["search"].(string)
				return answer{content: metricList(search, "http_server_request_duration_seconds_bucket", "http_server_request_duration_seconds_count")}
			case "list_annotations":
				return unavailable("")
			}
			return answer{content: map[string]string{"code": "unknown_tool"}, isError: true}
		},
		frame: func(_, q string, rng datasource.TimeRange, points int) []datasource.Frame {
			if strings.Contains(q, "5..") {
				return nil
			}
			return []datasource.Frame{frameOf(map[string]string{"service_name": "cart"}, rng, points, func(time.Time) float64 { return 10 })}
		},
	}
}

func sweepCases() []struct {
	name     string
	sources  []Source
	question *Window
	gen      *generator
} {
	return []struct {
		name     string
		sources  []Source
		question *Window
		gen      *generator
	}{
		{"span metrics show a fault in payment while Loki is down", []Source{{"loki", "loki"}, {"prom", "prometheus"}}, &Window{From: end.Add(-time.Hour), To: end}, spanMetricsGenerator()},
		{"trace rates with flags, patterns and knock-ons", []Source{{"loki", "loki"}, {"tempo", "tempo"}}, nil, traceFallbackGenerator()},
		{"HTTP server metrics with an empty error read", []Source{{"prom", "prometheus"}}, nil, failedErrorReadGenerator()},
	}
}

func runSweep(tt *transcriptTools, sources []Source, question *Window) Outcome {
	return Sweeper{Reader: ToolReader{Tools: tt}, Limits: DefaultLimits}.Sweep(context.Background(), Input{Sources: sources, End: end, Question: question})
}

func TestSweepTranscripts(t *testing.T) {
	var cases []sweepCase
	if *updateFixtures {
		for _, c := range sweepCases() {
			tt := &transcriptTools{gen: c.gen, recorded: map[string]transcriptCall{}}
			live := runSweep(tt, c.sources, c.question)
			sc := sweepCase{Name: c.name, End: end}
			for _, src := range c.sources {
				sc.Sources = append(sc.Sources, sourceJSON(src))
			}
			if c.question != nil {
				sc.Question = &windowJSON{c.question.From, c.question.To}
			}
			for _, call := range tt.recorded {
				sc.Calls = append(sc.Calls, call)
			}
			slices.SortFunc(sc.Calls, func(a, b transcriptCall) int { return strings.Compare(keyOf(a.Tool, a.Input), keyOf(b.Tool, b.Input)) })
			sc.Want.Block, sc.Want.Leads, sc.Want.Calls = maskTiming(live.Block), list(live.Leads), live.Calls
			cases = append(cases, sc)
		}
	} else {
		readFixture(t, "sweep.json", &cases)
	}
	if len(cases) == 0 {
		t.Fatal("the sweep transcripts are empty")
	}
	for _, c := range cases {
		tt := &transcriptTools{replay: map[string]transcriptCall{}}
		for _, call := range c.Calls {
			tt.replay[keyOf(call.Tool, canonical(call.Input))] = call
		}
		var question *Window
		if c.Question != nil {
			question = &Window{From: c.Question.From, To: c.Question.To}
		}
		got := runSweep(tt, sourcesOfJSON(c.Sources), question)
		if len(tt.missing) > 0 {
			t.Errorf("%s: calls with no recorded answer: %q", c.Name, tt.missing)
		}
		if block := maskTiming(got.Block); block != c.Want.Block {
			t.Errorf("%s: block =\n%s\nwant\n%s", c.Name, block, c.Want.Block)
		}
		if got.Calls != c.Want.Calls || got.Calls != len(c.Calls) {
			t.Errorf("%s: calls = %d, want %d of %d recorded", c.Name, got.Calls, c.Want.Calls, len(c.Calls))
		}
		var b bytes.Buffer
		_ = json.NewEncoder(&b).Encode(list(got.Leads))
		var w bytes.Buffer
		_ = json.NewEncoder(&w).Encode(c.Want.Leads)
		if b.String() != w.String() {
			t.Errorf("%s: leads = %s, want %s", c.Name, b.String(), w.String())
		}
	}
	if *updateFixtures {
		writeFixture(t, "sweep.json", cases)
	}
}
