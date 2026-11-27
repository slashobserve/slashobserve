package triage

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestEachCandidateBecomesALeadWithWhatMovedAndItsOnset(t *testing.T) {
	got := Leads(sampleReport().Candidates)
	want := Lead{
		ID: "c1", Service: "payment",
		Text: "payment: errors 25% from 0.1% (1h); onset 2026-11-20T09:40:00Z",
		Next: "Read payment errors before and after the onset, then close it as cause, knock_on, background, transient or unrelated",
	}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("leads = %+v\nwant %+v", got, want)
	}
}

func TestLeadsAreCappedAndClipped(t *testing.T) {
	var candidates []Candidate
	for i := range MaxLeads + 2 {
		rows := []Row{
			{Signal: SignalErrors, Rung: time.Hour, Window: 0.5, Baseline: 0.01, Unit: "ratio"},
			{Signal: SignalLatency, Rung: 6 * time.Hour, Window: 2500, Baseline: 90, Unit: "ms"},
			{Signal: SignalErrors, Rung: 24 * time.Hour, Window: 0.4, Baseline: 0.01, Unit: "ratio"},
		}
		candidates = append(candidates, Candidate{ID: fmt.Sprintf("c%d", i+1), Service: strings.Repeat("s", 90), Rows: rows})
	}
	got := Leads(candidates)
	if len(got) != MaxLeads || got[MaxLeads-1].ID != "c5" {
		t.Fatalf("leads = %d, last %s", len(got), got[len(got)-1].ID)
	}
	for _, l := range got {
		if n := utf8.RuneCountInString(l.Text); n > maxLeadRunes || !strings.HasSuffix(l.Text, "…") {
			t.Errorf("text is %d runes: %q", n, l.Text)
		}
		if n := utf8.RuneCountInString(l.Next); n > maxLeadRunes || !strings.Contains(l.Next, "errors and latency") {
			t.Errorf("next is %d runes: %q", n, l.Next)
		}
	}
}
