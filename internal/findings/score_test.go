package findings

import (
	"strings"
	"testing"
)

func TestTheScoreAddsUpTheSignalsTheServerCouldCheck(t *testing.T) {
	metrics := []string{SignalMetrics}
	every := []string{SignalMetrics, SignalLogs, SignalTraces}
	cases := []struct {
		name string
		in   Signals
		want float64
		band string
	}{
		{"nothing", Signals{}, 0, BandLow},
		{"a symptom only", Signals{Kinds: metrics, Component: true}, 0.35, BandLow},
		{"symptom with an aligned onset", Signals{Kinds: metrics, Component: true, OnsetFact: true}, 0.5, BandMedium},
		{"onset only inside a read", Signals{Kinds: metrics, Component: true, OnsetRead: true}, 0.4, BandMedium},
		{"a fact on two signals", Signals{Kinds: []string{SignalMetrics, SignalTraces}, Component: true, OnsetFact: true}, 0.65, BandMedium},
		{"change, onset and a fact", Signals{Kinds: metrics, Component: true, OnsetFact: true, TriggerRead: true}, 0.7, BandHigh},
		{"everything", Signals{Kinds: every, Component: true, OnsetFact: true, OnsetRead: true, TriggerRead: true, RuledOut: true}, 1, BandHigh},
		{"no evidence however much else", Signals{Component: true, OnsetRead: true, TriggerRead: true, RuledOut: true}, 0.3, BandLow},
		{"unknown signal kinds count for nothing", Signals{Kinds: []string{"dashboards"}, Component: true}, 0.1, BandLow},
		{"a kind named twice counts once", Signals{Kinds: []string{SignalTraces, SignalTraces}}, 0.25, BandLow},
	}
	for _, c := range cases {
		got := Score(c.in)
		if got != c.want || BandOf(got) != c.band {
			t.Errorf("%s: score %.2f band %s, want %.2f %s", c.name, got, BandOf(got), c.want, c.band)
		}
	}
}

func TestTheBandDecidesWhetherACauseIsStated(t *testing.T) {
	cases := []struct {
		score  float64
		band   string
		stated bool
	}{
		{0, BandLow, false}, {0.39, BandLow, false}, {StatedFrom, BandMedium, true}, {0.69, BandMedium, true}, {HighFrom, BandHigh, true}, {1, BandHigh, true},
	}
	for _, c := range cases {
		if got := BandOf(c.score); got != c.band || (c.score >= StatedFrom) != c.stated {
			t.Errorf("%.2f: band %s, want %s", c.score, got, c.band)
		}
	}
}

func TestScoringRanksByBandKeepsKnockOnLinksAndNamesDisagreements(t *testing.T) {
	s := Structured{Causes: []Cause{
		{Component: Component{Name: "guess"}, Kind: "unknown", Mechanism: "m", Estimate: BandHigh},
		{Component: Component{Name: "checkout"}, Kind: "dependency", Mechanism: "m", KnockOnOf: new(2)},
		{Component: Component{Name: "cart"}, Kind: "config_or_flag", Mechanism: "m", Estimate: BandHigh},
	}}
	notes := s.Score([]Signals{
		{},
		{Kinds: []string{SignalTraces}, Component: true, OnsetFact: true},
		{Kinds: []string{SignalMetrics}, Component: true, OnsetFact: true, TriggerRead: true},
	})
	names := []string{}
	for _, c := range s.Causes {
		names = append(names, c.Component.Name+":"+c.Band)
	}
	if got := strings.Join(names, ","); got != "cart:high,checkout:medium,guess:low" {
		t.Fatalf("order %s", got)
	}
	if k := s.Causes[1].KnockOnOf; k == nil || *k != 0 {
		t.Errorf("knock-on now points at %v, want 0 (cart)", k)
	}
	if s.Causes[2].Stated || !s.Causes[0].Stated || !s.Causes[1].Stated {
		t.Errorf("stated %+v", s.Causes)
	}
	if s.RootCause == nil || s.RootCause.Component != "cart" {
		t.Errorf("root cause %+v, want the first stated independent cause", s.RootCause)
	}
	note := ScoreNote(notes)
	if !strings.Contains(note, "causes[0] (guess): you estimated high, the evidence scores low (0.00)") || !strings.Contains(note, "causes[0] (guess) scored 0.00, under 0.40, and is listed as unconfirmed") {
		t.Errorf("note %q", note)
	}
	if strings.Contains(note, "cart") {
		t.Errorf("a cause within a band of its estimate is named: %q", note)
	}
}

func TestAFindingWhoseOnlyCauseIsUnconfirmedHasNoRootCause(t *testing.T) {
	s := Structured{Causes: []Cause{{Component: Component{Name: "checkout"}, Kind: "deploy", Mechanism: "m", Stated: true}}}
	s.settle()
	if s.RootCause == nil {
		t.Fatal("an unscored stated cause is not the root cause")
	}
	s.Score([]Signals{{Component: true}})
	if s.RootCause != nil || s.Causes[0].Stated {
		t.Errorf("root cause %+v, want none for an unconfirmed cause", s.RootCause)
	}
}
