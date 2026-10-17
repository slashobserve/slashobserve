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
)

func (r *Runner) VariableValues(ctx context.Context, d *spec.Dashboard, name string, rng datasource.TimeRange, earlier map[string]Selection) (values []string, truncated bool, err error) {
	idx := variableIndex(d, name)
	if idx < 0 {
		return nil, false, fmt.Errorf("%w: %q", ErrUnknownVariable, name)
	}
	values, err = r.resolveValues(ctx, d, idx, rng, earlier)
	if err != nil {
		return nil, false, err
	}
	if len(values) > maxVariableResults {
		return values[:maxVariableResults], true, nil
	}
	return values, false, nil
}

func (r *Runner) Defaults(ctx context.Context, d *spec.Dashboard, rng datasource.TimeRange, given map[string]Selection) (map[string]Selection, error) {
	for _, name := range slices.Sorted(maps.Keys(given)) {
		idx := variableIndex(d, name)
		if idx < 0 {
			return nil, fmt.Errorf("%w: %q", ErrUnknownVariable, name)
		}
		if err := checkSelectionShape(d.Variables[idx], given[name]); err != nil {
			return nil, err
		}
	}
	out := maps.Clone(given)
	if out == nil {
		out = make(map[string]Selection, len(d.Variables))
	}
	for i, v := range d.Variables {
		if _, ok := out[v.Name]; ok {
			continue
		}
		sel, err := r.defaultSelection(ctx, d, i, rng, out)
		if err != nil {
			return nil, err
		}
		out[v.Name] = sel
	}
	return out, nil
}

func (r *Runner) defaultSelection(ctx context.Context, d *spec.Dashboard, idx int, rng datasource.TimeRange, selections map[string]Selection) (Selection, error) {
	v := d.Variables[idx]
	if v.IncludeAll {
		return Selection{All: true}, nil
	}
	values, err := r.resolveValues(ctx, d, idx, rng, selections)
	if err != nil {
		return Selection{}, err
	}
	if len(values) == 0 {
		return Selection{}, fmt.Errorf("%w: %q", ErrVariableEmpty, v.Name)
	}
	return Selection{Values: []string{values[0]}}, nil
}

func (r *Runner) resolveValues(ctx context.Context, d *spec.Dashboard, idx int, rng datasource.TimeRange, selections map[string]Selection) ([]string, error) {
	v := d.Variables[idx]
	if v.Type == spec.CustomVariable {
		return slices.Clone(v.Options), nil
	}
	if v.Datasource == nil {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "variable %q has no datasource", v.Name)
	}
	inst, err := r.instance(*v.Datasource)
	if err != nil {
		return nil, err
	}
	if err := rng.Validate(); err != nil {
		return nil, err
	}
	vars, err := selectionVariables(d.Variables[:idx], selections)
	if err != nil {
		return nil, err
	}
	q := datasource.Query{Text: v.Query, Variables: vars}
	return upstream(ctx, r, "variable:"+v.Name, inst.UID, func(ctx context.Context) ([]string, error) {
		return inst.Datasource.VariableValues(ctx, q, rng)
	})
}

func variableIndex(d *spec.Dashboard, name string) int {
	if d == nil {
		return -1
	}
	return slices.IndexFunc(d.Variables, func(v spec.Variable) bool { return v.Name == name })
}

func checkSelectionShape(v spec.Variable, sel Selection) error {
	if sel.All && !v.IncludeAll {
		return datasource.Errorf(datasource.ErrInvalidQuery, "variable %q does not allow All", v.Name)
	}
	if len(sel.Values) > 1 && !v.Multi {
		return datasource.Errorf(datasource.ErrInvalidQuery, "variable %q does not allow multiple values", v.Name)
	}
	return nil
}

func selectionVariables(declared []spec.Variable, selections map[string]Selection) ([]datasource.Variable, error) {
	out := make([]datasource.Variable, 0, len(declared))
	for _, v := range declared {
		sel, ok := selections[v.Name]
		if !ok {
			continue
		}
		if err := checkSelectionValues(v.Name, sel.Values); err != nil {
			return nil, err
		}
		out = append(out, datasource.Variable{Name: v.Name, Values: slices.Clone(sel.Values), All: sel.All})
	}
	return out, nil
}

func checkSelectionValues(name string, values []string) error {
	if len(values) > maxSelectionValues {
		return datasource.Errorf(datasource.ErrInvalidQuery, "variable %q has more than %d values", name, maxSelectionValues)
	}
	for _, value := range values {
		if len(value) > maxSelectionValueBytes {
			return datasource.Errorf(datasource.ErrInvalidQuery, "variable %q has a value longer than %d bytes", name, maxSelectionValueBytes)
		}
		for i := range len(value) {
			if value[i] < 0x20 || value[i] == 0x7f {
				return datasource.Errorf(datasource.ErrInvalidQuery, "variable %q has a value with a control character", name)
			}
		}
	}
	return nil
}
