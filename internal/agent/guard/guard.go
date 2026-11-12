package guard

import (
	"encoding/json"
	"fmt"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	MaxRejectionsInRow     = 3
	maxIdenticalReads      = 3
	maxCallsBeforeSave     = 6
	maxCallsBeforeProposal = 12
	maxStalledTurns        = 2
	maxChangesPerPanel     = 4
	maxChurnedSavesInRow   = 3
	maxIssueCodes          = 3

	PhaseRepair = "repair"

	codeInvalidRequest = "invalid_request"
	toolSaveFinding    = "save_finding"
	toolImportGrafana  = "import_grafana_dashboard"

	StepReusedRead = "Reused the result of an identical call"

	guardSaveNow      = "save_now"
	guardFindingNow   = "finding_now"
	guardProposeNow   = "propose_now"
	guardRepeatedRead = "repeated_read"
	guardRepeatedSave = "repeated_save"
)

const (
	messageReplacement = "This save deleted a panel and added another, which is not progress. Keep the panels you have, save the next panel, or set finish: true when every planned panel is built."
	messageReplayed    = "This is the result of the same earlier call. A third identical call is refused."

	messageSaveNext    = "Save the next panel, or call save_finding if the request only asks a question, in your next call. From then on, reads run only in the same call as a save."
	messageProposeNext = "Propose the panel in your next call. From then on, reads run only in the same call as a proposal."
	messageFindingNext = "One more read at most, then call save_finding."
)

type rule struct {
	message string
	refused string
	stopped string
}

var rules = map[string]rule{
	guardSaveNow: {
		message: fmt.Sprintf("%d model calls without a save; only a save can run now. Save the panel you have checked, or call save_finding if the request only asks a question; reads in the same call as a save still run.", maxCallsBeforeSave),
		refused: fmt.Sprintf("%d model calls passed without a save", maxCallsBeforeSave),
		stopped: fmt.Sprintf("Stopped: the model kept reading after %d calls without a save", maxCallsBeforeSave),
	},
	guardFindingNow: {
		message: fmt.Sprintf("%d model calls without a finding; only save_finding can run now. Save what you found and the evidence you checked.", maxCallsBeforeProposal),
		refused: fmt.Sprintf("%d model calls passed without a finding", maxCallsBeforeProposal),
		stopped: fmt.Sprintf("Stopped: the model kept reading after %d calls without a finding", maxCallsBeforeProposal),
	},
	guardProposeNow: {
		message: fmt.Sprintf("%d model calls without a proposal; only a proposal can run now. Propose the panel you have checked.", maxCallsBeforeProposal),
		refused: fmt.Sprintf("%d model calls passed without a proposal", maxCallsBeforeProposal),
		stopped: fmt.Sprintf("Stopped: the model kept reading after %d calls without a proposal", maxCallsBeforeProposal),
	},
	guardRepeatedRead: {
		message: fmt.Sprintf("this exact call already ran %d times; use its earlier result, change its input, or save", maxIdenticalReads-1),
		refused: fmt.Sprintf("the same read was asked %d times", maxIdenticalReads),
		stopped: fmt.Sprintf("Stopped: the same read was asked %d times", maxIdenticalReads),
	},
	guardRepeatedSave: {
		message: "this save is identical to an earlier one and changes nothing; save the next panel, or set finish: true when every planned panel is built",
		refused: "the same save was sent again",
		stopped: "Stopped: the same save was sent again and again",
	},
}

type Call struct {
	Name  string
	Label string
	Input json.RawMessage
}

func (c Call) label() string {
	if c.Label == "" {
		return c.Name
	}
	return c.Label
}

type Verdict int

const (
	Dispatch Verdict = iota
	Replay
	Reuse
	Refuse
)

type Admission struct {
	Verdict Verdict
	ReuseOf int
	Key     string
	Phase   string
	Content []byte
	Step    string
	Note    string
}

type Stop struct {
	Failure revisions.FailureCode
	Step    string
}

func isSave(name string) bool {
	return tools.Phase(name) == tools.PhaseSave
}

func PhaseFor(name string, repairing bool) string {
	if repairing && isSave(name) {
		return PhaseRepair
	}
	return tools.Phase(name)
}

func (s *State) Phase(name string) string {
	return PhaseFor(name, s.Rejections > 0)
}

func windowLimit(mode tools.Mode) int {
	if mode == tools.ModeSuggest || mode == tools.ModeInvestigate {
		return maxCallsBeforeProposal
	}
	return maxCallsBeforeSave
}

func readRefusal(mode tools.Mode, w Window, hasSave bool) string {
	if hasSave || w.Diagnose || w.Calls < windowLimit(mode) {
		return ""
	}
	switch mode {
	case tools.ModeSuggest:
		return guardProposeNow
	case tools.ModeInvestigate:
		return guardFindingNow
	}
	return guardSaveNow
}

func nudgeFor(mode tools.Mode, w Window) string {
	if w.Calls != windowLimit(mode)-1 {
		return ""
	}
	switch mode {
	case tools.ModeSuggest:
		return messageProposeNext
	case tools.ModeInvestigate:
		return messageFindingNext
	}
	return messageSaveNext
}

func (s *State) Admit(position int, c Call) Admission {
	s.Counts.ToolCalls++
	adm := Admission{Phase: s.Phase(c.Name)}
	if isSave(c.Name) {
		return s.admitSave(c, adm)
	}
	return s.admitRead(position, c, adm)
}

func (s *State) admitRead(position int, c Call, adm Admission) Admission {
	if s.Turn.ReadGuard != "" {
		s.Counts.SaveNowRefusals++
		return s.refuse(c, adm, s.Turn.ReadGuard)
	}
	adm.Key = Key(c.Name, c.Input)
	if first, ok := s.Turn.Keys[adm.Key]; ok {
		if !first.Refused {
			s.Turn.Answered++
		}
		adm.Verdict, adm.ReuseOf, adm.Step = Reuse, first.Position, StepReusedRead
		return adm
	}
	if s.Turn.Keys == nil {
		s.Turn.Keys = map[string]TurnKey{}
	}
	if s.Reads == nil {
		s.Reads = map[string]Read{}
	}
	entry := s.Reads[adm.Key]
	entry.Times++
	s.Reads[adm.Key] = entry
	if entry.Times >= maxIdenticalReads {
		s.Turn.Keys[adm.Key] = TurnKey{Position: position, Refused: true}
		return s.refuse(c, adm, guardRepeatedRead)
	}
	s.Turn.Keys[adm.Key] = TurnKey{Position: position}
	s.Turn.Answered++
	if entry.Replayable {
		adm.Verdict, adm.Step, adm.Note = Replay, StepReusedRead, messageReplayed
	}
	return adm
}

func (s *State) admitSave(c Call, adm Admission) Admission {
	adm.Key = Key(c.Name, c.Input)
	switch {
	case s.Turn.SaveSeen:
		return s.reject(c, adm, "only one save per turn")
	case c.Name == toolSaveFinding && s.Building:
		s.Turn.SaveSeen = true
		return s.reject(c, adm, "this run already saved panels; finish the dashboard instead of saving a finding")
	}
	s.Turn.SaveSeen = true
	if s.noteSave(adm.Key) {
		return s.refuse(c, adm, guardRepeatedSave)
	}
	s.Turn.Answered++
	if s.Rejections > 0 {
		s.Counts.Repairs++
	}
	return adm
}

func (s *State) noteSave(key string) (repeated bool) {
	if s.Saves[key] {
		return true
	}
	if s.Saves == nil {
		s.Saves = map[string]bool{}
	}
	s.Saves[key] = true
	return false
}

func (s *State) reject(c Call, adm Admission, reason string) Admission {
	adm.Verdict = Refuse
	adm.Content, _ = json.Marshal(map[string]string{"code": codeInvalidRequest, "message": reason})
	adm.Step = "Rejected " + c.label() + " input: " + reason
	return adm
}

func (s *State) refuse(c Call, adm Admission, name string) Admission {
	s.LastGuard = name
	adm = s.reject(c, adm, rules[name].message)
	adm.Step = "Refused " + c.label() + ": " + rules[name].refused
	return adm
}

func (s *State) EndTurn() (nudge string, stop *Stop) {
	s.Window.Diagnose = s.Turn.Rejected
	if s.Turn.Answered > 0 {
		s.Window.Stalled = 0
	} else {
		s.Counts.StalledTurns++
		if s.Window.Stalled++; s.Window.Stalled >= maxStalledTurns {
			return "", &Stop{Failure: revisions.FailureNoProgress, Step: s.stalledReason()}
		}
	}
	if !s.Turn.Progress {
		if nudge = nudgeFor(s.Mode, s.Window); nudge != "" {
			s.Counts.Nudges++
		}
	}
	if s.Turn.Rejected && s.Rejections >= MaxRejectionsInRow {
		return nudge, &Stop{
			Failure: revisions.FailureValidationExhausted,
			Step:    fmt.Sprintf("Stopped: %d %s in a row were rejected", MaxRejectionsInRow, rejectedThing(s.Mode)),
		}
	}
	return nudge, nil
}

func (s *State) stalledReason() string {
	if reason := rules[s.LastGuard].stopped; reason != "" {
		return reason
	}
	return fmt.Sprintf("Stopped: %d turns in a row had every call refused", maxStalledTurns)
}

func rejectedThing(mode tools.Mode) string {
	switch mode {
	case tools.ModeSuggest:
		return "proposals"
	case tools.ModeInvestigate:
		return "findings"
	}
	return "saves"
}
