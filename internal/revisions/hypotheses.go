package revisions

import "slices"

const (
	HypothesisOpen     = "open"
	HypothesisSupport  = "supported"
	HypothesisRuledOut = "ruled_out"

	ClosingCause      = "cause"
	ClosingKnockOn    = "knock_on"
	ClosingBackground = "background"
	ClosingTransient  = "transient"
	ClosingUnrelated  = "unrelated"
)

var Closings = []string{ClosingCause, ClosingKnockOn, ClosingBackground, ClosingTransient, ClosingUnrelated}

type Hypothesis struct {
	ID        string   `json:"id"`
	Text      string   `json:"text"`
	Status    string   `json:"status"`
	Closing   string   `json:"closing,omitempty"`
	KnockOnOf string   `json:"knock_on_of,omitempty"`
	Evidence  []string `json:"evidence,omitempty"`
	Next      string   `json:"next,omitempty"`
}

func (h Hypothesis) Size() int {
	size := len(h.ID) + len(h.Text) + len(h.Status) + len(h.Closing) + len(h.KnockOnOf) + len(h.Next)
	for _, line := range h.Evidence {
		size += len(line)
	}
	return size
}

func ClosingStatus(closing string) string {
	if closing == ClosingCause {
		return HypothesisSupport
	}
	return HypothesisRuledOut
}

func (h Hypothesis) Excluded() bool {
	return slices.Contains([]string{ClosingBackground, ClosingTransient, ClosingUnrelated}, h.Closing)
}
