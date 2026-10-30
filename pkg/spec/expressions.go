package spec

import (
	"fmt"
	"slices"
	"strconv"
)

const (
	ExpressionMath      = "math"
	ExpressionReduce    = "reduce"
	ExpressionResample  = "resample"
	ExpressionThreshold = "threshold"

	ModeReplaceNonNumbers = "replaceNonNumbers"
)

var thresholdParams = map[string]int{
	"gt":                   1,
	"lt":                   1,
	"withinRange":          2,
	"outsideRange":         2,
	"withinRangeIncluded":  2,
	"outsideRangeIncluded": 2,
}

func (q Query) IsExpression() bool { return q.Expression != nil }

func (e Expression) Inputs() []string {
	if e.Type == ExpressionMath {
		return MathRefs(e.Math)
	}
	if e.Input == "" {
		return nil
	}
	return []string{e.Input}
}

func MathRefs(text string) []string {
	var refs []string
	for i := 0; i < len(text); i++ {
		if text[i] != '$' {
			continue
		}
		start, braced := i+1, false
		if start < len(text) && text[start] == '{' {
			start, braced = start+1, true
		}
		end := start
		for end < len(text) && isRefByte(text[end], end == start) {
			end++
		}
		if end == start || (braced && (end >= len(text) || text[end] != '}')) {
			continue
		}
		if ref := text[start:end]; !slices.Contains(refs, ref) {
			refs = append(refs, ref)
		}
		i = end - 1
	}
	return refs
}

func isRefByte(b byte, first bool) bool {
	letter := b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z'
	return letter || (!first && (b >= '0' && b <= '9' || b == '_'))
}

func checkExpressions(entries []PanelEntry) []Issue {
	var issues []Issue
	for _, e := range entries {
		if !e.Panel.Type.IsData() {
			continue
		}
		issues = append(issues, checkPanelExpressions(e)...)
	}
	return issues
}

func checkPanelExpressions(e PanelEntry) []Issue {
	p := e.Panel
	index := map[string]int{}
	for j := range p.Queries {
		if _, taken := index[p.QueryRef(j)]; !taken {
			index[p.QueryRef(j)] = j
		}
	}
	var issues []Issue
	edges := map[int][]int{}
	for j, q := range p.Queries {
		if !q.IsExpression() {
			continue
		}
		base := e.Pointer + pointer("queries", strconv.Itoa(j), "expression")
		issues = append(issues, checkExpressionShape(*q.Expression, base)...)
		inputPath := base + "/input"
		if q.Expression.Type == ExpressionMath {
			inputPath = base + "/math"
		}
		for _, ref := range q.Expression.Inputs() {
			k, ok := index[ref]
			if !ok {
				issues = append(issues, Issue{
					Path:    inputPath,
					Code:    CodeUnknownQueryRef,
					Message: fmt.Sprintf("expression references query ref %q, which the panel does not have", ref),
				})
				continue
			}
			edges[j] = append(edges[j], k)
		}
	}
	onCycle := map[int]bool{}
	for j := range p.Queries {
		if onCycle[j] || !reaches(edges, j, j, map[int]bool{}) {
			continue
		}
		for k := range p.Queries {
			if reaches(edges, j, k, map[int]bool{}) && reaches(edges, k, j, map[int]bool{}) {
				onCycle[k] = true
			}
		}
		issues = append(issues, Issue{
			Path:    e.Pointer + pointer("queries", strconv.Itoa(j), "expression"),
			Code:    CodeInvalidReference,
			Message: fmt.Sprintf("expression %q depends on itself through its inputs; break the cycle", p.QueryRef(j)),
		})
	}
	return issues
}

func reaches(edges map[int][]int, from, target int, seen map[int]bool) bool {
	for _, next := range edges[from] {
		if next == target {
			return true
		}
		if seen[next] {
			continue
		}
		seen[next] = true
		if reaches(edges, next, target, seen) {
			return true
		}
	}
	return false
}

func checkExpressionShape(x Expression, base string) []Issue {
	switch x.Type {
	case ExpressionReduce:
		switch {
		case x.Mode == ModeReplaceNonNumbers && x.ReplaceWith == nil:
			return []Issue{{Path: base, Code: CodeSchema, Message: "mode replaceNonNumbers needs replaceWith"}}
		case x.Mode != ModeReplaceNonNumbers && x.ReplaceWith != nil:
			return []Issue{{Path: base + "/replaceWith", Code: CodeSchema, Message: "replaceWith applies only with mode replaceNonNumbers; remove it or set that mode"}}
		}
	case ExpressionThreshold:
		if want := thresholdParams[x.Evaluator]; len(x.Params) != want {
			return []Issue{{Path: base + "/params", Code: CodeSchema, Message: fmt.Sprintf("evaluator %s takes %d number(s) in params, not %d", x.Evaluator, want, len(x.Params))}}
		}
	}
	return nil
}
