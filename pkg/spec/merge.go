package spec

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
)

type UnitKind string

const (
	UnitPanel    UnitKind = "panel"
	UnitVariable UnitKind = "variable"
	UnitSetting  UnitKind = "setting"
)

type ConflictReason string

const (
	ReasonBothChanged       ConflictReason = "both_changed"
	ReasonChangedAndRemoved ConflictReason = "changed_and_removed"
	ReasonInvalid           ConflictReason = "invalid"
)

type Conflict struct {
	Kind    UnitKind       `json:"kind"`
	ID      string         `json:"id"`
	Reason  ConflictReason `json:"reason"`
	Message string         `json:"message"`
}

type Side string

const (
	SideOurs   Side = "ours"
	SideTheirs Side = "theirs"
)

type Choice struct {
	Kind UnitKind `json:"kind"`
	ID   string   `json:"id"`
	Take Side     `json:"take"`
}

type Merge struct {
	Dashboard *Dashboard
	Conflicts []Conflict
}

var ErrUnknownChoice = errors.New("spec: choice names no panel, variable or setting")

type ChangeKind string

const (
	ChangeAdded   ChangeKind = "added"
	ChangeChanged ChangeKind = "changed"
	ChangeRemoved ChangeKind = "removed"
	ChangeMoved   ChangeKind = "moved"
)

type UnitChange struct {
	Kind   UnitKind   `json:"kind"`
	ID     string     `json:"id"`
	Change ChangeKind `json:"change"`
}

type unitRef struct {
	kind UnitKind
	id   string
}

type unitContent struct {
	present bool
	v       any
	key     string
}

func presentValue(v any) unitContent {
	return unitContent{present: true, v: v, key: string(raw(v))}
}

func (u unitContent) same(o unitContent) bool {
	return u.present == o.present && u.key == o.key
}

const (
	fieldResponsive = 4
	fieldContainer  = 5
)

type placement [6]any

func (p placement) differsAt(o placement, i int) bool {
	return !bytes.Equal(raw(p[i]), raw(o[i]))
}

func (p placement) same(o placement) bool {
	for i := range p {
		if p.differsAt(o, i) {
			return false
		}
	}
	return true
}

func (p placement) container() string {
	c, _ := p[fieldContainer].(string)
	return c
}

func mergePlacement(base, ours, theirs placement) placement {
	out := ours
	for i := range out {
		if theirs.differsAt(base, i) {
			out[i] = theirs[i]
		}
	}
	return out
}

type panelUnit struct {
	content unitContent
	place   placement
}

type docIndex struct {
	panels        map[string]panelUnit
	panelIDs      []string
	lists         map[string][]string
	variables     map[string]unitContent
	variableNames []string
	settings      map[string]unitContent
	settingNames  []string
}

func newDocIndex(d *Dashboard) *docIndex {
	root := asMap(decodeTree(d))
	ix := &docIndex{
		panels:    map[string]panelUnit{},
		lists:     map[string][]string{},
		variables: map[string]unitContent{},
		settings:  map[string]unitContent{},
	}
	for _, p := range children(root) {
		row := asMap(p)
		ix.addPanel("", row)
		for _, c := range children(row) {
			ix.addPanel(panelID(row), asMap(c))
		}
	}
	vars, _ := root["variables"].([]any)
	for _, v := range vars {
		name, _ := asMap(v)["name"].(string)
		ix.variables[name] = presentValue(v)
		ix.variableNames = append(ix.variableNames, name)
	}
	for name, v := range root {
		if name != "panels" && name != "variables" {
			ix.settings[name] = presentValue(v)
			ix.settingNames = append(ix.settingNames, name)
		}
	}
	slices.Sort(ix.settingNames)
	return ix
}

func (ix *docIndex) addPanel(container string, p map[string]any) {
	id := panelID(p)
	grid := asMap(p["gridPos"])
	content := make(map[string]any, len(p))
	for k, v := range p {
		if k != "gridPos" && k != "responsive" && k != "panels" {
			content[k] = v
		}
	}
	ix.panels[id] = panelUnit{
		content: presentValue(content),
		place:   placement{grid["x"], grid["y"], grid["w"], grid["h"], p["responsive"], container},
	}
	ix.panelIDs = append(ix.panelIDs, id)
	ix.lists[container] = append(ix.lists[container], id)
}

func (ix *docIndex) value(kind UnitKind, id string) unitContent {
	switch kind {
	case UnitPanel:
		return ix.panels[id].content
	case UnitVariable:
		return ix.variables[id]
	case UnitSetting:
		return ix.settings[id]
	}
	return unitContent{}
}

func (ix *docIndex) has(kind UnitKind, id string) bool {
	return ix.value(kind, id).present
}

func orderedUnion(lists ...[]string) []string {
	var out []string
	seen := map[string]bool{}
	for _, list := range lists {
		for _, id := range list {
			if !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
	}
	return out
}

func keepPresent[V any](ids []string, set map[string]V) []string {
	return slices.DeleteFunc(slices.Clone(ids), func(id string) bool {
		_, ok := set[id]
		return !ok
	})
}

func mergeContent(base, ours, theirs unitContent) (unitContent, ConflictReason) {
	switch {
	case ours.same(theirs):
		return ours, ""
	case ours.same(base):
		return theirs, ""
	case theirs.same(base):
		return ours, ""
	case ours.present && theirs.present:
		return unitContent{}, ReasonBothChanged
	}
	return unitContent{}, ReasonChangedAndRemoved
}

func restrictTo(seq, to []string) []string {
	return slices.DeleteFunc(slices.Clone(seq), func(id string) bool { return !slices.Contains(to, id) })
}

func mergeOrder(base, ours, theirs, wanted []string) []string {
	chosen, other := ours, theirs
	if slices.Equal(restrictTo(ours, base), restrictTo(base, ours)) {
		chosen, other = theirs, ours
	}
	want := make(map[string]bool, len(wanted))
	for _, id := range wanted {
		want[id] = true
	}
	out := make([]string, 0, len(wanted))
	for _, id := range orderedUnion(chosen, other, wanted) {
		if want[id] {
			out = append(out, id)
		}
	}
	return out
}

type merger struct {
	base, ours, theirs *docIndex
	choices            map[unitRef]Side
	conflicts          []Conflict
	panelIDs           []string
	variableNames      []string
	settingNames       []string
	rank               map[unitRef]int
}

func newMerger(base, ours, theirs *Dashboard) *merger {
	m := &merger{
		base:      newDocIndex(base),
		ours:      newDocIndex(ours),
		theirs:    newDocIndex(theirs),
		choices:   map[unitRef]Side{},
		conflicts: []Conflict{},
		rank:      map[unitRef]int{},
	}
	m.panelIDs = orderedUnion(m.ours.panelIDs, m.theirs.panelIDs, m.base.panelIDs)
	m.variableNames = orderedUnion(m.ours.variableNames, m.theirs.variableNames, m.base.variableNames)
	m.settingNames = orderedUnion(m.ours.settingNames, m.theirs.settingNames, m.base.settingNames)
	for kind, ids := range map[UnitKind][]string{UnitPanel: m.panelIDs, UnitVariable: m.variableNames, UnitSetting: m.settingNames} {
		for i, id := range ids {
			m.rank[unitRef{kind, id}] = i
		}
	}
	return m
}

func (m *merger) side(s Side) *docIndex {
	if s == SideTheirs {
		return m.theirs
	}
	return m.ours
}

func (m *merger) choose(choices []Choice) error {
	for _, c := range choices {
		if c.Take != SideOurs && c.Take != SideTheirs {
			return fmt.Errorf("spec: choice for %s %q takes unknown side %q", c.Kind, c.ID, c.Take)
		}
		if !m.base.has(c.Kind, c.ID) && !m.ours.has(c.Kind, c.ID) && !m.theirs.has(c.Kind, c.ID) {
			return fmt.Errorf("%w: %s %q", ErrUnknownChoice, c.Kind, c.ID)
		}
		m.choices[unitRef{c.Kind, c.ID}] = c.Take
	}
	return nil
}

func (m *merger) conflict(kind UnitKind, id string, reason ConflictReason, message string) {
	m.conflicts = append(m.conflicts, Conflict{Kind: kind, ID: id, Reason: reason, Message: message})
}

func conflictMessage(kind UnitKind, id string, reason ConflictReason) string {
	if reason == ReasonBothChanged {
		return fmt.Sprintf("%s %q was changed on both sides", kind, id)
	}
	return fmt.Sprintf("%s %q was removed on one side and changed on the other", kind, id)
}

func (m *merger) resolve(kind UnitKind, id string) (unitContent, bool) {
	if s, ok := m.choices[unitRef{kind, id}]; ok {
		return m.side(s).value(kind, id), true
	}
	merged, reason := mergeContent(m.base.value(kind, id), m.ours.value(kind, id), m.theirs.value(kind, id))
	if reason != "" {
		m.conflict(kind, id, reason, conflictMessage(kind, id, reason))
		return unitContent{}, false
	}
	return merged, true
}

func (m *merger) mergeUnits(kind UnitKind, ids []string) map[string]unitContent {
	out := map[string]unitContent{}
	for _, id := range ids {
		if v, ok := m.resolve(kind, id); ok && v.present {
			out[id] = v
		}
	}
	return out
}

func (m *merger) placementOf(id string) placement {
	if s, ok := m.choices[unitRef{UnitPanel, id}]; ok {
		return m.side(s).panels[id].place
	}
	base, ours, theirs := m.base.panels[id], m.ours.panels[id], m.theirs.panels[id]
	switch {
	case !theirs.content.present:
		return ours.place
	case !ours.content.present, !base.content.present:
		return theirs.place
	}
	return mergePlacement(base.place, ours.place, theirs.place)
}

func (m *merger) mergePanels() map[string]panelUnit {
	contents := m.mergeUnits(UnitPanel, m.panelIDs)
	panels := make(map[string]panelUnit, len(contents))
	for id, content := range contents {
		panels[id] = panelUnit{content: content, place: m.placementOf(id)}
	}
	var orphans []string
	for _, id := range m.panelIDs {
		p, ok := panels[id]
		if !ok || p.place.container() == "" {
			continue
		}
		if _, hasRow := panels[p.place.container()]; !hasRow {
			orphans = append(orphans, id)
			m.conflict(UnitPanel, id, ReasonChangedAndRemoved, fmt.Sprintf("panel %q is in row %q, which was removed", id, p.place.container()))
		}
	}
	for _, id := range orphans {
		delete(panels, id)
	}
	return panels
}

func (m *merger) touched(panels map[string]panelUnit) map[string]bool {
	touched := map[string]bool{}
	for id, p := range panels {
		o := m.ours.panels[id]
		if !o.content.same(p.content) || !o.place.same(p.place) {
			touched[id] = true
		}
	}
	return touched
}

func (m *merger) panelObjects(container string, panels map[string]panelUnit) []any {
	var wanted []string
	for _, id := range m.panelIDs {
		if p, ok := panels[id]; ok && p.place.container() == container {
			wanted = append(wanted, id)
		}
	}
	order := mergeOrder(m.base.lists[container], m.ours.lists[container], m.theirs.lists[container], wanted)
	out := make([]any, 0, len(order))
	for _, id := range order {
		var kids []any
		if container == "" {
			kids = m.panelObjects(id, panels)
		}
		out = append(out, panelObject(panels[id], kids))
	}
	return out
}

func panelObject(p panelUnit, kids []any) map[string]any {
	obj := maps.Clone(asMap(p.content.v))
	obj["gridPos"] = map[string]any{"x": p.place[0], "y": p.place[1], "w": p.place[2], "h": p.place[3]}
	if p.place[fieldResponsive] != nil {
		obj["responsive"] = p.place[fieldResponsive]
	}
	if len(kids) > 0 {
		obj["panels"] = kids
	}
	return obj
}

func (m *merger) assemble(settings, variables map[string]unitContent, panels map[string]panelUnit) map[string]any {
	root := make(map[string]any, len(settings)+2)
	for name, v := range settings {
		root[name] = v.v
	}
	names := mergeOrder(m.base.variableNames, m.ours.variableNames, m.theirs.variableNames, keepPresent(m.variableNames, variables))
	if len(names) > 0 {
		list := make([]any, 0, len(names))
		for _, name := range names {
			list = append(list, variables[name].v)
		}
		root["variables"] = list
	}
	root["panels"] = m.panelObjects("", panels)
	return root
}

func (m *merger) sortedConflicts() []Conflict {
	kindRank := map[UnitKind]int{UnitPanel: 0, UnitVariable: 1, UnitSetting: 2}
	rankOf := func(c Conflict) int {
		if r, ok := m.rank[unitRef{c.Kind, c.ID}]; ok {
			return r
		}
		return math.MaxInt
	}
	slices.SortStableFunc(m.conflicts, func(a, b Conflict) int {
		if a.Kind != b.Kind {
			return kindRank[a.Kind] - kindRank[b.Kind]
		}
		ra, rb := rankOf(a), rankOf(b)
		switch {
		case ra < rb:
			return -1
		case ra > rb:
			return 1
		}
		return 0
	})
	return m.conflicts
}

func (m *merger) validate(root map[string]any, touched map[string]bool) (Merge, error) {
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(root); err != nil {
		return Merge{}, err
	}
	merged, err := parse(out.Bytes(), touched)
	var ve *ValidationError
	switch {
	case errors.As(err, &ve):
		seen := map[unitRef]bool{}
		for _, issue := range ve.Issues {
			c := conflictForIssue(issue, root)
			if ref := (unitRef{c.Kind, c.ID}); !seen[ref] {
				seen[ref] = true
				m.conflicts = append(m.conflicts, c)
			}
		}
		return Merge{Conflicts: m.sortedConflicts()}, nil
	case err != nil:
		return Merge{}, err
	}
	return Merge{Dashboard: merged, Conflicts: m.conflicts}, nil
}

func conflictForIssue(issue Issue, root map[string]any) Conflict {
	kind, id := issueUnit(issue.Path, root)
	return Conflict{Kind: kind, ID: id, Reason: ReasonInvalid, Message: fmt.Sprintf("%s %q: %s", kind, id, issue.Message)}
}

func issueUnit(path string, root map[string]any) (UnitKind, string) {
	if path == "" {
		return UnitSetting, ""
	}
	tokens := pathTokens(path)
	for i, t := range tokens {
		tokens[i] = pointerUnescaper.Replace(t)
	}
	switch {
	case tokens[0] == "panels" && len(tokens) >= 2:
		at := tokens[:2]
		if len(tokens) >= 4 && tokens[2] == "panels" {
			at = tokens[:4]
		}
		if id, ok := (&panelTree{root: root}).panelIDs()[pointer(at...)]; ok {
			return UnitPanel, id
		}
	case tokens[0] == "variables" && len(tokens) >= 2:
		vars, _ := root["variables"].([]any)
		if i, err := strconv.Atoi(tokens[1]); err == nil && i >= 0 && i < len(vars) {
			name, _ := asMap(vars[i])["name"].(string)
			return UnitVariable, name
		}
	}
	return UnitSetting, tokens[0]
}

func Merge3(base, ours, theirs *Dashboard, choices ...Choice) (Merge, error) {
	m := newMerger(base, ours, theirs)
	if err := m.choose(choices); err != nil {
		return Merge{}, err
	}
	settings := m.mergeUnits(UnitSetting, m.settingNames)
	variables := m.mergeUnits(UnitVariable, m.variableNames)
	panels := m.mergePanels()
	if len(m.conflicts) > 0 {
		return Merge{Conflicts: m.sortedConflicts()}, nil
	}
	return m.validate(m.assemble(settings, variables, panels), m.touched(panels))
}

func unitChange(before, after unitContent) (ChangeKind, bool) {
	switch {
	case !before.present && !after.present:
		return "", false
	case !before.present:
		return ChangeAdded, true
	case !after.present:
		return ChangeRemoved, true
	case !before.same(after):
		return ChangeChanged, true
	}
	return "", false
}

func Changes(from, to *Dashboard) []UnitChange {
	a, b := newDocIndex(from), newDocIndex(to)
	out := []UnitChange{}
	add := func(kind UnitKind, ids []string, change func(id string) (ChangeKind, bool)) {
		for _, id := range ids {
			if c, ok := change(id); ok {
				out = append(out, UnitChange{Kind: kind, ID: id, Change: c})
			}
		}
	}
	add(UnitPanel, orderedUnion(b.panelIDs, a.panelIDs), func(id string) (ChangeKind, bool) {
		before, after := a.panels[id], b.panels[id]
		if c, ok := unitChange(before.content, after.content); ok {
			return c, true
		}
		if before.content.present && !before.place.same(after.place) {
			return ChangeMoved, true
		}
		return "", false
	})
	commonFrom, commonTo := restrictTo(a.variableNames, b.variableNames), restrictTo(b.variableNames, a.variableNames)
	add(UnitVariable, orderedUnion(b.variableNames, a.variableNames), func(id string) (ChangeKind, bool) {
		before, after := a.variables[id], b.variables[id]
		if c, ok := unitChange(before, after); ok {
			return c, true
		}
		if before.present && slices.Index(commonFrom, id) != slices.Index(commonTo, id) {
			return ChangeMoved, true
		}
		return "", false
	})
	names := orderedUnion(a.settingNames, b.settingNames)
	slices.Sort(names)
	add(UnitSetting, names, func(id string) (ChangeKind, bool) {
		return unitChange(a.settings[id], b.settings[id])
	})
	return out
}
