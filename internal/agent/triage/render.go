package triage

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	MaxBlockBytes = 6 << 10

	blockOpen  = "<triage>"
	blockClose = "</triage>"
	blockLead  = "Computed by the server before your first call, with no model: a starting point, not evidence. Start from the candidates and test each with your own reads; every figure your answer states needs its own read, and a candidate is a lead, not a cause."

	maxTextBytes     = 120
	maxServicesShown = 12
)

const (
	keep = iota
	prioCandidate
	prioSignals
	prioEvidence
	prioServices
	prioPatterns
	prioBackground
	prioSkipped
)

type line struct {
	prio int
	text string
}

func Render(r Report, budget int) string {
	lines := reportLines(r)
	return fit(lines, budget)
}

func reportLines(r Report) []line {
	lines := []line{{keep, blockOpen}, {keep, blockLead}, {keep, windowsLine(r)}}
	for _, s := range r.Services {
		lines = append(lines, line{prioServices, serviceLine(s)})
	}
	for _, s := range r.Signals {
		lines = append(lines, line{prioSignals, "Signal: " + s})
	}
	if len(r.Candidates) == 0 {
		lines = append(lines, line{keep, "Candidates: none. No service's errors or latency moved against its baseline on any window the sweep read."})
	} else {
		lines = append(lines, line{keep, fmt.Sprintf("Candidates, by onset (change events within %s before an onset):", short(EventLookback))})
	}
	for _, c := range r.Candidates {
		lines = append(lines, candidateLines(c)...)
	}
	for _, b := range r.Background {
		lines = append(lines, line{prioBackground, fmt.Sprintf("Background, not a candidate: %s %s %s, %s before it (%s, %s).", b.Service, b.Signal, figure(b.Window, b.Unit), figure(b.Baseline, b.Unit), short(b.Rung), b.Source)})
	}
	for _, s := range r.Skipped {
		lines = append(lines, line{prioSkipped, fmt.Sprintf("Skipped: %s%s: %s.", s.Step, onSource(s.Source), s.Reason)})
	}
	lines = append(lines, line{keep, fmt.Sprintf("Sweep: %d datasource calls in %s.", r.Calls, r.Duration.Round(100*time.Millisecond))}, line{keep, blockClose})
	return lines
}

func windowsLine(r Report) string {
	rungs := make([]string, len(Rungs))
	for i, w := range Rungs {
		rungs[i] = short(w)
	}
	text := fmt.Sprintf("Windows: %s, each ending %s and compared with the same length just before it.", strings.Join(rungs, ", "), stamp(r.End))
	if r.Question != nil {
		text += fmt.Sprintf(" The question's window is %s to %s.", stamp(r.Question.From), stamp(r.Question.To))
	}
	return text
}

func serviceLine(s ServiceSource) string {
	names := s.Services[:min(len(s.Services), maxServicesShown)]
	text := fmt.Sprintf("Services on %s (%s %s", s.Source, s.Type, s.Signal)
	if s.Field != "" {
		text += ", in " + s.Field
	}
	text += "): " + strings.Join(names, ", ")
	if more := len(s.Services) - len(names); more > 0 {
		text += fmt.Sprintf(" and %d more", more)
	}
	if len(s.Services) == 0 {
		text += "none seen"
	}
	return text
}

func candidateLines(c Candidate) []line {
	parts := make([]string, len(c.Rows))
	for i, row := range c.Rows {
		parts[i] = fmt.Sprintf("%s %s from %s (%s, %s %s)", row.Signal, figure(row.Window, row.Unit), figure(row.Baseline, row.Unit), short(row.Rung), row.Source, row.Basis)
	}
	head := fmt.Sprintf("%s %s: %s; onset %s", c.ID, c.Service, strings.Join(parts, "; "), onsetText(c.Onset))
	if len(c.KnockOn) > 0 {
		head += "; knock-on, relaying its errors: " + strings.Join(c.KnockOn, ", ")
	}
	out := []line{{prioCandidate, head}}
	for _, o := range c.Origins {
		text := fmt.Sprintf("  origin (%s): %s %s", o.Source, o.Service, clipText(o.Operation))
		if o.Message != "" {
			text += " " + strconv.Quote(clipText(o.Message))
		}
		text += fmt.Sprintf(", %d failing traces, example %s", o.Traces, o.Example)
		if len(o.RelayedBy) > 0 {
			text += ", relayed by " + strings.Join(o.RelayedBy, ", ")
		}
		out = append(out, line{prioEvidence, text})
	}
	for _, e := range c.Events {
		text := fmt.Sprintf("  change (%s): %s at %s", sourceOr(e.Source, "SlashObserve"), e.Kind, stamp(e.At))
		if e.Service != "" {
			text += " on " + e.Service
		}
		if e.Text != "" {
			text += ": " + clipText(e.Text)
		}
		out = append(out, line{prioEvidence, text})
	}
	for _, p := range c.Patterns {
		state := fmt.Sprintf("grew to %d from %d", p.Window, p.Baseline)
		if p.New() {
			state = fmt.Sprintf("new, %d lines", p.Window)
		}
		out = append(out, line{prioPatterns, fmt.Sprintf("  log pattern (%s, %s) %s: %s", p.Source, sourceOr(p.Level, "no level"), state, strconv.Quote(clipText(p.Text)))})
	}
	return out
}

func fit(lines []line, budget int) string {
	kept := slices.Clone(lines)
	cut := 0
	for size(kept, cut) > budget {
		i := lowest(kept)
		if i < 0 {
			break
		}
		kept = slices.Delete(kept, i, i+1)
		cut++
	}
	var b strings.Builder
	for i, l := range kept {
		if cut > 0 && i == len(kept)-1 {
			b.WriteString(cutNote(cut) + "\n")
		}
		b.WriteString(l.text)
		if i < len(kept)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func size(lines []line, cut int) int {
	n := 0
	for _, l := range lines {
		n += len(l.text) + 1
	}
	if cut > 0 {
		n += len(cutNote(cut)) + 1
	}
	return n
}

func cutNote(n int) string {
	return fmt.Sprintf("Cut for size: %d lines, skipped steps first, then background, log patterns, service lists, evidence, signals and the last candidates.", n)
}

func lowest(lines []line) int {
	best, at := keep, -1
	for i, l := range lines {
		if l.prio != keep && l.prio >= best {
			best, at = l.prio, i
		}
	}
	return at
}

func figure(v float64, unit string) string {
	switch unit {
	case "ratio":
		return strconv.FormatFloat(v*100, 'g', 3, 64) + "%"
	case "":
		return strconv.FormatFloat(v, 'g', 3, 64)
	}
	return strconv.FormatFloat(v, 'g', 3, 64) + " " + unit
}

func onsetText(t *time.Time) string {
	if t == nil {
		return "not found at the steps read"
	}
	return stamp(*t)
}

func stamp(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func short(d time.Duration) string {
	switch {
	case d%time.Hour == 0:
		return fmt.Sprintf("%dh", d/time.Hour)
	case d%time.Minute == 0:
		return fmt.Sprintf("%dm", d/time.Minute)
	}
	return d.String()
}

func onSource(uid string) string {
	if uid == "" {
		return ""
	}
	return " on " + uid
}

func sourceOr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

func clipText(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= maxTextBytes {
		return s
	}
	cut := maxTextBytes
	for cut > 0 && !utf8Start(s[cut]) {
		cut--
	}
	return s[:cut] + "..."
}

func utf8Start(b byte) bool { return b&0xC0 != 0x80 }
