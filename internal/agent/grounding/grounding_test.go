package grounding

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/findings"
)

func figureTexts(text string) []string {
	var out []string
	for _, f := range UncitedFigures(text, "") {
		out = append(out, f.Text)
	}
	return out
}

func TestFiguresAreTakenFromProseAndNotFromTimesCodeLinksOrSmallCounts(t *testing.T) {
	cases := []struct {
		text string
		want []string
	}{
		{"p99 rose to 523 ms at 10:05 UTC, up from 287ms (+82%).", []string{"523 ms", "287ms", "82%"}},
		{"About 4.1% of requests failed (1,284 of 31.2K).", []string{"4.1%", "1,284", "31.2K"}},
		{"Payment calls timed out after 2.4 s and 1.5 seconds.", []string{"2.4 s", "1.5 seconds"}},
		{"Memory reached 512 MiB, 3x the usual 1.2M rows.", []string{"512 MiB", "3x", "1.2M"}},
		{"3 pods restarted and 2 of 7 services saw 5xx errors.", nil},
		{"Since 2026-11-20T10:05:00Z and Nov 20, 2026 at 9:30 am, over the last 15 minutes and 5m windows.", nil},
		{"See [run 42](/o/0192) and `rate(x[5m]) > 0.25`, or https://example.com/a/123.", nil},
		{"1. First item\n2. Second item with 640 errors", []string{"640"}},
		{"Version v1.2.3 of checkout-7f9c on pod-12 using http2 and p99.", nil},
		{"Errors fell -12% to 0.75.", []string{"12%", "0.75"}},
	}
	for _, c := range cases {
		if got := figureTexts(c.text); !slices.Equal(got, c.want) {
			t.Errorf("figures of %q = %q, want %q", c.text, got, c.want)
		}
	}
}

func TestTheWrittenPrecisionOfAFigure(t *testing.T) {
	cases := []struct {
		text string
		step float64
	}{
		{"0.52 s", 0.01},
		{"523 ms", 1},
		{"31.2K", 0.1},
		{"1,500 errors", 100},
		{"40%", 1},
	}
	for _, c := range cases {
		got := scan(blank(c.text))
		if len(got) != 1 || got[0].Step != c.step {
			t.Errorf("figures of %q = %+v, want one figure with step %v", c.text, got, c.step)
		}
	}
}

func at(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func fixtureRecord(t *testing.T) *Record {
	t.Helper()
	r := NewRecord(Report)
	window := []Range{{From: at("2026-11-20T09:00:00Z"), To: at("2026-11-20T11:00:00Z")}}
	r.AddRead([]byte(`{"kind":"range","from":"2026-11-20T09:00:00Z","to":"2026-11-20T11:00:00Z","step_ms":60000,"series":[
		{"labels":{"service_name":"checkout","http_status_code":"500"},"min":0.00121,"max":0.52341,"mean":0.28712,"last":0.4912},
		{"labels":{"service_name":"payment"},"min":0.0103,"max":0.0874,"last":0.0632}]}`), nil, window)
	r.AddRead([]byte(`{"series":[{"labels":{"service_name":"checkout"},"last":1284},{"labels":{"job":"frontend"},"last":31200}]}`), nil, window)
	r.AddRead([]byte(`{"table":{"columns":["time","line"],"rows":[
		["2026-11-20T10:05:12Z","payment request failed after 2.4s: upstream timeout"],
		["2026-11-20T10:05:13Z","cache miss {region=\"eu-west-1\"} retrying in 250ms"]]}}`), nil, nil)
	r.AddRead([]byte(`{"count":4512,"p50_ms":143.2,"p99_ms":812.4}`), []string{"frontend-proxy", "flagd"}, nil)
	return r
}

func finding(markdown string, extra map[string]any) json.RawMessage {
	in := map[string]any{"title": "Checkout latency", "markdown": markdown}
	for k, v := range extra {
		in[k] = v
	}
	raw, err := json.Marshal(in)
	if err != nil {
		panic(err)
	}
	return raw
}

func TestARealFindingPassesOnReadRoundedAndDerivedValues(t *testing.T) {
	r := fixtureRecord(t)
	markdown := "Checkout p99 latency rose to 523 ms at 10:05 UTC, up from 287 ms (+82%), a rise of 236 ms.\n\n" +
		"About 4.1% of requests failed (1,284 of 31.2K), and payment calls timed out after 2.4 s, with retries every 0.25 s.\n\n" +
		"Trace p99 was 812 ms and p50 143 ms over 4.5K traces, so the slow tail is 5.7x the median. 3 pods restarted.\n\n" +
		"See [the dashboard](/o/abc123) and `rate(http_requests_total[5m])`."
	in := finding(markdown, map[string]any{
		"impact":     "1,284 checkouts failed over 2 hours, about 4% of traffic.",
		"root_cause": map[string]any{"component": "checkout service", "kind": "dependency", "summary": "s"},
		"onset":      "2026-11-20T10:05:00Z",
	})
	if got := Unverified(r.Review(in, "")); len(got) != 0 {
		t.Errorf("unverified = %+v, want none", got)
	}
}

func TestFiguresNamesAndTimesTheRunNeverReadAreListed(t *testing.T) {
	r := fixtureRecord(t)
	in := finding("Checkout p99 reached 9.3 s and 2,950 orders failed.", map[string]any{
		"impact":     "6,400 customers saw errors.",
		"root_cause": map[string]any{"component": "cartservice", "kind": "dependency", "summary": "s"},
		"onset":      "2026-11-19T22:00:00Z",
	})
	want := []findings.Unverified{
		{Kind: findings.UnverifiedFigure, Text: "9.3 s"},
		{Kind: findings.UnverifiedFigure, Text: "2,950"},
		{Kind: findings.UnverifiedFigure, Text: "6,400"},
		{Kind: findings.UnverifiedComponent, Text: "cartservice"},
		{Kind: findings.UnverifiedOnset, Text: "2026-11-19T22:00:00Z"},
	}
	if got := Unverified(r.Review(in, "")); !slices.Equal(got, want) {
		t.Errorf("unverified = %+v\nwant %+v", got, want)
	}
}

func TestAPercentagePassesOnlyAsARatioOrChangeOfReadValues(t *testing.T) {
	r := NewRecord(Enforce)
	r.AddRead([]byte(`{"errors":1284,"requests":31200,"before":0.287,"after":0.523}`), nil, nil)
	cases := map[string]bool{
		"4.1% of requests failed":   true,
		"latency rose 82%":          true,
		"latency is 45% lower":      true,
		"7.5% of requests failed":   false,
		"latency rose 140 percent":  false,
		"the share is 0.041 of all": true,
	}
	for text, ok := range cases {
		if got := len(Unverified(r.Review(finding(text, nil), ""))) == 0; got != ok {
			t.Errorf("%q verified = %t, want %t", text, got, ok)
		}
	}
}

func TestAComponentPassesOnANameFromDiscoveryAndAnOnsetJustOutsideARangeInSlack(t *testing.T) {
	r := fixtureRecord(t)
	in := finding("Nothing numeric.", map[string]any{
		"root_cause": map[string]any{"component": "flagd", "kind": "config_or_flag", "summary": "s"},
		"onset":      "2026-11-20T08:59:30Z",
	})
	if got := Unverified(r.Review(in, "")); len(got) != 0 {
		t.Errorf("unverified = %+v, want none", got)
	}
}

func TestARecordIsBounded(t *testing.T) {
	r := NewRecord(Enforce)
	for read := range 10 {
		values := make([]float64, 600)
		for i := range values {
			values[i] = float64(100+i) * math.Pow10(read)
		}
		raw, _ := json.Marshal(values)
		r.AddRead(raw, nil, nil)
	}
	if got := len(allValues(r)); got != MaxValues {
		t.Fatalf("values = %d, want the cap %d", got, MaxValues)
	}
	last := r.Reads[len(r.Reads)-1]
	if newest := last[len(last)-1]; newest != rounded(float64(100+maxValuesOfRead-1)*math.Pow10(9)) {
		t.Errorf("newest value = %v, want the last read kept", newest)
	}
}

func TestValuesAreRoundedToThreeSignificantFigures(t *testing.T) {
	for in, want := range map[float64]float64{0.52341: 0.523, 31234: 31200, -4.567: 4.57, 0: 0, 812.4: 812} {
		if got := rounded(in); got != want {
			t.Errorf("rounded(%v) = %v, want %v", in, got, want)
		}
	}
}

func TestOffRecordsNothing(t *testing.T) {
	r := NewRecord(Off)
	r.AddRead([]byte(`{"v":1}`), []string{"checkout"}, nil)
	if r != nil || Unverified(r.Review(finding("12,345 errors", nil), "")) != nil {
		t.Error("an off record holds or checks something")
	}
}

func TestParseMode(t *testing.T) {
	for raw, want := range map[string]Mode{"": Enforce, "off": Off, "report": Report, "enforce": Enforce} {
		if got, err := ParseMode(raw); err != nil || got != want {
			t.Errorf("ParseMode(%q) = %q, %v", raw, got, err)
		}
	}
	if _, err := ParseMode("on"); err == nil {
		t.Error("ParseMode accepted on")
	}
}

func TestOnlyRealMonthNamesMakeADate(t *testing.T) {
	text := "Orders declined 37% and errors decreased 45, marginally 80 above, across 12 separate pods, since Dec 3 and 4 Sept. 2026."
	if got, want := figureTexts(text), []string{"37%", "45", "80", "12"}; !slices.Equal(got, want) {
		t.Errorf("figures = %q, want %q", got, want)
	}
}

func TestTheToleranceIsAtMostATenthOfTheFigure(t *testing.T) {
	cases := []struct {
		read float64
		text string
		ok   bool
	}{
		{0.62, "100% of requests", false},
		{500, "1,000 errors", false},
		{1000, "1,000 errors", true},
		{940, "1,000 errors", true},
	}
	for _, c := range cases {
		r := NewRecord(Report)
		raw, _ := json.Marshal(map[string]float64{"v": c.read})
		r.AddRead(raw, nil, nil)
		if got := len(Unverified(r.Review(finding(c.text, nil), ""))) == 0; got != c.ok {
			t.Errorf("%q on a read of %v verified = %t, want %t", c.text, c.read, got, c.ok)
		}
	}
}

func verified(r *Record, text string) bool {
	return len(Unverified(r.Review(finding(text, nil), ""))) == 0
}

func recordOf(reads ...string) *Record {
	r := NewRecord(Report)
	for _, read := range reads {
		r.AddRead([]byte(read), nil, nil)
	}
	return r
}

func TestAPercentIsDerivedOnlyAsAFractionOrAChange(t *testing.T) {
	if verified(recordOf(`[300, 382]`), "orders rose 82%") {
		t.Error("82% passed as the raw difference of 300 and 382")
	}
	if verified(recordOf(`[30, 52]`), "orders rose 82%") {
		t.Error("82% passed as the raw sum of 30 and 52")
	}
	if !verified(recordOf(`[100, 182]`), "orders rose 82%") {
		t.Error("82% failed as the change from 100 to 182")
	}
	if !verified(recordOf(`[0.4, 0.42]`), "the share grew by 2 percent") {
		t.Error("2 percent failed as the difference of two fractions")
	}
}

func TestDerivedFiguresComeFromOneRead(t *testing.T) {
	if verified(recordOf(`[300]`, `[382]`), "there were 82 more orders") {
		t.Error("a difference of values from two reads passed")
	}
	if !verified(recordOf(`[300, 382]`), "there were 82 more orders") {
		t.Error("a difference of values from one read failed")
	}
}

func allValues(r *Record) []float64 {
	var out []float64
	for _, read := range r.Reads {
		out = append(out, read...)
	}
	slices.Sort(out)
	return out
}

func TestTimesAndNumericKeysAreNotValues(t *testing.T) {
	r := recordOf(
		`{"before":[{"time":"2026-11-20T10:05:12Z","time_ns":"1763633112000000000","body":"took 523ms"}]}`,
		`{"columns":[{"name":"Timestamp","type":"time"},{"name":"p99","type":"number"}],"rows":[[1763633112000,4.5]],"total_rows":1}`,
		`{"buckets":{"0.25":12,"0.5":40}}`,
	)
	if got, want := allValues(r), []float64{4.5, 12, 40, 523}; !slices.Equal(got, want) {
		t.Errorf("values = %v, want %v", got, want)
	}
}

func TestALongTextIsCutAtAWordBoundary(t *testing.T) {
	raw, _ := json.Marshal([]string{strings.Repeat("a", maxScannedBytes-4) + " 123456 more"})
	if got := allValues(recordOf(string(raw))); len(got) != 0 {
		t.Errorf("values = %v, want none from a number the cut split", got)
	}
}

func TestTheRecordKeepsTheSameValuesWhateverTheMapOrder(t *testing.T) {
	object := map[string]int{}
	for i := range 600 {
		object[fmt.Sprintf("k%03d", i)] = (100 + i) * 10
	}
	raw, _ := json.Marshal(object)
	first := allValues(recordOf(string(raw)))
	if len(first) != maxValuesOfRead || first[len(first)-1] != (100+maxValuesOfRead-1)*10 {
		t.Fatalf("kept %d values ending in %v, want the first %d keys", len(first), first[len(first)-1], maxValuesOfRead)
	}
	for range 10 {
		if got := allValues(recordOf(string(raw))); !slices.Equal(got, first) {
			t.Fatal("a second read of the same result kept other values")
		}
	}
}

func TestAnOnsetPassesOnATimeAResultShowed(t *testing.T) {
	r := recordOf(`{"anchor":"x","before":[{"time":"2026-11-20T10:05:12Z","body":"timeout"}]}`)
	for onset, ok := range map[string]bool{"2026-11-20T10:05:00Z": true, "2026-11-20T12:00:00Z": false} {
		got := len(Unverified(r.Review(finding("Nothing numeric.", map[string]any{"onset": onset}), ""))) == 0
		if got != ok {
			t.Errorf("onset %s verified = %t, want %t", onset, got, ok)
		}
	}
}

func TestOnlyLabelValuesServicesAndDiscoveryAreNames(t *testing.T) {
	r := recordOf(`{"resultType":"range","status":"error","note":"service","series":[{"labels":{"service_name":"payment","pod":"checkout-7f9c"},"last":3}],"services":[{"name":"cart","spans":4}],"slowest":[{"name":"GET /api","service":"ad"}]}`)
	r.AddRead(nil, []string{"frontend-proxy"}, nil)
	got := append(slices.Clone(r.Names), r.Discovered...)
	slices.Sort(got)
	if want := []string{"ad", "cart", "checkout-7f9c", "frontend-proxy", "payment"}; !slices.Equal(got, want) {
		t.Errorf("names = %q, want %q", got, want)
	}
}

func TestAComponentMatchesAWholeNameOrItsTokens(t *testing.T) {
	r := recordOf(`{"series":[{"labels":{"service_name":"payment","pod":"checkout-7f9c"}}]}`)
	r.AddRead(nil, []string{"frontend-proxy"}, nil)
	for component, ok := range map[string]bool{
		"payment-error-handler": false,
		"payment service":       true,
		"checkout":              true,
		"frontend proxy":        true,
		"pay":                   false,
		"cartservice":           false,
	} {
		in := finding("Nothing numeric.", map[string]any{"root_cause": map[string]any{"component": component, "kind": "dependency", "summary": "s"}})
		if got := len(Unverified(r.Review(in, ""))) == 0; got != ok {
			t.Errorf("component %q verified = %t, want %t", component, got, ok)
		}
	}
}

func TestDiscoveredNamesAreNeverEvicted(t *testing.T) {
	r := NewRecord(Report)
	r.AddRead(nil, []string{"flagd"}, nil)
	labels := make([]map[string]any, 600)
	for i := range labels {
		labels[i] = map[string]any{"labels": map[string]string{"pod": fmt.Sprintf("pod-%d", i)}}
	}
	raw, _ := json.Marshal(labels)
	r.AddRead(raw, nil, nil)
	in := finding("Nothing numeric.", map[string]any{"root_cause": map[string]any{"component": "flagd", "kind": "config_or_flag", "summary": "s"}})
	if got := Unverified(r.Review(in, "")); len(got) != 0 {
		t.Errorf("unverified = %+v, want the discovered name kept", got)
	}
}
