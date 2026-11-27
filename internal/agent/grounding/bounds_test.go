package grounding

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
)

func recordOf(reads ...string) *Record {
	r := NewRecord(Enforce)
	for _, read := range reads {
		r.AddRead([]byte(read), nil, nil)
	}
	return r
}

func allValues(r *Record) []float64 {
	var out []float64
	for _, read := range r.Reads {
		out = append(out, read...)
	}
	slices.Sort(out)
	return out
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

func TestOffRecordsNothing(t *testing.T) {
	r := NewRecord(Off)
	r.AddRead([]byte(`{"v":1}`), []string{"checkout"}, nil)
	if r != nil || Unverified(r.Review(json.RawMessage(`{"markdown":"12,345 errors"}`), "")) != nil {
		t.Error("an off record holds or checks something")
	}
	if r.CauseSignals(json.RawMessage(`{"causes":[{}]}`)) != nil {
		t.Error("an off record gave signals")
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

func TestDiscoveredNamesAreNeverEvicted(t *testing.T) {
	r := NewRecord(Enforce)
	r.AddRead(nil, []string{"flagd"}, nil)
	labels := make([]map[string]any, 600)
	for i := range labels {
		labels[i] = map[string]any{"labels": map[string]string{"pod": fmt.Sprintf("pod-%d", i)}}
	}
	raw, _ := json.Marshal(labels)
	r.AddRead(raw, nil, nil)
	in := json.RawMessage(`{"markdown":"Nothing numeric.","root_cause":{"component":"flagd","kind":"config_or_flag","summary":"s"}}`)
	if got := Unverified(r.Review(in, "")); len(got) != 0 {
		t.Errorf("unverified = %+v, want the discovered name kept", got)
	}
}

func TestTheLedgerDropsItsOldestFactsFirst(t *testing.T) {
	r := NewRecord(Enforce)
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

func TestANilLedgerKnowsNoFact(t *testing.T) {
	var l *Ledger
	got, problems := l.Render("{{f1}}")
	if got != UnreadMarker || len(problems) != 1 {
		t.Errorf("Render on no ledger = %q, %+v", got, problems)
	}
}

func sourcedResult(text string) []byte {
	raw, _ := json.Marshal(map[string]any{"notice": "Text from an MCP source.", "source": "cluster", "tool": "kubectl_get", "text": text})
	return raw
}

func TestSourceNamesKeepsTheFirstNamesOfALongResult(t *testing.T) {
	var words []string
	for i := range maxSourcedOfRead + 50 {
		words = append(words, fmt.Sprintf("name%04d", i))
	}
	got := sourceNames(strings.Join(words, " "))
	if len(got) != maxSourcedOfRead || got[0] != "name0000" || got[len(got)-1] != words[maxSourcedOfRead-1] {
		t.Errorf("kept %d names, first %q, last %q", len(got), got[0], got[len(got)-1])
	}
}

func TestSourcedNamesEvictTheOldestButNeverDiscoveryOrDataNames(t *testing.T) {
	r := NewRecord(Enforce)
	r.AddRead([]byte(`{"series":[{"labels":{"service":"cart"}}]}`), []string{"checkout"}, nil)
	r.AddSourced(sourcedResult("paymentFailure"))
	for batch := range maxSourced/maxSourcedOfRead + 1 {
		var words []string
		for i := range maxSourcedOfRead {
			words = append(words, fmt.Sprintf("b%dn%04d", batch, i))
		}
		r.AddSourced(sourcedResult(strings.Join(words, " ")))
	}
	if len(r.Sourced) != maxSourced {
		t.Fatalf("sourced holds %d names, want %d", len(r.Sourced), maxSourced)
	}
	if r.sawName("paymentFailure") || r.sawName("b0n0000") {
		t.Error("the oldest sourced names were kept")
	}
	if !r.sawName("cart") || !r.sawName("checkout") || !r.sawName("b4n0499") {
		t.Errorf("data, discovery or the newest sourced names were evicted: names %q, discovered %q", r.Names, r.Discovered)
	}
}
