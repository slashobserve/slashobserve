package grounding

import (
	"regexp"
	"strings"
)

var (
	versionBefore  = regexp.MustCompile(`(?i)\b(?:version|release|v)\s*$`)
	portBefore     = regexp.MustCompile(`(?i)\bports?\s*$|[A-Za-z0-9_\]]:$`)
	statusBefore   = regexp.MustCompile(`(?i)\b(?:http|status|codes?|returned|returning|returns|answered)\s*(?:[1-5]\d\d\s*(?:,|and|or|/)\s*)*$`)
	windowBefore   = regexp.MustCompile(`(?i)\b(?:last|past|every|each|within|step|steps|interval|resolution|scrape|rate)\s+(?:of\s+)?$`)
	windowAfter    = regexp.MustCompile(`(?i)^\s*(?:steps?|windows?|intervals?|resolution|buckets?|scrape)\b`)
	quantileBefore = regexp.MustCompile(`(?i)\b(?:quantile|percentile)\s*$`)
	quantileAfter  = regexp.MustCompile(`(?i)^\s*(?:quantile|percentile)\b`)
	rankBefore     = regexp.MustCompile(`(?i)\b(?:top|first|bottom)\s*$`)
	yearBefore     = regexp.MustCompile(`(?i)\b(?:in|since|year|during|of)\s*$`)
)

type exemption func(f Figure, before, after string) bool

var exemptions = []exemption{
	func(f Figure, before, _ string) bool { return versionBefore.MatchString(before) },
	func(f Figure, before, _ string) bool {
		return f.Unit == "" && f.Step >= 1 && portBefore.MatchString(before)
	},
	func(f Figure, before, _ string) bool {
		return statusCode(f) && statusBefore.MatchString(before)
	},
	func(f Figure, before, after string) bool {
		return isDuration(f.Unit) && (windowBefore.MatchString(before) || windowAfter.MatchString(after))
	},
	func(f Figure, before, after string) bool {
		return f.Unit == "" && (quantileBefore.MatchString(before) || quantileAfter.MatchString(after))
	},
	func(f Figure, before, _ string) bool {
		return f.Unit == "" && f.Step >= 1 && rankBefore.MatchString(before)
	},
	func(f Figure, before, _ string) bool {
		return f.Unit == "" && whole(f) && f.Value >= 1990 && f.Value <= 2100 && yearBefore.MatchString(before)
	},
}

func statusCode(f Figure) bool {
	return (f.Unit == "" || f.Unit == "s" && !strings.Contains(f.Text, " ")) && whole(f) && f.Value >= 100 && f.Value <= 599
}

func whole(f Figure) bool {
	return f.Step >= 1 && !strings.ContainsAny(f.Text, ".,")
}

func isDuration(unit string) bool {
	return unit == "s" || unit == "ms"
}

func UncitedFigures(text, question string) []Figure {
	asked := questionFigures(question)
	blanked := blank(blankIDs(withoutCitations(text)))
	var out []Figure
	for _, f := range scan(blanked) {
		if f.Unit == "" && f.Step >= 1 && f.Value <= 10 {
			continue
		}
		before, after := blanked[:f.start], blanked[f.end:]
		if inQuestion(f, asked) || exempt(f, before, after) {
			continue
		}
		out = append(out, f)
	}
	return out
}

func exempt(f Figure, before, after string) bool {
	for _, e := range exemptions {
		if e(f, before, after) {
			return true
		}
	}
	return false
}

func questionFigures(question string) []Figure {
	if question == "" {
		return nil
	}
	return scan(blank(question))
}

func inQuestion(f Figure, asked []Figure) bool {
	for _, a := range asked {
		if a.Value == f.Value {
			return true
		}
	}
	return false
}
