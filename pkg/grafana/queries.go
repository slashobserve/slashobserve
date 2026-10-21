package grafana

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	maxPanelQueries   = 26
	maxLegend         = 200
	altinityPlugin    = "vertamedia-clickhouse-datasource"
	builderReason     = "Imported the generated SQL; the query builder settings are not kept."
	genericQueryNote  = "Some Grafana options of this query are not supported yet."
	sqlFormatTable    = 1
	sqlFormatLogs     = 2
	sqlFormatTraces   = 3
	tracesDropped     = "Trace queries are not supported yet."
	expressionDropped = "Server-side expressions are not supported yet."
)

var refPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,19}$`)

type builtQuery struct {
	q     spec.Query
	refID string
	index int
	src   *sourceDS
}

type targetBuild struct {
	b        *panelBuild
	t        map[string]any
	path     string
	src      *sourceDS
	used     map[string]bool
	rewrites []string
}

var targetSilent = map[string]bool{
	"refId": true, "datasource": true, "editorMode": true, "pluginVersion": true, "meta": true,
	"formattedQuery": true, "rawQuery": true, "showFormattedSQL": true, "dateLoading": true,
	"datetimeLoading": true, "tableLoading": true, "initialized": true, "expand": true,
	"metric": true, "key": true, "requestId": true, "hide": true, "step": true,
	"queryType": true, "builderOptions": true, "editorType": true, "format": true,
	"extrapolate": true, "skip_comments": true, "add_metadata": true, "contextWindowSize": true,
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
	overflow := len(items) > len(b.lines)
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
		tb.reportUnconsumed()
		line := b.lineFor(i)
		line.Rewrites = append(line.Rewrites, tb.rewrites...)
		b.built = append(b.built, &builtQuery{q: q, refID: asString(target["refId"]), index: i, src: tb.src})
	}
	if overflow {
		b.issues.limit++
	}
	if b.unlisted > 0 {
		b.partial("targets", unlistedQueriesReason(b.unlisted))
	}
}

func unlistedQueriesReason(n int) string {
	return fmt.Sprintf("%d more queries were not imported.", n)
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
		return spec.Query{}, expressionDropped
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
	if intervalPattern.MatchString(interval) {
		return interval
	}
	tb.partial("interval", fmt.Sprintf("The min step %s cannot be kept; SlashObserve picks the step.", interval))
	return ""
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
	q := spec.Query{Query: expr, Legend: tb.legend(), Interval: tb.minInterval()}
	if truthy(tb.t["instant"]) {
		q.Instant = !truthy(tb.t["range"])
		if !q.Instant {
			tb.partial("instant", "Grafana runs an instant and a range query here; only the range query is kept.")
		}
	}
	switch asString(tb.t["format"]) {
	case "table":
		q.Format = "table"
	case "heatmap":
		q.Format = "heatmap"
	}
	if factor, ok := asFloat(tb.t["intervalFactor"]); ok && factor != 1 {
		tb.partial("intervalFactor", "The resolution factor of this query is not supported yet.")
	}
	if truthy(tb.t["exemplar"]) {
		tb.partial("exemplar", "Exemplars are not supported yet.")
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
	if asString(tb.t["editorType"]) == "builder" {
		tb.partial("builderOptions", builderReason)
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
		return "", tracesDropped
	}
	n, _ := asInt(tb.t["format"])
	switch n {
	case sqlFormatTable:
		return "table", ""
	case sqlFormatLogs:
		return "logs", ""
	case sqlFormatTraces:
		return "", tracesDropped
	}
	return "", ""
}

func (tb *targetBuild) altinity(declared map[string]bool) (spec.Query, string) {
	tb.use("query", "interval", "intervalFactor", "round", "adHocFilters", "adHocValuesQuery")
	query := asString(tb.t["query"])
	if strings.TrimSpace(query) == "" {
		return spec.Query{}, "The query is empty."
	}
	text, reason := tb.rewriteAltinity(query, declared)
	if reason != "" {
		return spec.Query{}, reason
	}
	q := spec.Query{Query: text, Interval: tb.minInterval()}
	switch asString(tb.t["format"]) {
	case "table":
		q.Format = "table"
	case "logs":
		q.Format = "logs"
	case "traces", "flamegraph":
		return spec.Query{}, tracesDropped
	}
	if factor, ok := asFloat(tb.t["intervalFactor"]); ok && factor != 1 {
		tb.partial("intervalFactor", "The resolution factor of this query is not supported yet.")
	}
	if round := asString(tb.t["round"]); round != "" && round != "0s" {
		tb.partial("round", "Rounding of the time range is not supported yet.")
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
	panelDS := b.built[0].src.uid()
	b.panel.Datasource = panelDS
	for k, bq := range b.built {
		q := bq.q
		if refs[k] != spec.DefaultQueryRef(k) {
			q.Ref = refs[k]
		}
		if uid := bq.src.uid(); uid != panelDS {
			q.Datasource = uid
		}
		b.panel.Queries = append(b.panel.Queries, q)
		line := b.lineFor(bq.index)
		line.Ref = ptr(refs[k])
		line.Datasource = ptr(bq.src.uid())
		if bq.refID == "" {
			continue
		}
		if _, mapped := b.refs[bq.refID]; !mapped {
			b.refs[bq.refID] = refs[k]
		}
		if refs[k] != bq.refID {
			line.Rewrites = append(line.Rewrites, fmt.Sprintf("refId %s -> %s", bq.refID, refs[k]))
		}
	}
}

func (b *panelBuild) datasourceVariables() {
	for _, bq := range b.built {
		if bq.src.variable {
			b.c.noteDatasourceVariable(bq.src)
		}
	}
}

func (c *converter) noteDatasourceVariable(src *sourceDS) {
	if c.dsNoted[src] {
		return
	}
	c.dsNoted[src] = true
	name := strip(src.key)
	if src.match == "" {
		c.note("variable "+name, fmt.Sprintf("Datasource variable $%s has no match yet.", name))
		return
	}
	c.note("variable "+name, fmt.Sprintf("Datasource variable $%s now uses %s.", name, src.match))
}

var altinityDropped = map[string]bool{
	"columns": true, "columnsMs": true, "rate": true, "rateColumns": true, "perSecond": true,
	"perSecondColumns": true, "delta": true, "deltaColumns": true, "increase": true,
	"increaseColumns": true, "lttb": true, "adhoc": true, "timeFilterByColumn": true,
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

func (tb *targetBuild) rewriteAltinity(text string, declared map[string]bool) (string, string) {
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
		if altinityDropped[name] {
			return "", fmt.Sprintf("Altinity macro $%s builds the whole query and is not supported; rewrite it as SQL.", name)
		}
		replacement, next, ok := tb.altinityMacro(name, text, i, end)
		if !ok {
			replacement, next = text[i:end], end
		}
		out.WriteString(replacement)
		i = next
	}
	return out.String(), ""
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
