package grafana

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

const (
	outcomeMapped      Outcome = "mapped"
	outcomeExcluded    Outcome = "excluded"
	outcomeReported    Outcome = "reported"
	outcomeDefault     Outcome = "default"
	outcomeNoEffect    Outcome = "no_effect"
	outcomeBookkeeping Outcome = "bookkeeping"
)

const (
	blockDashboard      BlockKind = "dashboard"
	blockVariable       BlockKind = "variable"
	blockRow            BlockKind = "row"
	blockPanel          BlockKind = "panel"
	blockQuery          BlockKind = "query"
	blockTransformation BlockKind = "transformation"
	blockFieldConfig    BlockKind = "field_config"
	blockOverride       BlockKind = "override"
)

const (
	maxBlockOptions  = 200
	maxReportOptions = 10000
	maxBlockChildren = 100
)

const overridePrefix = "overrides[]."

func (o Outcome) implicit() bool {
	return o == outcomeDefault || o == outcomeNoEffect || o == outcomeBookkeeping
}

func (o Outcome) rank() int {
	switch o {
	case outcomeExcluded:
		return 0
	case outcomeReported:
		return 1
	case outcomeMapped:
		return 2
	}
	return 3
}

func reportedOutcome(reason string) Outcome {
	if isDecisionReason(reason) {
		return outcomeExcluded
	}
	return outcomeReported
}

type optionEntry struct {
	path    string
	outcome Outcome
	spec    []string
	reason  string
}

func scopedFullPath(root, rel string) string {
	if root == "" {
		return rel
	}
	return root + "." + rel
}

func (b *panelBuild) addOption(e optionEntry) {
	if coveredBy(e.path, func(ancestor string) bool { return b.recorded[ancestor] }) {
		return
	}
	b.recorded[e.path] = true
	b.options = append(b.options, e)
}

func (b *panelBuild) record(path string, fallback Outcome) {
	full := scopedFullPath(b.n.path, path)
	p := b.c.src.scopeOf(full)
	fields, option, found := loadedOptionMap().specFields(p, func(rel string) (any, bool) { return b.c.src.valueAt(p.root, rel) })
	if found {
		b.addOption(optionEntry{path: scopedFullPath(p.root, option), outcome: outcomeMapped, spec: fields})
		return
	}
	b.addOption(optionEntry{path: full, outcome: fallback})
}

func (b *panelBuild) recordImplicit(path string, outcome Outcome) {
	b.addOption(optionEntry{path: scopedFullPath(b.n.path, path), outcome: outcome})
}

var blockPrefix = regexp.MustCompile(`^(targets\[[0-9]+\]|transformations\[[0-9]+\]|fieldConfig\.overrides\[[0-9]+\])`)

func (b *panelBuild) consumedByName(path string) bool {
	floor := len(blockPrefix.FindString(path))
	if len(path) > floor && b.consumed[path] {
		return true
	}
	for i := floor + 1; i < len(path); i++ {
		if (path[i] == '.' || path[i] == '[') && b.consumed[path[:i]] {
			return true
		}
	}
	return false
}

func (b *panelBuild) recordSilentKeys() {
	for _, key := range sortedKeys(b.raw) {
		if !silentPanelKeys[key] || key == "targets" || key == "panels" || key == "type" || !present(b.raw[key]) || b.issues.covers(key) {
			continue
		}
		b.record(key, outcomeBookkeeping)
	}
}

func (b *panelBuild) optionEntries() []optionEntry {
	out := slices.Clone(b.options)
	for _, g := range b.issues.entries() {
		reason := strings.Join(g.reasons, " ")
		outcome := outcomeReported
		if slices.ContainsFunc(g.reasons, isDecisionReason) {
			outcome = outcomeExcluded
		}
		out = append(out, optionEntry{path: scopedFullPath(b.n.path, g.path), outcome: outcome, reason: reason})
	}
	return out
}

func coveredBy(path string, has func(ancestor string) bool) bool {
	for {
		if has(path) {
			return true
		}
		cut := strings.LastIndexAny(path, ".[")
		if cut < 0 {
			return false
		}
		path = path[:cut]
	}
}

func (c *converter) optionPaths() func(path string) bool {
	set := make(map[string]bool, len(c.options))
	for _, e := range c.options {
		set[e.path] = true
	}
	return func(path string) bool {
		return coveredBy(path, func(ancestor string) bool { return set[ancestor] })
	}
}

func (c *converter) recordKey(key string, fallback func() Outcome) {
	p := c.src.scopeOf(key)
	fields, option, found := loadedOptionMap().specFields(p, func(rel string) (any, bool) { return c.src.valueAt(p.root, rel) })
	if found {
		c.options = append(c.options, optionEntry{path: scopedFullPath(p.root, option), outcome: outcomeMapped, spec: fields})
		return
	}
	c.options = append(c.options, optionEntry{path: key, outcome: fallback()})
}

func hasKnownChildren(scope optionScope, key string) bool {
	for option := range loadedOptionMap()[scope] {
		if strings.HasPrefix(option, key+".") || strings.HasPrefix(option, key+"[") {
			return true
		}
	}
	return false
}

func (c *converter) recordDashboardKeys() {
	covered := c.optionPaths()
	for _, key := range sortedKeys(c.src.dash) {
		if key == "panels" || key == "rows" || key == "templating" || !present(c.src.dash[key]) || covered(key) {
			continue
		}
		c.recordKey(key, func() Outcome {
			if slices.Contains(timeSettingKeys, key) || hasKnownChildren(scopeDashboard, key) {
				return outcomeMapped
			}
			return outcomeBookkeeping
		})
	}
}

func (c *converter) recordVariableKeys() {
	covered := c.optionPaths()
	seen := map[string]bool{}
	for _, v := range c.vars {
		if seen[v.path] {
			continue
		}
		seen[v.path] = true
		item, _ := lookup(c.src.dash, v.path)
		raw, _ := asObject(item)
		handled := variableHandledKeys[asString(raw["type"])]
		for _, key := range sortedKeys(raw) {
			path := v.path + "." + key
			if !present(raw[key]) || covered(path) {
				continue
			}
			c.recordKey(path, func() Outcome {
				if slices.Contains(handled, key) {
					return outcomeMapped
				}
				return outcomeBookkeeping
			})
		}
	}
}

type blockBuild struct {
	kind       BlockKind
	path       string
	title      string
	sourceType string
	panelID    *string
	outcome    *Outcome
	reason     *string
	unkept     bool
	entries    []optionEntry
	children   []*blockBuild
}

type blockIndex map[string]*blockBuild

func (idx blockIndex) add(b *blockBuild) *blockBuild {
	idx[b.path] = b
	return b
}

func (idx blockIndex) route(path string) *blockBuild {
	for {
		if b, ok := idx[path]; ok {
			return b
		}
		cut := strings.LastIndexAny(path, ".[")
		if cut < 0 {
			return idx[""]
		}
		path = path[:cut]
	}
}

func (b *blockBuild) fail(outcome Outcome, reason string) {
	b.outcome, b.reason = &outcome, &reason
}

func (c *converter) blocks() []Block {
	idx := blockIndex{}
	dashboard := idx.add(&blockBuild{kind: blockDashboard})
	tops := []*blockBuild{dashboard}
	tops = append(tops, c.variableBlocks(idx)...)
	for _, bp := range c.built {
		if bp != nil {
			tops = append(tops, c.panelBlock(bp, idx))
		}
	}
	for _, e := range c.options {
		b := idx.route(e.path)
		b.entries = append(b.entries, e)
	}
	for _, bp := range c.built {
		if bp != nil {
			c.routePanelEntries(bp, idx)
		}
	}
	out := make([]Block, len(tops))
	for i, b := range tops {
		out[i] = b.materialize()
	}
	return out
}

func (c *converter) routePanelEntries(bp *builtPanel, idx blockIndex) {
	for _, e := range bp.options {
		b := idx.route(e.path)
		b.entries = append(b.entries, e)
	}
	for _, child := range bp.children {
		if child != nil {
			c.routePanelEntries(child, idx)
		}
	}
}

func (c *converter) variableBlocks(idx blockIndex) []*blockBuild {
	templating, _ := asObject(c.src.dash["templating"])
	list, _ := asArray(templating["list"])
	kept := map[string]bool{}
	for _, v := range c.vars {
		kept[v.path] = true
	}
	var out []*blockBuild
	for i, item := range list[:min(len(list), maxBlockChildren)] {
		raw, _ := asObject(item)
		path := indexedPath("templating.list", i)
		out = append(out, idx.add(&blockBuild{
			kind:       blockVariable,
			path:       path,
			title:      asString(raw["name"]),
			sourceType: asString(raw["type"]),
			unkept:     !kept[path],
		}))
	}
	return out
}

func indexedPath(base string, i int) string {
	return fmt.Sprintf("%s[%d]", base, i)
}

func (c *converter) panelBlock(bp *builtPanel, idx blockIndex) *blockBuild {
	n := bp.node
	if bp.isRow() {
		row := idx.add(&blockBuild{kind: blockRow, path: n.path, title: bp.panel.Title, sourceType: n.sourceType, panelID: ptr(n.id)})
		for _, child := range bp.children {
			if child != nil {
				row.children = append(row.children, c.panelBlock(child, idx))
			}
		}
		return row
	}
	panel := idx.add(&blockBuild{kind: blockPanel, path: n.path, title: bp.line.Title, sourceType: n.sourceType, panelID: ptr(n.id)})
	if bp.line.Status == statusPlaceholder {
		panel.fail(reportedOutcome(bp.line.Reasons[0]), bp.line.Reasons[0])
		return panel
	}
	panel.children = append(panel.children, c.queryBlocks(bp, idx)...)
	panel.children = append(panel.children, c.transformationBlocks(bp, idx)...)
	fieldConfig := idx.add(&blockBuild{kind: blockFieldConfig, path: n.path + ".fieldConfig.defaults"})
	panel.children = append(panel.children, fieldConfig)
	panel.children = append(panel.children, c.overrideBlocks(bp, idx)...)
	return panel
}

func (c *converter) queryBlocks(bp *builtPanel, idx blockIndex) []*blockBuild {
	n := bp.node
	items, _ := asArray(n.raw["targets"])
	var out []*blockBuild
	for k, item := range items[:min(len(items), maxBlockChildren)] {
		target, _ := asObject(item)
		var ref dsRef
		if k < len(n.ds.targets) {
			ref = n.ds.targets[k]
		}
		b := idx.add(&blockBuild{
			kind:       blockQuery,
			path:       indexedPath(n.path+".targets", k),
			title:      asString(target["refId"]),
			sourceType: string(queryKindOf(ref, target)),
		})
		if k < len(bp.line.Queries) && bp.line.Queries[k].Dropped != nil {
			reason := *bp.line.Queries[k].Dropped
			b.fail(reportedOutcome(reason), reason)
		}
		out = append(out, b)
	}
	return out
}

func (c *converter) transformationBlocks(bp *builtPanel, idx blockIndex) []*blockBuild {
	n := bp.node
	items, _ := asArray(n.raw["transformations"])
	var out []*blockBuild
	for k, item := range items[:min(len(items), maxBlockChildren)] {
		transformation, _ := asObject(item)
		out = append(out, idx.add(&blockBuild{
			kind:  blockTransformation,
			path:  indexedPath(n.path+".transformations", k),
			title: asString(transformation["id"]),
		}))
	}
	return out
}

func (c *converter) overrideBlocks(bp *builtPanel, idx blockIndex) []*blockBuild {
	n := bp.node
	value, _ := lookup(n.raw, "fieldConfig.overrides")
	items, _ := asArray(value)
	var out []*blockBuild
	for k, item := range items[:min(len(items), maxBlockChildren)] {
		override, _ := asObject(item)
		matcher, _ := asObject(override["matcher"])
		out = append(out, idx.add(&blockBuild{
			kind:  blockOverride,
			path:  indexedPath(n.path+".fieldConfig.overrides", k),
			title: asString(matcher["id"]),
		}))
	}
	return out
}

func dedupeEntries(entries []optionEntry) []optionEntry {
	at := map[string]int{}
	var out []optionEntry
	for _, e := range entries {
		i, seen := at[e.path]
		switch {
		case !seen:
			at[e.path] = len(out)
			out = append(out, e)
		case e.outcome.rank() < out[i].outcome.rank():
			out[i] = e
		}
	}
	return out
}

func (b *blockBuild) takesOwnEntry() bool {
	return b.kind == blockQuery || b.kind == blockTransformation || b.kind == blockOverride
}

func (b *blockBuild) ownEntry(e optionEntry) bool {
	if e.outcome != outcomeExcluded && e.outcome != outcomeReported {
		return false
	}
	return b.takesOwnEntry() && e.path == b.path
}

func (b *blockBuild) split(entries []optionEntry) []optionEntry {
	kept := make([]optionEntry, 0, len(entries))
	adopted := false
	for _, e := range entries {
		switch {
		case b.unkept && !adopted:
			adopted = true
			b.adopt(e)
		case b.ownEntry(e):
			b.adopt(e)
		default:
			kept = append(kept, e)
		}
	}
	return kept
}

func (b *blockBuild) adopt(e optionEntry) {
	if b.outcome == nil && (e.outcome == outcomeExcluded || e.outcome == outcomeReported) {
		b.fail(e.outcome, e.reason)
	}
}

func relativeOptionPath(blockPath, path string) string {
	switch {
	case blockPath == "":
		return path
	case path == blockPath:
		return ""
	}
	return strings.TrimPrefix(path[len(blockPath):], ".")
}

func (b *blockBuild) options(entries []optionEntry) []OptionLine {
	lines := make([]OptionLine, len(entries))
	for i, e := range entries {
		line := OptionLine{Path: relativeOptionPath(b.path, e.path), Outcome: e.outcome, Spec: []string{}}
		for _, field := range e.spec {
			if b.kind == blockOverride {
				field = strings.TrimPrefix(field, overridePrefix)
			}
			line.Spec = append(line.Spec, field)
		}
		if e.outcome == outcomeExcluded || e.outcome == outcomeReported {
			line.Reason = ptr(e.reason)
		}
		lines[i] = line
	}
	slices.SortStableFunc(lines, func(a, z OptionLine) int {
		if d := a.Outcome.rank() - z.Outcome.rank(); d != 0 {
			return d
		}
		return strings.Compare(a.Path, z.Path)
	})
	return lines
}

func (b *blockBuild) materialize() Block {
	entries := b.split(dedupeEntries(b.entries))
	block := Block{
		Kind:       b.kind,
		Path:       b.path,
		Title:      b.title,
		SourceType: b.sourceType,
		PanelID:    b.panelID,
		Outcome:    b.outcome,
		Reason:     b.reason,
		Options:    b.options(entries),
		Blocks:     []Block{},
	}
	for _, e := range entries {
		if e.outcome.implicit() {
			block.Implicit++
		}
	}
	for _, child := range b.children {
		if child.kind == blockFieldConfig && len(child.entries) == 0 {
			continue
		}
		block.Blocks = append(block.Blocks, child.materialize())
	}
	return block
}

func walkBlocks(blocks []Block, visit func(*Block)) {
	for i := range blocks {
		visit(&blocks[i])
		walkBlocks(blocks[i].Blocks, visit)
	}
}

func applyLimits(blocks []Block) {
	total := 0
	walkBlocks(blocks, func(b *Block) {
		if len(b.Options) > maxBlockOptions {
			b.Omitted += len(b.Options) - maxBlockOptions
			b.Options = b.Options[:maxBlockOptions]
		}
		for _, line := range b.Options {
			if line.Outcome == outcomeExcluded || line.Outcome == outcomeReported {
				total++
			}
		}
	})
	limitTo(blocks, &total, func(o Outcome) bool { return o == outcomeMapped })
	limitTo(blocks, &total, Outcome.implicit)
}

func limitTo(blocks []Block, total *int, selected func(Outcome) bool) {
	walkBlocks(blocks, func(b *Block) {
		kept := b.Options[:0]
		for _, line := range b.Options {
			switch {
			case !selected(line.Outcome):
				kept = append(kept, line)
			case *total < maxReportOptions:
				*total++
				kept = append(kept, line)
			default:
				b.Omitted++
			}
		}
		b.Options = kept
	})
}
