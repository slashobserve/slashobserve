package runsession

import (
	"encoding/json"
	"slices"

	"github.com/slashobserve/slashobserve/internal/agent/route"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

type SettledKind string

const (
	settledSaved     SettledKind = "saved"
	settledProposed  SettledKind = "proposed"
	settledFinding   SettledKind = "finding"
	settledSuggested SettledKind = "suggested"
	settledRestarted SettledKind = "restarted"
	settledEnded     SettledKind = "ended"
)

type State struct {
	Mode              tools.Mode           `json:"mode"`
	ParentVersion     int                  `json:"parent_version"`
	Rejected          int                  `json:"rejected,omitempty"`
	Counts            Counts               `json:"counts"`
	Restarts          int                  `json:"restarts"`
	Build             *tools.BuildProgress `json:"build,omitempty"`
	Suggestion        json.RawMessage      `json:"suggestion,omitempty"`
	IssueCodes        []string             `json:"issue_codes,omitempty"`
	Failing           []string             `json:"failing_panels,omitempty"`
	FailingOther      int                  `json:"failing_other,omitempty"`
	RejectedFinding   bool                 `json:"rejected_finding,omitempty"`
	RejectedTransient bool                 `json:"rejected_transient,omitempty"`
	Settled           Settled              `json:"settled"`
	Created           []string             `json:"created,omitempty"`
	Wrote             bool                 `json:"wrote,omitempty"`
	Committed         string               `json:"committed,omitempty"`
	Usage             revisions.Usage      `json:"usage"`

	Ledger []revisions.Hypothesis `json:"ledger,omitempty"`

	Task     route.Task `json:"task,omitempty"`
	AllTools bool       `json:"all_tools,omitempty"`

	Agent *tools.AgentState `json:"agent_state,omitempty"`
}

type Counts struct {
	ToolCalls        int `json:"tool_calls"`
	Repairs          int `json:"repairs"`
	BuildSaves       int `json:"build_saves"`
	OnePanelRefusals int `json:"one_panel_refusals"`
}

type Settled struct {
	Kind    SettledKind `json:"kind,omitempty"`
	Version int         `json:"version,omitempty"`
}

func (s *State) rejection() revisions.Rejection {
	return revisions.Rejection{Finding: s.RejectedFinding, Transient: s.RejectedTransient, Panels: s.Failing, Other: s.FailingOther, Codes: s.IssueCodes}
}

func (s *State) writeScope() tools.WriteScope {
	return tools.WriteScope{Created: slices.Clone(s.Created), Committed: s.Committed}
}

func (s *State) noteCreated(ids []string) {
	for _, id := range ids {
		if id != "" && len(s.Created) < tools.MaxCreatedIDs && !slices.Contains(s.Created, id) {
			s.Created = append(s.Created, id)
		}
	}
}

func (s *State) noteLedger(ledger []revisions.Hypothesis) (changed bool) {
	changed = !sameLedger(s.Ledger, ledger)
	s.Ledger = ledger
	return changed
}

func sameLedger(a, b []revisions.Hypothesis) bool {
	return slices.EqualFunc(a, b, func(x, y revisions.Hypothesis) bool {
		return x.ID == y.ID && x.Text == y.Text && x.Status == y.Status && x.Closing == y.Closing && x.KnockOnOf == y.KnockOnOf && x.Next == y.Next && slices.Equal(x.Evidence, y.Evidence)
	})
}

func (s *State) beginSession(parentVersion int) {
	s.ParentVersion = parentVersion
}

func (s *State) restart() {
	s.Rejected = 0
}

func (s *State) phase(name string) string {
	if s.Rejected > 0 && tools.Phase(name) == tools.PhaseSave {
		return phaseRepair
	}
	return tools.Phase(name)
}

func (s *State) meta(restarted bool) RunMeta {
	meta := RunMeta{Build: s.Build}
	switch {
	case s.Settled.Kind == settledEnded:
		meta.Build = nil
		meta.Settled = ptr(settledEnded)
	case restarted:
		meta.Settled = ptr(settledRestarted)
	case s.Settled.Kind != "":
		meta.Settled = ptr(s.Settled.Kind)
	}
	return meta
}

func ptr[T any](v T) *T { return &v }
