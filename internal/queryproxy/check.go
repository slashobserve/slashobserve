package queryproxy

import (
	"cmp"
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
	return r.checkSpec(ctx, d, now, user, nil)
}

func (r *Runner) CheckPanels(ctx context.Context, d *spec.Dashboard, now time.Time, user datasource.QueryUser, ids []string) ([]spec.Issue, error) {
	only := map[string]bool{}
	for _, e := range d.DataPanels() {
		if slices.Contains(ids, e.Panel.ID) {
			only[e.Pointer] = true
		}
	}
	if len(only) == 0 {
		return nil, nil
	}
	return r.checkSpec(ctx, d, now, user, only)
}

func (r *Runner) checkSpec(ctx context.Context, d *spec.Dashboard, now time.Time, user datasource.QueryUser, only map[string]bool) ([]spec.Issue, error) {
	canceled := func() bool { return errors.Is(ctx.Err(), context.Canceled) }
	if canceled() {
		return nil, context.Canceled
	}

	issues, badVariables, badQueries := r.checkDatasources(d)
	finish := func(issues []spec.Issue) []spec.Issue {
		if only != nil {
			issues = slices.DeleteFunc(issues, func(i spec.Issue) bool { return !issueInPanels(i, only) })
		}
		if len(issues) == 0 {
			return nil
		}
		sortIssues(issues)
		return issues
	}

	opts, err := spec.TimeOptionsFor(d, "")
	if err != nil {
		issues = append(issues, spec.Issue{Path: "/timezone", Code: spec.CodeUnknownTimezone, Message: err.Error()})
		return finish(issues), nil
	}
	from, to, err := spec.ResolveTimeRange(d.Time, now, opts)
	if err != nil {
		issues = append(issues, spec.Issue{Path: "/time", Code: CodeTimeRangeInvalid, Message: err.Error()})
		return finish(issues), nil
	}
	rng := datasource.TimeRange{From: from, To: to}

	budget, cancel := context.WithTimeout(ctx, checkSpecBudget)
	defer cancel()

	phase, cancelPhase := context.WithTimeout(budget, checkVariablePhase)
	st := newVarState()
	selections, failed, variableIssues := r.checkVariables(phase, d, rng, badVariables, user, st, guardOnlyVariables(d))
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
		if only != nil && !only[e.Pointer] {
			continue
		}
		skip := slices.Clone(badQueries[e.Pointer])
		for j, q := range e.Panel.Queries {
			if skip[j] {
				continue
			}
			if name := cmp.Or(failedReference(q.Query, failed), failedDatasource(e.Panel.QueryDatasource(j), failed)); name != "" {
				skip[j] = true
				panelIssues[i] = append(panelIssues[i], issue(queryPath(e.Pointer, j), CodeVariableUnresolved, "variable %q could not be resolved", name))
			}
		}
		if !slices.Contains(skip, false) {
			continue
		}
		group.Go(func() error {
			panelIssues[i] = append(panelIssues[i], r.checkPanel(budget, d, e, PanelRange(e.Panel, rng, now, opts), selections, user, st.fork(), skip)...)
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

	return finish(issues), nil
}

func issueInPanels(i spec.Issue, only map[string]bool) bool {
	if i.Path == "/time" {
		return true
	}
	for pointer := range only {
		if i.Path == pointer || strings.HasPrefix(i.Path, pointer+"/") {
			return true
		}
	}
	return false
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
		found := r.checkVariableDatasource(d, i, v)
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
			found := r.checkRef(d, base, e.Panel.QueryDatasource(j), queryPath(e.Pointer, j), q.Query, adhoc)
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

func (r *Runner) checkVariableDatasource(d *spec.Dashboard, i int, v spec.Variable) []spec.Issue {
	path := itemPath("variables", i)
	switch v.Type {
	case spec.QueryVariable:
		if v.Datasource != "" {
			return r.checkRef(d, path+"/datasource", v.Datasource, path+"/query", v.Query, nil)
		}
	case spec.AdhocVariable:
		if _, referenced := spec.DatasourceVariableName(v.Datasource); referenced {
			return nil
		}
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
	case v.Regex != "" && regexIsStatic(v.Regex):
		if re, err := spec.CompileVariableRegex(v.Regex); err == nil && !re.MatchString(uid) {
			return []spec.Issue{issue(path, spec.CodeInvalidDefault, "default %q of variable %q does not match its regex", uid, v.Name)}
		}
	}
	return nil
}

func regexIsStatic(regex string) bool {
	refs, err := spec.ScanQuery(regex)
	return err == nil && !slices.ContainsFunc(refs, func(ref spec.Reference) bool { return ref.Kind == spec.VariableRef })
}

func (r *Runner) checkRef(d *spec.Dashboard, path, ref, queryAt, query string, adhoc map[string]bool) []spec.Issue {
	typ, err := r.datasourceType(d, ref)
	if errors.Is(err, ErrUnknownDatasource) {
		return []spec.Issue{issue(path, CodeUnknownDatasource, "datasource %q is not configured", ref)}
	}
	refs, err := spec.ScanQuery(query)
	if err != nil {
		return nil
	}
	dsType := string(typ)
	available := spec.Macros(typ)
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

func (r *Runner) checkVariables(ctx context.Context, d *spec.Dashboard, rng datasource.TimeRange, bad []bool, user datasource.QueryUser, st *varState, guardOnly map[string]bool) (selections map[string]Selection, failed map[string]bool, issues []spec.Issue) {
	selections = make(map[string]Selection, len(d.Variables))
	failed = map[string]bool{}
	for i, v := range d.Variables {
		if bad[i] {
			failed[v.Name] = true
			continue
		}
		path := itemPath("variables", i)
		if name := cmp.Or(failedReference(v.Query, failed), failedReference(v.Regex, failed), failedDatasource(v.Datasource, failed)); name != "" {
			failed[v.Name] = true
			issues = append(issues, issue(path, CodeVariableUnresolved, "variable %q could not be resolved", name))
			continue
		}
		vctx, cancel := context.WithTimeout(ctx, checkVariableBudget)
		sel, err := r.defaultSelection(vctx, d, i, rng, selections, user, st)
		listReported := false
		if err == nil && sel.All {
			var listErr error
			if v.AllValue == "" {
				_, listErr = r.allValues(vctx, d, i, rng, selections, user, st)
			} else {
				_, listErr = r.resolveValues(vctx, d, i, rng, selections, user, st)
			}
			if listErr != nil {
				if guardOnly[v.Name] {
					issues = append(issues, variableIssue(path, listErr))
					listReported = true
				} else {
					err = listErr
				}
			}
		}
		if err == nil && !listReported {
			err = r.checkSelection(vctx, d, i, rng, selections, user, st, sel)
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

func (r *Runner) checkPanel(ctx context.Context, d *spec.Dashboard, e spec.PanelEntry, rng datasource.TimeRange, selections map[string]Selection, user datasource.QueryUser, st *varState, skip []bool) []spec.Issue {
	if e.Panel.TimeFrom != "" && rng.To.Sub(rng.From) > MaxRange {
		return []spec.Issue{issue(e.Pointer+"/timeFrom", CodeTimeRangeInvalid, "the time range must be at most 400 days")}
	}
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
	}, st, false)
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
	if len(res.Queries) > 0 && !emptyResultIsValid(e.Panel) && !skippedVisible(e.Panel, skip) && !slices.ContainsFunc(res.Queries, func(q QueryResult) bool { return !framesEmpty(q.Frames) }) {
		return []spec.Issue{issue(e.Pointer+"/queries", CodeQueryEmpty, "%s", emptyPanelMessage)}
	}
	return nil
}

func emptyResultIsValid(p *spec.Panel) bool {
	return p.Type == spec.AlertList
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

func failedDatasource(ref string, failed map[string]bool) string {
	if name, ok := spec.DatasourceVariableName(ref); ok && failed[name] {
		return name
	}
	return ""
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
