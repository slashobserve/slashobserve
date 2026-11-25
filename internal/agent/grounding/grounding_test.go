package grounding

import (
	"encoding/json"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/findings"
)

func figureTexts(text string) []string {
	var out []string
	for _, f := range Figures(text) {
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
			t.Errorf("Figures(%q) = %q, want %q", c.text, got, c.want)
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
		got := Figures(c.text)
		if len(got) != 1 || got[0].Step != c.step {
			t.Errorf("Figures(%q) = %+v, want one figure with step %v", c.text, got, c.step)
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
	if got := r.Check(in); len(got) != 0 {
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
	if got := r.Check(in); !slices.Equal(got, want) {
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
		if got := len(r.Check(finding(text, nil))) == 0; got != ok {
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
	if got := r.Check(in); len(got) != 0 {
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
	if len(r.Values) != MaxValues {
		t.Fatalf("values = %d, want the cap %d", len(r.Values), MaxValues)
	}
	if newest := r.Values[len(r.Values)-1]; newest != rounded(float64(100+maxValuesOfRead-1)*math.Pow10(9)) {
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
	if r != nil || r.Check(finding("12,345 errors", nil)) != nil {
		t.Error("an off record holds or checks something")
	}
}

func TestTheSendBackListsEachItemWithWhatToDo(t *testing.T) {
	got := SendBack([]findings.Unverified{{Kind: findings.UnverifiedFigure, Text: "37%"}, {Kind: findings.UnverifiedComponent, Text: "cartservice"}})
	want := sendBackLead + "\n- figure \"37%\": read it, correct it, or remove it\n- root_cause.component, not a name this run saw, \"cartservice\": read it, correct it, or remove it"
	if got != want {
		t.Errorf("SendBack =\n%s\nwant\n%s", got, want)
	}
}

func TestParseMode(t *testing.T) {
	for raw, want := range map[string]Mode{"": Off, "off": Off, "report": Report, "enforce": Enforce} {
		if got, err := ParseMode(raw); err != nil || got != want {
			t.Errorf("ParseMode(%q) = %q, %v", raw, got, err)
		}
	}
	if _, err := ParseMode("on"); err == nil {
		t.Error("ParseMode accepted on")
	}
}
