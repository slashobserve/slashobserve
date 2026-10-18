package spec

import (
	"slices"
	"strconv"
)

type PanelEntry struct {
	Pointer string
	Panel   *Panel
	Row     *Panel
}

func (d *Dashboard) AllPanels() []PanelEntry {
	entries := make([]PanelEntry, 0, len(d.Panels))
	for i := range d.Panels {
		p := &d.Panels[i]
		entries = append(entries, PanelEntry{Pointer: pointer("panels", strconv.Itoa(i)), Panel: p})
		for j := range p.Panels {
			entries = append(entries, PanelEntry{
				Pointer: pointer("panels", strconv.Itoa(i), "panels", strconv.Itoa(j)),
				Panel:   &p.Panels[j],
				Row:     p,
			})
		}
	}
	return entries
}

func (d *Dashboard) FindPanel(id string) (PanelEntry, bool) {
	for _, e := range d.AllPanels() {
		if e.Panel.ID == id {
			return e, true
		}
	}
	return PanelEntry{}, false
}

func (d *Dashboard) DataPanels() []PanelEntry {
	return slices.DeleteFunc(d.AllPanels(), func(e PanelEntry) bool {
		return !e.Panel.Type.IsData() || len(e.Panel.Queries) == 0
	})
}

func (d *Dashboard) FindDataPanel(id string) (PanelEntry, bool) {
	for _, e := range d.DataPanels() {
		if e.Panel.ID == id {
			return e, true
		}
	}
	return PanelEntry{}, false
}

func (d *Dashboard) PanelCount() int {
	n := len(d.Panels)
	for i := range d.Panels {
		n += len(d.Panels[i].Panels)
	}
	return n
}

func DefaultQueryRef(i int) string {
	if i < 0 || i > 25 {
		panic("spec: query index outside 0..25")
	}
	return string(rune('A' + i))
}

func (p *Panel) QueryRef(i int) string {
	if ref := p.Queries[i].Ref; ref != "" {
		return ref
	}
	return DefaultQueryRef(i)
}

func (p *Panel) QueryDatasource(i int) string {
	if ds := p.Queries[i].Datasource; ds != "" {
		return ds
	}
	return p.Datasource
}
