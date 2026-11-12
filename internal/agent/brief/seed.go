package brief

import (
	"fmt"
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/internal/dashboards"
)

type Reason string

const (
	Fresh       Reason = ""
	Restart     Reason = "restart"
	OutputLimit Reason = "output_limit"
	Pending     Reason = "pending"
	Conflict    Reason = "conflict"
	Continue    Reason = "continue"
	Renewed     Reason = "renewed"
	CallTimeout Reason = "call_timeout"
	NoFinding   Reason = "no_finding"
	NoAnswer    Reason = "no_answer"
)

var reasons = []Reason{Restart, OutputLimit, Pending, Conflict, Continue, Renewed, CallTimeout, NoFinding, NoAnswer}

func ParseReason(s string) (Reason, bool) {
	if s == "" {
		return Fresh, true
	}
	r := Reason(s)
	return r, slices.Contains(reasons, r)
}

var seedSentences = map[Reason]string{
	Restart:     "The server restarted while you worked.",
	OutputLimit: "Your last answer was cut off at the output limit. Save one panel per call, and check only what that panel needs.",
	Pending:     "You answered without a tool call, but the build has fewer panels than you planned. Add the rest one per save, or resend planned_panels with the number you built, then finish.",
	CallTimeout: "Your last call ran past the time limit for one model call. Keep each answer short: save one panel per call, and check only what that panel needs.",
	Continue:    "Your earlier run stopped before it finished, and the person asked you to continue it from the build above.",
	Renewed:     "Your session was renewed to keep it short; the build above holds every panel you saved.",
	Conflict:    "Someone saved a new version while you worked; your build was discarded. Make the change again on the dashboard above.",
	NoFinding:   "You answered without a tool call. Call save_finding now with your answer and the evidence you checked; your text alone is not saved.",
	NoAnswer:    "Your answer is not shown to anyone. Call save_finding with it, or save a panel.",
}

func (r Reason) shortensConversation() bool {
	return r == Renewed || r == OutputLimit || r == CallTimeout || r == Pending
}

func (r Reason) heading() string {
	if r.shortensConversation() {
		return "Why your conversation was shortened: "
	}
	return "Why this is a new session: "
}

type seed struct {
	build  *dashboards.Build
	reason Reason
	note   string
}

func ResumeReason(t Task) Reason {
	if t.Continues && t.Attempt == 1 {
		return Continue
	}
	return Restart
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
