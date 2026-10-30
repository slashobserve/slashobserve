package spec

import (
	"fmt"
	"strconv"
)

func (q Query) IsDashboardSource() bool { return q.Source != nil }

func (p *Panel) DashboardSource() (DashboardSource, bool) {
	if !p.Type.IsData() || len(p.Queries) != 1 || p.Queries[0].Source == nil {
		return DashboardSource{}, false
	}
	return *p.Queries[0].Source, true
}

func checkDashboardSources(d *Dashboard, entries []PanelEntry) []Issue {
	var issues []Issue
	for _, e := range entries {
		if !e.Panel.Type.IsData() {
			continue
		}
		for j, q := range e.Panel.Queries {
			if q.IsDashboardSource() {
				issues = append(issues, checkSourceQuery(d, e, j)...)
			}
		}
	}
	return issues
}

func checkSourceQuery(d *Dashboard, e PanelEntry, j int) []Issue {
	p := e.Panel
	base := e.Pointer + pointer("queries", strconv.Itoa(j))
	invalid := func(path, format string, args ...any) Issue {
		return Issue{Path: path, Code: CodeInvalidReference, Message: fmt.Sprintf(format, args...)}
	}
	var issues []Issue
	if len(p.Queries) != 1 {
		issues = append(issues, invalid(base, "a panel that reuses another panel's results has exactly one query"))
	}
	for _, own := range []struct{ key, value string }{
		{"datasource", p.Datasource},
		{"interval", p.Interval},
		{"timeFrom", p.TimeFrom},
		{"timeShift", p.TimeShift},
	} {
		if own.value != "" {
			issues = append(issues, invalid(e.Pointer+"/"+own.key, "a panel that reuses another panel's results has no %s of its own", own.key))
		}
	}
	if p.MaxDataPoints != 0 {
		issues = append(issues, invalid(e.Pointer+"/maxDataPoints", "a panel that reuses another panel's results has no maxDataPoints of its own"))
	}
	panelPath := base + "/source/panel"
	source := p.Queries[j].Source.Panel
	target, ok := d.FindPanel(source)
	switch {
	case source == p.ID:
		issues = append(issues, invalid(panelPath, "panel %q cannot reuse its own results", source))
	case !ok || !target.Panel.Type.IsData() || len(target.Panel.Queries) == 0:
		issues = append(issues, invalid(panelPath, "source panel %q is not a data panel of this dashboard", source))
	case hasDashboardSource(target.Panel):
		issues = append(issues, invalid(panelPath, "source panel %q reuses another panel's results; a source cannot be chained", source))
	case target.Panel.Repeat != "" || (target.Row != nil && target.Row.Repeat != ""):
		issues = append(issues, invalid(panelPath, "source panel %q repeats, so it has no single set of results to reuse", source))
	}
	return issues
}

func hasDashboardSource(p *Panel) bool {
	for _, q := range p.Queries {
		if q.IsDashboardSource() {
			return true
		}
	}
	return false
}

func (d *Dashboard) SourceOf(id string) (PanelEntry, bool) {
	entry, ok := d.FindPanel(id)
	if !ok {
		return PanelEntry{}, false
	}
	source, reuses := entry.Panel.DashboardSource()
	if !reuses {
		return PanelEntry{}, false
	}
	target, ok := d.FindDataPanel(source.Panel)
	return target, ok
}
