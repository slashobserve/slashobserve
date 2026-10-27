package dashboards

import (
	"encoding/json"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

type Unverified struct {
	Panels    []UnverifiedPanel    `json:"panels"`
	Variables []UnverifiedVariable `json:"variables"`
}

type UnverifiedPanel struct {
	PanelID string       `json:"panel_id"`
	Issues  []spec.Issue `json:"issues"`
}

type UnverifiedVariable struct {
	Name   string       `json:"name"`
	Issues []spec.Issue `json:"issues"`
}

func (u Unverified) Empty() bool {
	return len(u.Panels) == 0 && len(u.Variables) == 0
}

func (u Unverified) withEmptyLists() Unverified {
	out := Unverified{Panels: make([]UnverifiedPanel, len(u.Panels)), Variables: make([]UnverifiedVariable, len(u.Variables))}
	for i, p := range u.Panels {
		out.Panels[i] = UnverifiedPanel{PanelID: p.PanelID, Issues: nonNilIssues(p.Issues)}
	}
	for i, v := range u.Variables {
		out.Variables[i] = UnverifiedVariable{Name: v.Name, Issues: nonNilIssues(v.Issues)}
	}
	return out
}

func nonNilIssues(issues []spec.Issue) []spec.Issue {
	if issues == nil {
		return []spec.Issue{}
	}
	return issues
}

func (u Unverified) Panel(id string) (UnverifiedPanel, bool) {
	i := slices.IndexFunc(u.Panels, func(p UnverifiedPanel) bool { return p.PanelID == id })
	if i < 0 {
		return UnverifiedPanel{}, false
	}
	return u.Panels[i], true
}

func (u Unverified) Variable(name string) (UnverifiedVariable, bool) {
	i := slices.IndexFunc(u.Variables, func(v UnverifiedVariable) bool { return v.Name == name })
	if i < 0 {
		return UnverifiedVariable{}, false
	}
	return u.Variables[i], true
}

func UnverifiedOf(d *spec.Dashboard, issues []spec.Issue) (Unverified, []spec.Issue) {
	byPanel := map[string][]spec.Issue{}
	byVariable := map[string][]spec.Issue{}
	var rest []spec.Issue
	entries := d.AllPanels()
	for _, is := range issues {
		if i, ok := variableIndex(is.Path); ok && i < len(d.Variables) {
			name := d.Variables[i].Name
			byVariable[name] = append(byVariable[name], storable(is))
			continue
		}
		if id, ok := owningPanel(entries, is.Path); ok {
			byPanel[id] = append(byPanel[id], storable(is))
			continue
		}
		rest = append(rest, is)
	}
	var out Unverified
	for _, e := range entries {
		if found := byPanel[e.Panel.ID]; len(found) > 0 {
			out.Panels = append(out.Panels, UnverifiedPanel{PanelID: e.Panel.ID, Issues: found})
			delete(byPanel, e.Panel.ID)
		}
	}
	for _, v := range d.Variables {
		if found := byVariable[v.Name]; len(found) > 0 {
			out.Variables = append(out.Variables, UnverifiedVariable{Name: v.Name, Issues: found})
			delete(byVariable, v.Name)
		}
	}
	return out, rest
}

func storable(is spec.Issue) spec.Issue {
	is.Message = strings.ReplaceAll(is.Message, "\x00", "\uFFFD")
	return is
}

func variableIndex(path string) (int, bool) {
	token, ok := strings.CutPrefix(path, "/variables/")
	if !ok {
		return 0, false
	}
	token, _, _ = strings.Cut(token, "/")
	i, err := strconv.Atoi(token)
	if err != nil || i < 0 || strconv.Itoa(i) != token {
		return 0, false
	}
	return i, true
}

func owningPanel(entries []spec.PanelEntry, path string) (string, bool) {
	best, id := -1, ""
	for _, e := range entries {
		if (path == e.Pointer || strings.HasPrefix(path, e.Pointer+"/")) && len(e.Pointer) > best {
			best, id = len(e.Pointer), e.Panel.ID
		}
	}
	return id, best >= 0
}

func CarryUnverified(prev *spec.Dashboard, prevUnverified Unverified, next *spec.Dashboard, issues []spec.Issue) (Unverified, []spec.Issue) {
	failing, blocking := UnverifiedOf(next, issues)
	var carried Unverified
	for _, p := range failing.Panels {
		if _, was := prevUnverified.Panel(p.PanelID); was && prev != nil && panelUnchanged(prev, next, p.PanelID) {
			carried.Panels = append(carried.Panels, p)
			continue
		}
		blocking = append(blocking, p.Issues...)
	}
	for _, v := range failing.Variables {
		if _, was := prevUnverified.Variable(v.Name); was && prev != nil && variableUnchanged(prev, next, v.Name) {
			carried.Variables = append(carried.Variables, v)
			continue
		}
		blocking = append(blocking, v.Issues...)
	}
	slices.SortStableFunc(blocking, func(a, b spec.Issue) int { return strings.Compare(a.Path, b.Path) })
	return carried, blocking
}

func panelUnchanged(prev, next *spec.Dashboard, id string) bool {
	was, ok := prev.FindDataPanel(id)
	if !ok {
		return false
	}
	now, ok := next.FindDataPanel(id)
	if !ok || len(was.Panel.Queries) != len(now.Panel.Queries) {
		return false
	}
	for j := range was.Panel.Queries {
		a, okA := queryKey(was.Panel, j)
		b, okB := queryKey(now.Panel, j)
		if !okA || !okB || a != b {
			return false
		}
	}
	before, okBefore := reachedByPanel(prev, was.Panel)
	after, okAfter := reachedByPanel(next, now.Panel)
	return okBefore && okAfter && maps.Equal(before, after)
}

func variableUnchanged(prev, next *spec.Dashboard, name string) bool {
	if !declares(prev, name) || !declares(next, name) {
		return false
	}
	before, okBefore := reached(prev, []string{name}, nil)
	after, okAfter := reached(next, []string{name}, nil)
	return okBefore && okAfter && maps.Equal(before, after)
}

func declares(d *spec.Dashboard, name string) bool {
	return slices.ContainsFunc(d.Variables, func(v spec.Variable) bool { return v.Name == name })
}

func queryKey(p *spec.Panel, j int) (string, bool) {
	q := p.Queries[j]
	q.Ref = p.QueryRef(j)
	q.Datasource = p.QueryDatasource(j)
	raw, err := json.Marshal(q)
	return string(raw), err == nil
}

func reachedByPanel(d *spec.Dashboard, p *spec.Panel) (map[string]string, bool) {
	var names []string
	datasources := map[string]bool{}
	for j, q := range p.Queries {
		names = append(names, variableNames(q.Query)...)
		datasources[p.QueryDatasource(j)] = true
	}
	return reached(d, names, datasources)
}

func reached(d *spec.Dashboard, names []string, datasources map[string]bool) (map[string]string, bool) {
	declared := map[string]*spec.Variable{}
	for i := range d.Variables {
		declared[d.Variables[i].Name] = &d.Variables[i]
	}
	queue := slices.Clone(names)
	for _, v := range d.Variables {
		if v.Type == spec.AdhocVariable && datasources[v.Datasource] {
			queue = append(queue, v.Name)
		}
	}
	out := map[string]string{}
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		v, ok := declared[name]
		if !ok {
			continue
		}
		if _, seen := out[name]; seen {
			continue
		}
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, false
		}
		out[name] = string(raw)
		queue = append(queue, variableNames(v.Query)...)
		queue = append(queue, variableNames(v.Regex)...)
	}
	return out, true
}

func variableNames(text string) []string {
	refs, err := spec.ScanQuery(text)
	if err != nil {
		return nil
	}
	var names []string
	for _, ref := range spec.FlattenReferences(refs) {
		if ref.Kind == spec.VariableRef {
			names = append(names, ref.Name)
		}
	}
	return names
}
