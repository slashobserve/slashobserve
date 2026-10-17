package queryproxy

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	checkPanelParallelism = 4
	checkPanelBudget      = 15 * time.Second
	checkVariableBudget   = 10 * time.Second
	checkVariablePhase    = 20 * time.Second
	checkSpecBudget       = 45 * time.Second
	checkMaxDataPoints    = 200
	emptyPanelMessage     = "query returned no data for the dashboard's default time range and variable values"
)

func (r *Runner) CheckDatasources(d *spec.Dashboard) []spec.Issue {
	issues, _, _ := r.checkDatasources(d)
	sortIssues(issues)
	return issues
}

func (r *Runner) CheckSpec(ctx context.Context, d *spec.Dashboard, now time.Time) ([]spec.Issue, error) {
	canceled := func() bool { return errors.Is(ctx.Err(), context.Canceled) }
	if canceled() {
		return nil, context.Canceled
	}

	issues, badVariables, badPanels := r.checkDatasources(d)

	from, to, err := spec.ResolveTimeRange(d.TimeRange, now)
	if err != nil {
		issues = append(issues, spec.Issue{Path: "/timeRange", Code: CodeTimeRangeInvalid, Message: err.Error()})
		sortIssues(issues)
		return issues, nil
	}
	rng := datasource.TimeRange{From: from, To: to}

	budget, cancel := context.WithTimeout(ctx, checkSpecBudget)
	defer cancel()

	phase, cancelPhase := context.WithTimeout(budget, checkVariablePhase)
	lists := map[string]allList{}
	selections, failed, variableIssues := r.checkVariables(phase, d, rng, badVariables, lists)
	cancelPhase()
	issues = append(issues, variableIssues...)
	if canceled() {
		return nil, context.Canceled
	}

	panelIssues := make([][]spec.Issue, len(d.Panels))
	var group errgroup.Group
	group.SetLimit(checkPanelParallelism)
	for i, p := range d.Panels {
		if badPanels[i] {
			continue
		}
		if name := failedReference(p.Query, failed); name != "" {
			panelIssues[i] = []spec.Issue{issue(queryPath("panels", i), CodeVariableUnresolved, "variable %q could not be resolved", name)}
			continue
		}
		group.Go(func() error {
			panelIssues[i] = r.checkPanel(budget, d, i, rng, selections, maps.Clone(lists))
			return nil
		})
	}
	_ = group.Wait()
	if canceled() {
		return nil, context.Canceled
	}
	for _, pi := range panelIssues {
		issues = append(issues, pi...)
	}

	if len(issues) == 0 {
		return nil, nil
	}
	sortIssues(issues)
	return issues, nil
}

func (r *Runner) checkDatasources(d *spec.Dashboard) (issues []spec.Issue, badVariables, badPanels []bool) {
	badVariables = make([]bool, len(d.Variables))
	badPanels = make([]bool, len(d.Panels))
	for i, v := range d.Variables {
		if v.Type != spec.QueryVariable || v.Datasource == nil {
			continue
		}
		found := r.checkRef(itemPath("variables", i)+"/datasource", *v.Datasource)
		badVariables[i] = len(found) > 0
		issues = append(issues, found...)
	}
	for i, p := range d.Panels {
		found := r.checkRef(itemPath("panels", i)+"/datasource", p.Datasource)
		badPanels[i] = len(found) > 0
		issues = append(issues, found...)
	}
	return issues, badVariables, badPanels
}

func (r *Runner) checkRef(base string, ref spec.DatasourceRef) []spec.Issue {
	_, err := r.instance(ref)
	switch {
	case errors.Is(err, ErrUnknownDatasource):
		return []spec.Issue{issue(base+"/uid", CodeUnknownDatasource, "datasource %q is not configured", ref.UID)}
	case errors.Is(err, ErrDatasourceTypeMismatch):
		return []spec.Issue{issue(base+"/type", CodeDatasourceMismatch, "%s", err.Error())}
	}
	return nil
}

func (r *Runner) checkVariables(ctx context.Context, d *spec.Dashboard, rng datasource.TimeRange, bad []bool, lists map[string]allList) (selections map[string]Selection, failed map[string]bool, issues []spec.Issue) {
	selections = make(map[string]Selection, len(d.Variables))
	failed = map[string]bool{}
	for i, v := range d.Variables {
		if bad[i] {
			failed[v.Name] = true
			continue
		}
		path := itemPath("variables", i)
		if name := failedReference(v.Query, failed); name != "" {
			failed[v.Name] = true
			issues = append(issues, issue(path, CodeVariableUnresolved, "variable %q could not be resolved", name))
			continue
		}
		vctx, cancel := context.WithTimeout(ctx, checkVariableBudget)
		sel, err := r.defaultSelection(vctx, d, i, rng, selections, lists)
		if err == nil && sel.All {
			_, err = r.allValues(vctx, d, i, rng, selections, lists)
		}
		if err == nil {
			err = checkSelection(v, sel)
		}
		cancel()
		if err != nil {
			failed[v.Name] = true
			_, code, message := Classify(err)
			if !errors.Is(err, ErrVariableEmpty) {
				code = string(CodeVariableFailed)
			}
			issues = append(issues, issue(path, spec.IssueCode(code), "%s", message))
			continue
		}
		selections[v.Name] = sel
	}
	return selections, failed, issues
}

func (r *Runner) checkPanel(ctx context.Context, d *spec.Dashboard, idx int, rng datasource.TimeRange, selections map[string]Selection, lists map[string]allList) []spec.Issue {
	path := queryPath("panels", idx)
	ctx, cancel := context.WithTimeout(ctx, checkPanelBudget)
	defer cancel()
	res, err := r.runPanel(ctx, Request{
		Dashboard:     d,
		PanelID:       d.Panels[idx].ID,
		Range:         rng,
		Selections:    selections,
		MaxDataPoints: checkMaxDataPoints,
		Limits:        datasource.Limits{Timeout: checkPanelBudget},
	}, lists, false)
	if err != nil {
		_, code, message := Classify(err)
		return []spec.Issue{issue(path, spec.IssueCode(code), "%s", message)}
	}
	if framesEmpty(res.Frames) {
		return []spec.Issue{issue(path, CodeQueryEmpty, "%s", emptyPanelMessage)}
	}
	return nil
}

func framesEmpty(frames []datasource.Frame) bool {
	for _, f := range frames {
		for _, field := range f.Fields {
			if field.Type == datasource.FieldTime {
				continue
			}
			if slices.ContainsFunc(field.Values, func(v any) bool { return v != nil }) {
				return false
			}
		}
	}
	return true
}

func failedReference(text string, failed map[string]bool) string {
	if len(failed) == 0 || text == "" {
		return ""
	}
	refs, err := spec.ScanQuery(text)
	if err != nil {
		return ""
	}
	for _, ref := range refs {
		if ref.Kind == spec.VariableRef && failed[ref.Name] {
			return ref.Name
		}
	}
	return ""
}

func sortIssues(issues []spec.Issue) {
	slices.SortStableFunc(issues, func(a, b spec.Issue) int { return strings.Compare(a.Path, b.Path) })
}

func itemPath(collection string, i int) string {
	return "/" + collection + "/" + strconv.Itoa(i)
}

func queryPath(collection string, i int) string {
	return itemPath(collection, i) + "/query"
}

func issue(path string, code spec.IssueCode, format string, args ...any) spec.Issue {
	return spec.Issue{Path: path, Code: code, Message: fmt.Sprintf(format, args...)}
}
