package guard

import (
	"fmt"
	"slices"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	StageOrient      = "orient"
	StageHypothesize = "hypothesize"
	StageTest        = "test"
	StageConclude    = "conclude"

	PhaseStage = "stage"

	LimitGrowth       = 3
	quietTurnsToNudge = 2

	messageConclude = "Your ledger has a supported cause and nothing open. Call save_finding now with the evidence you read, and say what you did not read."
	messageReminder = "You read for two turns without changing your hypotheses. Send the updated ledger with your next reads, or call save_finding."
)

type Loop struct {
	Limit           int    `json:"limit"`
	Max             int    `json:"max"`
	Base            string `json:"base,omitempty"`
	ReadSinceGrowth bool   `json:"read_since_growth,omitempty"`
	QuietTurns      int    `json:"quiet_turns,omitempty"`
	Reminded        bool   `json:"reminded,omitempty"`
	ConcludeSent    bool   `json:"conclude_sent,omitempty"`
	Shown           string `json:"shown,omitempty"`
	SavedStage      string `json:"saved_stage,omitempty"`
}

func (s *State) StartLoop(callsMax int) {
	if s.Loop != nil || s.Mode != tools.ModeInvestigate {
		return
	}
	limit := s.investigationCalls()
	s.Loop = &Loop{Limit: limit, Max: max(callsMax, limit)}
}

func SameLedger(a, b []revisions.Hypothesis) bool {
	return slices.EqualFunc(a, b, func(x, y revisions.Hypothesis) bool {
		return x.ID == y.ID && x.Text == y.Text && x.Status == y.Status && x.Next == y.Next && slices.Equal(x.Evidence, y.Evidence)
	})
}

func (s *State) NoteLedger(prev, next []revisions.Hypothesis) {
	l := s.Loop
	if l == nil {
		return
	}
	l.Base = baseStage(next)
	if SameLedger(prev, next) {
		return
	}
	s.Turn.LedgerChanged = true
	if l.ReadSinceGrowth && provedSomething(prev, next) {
		if grown := min(l.Limit+LimitGrowth, l.Max); grown > l.Limit {
			l.Limit, l.ReadSinceGrowth = grown, false
		}
	}
}

func baseStage(ledger []revisions.Hypothesis) string {
	if len(ledger) == 0 {
		return StageOrient
	}
	var evidence, supported, open bool
	for _, h := range ledger {
		evidence = evidence || len(h.Evidence) > 0
		supported = supported || h.Status == revisions.HypothesisSupport
		open = open || h.Status == revisions.HypothesisOpen
	}
	switch {
	case !evidence:
		return StageHypothesize
	case supported && !open:
		return StageConclude
	}
	return StageTest
}

func provedSomething(prev, next []revisions.Hypothesis) bool {
	var known []string
	for _, p := range prev {
		known = append(known, p.Evidence...)
	}
	newLine := func(h revisions.Hypothesis) bool {
		return slices.ContainsFunc(h.Evidence, func(line string) bool { return !slices.Contains(known, line) })
	}
	for _, h := range next {
		if h.Status == revisions.HypothesisOpen {
			continue
		}
		i := slices.IndexFunc(prev, func(p revisions.Hypothesis) bool { return p.ID == h.ID })
		switch {
		case i < 0 && newLine(h):
			return true
		case i >= 0 && prev[i].Status == revisions.HypothesisOpen:
			return true
		case i >= 0 && prev[i].Status != h.Status && newLine(h):
			return true
		}
	}
	return false
}

func (s *State) Stage() string {
	l := s.Loop
	switch {
	case l == nil:
		return ""
	case l.Base == StageConclude, s.Window.Calls >= l.Limit-1:
		return StageConclude
	case l.Base == "":
		return StageOrient
	}
	return l.Base
}

func (s *State) StageLine() string {
	if s.Loop == nil || s.Turn.Progress {
		return ""
	}
	return fmt.Sprintf("Stage: %s. Model call %d of %d.", s.Stage(), s.Window.Calls, s.Loop.Limit)
}

func (s *State) StageChange() (stage string, changed bool) {
	l := s.Loop
	if l == nil || s.Turn.Progress || l.Shown == s.Stage() {
		return "", false
	}
	l.Shown = s.Stage()
	return l.Shown, true
}

func (s *State) noteRead(res tools.Result) {
	if s.Loop != nil && !res.IsError {
		s.Loop.ReadSinceGrowth = true
	}
}

func (s *State) loopNudge(limitNudge string) string {
	l := s.Loop
	if l == nil || s.Turn.Progress {
		return limitNudge
	}
	switch {
	case s.Turn.LedgerChanged:
		l.QuietTurns, l.Reminded = 0, false
	case s.Turn.Reads > 0:
		l.QuietTurns++
	default:
		l.QuietTurns = 0
	}
	if l.Base != StageConclude {
		l.ConcludeSent = false
	}
	switch {
	case limitNudge != "":
		return limitNudge
	case l.Base == StageConclude && !l.ConcludeSent:
		l.ConcludeSent = true
		return messageConclude
	case l.Base != "" && l.QuietTurns >= quietTurnsToNudge && !l.Reminded:
		l.Reminded = true
		return messageReminder
	}
	return ""
}
