package revisions

const (
	HypothesisOpen     = "open"
	HypothesisSupport  = "supported"
	HypothesisRuledOut = "ruled_out"
)

type Hypothesis struct {
	ID       string   `json:"id"`
	Text     string   `json:"text"`
	Status   string   `json:"status"`
	Evidence []string `json:"evidence,omitempty"`
	Next     string   `json:"next,omitempty"`
}

func (h Hypothesis) Size() int {
	size := len(h.ID) + len(h.Text) + len(h.Status) + len(h.Next)
	for _, line := range h.Evidence {
		size += len(line)
	}
	return size
}
