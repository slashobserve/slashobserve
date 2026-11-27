package tools

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func TestPlanMeasureBuildsTheWindowQuery(t *testing.T) {
	window := 6 * time.Hour
	for _, tc := range []struct {
		name    string
		in      measureInput
		query   string
		sampled string
	}{
		{
			"a plain selector reads every stored sample",
			measureInput{Query: `queue_depth{service="orders"}`, Stat: statMax},
			`max_over_time(queue_depth{service="orders"}[21600s])`, everySample,
		},
		{
			"an expression is evaluated every minute",
			measureInput{Query: `sum(queue_depth)`, Stat: statMax},
			`max_over_time((sum(queue_depth))[21600s:60s])`, "every 60s",
		},
		{
			"a named step is used as given",
			measureInput{Query: `queue_depth`, Stat: statAvg, Step: "5m"},
			`avg_over_time((queue_depth)[21600s:300s])`, "every 300s",
		},
		{
			"the first time a condition held",
			measureInput{Query: `sum(queue_depth)`, Stat: statFirstTime, Where: "== 0"},
			`min_over_time((timestamp(sum(queue_depth)) and ((sum(queue_depth)) == 0))[21600s:15s])`, "every 15s",
		},
		{
			"the last time a condition held",
			measureInput{Query: `queue_depth`, Stat: statLastTime, Where: ">100", Step: "1m"},
			`max_over_time((timestamp(queue_depth) and ((queue_depth) > 100))[21600s:60s])`, "every 60s",
		},
	} {
		plan, problem := planMeasure(tc.in, window)
		if problem != "" || plan.query != tc.query || plan.sampled != tc.sampled {
			t.Errorf("%s:\n got  %q (%s) %q\n want %q (%s)", tc.name, plan.query, plan.sampled, problem, tc.query, tc.sampled)
		}
	}
}

func TestPlanMeasureRefusesWhatItCannotMeasure(t *testing.T) {
	for name, in := range map[string]measureInput{
		"a step without a unit":             {Query: "up", Stat: statMax, Step: "60"},
		"a time without a condition":        {Query: "up", Stat: statFirstTime},
		"a condition on a value":            {Query: "up", Stat: statMax, Where: "== 0"},
		"a condition that is not a number":  {Query: "up", Stat: statFirstTime, Where: "== foo"},
		"a condition that carries PromQL":   {Query: "up", Stat: statFirstTime, Where: "== 0) or vector(1"},
		"more steps than a window may hold": {Query: "sum(up)", Stat: statMax, Step: "1s"},
	} {
		if plan, problem := planMeasure(in, 6*time.Hour); problem == "" {
			t.Errorf("%s: planned %q, want a refusal", name, plan.query)
		}
	}
}

type measureFake struct {
	mu      sync.Mutex
	queries []string
	ranges  []datasource.TimeRange
	answer  func(query string) []datasource.Frame
}

func (f *measureFake) registry(t *testing.T, typ string) *Registry {
	t.Helper()
	return newTestRegistry(t, &fakeDS{typ: typ, instantFn: func(_ context.Context, q datasource.Query, r datasource.TimeRange) ([]datasource.Frame, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.queries = append(f.queries, q.Text)
		f.ranges = append(f.ranges, r)
		return f.answer(q.Text), nil
	}}, nil)
}

func measured(t *testing.T, r *Registry, input string) measureOut {
	t.Helper()
	res, err := call(t, r, nameMeasure, input)
	if err != nil || res.IsError {
		t.Fatalf("res = %#v err = %v", res.Content, err)
	}
	out, ok := res.Content.(measureOut)
	if !ok {
		t.Fatalf("content is %T", res.Content)
	}
	return out
}

func TestMeasureReturnsThePeakAtFullPrecisionAndWhenItWasReached(t *testing.T) {
	reached := time.Date(2026, 11, 20, 7, 12, 30, 0, time.UTC)
	fake := &measureFake{answer: func(query string) []datasource.Frame {
		if strings.HasPrefix(query, "max_over_time(") {
			return []datasource.Frame{timeNumberFrame(nil, 1847.3125)}
		}
		return []datasource.Frame{timeNumberFrame(nil, float64(reached.Unix()))}
	}}
	out := measured(t, fake.registry(t, ""), `{"datasource":"fake","query":"sum(queue_depth)","stat":"max","from":"2026-11-20T04:00:00Z","to":"2026-11-20T10:00:00Z"}`)

	if len(out.Values) != 1 || out.Values[0].Value == nil || *out.Values[0].Value != 1847.3125 {
		t.Fatalf("values = %+v", out.Values)
	}
	if out.Values[0].At != "2026-11-20T07:12:30Z" || *out.Values[0].Seconds != 11550 {
		t.Errorf("at = %q seconds = %v", out.Values[0].At, *out.Values[0].Seconds)
	}
	if out.Query != `max_over_time((sum(queue_depth))[21600s:60s])` || out.Sampled != "every 60s" {
		t.Errorf("query = %q sampled = %q", out.Query, out.Sampled)
	}
	want := `min_over_time((timestamp(sum(queue_depth)) and ((sum(queue_depth)) == 1847.3125))[21600s:15s])`
	if len(fake.queries) != 2 || fake.queries[1] != want {
		t.Errorf("queries = %q", fake.queries)
	}
	if !fake.ranges[0].To.Equal(time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("evaluated at %s, want the end of the window", fake.ranges[0].To)
	}
}

func TestMeasureTimesADrainFromThePeak(t *testing.T) {
	emptied := time.Date(2026, 11, 20, 7, 33, 30, 0, time.UTC)
	fake := &measureFake{answer: func(string) []datasource.Frame {
		return []datasource.Frame{timeNumberFrame(nil, float64(emptied.Unix()))}
	}}
	out := measured(t, fake.registry(t, ""), `{"datasource":"fake","query":"sum(queue_depth)","stat":"first_time","where":"== 0","from":"2026-11-20T07:12:30Z","to":"2026-11-20T10:00:00Z"}`)

	if len(out.Values) != 1 || out.Values[0].At != "2026-11-20T07:33:30Z" || *out.Values[0].Seconds != 1260 || out.Values[0].Value != nil {
		t.Fatalf("values = %+v", out.Values)
	}
	if len(fake.queries) != 1 {
		t.Errorf("ran %d queries, want 1", len(fake.queries))
	}
}

func TestMeasureKeepsEverySeriesAndLooksUpNoTimeForSeveral(t *testing.T) {
	fake := &measureFake{answer: func(string) []datasource.Frame {
		return []datasource.Frame{
			timeNumberFrame(map[string]string{"service": "orders"}, 12.5),
			timeNumberFrame(map[string]string{"service": "payments"}, 3),
		}
	}}
	out := measured(t, fake.registry(t, ""), `{"datasource":"fake","query":"queue_depth","stat":"max"}`)

	if len(out.Values) != 2 || out.Values[0].Labels["service"] != "orders" || *out.Values[1].Value != 3 || out.Values[0].At != "" {
		t.Fatalf("values = %+v", out.Values)
	}
	if len(fake.queries) != 1 || out.Sampled != everySample {
		t.Errorf("queries = %q sampled = %q", fake.queries, out.Sampled)
	}
}

func TestMeasureSaysAnEmptyResultIsNotAZero(t *testing.T) {
	fake := &measureFake{answer: func(string) []datasource.Frame { return nil }}
	out := measured(t, fake.registry(t, ""), `{"datasource":"fake","query":"queue_depth","stat":"max"}`)
	if !out.Empty || !strings.Contains(out.Note, "does not show the value was zero") {
		t.Fatalf("out = %+v", out)
	}
}

func TestMeasureRefusesAStoreItCannotMeasure(t *testing.T) {
	fake := &measureFake{answer: func(string) []datasource.Frame { return nil }}
	res, err := call(t, fake.registry(t, "loki"), nameMeasure, `{"datasource":"fake","query":"{a=\"b\"}","stat":"max"}`)
	if err != nil || errorCode(t, res) != codeInvalidRequest || len(fake.queries) != 0 {
		t.Fatalf("res = %#v err = %v queries = %q", res.Content, err, fake.queries)
	}
}
