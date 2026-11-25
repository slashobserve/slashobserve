package guard

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	codeVersionConflict = "version_conflict"
	codeNotFound        = "not_found"
	codeProposed        = "proposed"
)

var (
	ErrMalformed = errors.New("save result is malformed")

	plainCode = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`)
	panelID   = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)
)

type Saved struct {
	DashboardID string
	Version     int
	Replayed    bool
}

type Outcome struct {
	Saved      *Saved
	Built      bool
	Suggestion json.RawMessage
	Finding    string
	Proposal   string
	Conflict   bool
	NotFound   bool
	Rejected   bool
	IssueCodes []string

	RejectedFinding bool
	Transient       bool

	FailingPanels []string
	FailingOther  int
}

type Settled struct {
	Outcome    Outcome
	Note       string
	Replayable bool
	Stop       *Stop
}

type saveBody struct {
	Code          string          `json:"code"`
	DashboardID   string          `json:"dashboard_id"`
	Version       int             `json:"version"`
	LatestVersion int             `json:"latest_version"`
	Replayed      *bool           `json:"replayed"`
	Suggested     bool            `json:"suggested"`
	FindingID     string          `json:"finding_id"`
	ProposalID    string          `json:"proposal_id"`
	Build         *struct{}       `json:"build"`
	Panel         json.RawMessage `json:"panel"`
	FailingPanels []string        `json:"failing_panels"`
	Issues        []issueBody     `json:"issues"`
}

type issueBody struct {
	Code string `json:"code"`
}

func (s *State) Settle(c Call, res tools.Result, content []byte) (Settled, error) {
	switch tools.Phase(c.Name) {
	case tools.PhaseWrite:
		if res.IsError {
			s.ForgetWrite(c)
		}
		return Settled{}, nil
	case tools.PhaseLedger:
		return Settled{}, nil
	case tools.PhaseSave:
	default:
		s.noteRead(res)
		return s.settleRead(c, res), nil
	}
	stage := s.Stage()
	if res.Refusal == tools.RefusalOnePanel {
		s.Counts.OnePanelRefusals++
	}
	settled, err := s.classifySave(c, res, content)
	if err != nil {
		return Settled{}, err
	}
	out := settled.Outcome
	if out.Finding != "" && s.Loop != nil {
		s.Loop.SavedStage = stage
	}
	s.Turn.Progress = out.Built || out.Saved != nil || out.Suggestion != nil || out.Finding != "" || out.Proposal != ""
	s.Turn.Rejected = out.Rejected
	return settled, nil
}

func (s *State) settleRead(c Call, res tools.Result) Settled {
	s.noteChecked(c, res)
	s.noteAbsenceRead(c, res)
	s.noteQueried(c, res)
	key := Key(c.Name, c.Input)
	entry, ok := s.Reads[key]
	if !ok || res.IsError || tools.Phase(c.Name) == tools.PhaseQuery {
		return Settled{}
	}
	entry.Replayable = true
	s.Reads[key] = entry
	return Settled{Replayable: true}
}

func (s *State) classifySave(c Call, res tools.Result, content []byte) (Settled, error) {
	var body saveBody
	if err := json.Unmarshal(content, &body); err != nil {
		return Settled{}, ErrMalformed
	}
	if !res.IsError {
		repairing := s.Rejections > 0
		settled, err := s.classifyOK(c, res, body)
		if err != nil {
			return Settled{}, err
		}
		if repairing {
			s.Counts.Repairs++
		}
		s.Rejections, s.RestrictedRefusals = 0, 0
		return settled, nil
	}
	switch {
	case body.Code == "":
		return Settled{}, ErrMalformed
	case body.Code == codeVersionConflict && s.Mode == tools.ModeRevise && body.LatestVersion != s.ParentVersion:
		return Settled{Outcome: Outcome{Conflict: true}}, nil
	case body.Code == codeNotFound && s.Mode == tools.ModeRevise:
		return Settled{Outcome: Outcome{NotFound: true}}, nil
	}
	if body.Code == folders.CodeRestrictedAnswer {
		s.RestrictedRefusals++
		s.Rejections = 0
		return Settled{Outcome: Outcome{Rejected: true}}, nil
	}
	s.Rejections++
	s.RestrictedRefusals = 0
	failing, other := body.failingPanels()
	return Settled{Outcome: Outcome{
		Rejected:        true,
		IssueCodes:      body.issueCodes(),
		FailingPanels:   failing,
		FailingOther:    other,
		RejectedFinding: s.Mode == tools.ModeInvestigate || c.label() == toolSaveFinding,
		Transient:       body.onlyUnreachable(),
	}}, nil
}

func (s *State) classifyOK(c Call, res tools.Result, body saveBody) (Settled, error) {
	if body.Code == codeProposed && body.ProposalID != "" && s.Mode == tools.ModeRevise {
		s.Counts.BuildSaves++
		s.Window = Window{}
		return Settled{Outcome: Outcome{Proposal: body.ProposalID}}, nil
	}
	return s.classifyAccepted(c, res, body)
}

func (s *State) classifyAccepted(c Call, res tools.Result, body saveBody) (Settled, error) {
	switch {
	case s.Mode == tools.ModeInvestigate || c.label() == toolSaveFinding:
		if body.FindingID == "" {
			return Settled{}, ErrMalformed
		}
		s.Window = Window{}
		return Settled{Outcome: Outcome{Finding: body.FindingID}}, nil
	case s.Mode == tools.ModeSuggest:
		if !body.Suggested || len(body.Panel) == 0 {
			return Settled{}, ErrMalformed
		}
		return Settled{Outcome: Outcome{Suggestion: body.Panel}}, nil
	case body.Build != nil && body.Version < 1:
		return s.classifyBuildSave(res), nil
	case body.DashboardID == "" || body.Version < 1:
		return Settled{}, ErrMalformed
	}
	if c.label() != toolImportGrafana {
		s.Counts.BuildSaves++
	}
	s.Saved = true
	s.Window = Window{}
	return Settled{Outcome: Outcome{Saved: &Saved{
		DashboardID: body.DashboardID,
		Version:     body.Version,
		Replayed:    body.Replayed != nil && *body.Replayed,
	}}}, nil
}

func (s *State) classifyBuildSave(res tools.Result) Settled {
	s.Building = true
	s.Window = Window{}
	s.Counts.BuildSaves++
	s.Reads = nil
	note, stop := s.noteChurn(res.Edits)
	if stop {
		return Settled{Stop: &Stop{
			Failure: revisions.FailureNoProgress,
			Step:    fmt.Sprintf("Stopped: %d saves in a row changed nothing new", maxChurnedSavesInRow+1),
		}}
	}
	return Settled{Outcome: Outcome{Built: true}, Note: note}
}

func (b saveBody) failingPanels() (ids []string, other int) {
	var seen []string
	for _, id := range b.FailingPanels {
		if slices.Contains(seen, id) {
			continue
		}
		seen = append(seen, id)
		if panelID.MatchString(id) {
			ids = append(ids, id)
		} else {
			other++
		}
	}
	return ids, other
}

func (b saveBody) onlyUnreachable() bool {
	return len(b.Issues) > 0 && !slices.ContainsFunc(b.Issues, func(issue issueBody) bool { return !tools.CouldNotCheck(issue.Code) })
}

func (b saveBody) issueCodes() []string {
	var codes []string
	for _, issue := range b.Issues {
		if plainCode.MatchString(issue.Code) && !slices.Contains(codes, issue.Code) {
			codes = append(codes, issue.Code)
		}
		if len(codes) == maxIssueCodes {
			break
		}
	}
	if len(codes) == 0 && plainCode.MatchString(b.Code) {
		codes = []string{b.Code}
	}
	return codes
}
