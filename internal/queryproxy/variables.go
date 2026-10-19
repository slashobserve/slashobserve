package queryproxy

import (
	"context"
	"fmt"
	"maps"
	"slices"

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
		if err := checkSelection(d.Variables[idx], given[name]); err != nil {
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
		if err := checkSelection(v, sel); err != nil {
			return nil, err
		}
		out[v.Name] = sel
	}
	return out, nil
}

func (r *Runner) defaultSelection(ctx context.Context, d *spec.Dashboard, idx int, rng datasource.TimeRange, selections map[string]Selection, user datasource.QueryUser, lists map[string]allList) (Selection, error) {
	v := d.Variables[idx]
	if v.IncludeAll {
		return Selection{All: true}, nil
	}
	values, err := r.resolveValues(ctx, d, idx, rng, selections, user, lists)
	if err != nil {
		return Selection{}, err
	}
	if len(values) == 0 {
		return Selection{}, fmt.Errorf("%w: %q", ErrVariableEmpty, v.Name)
	}
	return Selection{Values: []string{values[0]}}, nil
}

func (r *Runner) resolveValues(ctx context.Context, d *spec.Dashboard, idx int, rng datasource.TimeRange, selections map[string]Selection, user datasource.QueryUser, lists map[string]allList) ([]string, error) {
	v := d.Variables[idx]
	if v.Type == spec.CustomVariable {
		return slices.Clone(v.Options), nil
	}
	if v.Type != spec.QueryVariable {
		return nil, fmt.Errorf("%w: variable %q of type %s is not supported by the query proxy yet", ErrInvalidRequest, v.Name, v.Type)
	}
	if v.Datasource == "" {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "variable %q has no datasource", v.Name)
	}
	inst, err := r.instance(v.Datasource)
	if err != nil {
		return nil, err
	}
	if err := rng.Validate(); err != nil {
		return nil, invalidRangeError(err)
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
	if r.values == nil {
		return fetch(ctx)
	}
	return r.values.get(ctx, valueKey(inst.UID, q, rng), fetch)
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
		if err := checkSelection(v, sel); err != nil {
			return nil, err
		}
		variable := datasource.Variable{Name: v.Name, Values: slices.Clone(sel.Values), All: sel.All}
		if sel.All && use.listed[v.Name] {
			list, err := r.allValues(ctx, d, i, rng, selections, user, lists)
			if err != nil {
				return nil, err
			}
			if !list.unlisted {
				variable.Values = slices.Clone(list.values)
			}
		}
		out = append(out, variable)
	}
	return out, nil
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
	if name := ref.Guard; name != "" && len(nested) > 0 {
		u.variables[name] = true
		nested = nested[:len(nested)-1]
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

func checkSelection(v spec.Variable, sel Selection) error {
	if sel.All {
		if !v.IncludeAll {
			return fmt.Errorf("%w: variable %q does not allow All", ErrInvalidRequest, v.Name)
		}
		if len(sel.Values) > 0 {
			return fmt.Errorf("%w: variable %q cannot combine All with values", ErrInvalidRequest, v.Name)
		}
		return nil
	}
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
