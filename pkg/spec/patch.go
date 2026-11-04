package spec

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
)

type PanelPlacement struct {
	ID      string   `json:"id"`
	GridPos *GridPos `json:"gridPos,omitempty"`
	Row     *string  `json:"row,omitempty"`
}

type DashboardFields struct {
	Time  json.RawMessage `json:"time,omitempty"`
	Title json.RawMessage `json:"title,omitempty"`
	Tags  json.RawMessage `json:"tags,omitempty"`
	Links json.RawMessage `json:"links,omitempty"`
}

type PanelPatch struct {
	Upsert    []json.RawMessage `json:"upsert,omitempty"`
	Delete    []string          `json:"delete,omitempty"`
	Layout    []PanelPlacement  `json:"layout,omitempty"`
	Variables json.RawMessage   `json:"variables,omitempty"`
	Dashboard *DashboardFields  `json:"dashboard,omitempty"`
}

type PatchError struct {
	Issues   []Issue
	PanelIDs map[string]string
}

func (e *PatchError) Error() string {
	return (&ValidationError{Issues: e.Issues}).Error()
}

type panelEdit struct {
	id   string
	body map[string]any
}

func ApplyPanelPatch(d *Dashboard, patch PanelPatch) (*Dashboard, error) {
	edits, issues := patchShape(patch)
	if len(issues) > 0 {
		return nil, newPatchError(issues, nil)
	}
	root, ok := decodeTree(d).(map[string]any)
	if !ok {
		return nil, errors.New("spec: dashboard is not an object")
	}
	t := &panelTree{root: root}
	phases := []func() []Issue{
		func() []Issue { return t.deletePanels(patch.Delete) },
		func() []Issue { return t.upsertPanels(edits) },
		func() []Issue { return t.placePanels(patch.Layout) },
		func() []Issue { return t.setVariables(patch.Variables) },
		func() []Issue { return t.setDashboardFields(patch.Dashboard) },
	}
	for _, phase := range phases {
		if issues := phase(); len(issues) > 0 {
			return nil, newPatchError(issues, nil)
		}
	}
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(root); err != nil {
		return nil, err
	}
	merged, err := Parse(out.Bytes())
	if err != nil {
		var ve *ValidationError
		if errors.As(err, &ve) {
			return nil, &PatchError{Issues: ve.Issues, PanelIDs: t.panelIDs()}
		}
		return nil, err
	}
	return merged, nil
}

func newPatchError(issues []Issue, ids map[string]string) *PatchError {
	return &PatchError{Issues: newValidationError(issues).Issues, PanelIDs: ids}
}

func patchIssue(path, format string, args ...any) Issue {
	return Issue{Path: path, Code: CodeSchema, Message: fmt.Sprintf(format, args...)}
}

func (f *DashboardFields) sets() bool {
	return f != nil && (len(f.Time) > 0 || len(f.Title) > 0 || len(f.Tags) > 0 || len(f.Links) > 0)
}

func patchShape(p PanelPatch) ([]panelEdit, []Issue) {
	if len(p.Upsert) == 0 && len(p.Delete) == 0 && len(p.Layout) == 0 && len(p.Variables) == 0 && !p.Dashboard.sets() {
		return nil, []Issue{patchIssue("", "the patch changes nothing; send at least one of upsert, delete, layout, variables or dashboard with a value")}
	}
	var issues []Issue
	edits := make([]panelEdit, 0, len(p.Upsert))
	for i, raw := range p.Upsert {
		path := "/upsert/" + strconv.Itoa(i)
		value, decodeIssues := decodeFragment(raw, path)
		if len(decodeIssues) > 0 {
			issues = append(issues, decodeIssues...)
			continue
		}
		body, isObject := value.(map[string]any)
		id, _ := body["id"].(string)
		switch {
		case !isObject:
			issues = append(issues, patchIssue(path, "a panel must be a JSON object"))
		case id == "":
			issues = append(issues, patchIssue(path+"/id", "every upserted panel needs a non-empty string id"))
		default:
			edits = append(edits, panelEdit{id: id, body: body})
		}
	}
	upserted := make([]string, 0, len(edits))
	for _, e := range edits {
		upserted = append(upserted, e.id)
	}
	issues = append(issues, duplicateIDs("/upsert", upserted)...)
	issues = append(issues, duplicateIDs("/delete", p.Delete)...)
	placed := make([]string, len(p.Layout))
	for i, pl := range p.Layout {
		placed[i] = pl.ID
	}
	issues = append(issues, duplicateIDs("/layout", placed)...)
	for i, id := range p.Delete {
		if slices.Contains(upserted, id) {
			issues = append(issues, patchIssue("/delete/"+strconv.Itoa(i), "panel %q is both upserted and deleted", id))
		}
	}
	return edits, issues
}

func duplicateIDs(base string, ids []string) []Issue {
	var issues []Issue
	seen := make(map[string]bool, len(ids))
	for i, id := range ids {
		if seen[id] {
			issues = append(issues, patchIssue(base+"/"+strconv.Itoa(i), "panel %q appears more than once in %s", id, base[1:]))
		}
		seen[id] = true
	}
	return issues
}

func decodeFragment(raw json.RawMessage, path string) (any, []Issue) {
	if issues := byteIssues(raw, 1); len(issues) > 0 {
		for i := range issues {
			issues[i].Path = path + issues[i].Path
		}
		return nil, issues
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, []Issue{patchIssue(path, "the value is not valid JSON")}
	}
	return value, nil
}

type panelLoc struct {
	row int
	idx int
}

const topLevel = -1

type panelTree struct {
	root map[string]any
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func panelID(m map[string]any) string {
	id, _ := m["id"].(string)
	return id
}

func isRow(m map[string]any) bool {
	t, _ := m["type"].(string)
	return t == string(Row)
}

func children(m map[string]any) []any {
	c, _ := m["panels"].([]any)
	return c
}

func (t *panelTree) top() []any {
	return children(t.root)
}

func (t *panelTree) list(row int) []any {
	if row == topLevel {
		return t.top()
	}
	return children(asMap(t.top()[row]))
}

func (t *panelTree) setList(row int, items []any) {
	owner := t.root
	if row != topLevel {
		owner = asMap(t.top()[row])
	}
	if len(items) == 0 && row != topLevel {
		delete(owner, "panels")
		return
	}
	owner["panels"] = items
}

func (t *panelTree) find(id string) (panelLoc, bool) {
	for i, p := range t.top() {
		m := asMap(p)
		if panelID(m) == id {
			return panelLoc{row: topLevel, idx: i}, true
		}
		for j, c := range children(m) {
			if panelID(asMap(c)) == id {
				return panelLoc{row: i, idx: j}, true
			}
		}
	}
	return panelLoc{}, false
}

func (t *panelTree) at(l panelLoc) map[string]any {
	return asMap(t.list(l.row)[l.idx])
}

func (t *panelTree) replace(l panelLoc, panel map[string]any) {
	t.list(l.row)[l.idx] = panel
}

func (t *panelTree) remove(l panelLoc) {
	t.setList(l.row, slices.Delete(t.list(l.row), l.idx, l.idx+1))
}

func (t *panelTree) append(row int, panel map[string]any) {
	t.setList(row, append(t.list(row), panel))
}

func (t *panelTree) panelIDs() map[string]string {
	ids := map[string]string{}
	for i, p := range t.top() {
		m := asMap(p)
		ids[pointer("panels", strconv.Itoa(i))] = panelID(m)
		for j, c := range children(m) {
			ids[pointer("panels", strconv.Itoa(i), "panels", strconv.Itoa(j))] = panelID(asMap(c))
		}
	}
	return ids
}

func (t *panelTree) deletePanels(ids []string) []Issue {
	var issues []Issue
	for i, id := range ids {
		path := "/delete/" + strconv.Itoa(i)
		l, ok := t.find(id)
		if !ok {
			issues = append(issues, patchIssue(path, "no panel with id %q to delete", id))
			continue
		}
		if l.row != topLevel || !isRow(t.at(l)) {
			continue
		}
		for _, c := range children(t.at(l)) {
			if child := panelID(asMap(c)); !slices.Contains(ids, child) {
				issues = append(issues, patchIssue(path, "row %q holds panel %q; delete every child of a row in the same call", id, child))
			}
		}
	}
	if len(issues) > 0 {
		return issues
	}
	for _, id := range ids {
		if l, ok := t.find(id); ok {
			t.remove(l)
		}
	}
	return nil
}

func (t *panelTree) upsertPanels(edits []panelEdit) []Issue {
	var issues []Issue
	for i, e := range edits {
		path := "/upsert/" + strconv.Itoa(i)
		l, exists := t.find(e.id)
		var current map[string]any
		if exists {
			current = t.at(l)
		}
		_, carriesChildren := e.body["panels"]
		if carriesChildren && exists && isRow(current) && len(children(current)) > 0 {
			issues = append(issues, patchIssue(path+"/panels", "row %q already has panels; upsert each child by its own id, or place new ones in the row with layout", e.id))
			continue
		}
		if carriesChildren && !isRow(e.body) {
			issues = append(issues, patchIssue(path+"/panels", "panel %q is not a row and cannot carry panels", e.id))
			continue
		}
		if !exists {
			t.append(topLevel, e.body)
			continue
		}
		if _, ok := e.body["gridPos"]; !ok {
			if gridPos, ok := current["gridPos"]; ok {
				e.body["gridPos"] = gridPos
			}
		}
		if kept, ok := current["panels"]; ok && !carriesChildren {
			e.body["panels"] = kept
		}
		t.replace(l, e.body)
	}
	return issues
}

func (t *panelTree) placePanels(layout []PanelPlacement) []Issue {
	var issues []Issue
	for i, pl := range layout {
		path := "/layout/" + strconv.Itoa(i)
		l, ok := t.find(pl.ID)
		if !ok {
			issues = append(issues, patchIssue(path, "no panel with id %q to place", pl.ID))
			continue
		}
		if pl.GridPos != nil {
			t.at(l)["gridPos"] = map[string]any{"x": pl.GridPos.X, "y": pl.GridPos.Y, "w": pl.GridPos.W, "h": pl.GridPos.H}
		}
		if pl.Row != nil {
			issues = append(issues, t.move(path+"/row", l, *pl.Row)...)
		}
	}
	return issues
}

func (t *panelTree) move(path string, from panelLoc, rowID string) []Issue {
	panel := t.at(from)
	if rowID == "" {
		if from.row != topLevel {
			t.remove(from)
			t.append(topLevel, panel)
		}
		return nil
	}
	target, ok := t.find(rowID)
	switch {
	case !ok:
		return []Issue{patchIssue(path, "no row with id %q to place the panel in", rowID)}
	case target.row != topLevel || !isRow(t.at(target)):
		return []Issue{patchIssue(path, "panel %q is not a row", rowID)}
	case isRow(panel):
		return []Issue{patchIssue(path, "a row cannot be placed inside another row")}
	case from.row == target.idx:
		return nil
	}
	t.remove(from)
	target, _ = t.find(rowID)
	t.append(target.idx, panel)
	return nil
}

func (t *panelTree) setVariables(raw json.RawMessage) []Issue {
	if len(raw) == 0 {
		return nil
	}
	value, issues := decodeFragment(raw, "/variables")
	if len(issues) > 0 {
		return issues
	}
	t.root["variables"] = value
	return nil
}

func (t *panelTree) setDashboardFields(f *DashboardFields) []Issue {
	if f == nil {
		return nil
	}
	var issues []Issue
	for _, field := range []struct {
		key string
		raw json.RawMessage
	}{{"time", f.Time}, {"title", f.Title}, {"tags", f.Tags}, {"links", f.Links}} {
		if len(field.raw) == 0 {
			continue
		}
		value, fieldIssues := decodeFragment(field.raw, "/dashboard/"+field.key)
		issues = append(issues, fieldIssues...)
		if len(fieldIssues) == 0 {
			t.root[field.key] = value
		}
	}
	return issues
}
