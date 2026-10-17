package spec

import (
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
	issues = append(issues, checkDuplicatePanelIDs(d.Panels)...)
	issues = append(issues, checkDuplicateVariableNames(d.Variables)...)
	issues = append(issues, checkGridOverflow(d.Panels)...)
	issues = append(issues, checkGridOverlap(d.Panels)...)
	issues = append(issues, checkTimeRange(d.TimeRange)...)
	issues = append(issues, checkThresholds(d.Panels)...)
	issues = append(issues, checkMinMax(d.Panels)...)
	issues = append(issues, checkVariableQueries(d.Variables)...)
	issues = append(issues, checkPanelQueries(d)...)
	return issues
}

func checkDuplicatePanelIDs(panels []Panel) []Issue {
	var issues []Issue
	first := map[string]int{}
	for j, p := range panels {
		i, dup := first[p.ID]
		if !dup {
			first[p.ID] = j
			continue
		}
		issues = append(issues, Issue{
			Path:    pointer("panels", strconv.Itoa(j), "id"),
			Code:    CodeDuplicatePanelID,
			Message: fmt.Sprintf("panel id %q is already used by %s", p.ID, pointer("panels", strconv.Itoa(i))),
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

func checkGridOverflow(panels []Panel) []Issue {
	var issues []Issue
	for i, p := range panels {
		g := p.GridPos
		if g.X+g.W > GridColumns {
			issues = append(issues, Issue{
				Path:    pointer("panels", strconv.Itoa(i), "gridPos"),
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

func checkGridOverlap(panels []Panel) []Issue {
	var issues []Issue
	for j, pj := range panels {
		for i := range j {
			if !gridsOverlap(panels[i].GridPos, pj.GridPos) {
				continue
			}
			issues = append(issues, Issue{
				Path:    pointer("panels", strconv.Itoa(j), "gridPos"),
				Code:    CodeGridOverlap,
				Message: fmt.Sprintf("panel %q overlaps panel %q at %s; move one of them to free grid cells", pj.ID, panels[i].ID, pointer("panels", strconv.Itoa(i))),
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
	p := timePoint{relative: true, fixed: true, roundUnit: m[4]}
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

func relativeInOrder(from, to timePoint) bool {
	if from.roundUnit != "" && from.roundUnit == to.roundUnit && from.fixed && to.fixed && timeUnits[from.roundUnit].min == timeUnits[from.roundUnit].max {
		unit := timeUnits[from.roundUnit].max
		if math.Mod(from.agoMax, unit) == 0 && math.Mod(to.agoMax, unit) == 0 {
			return from.agoMax >= to.agoMax
		}
	}
	return from.agoMax+from.roundMax > to.agoMin-to.roundMax
}

func checkTimeRange(tr TimeRange) []Issue {
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
				Path:    pointer("timeRange", f.name),
				Code:    CodeSchema,
				Message: fmt.Sprintf("timeRange.%s %q is not a valid time; use a relative time such as \"now-6h\" or a real RFC 3339 timestamp", f.name, f.value),
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
			Path:    pointer("timeRange"),
			Code:    CodeTimeRangeOrder,
			Message: fmt.Sprintf("timeRange.from %q is not earlier than timeRange.to %q; swap them or move from further back", tr.From, tr.To),
		})
	}
	return issues
}

func checkThresholds(panels []Panel) []Issue {
	var issues []Issue
	for i, p := range panels {
		for k := 1; k < len(p.Thresholds); k++ {
			if p.Thresholds[k].Value > p.Thresholds[k-1].Value {
				continue
			}
			issues = append(issues, Issue{
				Path:    pointer("panels", strconv.Itoa(i), "thresholds", strconv.Itoa(k)),
				Code:    CodeThresholdsUnordered,
				Message: fmt.Sprintf("threshold value %v must be greater than the previous value %v; list thresholds in strictly ascending order", p.Thresholds[k].Value, p.Thresholds[k-1].Value),
			})
			break
		}
	}
	return issues
}

func checkMinMax(panels []Panel) []Issue {
	var issues []Issue
	for i, p := range panels {
		if p.Min == nil || p.Max == nil || *p.Min < *p.Max {
			continue
		}
		issues = append(issues, Issue{
			Path:    pointer("panels", strconv.Itoa(i), "max"),
			Code:    CodeMinMaxOrder,
			Message: fmt.Sprintf("max %v must be greater than min %v", *p.Max, *p.Min),
		})
	}
	return issues
}

type refChecker struct {
	declared map[string]int
}

func newRefChecker(vars []Variable) refChecker {
	declared := map[string]int{}
	for i, v := range vars {
		if _, ok := declared[v.Name]; !ok {
			declared[v.Name] = i
		}
	}
	return refChecker{declared: declared}
}

func checkVariableQueries(vars []Variable) []Issue {
	var issues []Issue
	rc := newRefChecker(vars)
	for i, v := range vars {
		if v.Type != QueryVariable || v.Datasource == nil {
			continue
		}
		if issue := rc.checkQuery(v.Query, v.Datasource.Type, i); issue != nil {
			issue.Path = pointer("variables", strconv.Itoa(i), "query")
			issues = append(issues, *issue)
		}
	}
	return issues
}

func checkPanelQueries(d *Dashboard) []Issue {
	var issues []Issue
	rc := newRefChecker(d.Variables)
	for i, p := range d.Panels {
		if issue := rc.checkQuery(p.Query, p.Datasource.Type, len(d.Variables)); issue != nil {
			issue.Path = pointer("panels", strconv.Itoa(i), "query")
			issues = append(issues, *issue)
		}
	}
	return issues
}

func (rc refChecker) checkQuery(query string, ds DatasourceType, visible int) *Issue {
	refs, err := ScanQuery(query)
	if err != nil {
		return &Issue{Code: CodeInvalidReference, Message: err.Error()}
	}
	for _, ref := range refs {
		var issue *Issue
		if ref.Kind == MacroRef {
			issue = checkMacroRef(ref, ds)
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
	case ref.Format != "" && !slices.Contains(VariableFormats(), ref.Format):
		return &Issue{Code: CodeInvalidReference, Message: fmt.Sprintf("unknown format %q in reference to variable %q; use one of %s", ref.Format, ref.Name, strings.Join(VariableFormats(), ", "))}
	}
	return nil
}

func checkMacroRef(ref Reference, ds DatasourceType) *Issue {
	macros := Macros(ds)
	idx := slices.IndexFunc(macros, func(m Macro) bool { return m.Name == ref.Name })
	if idx < 0 {
		return &Issue{Code: CodeUnknownMacro, Message: fmt.Sprintf("macro $%s is not available for %s queries", ref.Name, ds)}
	}
	switch {
	case ref.Format != "":
		return &Issue{Code: CodeInvalidReference, Message: fmt.Sprintf("macro $%s does not take a format", ref.Name)}
	case macros[idx].Function && strings.TrimSpace(ref.Args) == "":
		return &Issue{Code: CodeInvalidReference, Message: fmt.Sprintf("macro $%s needs arguments, for example $%s(column)", ref.Name, ref.Name)}
	case macros[idx].Function && !IsColumnArg(strings.TrimSpace(ref.Args)):
		return &Issue{Code: CodeInvalidReference, Message: fmt.Sprintf("macro $%s argument must be a column name such as column or table.column", ref.Name)}
	case !macros[idx].Function && ref.HasArgs:
		return &Issue{Code: CodeInvalidReference, Message: fmt.Sprintf("macro $%s does not take arguments; remove the parentheses", ref.Name)}
	}
	return nil
}
