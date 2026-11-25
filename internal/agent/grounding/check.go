package grounding

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/slashobserve/slashobserve/internal/findings"
)

const (
	tolerance    = 0.02
	maxTolerance = 0.1
	onsetSlack   = time.Minute
	minNameRunes = 3
	maxItemRunes = 120

	sendBackLead = "Some figures or names in this finding were not found in what this run read. For each item below: read it, correct it, or remove it, then call save_finding again."
	itemAction   = "read it, correct it, or remove it"
)

type saved struct {
	Markdown  string `json:"markdown"`
	Impact    string `json:"impact"`
	Onset     string `json:"onset"`
	RootCause *struct {
		Component string `json:"component"`
	} `json:"root_cause"`
}

func (r *Record) Check(input json.RawMessage) []findings.Unverified {
	if r == nil {
		return nil
	}
	var in saved
	if json.Unmarshal(input, &in) != nil {
		return nil
	}
	seen := r.sortedReads()
	var out []findings.Unverified
	add := func(kind, text string) {
		item := findings.Unverified{Kind: kind, Text: clip(text)}
		if len(out) < findings.MaxUnverified && !slices.Contains(out, item) {
			out = append(out, item)
		}
	}
	for _, text := range []string{in.Markdown, in.Impact} {
		for _, f := range Figures(text) {
			if !seen.found(f) {
				add(findings.UnverifiedFigure, f.Text)
			}
		}
	}
	if in.RootCause != nil && strings.TrimSpace(in.RootCause.Component) != "" && !r.sawName(in.RootCause.Component) {
		add(findings.UnverifiedComponent, strings.TrimSpace(in.RootCause.Component))
	}
	if onset := strings.TrimSpace(in.Onset); onset != "" {
		if at, err := time.Parse(time.RFC3339, onset); err == nil && !r.queried(at) {
			add(findings.UnverifiedOnset, onset)
		}
	}
	return out
}

func clip(text string) string {
	runes := []rune(text)
	if len(runes) <= maxItemRunes {
		return text
	}
	return string(runes[:maxItemRunes])
}

type interval struct{ lo, hi float64 }

var unitless = map[string]bool{"": true, "%": true, "x": true}

func intervalsOf(f Figure) []interval {
	spread := min(max(tolerance*f.Value, f.Step/2), maxTolerance*f.Value)
	scales := scalesOf(f.Unit)
	out := make([]interval, 0, len(scales))
	for _, scale := range scales {
		out = append(out, interval{lo: (f.Value - spread) * scale, hi: (f.Value + spread) * scale})
	}
	return out
}

type sortedReads struct {
	all   []float64
	reads [][]float64
}

func (r *Record) sortedReads() sortedReads {
	var out sortedReads
	for _, read := range r.Reads {
		sorted := slices.Clone(read)
		sort.Float64s(sorted)
		out.reads = append(out.reads, sorted)
		out.all = append(out.all, read...)
	}
	sort.Float64s(out.all)
	return out
}

func (s sortedReads) found(f Figure) bool {
	intervals := intervalsOf(f)
	for _, in := range intervals {
		if anyIn(s.all, in.lo, in.hi) {
			return true
		}
	}
	if f.Unit == "%" {
		fraction := intervals[1]
		changes := []interval{{1 + fraction.lo, 1 + fraction.hi}, {1 - fraction.hi, 1 - fraction.lo}}
		return slices.ContainsFunc(s.reads, func(read []float64) bool {
			return derived(read, fraction) || ratioIn(read, fraction) || ratioIn(read, changes[0]) || ratioIn(read, changes[1])
		})
	}
	ratios := unitless[f.Unit]
	return slices.ContainsFunc(s.reads, func(read []float64) bool {
		for _, in := range intervals {
			if derived(read, in) || ratios && ratioIn(read, in) {
				return true
			}
		}
		return false
	})
}

func derived(values []float64, in interval) bool {
	if in.hi <= 0 {
		return false
	}
	lo := max(in.lo, 0)
	for _, a := range values {
		if otherIn(values, a, a-in.hi, a-lo) || a <= in.hi && otherIn(values, a, lo-a, in.hi-a) {
			return true
		}
	}
	return false
}

func ratioIn(values []float64, in interval) bool {
	if in.hi <= 0 {
		return false
	}
	upper := math.Inf(1)
	if in.lo > 0 {
		upper = 1 / in.lo
	}
	for _, a := range values {
		if a > 0 && otherIn(values, a, a/in.hi, a*upper) {
			return true
		}
	}
	return false
}

func anyIn(sorted []float64, lo, hi float64) bool {
	i := sort.SearchFloat64s(sorted, lo)
	return i < len(sorted) && sorted[i] <= hi
}

func otherIn(sorted []float64, self, lo, hi float64) bool {
	for i := sort.SearchFloat64s(sorted, lo); i < len(sorted) && sorted[i] <= hi; i++ {
		if sorted[i] != self {
			return true
		}
	}
	return false
}

func (r *Record) sawName(component string) bool {
	want := normalName(component)
	if len([]rune(want)) == 0 {
		return true
	}
	for _, name := range r.Names {
		seen := normalName(name)
		if len([]rune(seen)) < minNameRunes {
			continue
		}
		if seen == want || strings.Contains(want, seen) || strings.Contains(seen, want) {
			return true
		}
	}
	return false
}

func normalName(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, s)
}

func (r *Record) queried(at time.Time) bool {
	for _, rng := range r.Ranges {
		if !at.Before(rng.From.Add(-onsetSlack)) && !at.After(rng.To.Add(onsetSlack)) {
			return true
		}
	}
	return slices.ContainsFunc(r.Instants, func(seen time.Time) bool {
		return at.Sub(seen).Abs() <= onsetSlack
	})
}

func SendBack(items []findings.Unverified) string {
	lines := []string{sendBackLead}
	for _, item := range items {
		lines = append(lines, fmt.Sprintf("- %s %q: %s", describe(item.Kind), item.Text, itemAction))
	}
	return strings.Join(lines, "\n")
}

func describe(kind string) string {
	switch kind {
	case findings.UnverifiedComponent:
		return "root_cause.component, not a name this run saw,"
	case findings.UnverifiedOnset:
		return "onset, outside every time range this run queried,"
	}
	return "figure"
}
