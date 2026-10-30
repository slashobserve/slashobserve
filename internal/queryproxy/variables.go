package queryproxy

import (
	"context"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/macros"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	maxVariableResults     = 1000
	maxSelectionValues     = 100
	maxSelectionValueBytes = 1024
	maxAllValues           = 1000
	maxAllBytes            = 64 << 10
)

type allList struct {
	values   []string
	unlisted bool
}

type varState struct {
	lists  map[string]allList
	parsed *parseCache
}

type parseCache struct {
	mu      sync.Mutex
	filters map[string]parsedFilters
	regexes map[string]*spec.VariableRegex
	checked map[string]bool
}

type parsedFilters struct {
	raw     []string
	filters []datasource.AdhocFilter
}

func newVarState() *varState {
	return &varState{
		lists:  map[string]allList{},
		parsed: &parseCache{filters: map[string]parsedFilters{}, regexes: map[string]*spec.VariableRegex{}, checked: map[string]bool{}},
	}
}

func (s *varState) fork() *varState {
	return &varState{lists: maps.Clone(s.lists), parsed: s.parsed}
}

func (r *Runner) VariableValues(ctx context.Context, d *spec.Dashboard, name string, rng datasource.TimeRange, earlier map[string]Selection, user datasource.QueryUser) (options []spec.VariableOption, truncated bool, err error) {
	idx := variableIndex(d, name)
	if idx < 0 {
		return nil, false, fmt.Errorf("%w: %q", ErrUnknownVariable, name)
	}
	st := newVarState()
	selections, err := r.complete(ctx, d, idx, rng, earlier, user, st)
	if err != nil {
		return nil, false, err
	}
	options, err = r.resolveOptions(ctx, d, idx, rng, selections, user, st)
	if err != nil {
		return nil, false, err
	}
	if len(options) > maxVariableResults {
		return options[:maxVariableResults], true, nil
	}
	return options, false, nil
}

func (r *Runner) Defaults(ctx context.Context, d *spec.Dashboard, rng datasource.TimeRange, given map[string]Selection, user datasource.QueryUser) (map[string]Selection, error) {
	return r.complete(ctx, d, len(d.Variables), rng, given, user, newVarState())
}

func (r *Runner) complete(ctx context.Context, d *spec.Dashboard, upto int, rng datasource.TimeRange, given map[string]Selection, user datasource.QueryUser, st *varState) (map[string]Selection, error) {
	given = withoutEmptyAdhoc(d, given)
	for _, name := range slices.Sorted(maps.Keys(given)) {
		idx := variableIndex(d, name)
		if idx < 0 {
			return nil, fmt.Errorf("%w: %q", ErrUnknownVariable, name)
		}
		if idx >= upto {
			return nil, fmt.Errorf("%w: variable %q cannot be selected here", ErrInvalidRequest, name)
		}
		if v := d.Variables[idx]; !checkNeedsEarlier(v) {
			if err := r.checkSelection(ctx, d, idx, rng, given, user, st, given[name]); err != nil {
				return nil, err
			}
		}
	}
	out := maps.Clone(given)
	if out == nil {
		out = make(map[string]Selection, upto)
	}
	for i := range upto {
		v := d.Variables[i]
		if sel, ok := out[v.Name]; ok {
			if checkNeedsEarlier(v) {
				if err := r.checkSelection(ctx, d, i, rng, out, user, st, sel); err != nil {
					return nil, err
				}
			}
			continue
		}
		sel, err := r.defaultSelection(ctx, d, i, rng, out, user, st)
		if err != nil {
			return nil, err
		}
		if err := r.checkSelection(ctx, d, i, rng, out, user, st, sel); err != nil {
			return nil, err
		}
		out[v.Name] = sel
	}
	return out, nil
}

func withoutEmptyAdhoc(d *spec.Dashboard, given map[string]Selection) map[string]Selection {
	var out map[string]Selection
	for name, sel := range given {
		i := variableIndex(d, name)
		if i < 0 || d.Variables[i].Type != spec.AdhocVariable || sel.All || len(sel.Values) != 1 || sel.Values[0] != "" {
			continue
		}
		if out == nil {
			out = maps.Clone(given)
		}
		out[name] = Selection{}
	}
	if out == nil {
		return given
	}
	return out
}

func adhocDefault(v spec.Variable) Selection {
	if v.Default == nil {
		return Selection{}
	}
	values := make([]string, len(v.Default.Filters))
	for i, f := range v.Default.Filters {
		values[i] = f.Key + "|" + f.Operator + "|" + f.Value
	}
	return Selection{Values: values}
}

func checkNeedsEarlier(v spec.Variable) bool {
	return v.Type == spec.DatasourceVariable && len(scanQueryUse(v.Regex).variables) > 0
}

func (r *Runner) defaultSelection(ctx context.Context, d *spec.Dashboard, idx int, rng datasource.TimeRange, selections map[string]Selection, user datasource.QueryUser, st *varState) (Selection, error) {
	v := d.Variables[idx]
	if sel, ok := fixedDefault(v); ok {
		return sel, nil
	}
	values, err := r.resolveValues(ctx, d, idx, rng, selections, user, st)
	if err != nil {
		return Selection{}, err
	}
	if value := declaredValue(v); v.Type == spec.DatasourceVariable && value != "" {
		if !slices.Contains(values, value) {
			return Selection{}, fmt.Errorf("%w: %q", ErrVariableEmpty, v.Name)
		}
		return Selection{Values: []string{value}}, nil
	}
	if len(values) == 0 {
		return Selection{}, fmt.Errorf("%w: %q", ErrVariableEmpty, v.Name)
	}
	return Selection{Values: []string{values[0]}}, nil
}

func fixedDefault(v spec.Variable) (Selection, bool) {
	switch v.Type {
	case spec.ConstantVariable:
		return Selection{Values: []string{v.Value}}, true
	case spec.TextboxVariable:
		return Selection{Values: []string{declaredValue(v)}}, true
	case spec.AdhocVariable:
		return adhocDefault(v), true
	case spec.QueryVariable, spec.CustomVariable:
		if v.Default != nil && len(v.Default.Values) > 0 {
			if slices.Contains(v.Default.Values, spec.SelectAll) {
				return Selection{All: true}, true
			}
			return Selection{Values: slices.Clone(v.Default.Values)}, true
		}
		return Selection{All: true}, v.IncludeAll
	case spec.IntervalVariable:
		switch value := declaredValue(v); {
		case value != "":
			return Selection{Values: []string{value}}, true
		case v.Auto:
			return Selection{Values: []string{spec.SelectAuto}}, true
		case len(v.Options) > 0:
			return Selection{Values: []string{v.Options[0].Value}}, true
		}
	}
	return Selection{}, false
}

func declaredValue(v spec.Variable) string {
	if v.Default == nil || len(v.Default.Values) == 0 {
		return ""
	}
	return v.Default.Values[0]
}

func (r *Runner) resolveValues(ctx context.Context, d *spec.Dashboard, idx int, rng datasource.TimeRange, selections map[string]Selection, user datasource.QueryUser, st *varState) ([]string, error) {
	options, err := r.resolveOptions(ctx, d, idx, rng, selections, user, st)
	if err != nil {
		return nil, err
	}
	return spec.OptionValues(options), nil
}

func (r *Runner) resolveOptions(ctx context.Context, d *spec.Dashboard, idx int, rng datasource.TimeRange, selections map[string]Selection, user datasource.QueryUser, st *varState) ([]spec.VariableOption, error) {
	v := d.Variables[idx]
	switch v.Type {
	case spec.CustomVariable:
		return slices.Clone(v.Options), nil
	case spec.ConstantVariable:
		return spec.PlainOptions(v.Value), nil
	case spec.TextboxVariable:
		return []spec.VariableOption{}, nil
	case spec.IntervalVariable:
		if v.Auto {
			return append(spec.PlainOptions(spec.SelectAuto), v.Options...), nil
		}
		return slices.Clone(v.Options), nil
	case spec.DatasourceVariable:
		uids, err := r.datasourceValues(ctx, d, idx, rng, selections, user, st)
		return spec.PlainOptions(uids...), err
	case spec.AdhocVariable:
		return nil, fmt.Errorf("%w: adhoc variable %q has no value list", ErrInvalidRequest, v.Name)
	}
	return r.queryOptions(ctx, d, idx, rng, selections, user, st)
}

func (r *Runner) datasourceValues(ctx context.Context, d *spec.Dashboard, idx int, rng datasource.TimeRange, selections map[string]Selection, user datasource.QueryUser, st *varState) ([]string, error) {
	v := d.Variables[idx]
	uids := []string{}
	for _, inst := range r.reg.List() {
		if inst.Datasource.Type() == string(v.DatasourceType) {
			uids = append(uids, inst.UID)
		}
	}
	re, err := r.variableRegex(ctx, d, idx, rng, selections, user, st)
	if err != nil || re == nil {
		return uids, err
	}
	return slices.DeleteFunc(uids, func(uid string) bool { return !re.MatchString(uid) }), nil
}

func (r *Runner) variableRegex(ctx context.Context, d *spec.Dashboard, idx int, rng datasource.TimeRange, selections map[string]Selection, user datasource.QueryUser, st *varState) (*spec.VariableRegex, error) {
	v := d.Variables[idx]
	if v.Regex == "" {
		return nil, nil
	}
	var vars []datasource.Variable
	if use := scanQueryUse(v.Regex); len(use.variables) > 0 {
		use.listed = map[string]bool{}
		var err error
		if vars, err = r.variablesFor(ctx, d, idx, use, rng, selections, user, st); err != nil {
			return nil, err
		}
	}
	pattern, err := macros.ExpandRegex(v.Regex, vars)
	if err != nil {
		return nil, err
	}
	return st.parsed.regex(v.Name, pattern)
}

func (c *parseCache) regex(name, pattern string) (*spec.VariableRegex, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if re, ok := c.regexes[pattern]; ok {
		return re, nil
	}
	re, err := spec.CompileVariableRegex(pattern)
	if err != nil {
		return nil, fmt.Errorf("%w: variable %q has an invalid regex", ErrInvalidRequest, name)
	}
	c.regexes[pattern] = re
	return re, nil
}

func (r *Runner) queryOptions(ctx context.Context, d *spec.Dashboard, idx int, rng datasource.TimeRange, selections map[string]Selection, user datasource.QueryUser, st *varState) ([]spec.VariableOption, error) {
	v := d.Variables[idx]
	if v.Datasource == "" {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "variable %q has no datasource", v.Name)
	}
	re, err := r.variableRegex(ctx, d, idx, rng, selections, user, st)
	if err != nil {
		return nil, err
	}
	inst, err := r.selectedDatasource(ctx, d, v.Datasource, rng, selections, user, st)
	if err != nil {
		return nil, err
	}
	if err := rng.Validate(); err != nil {
		return nil, invalidRangeError(err)
	}
	if name := adhocReference(d, idx, v.Query); name != "" {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "a variable query cannot use adhoc variable %q", name)
	}
	vars, err := r.variablesFor(ctx, d, idx, scanQueryUse(v.Query), rng, selections, user, st)
	if err != nil {
		return nil, err
	}
	q := datasource.Query{Text: v.Query, Variables: vars, Dashboard: d.Title, User: user}
	fetch := func(ctx context.Context) ([]string, error) {
		return upstream(ctx, r, "variable:"+v.Name, inst, func(ctx context.Context) ([]string, error) {
			return inst.Datasource.VariableValues(ctx, q, rng)
		})
	}
	var values []string
	if r.values == nil {
		values, err = fetch(ctx)
	} else {
		values, err = r.values.get(ctx, valueKey(inst.UID, q, rng), fetch)
	}
	if err != nil {
		return nil, err
	}
	options := spec.PlainOptions(values...)
	if re != nil {
		options = spec.ApplyVariableRegex(re, values)
	}
	if v.Sort != "" {
		options = spec.SortVariableOptions(options, v.Sort)
	}
	return options, nil
}

func adhocReference(d *spec.Dashboard, upto int, text string) string {
	for _, name := range slices.Sorted(maps.Keys(scanQueryUse(text).variables)) {
		if i := variableIndex(d, name); i >= 0 && i < upto && d.Variables[i].Type == spec.AdhocVariable {
			return name
		}
	}
	return ""
}

func (r *Runner) allValues(ctx context.Context, d *spec.Dashboard, idx int, rng datasource.TimeRange, selections map[string]Selection, user datasource.QueryUser, st *varState) (allList, error) {
	name := d.Variables[idx].Name
	if list, ok := st.lists[name]; ok {
		return list, nil
	}
	values, err := r.resolveValues(ctx, d, idx, rng, selections, user, st)
	if err != nil {
		return allList{}, err
	}
	if len(values) == 0 {
		return allList{}, fmt.Errorf("%w: %q", ErrVariableEmpty, name)
	}
	total := 0
	for _, value := range values {
		total += len(value)
	}
	list := allList{values: values}
	if len(values) > maxAllValues || total > maxAllBytes || slices.ContainsFunc(values, func(value string) bool { return checkValue(name, value) != nil }) {
		list = allList{unlisted: true}
	}
	st.lists[name] = list
	return list, nil
}

func (r *Runner) variablesFor(ctx context.Context, d *spec.Dashboard, upto int, use queryUse, rng datasource.TimeRange, selections map[string]Selection, user datasource.QueryUser, st *varState) ([]datasource.Variable, error) {
	out := make([]datasource.Variable, 0, upto)
	for i, v := range d.Variables[:upto] {
		sel, ok := selections[v.Name]
		if !ok {
			if use.variables[v.Name] {
				return nil, fmt.Errorf("%w: no selection for variable %q", ErrInvalidRequest, v.Name)
			}
			continue
		}
		if err := r.checkSelection(ctx, d, i, rng, selections, user, st, sel); err != nil {
			return nil, err
		}
		if v.Type == spec.AdhocVariable && !use.variables[v.Name] {
			continue
		}
		variable, err := r.variableFor(ctx, d, i, rng, use, selections, user, st)
		if err != nil {
			return nil, err
		}
		out = append(out, variable)
	}
	return out, nil
}

func (r *Runner) variableFor(ctx context.Context, d *spec.Dashboard, idx int, rng datasource.TimeRange, use queryUse, selections map[string]Selection, user datasource.QueryUser, st *varState) (datasource.Variable, error) {
	v := d.Variables[idx]
	sel := selections[v.Name]
	variable := datasource.Variable{Name: v.Name, Values: slices.Clone(sel.Values), All: sel.All}
	switch {
	case v.Type == spec.AdhocVariable:
		filters, err := r.adhocFilters(st, d, v, sel)
		return datasource.Variable{Name: v.Name, Adhoc: true, Filters: filters}, err
	case v.Type == spec.IntervalVariable && slices.Contains(sel.Values, spec.SelectAuto):
		variable.Values = []string{spec.FormatInterval(autoInterval(rng, v))}
	case sel.All && v.AllValue != "":
		variable = datasource.Variable{Name: v.Name, Values: []string{v.AllValue}, All: true, AllValue: true}
	case sel.All && use.listed[v.Name]:
		list, err := r.allValues(ctx, d, idx, rng, selections, user, st)
		if err != nil {
			return datasource.Variable{}, err
		}
		if !list.unlisted {
			variable.Values = slices.Clone(list.values)
		}
	}
	return variable, nil
}

var autoLadder = []time.Duration{
	10 * time.Millisecond, 20 * time.Millisecond, 50 * time.Millisecond,
	100 * time.Millisecond, 200 * time.Millisecond, 500 * time.Millisecond,
	time.Second, 2 * time.Second, 5 * time.Second, 10 * time.Second, 15 * time.Second, 20 * time.Second, 30 * time.Second,
	time.Minute, 2 * time.Minute, 5 * time.Minute, 10 * time.Minute, 15 * time.Minute, 20 * time.Minute, 30 * time.Minute,
	time.Hour, 2 * time.Hour, 3 * time.Hour, 6 * time.Hour, 12 * time.Hour,
	24 * time.Hour, 7 * 24 * time.Hour, 30 * 24 * time.Hour, 365 * 24 * time.Hour,
}

const (
	defaultAutoSteps = 30
	defaultAutoFloor = 10 * time.Second
	day              = 24 * time.Hour
)

func autoInterval(rng datasource.TimeRange, v spec.Variable) time.Duration {
	floor := defaultAutoFloor
	if v.AutoMin != "" {
		if d, err := spec.ParseInterval(v.AutoMin); err == nil {
			floor = d
		}
	}
	steps := v.AutoSteps
	if steps <= 0 {
		steps = defaultAutoSteps
	}
	raw := rng.To.Sub(rng.From) / time.Duration(steps)
	rounded := (raw + day - 1) / day * day
	if i := slices.IndexFunc(autoLadder, func(step time.Duration) bool { return raw <= step }); i >= 0 {
		rounded = autoLadder[i]
	}
	return max(floor, rounded)
}

func (r *Runner) adhocFilters(st *varState, d *spec.Dashboard, v spec.Variable, sel Selection) ([]datasource.AdhocFilter, error) {
	if len(sel.Values) == 0 {
		return nil, nil
	}
	if filters, ok := st.parsed.cachedFilters(v.Name, sel.Values); ok {
		return filters, nil
	}
	typ, err := r.datasourceType(d, v.Datasource)
	if err != nil {
		return nil, err
	}
	filters := make([]datasource.AdhocFilter, len(sel.Values))
	for i, text := range sel.Values {
		if filters[i], err = parseAdhocFilter(v.Name, string(typ), text); err != nil {
			return nil, err
		}
	}
	st.parsed.storeFilters(v.Name, sel.Values, filters)
	return filters, nil
}

func (c *parseCache) cachedFilters(name string, raw []string) ([]datasource.AdhocFilter, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.filters[name]
	if !ok || !slices.Equal(entry.raw, raw) {
		return nil, false
	}
	return entry.filters, true
}

func (c *parseCache) storeFilters(name string, raw []string, filters []datasource.AdhocFilter) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.filters[name] = parsedFilters{raw: slices.Clone(raw), filters: filters}
}

func implicitScope(dsType string, use queryUse) func(spec.Variable) bool {
	switch dsType {
	case string(spec.Prometheus):
		return func(spec.Variable) bool { return true }
	case string(spec.ClickHouse):
		return func(v spec.Variable) bool { return v.ApplyToAll && !use.variables[v.Name] }
	}
	return nil
}

func (r *Runner) implicitFilters(st *varState, d *spec.Dashboard, selections map[string]Selection, uid string, applies func(spec.Variable) bool) ([]datasource.AdhocFilter, error) {
	var out []datasource.AdhocFilter
	for _, v := range d.Variables {
		sel := selections[v.Name]
		if v.Type != spec.AdhocVariable || len(sel.Values) == 0 || !applies(v) {
			continue
		}
		resolved, err := ResolveDatasourceUID(d, v.Datasource, selections)
		if err != nil {
			return nil, err
		}
		if resolved != uid {
			continue
		}
		filters, err := r.adhocFilters(st, d, v, sel)
		if err != nil {
			return nil, err
		}
		out = append(out, filters...)
	}
	return out, nil
}

func parseAdhocFilter(name, dsType, text string) (datasource.AdhocFilter, error) {
	parts := strings.SplitN(text, "|", 3)
	if len(parts) != 3 {
		return datasource.AdhocFilter{}, fmt.Errorf("%w: variable %q has a filter that is not key|operator|value", ErrInvalidRequest, name)
	}
	f := datasource.AdhocFilter{Key: parts[0], Op: parts[1], Value: parts[2]}
	switch f.Op {
	case "=", "!=", "=~", "!~":
	default:
		return datasource.AdhocFilter{}, fmt.Errorf("%w: variable %q has a filter with an operator other than =, !=, =~ or !~", ErrInvalidRequest, name)
	}
	validKey := spec.IsLabelName(f.Key)
	if dsType == string(spec.ClickHouse) {
		validKey = spec.IsColumnArg(f.Key)
	}
	if !validKey {
		return datasource.AdhocFilter{}, fmt.Errorf("%w: variable %q has a filter with an invalid key", ErrInvalidRequest, name)
	}
	if err := checkValue(name, f.Value); err != nil {
		return datasource.AdhocFilter{}, err
	}
	if f.Op == "=~" || f.Op == "!~" {
		if _, err := regexp.Compile(f.Value); err != nil {
			return datasource.AdhocFilter{}, fmt.Errorf("%w: variable %q has a filter with an invalid regular expression", ErrInvalidRequest, name)
		}
	}
	return f, nil
}

type queryUse struct {
	variables map[string]bool
	listed    map[string]bool
	user      bool
	dashboard bool
}

func scanQueryUse(text string) queryUse {
	refs, err := spec.ScanQuery(text)
	if err != nil {
		return queryUse{}
	}
	use := queryUse{variables: map[string]bool{}, listed: map[string]bool{}}
	for _, ref := range refs {
		use.add(ref, "")
	}
	return use
}

func (u *queryUse) add(ref spec.Reference, guard string) {
	nested := ref.Nested
	if name := ref.Guard; name != "" {
		u.variables[name] = true
		if len(nested) > 0 {
			nested = nested[:len(nested)-1]
		}
		guard = name
	}
	if ref.Kind == spec.VariableRef {
		u.variables[ref.Name] = true
		u.listed[ref.Name] = u.listed[ref.Name] || ref.Name != guard
	} else if macro, ok := spec.LookupMacro(ref.Name); ok {
		u.user = u.user || macro.PerUser
		u.dashboard = u.dashboard || macro.PerDashboard
	}
	for _, n := range nested {
		u.add(n, guard)
	}
}

func DashboardUsesUser(d *spec.Dashboard) bool {
	if d == nil {
		return false
	}
	if slices.ContainsFunc(d.Variables, func(v spec.Variable) bool { return v.Type == spec.QueryVariable && scanQueryUse(v.Query).user }) {
		return true
	}
	for _, e := range d.DataPanels() {
		if slices.ContainsFunc(e.Panel.Queries, func(q spec.Query) bool { return scanQueryUse(q.Query).user }) {
			return true
		}
	}
	return false
}

func variableIndex(d *spec.Dashboard, name string) int {
	if d == nil {
		return -1
	}
	return slices.IndexFunc(d.Variables, func(v spec.Variable) bool { return v.Name == name })
}

const maxAdhocFilters = 20

func (r *Runner) checkSelection(ctx context.Context, d *spec.Dashboard, idx int, rng datasource.TimeRange, selections map[string]Selection, user datasource.QueryUser, st *varState, sel Selection) error {
	// A varState belongs to one request and sees one fixed selection set, so a check that passed stays valid for it.
	key := selectionKey(d.Variables[idx].Name, sel)
	if st.parsed.isChecked(key) {
		return nil
	}
	if err := r.checkSelectionUncached(ctx, d, idx, rng, selections, user, st, sel); err != nil {
		return err
	}
	st.parsed.markChecked(key)
	return nil
}

func selectionKey(name string, sel Selection) string {
	var b strings.Builder
	b.WriteString(strconv.Quote(name))
	if sel.All {
		b.WriteString(" all")
	}
	for _, value := range sel.Values {
		b.WriteByte(' ')
		b.WriteString(strconv.Quote(value))
	}
	return b.String()
}

func (c *parseCache) isChecked(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.checked[key]
}

func (c *parseCache) markChecked(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.checked[key] = true
}

func (r *Runner) checkSelectionUncached(ctx context.Context, d *spec.Dashboard, idx int, rng datasource.TimeRange, selections map[string]Selection, user datasource.QueryUser, st *varState, sel Selection) error {
	v := d.Variables[idx]
	switch {
	case sel.All:
		return checkAll(v, sel)
	case v.Type == spec.AdhocVariable:
		if len(sel.Values) > maxAdhocFilters {
			return fmt.Errorf("%w: variable %q has more than %d filters", ErrInvalidRequest, v.Name, maxAdhocFilters)
		}
		_, err := r.adhocFilters(st, d, v, sel)
		return err
	case v.Type == spec.QueryVariable || v.Type == spec.CustomVariable:
		return checkListSelection(v, sel)
	}
	return r.checkSingleSelection(ctx, d, idx, rng, selections, user, st, sel)
}

func checkAll(v spec.Variable, sel Selection) error {
	if (v.Type != spec.QueryVariable && v.Type != spec.CustomVariable) || !v.IncludeAll {
		return fmt.Errorf("%w: variable %q does not allow All", ErrInvalidRequest, v.Name)
	}
	if len(sel.Values) > 0 {
		return fmt.Errorf("%w: variable %q cannot combine All with values", ErrInvalidRequest, v.Name)
	}
	return nil
}

func (r *Runner) checkSingleSelection(ctx context.Context, d *spec.Dashboard, idx int, rng datasource.TimeRange, selections map[string]Selection, user datasource.QueryUser, st *varState, sel Selection) error {
	v := d.Variables[idx]
	if len(sel.Values) != 1 {
		return fmt.Errorf("%w: variable %q takes exactly one value", ErrInvalidRequest, v.Name)
	}
	value := sel.Values[0]
	if v.Type == spec.ConstantVariable {
		if value != v.Value {
			return fmt.Errorf("%w: variable %q is a constant and accepts only its own value", ErrInvalidRequest, v.Name)
		}
		return nil
	}
	if err := checkValue(v.Name, value); err != nil {
		return err
	}
	switch v.Type {
	case spec.IntervalVariable:
		if !slices.Contains(spec.OptionValues(v.Options), value) && (value != spec.SelectAuto || !v.Auto) {
			return fmt.Errorf("%w: variable %q has a value that is not one of its options", ErrInvalidRequest, v.Name)
		}
	case spec.DatasourceVariable:
		values, err := r.datasourceValues(ctx, d, idx, rng, selections, user, st)
		if err != nil {
			return err
		}
		if !slices.Contains(values, value) {
			return fmt.Errorf("%w: variable %q has a value that is not one of its datasources", ErrInvalidRequest, v.Name)
		}
	}
	return nil
}

func checkListSelection(v spec.Variable, sel Selection) error {
	if len(sel.Values) == 0 {
		return fmt.Errorf("%w: variable %q has no value selected", ErrInvalidRequest, v.Name)
	}
	if len(sel.Values) > 1 && !v.Multi {
		return fmt.Errorf("%w: variable %q does not allow multiple values", ErrInvalidRequest, v.Name)
	}
	if len(sel.Values) > maxSelectionValues {
		return fmt.Errorf("%w: variable %q has more than %d values", ErrInvalidRequest, v.Name, maxSelectionValues)
	}
	for _, value := range sel.Values {
		if err := checkValue(v.Name, value); err != nil {
			return err
		}
		if v.Type == spec.CustomVariable && !v.AllowCustomValue && !slices.Contains(spec.OptionValues(v.Options), value) {
			return fmt.Errorf("%w: variable %q has a value that is not one of its options", ErrInvalidRequest, v.Name)
		}
	}
	return nil
}

func checkValue(name, value string) error {
	if len(value) > maxSelectionValueBytes {
		return fmt.Errorf("%w: variable %q has a value longer than %d bytes", ErrInvalidRequest, name, maxSelectionValueBytes)
	}
	for i := range len(value) {
		if value[i] < 0x20 || value[i] == 0x7f {
			return fmt.Errorf("%w: variable %q has a value with a control character", ErrInvalidRequest, name)
		}
	}
	return nil
}
