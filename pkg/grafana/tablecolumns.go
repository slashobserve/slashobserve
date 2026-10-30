package grafana

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	minColumnWidth   = 20
	maxColumnWidth   = 2000
	maxFrozenColumns = 10
	maxFooterFields  = 50
	maxFieldNameKept = 200
)

var tableColumnProps = map[string]bool{
	"custom.cellOptions": true, "custom.displayMode": true, "custom.wrapText": true,
	"custom.width": true, "custom.minWidth": true, "custom.align": true,
	"custom.filterable": true, "custom.inspect": true, "custom.hideHeader": true,
}

func propColumnWidth(b *panelBuild, p *spec.FieldProps, v any, s propSite) {
	p.Width = b.columnPixels(v, s, "column width")
}

func propColumnMinWidth(b *panelBuild, p *spec.FieldProps, v any, s propSite) {
	p.MinWidth = b.columnPixels(v, s, "minimum column width")
}

func (b *panelBuild) columnPixels(v any, s propSite, what string) int {
	b.consume(s.value)
	f, ok := numberValue(v)
	if !ok || f <= 0 {
		return 0
	}
	pixels := int(math.Round(f))
	if pixels < minColumnWidth || pixels > maxColumnWidth {
		b.partial(s.at(""), fmt.Sprintf("The %s was kept between %d and %d pixels.", what, minColumnWidth, maxColumnWidth))
		pixels = min(max(pixels, minColumnWidth), maxColumnWidth)
	}
	return pixels
}

func propColumnAlign(b *panelBuild, p *spec.FieldProps, v any, s propSite) {
	b.consume(s.value)
	switch align := asString(v); align {
	case "left", "center", "right":
		p.Align = align
	case "auto":
		if s.whole {
			p.Align = align
		}
	default:
		b.partial(s.at(""), fmt.Sprintf("The column alignment %s is not supported yet.", align))
	}
}

func columnFlag(target func(*spec.FieldProps) **bool) propHandler {
	return func(b *panelBuild, p *spec.FieldProps, v any, s propSite) {
		b.consume(s.value)
		if on, isBool := v.(bool); isBool && (on || s.whole) {
			*target(p) = &on
		}
	}
}

var (
	propColumnFilterable = columnFlag(func(p *spec.FieldProps) **bool { return &p.Filterable })
	propColumnInspect    = columnFlag(func(p *spec.FieldProps) **bool { return &p.Inspect })
	propColumnHideHeader = columnFlag(func(p *spec.FieldProps) **bool { return &p.HideHeader })
)

func (b *panelBuild) tableOptions(d *spec.Display) {
	b.tableCellHeight(d)
	if on, ok := b.boolOption("options.showTypeIcons"); ok && *on {
		d.TypeIcons = on
	}
	b.tablePagination(d)
	b.tableFrozenColumns(d)
	b.tableSortBy(d)
	b.tableFooter(d)
}

func (b *panelBuild) tableCellHeight(d *spec.Display) {
	const path = "options.cellHeight"
	v, ok := b.value(path)
	if !ok {
		return
	}
	switch height := asString(v); height {
	case "sm":
		b.consume(path)
	case "md", "lg":
		b.consume(path)
		d.CellHeight = height
	default:
		b.partial(path, fmt.Sprintf("The row height %s is not supported yet.", height))
	}
}

func (b *panelBuild) tablePagination(d *spec.Display) {
	for _, path := range []string{"options.enablePagination", "options.footer.enablePagination"} {
		if on, ok := b.boolOption(path); ok && *on {
			d.Pagination = on
		}
	}
}

func (b *panelBuild) tableFrozenColumns(d *spec.Display) {
	const path = "options.frozenColumns.left"
	v, ok := b.take(path)
	if !ok {
		return
	}
	n, isInt := asInt(v)
	if !isInt || n < 0 {
		b.partial(path, "The number of frozen columns must be a whole number.")
		return
	}
	if n > maxFrozenColumns {
		b.partial(path, fmt.Sprintf("At most %d columns can be frozen.", maxFrozenColumns))
		n = maxFrozenColumns
	}
	d.FrozenColumns = n
}

func (b *panelBuild) tableSortBy(d *spec.Display) {
	const path = "options.sortBy"
	v, ok := b.take(path)
	if !ok {
		return
	}
	entries, _ := asArray(v)
	if len(entries) == 0 {
		return
	}
	first, _ := asObject(entries[0])
	name := strings.TrimSpace(asString(first["displayName"]))
	if name == "" || len([]rune(name)) > maxFieldNameKept {
		b.partial(path, "The sort column is missing or its name is too long to be kept.")
		return
	}
	d.SortBy = name
	if truthy(first["desc"]) {
		d.SortDesc = ptr(true)
	}
	if len(entries) > 1 {
		b.partial(path, "Only the first sort column is kept.")
	}
}

func (b *panelBuild) tableFooter(d *spec.Display) {
	show, hasShow := b.boolOption("options.footer.show")
	reducer, hasReducer := b.take("options.footer.reducer")
	countRows, hasCount := b.boolOption("options.footer.countRows")
	fields, hasFields := b.take("options.footer.fields")
	if !hasShow || !*show {
		return
	}
	switch {
	case !hasReducer:
		d.Footer = []string{"sum"}
	default:
		d.Footer = b.footerReducers(singleToList(reducer))
	}
	if hasCount && *countRows {
		d.FooterCountRows = countRows
	}
	if hasFields {
		d.FooterFields = b.footerFieldNames(singleToList(fields))
	}
}

func singleToList(v any) any {
	if text, isText := v.(string); isText {
		return []any{text}
	}
	return v
}

func (b *panelBuild) footerFieldNames(v any) []string {
	var out []string
	cut := false
	for _, name := range stringList(v) {
		name = strings.TrimSpace(name)
		switch {
		case name == "" || slices.Contains(out, name):
		case len([]rune(name)) > maxFieldNameKept || len(out) >= maxFooterFields:
			cut = true
		default:
			out = append(out, name)
		}
	}
	if cut {
		b.partial("options.footer.fields", "Some footer columns are too many or too long to be kept.")
	}
	return out
}
