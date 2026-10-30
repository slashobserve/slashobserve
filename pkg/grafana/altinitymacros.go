package grafana

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

var (
	identifierPattern  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	aliasedPattern     = regexp.MustCompile(`(?is)^(.*\S)\s+as\s+([A-Za-z_][A-Za-z0-9_]*)$`)
	spacedAliasPattern = regexp.MustCompile(`(?s)^(.*\S)\s+([A-Za-z_][A-Za-z0-9_]*)$`)
	fromKeyword        = regexp.MustCompile(`(?i)^from\b`)
	timeFilterPresent  = map[string]*regexp.Regexp{
		"timeFilter":   regexp.MustCompile(`\$timeFilter\b`),
		"timeFilterMs": regexp.MustCompile(`\$timeFilterMs\b`),
	}
)

var clauseWords = map[string]bool{
	"group": true, "having": true, "qualify": true, "window": true, "order": true, "limit": true,
	"offset": true, "settings": true, "union": true, "intersect": true, "except": true,
}

var tailClauses = []string{"group", "having", "order"}

var danglingWords = map[string]bool{"and": true, "or": true, "not": true, "by": true, "as": true, "in": true, "like": true, "is": true}

var counterDerivations = map[string]struct {
	suffix string
	derive func(column string) string
}{
	"perSecond": {"PerSecond", perSecondOf},
	"delta":     {"Delta", lagDifference},
	"increase":  {"Increase", increaseOf},
}

const (
	epochOfT        = "toInt64(toUnixTimestamp(t))"
	previousEpoch   = "lagInFrame(" + epochOfT + ", 1, " + epochOfT + ") OVER w"
	stepsSincePrior = epochOfT + " - lagInFrame(" + epochOfT + ", 1, " + epochOfT + " - $__interval_s) OVER w"
)

type sqlWord struct {
	text       string
	start, end int
	depth      int
}

type altinityColumn struct {
	expr, alias, text string
}

func isSQLIdentByte(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

func skipQuotedSQL(s string, open int) int {
	quote := s[open]
	for j := open + 1; j < len(s); j++ {
		switch {
		case s[j] == '\\':
			j++
		case s[j] == quote && j+1 < len(s) && s[j+1] == quote:
			j++
		case s[j] == quote:
			return j
		}
	}
	return len(s)
}

func skipSQLOpaque(s string, i int) (int, bool) {
	rest := s[i:]
	switch {
	case strings.HasPrefix(rest, "--"), strings.HasPrefix(rest, "# "), strings.HasPrefix(rest, "#!"):
		if end := strings.IndexByte(rest, '\n'); end >= 0 {
			return i + end, true
		}
		return len(s), true
	case strings.HasPrefix(rest, "/*"):
		if end := strings.Index(rest[2:], "*/"); end >= 0 {
			return i + 2 + end + 1, true
		}
		return len(s), true
	case rest[0] == '\'', rest[0] == '"', rest[0] == '`':
		return skipQuotedSQL(s, i), true
	}
	return i, false
}

func sqlWords(s string) []sqlWord {
	var words []sqlWord
	depth := 0
	for i := 0; i < len(s); i++ {
		if end, skipped := skipSQLOpaque(s, i); skipped {
			i = end
			continue
		}
		c := s[i]
		switch {
		case c == '(' || c == '[':
			depth++
		case c == ')' || c == ']':
			if c == ')' {
				words = append(words, sqlWord{")", i, i + 1, depth})
			}
			depth--
		case c == '$':
			i = skipReferenceName(s, i)
		case (c == '_' || unicode.IsLetter(rune(c))) && (i == 0 || !isSQLIdentByte(s[i-1])):
			j := i
			for j < len(s) && isSQLIdentByte(s[j]) {
				j++
			}
			words = append(words, sqlWord{strings.ToLower(s[i:j]), i, j, depth})
			i = j - 1
		}
	}
	return words
}

func skipReferenceName(s string, dollar int) int {
	if dollar+1 < len(s) && s[dollar+1] == '{' {
		if end := strings.IndexByte(s[dollar:], '}'); end >= 0 {
			return dollar + end
		}
		return len(s)
	}
	j := dollar + 1
	for j < len(s) && isSQLIdentByte(s[j]) {
		j++
	}
	return j - 1
}

func closingParen(s string, open int) int {
	depth := 0
	for i := open; i < len(s); i++ {
		if end, skipped := skipSQLOpaque(s, i); skipped {
			i = end
			continue
		}
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func splitAltinityCall(text string, open int) ([]string, string, bool) {
	closing := closingParen(text, open)
	if closing < 0 {
		return nil, "", false
	}
	rest := strings.TrimLeftFunc(text[closing+1:], unicode.IsSpace)
	if !fromKeyword.MatchString(rest) {
		return nil, "", false
	}
	return splitCallArguments(text[open+1 : closing]), rest, true
}

func splitCallArguments(inner string) []string {
	var args []string
	depth, from := 0, 0
	for i := 0; i < len(inner); i++ {
		if end, skipped := skipSQLOpaque(inner, i); skipped {
			i = end
			continue
		}
		switch inner[i] {
		case '$':
			i = skipReferenceName(inner, i)
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case ',':
			if depth == 0 {
				args = append(args, strings.TrimSpace(inner[from:i]))
				from = i + 1
			}
		}
	}
	return append(args, strings.TrimSpace(inner[min(from, len(inner)):]))
}

func parseAltinityColumn(arg string) (altinityColumn, bool) {
	arg = strings.TrimSpace(arg)
	if identifierPattern.MatchString(arg) {
		return altinityColumn{arg, arg, arg}, true
	}
	if m := aliasedPattern.FindStringSubmatch(arg); m != nil {
		return altinityColumn{m[1], m[2], arg}, true
	}
	if m := spacedAliasPattern.FindStringSubmatch(arg); m != nil && !dangling(m[1]) {
		return altinityColumn{m[1], m[2], arg}, true
	}
	return altinityColumn{}, false
}

func dangling(expr string) bool {
	if strings.ContainsRune("+-*/%=<>,|&!.", rune(expr[len(expr)-1])) {
		return true
	}
	words := strings.Fields(strings.ToLower(expr))
	return danglingWords[words[len(words)-1]]
}

func parseAltinityColumns(args []string) ([]altinityColumn, bool) {
	columns := make([]altinityColumn, len(args))
	for i, arg := range args {
		column, ok := parseAltinityColumn(arg)
		if !ok {
			return nil, false
		}
		columns[i] = column
	}
	return columns, true
}

func splitSourceTail(rest string) (string, string) {
	for _, w := range sqlWords(rest) {
		if w.depth == 0 && clauseWords[w.text] {
			return rest[:w.start], rest[w.start:]
		}
	}
	return rest, ""
}

type sourceTail struct {
	groupBy, having, orderBy string
}

func parseSourceTail(tail string) (sourceTail, bool) {
	var found sourceTail
	if tail == "" {
		return found, true
	}
	type boundary struct {
		clause string
		start  int
	}
	var boundaries []boundary
	for _, w := range sqlWords(tail) {
		if w.depth != 0 || !clauseWords[w.text] {
			continue
		}
		boundaries = append(boundaries, boundary{w.text, w.start})
	}
	next := 0
	for i, b := range boundaries {
		for next < len(tailClauses) && tailClauses[next] != b.clause {
			next++
		}
		if next == len(tailClauses) {
			return sourceTail{}, false
		}
		end := len(tail)
		if i+1 < len(boundaries) {
			end = boundaries[i+1].start
		}
		text := strings.TrimRight(tail[b.start:end], " \t\r\n")
		switch b.clause {
		case "group":
			found.groupBy = text
		case "having":
			found.having = text
		case "order":
			found.orderBy = text
		}
		next++
	}
	return found, true
}

func withTimeFilter(body, filter, macro string) string {
	words := sqlWords(body)
	shallowest := -1
	for _, w := range words {
		if w.text == "where" && (shallowest < 0 || w.depth < shallowest) {
			shallowest = w.depth
		}
	}
	if shallowest < 0 {
		return strings.TrimRight(body, " \t\r\n") + "\nWHERE " + filter
	}
	for k := len(words) - 1; k >= 0; k-- {
		w := words[k]
		if w.text != "where" || w.depth != shallowest {
			continue
		}
		stop, hasOr := conditionEnd(body, words, k)
		condition := body[w.end:stop]
		if timeFilterPresent[macro].MatchString(condition) {
			continue
		}
		body = body[:w.end] + guardedCondition(filter, condition, hasOr) + body[stop:]
	}
	return body
}

func conditionEnd(body string, words []sqlWord, where int) (int, bool) {
	hasOr := false
	for _, w := range words[where+1:] {
		switch {
		case w.depth == words[where].depth && (clauseWords[w.text] || w.text == ")"):
			return w.start, hasOr
		case w.depth == words[where].depth && w.text == "or":
			hasOr = true
		}
	}
	return len(body), hasOr
}

func guardedCondition(filter, condition string, wrap bool) string {
	lead := ""
	if condition == "" || !unicode.IsSpace(rune(condition[0])) {
		lead = " "
	}
	if !wrap {
		return " " + filter + " AND" + lead + condition
	}
	trimmed := strings.TrimRight(condition, " \t\r\n")
	trailing := condition[len(trimmed):]
	if trailing == "" {
		trailing = " "
	}
	return " " + filter + " AND (" + trimmed + "\n)" + trailing
}

func lagDifference(column string) string {
	return column + " - lagInFrame(" + column + ", 1, " + column + ") OVER w"
}

func perSecondOf(column string) string {
	difference := lagDifference(column)
	return "if((" + difference + ") < 0, nan, (" + difference + ") / (" + epochOfT + " - " + previousEpoch + "))"
}

func increaseOf(column string) string {
	difference := lagDifference(column)
	return "if((" + difference + ") < 0, 0, " + difference + ")"
}

func rateOf(column string) string {
	return column + " / (" + stepsSincePrior + ")"
}

type altinitySource struct {
	series, from string
	tail         sourceTail
	tailText     string
}

func (tb *targetBuild) altinitySource(rest string, ms bool) (altinitySource, bool) {
	seriesName, filterName := "timeSeries", "timeFilter"
	if ms {
		seriesName, filterName = "timeSeriesMs", "timeFilterMs"
	}
	series, seriesOK := tb.altinitySimple(seriesName)
	filter, filterOK := tb.altinitySimple(filterName)
	if !seriesOK || !filterOK {
		return altinitySource{}, false
	}
	body, tailText := splitSourceTail(rest)
	tail, tailOK := parseSourceTail(tailText)
	if !tailOK {
		return altinitySource{}, false
	}
	return altinitySource{
		series:   series,
		from:     withTimeFilter(body, filter, filterName),
		tail:     tail,
		tailText: tailText,
	}, true
}

func (s altinitySource) plain() bool {
	return s.tailText == ""
}

func (s altinitySource) havingOnly() bool {
	return s.tail.groupBy == "" && s.tail.orderBy == ""
}

func (tb *targetBuild) expandAltinity(name string, args []string, rest string) (string, bool) {
	switch name {
	case "columns", "columnsMs":
		return tb.expandColumns(name == "columnsMs", args, rest)
	case "lttb":
		return tb.expandLTTB(args, rest)
	}
	base, form := counterFamily(name)
	switch form {
	case "":
		return tb.expandCounter(base, args, rest)
	case "Columns":
		return tb.expandCounterColumns(base, args, rest)
	case "ColumnsAggregated":
		return tb.expandCounterAggregated(base, args, rest)
	}
	return "", false
}

func counterFamily(name string) (string, string) {
	if base, ok := strings.CutSuffix(name, "ColumnsAggregated"); ok {
		return base, "ColumnsAggregated"
	}
	if base, ok := strings.CutSuffix(name, "Columns"); ok {
		return base, "Columns"
	}
	return name, ""
}

func isCounterMacro(name string) bool {
	base, _ := counterFamily(name)
	_, derived := counterDerivations[base]
	return base == "rate" || derived
}

func derivedValue(base, column string) (string, string) {
	if base == "rate" {
		return rateOf(column), "Rate"
	}
	kind := counterDerivations[base]
	return kind.derive(column), kind.suffix
}

func (tb *targetBuild) expandCounter(base string, args []string, rest string) (string, bool) {
	source, ok := tb.altinitySource(rest, false)
	if !ok || !source.plain() || len(args) < 1 {
		return "", false
	}
	var inner, derived []string
	if base == "rate" {
		columns, ok := parseAltinityColumns(args)
		if !ok {
			return "", false
		}
		for _, column := range columns {
			value, suffix := derivedValue(base, column.alias)
			inner = append(inner, column.text)
			derived = append(derived, value+" AS "+column.alias+suffix)
		}
	} else {
		for i, arg := range args {
			alias := fmt.Sprintf("max_%d", i)
			value, suffix := derivedValue(base, alias)
			inner = append(inner, "toFloat64(max("+arg+")) AS "+alias)
			derived = append(derived, value+" AS "+alias+"_"+suffix)
		}
	}
	return "SELECT t, " + strings.Join(derived, ", ") +
		" FROM (SELECT " + source.series + " AS t, " + strings.Join(inner, ", ") +
		"\n" + source.from + "\nGROUP BY t ORDER BY t)" +
		" WINDOW w AS (ORDER BY t) ORDER BY t", true
}

func (tb *targetBuild) expandColumns(ms bool, args []string, rest string) (string, bool) {
	source, ok := tb.altinitySource(rest, ms)
	if !ok || len(args) != 2 {
		return "", false
	}
	columns, ok := parseAltinityColumns(args)
	if !ok {
		return "", false
	}
	key, value := columns[0], columns[1]
	return "SELECT t, toString(" + key.alias + ") AS " + key.alias + ", " + value.alias + " AS Value" +
		" FROM (" + keyedSelect(source, key, value.text) + ") ORDER BY t, " + key.alias, true
}

func keyedSelect(source altinitySource, key altinityColumn, value string) string {
	groupBy, orderBy := source.tail.groupBy, source.tail.orderBy
	if groupBy == "" {
		groupBy = "GROUP BY t, " + key.alias
	}
	if orderBy == "" {
		orderBy = "ORDER BY t, " + key.alias
	}
	having := ""
	if source.tail.having != "" {
		having = "\n" + source.tail.having
	}
	return "SELECT " + source.series + " AS t, " + key.text + ", " + value +
		"\n" + source.from + "\n" + groupBy + having + "\n" + orderBy
}

func (tb *targetBuild) expandCounterColumns(base string, args []string, rest string) (string, bool) {
	source, ok := tb.altinitySource(rest, false)
	if !ok || len(args) != 2 {
		return "", false
	}
	key, ok := parseAltinityColumn(args[0])
	if !ok {
		return "", false
	}
	partitioned := " WINDOW w AS (PARTITION BY " + key.alias + " ORDER BY t) ORDER BY t, " + key.alias
	if base == "rate" {
		value, ok := parseAltinityColumn(args[1])
		if !ok {
			return "", false
		}
		derived, _ := derivedValue(base, value.alias)
		return "SELECT t, toString(" + key.alias + ") AS " + key.alias + ", " + derived + " AS Value" +
			" FROM (" + keyedSelect(source, key, value.text) + ")" + partitioned, true
	}
	if !source.havingOnly() {
		return "", false
	}
	having := ""
	if source.tail.having != "" {
		having = "\n" + source.tail.having
	}
	derived, _ := derivedValue(base, "max_0")
	return "SELECT t, toString(" + key.alias + ") AS " + key.alias + ", " + derived + " AS Value" +
		" FROM (SELECT " + source.series + " AS t, " + key.text + ", toFloat64(max(" + strings.TrimSpace(args[1]) + ")) AS max_0" +
		"\n" + source.from + "\nGROUP BY t, " + key.alias + having + "\nORDER BY " + key.alias + ", t)" + partitioned, true
}

func (tb *targetBuild) expandCounterAggregated(base string, args []string, rest string) (string, bool) {
	source, ok := tb.altinitySource(rest, false)
	if !ok || !source.havingOnly() || len(args) < 4 || len(args)%2 != 0 {
		return "", false
	}
	key, keyOK := parseAltinityColumn(args[0])
	subKey, subKeyOK := parseAltinityColumn(args[1])
	if !keyOK || !subKeyOK {
		return "", false
	}
	var values, derived, aggregated []string
	for i := 2; i < len(args); i += 2 {
		function := strings.TrimSpace(args[i])
		value, ok := parseAltinityColumn(args[i+1])
		if !ok || !identifierPattern.MatchString(function) {
			return "", false
		}
		expr := value.expr
		if !strings.Contains(expr, "(") {
			expr = "max(" + expr + ")"
		}
		if base != "rate" {
			expr = "toFloat64(" + expr + ")"
		}
		derivedValue, suffix := derivedValue(base, value.alias)
		values = append(values, expr+" AS "+value.alias)
		derived = append(derived, derivedValue+" AS "+value.alias+suffix)
		aggregated = append(aggregated, function+"("+value.alias+suffix+") AS "+value.alias+suffix+"Agg")
	}
	having := ""
	if source.tail.having != "" {
		having = "\n" + source.tail.having
	}
	series := key.alias + ", " + subKey.alias
	inner := "SELECT " + source.series + " AS t, " + key.text + ", " + subKey.text + ", " + strings.Join(values, ", ") +
		"\n" + source.from + "\nGROUP BY " + series + ", t" + having + "\nORDER BY " + series + ", t"
	middle := "SELECT t, " + series + ", " + strings.Join(derived, ", ") +
		" FROM (" + inner + ") WINDOW w AS (PARTITION BY " + series + " ORDER BY t)"
	return "SELECT t, toString(" + key.alias + ") AS " + key.alias + ", " + strings.Join(aggregated, ", ") +
		" FROM (" + middle + ") GROUP BY " + key.alias + ", t ORDER BY t, " + key.alias, true
}

func (tb *targetBuild) expandLTTB(args []string, rest string) (string, bool) {
	source, ok := tb.altinitySource(rest, false)
	if !ok || len(args) < 3 {
		return "", false
	}
	columns, ok := parseAltinityColumns(args[1:])
	if !ok {
		return "", false
	}
	buckets := strings.TrimSpace(args[0])
	if strings.EqualFold(buckets, "auto") {
		buckets = "toUInt64((${__to:date:seconds} - ${__from:date:seconds}) / $__interval_s)"
	}
	extras, x, y := columns[:len(columns)-2], columns[len(columns)-2], columns[len(columns)-1]
	var inner, outer []string
	outer = append(outer, "point.1 AS "+x.alias)
	for _, extra := range extras {
		inner = append(inner, extra.text)
		outer = append(outer, extra.alias)
	}
	outer = append(outer, "point.2 AS "+y.alias)
	inner = append(inner, "arrayJoin(largestTriangleThreeBuckets("+buckets+")("+x.expr+", "+y.expr+")) AS point")
	return "SELECT " + strings.Join(outer, ", ") +
		" FROM (SELECT " + strings.Join(inner, ", ") + "\n" + source.from + "\n" + source.tailText + ") ORDER BY " + x.alias, true
}

func blankOrComments(s string) bool {
	for i := 0; i < len(s); {
		rest := s[i:]
		switch {
		case unicode.IsSpace(rune(s[i])):
			i++
		case strings.HasPrefix(rest, "--"), strings.HasPrefix(rest, "# "):
			end := strings.IndexByte(rest, '\n')
			if end < 0 {
				return true
			}
			i += end
		case strings.HasPrefix(rest, "/*"):
			end := strings.Index(rest[2:], "*/")
			if end < 0 {
				return false
			}
			i += end + 4
		default:
			return false
		}
	}
	return true
}
