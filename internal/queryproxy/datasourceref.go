package queryproxy

import (
	"context"
	"fmt"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func referencedVariable(d *spec.Dashboard, ref string) (name string, ok bool) {
	name, ok = spec.DatasourceVariableName(ref)
	if !ok {
		return "", false
	}
	idx := variableIndex(d, name)
	return name, idx >= 0 && d.Variables[idx].Type == spec.DatasourceVariable
}

func ResolveDatasourceUID(d *spec.Dashboard, ref string, selections map[string]Selection) (string, error) {
	name, ok := spec.DatasourceVariableName(ref)
	if !ok {
		return ref, nil
	}
	if _, ok := referencedVariable(d, ref); !ok {
		return "", fmt.Errorf("%w: %q", ErrUnknownVariable, name)
	}
	sel, ok := selections[name]
	if !ok || sel.All || len(sel.Values) != 1 {
		return "", fmt.Errorf("%w: no datasource selected for variable %q", ErrInvalidRequest, name)
	}
	return sel.Values[0], nil
}

func (r *Runner) resolveDatasource(d *spec.Dashboard, ref string, selections map[string]Selection) (datasource.Instance, error) {
	uid, err := ResolveDatasourceUID(d, ref, selections)
	if err != nil {
		return datasource.Instance{}, err
	}
	inst, err := r.instance(uid)
	if err != nil {
		return datasource.Instance{}, err
	}
	if name, ok := referencedVariable(d, ref); ok {
		v := d.Variables[variableIndex(d, name)]
		if inst.Datasource.Type() != string(v.DatasourceType) {
			return datasource.Instance{}, fmt.Errorf("%w: variable %q selects a datasource that is not %s", ErrInvalidRequest, name, v.DatasourceType)
		}
	}
	return inst, nil
}

func (r *Runner) datasourceType(d *spec.Dashboard, ref string) (spec.DatasourceType, error) {
	if name, ok := spec.DatasourceVariableName(ref); ok {
		if _, declared := referencedVariable(d, ref); !declared {
			return "", fmt.Errorf("%w: %q", ErrUnknownVariable, name)
		}
		return d.Variables[variableIndex(d, name)].DatasourceType, nil
	}
	inst, err := r.instance(ref)
	if err != nil {
		return "", err
	}
	return spec.DatasourceType(inst.Datasource.Type()), nil
}

func (r *Runner) selectedDatasource(ctx context.Context, d *spec.Dashboard, ref string, rng datasource.TimeRange, selections map[string]Selection, user datasource.QueryUser, st *varState) (datasource.Instance, error) {
	if name, ok := referencedVariable(d, ref); ok {
		if sel, selected := selections[name]; selected {
			if err := r.checkSelection(ctx, d, variableIndex(d, name), rng, selections, user, st, sel); err != nil {
				return datasource.Instance{}, err
			}
		}
	}
	return r.resolveDatasource(d, ref, selections)
}
