package grounding

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/findings"
)

func sourcedResult(text string) []byte {
	raw, _ := json.Marshal(map[string]any{"notice": "Text from an MCP source.", "source": "cluster", "tool": "kubectl_get", "text": text})
	return raw
}

func TestSourceNamesTakesIdentifiersOfASensibleLengthOnce(t *testing.T) {
	long := strings.Repeat("a", maxNameBytes+1)
	cases := []struct {
		name string
		text string
		want []string
	}{
		{"kubectl table", "NAME                 READY   STATUS\ncheckout-7d9f8b-x2   0/1     CrashLoopBackOff", []string{"NAME", "READY", "STATUS", "checkout-7d9f8b-x2", "CrashLoopBackOff"}},
		{"flag file JSON", `{"flags":{"paymentFailure":{"state":"ENABLED","defaultVariant":"off"}}}`, []string{"flags", "paymentFailure", "state", "ENABLED", "defaultVariant", "off"}},
		{"env and dotted names", "OTEL_SERVICE_NAME=cart kube-system/coredns my.config.v2", []string{"OTEL_SERVICE_NAME", "cart", "kube-system", "coredns", "my.config.v2"}},
		{"edges trimmed", "-deny-all-ingress- .svc. _x_", []string{"deny-all-ingress", "svc"}},
		{"numbers, times and ids are not names", "523 ms at 2026-11-20T10:05:00Z, 0.95, 4bf92f3577b34da6", nil},
		{"too short and too long left out", "a ab abc " + long + " " + long[:maxNameBytes], []string{"abc", long[:maxNameBytes]}},
		{"repeats kept once", "cart cart Cart", []string{"cart", "Cart"}},
		{"no letters", "12 34 -- //", nil},
	}
	for _, c := range cases {
		if got := sourceNames(c.text); !slices.Equal(got, c.want) {
			t.Errorf("%s: names %q, want %q", c.name, got, c.want)
		}
	}
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
	r := NewRecord(Report)
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

func TestASourcedNameSeenAgainMovesToTheNewest(t *testing.T) {
	r := NewRecord(Report)
	r.AddSourced(sourcedResult("alpha beta gamma"))
	r.AddSourced(sourcedResult("alpha"))
	if !slices.Equal(r.Sourced, []string{"beta", "gamma", "alpha"}) {
		t.Errorf("sourced %q", r.Sourced)
	}
}

func TestSourcedTextGivesNoFigureIDOrInstant(t *testing.T) {
	r := NewRecord(Enforce)
	r.AddSourced(sourcedResult("checkout p99 was 9.3 s for 2,950 orders at 2026-11-20T10:05:00Z, trace 4bf92f3577b34da6a3ce929d0e0e4736"))
	if r.Reads != nil || r.IDs != nil || r.Instants != nil || r.Facts != nil || r.Names != nil {
		t.Fatalf("MCP text reached more than the sourced names: %+v", r)
	}
	got := r.Check(json.RawMessage(`{"markdown":"Checkout p99 was 9.3 s for 2,950 orders.","onset":"2026-11-20T10:05:00Z","root_cause":{"component":"checkout","kind":"dependency","summary":"s"}}`))
	want := []findings.Unverified{
		{Kind: findings.UnverifiedFigure, Text: "9.3 s"},
		{Kind: findings.UnverifiedFigure, Text: "2,950"},
		{Kind: findings.UnverifiedOnset, Text: "2026-11-20T10:05:00Z"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("unverified %+v\nwant %+v", got, want)
	}
}

func TestAComponentAndTriggerReadOnlyThroughAnMCPSourceCountAsSeen(t *testing.T) {
	r := NewRecord(Enforce)
	r.AddSourced(sourcedResult(`{"kind":"NetworkPolicy","metadata":{"name":"deny-all-ingress","namespace":"shop"}}`))
	r.AddSourced(sourcedResult(`"adFailure": {"state": "ENABLED", "defaultVariant": "on"}`))
	input := `{"markdown":"m","causes":[{"component":{"kind":"config","name":"deny-all-ingress","namespace":"shop"},"kind":"config_or_flag","mechanism":"m",
		"trigger":{"kind":"feature_flag","key":"adFailure","value":"on"},"evidence":{}}]}`
	if problems := r.Review(json.RawMessage(input), ""); problems != nil {
		t.Errorf("problems %+v", problems)
	}
	got := r.CauseSignals(json.RawMessage(input))
	if len(got) != 1 || !got[0].Component || !got[0].TriggerRead || got[0].Kinds != nil {
		t.Fatalf("signals %+v", got)
	}
	if score := findings.Score(got[0]); score != 0.3 {
		t.Errorf("score %.2f, want the component and trigger points under the no-evidence ceiling", score)
	}
}
