package queryproxy

import (
	"context"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
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

func (r *Runner) VariableValues(ctx context.Context, d *spec.Dashboard, name string, rng datasource.TimeRange, earlier map[string]Selection, user datasource.QueryUser) (values []string, truncated bool, err error) {
	idx := variableIndex(d, name)
	if idx < 0 {
		return nil, false, fmt.Errorf("%w: %q", ErrUnknownVariable, name)
	}
	lists := map[string]allList{}
	selections, err := r.complete(ctx, d, idx, rng, earlier, user, lists)
	if err != nil {
		return nil, false, err
	}
	values, err = r.resolveValues(ctx, d, idx, rng, selections, user, lists)
	if err != nil {
		return nil, false, err
	}
	if len(values) > maxVariableResults {
		return values[:maxVariableResults], true, nil
	}
	return values, false, nil
}

func (r *Runner) Defaults(ctx context.Context, d *spec.Dashboard, rng datasource.TimeRange, given map[string]Selection, user datasource.QueryUser) (map[string]Selection, error) {
	return r.complete(ctx, d, len(d.Variables), rng, given, user, map[string]allList{})
}

func (r *Runner) complete(ctx context.Context, d *spec.Dashboard, upto int, rng datasource.TimeRange, given map[string]Selection, user datasource.QueryUser, lists map[string]allList) (map[string]Selection, error) {
	for _, name := range slices.Sorted(maps.Keys(given)) {
		idx := variableIndex(d, name)
		if idx < 0 {
			return nil, fmt.Errorf("%w: %q", ErrUnknownVariable, name)
		}
		if idx >= upto {
			return nil, fmt.Errorf("%w: variable %q cannot be selected here", ErrInvalidRequest, name)
		}
		if err := r.checkSelection(d.Variables[idx], given[name]); err != nil {
			return nil, err
		}
	}
	out := maps.Clone(given)
	if out == nil {
		out = make(map[string]Selection, upto)
	}
	for i := range upto {
		v := d.Variables[i]
		if _, ok := out[v.Name]; ok {
			continue
		}
		sel, err := r.defaultSelection(ctx, d, i, rng, out, user, lists)
		if err != nil {
			return nil, err
		}
		if err := r.checkSelection(v, sel); err != nil {
			return nil, err
		}
		out[v.Name] = sel
	}
	return out, nil
}

func (r *Runner) defaultSelection(ctx context.Context, d *spec.Dashboard, idx int, rng datasource.TimeRange, selections map[string]Selection, user datasource.QueryUser, lists map[string]allList) (Selection, error) {
	v := d.Variables[idx]
	if sel, ok := fixedDefault(v); ok {
		return sel, nil
	}
	values, err := r.resolveValues(ctx, d, idx, rng, selections, user, lists)
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
		return Selection{}, true
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
		case len(v.Options) > 0:
			return Selection{Values: []string{v.Options[0]}}, true
		case v.Auto:
			return Selection{Values: []string{spec.SelectAuto}}, true
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

func (r *Runner) resolveValues(ctx context.Context, d *spec.Dashboard, idx int, rng datasource.TimeRange, selections map[string]Selection, user datasource.QueryUser, lists map[string]allList) ([]string, error) {
	v := d.Variables[idx]
	switch v.Type {
	case spec.CustomVariable:
		return slices.Clone(v.Options), nil
	case spec.ConstantVariable:
		return []string{v.Value}, nil
	case spec.TextboxVariable:
		return []string{}, nil
	case spec.IntervalVariable:
		if v.Auto {
			return append([]string{spec.SelectAuto}, v.Options...), nil
		}
		return slices.Clone(v.Options), nil
	case spec.DatasourceVariable:
		return r.datasourceValues(v)
	case spec.AdhocVariable:
		return nil, fmt.Errorf("%w: adhoc variable %q has no value list", ErrInvalidRequest, v.Name)
	}
	return r.queryValues(ctx, d, idx, rng, selections, user, lists)
}

func (r *Runner) datasourceValues(v spec.Variable) ([]string, error) {
	uids := []string{}
	for _, inst := range r.reg.List() {
		if inst.Datasource.Type() == string(v.DatasourceType) {
			uids = append(uids, inst.UID)
		}
	}
	if v.Regex == "" {
		return uids, nil
	}
	re, err := spec.CompileVariableRegex(v.Regex)
	if err != nil {
		return nil, fmt.Errorf("%w: variable %q has an invalid regex", ErrInvalidRequest, v.Name)
	}
	return spec.ApplyVariableRegex(re, uids), nil
}

func (r *Runner) queryValues(ctx context.Context, d *spec.Dashboard, idx int, rng datasource.TimeRange, selections map[string]Selection, user datasource.QueryUser, lists map[string]allList) ([]string, error) {
	v := d.Variables[idx]
	if v.Datasource == "" {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "variable %q has no datasource", v.Name)
	}
	var re *spec.VariableRegex
	if v.Regex != "" {
		var err error
		if re, err = spec.CompileVariableRegex(v.Regex); err != nil {
			return nil, fmt.Errorf("%w: variable %q has an invalid regex", ErrInvalidRequest, v.Name)
		}
	}
	inst, err := r.instance(v.Datasource)
	if err != nil {
		return nil, err
	}
	if err := rng.Validate(); err != nil {
		return nil, invalidRangeError(err)
	}
	if name := adhocReference(d, idx, v.Query); name != "" {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "a variable query cannot use adhoc variable %q", name)
	}
	vars, err := r.variablesFor(ctx, d, idx, v.Query, rng, selections, user, lists)
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
	if re != nil {
		values = spec.ApplyVariableRegex(re, values)
	}
	if v.Sort != "" {
		values = spec.SortVariableValues(values, v.Sort)
	}
	return values, nil
}

func adhocReference(d *spec.Dashboard, upto int, text string) string {
	for _, name := range slices.Sorted(maps.Keys(scanQueryUse(text).variables)) {
		if i := variableIndex(d, name); i >= 0 && i < upto && d.Variables[i].Type == spec.AdhocVariable {
			return name
		}
	}
	return ""
}

func (r *Runner) allValues(ctx context.Context, d *spec.Dashboard, idx int, rng datasource.TimeRange, selections map[string]Selection, user datasource.QueryUser, lists map[string]allList) (allList, error) {
	name := d.Variables[idx].Name
	if list, ok := lists[name]; ok {
		return list, nil
	}
	values, err := r.resolveValues(ctx, d, idx, rng, selections, user, lists)
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
	lists[name] = list
	return list, nil
}

func (r *Runner) variablesFor(ctx context.Context, d *spec.Dashboard, upto int, text string, rng datasource.TimeRange, selections map[string]Selection, user datasource.QueryUser, lists map[string]allList) ([]datasource.Variable, error) {
	use := scanQueryUse(text)
	out := make([]datasource.Variable, 0, upto)
	for i, v := range d.Variables[:upto] {
		sel, ok := selections[v.Name]
		if !ok {
			if use.variables[v.Name] {
				return nil, fmt.Errorf("%w: no selection for variable %q", ErrInvalidRequest, v.Name)
			}
			continue
		}
		if err := r.checkSelection(v, sel); err != nil {
			return nil, err
		}
		if v.Type == spec.AdhocVariable && !use.variables[v.Name] {
			continue
		}
		variable, err := r.variableFor(ctx, d, i, rng, use, selections, user, lists)
		if err != nil {
			return nil, err
		}
		out = append(out, variable)
	}
	return out, nil
}

func (r *Runner) variableFor(ctx context.Context, d *spec.Dashboard, idx int, rng datasource.TimeRange, use queryUse, selections map[string]Selection, user datasource.QueryUser, lists map[string]allList) (datasource.Variable, error) {
	v := d.Variables[idx]
	sel := selections[v.Name]
	variable := datasource.Variable{Name: v.Name, Values: slices.Clone(sel.Values), All: sel.All}
	switch {
	case v.Type == spec.AdhocVariable:
		filters, err := r.adhocFilters(v, sel)
		return datasource.Variable{Name: v.Name, Adhoc: true, Filters: filters}, err
	case v.Type == spec.IntervalVariable && slices.Contains(sel.Values, spec.SelectAuto):
		variable.Values = []string{spec.FormatInterval(autoInterval(rng, v))}
	case sel.All && v.AllValue != "":
		variable = datasource.Variable{Name: v.Name, Values: []string{v.AllValue}, All: true, AllValue: true}
	case sel.All && use.listed[v.Name]:
		list, err := r.allValues(ctx, d, idx, rng, selections, user, lists)
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

func (r *Runner) adhocFilters(v spec.Variable, sel Selection) ([]datasource.AdhocFilter, error) {
	if len(sel.Values) == 0 {
		return nil, nil
	}
	inst, err := r.instance(v.Datasource)
	if err != nil {
		return nil, err
	}
	filters := make([]datasource.AdhocFilter, len(sel.Values))
	for i, text := range sel.Values {
		if filters[i], err = parseAdhocFilter(v.Name, inst.Datasource.Type(), text); err != nil {
			return nil, err
		}
	}
	return filters, nil
}

func (r *Runner) implicitFilters(d *spec.Dashboard, selections map[string]Selection, uid string) ([]datasource.AdhocFilter, error) {
	var out []datasource.AdhocFilter
	for _, v := range d.Variables {
		if v.Type != spec.AdhocVariable || v.Datasource != uid {
			continue
		}
		filters, err := r.adhocFilters(v, selections[v.Name])
		if err != nil {
			return nil, err
		}
		out = append(out, filters...)
	}
	return out, nil
}

var labelNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

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
	validKey := labelNamePattern.MatchString(f.Key)
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

func (r *Runner) checkSelection(v spec.Variable, sel Selection) error {
	switch {
	case sel.All:
		return checkAll(v, sel)
	case v.Type == spec.AdhocVariable:
		if len(sel.Values) > maxAdhocFilters {
			return fmt.Errorf("%w: variable %q has more than %d filters", ErrInvalidRequest, v.Name, maxAdhocFilters)
		}
		_, err := r.adhocFilters(v, sel)
		return err
	case v.Type == spec.QueryVariable || v.Type == spec.CustomVariable:
		return checkListSelection(v, sel)
	}
	return r.checkSingleSelection(v, sel)
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

func (r *Runner) checkSingleSelection(v spec.Variable, sel Selection) error {
	if len(sel.Values) != 1 {
		return fmt.Errorf("%w: variable %q takes exactly one value", ErrInvalidRequest, v.Name)
	}
	value := sel.Values[0]
	if err := checkValue(v.Name, value); err != nil {
		return err
	}
	switch v.Type {
	case spec.ConstantVariable:
		if value != v.Value {
			return fmt.Errorf("%w: variable %q is a constant and accepts only its own value", ErrInvalidRequest, v.Name)
		}
	case spec.IntervalVariable:
		if !slices.Contains(v.Options, value) && (value != spec.SelectAuto || !v.Auto) {
			return fmt.Errorf("%w: variable %q has a value that is not one of its options", ErrInvalidRequest, v.Name)
		}
	case spec.DatasourceVariable:
		values, err := r.datasourceValues(v)
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
		if v.Type == spec.CustomVariable && !slices.Contains(v.Options, value) {
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
