package brief

import (
	"strings"

	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	hypothesesOpen   = "<hypotheses>\nYour ledger so far; send the whole ledger with note_hypotheses when it changes, and do not repeat a read that ruled one out.\n"
	hypothesesClosed = "</hypotheses>"
)

func Hypotheses(ledger []revisions.Hypothesis) string {
	if len(ledger) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(hypothesesOpen)
	for _, h := range ledger {
		b.WriteString("- " + h.ID + " [" + h.Status + "] " + h.Text + "\n")
		for _, line := range h.Evidence {
			b.WriteString("  evidence: " + line + "\n")
		}
		if h.Next != "" {
			b.WriteString("  next: " + h.Next + "\n")
		}
	}
	b.WriteString(hypothesesClosed)
	return b.String()
}
