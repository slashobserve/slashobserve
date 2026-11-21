package queryproxy

import (
	"context"
	"slices"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func (r *Runner) UsesRestricted(d *spec.Dashboard) bool {
	return DashboardUsesRestricted(d, r.reg)
}

func DashboardUsesRestricted(d *spec.Dashboard, reg *datasource.Registry) bool {
	_, found := FirstRestricted(DatasourceRefs(d), reg.AdminsOnly)
	return found
}

func FirstRestricted(refs []string, adminsOnly func(uid string) bool) (string, bool) {
	for _, ref := range refs {
		if adminsOnly(ref) {
			return ref, true
		}
	}
	return "", false
}

func DatasourceRefs(d *spec.Dashboard) []string {
	if d == nil {
		return nil
	}
	var refs []string
	add := func(ref string) {
		if _, variable := spec.DatasourceVariableName(ref); ref != "" && !variable && !slices.Contains(refs, ref) {
			refs = append(refs, ref)
		}
	}
	for _, v := range d.Variables {
		add(v.Datasource)
		if v.Type != spec.DatasourceVariable {
			continue
		}
		if v.Default != nil {
			for _, value := range v.Default.Values {
				add(value)
			}
		}
		for _, o := range v.Options {
			add(o.Value)
		}
		for _, o := range v.StaticOptions {
			add(o.Value)
		}
		add(v.Value)
	}
	for _, e := range d.AllPanels() {
		add(e.Panel.Datasource)
		for _, q := range e.Panel.Queries {
			add(q.Datasource)
		}
	}
	return refs
}

func (r *Runner) CheckPanelAccess(ctx context.Context, d *spec.Dashboard, panelID string, selections map[string]Selection) error {
	panel, ok := dataPanel(d, panelID)
	if !ok {
		return nil
	}
	for j, q := range panel.Queries {
		if q.IsExpression() || q.IsDashboardSource() {
			continue
		}
		if _, err := r.resolveDatasource(ctx, d, panel.QueryDatasource(j), selections); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) CheckVariableAccess(ctx context.Context, d *spec.Dashboard, name string, selections map[string]Selection) error {
	i := variableIndex(d, name)
	if i < 0 || d.Variables[i].Datasource == "" {
		return nil
	}
	ref := d.Variables[i].Datasource
	if picker, ok := referencedVariable(d, ref); ok {
		if _, selected := selections[picker]; !selected {
			return nil
		}
	}
	_, err := r.resolveDatasource(ctx, d, ref, selections)
	return err
}
