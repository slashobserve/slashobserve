package grounding

import (
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/findings"
)

var testLedger = &Ledger{Facts: []Fact{
	{ID: "f1", Text: "0.5234123", Value: 0.5234123, Kind: FactNumber},
	{ID: "f2", Text: "1312.75", Value: 1312.75, Kind: FactDuration, Unit: "ms"},
	{ID: "f3", Text: "0.4167", Value: 0.4167, Kind: FactRatio},
	{ID: "f4", Text: "1523", Value: 1523, Kind: FactCount},
	{ID: "f5", Text: "4bf92f3577b34da6a3ce929d0e0e4736", Kind: FactTraceID},
	{ID: "f6", Text: "0", Value: 0, Kind: FactNumber},
}}

func TestAPlaceholderRendersTheFactAtFullPrecisionOrWithAnOption(t *testing.T) {
	cases := map[string]string{
		"{{f1}}":       "0.5234123",
		"{{f1:ms}}":    "523.4123 ms",
		"{{f1:s}}":     "0.5234123 s",
		"{{f1:ms:3}}":  "523 ms",
		"{{ f1 : 4 }}": "0.5234",
		"{{f2}}":       "1312.75 ms",
		"{{f2:s}}":     "1.31275 s",
		"{{f2:s:3}}":   "1.31 s",
		"{{f2:bare}}":  "1312.75",
		"{{f3:%}}":     "41.67%",
		"{{f3:%:3}}":   "41.7%",
		"{{f4}}":       "1523",
		"{{f5}}":       "4bf92f3577b34da6a3ce929d0e0e4736",
		"{{f6:2}}":     "0",
		"{{f2:min}}":   "0.0218791666667 min",
	}
	for in, want := range cases {
		got, problems := testLedger.Render("p99 was " + in + ".")
		if got != "p99 was "+want+"." || len(problems) != 0 {
			t.Errorf("Render(%q) = %q, %+v, want %q", in, got, problems, want)
		}
	}
}

func TestABrokenPlaceholderIsAProblemNamingItsID(t *testing.T) {
	cases := []struct {
		in, rendered, fix string
	}{
		{"{{f9}}", UnreadMarker, "f9 is not a fact this run read"},
		{"{{f4:2}}", "1523", "would show 1500, 1.5% away from 1523"},
		{"{{f4:ms}}", "1523", "f4 is a count, so it cannot be shown in ms"},
		{"{{f2:%}}", "1312.75 ms", "f2 is a duration in ms, so it cannot be shown as a percent"},
		{"{{f5:3}}", "4bf92f3577b34da6a3ce929d0e0e4736", "f5 is a trace id, which takes no options"},
		{"{{f1:km}}", "0.5234123", `"km" is not an option for f1`},
	}
	for _, c := range cases {
		got, problems := testLedger.Render(c.in)
		if c.rendered != UnreadMarker && got != c.rendered || len(problems) != 1 || problems[0].Kind != findings.UnverifiedFact || problems[0].Text != c.in || !strings.Contains(problems[0].Fix, c.fix) {
			t.Errorf("Render(%q) = %q, %+v, want %q and a problem saying %q", c.in, got, problems, c.rendered, c.fix)
		}
	}
	if got, _ := testLedger.Render("{{f9}}"); got != UnreadMarker {
		t.Errorf("an unknown fact rendered as %q", got)
	}
}

func TestTextThatIsNotAPlaceholderIsLeftAlone(t *testing.T) {
	for _, text := range []string{
		"`{{.status}}` in line_format",
		"{{fx}} and {{ f }} and {f1} and {{F1}}",
		`sum(rate(x{job="a"}[5m]))`,
	} {
		if got, problems := testLedger.Render(text); got != text || problems != nil {
			t.Errorf("Render(%q) = %q, %+v", text, got, problems)
		}
	}
}

func TestANilLedgerKnowsNoFact(t *testing.T) {
	var l *Ledger
	got, problems := l.Render("{{f1}}")
	if got != UnreadMarker || len(problems) != 1 {
		t.Errorf("Render on no ledger = %q, %+v", got, problems)
	}
}

func TestTheReviewListsBrokenCitationsInEveryTextField(t *testing.T) {
	r := NewRecord(Report)
	r.IssueFacts("measure_window", nil, []byte(`{"stat":"max","values":[{"value":0.5234}]}`), nil)
	got := Unverified(r.Review(finding("p99 peaked at {{f1:ms}}, then {{f7}}.", map[string]any{
		"impact":     "Up to {{f8}} slower.",
		"root_cause": map[string]any{"component": "checkout", "kind": "dependency", "summary": "{{f1:1}} at peak"},
	}), ""))
	want := []findings.Unverified{
		{Kind: findings.UnverifiedFact, Text: "{{f7}}"},
		{Kind: findings.UnverifiedFact, Text: "{{f8}}"},
		{Kind: findings.UnverifiedFact, Text: "{{f1:1}}"},
		{Kind: findings.UnverifiedComponent, Text: "checkout"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("unverified = %+v\nwant %+v", got, want)
	}
}
