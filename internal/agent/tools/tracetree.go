package tools

import (
	"cmp"
	"slices"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

type requestOrigin struct {
	span   *traceSpan
	depth  int
	relays []*traceSpan
}

func errorOriginIDs(failed []*traceSpan, byID map[string]*traceSpan) map[string]bool {
	hasErrorBelow := ancestorsOf(failed, byID)
	origins := make(map[string]bool, len(failed))
	for _, s := range failed {
		if !hasErrorBelow[s.id] {
			origins[s.id] = true
		}
	}
	return origins
}

func originOf(spans []traceSpan) (requestOrigin, bool) {
	byID := make(map[string]*traceSpan, len(spans))
	var failed []*traceSpan
	for i := range spans {
		byID[spans[i].id] = &spans[i]
		if datasource.IsErrorStatus(spans[i].status) {
			failed = append(failed, &spans[i])
		}
	}
	origins := errorOriginIDs(failed, byID)
	var best requestOrigin
	for _, s := range failed {
		if !origins[s.id] {
			continue
		}
		depth := depthOf(*s, byID)
		if best.span == nil || depth > best.depth || depth == best.depth && cmp.Or(s.start.Compare(best.span.start), cmp.Compare(s.id, best.span.id)) < 0 {
			best = requestOrigin{span: s, depth: depth}
		}
	}
	if best.span == nil {
		return requestOrigin{}, false
	}
	seen := map[string]bool{best.span.id: true}
	for parent := best.span.parent; parent != "" && !seen[parent]; {
		seen[parent] = true
		p, ok := byID[parent]
		if !ok {
			break
		}
		if datasource.IsErrorStatus(p.status) {
			best.relays = append(best.relays, p)
		}
		parent = p.parent
	}
	return best, true
}

func selfTimes(spans []traceSpan) map[string]float64 {
	children := childrenOf(spans)
	out := make(map[string]float64, len(spans))
	for _, s := range spans {
		out[s.id] = selfTime(s, children[s.id])
	}
	return out
}

func extentOf(spans []traceSpan) float64 {
	if len(spans) == 0 {
		return 0
	}
	start, end := spans[0].start, spans[0].start
	for _, s := range spans {
		start = minTime(start, s.start)
		end = maxTime(end, s.start.Add(time.Duration(s.duration)))
	}
	return float64(end.Sub(start))
}

func ancestorsOf(failed []*traceSpan, byID map[string]*traceSpan) map[string]bool {
	hasErrorBelow := map[string]bool{}
	for _, s := range failed {
		seen := map[string]bool{s.id: true}
		for parent := s.parent; parent != "" && !seen[parent]; {
			seen[parent] = true
			p, ok := byID[parent]
			if !ok {
				break
			}
			hasErrorBelow[p.id] = true
			parent = p.parent
		}
	}
	return hasErrorBelow
}

func childrenOf(spans []traceSpan) map[string][]traceSpan {
	children := map[string][]traceSpan{}
	for _, s := range spans {
		if s.parent != "" && s.parent != s.id {
			children[s.parent] = append(children[s.parent], s)
		}
	}
	return children
}

func selfTime(s traceSpan, children []traceSpan) float64 {
	end := s.start.Add(time.Duration(s.duration))
	covered := time.Duration(0)
	var cursor time.Time
	for _, c := range sortedByStart(children) {
		from, to := maxTime(c.start, s.start), minTime(c.start.Add(time.Duration(c.duration)), end)
		from = maxTime(from, cursor)
		if to.After(from) {
			covered += to.Sub(from)
			cursor = to
		}
	}
	return max(s.duration-float64(covered), 0)
}

func sortedByStart(spans []traceSpan) []traceSpan {
	sorted := slices.Clone(spans)
	slices.SortStableFunc(sorted, func(a, b traceSpan) int { return a.start.Compare(b.start) })
	return sorted
}

func depthOf(s traceSpan, byID map[string]*traceSpan) int {
	depth := 0
	seen := map[string]bool{s.id: true}
	for parent := s.parent; parent != "" && !seen[parent]; depth++ {
		seen[parent] = true
		p, ok := byID[parent]
		if !ok {
			return depth + 1
		}
		parent = p.parent
	}
	return depth
}
