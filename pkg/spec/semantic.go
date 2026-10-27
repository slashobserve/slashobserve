package spec

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

func checkSemantics(d *Dashboard) []Issue {
	var issues []Issue
	entries := d.AllPanels()
	issues = append(issues, checkPanelCount(d)...)
	issues = append(issues, checkDuplicatePanelIDs(entries)...)
	issues = append(issues, checkDuplicateVariableNames(d.Variables)...)
	issues = append(issues, checkVariableDefinitions(d.Variables)...)
	issues = append(issues, checkGridOverflow(entries)...)
	issues = append(issues, checkGridOverlap(d)...)
	issues = append(issues, checkTime(d.Time)...)
	issues = append(issues, checkThresholds(entries)...)
	issues = append(issues, checkMinMax(entries)...)
	issues = append(issues, checkQueryRefs(entries)...)
	issues = append(issues, checkOverrideMembers(entries)...)
	issues = append(issues, checkRepeat(d, entries)...)
	issues = append(issues, checkVariableQueries(d.Variables)...)
	issues = append(issues, checkPanelQueries(d, entries)...)
	issues = append(issues, checkCanvas(entries)...)
	return issues
}

func checkCanvas(entries []PanelEntry) []Issue {
	var issues []Issue
	for _, e := range entries {
		if e.Panel.Type != Canvas || e.Panel.Display == nil {
			continue
		}
		base := e.Pointer + "/display"
		ids := map[string]bool{}
		for i, el := range e.Panel.Display.Elements {
			if ids[el.ID] {
				issues = append(issues, Issue{
					Path:    base + "/elements/" + strconv.Itoa(i) + "/id",
					Code:    CodeDuplicateElementID,
					Message: fmt.Sprintf("canvas element id %q is already used in this panel", el.ID),
				})
			}
			ids[el.ID] = true
		}
		for i, c := range e.Panel.Display.Connections {
			for _, end := range []struct{ name, id string }{{"from", c.From}, {"to", c.To}} {
				if ids[end.id] {
					continue
				}
				issues = append(issues, Issue{
					Path:    base + "/connections/" + strconv.Itoa(i) + "/" + end.name,
					Code:    CodeUnknownElement,
					Message: fmt.Sprintf("connection %s names no canvas element %q", end.name, end.id),
				})
			}
		}
	}
	return issues
}

func checkPanelCount(d *Dashboard) []Issue {
	n := d.PanelCount()
	if n <= MaxPanels {
		return nil
	}
	return []Issue{{
		Path:    pointer("panels"),
		Code:    CodeTooManyPanels,
		Message: fmt.Sprintf("the dashboard has %d panels including row children; the limit is %d", n, MaxPanels),
	}}
}

func checkDuplicatePanelIDs(entries []PanelEntry) []Issue {
	var issues []Issue
	first := map[string]string{}
	for _, e := range entries {
		at, dup := first[e.Panel.ID]
		if !dup {
			first[e.Panel.ID] = e.Pointer
			continue
		}
		issues = append(issues, Issue{
			Path:    e.Pointer + "/id",
			Code:    CodeDuplicatePanelID,
			Message: fmt.Sprintf("panel id %q is already used by %s", e.Panel.ID, at),
		})
	}
	return issues
}

func checkDuplicateVariableNames(vars []Variable) []Issue {
	var issues []Issue
	first := map[string]int{}
	for j, v := range vars {
		i, dup := first[v.Name]
		if !dup {
			first[v.Name] = j
			continue
		}
		issues = append(issues, Issue{
			Path:    pointer("variables", strconv.Itoa(j), "name"),
			Code:    CodeDuplicateVariableName,
			Message: fmt.Sprintf("variable name %q is already used by %s", v.Name, pointer("variables", strconv.Itoa(i))),
		})
	}
	return issues
}

func checkVariableDefinitions(vars []Variable) []Issue {
	var issues []Issue
	for i, v := range vars {
		if v.Regex != "" {
			if _, err := CompileVariableRegex(v.Regex); err != nil {
				issues = append(issues, Issue{
					Path:    pointer("variables", strconv.Itoa(i), "regex"),
					Code:    CodeInvalidRegex,
					Message: fmt.Sprintf("variable %q: %s", v.Name, err),
				})
			}
		}
		if message := invalidDefault(v); message != "" {
			issues = append(issues, Issue{
				Path:    pointer("variables", strconv.Itoa(i), "default"),
				Code:    CodeInvalidDefault,
				Message: message,
			})
		}
	}
	return issues
}

func invalidDefault(v Variable) string {
	if v.Default == nil {
		return ""
	}
	switch v.Type {
	case QueryVariable, CustomVariable:
		return invalidListDefault(v)
	case IntervalVariable:
		for _, value := range v.Default.Values {
			if !slices.Contains(v.Options, value) {
				return fmt.Sprintf("default %q of variable %q is not one of its options", value, v.Name)
			}
		}
	}
	return ""
}

func invalidListDefault(v Variable) string {
	switch {
	case v.Default.List && !v.Multi:
		return fmt.Sprintf("variable %q has an array default but is not multi; give one value or set multi", v.Name)
	case slices.Contains(v.Default.Values, SelectAll) && !v.IncludeAll:
		return fmt.Sprintf("variable %q defaults to %s but does not set includeAll", v.Name, SelectAll)
	case slices.Contains(v.Default.Values, SelectAll) && len(v.Default.Values) > 1:
		return fmt.Sprintf("variable %q combines %s with other values in its default; give %s alone or list the values", v.Name, SelectAll, SelectAll)
	case v.Type == CustomVariable:
		for _, value := range v.Default.Values {
			if value != SelectAll && !slices.Contains(v.Options, value) {
				return fmt.Sprintf("default %q of variable %q is not one of its options", value, v.Name)
			}
		}
	}
	return ""
}

func checkGridOverflow(entries []PanelEntry) []Issue {
	var issues []Issue
	for _, e := range entries {
		g := e.Panel.GridPos
		if g.X+g.W > GridColumns {
			issues = append(issues, Issue{
				Path:    e.Pointer + "/gridPos",
				Code:    CodeGridOverflow,
				Message: fmt.Sprintf("gridPos x %d + w %d = %d exceeds the %d-column grid; reduce x or w", g.X, g.W, g.X+g.W, GridColumns),
			})
		}
	}
	return issues
}

func gridsOverlap(a, b GridPos) bool {
	return a.X < b.X+b.W && b.X < a.X+a.W && a.Y < b.Y+b.H && b.Y < a.Y+a.H
}

func checkGridOverlap(d *Dashboard) []Issue {
	var layout []PanelEntry
	for _, e := range d.AllPanels() {
		if e.Row == nil || !e.Row.Collapsed {
			layout = append(layout, e)
		}
	}
	issues := overlaps(layout)
	for i := range d.Panels {
		row := &d.Panels[i]
		if row.Type != Row || !row.Collapsed {
			continue
		}
		var children []PanelEntry
		for j := range row.Panels {
			children = append(children, PanelEntry{
				Pointer: pointer("panels", strconv.Itoa(i), "panels", strconv.Itoa(j)),
				Panel:   &row.Panels[j],
				Row:     row,
			})
		}
		issues = append(issues, overlaps(children)...)
	}
	return issues
}

func overlaps(set []PanelEntry) []Issue {
	var issues []Issue
	for j, pj := range set {
		for i := range j {
			if !gridsOverlap(set[i].Panel.GridPos, pj.Panel.GridPos) {
				continue
			}
			issues = append(issues, Issue{
				Path:    pj.Pointer + "/gridPos",
				Code:    CodeGridOverlap,
				Message: fmt.Sprintf("panel %q overlaps panel %q at %s; move one of them to free grid cells", pj.Panel.ID, set[i].Panel.ID, set[i].Pointer),
			})
			break
		}
	}
	return issues
}

var columnPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$`)

func IsColumnArg(arg string) bool {
	return columnPattern.MatchString(arg)
}

var labelNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func IsLabelName(name string) bool {
	return labelNamePattern.MatchString(name)
}

var relativeTime = regexp.MustCompile(`^now(?:([+-])([0-9]+)([smhdwMy]))?(?:/([smhdwMy]))?$`)

type unitBounds struct{ min, max float64 }

var timeUnits = map[string]unitBounds{
	"s": {1, 1},
	"m": {60, 60},
	"h": {3600, 3600},
	"d": {86400, 86400},
	"w": {7 * 86400, 7 * 86400},
	"M": {28 * 86400, 31 * 86400},
	"y": {365 * 86400, 366 * 86400},
}

const maxOffsetYears = maxResolvedYear - minResolvedYear

const maxOffsetSeconds = float64(maxOffsetYears) * 366 * 86400

type timePoint struct {
	relative  bool
	agoMin    float64
	agoMax    float64
	roundMax  float64
	roundUnit string
	fixed     bool
	monthly   bool
	months    int64
	at        time.Time
}

func parseTimePoint(s string) (timePoint, bool) {
	if strings.HasPrefix(s, "now") {
		return parseRelativeTime(s)
	}
	at, err := time.Parse(time.RFC3339, s)
	if err != nil || at.IsZero() {
		return timePoint{}, false
	}
	if year := at.UTC().Year(); year < minResolvedYear || year > maxResolvedYear {
		return timePoint{}, false
	}
	return timePoint{at: at}, true
}

func parseRelativeTime(s string) (timePoint, bool) {
	m := relativeTime.FindStringSubmatch(s)
	if m == nil {
		return timePoint{}, false
	}
	p := timePoint{relative: true, fixed: true, monthly: true, roundUnit: m[4]}
	if m[2] != "" {
		n, err := strconv.ParseInt(m[2], 10, 32)
		if err != nil {
			return timePoint{}, false
		}
		u := timeUnits[m[3]]
		if offsetOutOfRange(n, m[3]) {
			return timePoint{}, false
		}
		p.agoMin, p.agoMax = float64(n)*u.min, float64(n)*u.max
		p.fixed = u.min == u.max
		p.monthly = m[3] == "M" || m[3] == "y"
		if p.monthly {
			p.months = n * monthsPerUnit[m[3]]
			if m[1] == "+" {
				p.months = -p.months
			}
		}
		if m[1] == "+" {
			p.agoMin, p.agoMax = -p.agoMax, -p.agoMin
		}
	}
	if m[4] != "" {
		p.roundMax = timeUnits[m[4]].max
	}
	return p, true
}

func offsetOutOfRange(n int64, unit string) bool {
	switch unit {
	case "y":
		return n > maxOffsetYears
	case "M":
		return n > 12*maxOffsetYears
	}
	return float64(n)*timeUnits[unit].min > maxOffsetSeconds
}

var monthsPerUnit = map[string]int64{"M": 1, "y": 12}

func relativeInOrder(from, to timePoint) bool {
	if inOrder, exact := fixedUnitOrder(from, to); exact {
		return inOrder
	}
	if inOrder, exact := monthOrder(from, to); exact {
		return inOrder
	}
	return from.agoMax+from.roundMax > to.agoMin-to.roundMax
}

func fixedUnitOrder(from, to timePoint) (inOrder, exact bool) {
	if from.roundUnit == "" || from.roundUnit != to.roundUnit || !from.fixed || !to.fixed {
		return false, false
	}
	bounds := timeUnits[from.roundUnit]
	if bounds.min != bounds.max || math.Mod(from.agoMax, bounds.max) != 0 || math.Mod(to.agoMax, bounds.max) != 0 {
		return false, false
	}
	return from.agoMax >= to.agoMax, true
}

func monthOrder(from, to timePoint) (inOrder, exact bool) {
	if !from.monthly || !to.monthly {
		return false, false
	}
	unit := from.roundUnit
	if unit == "" {
		unit = to.roundUnit
	}
	if from.roundUnit != "" && to.roundUnit != "" && from.roundUnit != to.roundUnit {
		return false, false
	}
	step := int64(1)
	if unit != "" {
		step = monthsPerUnit[unit]
		if step == 0 || from.months%step != 0 || to.months%step != 0 {
			return false, false
		}
	}
	unrounded := from.roundUnit == "" && to.roundUnit == ""
	if unrounded {
		return from.months > to.months, true
	}
	return from.months/step >= to.months/step, true
}

func checkTime(tr TimeRange) []Issue {
	var issues []Issue
	from, fromOK := parseTimePoint(tr.From)
	to, toOK := parseTimePoint(tr.To)
	for _, f := range []struct {
		name  string
		value string
		ok    bool
	}{{"from", tr.From, fromOK}, {"to", tr.To, toOK}} {
		if !f.ok {
			issues = append(issues, Issue{
				Path:    pointer("time", f.name),
				Code:    CodeSchema,
				Message: fmt.Sprintf("time.%s %q is not a valid time; use a relative time such as \"now-6h\" or a real RFC 3339 timestamp", f.name, f.value),
			})
		}
	}
	if !fromOK || !toOK || from.relative != to.relative {
		return issues
	}
	inOrder := from.at.Before(to.at)
	if from.relative {
		inOrder = relativeInOrder(from, to)
	}
	if !inOrder {
		issues = append(issues, Issue{
			Path:    pointer("time"),
			Code:    CodeTimeRangeOrder,
			Message: fmt.Sprintf("time.from %q is not earlier than time.to %q; swap them or move from further back", tr.From, tr.To),
		})
	}
	return issues
}

type fieldLocation struct {
	prefix string
	props  *FieldProps
}

func fieldLocations(entries []PanelEntry) []fieldLocation {
	var out []fieldLocation
	for _, e := range entries {
		if !e.Panel.Type.IsData() {
			continue
		}
		out = append(out, fieldLocation{prefix: e.Pointer, props: &e.Panel.FieldProps})
		for k := range e.Panel.Overrides {
			out = append(out, fieldLocation{
				prefix: e.Pointer + pointer("overrides", strconv.Itoa(k)),
				props:  &e.Panel.Overrides[k].FieldProps,
			})
		}
	}
	return out
}

func checkThresholds(entries []PanelEntry) []Issue {
	var issues []Issue
	for _, loc := range fieldLocations(entries) {
		var prev *float64
		for k, step := range loc.props.Thresholds {
			path := loc.prefix + pointer("thresholds", strconv.Itoa(k))
			if step.Value == nil {
				if k == 0 {
					continue
				}
				issues = append(issues, Issue{
					Path:    path,
					Code:    CodeThresholdValueMissing,
					Message: "only the first threshold may omit value",
				})
				break
			}
			if prev != nil && *step.Value <= *prev {
				issues = append(issues, Issue{
					Path:    path,
					Code:    CodeThresholdsUnordered,
					Message: fmt.Sprintf("threshold value %v must be greater than the previous value %v; list thresholds in strictly ascending order", *step.Value, *prev),
				})
				break
			}
			prev = step.Value
		}
	}
	return issues
}

func checkMinMax(entries []PanelEntry) []Issue {
	var issues []Issue
	for _, loc := range fieldLocations(entries) {
		p := loc.props
		if p.Min == nil || p.Max == nil || *p.Min < *p.Max {
			continue
		}
		issues = append(issues, Issue{
			Path:    loc.prefix + "/max",
			Code:    CodeMinMaxOrder,
			Message: fmt.Sprintf("max %v must be greater than min %v", *p.Max, *p.Min),
		})
	}
	return issues
}

func checkQueryRefs(entries []PanelEntry) []Issue {
	var issues []Issue
	for _, e := range entries {
		p := e.Panel
		if !p.Type.IsData() {
			continue
		}
		first := map[string]int{}
		for j := range p.Queries {
			ref := p.QueryRef(j)
			k, dup := first[ref]
			if !dup {
				first[ref] = j
				continue
			}
			path := e.Pointer + pointer("queries", strconv.Itoa(j))
			if p.Queries[j].Ref != "" {
				path += "/ref"
			}
			issues = append(issues, Issue{
				Path:    path,
				Code:    CodeDuplicateQueryRef,
				Message: fmt.Sprintf("query ref %q is already used by %s", ref, e.Pointer+pointer("queries", strconv.Itoa(k))),
			})
		}
		for k, o := range p.Overrides {
			if o.Match.Query == "" {
				continue
			}
			if _, ok := first[o.Match.Query]; ok {
				continue
			}
			issues = append(issues, Issue{
				Path:    e.Pointer + pointer("overrides", strconv.Itoa(k), "match", "query"),
				Code:    CodeUnknownQueryRef,
				Message: fmt.Sprintf("override matches query ref %q, which the panel does not have", o.Match.Query),
			})
		}
	}
	return issues
}

func checkOverrideMembers(entries []PanelEntry) []Issue {
	var issues []Issue
	for _, e := range entries {
		if !e.Panel.Type.IsData() {
			continue
		}
		for k, o := range e.Panel.Overrides {
			if storesMembers(o.FieldProps) {
				continue
			}
			for _, empty := range []struct {
				name string
				set  bool
			}{{"mappings", o.Mappings != nil}, {"links", o.Links != nil}} {
				if !empty.set {
					continue
				}
				issues = append(issues, Issue{
					Path:    e.Pointer + pointer("overrides", strconv.Itoa(k), empty.name),
					Code:    CodeSchema,
					Message: fmt.Sprintf("override sets only an empty %q array, which is not stored; add a field option or remove the override", empty.name),
				})
			}
		}
	}
	return issues
}

func storesMembers(p FieldProps) bool {
	b, err := json.Marshal(p)
	return err != nil || string(b) != "{}"
}

func checkRepeat(d *Dashboard, entries []PanelEntry) []Issue {
	declared := map[string]bool{}
	for _, v := range d.Variables {
		declared[v.Name] = true
	}
	var issues []Issue
	for _, e := range entries {
		if e.Panel.Repeat == "" || declared[e.Panel.Repeat] {
			continue
		}
		issues = append(issues, Issue{
			Path:    e.Pointer + "/repeat",
			Code:    CodeUnknownVariable,
			Message: fmt.Sprintf("repeat references undeclared variable %q; declare it in variables or remove repeat", e.Panel.Repeat),
		})
	}
	return issues
}

type refChecker struct {
	declared     map[string]int
	adhoc        map[string]bool
	fromVariable bool
}

func newRefChecker(vars []Variable) refChecker {
	declared := map[string]int{}
	adhoc := map[string]bool{}
	for i, v := range vars {
		if _, ok := declared[v.Name]; !ok {
			declared[v.Name] = i
			adhoc[v.Name] = v.Type == AdhocVariable
		}
	}
	return refChecker{declared: declared, adhoc: adhoc}
}

func checkVariableQueries(vars []Variable) []Issue {
	var issues []Issue
	rc := newRefChecker(vars)
	rc.fromVariable = true
	for i, v := range vars {
		if v.Type != QueryVariable {
			continue
		}
		if issue := rc.checkQuery(v.Query, i); issue != nil {
			issue.Path = pointer("variables", strconv.Itoa(i), "query")
			issues = append(issues, *issue)
		}
	}
	return issues
}

func checkPanelQueries(d *Dashboard, entries []PanelEntry) []Issue {
	var issues []Issue
	rc := newRefChecker(d.Variables)
	for _, e := range entries {
		if !e.Panel.Type.IsData() {
			continue
		}
		for j, q := range e.Panel.Queries {
			if issue := rc.checkQuery(q.Query, len(d.Variables)); issue != nil {
				issue.Path = e.Pointer + pointer("queries", strconv.Itoa(j), "query")
				issues = append(issues, *issue)
			}
		}
	}
	return issues
}

func (rc refChecker) checkQuery(query string, visible int) *Issue {
	refs, err := ScanQuery(query)
	if err != nil {
		return &Issue{Code: CodeInvalidReference, Message: err.Error()}
	}
	for _, ref := range FlattenReferences(refs) {
		var issue *Issue
		if ref.Kind == MacroRef {
			issue = checkMacroRef(ref)
		} else {
			issue = rc.checkVariableRef(ref, visible)
		}
		if issue != nil {
			return issue
		}
	}
	return nil
}

func (rc refChecker) checkVariableRef(ref Reference, visible int) *Issue {
	idx, ok := rc.declared[ref.Name]
	switch {
	case !ok:
		return &Issue{Code: CodeUnknownVariable, Message: fmt.Sprintf("query references undeclared variable %q; declare it in variables or remove the reference", ref.Name)}
	case idx >= visible:
		return &Issue{Code: CodeVariableOrder, Message: fmt.Sprintf("query references variable %q declared at %s; a variable query can only reference variables declared before it", ref.Name, pointer("variables", strconv.Itoa(idx)))}
	case rc.adhoc[ref.Name] && rc.fromVariable:
		return &Issue{Code: CodeInvalidReference, Message: fmt.Sprintf("a variable query cannot use adhoc variable %q", ref.Name)}
	case rc.adhoc[ref.Name] && ref.Format != "":
		return &Issue{Code: CodeInvalidReference, Message: fmt.Sprintf("adhoc variable %q takes no format", ref.Name)}
	case ref.Format != "" && !slices.Contains(VariableFormats(), ref.Format):
		return unknownFormatIssue(ref)
	}
	return nil
}

func unknownFormatIssue(ref Reference) *Issue {
	return &Issue{Code: CodeInvalidReference, Message: fmt.Sprintf("unknown format %q in reference to variable %q; use one of %s", ref.Format, ref.Name, strings.Join(VariableFormats(), ", "))}
}

func checkMacroRef(ref Reference) *Issue {
	macro, ok := LookupMacro(ref.Name)
	if !ok {
		return &Issue{Code: CodeUnknownMacro, Message: fmt.Sprintf("macro $%s does not exist", ref.Name)}
	}
	if ref.Format != "" {
		if err := macro.CheckFormat(ref.Format); err != nil {
			return &Issue{Code: CodeInvalidReference, Message: err.Error()}
		}
	}
	if _, err := macro.ParseArguments(ref); err != nil {
		return &Issue{Code: CodeInvalidReference, Message: err.Error()}
	}
	return nil
}
