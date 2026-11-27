package runsession

import (
	"encoding/json"
	"errors"
	"regexp"
	"slices"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/folders"
)

const (
	phaseRepair = "repair"

	codeVersionConflict = "version_conflict"
	codeNotFound        = "not_found"
	codeProposed        = "proposed"

	toolSaveFinding   = "save_finding"
	toolImportGrafana = "import_grafana_dashboard"

	maxIssueCodes = 3
)

var (
	errMalformed = errors.New("save result is malformed")

	plainCode = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`)
	panelID   = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)
)

type saved struct {
	DashboardID string
	Version     int
}

type outcome struct {
	Saved      *saved
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

type saveBody struct {
	Code          string          `json:"code"`
	DashboardID   string          `json:"dashboard_id"`
	Version       int             `json:"version"`
	LatestVersion int             `json:"latest_version"`
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

func (s *State) classifySave(name string, res tools.Result, content []byte) (outcome, error) {
	if res.Refusal == tools.RefusalOnePanel {
		s.Counts.OnePanelRefusals++
	}
	var body saveBody
	if err := json.Unmarshal(content, &body); err != nil {
		return outcome{}, errMalformed
	}
	if !res.IsError {
		out, err := s.classifyAccepted(name, body)
		if err != nil {
			return outcome{}, err
		}
		if s.Rejected > 0 {
			s.Counts.Repairs++
		}
		s.Rejected = 0
		return out, nil
	}
	switch {
	case body.Code == "":
		return outcome{}, errMalformed
	case body.Code == codeVersionConflict && s.Mode == tools.ModeRevise && body.LatestVersion != s.ParentVersion:
		return outcome{Conflict: true}, nil
	case body.Code == codeNotFound && s.Mode == tools.ModeRevise:
		return outcome{NotFound: true}, nil
	case body.Code == folders.CodeRestrictedAnswer:
		s.Rejected = 0
		return outcome{Rejected: true}, nil
	}
	s.Rejected++
	failing, other := body.failingPanels()
	return outcome{
		Rejected:        true,
		IssueCodes:      body.issueCodes(),
		FailingPanels:   failing,
		FailingOther:    other,
		RejectedFinding: s.Mode == tools.ModeInvestigate || name == toolSaveFinding,
		Transient:       body.onlyUnreachable(),
	}, nil
}

func (s *State) classifyAccepted(name string, body saveBody) (outcome, error) {
	switch {
	case body.Code == codeProposed && body.ProposalID != "" && s.Mode == tools.ModeRevise:
		s.Counts.BuildSaves++
		return outcome{Proposal: body.ProposalID}, nil
	case s.Mode == tools.ModeInvestigate || name == toolSaveFinding:
		if body.FindingID == "" {
			return outcome{}, errMalformed
		}
		return outcome{Finding: body.FindingID}, nil
	case s.Mode == tools.ModeSuggest:
		if !body.Suggested || len(body.Panel) == 0 {
			return outcome{}, errMalformed
		}
		return outcome{Suggestion: body.Panel}, nil
	case body.Build != nil && body.Version < 1:
		s.Counts.BuildSaves++
		return outcome{Built: true}, nil
	case body.DashboardID == "" || body.Version < 1:
		return outcome{}, errMalformed
	}
	if name != toolImportGrafana {
		s.Counts.BuildSaves++
	}
	return outcome{Saved: &saved{DashboardID: body.DashboardID, Version: body.Version}}, nil
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
