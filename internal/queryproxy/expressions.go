package queryproxy

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy/expr"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func queryIndex(p *spec.Panel) map[string]int {
	index := make(map[string]int, len(p.Queries))
	for j := range p.Queries {
		if _, taken := index[p.QueryRef(j)]; !taken {
			index[p.QueryRef(j)] = j
		}
	}
	return index
}

func propagateSkips(p *spec.Panel, skip []bool) []bool {
	out := make([]bool, len(p.Queries))
	copy(out, skip)
	index := queryIndex(p)
	for changed := true; changed; {
		changed = false
		for j, q := range p.Queries {
			if out[j] || !q.IsExpression() {
				continue
			}
			for _, ref := range q.Expression.Inputs() {
				if k, ok := index[ref]; ok && out[k] {
					out[j], changed = true, true
					break
				}
			}
		}
	}
	return out
}

func wantedQueries(req Request, p *spec.Panel) []bool {
	skip := propagateSkips(p, req.skip)
	index := queryIndex(p)
	wanted := make([]bool, len(p.Queries))
	var pending []int
	for j, q := range p.Queries {
		if skip[j] || (q.Hide && !req.IncludeHidden) {
			continue
		}
		wanted[j] = true
		pending = append(pending, j)
	}
	for len(pending) > 0 {
		j := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if !p.Queries[j].IsExpression() {
			continue
		}
		for _, ref := range p.Queries[j].Expression.Inputs() {
			if k, ok := index[ref]; ok && !wanted[k] && !skip[k] {
				wanted[k] = true
				pending = append(pending, k)
			}
		}
	}
	return wanted
}

func queryKinds(p *spec.Panel) []Kind {
	kinds := make([]Kind, len(p.Queries))
	for j, q := range p.Queries {
		kinds[j] = KindInstant
		if !q.IsExpression() {
			kinds[j] = QueryKind(p, q)
		}
	}
	index := queryIndex(p)
	for changed := true; changed; {
		changed = false
		for j, q := range p.Queries {
			if !q.IsExpression() || kinds[j] == KindRange {
				continue
			}
			for _, ref := range q.Expression.Inputs() {
				if k, ok := index[ref]; ok && kinds[k] == KindRange {
					kinds[j], changed = KindRange, true
					break
				}
			}
		}
	}
	return kinds
}

type evaluation struct {
	req     Request
	panel   *spec.Panel
	index   map[string]int
	kinds   []Kind
	results []QueryResult
	have    []bool
	errs    []error
}

func (ev *evaluation) settled(k int) bool { return ev.have[k] || ev.errs[k] != nil }

func (ev *evaluation) fail(j int, err error) {
	ev.errs[j] = &QueryError{Ref: ev.panel.QueryRef(j), Index: j, Err: err}
}

func (ev *evaluation) ready(j int) bool {
	for _, ref := range ev.panel.Queries[j].Expression.Inputs() {
		if k, ok := ev.index[ref]; ok && !ev.settled(k) {
			return false
		}
	}
	return true
}

func evalExpressions(req Request, panel *spec.Panel, wanted []bool, results []QueryResult, have []bool, errs []error) error {
	ev := &evaluation{req: req, panel: panel, index: queryIndex(panel), kinds: queryKinds(panel), results: results, have: have, errs: errs}
	var pending []int
	for j, q := range panel.Queries {
		if wanted[j] && q.IsExpression() {
			pending = append(pending, j)
		}
	}
	for len(pending) > 0 {
		var blocked []int
		for _, j := range pending {
			if !ev.ready(j) {
				blocked = append(blocked, j)
				continue
			}
			if err := ev.evaluate(j); err != nil {
				return err
			}
		}
		if len(blocked) == len(pending) {
			for _, j := range blocked {
				ev.fail(j, fmt.Errorf("%w: it depends on itself", expr.ErrExpression))
			}
			return nil
		}
		pending = blocked
	}
	return nil
}

func (ev *evaluation) evaluate(j int) error {
	q := ev.panel.Queries[j]
	inputs := map[string][]datasource.Frame{}
	var step time.Duration
	for _, ref := range q.Expression.Inputs() {
		k, ok := ev.index[ref]
		if !ok {
			ev.fail(j, fmt.Errorf("%w: input %q is not a query of this panel", expr.ErrExpression, ref))
			return nil
		}
		if inErr := ev.errs[k]; inErr != nil {
			if errors.Is(inErr, context.Canceled) || errors.Is(inErr, context.DeadlineExceeded) {
				ev.fail(j, inErr)
			} else {
				ev.fail(j, fmt.Errorf("%w: input %q failed", expr.ErrExpression, ref))
			}
			return nil
		}
		inputs[ref] = ev.results[k].Frames
		step = max(step, ev.results[k].Step)
	}
	frames, err := expr.Eval(*q.Expression, inputs, ev.req.Range, ev.req.Limits)
	if err != nil {
		ev.fail(j, err)
		return nil
	}
	res := QueryResult{Ref: ev.panel.QueryRef(j), Kind: ev.kinds[j], Step: step, Frames: frames}
	if ev.req.Admit != nil && !q.Hide {
		if err := ev.req.Admit(j, res); err != nil {
			var abort *AbortError
			if errors.As(err, &abort) {
				return abort.Err
			}
			ev.fail(j, err)
			return nil
		}
	}
	ev.results[j], ev.have[j] = res, true
	return nil
}
