package grafana

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	maxPanelTitle       = 200
	maxPanelDescription = 2000
	maxTextContent      = 20000
	maxPanelPaths       = 100
	noQueriesReason     = "None of this panel's queries can be imported."
	debugSource         = "debug"
	docsURL             = "https://github.com/slashobserve/slashobserve/blob/main/docs/"
)

var homePanelContent = map[string]string{
	"welcome": "Welcome to Grafana. This panel was Grafana's home page welcome panel.",
	"gettingstarted": "Getting started with SlashObserve:\n\n" +
		"- [ ] [Connect a datasource](" + docsURL + "connect/prometheus.md)\n" +
		"- [ ] [Describe a dashboard to the agent](" + docsURL + "use/chat.md)\n" +
		"- [ ] [Import more dashboards from Grafana](" + docsURL + "use/import-grafana.md)\n" +
		"- [ ] [Invite your team](" + docsURL + "teams/roles.md)\n",
}

var convertiblePanelTypes = map[spec.PanelType]bool{
	spec.Timeseries: true, spec.Stat: true, spec.Gauge: true, spec.BarGauge: true,
	spec.Table: true, spec.PieChart: true, spec.Heatmap: true, spec.Histogram: true,
	spec.StateTimeline: true, spec.Logs: true, spec.Text: true,
	spec.BarChart: true, spec.StatusHistory: true, spec.Trend: true, spec.XYChart: true,
	spec.Candlestick: true, spec.NodeGraph: true, spec.Traces: true, spec.FlameGraph: true,
	spec.Geomap: true, spec.Canvas: true, spec.AlertList: true, spec.DashList: true, spec.News: true,
}

var placeholderReasons = map[string]string{
	"annolist":    decisionAnnotations.reason,
	"live":        decisionLive.reason,
	"alertGroups": decisionAlertRules.reason,
}

var silentPanelKeys = map[string]bool{
	"id": true, "gridPos": true, "type": true, "title": true, "description": true,
	"datasource": true, "targets": true, "pluginVersion": true,
	"cacheTimeout": true, "queryCachingTTL": true,
	"links": true, "repeat": true, "repeatDirection": true, "maxPerRow": true,
	"timeFrom": true, "timeShift": true, "interval": true, "maxDataPoints": true,
	"libraryPanel": true, "collapsed": true, "panels": true, "isNew": true,
	"span": true, "height": true, "error": true, "editable": true, "scopedVars": true,
}

type issueEntry struct{ path, reason string }

type issueGroup struct {
	path    string
	reasons []string
}

type issueLog struct {
	paths     []string
	omitted   []string
	reasons   []string
	all       []issueEntry
	entered   map[string]int
	listed    map[string]bool
	dropped   map[string]bool
	reasonSet map[string]bool
	limit     int
	excluded  func(path string) (decision, bool)
}

type issueMark struct{ paths, omitted, reasons, all int }

func newIssueLog(excluded func(path string) (decision, bool)) *issueLog {
	return &issueLog{entered: map[string]int{}, listed: map[string]bool{}, dropped: map[string]bool{}, reasonSet: map[string]bool{}, limit: maxPanelPaths, excluded: excluded}
}

func (l *issueLog) add(path, reason string) {
	if decided, ok := l.excluded(path); ok {
		reason = decided.reason
	}
	l.all = append(l.all, issueEntry{path: path, reason: reason})
	l.entered[path]++
	switch {
	case l.listed[path]:
	case l.dropped[path]:
		return
	case len(l.paths) >= l.limit:
		l.dropped[path] = true
		l.omitted = append(l.omitted, path)
		return
	default:
		l.listed[path] = true
		l.paths = append(l.paths, path)
	}
	if !l.reasonSet[reason] {
		l.reasonSet[reason] = true
		l.reasons = append(l.reasons, reason)
	}
}

func (l *issueLog) mark() issueMark {
	return issueMark{paths: len(l.paths), omitted: len(l.omitted), reasons: len(l.reasons), all: len(l.all)}
}

func (l *issueLog) rollBack(m issueMark) {
	for _, reason := range l.reasons[m.reasons:] {
		delete(l.reasonSet, reason)
	}
	for _, e := range l.all[m.all:] {
		l.entered[e.path]--
	}
	l.paths, l.omitted, l.reasons, l.all = l.paths[:m.paths], l.omitted[:m.omitted], l.reasons[:m.reasons], l.all[:m.all]
}

func (l *issueLog) covers(path string) bool {
	return coveredBy(path, func(ancestor string) bool { return l.entered[ancestor] > 0 })
}

func (l *issueLog) entries() []issueGroup {
	var groups []issueGroup
	at := map[string]int{}
	for _, e := range l.all {
		i, seen := at[e.path]
		if !seen {
			i = len(groups)
			at[e.path] = i
			groups = append(groups, issueGroup{path: e.path})
		}
		if !slices.Contains(groups[i].reasons, e.reason) {
			groups[i].reasons = append(groups[i].reasons, e.reason)
		}
	}
	return groups
}

func (l *issueLog) any() bool {
	return len(l.paths) > 0
}

func (l *issueLog) decided() bool {
	return len(l.omitted) == 0 && !slices.ContainsFunc(l.all, func(e issueEntry) bool { return !isDecisionReason(e.reason) })
}

func (l *issueLog) report() (paths, reasons []string) {
	paths = append([]string{}, l.paths...)
	reasons = append([]string{}, l.reasons...)
	if len(l.omitted) > 0 {
		reasons = append(reasons, notImportedReason(len(l.omitted), "option", "options"))
	}
	return paths, reasons
}

func notImportedReason(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("1 more %s was not imported.", one)
	}
	return fmt.Sprintf("%d more %s were not imported.", n, many)
}

type builtPanel struct {
	node        *panelNode
	panel       spec.Panel
	line        PanelLine
	children    []*builtPanel
	repeat      string
	queries     []*builtQuery
	refs        map[string]string
	instantRefs map[string]string
	options     []optionEntry
}

func (bp *builtPanel) isRow() bool {
	return bp.node.row
}

func (bp *builtPanel) isPlaceholder() bool {
	return bp.panel.Type == spec.Placeholder
}

type panelBuild struct {
	c                *converter
	n                *panelNode
	raw              map[string]any
	typ              spec.PanelType
	panel            spec.Panel
	consumed         map[string]bool
	issues           *issueLog
	built            []*builtQuery
	lines            []QueryLine
	unlisted         int
	refs             map[string]string
	instantRefs      map[string]string
	thresholdsMatter bool
	display          *spec.Display
	options          []optionEntry
	recorded         map[string]bool
}

func (c *converter) convertPanel(n *panelNode) *builtPanel {
	if reason := libraryPanelReason(n); reason != "" {
		return placeholderPanel(n, reason)
	}
	typ := spec.PanelType(n.typ)
	switch n.sourceType {
	case "welcome", "gettingstarted":
		return c.newPanelBuild(n, spec.Text).buildTextPanel(func() string { return homePanelContent[n.sourceType] })
	case debugSource:
		return c.debugPanel(n)
	}
	if !convertiblePanelTypes[typ] {
		return placeholderPanel(n, c.unsupportedReason(n))
	}
	if reason := legacyPlaceholderReason(n); reason != "" {
		return placeholderPanel(n, reason)
	}
	if reason := alertListReason(n); reason != "" {
		return placeholderPanel(n, reason)
	}
	if _, static := staticPanelNames[typ]; !static {
		if reason := c.datasourceReason(n); reason != "" {
			return placeholderPanel(n, reason)
		}
	}
	b := c.newPanelBuild(n, typ)
	switch typ {
	case spec.Text:
		return b.buildText()
	case spec.DashList:
		return b.buildDashList()
	case spec.News:
		return b.buildNews()
	}
	return b.buildData()
}

func (c *converter) newPanelBuild(n *panelNode, typ spec.PanelType) *panelBuild {
	b := &panelBuild{c: c, n: n, raw: n.raw, typ: typ, consumed: map[string]bool{}, recorded: map[string]bool{}, refs: map[string]string{}, instantRefs: map[string]string{}, issues: newIssueLog(func(path string) (decision, bool) { return c.exclusionAt(n.path + "." + path) })}
	b.panel = spec.Panel{
		ID:      n.id,
		Type:    typ,
		GridPos: spec.GridPos{X: n.pos.x, Y: n.pos.y, W: n.pos.w, H: n.pos.h},
	}
	if n.moved {
		b.partial("gridPos", movedReason)
	}
	return b
}

func (c *converter) debugPanel(n *panelNode) *builtPanel {
	targets, _ := asArray(n.raw["targets"])
	if len(targets) == 0 {
		b := c.newPanelBuild(n, spec.Text)
		b.partial("type", "")
		return b.buildTextPanel(func() string { return decisionDebugPanel.reason })
	}
	if reason := c.datasourceReason(n); reason != "" {
		return placeholderPanel(n, reason)
	}
	b := c.newPanelBuild(n, spec.Table)
	b.partial("type", "")
	return b.buildData()
}

func (c *converter) datasourceReason(n *panelNode) string {
	if own := n.ds.own; len(n.ds.targets) == 0 && own.kind == refSource && !own.supported() {
		return own.label() + " is not supported yet."
	}
	if len(n.ds.targets) == 0 {
		return ""
	}
	for _, ref := range n.ds.targets {
		if ref.supported() || ref.isDashboard() {
			return ""
		}
	}
	return n.ds.targets[0].label() + " is not supported yet."
}

func (c *converter) unsupportedReason(n *panelNode) string {
	if reason := c.datasourceReason(n); reason != "" {
		return reason
	}
	if reason, ok := placeholderReasons[n.sourceType]; ok {
		return reason
	}
	return fmt.Sprintf("Grafana panel type %s is not supported yet.", n.sourceType)
}

func hasInlineModel(p map[string]any) bool {
	for _, key := range []string{"targets", "options", "fieldConfig", "content"} {
		if present(p[key]) {
			return true
		}
	}
	return false
}

func libraryPanelReason(n *panelNode) string {
	lib, ok := asObject(n.raw["libraryPanel"])
	if !ok || hasInlineModel(n.raw) {
		return ""
	}
	name := asString(lib["name"])
	if name == "" {
		name = asString(lib["uid"])
	}
	return fmt.Sprintf("Library panel %s was not included in the JSON.", name)
}

func placeholderPanel(n *panelNode, reason string) *builtPanel {
	panel := spec.Panel{
		ID:      n.id,
		Type:    spec.Placeholder,
		GridPos: spec.GridPos{X: n.pos.x, Y: n.pos.y, W: n.pos.w, H: n.pos.h},
	}
	if title := strings.TrimSpace(n.title); title != "" {
		panel.Title, _ = cutRunes(title, 200)
	}
	panel.Description, _ = cutRunes(asString(n.raw["description"]), 2000)
	panel.Reason, _ = cutRunes(reason, 500)
	panel.SourceType, _ = cutRunes(n.sourceType, 100)

	line := PanelLine{
		PanelID:     n.id,
		GrafanaID:   n.grafanaID,
		GrafanaPath: n.path,
		Title:       n.title,
		Type:        string(spec.Placeholder),
		SourceType:  n.sourceType,
		Row:         n.rowTitle,
		Status:      statusPlaceholder,
		Reasons:     []string{reason},
		Paths:       []string{},
		Queries:     []QueryLine{},
	}
	if n.moved {
		line.Reasons = append(line.Reasons, movedReason)
		line.Paths = append(line.Paths, "gridPos")
	}
	targets, _ := asArray(n.raw["targets"])
	if unlisted := visibleBeyondPathCap(targets); unlisted > 0 {
		line.Paths = append(line.Paths, "targets")
		line.Reasons = append(line.Reasons, unlistedQueriesReason(unlisted))
	}
	targets = targets[:min(len(targets), maxPanelPaths)]
	for _, item := range targets {
		target, _ := asObject(item)
		line.Queries = append(line.Queries, QueryLine{
			RefID:    asString(target["refId"]),
			Rewrites: []string{},
			Dropped:  ptr("The panel is a placeholder, so its query is not imported."),
		})
	}
	return &builtPanel{node: n, panel: panel, line: line}
}

var indexedKey = regexp.MustCompile(`^(.*?)((?:\[[0-9]+\])*)$`)

func lookup(m map[string]any, path string) (any, bool) {
	var cur any = m
	for _, key := range strings.Split(path, ".") {
		parts := indexedKey.FindStringSubmatch(key)
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		if cur, ok = obj[parts[1]]; !ok {
			return nil, false
		}
		for _, index := range indexPattern.FindAllString(parts[2], -1) {
			items, isArray := cur.([]any)
			i, _ := strconv.Atoi(strings.Trim(index, "[]"))
			if !isArray || i >= len(items) {
				return nil, false
			}
			cur = items[i]
		}
	}
	return cur, true
}

func (b *panelBuild) value(path string) (any, bool) {
	return lookup(b.raw, path)
}

func (b *panelBuild) take(path string) (any, bool) {
	v, ok := lookup(b.raw, path)
	if ok {
		b.consume(path)
	}
	return v, ok
}

func (b *panelBuild) consume(paths ...string) {
	for _, path := range paths {
		b.consumed[path] = true
	}
}

func (b *panelBuild) isConsumed(path string) bool {
	if b.consumed[path] {
		return true
	}
	for i := 0; i < len(path); i++ {
		if (path[i] == '.' || path[i] == '[') && b.consumed[path[:i]] {
			return true
		}
	}
	return false
}

func (b *panelBuild) partial(path, reason string) {
	b.consume(path)
	if reason == "" {
		reason = partialReason(path)
	}
	b.issues.add(path, reason)
}

func (b *panelBuild) reportUnconsumed() {
	b.walk("", b.raw)
	b.recordSilentKeys()
}

func (b *panelBuild) walk(prefix string, v any) {
	switch x := v.(type) {
	case map[string]any:
		for _, key := range sortedKeys(x) {
			if prefix == "" && silentPanelKeys[key] {
				continue
			}
			path := key
			if prefix != "" {
				path = prefix + "." + key
			}
			b.walk(path, x[key])
		}
	case []any:
		if !slices.ContainsFunc(x, isContainer) {
			b.leaf(prefix, v)
			return
		}
		for i, item := range x {
			b.walk(fmt.Sprintf("%s[%d]", prefix, i), item)
		}
	default:
		b.leaf(prefix, v)
	}
}

func isContainer(v any) bool {
	switch v.(type) {
	case map[string]any, []any:
		return true
	}
	return false
}

func (b *panelBuild) leaf(path string, v any) {
	switch x := v.(type) {
	case nil:
		return
	case string:
		if x == "" {
			return
		}
	case []any:
		if len(x) == 0 {
			return
		}
	case map[string]any:
		if len(x) == 0 {
			return
		}
	}
	if b.issues.covers(path) {
		return
	}
	switch {
	case b.isConsumed(path):
		if b.consumedByName(path) || strings.HasPrefix(path, "transformations[") {
			b.record(path, outcomeMapped)
		}
	case isGrafanaDefault(b.typ, path, v):
		b.recordImplicit(path, outcomeDefault)
	case isIgnoredOption(b.n.scope(), path), isCosmeticOn(b.typ, path, v):
		b.recordImplicit(path, outcomeNoEffect)
	default:
		b.partial(path, "")
	}
}

func (b *panelBuild) title() string {
	title := strings.TrimSpace(b.n.title)
	if title == "" {
		return ""
	}
	cut, wasCut := cutRunes(title, maxPanelTitle)
	if wasCut {
		b.partial("title", "The title was cut to 200 characters.")
	}
	return cut
}

func (b *panelBuild) description() string {
	description := asString(b.raw["description"])
	cut, wasCut := cutRunes(description, maxPanelDescription)
	if wasCut {
		b.partial("description", "The description was cut to 2000 characters.")
	}
	return cut
}

func (b *panelBuild) buildData() *builtPanel {
	b.panel.Title = b.title()
	if b.panel.Title == "" {
		b.panel.Title = b.untitled()
		b.panel.TitleHidden = true
	}
	b.panel.Description = b.description()
	b.convertPanelQueries()
	if len(b.built) == 0 {
		return b.noQueries()
	}
	b.assignQueries()
	b.convertFieldConfig()
	b.convertTransforms()
	b.convertPanelTimezone()
	b.convertDisplay()
	b.convertPanelOptions()
	b.convertPanelChrome()
	b.convertLinks()
	b.reportUnconsumed()
	return b.finish()
}

var timezonePanelTypes = map[spec.PanelType]bool{
	spec.Timeseries: true, spec.StateTimeline: true, spec.StatusHistory: true,
}

const timezoneListReason = "A panel draws its times in one time zone, and Grafana's list of zones must hold a single supported one."

func (b *panelBuild) convertPanelTimezone() {
	const path = "options.timezone"
	if !timezonePanelTypes[b.typ] {
		return
	}
	v, ok := b.value(path)
	if !ok {
		return
	}
	zones, listed := zonesOf(v)
	if !listed {
		b.partial(path, timezoneListReason)
		return
	}
	var distinct []string
	for _, zone := range zones {
		if zone = normalizedZone(zone); !slices.Contains(distinct, zone) {
			distinct = append(distinct, zone)
		}
	}
	switch {
	case len(distinct) == 0, len(distinct) == 1 && distinct[0] == "":
		b.consume(path)
	case len(distinct) == 1 && importableZone(distinct[0]):
		b.panel.Timezone = distinct[0]
		b.consume(path)
	default:
		b.partial(path, timezoneListReason)
	}
}

func zonesOf(v any) ([]string, bool) {
	switch zones := v.(type) {
	case nil:
		return nil, true
	case string:
		return []string{zones}, true
	case []any:
		out := make([]string, 0, len(zones))
		for _, item := range zones {
			zone, isText := item.(string)
			if !isText {
				return nil, false
			}
			out = append(out, zone)
		}
		return out, true
	}
	return nil, false
}

func normalizedZone(zone string) string {
	switch {
	case strings.EqualFold(zone, "utc"):
		return "utc"
	case zone == "default":
		return ""
	}
	return zone
}

func importableZone(zone string) bool {
	return zonePattern.MatchString(zone) && spec.CheckTimezone(zone) == nil
}

func (b *panelBuild) convertHideTimeOverride() {
	if truthy(b.raw["hideTimeOverride"]) && b.typ.IsData() {
		b.panel.HideTimeOverride = true
	}
	b.consume("hideTimeOverride")
}

func (b *panelBuild) convertPanelQueries() {
	if b.typ == spec.AlertList {
		b.alertListQueries()
		return
	}
	b.convertQueries()
}

func (b *panelBuild) untitled() string {
	if b.n.grafanaID != nil {
		return fmt.Sprintf("Panel %d", *b.n.grafanaID)
	}
	return "Panel " + b.n.id
}

func (b *panelBuild) noQueries() *builtPanel {
	reasons := []string{noQueriesReason}
	for _, line := range b.lines {
		if line.Dropped != nil && !slices.Contains(reasons, *line.Dropped) {
			reasons = append(reasons, *line.Dropped)
		}
	}
	placeholder := placeholderPanel(b.n, strings.Join(reasons, " "))
	placeholder.line.Reasons = append(reasons, placeholder.line.Reasons[1:]...)
	if len(b.lines) > 0 {
		placeholder.line.Queries = b.lines
	}
	return placeholder
}

func (b *panelBuild) buildText() *builtPanel {
	return b.buildTextPanel(b.textContent)
}

func (b *panelBuild) buildTextPanel(content func() string) *builtPanel {
	b.panel.Title = b.title()
	b.panel.Description = b.description()
	text := content()
	b.panel.Content = &text
	if asString(b.raw["repeat"]) != "" {
		b.partial("repeat", "Repeating text panels is not supported yet.")
	}
	b.convertPanelChrome()
	b.convertLinks()
	b.reportUnconsumed()
	return b.finish()
}

func (b *panelBuild) textContent() string {
	content := ""
	if v, ok := b.take("options.content"); ok {
		content = asString(v)
	} else if v, ok := b.take("content"); ok {
		content = asString(v)
	}
	b.textMode()
	cut, wasCut := cutRunes(content, maxTextContent)
	if wasCut {
		b.partial("options.content", "The text was cut to 20000 characters.")
	}
	return cut
}

var textCodeLanguages = []string{"json", "yaml", "xml", "sql", "go", "typescript", "markdown", "html"}

func (b *panelBuild) textMode() {
	path := "options.mode"
	v, ok := b.value(path)
	if !ok {
		path = "mode"
		v, ok = b.value(path)
	}
	if ok {
		b.consume(path)
	}
	switch asString(v) {
	case "html":
		b.panel.Mode = "html"
	case "code":
		b.panel.Mode = "code"
		b.panel.Code = b.textCode()
	default:
		b.consume("options.code")
	}
}

func (b *panelBuild) textCode() *spec.TextCode {
	code := &spec.TextCode{}
	if v, ok := b.take("options.code.language"); ok {
		language := asString(v)
		switch {
		case slices.Contains(textCodeLanguages, language):
			code.Language = language
		case language != "" && language != "plaintext":
			b.issues.add("options.code.language", "This language is shown as plain text.")
		}
	}
	if flag, ok := b.boolOption("options.code.showLineNumbers"); ok && *flag {
		code.LineNumbers = flag
	}
	if code.Language == "" && code.LineNumbers == nil {
		return nil
	}
	return code
}

func (b *panelBuild) finish() *builtPanel {
	n := b.n
	status := statusImported
	if b.issues.any() && !b.issues.decided() {
		status = statusPartial
	}
	paths, reasons := b.issues.report()
	queries := b.lines
	if queries == nil {
		queries = []QueryLine{}
	}
	line := PanelLine{
		PanelID:     n.id,
		GrafanaID:   n.grafanaID,
		GrafanaPath: n.path,
		Title:       n.title,
		Type:        string(b.typ),
		SourceType:  n.sourceType,
		Row:         n.rowTitle,
		Status:      status,
		Reasons:     reasons,
		Paths:       paths,
		Queries:     queries,
	}
	b.panel.Display = b.display
	return &builtPanel{node: n, panel: b.panel, line: line, queries: b.built, refs: b.refs, instantRefs: b.instantRefs, options: b.optionEntries()}
}

func inlineLibraryPanels(dash map[string]any) {
	elements, _ := asObject(dash["__elements"])
	if len(elements) == 0 {
		return
	}
	var visit func(items []any)
	visit = func(items []any) {
		for _, item := range items {
			p, ok := asObject(item)
			if !ok {
				continue
			}
			inlineLibraryPanel(p, elements)
			children, _ := asArray(p["panels"])
			visit(children)
		}
	}
	visit(anySlice(dash["panels"]))
	rows, _ := asArray(dash["rows"])
	for _, item := range rows {
		row, _ := asObject(item)
		visit(anySlice(row["panels"]))
	}
}

func inlineLibraryPanel(p map[string]any, elements map[string]any) {
	lib, ok := asObject(p["libraryPanel"])
	if !ok || hasInlineModel(p) {
		return
	}
	element, _ := asObject(elements[asString(lib["uid"])])
	model, ok := asObject(element["model"])
	if !ok {
		return
	}
	for key, value := range model {
		switch key {
		case "id", "gridPos", "libraryPanel":
		case "title":
			if !present(p["title"]) {
				p[key] = value
			}
		default:
			p[key] = value
		}
	}
}
