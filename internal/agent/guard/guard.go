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
	maxWritesPerAttempt    = 8

	DefaultInvestigationCalls = 12

	PhaseRepair = "repair"

	codeInvalidRequest = "invalid_request"
	toolSaveFinding    = "save_finding"
	toolImportGrafana  = "import_grafana_dashboard"

	StepReusedRead = "Reused the result of an identical call"

	guardSaveNow       = "save_now"
	guardFindingNow    = "finding_now"
	guardProposeNow    = "propose_now"
	guardRepeatedRead  = "repeated_read"
	guardRepeatedSave  = "repeated_save"
	guardTooManyWrites = "too_many_writes"
	guardRepeatedWrite = "repeated_write"
)

const (
	messageReplacement = "This save deleted a panel and added another, which is not progress. Keep the panels you have, save the next panel, or set finish: true when every planned panel is built."
	messageReplayed    = "This is the result of the same earlier call. A third identical call is refused."

	messageSaveNext    = "Save the next panel, or call save_finding if the request only asks a question, in your next call. From then on, reads run only in the same call as a save."
	messageAnswerNext  = "Save the first panel if the request asks for a dashboard, otherwise call save_finding with what you found, in your next call. From then on, reads run only in the same call as a save."
	messageProposeNext = "Propose the panel in your next call. From then on, reads run only in the same call as a proposal."
	messageFindingNext = "One more read at most, then call save_finding."
)

type rule struct {
	message string
	refused string
	stopped string
}

var rules = map[string]rule{
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
	guardTooManyWrites: {
		message: fmt.Sprintf("this run already made %d changes, the most one run may; say in your answer what is left for the person to do", maxWritesPerAttempt),
		refused: fmt.Sprintf("%d writes already ran", maxWritesPerAttempt),
		stopped: fmt.Sprintf("Stopped: the model kept writing after %d writes", maxWritesPerAttempt),
	},
	guardRepeatedWrite: {
		message: "this change is identical to an earlier one in this run and changes nothing; say what you did instead of repeating it",
		refused: "the same write was sent again",
		stopped: "Stopped: the same write was sent again and again",
	},
}

func saveRule(calls int, answering bool) rule {
	choice := "Save the panel you have checked, or call save_finding if the request only asks a question"
	if answering {
		choice = "Save the first panel if the request asks for a dashboard, otherwise call save_finding with what you found"
	}
	return rule{
		message: fmt.Sprintf("%d model calls without a save; only a save can run now. %s, and say in it what you did not read; reads in the same call as a save still run.", calls, choice),
		refused: fmt.Sprintf("%d model calls passed without a save", calls),
		stopped: fmt.Sprintf("Stopped: the model kept reading after %d calls without a save", calls),
	}
}

func findingRule(calls int) rule {
	return rule{
		message: fmt.Sprintf("%d model calls without a finding; only save_finding can run now. Save what you found and the evidence you checked, and say in it what you did not read.", calls),
		refused: fmt.Sprintf("%d model calls passed without a finding", calls),
		stopped: fmt.Sprintf("Stopped: the model kept reading after %d calls without a finding", calls),
	}
}

func (s *State) rule(name string) rule {
	return s.ruleAt(name, s.windowLimit())
}

func (s *State) ruleAt(name string, calls int) rule {
	if calls == 0 {
		calls = s.windowLimit()
	}
	switch name {
	case guardFindingNow:
		return findingRule(calls)
	case guardSaveNow:
		return saveRule(calls, s.answering())
	}
	return rules[name]
}

func (s *State) answering() bool {
	return s.Mode == tools.ModeCreate && !s.Building && !s.Saved
}

func (s *State) investigationCalls() int {
	if s.InvestigationCalls > 0 {
		return s.InvestigationCalls
	}
	return DefaultInvestigationCalls
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

	ReadsRefused int
	Unchecked    []tools.Unchecked
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

func (s *State) windowLimit() int {
	switch s.Mode {
	case tools.ModeSuggest:
		return maxCallsBeforeProposal
	case tools.ModeInvestigate:
		return s.investigationCalls()
	}
	if s.answering() {
		return s.investigationCalls()
	}
	return maxCallsBeforeSave
}

func (s *State) readRefusal(hasSave bool) string {
	if hasSave || s.Window.Diagnose || s.Window.Calls < s.windowLimit() {
		return ""
	}
	switch s.Mode {
	case tools.ModeSuggest:
		return guardProposeNow
	case tools.ModeInvestigate:
		return guardFindingNow
	}
	return guardSaveNow
}

func (s *State) nudge() string {
	if s.Window.Calls != s.windowLimit()-1 {
		return ""
	}
	switch s.Mode {
	case tools.ModeSuggest:
		return messageProposeNext
	case tools.ModeInvestigate:
		return messageFindingNext
	}
	if s.answering() {
		return messageAnswerNext
	}
	return messageSaveNext
}

func (s *State) Admit(position int, c Call) Admission {
	s.Counts.ToolCalls++
	adm := Admission{Phase: s.Phase(c.Name)}
	switch tools.Phase(c.Name) {
	case tools.PhaseSave:
		return s.admitSave(c, adm)
	case tools.PhaseWrite:
		return s.admitWrite(c, adm)
	}
	return s.admitRead(position, c, adm)
}

func (s *State) admitWrite(c Call, adm Admission) Admission {
	adm.Key = writeKey(c)
	switch {
	case s.Writes >= maxWritesPerAttempt:
		return s.refuse(c, adm, guardTooManyWrites)
	case s.WriteKeys[adm.Key]:
		return s.refuse(c, adm, guardRepeatedWrite)
	}
	if s.WriteKeys == nil {
		s.WriteKeys = map[string]bool{}
	}
	s.WriteKeys[adm.Key] = true
	s.Writes++
	s.Turn.Answered++
	return adm
}

func writeKey(c Call) string {
	var parsed any
	if json.Unmarshal(c.Input, &parsed) == nil {
		if canonical, err := json.Marshal(parsed); err == nil {
			return Key(c.Name, canonical)
		}
	}
	return Key(c.Name, c.Input)
}

func (s *State) admitRead(position int, c Call, adm Admission) Admission {
	if s.Turn.ReadGuard != "" {
		s.Counts.SaveNowRefusals++
		s.Window.Refused++
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
	case c.Name == toolSaveFinding:
		if reason := s.unbackedAbsence(c.Input); reason != "" {
			s.Absence.Asked = true
			return s.reject(c, adm, reason)
		}
	}
	s.Turn.SaveSeen = true
	if s.noteSave(adm.Key) {
		return s.refuse(c, adm, guardRepeatedSave)
	}
	s.Turn.Answered++
	if c.Name == toolSaveFinding {
		adm.ReadsRefused = s.Window.Refused
		adm.Unchecked = s.uncheckedReads()
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
	return s.rejectWith(c, adm, codeInvalidRequest, reason)
}

func (s *State) rejectWith(c Call, adm Admission, code, reason string) Admission {
	adm.Verdict = Refuse
	adm.Content, _ = json.Marshal(map[string]string{"code": code, "message": reason})
	adm.Step = "Rejected " + c.label() + " input: " + reason
	return adm
}

func (s *State) refuse(c Call, adm Admission, name string) Admission {
	s.LastGuard, s.LastLimit = name, s.windowLimit()
	code := codeInvalidRequest
	if name == guardTooManyWrites || name == guardRepeatedWrite {
		code = name
	}
	adm = s.rejectWith(c, adm, code, s.rule(name).message)
	adm.Step = "Refused " + c.label() + ": " + s.rule(name).refused
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
		if nudge = s.nudge(); nudge != "" {
			s.Counts.Nudges++
		}
	}
	if s.Turn.Rejected && s.RestrictedRefusals >= MaxRejectionsInRow {
		return nudge, &Stop{
			Failure: revisions.FailureRestrictedAnswer,
			Step:    fmt.Sprintf("Stopped: %d %s in a row were refused because the run read data only Admins can read", MaxRejectionsInRow, rejectedThing(s.Mode)),
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
	if reason := s.ruleAt(s.LastGuard, s.LastLimit).stopped; reason != "" {
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
