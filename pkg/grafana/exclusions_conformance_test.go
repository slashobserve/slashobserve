package grafana

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func decisionReasonsIn(res *Result) []string {
	var out []string
	for _, p := range res.Report.Panels {
		for _, reason := range p.Reasons {
			if isDecisionReason(reason) {
				out = append(out, reason)
			}
		}
	}
	for _, n := range res.Report.Notes {
		if isDecisionReason(n.Reason) {
			out = append(out, n.Reason)
		}
	}
	return out
}

func reportedReasons(res *Result) string {
	var parts []string
	for _, p := range res.Report.Panels {
		if len(p.Reasons) > 0 {
			parts = append(parts, fmt.Sprintf("panel %s %q at %s", p.PanelID, p.Reasons, strings.Join(p.Paths, " ")))
		}
	}
	for _, n := range res.Report.Notes {
		parts = append(parts, fmt.Sprintf("note %s %q at %s", n.Subject, n.Reason, strings.Join(n.Paths, " ")))
	}
	return strings.Join(parts, "; ")
}

func variantResult(t *testing.T, a *conformanceArea, e conformanceEntry) *Result {
	t.Helper()
	_, variant, err := a.documents(e)
	if err != nil {
		t.Fatalf("%s %s: %v", a.name, e.ID, err)
	}
	res, err := Convert(variant, conformanceOptions)
	if err != nil {
		t.Fatalf("%s %s: %v", a.name, e.ID, err)
	}
	return res
}

func related(a, b string) bool {
	return isUnder(a, b) || isUnder(b, a)
}

func excludedAt(blocks []Block, full string) bool {
	for _, b := range blocks {
		if b.Outcome != nil && *b.Outcome == outcomeExcluded && related(full, b.Path) {
			return true
		}
		for _, line := range b.Options {
			optionPath := b.Path
			if line.Path != "" {
				optionPath = joinPath(b.Path, line.Path)
			}
			if line.Outcome == outcomeExcluded && related(full, optionPath) {
				return true
			}
		}
		if excludedAt(b.Blocks, full) {
			return true
		}
	}
	return false
}

func blocksJSON(t *testing.T, blocks []Block) string {
	t.Helper()
	raw, err := json.Marshal(blocks)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestExcludedEntriesCarryADecision(t *testing.T) {
	for _, a := range loadConformanceAreas(t, conformanceRoot(t)) {
		for _, e := range a.catalog.Entries {
			if e.expectation() != expectExcluded {
				continue
			}
			t.Run(a.name+"/"+e.ID, func(t *testing.T) {
				res := variantResult(t, a, e)
				if len(decisionReasonsIn(res)) == 0 {
					t.Errorf("path %s is excluded but the report carries no decision: %s", e.Path, reportedReasons(res))
				}
				full := joinPath(a.catalog.Root, e.Path)
				if !excludedAt(res.Report.Blocks, full) {
					t.Errorf("path %s is excluded but no excluded block or option covers it; blocks: %s", full, blocksJSON(t, res.Report.Blocks))
				}
			})
		}
	}
}

func TestEveryDecisionIsCatalogued(t *testing.T) {
	seen := map[string]bool{}
	for _, a := range loadConformanceAreas(t, conformanceRoot(t)) {
		for _, e := range a.catalog.Entries {
			if e.expectation() != expectExcluded {
				continue
			}
			for _, reason := range decisionReasonsIn(variantResult(t, a, e)) {
				seen[reason] = true
			}
		}
	}
	for _, d := range decisions {
		if !seen[d.reason] {
			t.Errorf("decision %s is used by no excluded catalog entry", d.id)
		}
	}
}
