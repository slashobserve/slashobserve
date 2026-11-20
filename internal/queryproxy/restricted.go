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
	if d == nil {
		return false
	}
	restricted := func(ref string) bool { return ref != "" && reg.AdminsOnly(ref) }
	for _, v := range d.Variables {
		if restricted(v.Datasource) {
			return true
		}
		if v.Type == spec.DatasourceVariable && v.Default != nil && slices.ContainsFunc(v.Default.Values, restricted) {
			return true
		}
	}
	for _, e := range d.DataPanels() {
		for j := range e.Panel.Queries {
			if restricted(e.Panel.QueryDatasource(j)) {
				return true
			}
		}
	}
	return false
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
