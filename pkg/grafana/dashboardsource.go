package grafana

import (
	"maps"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	reusedPanelMissingReason = "The panel this one reuses could not be imported."
	mixedSourceReason        = "A panel that reuses another panel's results has no other queries."
	adHocFiltersReason       = "Ad hoc filters always apply to the queries of the panel this one reuses."
	reusedSettingReason      = "A panel that reuses another panel's results has no query settings of its own."
)

var reusedPanelSettings = []string{"interval", "maxDataPoints", "timeFrom", "timeShift"}

type reusableSource struct {
	panel   *builtPanel
	repeats bool
}

func (n *panelNode) reusesPanel() bool {
	for _, ref := range n.ds.targets {
		if ref.isDashboard() {
			return true
		}
	}
	return false
}

func (c *converter) convertOrDefer(n *panelNode, repeats bool, later *[]func(), set func(*builtPanel)) {
	if n.reusesPanel() {
		*later = append(*later, func() { set(c.convertPanel(n)) })
		return
	}
	bp := c.convertPanel(n)
	c.registerReusable(n, bp, repeats)
	set(bp)
}

func (c *converter) registerReusable(n *panelNode, bp *builtPanel, repeats bool) {
	if n.grafanaID == nil || bp.isRow() || !bp.panel.Type.IsData() || len(bp.panel.Queries) == 0 {
		return
	}
	if c.reusable == nil {
		c.reusable = map[int]reusableSource{}
	}
	c.reusable[*n.grafanaID] = reusableSource{panel: bp, repeats: repeats || asString(n.raw["repeat"]) != ""}
}

func (tb *targetBuild) dashboardSource() (spec.Query, string) {
	tb.use("panelId", "withTransforms")
	if truthy(tb.t["adHocFiltersEnabled"]) {
		tb.partial("adHocFiltersEnabled", adHocFiltersReason)
	} else {
		tb.use("adHocFiltersEnabled")
	}
	id, ok := asInt(tb.t["panelId"])
	if !ok {
		return spec.Query{}, reusedPanelMissingReason
	}
	source, found := tb.b.c.reusable[id]
	if !found || source.repeats {
		return spec.Query{}, reusedPanelMissingReason
	}
	tb.reuses = source.panel
	return spec.Query{
		Hide:   truthy(tb.t["hide"]),
		Source: &spec.DashboardSource{Panel: source.panel.panel.ID, WithTransforms: truthy(tb.t["withTransforms"])},
	}, ""
}

func (b *panelBuild) reusesPanel() bool {
	for _, bq := range b.built {
		if bq.reuses != nil {
			return true
		}
	}
	return false
}

func (b *panelBuild) dropMixedSources() {
	if len(b.built) < 2 {
		return
	}
	for k := len(b.built) - 1; k >= 0; k-- {
		if b.built[k].reuses != nil {
			b.dropBuilt(k, mixedSourceReason)
		}
	}
}

func (b *panelBuild) adoptSourceRefs() {
	if !b.reusesPanel() {
		return
	}
	b.refs = maps.Clone(b.built[0].reuses.refs)
	b.instantRefs = maps.Clone(b.built[0].reuses.instantRefs)
}

func (b *panelBuild) dropOwnQuerySettings() {
	for _, key := range reusedPanelSettings {
		if v, ok := b.value(key); ok && present(v) {
			if n, isNumber := numberValue(v); isNumber && n == 0 {
				b.consume(key)
				continue
			}
			b.partial(key, reusedSettingReason)
		}
	}
}
