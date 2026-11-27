package findings

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strings"
)

// The weights, the no-evidence ceiling and the band edges are documented in
// docs/concepts/investigate.md; change both together.
const (
	BandHigh   = "high"
	BandMedium = "medium"
	BandLow    = "low"

	HighFrom   = 0.7
	StatedFrom = 0.4

	weightFirstSignal = 0.25
	weightMoreSignal  = 0.15
	weightComponent   = 0.1
	weightOnsetFact   = 0.15
	weightOnsetRead   = 0.05
	weightTrigger     = 0.2
	weightRuledOut    = 0.05
	noEvidenceCeiling = 0.3

	SignalMetrics = "metrics"
	SignalLogs    = "logs"
	SignalTraces  = "traces"
)

type Signals struct {
	Kinds       []string
	Component   bool
	OnsetRead   bool
	OnsetFact   bool
	TriggerRead bool
	RuledOut    bool
}

func Score(s Signals) float64 {
	kinds := 0
	for _, k := range []string{SignalMetrics, SignalLogs, SignalTraces} {
		if slices.Contains(s.Kinds, k) {
			kinds++
		}
	}
	score := 0.0
	if kinds > 0 {
		score += weightFirstSignal + float64(kinds-1)*weightMoreSignal
	}
	add := func(on bool, w float64) {
		if on {
			score += w
		}
	}
	add(s.Component, weightComponent)
	switch {
	case s.OnsetFact:
		score += weightOnsetFact
	case s.OnsetRead:
		score += weightOnsetRead
	}
	add(s.TriggerRead, weightTrigger)
	add(s.RuledOut, weightRuledOut)
	if kinds == 0 {
		score = min(score, noEvidenceCeiling)
	}
	return math.Round(min(score, 1)*100) / 100
}

func BandOf(score float64) string {
	switch {
	case score >= HighFrom:
		return BandHigh
	case score >= StatedFrom:
		return BandMedium
	}
	return BandLow
}

func bandRank(band string) int {
	return slices.Index([]string{BandLow, BandMedium, BandHigh}, band)
}

type GivenScore struct {
	Score *float64
	Band  string
}

func (s *Structured) Score(signals []Signals) []string {
	return s.ScoreGiven(signals, nil)
}

func (s *Structured) ScoreGiven(signals []Signals, given []GivenScore) []string {
	for i := range s.Causes {
		var sig Signals
		if i < len(signals) {
			sig = signals[i]
		}
		score, band := Score(sig), ""
		if i < len(given) && given[i].Score != nil && !math.IsNaN(*given[i].Score) {
			score = math.Round(min(max(*given[i].Score, 0), 1)*100) / 100
			if bandRank(given[i].Band) >= 0 {
				band = given[i].Band
			}
		}
		c := &s.Causes[i]
		c.Score, c.Band, c.Stated = &score, cmp.Or(band, BandOf(score)), score >= StatedFrom
	}
	notes := s.disagreements()
	s.rank()
	s.settle()
	return notes
}

func (s *Structured) disagreements() []string {
	var notes []string
	for i, c := range s.Causes {
		if c.Estimate != "" && abs(bandRank(c.Estimate)-bandRank(c.Band)) > 1 {
			notes = append(notes, fmt.Sprintf("causes[%d] (%s): you estimated %s, the evidence scores %s (%.2f)", i, c.Component.Text(), c.Estimate, c.Band, *c.Score))
		}
	}
	for i, c := range s.Causes {
		if !c.Stated {
			notes = append(notes, fmt.Sprintf("causes[%d] (%s) scored %.2f, under %.2f, and is listed as unconfirmed", i, c.Component.Text(), *c.Score, StatedFrom))
		}
	}
	return notes
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func (s *Structured) rank() {
	order := make([]int, len(s.Causes))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int {
		return cmp.Compare(bandRank(s.Causes[b].Band), bandRank(s.Causes[a].Band))
	})
	position := make([]int, len(order))
	ranked := make([]Cause, len(order))
	for to, from := range order {
		position[from] = to
		ranked[to] = s.Causes[from]
	}
	for i := range ranked {
		if k := ranked[i].KnockOnOf; k != nil {
			ranked[i].KnockOnOf = new(position[*k])
		}
	}
	s.Causes = ranked
}

func ScoreNote(notes []string) string {
	if len(notes) == 0 {
		return ""
	}
	return "Confidence comes from the evidence, not the estimate: " + strings.Join(notes, "; ") + "."
}
