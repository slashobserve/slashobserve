package agent

import (
	"fmt"
	"strings"

	"github.com/slashobserve/slashobserve/internal/dashboards"
)

type seedReason string

const (
	seedFresh       seedReason = ""
	seedRestart     seedReason = "restart"
	seedOutputLimit seedReason = "output_limit"
	seedPending     seedReason = "pending"
	seedConflict    seedReason = "conflict"
	seedContinue    seedReason = "continue"
	seedRenewed     seedReason = "renewed"
	seedCallTimeout seedReason = "call_timeout"
)

var seedSentences = map[seedReason]string{
	seedRestart:     "The server restarted while you worked.",
	seedOutputLimit: "Your last answer was cut off at the output limit. Save one panel per call, and check only what that panel needs.",
	seedPending:     "You answered without a tool call, but the build has fewer panels than you planned. Add the rest one per save, or resend planned_panels with the number you built, then finish.",
	seedCallTimeout: "Your last call ran past the time limit for one model call. Keep each answer short: save one panel per call, and check only what that panel needs.",
	seedContinue:    "Your earlier run stopped before it finished, and the person asked you to continue it from the build above.",
	seedRenewed:     "Your session was renewed to keep it short; the build above holds every panel you saved.",
	seedConflict:    "Someone saved a new version while you worked; your build was discarded. Make the change again on the dashboard above.",
}

func (r seedReason) shortensConversation() bool {
	return r == seedRenewed || r == seedOutputLimit || r == seedCallTimeout
}

func (r seedReason) heading() string {
	if r.shortensConversation() {
		return "Why your conversation was shortened: "
	}
	return "Why this is a new session: "
}

type seed struct {
	build  *dashboards.Build
	reason seedReason
	note   string
}

func resumeReason(t Task) seedReason {
	if t.Continues && t.Attempt == 1 {
		return seedContinue
	}
	return seedRestart
}

func writeBuild(b *strings.Builder, sd seed) {
	p := sd.build.Progress()
	b.WriteString("\n<build>\n")
	b.WriteString("You are building this dashboard. Your saves so far are in the build above; do not redo them.\n")
	if p.PlannedPanels > 0 {
		fmt.Fprintf(b, "Panels built: %d of %d.\n", p.Panels, p.PlannedPanels)
	} else {
		fmt.Fprintf(b, "The build holds %d panels.\n", p.Panels)
	}
	if sentence := seedSentences[sd.reason]; sentence != "" {
		b.WriteString(sd.reason.heading() + sentence + "\n")
	}
	writeContinueNote(b, sd.note)
	b.WriteString("Continue with the next panel, one panel per save, and set finish: true on the last one. If nothing is missing, answer with one short sentence and no tool call.\n")
	b.WriteString("</build>")
}

func writeSeedNote(b *strings.Builder, sd seed) {
	if sentence := seedSentences[sd.reason]; sentence != "" {
		b.WriteString("\n" + sd.reason.heading() + sentence)
	}
	if sd.note != "" {
		b.WriteString("\n")
		writeContinueNote(b, sd.note)
	}
}

func writeContinueNote(b *strings.Builder, note string) {
	if note != "" {
		b.WriteString("The person's note for this run: " + note + "\n")
	}
}
