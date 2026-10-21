package grafana

import (
	"fmt"
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	maxPanelTitle       = 200
	maxPanelDescription = 2000
	maxTextContent      = 20000
	maxPanelPaths       = 100
	noQueriesReason     = "None of this panel's queries can be imported."
)

var convertiblePanelTypes = map[spec.PanelType]bool{
	spec.Timeseries: true, spec.Stat: true, spec.Gauge: true, spec.BarGauge: true,
	spec.Table: true, spec.PieChart: true, spec.Heatmap: true, spec.Histogram: true,
	spec.StateTimeline: true, spec.Logs: true, spec.Text: true,
}

var silentPanelKeys = map[string]bool{
	"id": true, "gridPos": true, "type": true, "title": true, "description": true,
	"datasource": true, "targets": true, "pluginVersion": true, "transparent": true,
	"hideTimeOverride": true, "cacheTimeout": true, "queryCachingTTL": true,
	"links": true, "repeat": true, "repeatDirection": true, "maxPerRow": true,
	"timeFrom": true, "timeShift": true, "interval": true, "maxDataPoints": true,
	"libraryPanel": true, "collapsed": true, "panels": true, "isNew": true,
	"span": true, "height": true, "error": true, "editable": true,
}

type issueLog struct {
	paths     []string
	omitted   []string
	reasons   []string
	listed    map[string]bool
	dropped   map[string]bool
	reasonSet map[string]bool
	limit     int
}

type issueMark struct{ paths, omitted, reasons int }

func newIssueLog() *issueLog {
	return &issueLog{listed: map[string]bool{}, dropped: map[string]bool{}, reasonSet: map[string]bool{}, limit: maxPanelPaths}
}

func (l *issueLog) add(path, reason string) {
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
	return issueMark{paths: len(l.paths), omitted: len(l.omitted), reasons: len(l.reasons)}
}

func (l *issueLog) rollBack(m issueMark) {
	for _, reason := range l.reasons[m.reasons:] {
		delete(l.reasonSet, reason)
	}
	l.paths, l.omitted, l.reasons = l.paths[:m.paths], l.omitted[:m.omitted], l.reasons[:m.reasons]
}

func (l *issueLog) any() bool {
	return len(l.paths) > 0
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
	node     *panelNode
	panel    spec.Panel
	line     PanelLine
	children []*builtPanel
	repeat   string
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
	thresholdsMatter bool
	display          *spec.Display
}

func (c *converter) convertPanel(n *panelNode) *builtPanel {
	if reason := libraryPanelReason(n); reason != "" {
		return placeholderPanel(n, reason)
	}
	typ := spec.PanelType(n.typ)
	if !convertiblePanelTypes[typ] {
		return placeholderPanel(n, c.unsupportedReason(n))
	}
	if reason := legacyPlaceholderReason(n); reason != "" {
		return placeholderPanel(n, reason)
	}
	if typ != spec.Text {
		if reason := c.datasourceReason(n); reason != "" {
			return placeholderPanel(n, reason)
		}
	}
	b := &panelBuild{c: c, n: n, raw: n.raw, typ: typ, consumed: map[string]bool{}, refs: map[string]string{}, issues: newIssueLog()}
	b.panel = spec.Panel{
		ID:      n.id,
		Type:    typ,
		GridPos: spec.GridPos{X: n.pos.x, Y: n.pos.y, W: n.pos.w, H: n.pos.h},
	}
	if n.moved {
		b.partial("gridPos", movedReason)
	}
	if typ == spec.Text {
		return b.buildText()
	}
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
		if ref.supported() {
			return ""
		}
	}
	return n.ds.targets[0].label() + " is not supported yet."
}

func (c *converter) unsupportedReason(n *panelNode) string {
	if reason := c.datasourceReason(n); reason != "" {
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

func lookup(m map[string]any, path string) (any, bool) {
	var cur any = m
	for _, key := range strings.Split(path, ".") {
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		if cur, ok = obj[key]; !ok {
			return nil, false
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
	if b.isConsumed(path) || isGrafanaDefault(b.typ, path, v) || isCosmetic(path, v) {
		return
	}
	b.partial(path, "")
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
		b.partial("title", "Grafana shows this panel without a title.")
	}
	b.panel.Description = b.description()
	b.convertQueries()
	if len(b.built) == 0 {
		return b.noQueries()
	}
	b.assignQueries()
	b.convertFieldConfig()
	b.convertTransforms()
	b.convertDisplay()
	b.convertPanelOptions()
	b.convertLinks()
	b.datasourceVariables()
	b.reportUnconsumed()
	return b.finish()
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
	b.panel.Title = b.title()
	b.panel.Description = b.description()
	content := b.textContent()
	b.panel.Content = &content
	if asString(b.raw["repeat"]) != "" {
		b.partial("repeat", "Repeating text panels is not supported yet.")
	}
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

func (b *panelBuild) textMode() {
	path := "options.mode"
	v, ok := b.value(path)
	if !ok {
		path = "mode"
		v, ok = b.value(path)
	}
	if !ok {
		return
	}
	b.consume(path)
	switch asString(v) {
	case "", "markdown":
	case "html":
		b.partial(path, "HTML text is shown as Markdown.")
	default:
		b.partial(path, "Code text is shown as Markdown.")
	}
}

func (b *panelBuild) finish() *builtPanel {
	n := b.n
	status := statusImported
	if b.issues.any() {
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
	return &builtPanel{node: n, panel: b.panel, line: line}
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
