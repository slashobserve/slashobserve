package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	maxUncovered         = 50
	maxNotes             = 20
	maxStoredInventories = 256
)

var (
	numericPanels = []spec.PanelType{spec.Stat, spec.Gauge, spec.BarGauge, spec.Timeseries}
	legendPanels  = []spec.PanelType{spec.Timeseries, spec.BarChart}
)

type coverage struct {
	Uncovered      []string `json:"uncovered"`
	UncoveredTotal int      `json:"uncovered_total"`
}

type advice struct {
	*coverage
	Notes []string `json:"notes,omitempty"`
}

func adviceFor(doc *spec.Dashboard, inventory []datasource.Family) advice {
	var out advice
	if doc == nil {
		return out
	}
	entries := doc.AllPanels()
	if inventory != nil {
		out.coverage = uncoveredBy(entries, inventory)
	}
	for _, e := range entries {
		out.Notes = append(out.Notes, panelNotes(e.Panel)...)
	}
	out.Notes = out.Notes[:min(len(out.Notes), maxNotes)]
	return out
}

func uncoveredBy(entries []spec.PanelEntry, inventory []datasource.Family) *coverage {
	var texts []string
	for _, e := range entries {
		for _, q := range e.Panel.Queries {
			if q.Query != "" {
				texts = append(texts, q.Query)
			}
		}
	}
	out := &coverage{Uncovered: []string{}}
	for _, f := range inventory {
		if slices.ContainsFunc(texts, func(text string) bool { return queriesFamily(text, f) }) {
			continue
		}
		out.UncoveredTotal++
		if len(out.Uncovered) < maxUncovered {
			out.Uncovered = append(out.Uncovered, clip(f.Name, maxStepNameBytes))
		}
	}
	return out
}

func queriesFamily(text string, f datasource.Family) bool {
	return slices.ContainsFunc(f.SeriesNames(), func(name string) bool { return containsName(text, name) })
}

func containsName(text, name string) bool {
	for from := 0; from < len(text); {
		at := strings.Index(text[from:], name)
		if at < 0 {
			return false
		}
		start, end := from+at, from+at+len(name)
		if (start == 0 || !isNameByte(text[start-1])) && (end == len(text) || !isNameByte(text[end])) {
			return true
		}
		from = start + 1
	}
	return false
}

func isNameByte(c byte) bool {
	return c == '_' || c == ':' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func panelNotes(p *spec.Panel) []string {
	id := clip(p.ID, maxStepNameBytes)
	var notes []string
	if slices.Contains(legendPanels, p.Type) && slices.ContainsFunc(p.Queries, func(q spec.Query) bool { return q.Query != "" && q.Legend == "" }) {
		notes = append(notes, fmt.Sprintf("panel %s has a query with no legend; set legend when it returns several series", id))
	}
	if slices.Contains(numericPanels, p.Type) && p.Unit == "" {
		notes = append(notes, fmt.Sprintf("panel %s is numeric and has no unit", id))
	}
	if p.Type == spec.Stat && p.Description == "" {
		notes = append(notes, fmt.Sprintf("stat panel %s has no description", id))
	}
	return notes
}

func withAdvice(ctx context.Context, out savedOut, doc *spec.Dashboard) savedOut {
	inventory, _ := inventories.get(ctx)
	out.advice = adviceFor(doc, inventory)
	body, err := json.Marshal(out.advice)
	if err != nil {
		return out
	}
	var cut bool
	out.Panels, cut = fit(out.Panels, listBudget-len(body))
	out.Truncated = out.Truncated || cut
	return out
}

type inventoryStore struct {
	mu       sync.Mutex
	families map[string][]datasource.Family
	order    []string
}

var inventories = &inventoryStore{families: map[string][]datasource.Family{}}

func inventoryKey(ctx context.Context) (string, bool) {
	if run, inRun := RunFrom(ctx); inRun {
		return "run:" + run.RequestID, run.RequestID != ""
	}
	principal, ok := auth.FromContext(ctx)
	return "principal:" + string(principal.Kind) + ":" + principal.ID, ok && principal.ID != ""
}

func (s *inventoryStore) put(ctx context.Context, families []datasource.Family) {
	key, ok := inventoryKey(ctx)
	if !ok {
		return
	}
	if families == nil {
		families = []datasource.Family{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.order = slices.DeleteFunc(s.order, func(k string) bool { return k == key })
	if len(s.order) == maxStoredInventories {
		delete(s.families, s.order[0])
		s.order = s.order[1:]
	}
	s.order = append(s.order, key)
	s.families[key] = families
}

func (s *inventoryStore) get(ctx context.Context) ([]datasource.Family, bool) {
	key, ok := inventoryKey(ctx)
	if !ok {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	families, found := s.families[key]
	return families, found
}
