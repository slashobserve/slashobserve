package grafana

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	maxPanelQueries  = 26
	maxLegend        = 200
	altinityPlugin   = "vertamedia-clickhouse-datasource"
	genericQueryNote = "Some Grafana options of this query are not supported yet."
	sqlFormatTable   = 1
	sqlFormatLogs    = 2
	sqlFormatTraces  = 3

	exemplarsTimeSeriesOnly = "Exemplars are drawn on time series only."

	minResolutionFactor = 2
	maxResolutionFactor = 10
)

var refPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,19}$`)

type builtQuery struct {
	q            spec.Query
	refID        string
	index        int
	src          *sourceDS
	splitInstant bool
	copyOf       *builtQuery
	expr         *pendingExpression
	late         *targetBuild
	reuses       *builtPanel
}

type targetBuild struct {
	b        *panelBuild
	t        map[string]any
	path     string
	src      *sourceDS
	used     map[string]bool
	rewrites []string

	splitInstant bool
	rateMacro    bool
	expanded     bool
	expr         *pendingExpression
	reuses       *builtPanel
}

var targetSilent = map[string]bool{
	"refId": true, "datasource": true, "editorMode": true, "pluginVersion": true, "meta": true,
	"formattedQuery": true, "rawQuery": true, "showFormattedSQL": true, "dateLoading": true,
	"datetimeLoading": true, "tableLoading": true, "initialized": true, "expand": true,
	"metric": true, "key": true, "requestId": true, "hide": true, "step": true,
	"queryType": true, "builderOptions": true, "editorType": true, "format": true,
	"skip_comments": true, "add_metadata": true, "contextWindowSize": true,
	"useWindowFuncForMacros": true, "nullifySparse": true, "database": true, "table": true,
	"dateColDataType": true, "dateTimeColDataType": true, "dateTimeType": true,
	"useBackend": true, "disableTextWrap": true, "fullMetaSearch": true, "includeNullMetadata": true,
	"intervalMs": true, "maxDataPoints": true, "hinting": true,
}

var targetDefaults = map[string]any{
	"intervalFactor": 1, "exemplar": false, "instant": false, "range": true,
	"round": "0s",
}

func (c *converter) declaredVariables() map[string]bool {
	declared := map[string]bool{}
	for _, v := range c.vars {
		declared[v.v.Name] = true
	}
	for name := range c.droppedVars {
		declared[name] = true
	}
	return declared
}

func (b *panelBuild) convertQueries() {
	items, _ := asArray(b.raw["targets"])
	b.lines = make([]QueryLine, min(len(items), maxPanelPaths))
	declared := b.c.declaredVariables()
	overflow := visibleBeyondPathCap(items) > 0
	if overflow {
		b.issues.limit--
	}
	for i, item := range items {
		target, _ := asObject(item)
		*b.lineFor(i) = QueryLine{RefID: asString(target["refId"]), Rewrites: []string{}}
		if len(b.built) >= maxPanelQueries {
			b.dropTarget(i, target, fmt.Sprintf("A panel holds at most %d queries.", maxPanelQueries))
			continue
		}
		var ref dsRef
		if i < len(b.n.ds.targets) {
			ref = b.n.ds.targets[i]
		}
		tb := &targetBuild{b: b, t: target, path: fmt.Sprintf("targets[%d]", i), used: map[string]bool{}}
		mark := b.issues.mark()
		q, reason := tb.convert(ref, declared)
		if reason != "" {
			b.issues.rollBack(mark)
			b.dropTarget(i, target, reason)
			continue
		}
		bq := &builtQuery{q: q, refID: asString(target["refId"]), index: i, src: tb.src, splitInstant: tb.splitInstant, expr: tb.expr, reuses: tb.reuses}
		if tb.expr != nil || tb.reuses != nil {
			bq.late = tb
		} else {
			tb.reportUnconsumed()
		}
		line := b.lineFor(i)
		line.Rewrites = append(line.Rewrites, tb.rewrites...)
		b.built = append(b.built, bq)
	}
	b.dropMixedSources()
	b.dropBrokenExpressions()
	b.splitInstantQueries()
	if overflow {
		b.issues.limit++
	}
	if b.unlisted > 0 {
		b.partial("targets", unlistedQueriesReason(b.unlisted))
	}
}

func (b *panelBuild) splitInstantQueries() {
	room := maxPanelQueries - len(b.built)
	split := make([]*builtQuery, 0, len(b.built)+max(room, 0))
	for _, bq := range b.built {
		split = append(split, bq)
		if !bq.splitInstant {
			continue
		}
		if room <= 0 {
			b.partial(fmt.Sprintf("targets[%d].instant", bq.index), "Grafana runs an instant and a range query here; only the range query is kept.")
			continue
		}
		room--
		instant := bq.q
		instant.Instant = true
		instant.Interval = ""
		instant.IntervalFactor = 0
		instant.Exemplar = false
		split = append(split, &builtQuery{q: instant, index: bq.index, src: bq.src, copyOf: bq})
	}
	b.built = split
}

func visibleBeyondPathCap(items []any) int {
	visible := 0
	for _, item := range items[min(len(items), maxPanelPaths):] {
		if target, _ := asObject(item); !truthy(target["hide"]) {
			visible++
		}
	}
	return visible
}

func unlistedQueriesReason(n int) string {
	return notImportedReason(n, "query", "queries")
}

func (b *panelBuild) lineFor(i int) *QueryLine {
	if i < len(b.lines) {
		return &b.lines[i]
	}
	return &QueryLine{}
}

func (b *panelBuild) dropTarget(i int, target map[string]any, reason string) {
	hidden := truthy(target["hide"])
	if i >= len(b.lines) {
		if !hidden {
			b.unlisted++
		}
		return
	}
	b.lines[i].Dropped = ptr(reason)
	if !hidden {
		b.partial(fmt.Sprintf("targets[%d]", i), reason)
	}
}

func (tb *targetBuild) altinityFlavor() bool {
	hasSQL := asString(tb.t["rawSql"]) != ""
	hasQuery := asString(tb.t["query"]) != ""
	switch {
	case hasSQL && !hasQuery:
		return false
	case hasQuery && !hasSQL:
		return true
	}
	return tb.src.typ == altinityPlugin
}

func (tb *targetBuild) use(keys ...string) {
	for _, key := range keys {
		tb.used[key] = true
	}
}

func (tb *targetBuild) partial(key, reason string) {
	tb.use(key)
	tb.b.partial(tb.path+"."+key, reason)
}

func (tb *targetBuild) addRewrite(from, to string) {
	rewrite := from + " -> " + to
	if !slices.Contains(tb.rewrites, rewrite) {
		tb.rewrites = append(tb.rewrites, rewrite)
	}
}

func (tb *targetBuild) convert(ref dsRef, declared map[string]bool) (spec.Query, string) {
	if ref.kind == refSpecial && ref.special == "expression" {
		return tb.expression()
	}
	if ref.isDashboard() {
		return tb.dashboardSource()
	}
	if !ref.supported() {
		return spec.Query{}, ref.label() + " is not supported yet."
	}
	tb.src = ref.src
	var q spec.Query
	var reason string
	switch {
	case ref.src.kind == kindPrometheus:
		q, reason = tb.prometheus()
	case tb.altinityFlavor():
		q, reason = tb.altinity(declared)
	default:
		q, reason = tb.officialClickHouse()
	}
	if reason != "" {
		return spec.Query{}, reason
	}
	q.Hide = truthy(tb.t["hide"])
	return tb.finishQuery(q, declared)
}

func (tb *targetBuild) reportUnconsumed() {
	for _, key := range sortedKeys(tb.t) {
		v := tb.t[key]
		if tb.used[key] || targetSilent[key] || !present(v) {
			continue
		}
		if def, ok := targetDefaults[key]; ok && sameJSON(v, def) {
			continue
		}
		tb.b.partial(tb.path+"."+key, genericQueryNote)
	}
}

func (tb *targetBuild) legend() string {
	tb.use("legendFormat")
	legend := asString(tb.t["legendFormat"])
	cut, wasCut := cutRunes(legend, maxLegend)
	if wasCut {
		tb.partial("legendFormat", "The legend was cut to 200 characters.")
	}
	return cut
}

func (tb *targetBuild) minInterval() string {
	tb.use("interval", "step")
	interval := strings.TrimSpace(asString(tb.t["interval"]))
	if interval == "" {
		return tb.legacyStep()
	}
	if value, ok := tb.b.c.minStep(interval); ok {
		return value
	}
	tb.partial("interval", fmt.Sprintf("The min step %s cannot be kept; SlashObserve picks the step.", interval))
	return ""
}

func (tb *targetBuild) resolutionFactor() int {
	tb.use("intervalFactor")
	factor, ok := asFloat(tb.t["intervalFactor"])
	switch {
	case !ok || factor == 1:
		return 0
	case factor == math.Trunc(factor) && factor >= minResolutionFactor && factor <= maxResolutionFactor:
		return int(factor)
	}
	tb.partial("intervalFactor", "The resolution factor of this query is not supported yet.")
	return 0
}

func (tb *targetBuild) legacyStep() string {
	seconds, ok := numberValue(tb.t["step"])
	if !ok || seconds <= 0 {
		return ""
	}
	interval := strconv.FormatFloat(seconds, 'f', -1, 64) + "s"
	if !intervalPattern.MatchString(interval) {
		tb.partial("step", fmt.Sprintf("The step %s cannot be kept; SlashObserve picks the step.", interval))
		return ""
	}
	return interval
}

func (tb *targetBuild) prometheus() (spec.Query, string) {
	tb.use("expr", "range", "instant", "format", "intervalFactor", "exemplar")
	expr := asString(tb.t["expr"])
	if strings.TrimSpace(expr) == "" {
		return spec.Query{}, "The query is empty."
	}
	q := spec.Query{Query: expr, Legend: tb.legend(), Interval: tb.minInterval(), IntervalFactor: tb.resolutionFactor()}
	if truthy(tb.t["instant"]) {
		tb.splitInstant = truthy(tb.t["range"])
		q.Instant = !tb.splitInstant
	}
	switch asString(tb.t["format"]) {
	case "table":
		q.Format = "table"
	case "heatmap":
		q.Format = "heatmap"
	}
	if truthy(tb.t["exemplar"]) {
		if q.Instant || tb.b.typ != spec.Timeseries {
			tb.partial("exemplar", exemplarsTimeSeriesOnly)
		} else {
			q.Exemplar = true
		}
	}
	return q, ""
}

func (tb *targetBuild) officialClickHouse() (spec.Query, string) {
	tb.use("rawSql")
	sql := asString(tb.t["rawSql"])
	if strings.TrimSpace(sql) == "" {
		return spec.Query{}, "The query is empty."
	}
	format, reason := tb.clickHouseFormat()
	if reason != "" {
		return spec.Query{}, reason
	}
	return spec.Query{Query: sql, Format: format}, ""
}

func (tb *targetBuild) clickHouseFormat() (string, string) {
	switch asString(tb.t["queryType"]) {
	case "timeseries":
		return "", ""
	case "table":
		return "table", ""
	case "logs":
		return "logs", ""
	case "traces":
		return "table", ""
	}
	n, _ := asInt(tb.t["format"])
	switch n {
	case sqlFormatTable:
		return "table", ""
	case sqlFormatLogs:
		return "logs", ""
	case sqlFormatTraces:
		return "table", ""
	}
	return "", ""
}

func (tb *targetBuild) altinity(declared map[string]bool) (spec.Query, string) {
	tb.use("query", "interval", "intervalFactor", "round", "extrapolate", "adHocFilters", "adHocValuesQuery")
	query := asString(tb.t["query"])
	if strings.TrimSpace(query) == "" {
		return spec.Query{}, "The query is empty."
	}
	text, reason := tb.rewriteAltinity(query, declared)
	if reason != "" {
		return spec.Query{}, reason
	}
	q := spec.Query{Query: text, Interval: tb.minInterval(), IntervalFactor: tb.resolutionFactor()}
	switch asString(tb.t["format"]) {
	case "table":
		q.Format = "table"
	case "logs":
		q.Format = "logs"
	case "traces", "flamegraph":
		q.Format = "table"
	}
	tb.reportRound(q)
	if tb.rateMacro {
		if extrapolate, set := tb.t["extrapolate"]; !set || truthy(extrapolate) {
			tb.partial("extrapolate", "Edge extrapolation of rate macros is not kept.")
		}
	}
	for _, key := range []string{"adHocFilters", "adHocValuesQuery"} {
		if present(tb.t[key]) {
			tb.partial(key, "Saved ad hoc filters are not kept.")
		}
	}
	if tb.timeColumnIsUnix() {
		tb.partial("dateTimeType", "The time column is a Unix timestamp; check the time filter.")
	}
	return q, ""
}

func (tb *targetBuild) reportRound(q spec.Query) {
	round := strings.TrimSpace(asString(tb.t["round"]))
	if round == "" || round == "0s" || round == "$step" {
		return
	}
	if step, ok := tb.minimumStep(q); ok {
		if size, err := spec.ParseInterval(round); err == nil && size > 0 && step%size == 0 {
			return
		}
	}
	tb.partial("round", "Rounding of the time range is not supported yet.")
}

func (tb *targetBuild) minimumStep(q spec.Query) (time.Duration, bool) {
	panelInterval := ""
	if raw := strings.TrimSpace(asString(tb.b.raw["interval"])); raw != "" {
		value, ok := tb.b.c.minStep(raw)
		if !ok {
			return 0, false
		}
		panelInterval = value
	}
	var step time.Duration
	for _, text := range []string{q.Interval, panelInterval} {
		if text == "" {
			continue
		}
		d, err := spec.ParseInterval(text)
		if err != nil {
			return 0, false
		}
		step = max(step, d)
	}
	if step == 0 {
		return 0, false
	}
	return step * time.Duration(max(q.IntervalFactor, 1)), true
}

func (tb *targetBuild) timeColumnIsUnix() bool {
	kind := asString(tb.t["dateTimeType"])
	return kind != "" && kind != "DATETIME" && kind != "DATETIME64"
}

func (tb *targetBuild) finishQuery(q spec.Query, declared map[string]bool) (spec.Query, string) {
	text, legacy := rewriteLegacyRefs(q.Query)
	for _, rewrite := range legacy {
		from, to, _ := strings.Cut(rewrite, " -> ")
		tb.addRewrite(from, to)
	}
	if tb.src.kind == kindClickHouse && !tb.altinityFlavor() {
		text = tb.rewriteOfficialMacros(text)
	}
	if len(text) > maxQueryLength {
		return spec.Query{}, "Its query is longer than 20000 bytes."
	}
	if reason := checkReferences(text, tb.src.kind, declared); reason != "" {
		return spec.Query{}, reason
	}
	q.Query = text
	return q, ""
}

func checkReferences(text, kind string, declared map[string]bool) string {
	refs, err := spec.ScanQuery(text)
	if err != nil {
		return unsupportedReference(scanErrorLabel(text, err))
	}
	macros := map[string]spec.Macro{}
	for _, m := range spec.Macros(spec.DatasourceType(kind)) {
		macros[m.Name] = m
	}
	for _, ref := range spec.FlattenReferences(refs) {
		if !referenceSupported(ref, macros, declared) {
			return unsupportedReference(referenceLabel(text, ref))
		}
	}
	return ""
}

func referenceSupported(ref spec.Reference, macros map[string]spec.Macro, declared map[string]bool) bool {
	if ref.Kind != spec.MacroRef {
		return declared[ref.Name]
	}
	macro, ok := macros[ref.Name]
	if !ok {
		return false
	}
	if ref.Format != "" && macro.CheckFormat(ref.Format) != nil {
		return false
	}
	_, err := macro.ParseArguments(ref)
	return err == nil
}

func unsupportedReference(label string) string {
	return fmt.Sprintf("It uses %s, which SlashObserve does not support.", label)
}

func referenceLabel(text string, ref spec.Reference) string {
	if ref.Start < 0 || ref.End > len(text) || ref.Start >= ref.End {
		return "$" + ref.Name
	}
	return shortened(text[ref.Start:ref.End])
}

func scanErrorLabel(text string, err error) string {
	var scan *spec.ScanError
	if !errors.As(err, &scan) || scan.Offset < 0 || scan.Offset >= len(text) {
		return "a reference it cannot read"
	}
	rest := text[scan.Offset:]
	if end := strings.IndexFunc(rest, unicode.IsSpace); end > 0 {
		rest = rest[:end]
	}
	return shortened(rest)
}

func shortened(s string) string {
	cut, wasCut := cutRunes(s, 60)
	if wasCut {
		return cut + "..."
	}
	return cut
}

func effectiveRefs(built []*builtQuery) []string {
	explicit := map[string]int{}
	used := map[string]bool{}
	for k, bq := range built {
		if _, dup := explicit[bq.refID]; !dup && refPattern.MatchString(bq.refID) {
			explicit[bq.refID] = k
			used[bq.refID] = true
		}
	}
	refs := make([]string, len(built))
	for k, bq := range built {
		if pos, ok := explicit[bq.refID]; ok && pos == k {
			refs[k] = bq.refID
			continue
		}
		candidate := spec.DefaultQueryRef(k)
		for i := 0; used[candidate]; i++ {
			candidate = spec.DefaultQueryRef(i)
		}
		used[candidate] = true
		refs[k] = candidate
	}
	return refs
}

func (b *panelBuild) assignQueries() {
	refs := effectiveRefs(b.built)
	explicit := explicitRefIDs(b.built)
	panelDS := ""
	if k := b.firstDataQuery(); k >= 0 {
		panelDS = b.built[k].src.ref()
	}
	b.panel.Datasource = panelDS
	for k, bq := range b.built {
		q := bq.q
		if bq.expr != nil || refs[k] != spec.DefaultQueryRef(k) {
			q.Ref = refs[k]
		}
		if bq.src != nil && bq.src.ref() != panelDS {
			q.Datasource = bq.src.ref()
		}
		b.panel.Queries = append(b.panel.Queries, q)
		line := b.lineFor(bq.index)
		if bq.copyOf != nil {
			b.instantRefs[refs[k-1]] = refs[k]
			if bq.copyOf.refID != "" {
				line.Rewrites = append(line.Rewrites, fmt.Sprintf("refId %s (instant) -> %s", bq.copyOf.refID, refs[k]))
			}
			continue
		}
		line.Ref = ptr(refs[k])
		if bq.src != nil {
			line.Datasource = ptr(bq.src.ref())
		}
		if bq.refID == "" {
			b.mapImpliedRef(bq.index, refs[k], explicit)
			continue
		}
		if _, mapped := b.refs[bq.refID]; !mapped {
			b.refs[bq.refID] = refs[k]
		}
		if refs[k] != bq.refID {
			line.Rewrites = append(line.Rewrites, fmt.Sprintf("refId %s -> %s", bq.refID, refs[k]))
		}
	}
	b.finishExpressions()
	b.adoptSourceRefs()
}

const impliedRefLetters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"

func explicitRefIDs(built []*builtQuery) map[string]bool {
	explicit := map[string]bool{}
	for _, bq := range built {
		if bq.refID != "" {
			explicit[bq.refID] = true
		}
	}
	return explicit
}

func impliedRefID(position int, explicit map[string]bool) (string, bool) {
	if position >= len(impliedRefLetters) {
		return "", false
	}
	implied := impliedRefLetters[position : position+1]
	return implied, !explicit[implied]
}

func (b *panelBuild) mapImpliedRef(position int, ref string, explicit map[string]bool) {
	if implied, ok := impliedRefID(position, explicit); ok {
		b.refs[implied] = ref
	}
}

var altinityWholeQuery = map[string]bool{
	"columns": true, "columnsMs": true, "rate": true, "rateColumns": true, "perSecond": true,
	"perSecondColumns": true, "delta": true, "deltaColumns": true, "increase": true,
	"increaseColumns": true, "lttb": true,
	"rateColumnsAggregated": true, "perSecondColumnsAggregated": true,
	"deltaColumnsAggregated": true, "increaseColumnsAggregated": true,
}

func identAt(s string, from int) string {
	end := from
	for end < len(s) {
		c := s[end]
		letter := c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
		if !letter && (end == from || c < '0' || c > '9') {
			break
		}
		end++
	}
	return s[from:end]
}

func altinityDropReason(name string) string {
	return fmt.Sprintf("Altinity macro $%s builds the whole query and is not supported; rewrite it as SQL.", name)
}

func (tb *targetBuild) rewriteAltinity(text string, declared map[string]bool) (string, string) {
	before := len(tb.rewrites)
	tb.expanded = false
	out, reason := tb.rewriteAltinityText(text, declared, true)
	if reason == "" && tb.expanded {
		tb.rewrites = append(tb.rewrites[:before:before], text+" -> "+out)
	}
	return out, reason
}

func (tb *targetBuild) rewriteAltinityText(text string, declared map[string]bool, wholeQuery bool) (string, string) {
	var out strings.Builder
	for i := 0; i < len(text); {
		if text[i] != '$' {
			out.WriteByte(text[i])
			i++
			continue
		}
		name := identAt(text, i+1)
		end := i + 1 + len(name)
		if name == "" || declared[name] {
			out.WriteString(text[i:end])
			i = end
			continue
		}
		if altinityWholeQuery[name] {
			expanded, reason := tb.expandWholeQuery(name, text, end, declared, wholeQuery && blankOrComments(text[:i]))
			if reason != "" {
				return "", reason
			}
			return out.String() + expanded, ""
		}
		replacement, next, reason := tb.inlineAltinity(name, text, i, end)
		if reason != "" {
			return "", reason
		}
		out.WriteString(replacement)
		i = next
	}
	return out.String(), ""
}

func (tb *targetBuild) expandWholeQuery(name, text string, end int, declared map[string]bool, allowed bool) (string, string) {
	if !allowed || !strings.HasPrefix(text[end:], "(") {
		return "", altinityDropReason(name)
	}
	args, rest, ok := splitAltinityCall(text, end)
	if !ok {
		return "", altinityDropReason(name)
	}
	sql, ok := tb.expandAltinity(name, args, rest)
	if !ok {
		return "", altinityDropReason(name)
	}
	tb.expanded = true
	tb.rateMacro = tb.rateMacro || isCounterMacro(name)
	return tb.rewriteAltinityText(sql, declared, false)
}

func (tb *targetBuild) inlineAltinity(name, text string, start, end int) (string, int, string) {
	switch name {
	case "adhoc":
		replacement := tb.adhocReplacement(text, end)
		tb.addRewrite("$adhoc", replacement)
		return replacement, end, ""
	case "timeFilterByColumn":
		m := timeFilterColumnArgument.FindStringSubmatch(text[end:])
		if m == nil {
			return "", 0, altinityDropReason(name)
		}
		replacement := "$__timeFilter(" + m[1] + ")"
		tb.addRewrite(text[start:end+len(m[0])], replacement)
		return replacement, end + len(m[0]), ""
	}
	replacement, next, ok := tb.altinityMacro(name, text, start, end)
	if !ok {
		replacement, next = text[start:end], end
	}
	return replacement, next, ""
}

var timeFilterColumnArgument = regexp.MustCompile(`^\(\s*([A-Za-z_][A-Za-z0-9_]*)\s*\)`)

func (tb *targetBuild) adhocReplacement(text string, end int) string {
	var names []string
	for _, v := range tb.b.c.vars {
		if v.v.Type == spec.AdhocVariable && tb.b.c.adhocSources[v.v.Name] == tb.src {
			names = append(names, v.v.Name)
		}
	}
	if len(names) != 1 {
		tb.partial("query", "Ad hoc filters inside this query are not applied.")
		return "1"
	}
	if end < len(text) && isSQLIdentByte(text[end]) {
		return "${" + names[0] + "}"
	}
	return "$" + names[0]
}

func (tb *targetBuild) altinityMacro(name, text string, start, end int) (string, int, bool) {
	switch name {
	case "unescape":
		return tb.unescapeMacro(text, start, end)
	case "conditionalTest":
		if !strings.HasPrefix(text[end:], "(") {
			return "", 0, false
		}
		tb.addRewrite("$conditionalTest(", "$__conditionalAll(")
		return "$__conditionalAll", end, true
	}
	replacement, ok := tb.altinitySimple(name)
	if !ok {
		return "", 0, false
	}
	tb.addRewrite("$"+name, replacement)
	return replacement, end, true
}

var unescapeArgument = regexp.MustCompile(`^\s*\(\s*(?:\$([A-Za-z_][A-Za-z0-9_]*)|\$\{([A-Za-z_][A-Za-z0-9_]*)(?::[A-Za-z]+)?\})\s*\)`)

func (tb *targetBuild) unescapeMacro(text string, start, end int) (string, int, bool) {
	m := unescapeArgument.FindStringSubmatch(text[end:])
	if m == nil {
		return "", 0, false
	}
	variable := m[1] + m[2]
	replacement := "${" + variable + ":raw}"
	tb.addRewrite(text[start:end+len(m[0])], replacement)
	return replacement, end + len(m[0]), true
}

func (tb *targetBuild) altinitySimple(name string) (string, bool) {
	table, database := asString(tb.t["table"]), asString(tb.t["database"])
	date, dateTime := asString(tb.t["dateColDataType"]), asString(tb.t["dateTimeColDataType"])
	timeMacro := func(format string, args ...any) (string, bool) {
		if dateTime == "" {
			return "", false
		}
		return fmt.Sprintf(format, args...), true
	}
	switch name {
	case "table":
		if table == "" {
			return "", false
		}
		if database == "" {
			return table, true
		}
		return database + "." + table, true
	case "timeFilter":
		if date != "" {
			return timeMacro("$__dateTimeFilter(%s, %s)", date, dateTime)
		}
		return timeMacro("$__timeFilter(%s)", dateTime)
	case "timeFilterMs":
		return timeMacro("$__timeFilter_ms(%s)", dateTime)
	case "timeSeries":
		return timeMacro("$__timeInterval(%s)", dateTime)
	case "timeSeriesMs":
		return timeMacro("$__timeInterval_ms(%s)", dateTime)
	case "dateTimeCol":
		return dateTime, dateTime != ""
	case "dateCol":
		return date, date != ""
	case "from":
		return "${__from:date:seconds}", true
	case "to":
		return "${__to:date:seconds}", true
	case "interval":
		return "$__interval_s", true
	}
	return "", false
}

type textEdit struct {
	start, end int
	to         string
}

func (tb *targetBuild) rewriteOfficialMacros(text string) string {
	refs, err := spec.ScanQuery(text)
	if err != nil {
		return text
	}
	var edits []textEdit
	for _, ref := range refs {
		if ref.Kind != spec.MacroRef {
			continue
		}
		if to, ok := officialReplacement(ref); ok {
			edits = append(edits, textEdit{ref.Start, ref.End, to})
			tb.addRewrite(text[ref.Start:ref.End], to)
		}
	}
	for i := len(edits) - 1; i >= 0; i-- {
		text = text[:edits[i].start] + edits[i].to + text[edits[i].end:]
	}
	return text
}

func officialReplacement(ref spec.Reference) (string, bool) {
	switch ref.Name {
	case "__timeFrom":
		return "toDateTime($__from / 1000)", !ref.HasArgs
	case "__timeTo":
		return "toDateTime($__to / 1000)", !ref.HasArgs
	case "__timeGroup":
		if args := spec.SplitMacroArgs(ref.Args); ref.HasArgs && len(args) == 2 {
			return timeGroupReplacement(args[0], args[1])
		}
	}
	return "", false
}

var (
	groupInterval   = regexp.MustCompile(`^['"]?\s*([1-9][0-9]*)\s*(s|m|h|d|w|(?:second|minute|hour|day|week)s?)\s*['"]?$`)
	groupUnits      = map[string]string{"s": "SECOND", "m": "MINUTE", "h": "HOUR", "d": "DAY", "w": "WEEK", "second": "SECOND", "minute": "MINUTE", "hour": "HOUR", "day": "DAY", "week": "WEEK"}
	autoIntervalArg = map[string]bool{"$__interval_s": true, "${__interval_s}": true}
)

func timeGroupReplacement(column, interval string) (string, bool) {
	if !spec.IsColumnArg(column) {
		return "", false
	}
	if autoIntervalArg[interval] {
		return "$__timeInterval(" + column + ")", true
	}
	m := groupInterval.FindStringSubmatch(interval)
	if m == nil {
		return "", false
	}
	unit := m[2]
	if len(unit) > 1 {
		unit = strings.TrimSuffix(unit, "s")
	}
	return fmt.Sprintf("toStartOfInterval(toDateTime(%s), INTERVAL %s %s)", column, m[1], groupUnits[unit]), true
}

var (
	minStepVariableRef = regexp.MustCompile(`^(?:\$([A-Za-z][A-Za-z0-9_]{0,39})|\$\{([A-Za-z][A-Za-z0-9_]{0,39})(?::[A-Za-z0-9_]+)?\}|\[\[([A-Za-z][A-Za-z0-9_]{0,39})(?::[A-Za-z0-9_]+)?\]\])$`)

	builtInMinSteps = []string{"$__interval", "${__interval}", "$__rate_interval", "${__rate_interval}"}
)

func minStepVariable(text string) (string, bool) {
	m := minStepVariableRef.FindStringSubmatch(text)
	if m == nil {
		return "", false
	}
	return m[1] + m[2] + m[3], true
}

func (c *converter) minStep(text string) (string, bool) {
	switch {
	case intervalPattern.MatchString(text):
		return text, true
	case slices.Contains(builtInMinSteps, text):
		return "", true
	}
	name, ok := minStepVariable(text)
	if !ok {
		return "", false
	}
	for _, v := range c.vars {
		if v.v.Name == name && v.v.Type == spec.IntervalVariable {
			return "${" + name + "}", true
		}
	}
	return "", false
}

func usedAsMinStep(dash map[string]any, name string) bool {
	uses := func(value any) bool {
		got, ok := minStepVariable(strings.TrimSpace(asString(value)))
		return ok && got == name
	}
	var visit func(items []any) bool
	visit = func(items []any) bool {
		for _, item := range items {
			p, _ := asObject(item)
			if uses(p["interval"]) {
				return true
			}
			for _, target := range anySlice(p["targets"]) {
				t, _ := asObject(target)
				if uses(t["interval"]) {
					return true
				}
			}
			if visit(anySlice(p["panels"])) {
				return true
			}
		}
		return false
	}
	if visit(anySlice(dash["panels"])) {
		return true
	}
	for _, item := range anySlice(dash["rows"]) {
		row, _ := asObject(item)
		if visit(anySlice(row["panels"])) {
			return true
		}
	}
	return false
}
