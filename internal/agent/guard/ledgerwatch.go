package guard

import (
	"encoding/json"
	"slices"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const faultLedgerBehind = "the finding names a cause or ruled-out items, but every hypothesis in your ledger is still open: send the whole ledger again with note_hypotheses, marking what you supported or ruled out with the evidence you read, then save the finding again"

type LedgerWatch struct {
	Resolved bool `json:"resolved,omitempty"`
	Asked    bool `json:"asked,omitempty"`
}

func (s *State) WatchLedger() {
	if s.Mode == tools.ModeInvestigate && s.Ledger == nil {
		s.Ledger = &LedgerWatch{}
	}
}

func (s *State) noteLedgerResult(res tools.Result) {
	if s.Ledger == nil || res.IsError || res.Ledger == nil {
		return
	}
	s.Ledger.Resolved = slices.ContainsFunc(res.Ledger, func(h revisions.Hypothesis) bool {
		return h.Status != revisions.HypothesisOpen
	})
}

func (s *State) ledgerBehind(input json.RawMessage) string {
	if s.Ledger == nil || s.Ledger.Asked || s.Ledger.Resolved || !namesConclusion(input) {
		return ""
	}
	s.Ledger.Asked = true
	s.Counts.Nudges++
	return faultLedgerBehind
}

func namesConclusion(input json.RawMessage) bool {
	var in struct {
		RootCause json.RawMessage `json:"root_cause"`
		RuledOut  []string        `json:"ruled_out"`
	}
	if json.Unmarshal(input, &in) != nil {
		return false
	}
	cause := string(in.RootCause)
	return len(in.RuledOut) > 0 || (cause != "" && cause != "null" && cause != "{}")
}
