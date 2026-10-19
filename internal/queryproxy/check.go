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

func (r *Runner) CheckSpec(ctx context.Context, d *spec.Dashboard, now time.Time, user datasource.QueryUser) ([]spec.Issue, error) {
	canceled := func() bool { return errors.Is(ctx.Err(), context.Canceled) }
	if canceled() {
		return nil, context.Canceled
	}

	issues, badVariables, badQueries := r.checkDatasources(d)

	from, to, err := spec.ResolveTimeRange(d.Time, now)
	if err != nil {
		issues = append(issues, spec.Issue{Path: "/time", Code: CodeTimeRangeInvalid, Message: err.Error()})
		sortIssues(issues)
		return issues, nil
	}
	rng := datasource.TimeRange{From: from, To: to}

	budget, cancel := context.WithTimeout(ctx, checkSpecBudget)
	defer cancel()

	phase, cancelPhase := context.WithTimeout(budget, checkVariablePhase)
	lists := map[string]allList{}
	selections, failed, variableIssues := r.checkVariables(phase, d, rng, badVariables, user, lists, guardOnlyVariables(d))
	cancelPhase()
	issues = append(issues, variableIssues...)
	if canceled() {
		return nil, context.Canceled
	}

	entries := d.DataPanels()
	panelIssues := make([][]spec.Issue, len(entries))
	var group errgroup.Group
	group.SetLimit(checkPanelParallelism)
	for i, e := range entries {
		skip := slices.Clone(badQueries[e.Pointer])
		for j, q := range e.Panel.Queries {
			if skip[j] {
				continue
			}
			if name := failedReference(q.Query, failed); name != "" {
				skip[j] = true
				panelIssues[i] = append(panelIssues[i], issue(queryPath(e.Pointer, j), CodeVariableUnresolved, "variable %q could not be resolved", name))
			}
		}
		if !slices.Contains(skip, false) {
			continue
		}
		group.Go(func() error {
			panelIssues[i] = append(panelIssues[i], r.checkPanel(budget, d, e, rng, selections, user, maps.Clone(lists), skip)...)
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

func (r *Runner) checkDatasources(d *spec.Dashboard) (issues []spec.Issue, badVariables []bool, badQueries map[string][]bool) {
	badVariables = make([]bool, len(d.Variables))
	badQueries = map[string][]bool{}
	adhoc := map[string]bool{}
	for _, v := range d.Variables {
		if v.Type == spec.AdhocVariable {
			adhoc[v.Name] = true
		}
	}
	for i, v := range d.Variables {
		found := r.checkVariableDatasource(i, v)
		badVariables[i] = len(found) > 0
		issues = append(issues, found...)
	}
	for _, e := range d.DataPanels() {
		bad := make([]bool, len(e.Panel.Queries))
		panelDatasourceReported := false
		for j, q := range e.Panel.Queries {
			base := e.Pointer + "/datasource"
			if q.Datasource != "" {
				base = e.Pointer + "/queries/" + strconv.Itoa(j) + "/datasource"
			}
			found := r.checkRef(base, e.Panel.QueryDatasource(j), queryPath(e.Pointer, j), q.Query, adhoc)
			bad[j] = len(found) > 0
			if q.Datasource == "" && len(found) == 1 && found[0].Path == base {
				if panelDatasourceReported {
					continue
				}
				panelDatasourceReported = true
			}
			issues = append(issues, found...)
		}
		badQueries[e.Pointer] = bad
	}
	return issues, badVariables, badQueries
}

func (r *Runner) checkVariableDatasource(i int, v spec.Variable) []spec.Issue {
	path := itemPath("variables", i)
	switch v.Type {
	case spec.QueryVariable:
		if v.Datasource != "" {
			return r.checkRef(path+"/datasource", v.Datasource, path+"/query", v.Query, nil)
		}
	case spec.AdhocVariable:
		if _, err := r.instance(v.Datasource); err != nil {
			return []spec.Issue{issue(path+"/datasource", CodeUnknownDatasource, "datasource %q is not configured", v.Datasource)}
		}
	case spec.DatasourceVariable:
		return r.checkDatasourceDefault(path+"/default", v)
	}
	return nil
}

func (r *Runner) checkDatasourceDefault(path string, v spec.Variable) []spec.Issue {
	uid := declaredValue(v)
	if uid == "" {
		return nil
	}
	inst, err := r.instance(uid)
	switch {
	case err != nil:
		return []spec.Issue{issue(path, CodeUnknownDatasource, "datasource %q is not configured", uid)}
	case inst.Datasource.Type() != string(v.DatasourceType):
		return []spec.Issue{issue(path, spec.CodeInvalidDefault, "default %q of variable %q is not a %s datasource", uid, v.Name, v.DatasourceType)}
	}
	return nil
}

func (r *Runner) checkRef(path, uid, queryAt, query string, adhoc map[string]bool) []spec.Issue {
	inst, err := r.instance(uid)
	if errors.Is(err, ErrUnknownDatasource) {
		return []spec.Issue{issue(path, CodeUnknownDatasource, "datasource %q is not configured", uid)}
	}
	refs, err := spec.ScanQuery(query)
	if err != nil {
		return nil
	}
	dsType := inst.Datasource.Type()
	available := spec.Macros(spec.DatasourceType(dsType))
	var issues []spec.Issue
	reported := map[string]bool{}
	for _, ref := range spec.FlattenReferences(refs) {
		if ref.Kind == spec.VariableRef && adhoc[ref.Name] && dsType == string(spec.Prometheus) && !reported["$"+ref.Name] {
			reported["$"+ref.Name] = true
			issues = append(issues, issue(queryAt, spec.CodeInvalidReference, "adhoc variable %q applies to every query on its datasource and cannot be referenced", ref.Name))
		}
		if ref.Kind != spec.MacroRef || reported[ref.Name] {
			continue
		}
		if slices.ContainsFunc(available, func(m spec.Macro) bool { return m.Name == ref.Name }) {
			continue
		}
		reported[ref.Name] = true
		issues = append(issues, issue(queryAt, spec.CodeUnknownMacro, "macro $%s is not available for %s queries", ref.Name, dsType))
	}
	return issues
}

func (r *Runner) checkVariables(ctx context.Context, d *spec.Dashboard, rng datasource.TimeRange, bad []bool, user datasource.QueryUser, lists map[string]allList, guardOnly map[string]bool) (selections map[string]Selection, failed map[string]bool, issues []spec.Issue) {
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
		sel, err := r.defaultSelection(vctx, d, i, rng, selections, user, lists)
		listReported := false
		if err == nil && sel.All && v.AllValue == "" {
			if _, listErr := r.allValues(vctx, d, i, rng, selections, user, lists); listErr != nil {
				if guardOnly[v.Name] {
					issues = append(issues, variableIssue(path, listErr))
					listReported = true
				} else {
					err = listErr
				}
			}
		}
		if err == nil && !listReported {
			err = r.checkSelection(v, sel)
		}
		cancel()
		if err != nil {
			failed[v.Name] = true
			issues = append(issues, variableIssue(path, err))
			continue
		}
		selections[v.Name] = sel
	}
	return selections, failed, issues
}

func variableIssue(path string, err error) spec.Issue {
	_, code, message := Classify(err)
	if !errors.Is(err, ErrVariableEmpty) {
		code = string(CodeVariableFailed)
	}
	return issue(path, spec.IssueCode(code), "%s", message)
}

func (r *Runner) checkPanel(ctx context.Context, d *spec.Dashboard, e spec.PanelEntry, rng datasource.TimeRange, selections map[string]Selection, user datasource.QueryUser, lists map[string]allList, skip []bool) []spec.Issue {
	ctx, cancel := context.WithTimeout(ctx, checkPanelDeadline(skip))
	defer cancel()
	maxDataPoints := checkMaxDataPoints
	if e.Panel.MaxDataPoints > 0 {
		maxDataPoints = min(maxDataPoints, e.Panel.MaxDataPoints)
	}
	_, res, errs, err := r.runQueries(ctx, Request{
		Dashboard:     d,
		PanelID:       e.Panel.ID,
		Range:         rng,
		Selections:    selections,
		MaxDataPoints: maxDataPoints,
		Limits:        datasource.Limits{Timeout: checkPanelBudget},
		IncludeHidden: true,
		User:          user,
		skip:          skip,
	}, lists, false)
	if err != nil {
		_, code, message := Classify(err)
		return []spec.Issue{issue(queryPath(e.Pointer, 0), spec.IssueCode(code), "%s", message)}
	}
	var issues []spec.Issue
	for j, qerr := range errs {
		if qerr == nil {
			continue
		}
		_, code, message := Classify(qerr)
		issues = append(issues, issue(queryPath(e.Pointer, j), spec.IssueCode(code), "%s", message))
	}
	if len(issues) > 0 {
		return issues
	}
	if len(res.Queries) > 0 && !skippedVisible(e.Panel, skip) && !slices.ContainsFunc(res.Queries, func(q QueryResult) bool { return !framesEmpty(q.Frames) }) {
		return []spec.Issue{issue(e.Pointer+"/queries", CodeQueryEmpty, "%s", emptyPanelMessage)}
	}
	return nil
}

func checkPanelDeadline(skip []bool) time.Duration {
	running := 0
	for _, skipped := range skip {
		if !skipped {
			running++
		}
	}
	waves := max(1, (running+queryParallelism-1)/queryParallelism)
	return min(checkPanelBudget*time.Duration(waves), checkSpecBudget)
}

func skippedVisible(p *spec.Panel, skip []bool) bool {
	for j, q := range p.Queries {
		if !q.Hide && j < len(skip) && skip[j] {
			return true
		}
	}
	return false
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

func guardOnlyVariables(d *spec.Dashboard) map[string]bool {
	used, listed := map[string]bool{}, map[string]bool{}
	note := func(text string) {
		use := scanQueryUse(text)
		maps.Copy(used, use.variables)
		for name, ok := range use.listed {
			listed[name] = listed[name] || ok
		}
	}
	for _, v := range d.Variables {
		note(v.Query)
	}
	for _, e := range d.DataPanels() {
		for _, q := range e.Panel.Queries {
			note(q.Query)
		}
	}
	out := map[string]bool{}
	for name := range used {
		out[name] = !listed[name]
	}
	return out
}

func failedReference(text string, failed map[string]bool) string {
	if len(failed) == 0 || text == "" {
		return ""
	}
	refs, err := spec.ScanQuery(text)
	if err != nil {
		return ""
	}
	for _, ref := range spec.FlattenReferences(refs) {
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

func queryPath(pointer string, i int) string {
	return pointer + "/queries/" + strconv.Itoa(i) + "/query"
}

func issue(path string, code spec.IssueCode, format string, args ...any) spec.Issue {
	return spec.Issue{Path: path, Code: code, Message: fmt.Sprintf(format, args...)}
}
