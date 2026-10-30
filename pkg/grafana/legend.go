package grafana

import (
	"math"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	legendSortReason = "Sorting the legend by this value is not supported yet."
	minLegendWidth   = 40
	maxLegendWidth   = 2000
)

func (b *panelBuild) legendTable(d *spec.Display) {
	if _, ok := b.value("options.legend"); !ok {
		return
	}
	table := b.enumOption("options.legend.displayMode", "table", "list") == "table"
	if v, ok := b.boolOption("options.legend.asTable"); ok && *v {
		table = true
	}
	b.legendMode(d, table)
	if v, ok := b.boolOption("options.legend.isVisible"); ok && !*v {
		d.Legend = "hidden"
	}
	if v, ok := b.take("options.legend.sortBy"); ok {
		b.legendSortBy(d, "options.legend.sortBy", v, modernSortReducer)
	}
	b.legendSortDesc(d, "options.legend.sortDesc")
	b.legendWidth(d, "options.legend.width")
}

func (b *panelBuild) graphLegendTable(d *spec.Display, legend map[string]any) {
	b.legendMode(d, truthy(legend["alignAsTable"]))
	b.consume("legend.alignAsTable")
	if v, ok := b.take("legend.sort"); ok {
		b.legendSortBy(d, "legend.sort", v, legacySortReducer)
	}
	b.legendSortDesc(d, "legend.sortDesc")
	b.legendWidth(d, "legend.sideWidth")
	if truthy(legend["hideZero"]) {
		d.LegendHideZero = ptr(true)
	}
	if truthy(legend["hideEmpty"]) {
		d.LegendHideEmpty = ptr(true)
	}
	b.consume("legend.hideZero", "legend.hideEmpty")
}

func (b *panelBuild) legendMode(d *spec.Display, table bool) {
	mode := "list"
	if table {
		mode = "table"
	}
	if mode != impliedLegendMode(d) {
		d.LegendMode = mode
	}
}

func impliedLegendMode(d *spec.Display) string {
	if len(d.LegendValues) > 0 {
		return "table"
	}
	return "list"
}

func (b *panelBuild) legendSortBy(d *spec.Display, path string, v any, reducerOf func(string) (string, bool)) {
	if v == nil || v == "" {
		return
	}
	if reducer, ok := reducerOf(asString(v)); ok {
		d.LegendSortBy = reducer
		return
	}
	b.partial(path, legendSortReason)
}

func modernSortReducer(name string) (string, bool) {
	if name == "Name" {
		return "name", true
	}
	for id, display := range reducerNames {
		if display == name {
			return id, true
		}
	}
	return "", false
}

func legacySortReducer(key string) (string, bool) {
	for _, calc := range legendCalcs {
		if calc.key == key {
			return calc.reducer, true
		}
	}
	return "", false
}

func (b *panelBuild) legendSortDesc(d *spec.Display, path string) {
	if v, ok := b.boolOption(path); ok && *v {
		d.LegendSortDesc = ptr(true)
	}
}

func (b *panelBuild) legendWidth(d *spec.Display, path string) {
	v, ok := b.take(path)
	if !ok {
		return
	}
	if px, isNumber := numberValue(v); isNumber && px > 0 {
		d.LegendWidth = int(math.Min(maxLegendWidth, math.Max(minLegendWidth, math.Round(px))))
	}
}
