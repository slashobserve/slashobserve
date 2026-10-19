package macros

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

type Dialect int

const (
	PromQL Dialect = iota + 1
	ClickHouse
)

const (
	defaultScrapeInterval = 15 * time.Second
	allPlaceholder        = "x"
)

type Env struct {
	Range          datasource.TimeRange
	Interval       time.Duration
	ScrapeInterval time.Duration
	Variables      []datasource.Variable
	Dashboard      string
	User           datasource.QueryUser
}

type span struct {
	expStart, expEnd   int
	origStart, origEnd int
	fromVariable       bool
}

type Expansion struct {
	Text  string
	spans []span
}

func (e Expansion) Locate(start, end int) (origStart, origEnd int, inVariable bool) {
	end = max(end, start)
	for _, s := range e.spans {
		if s.fromVariable && s.touches(start, end) {
			inVariable = true
		}
	}
	origStart = e.mapOffset(start, false)
	if end == start {
		return origStart, origStart, inVariable
	}
	return origStart, e.mapOffset(end-1, true), inVariable
}

func (s span) touches(start, end int) bool {
	if s.expStart == s.expEnd {
		return start <= s.expStart && s.expStart <= end
	}
	return s.expStart < end && start < s.expEnd
}

func (e Expansion) mapOffset(offset int, exclusiveEnd bool) int {
	delta := 0
	for _, s := range e.spans {
		if s.expStart <= offset && offset < s.expEnd {
			if exclusiveEnd {
				return s.origEnd
			}
			return s.origStart
		}
		if s.expEnd <= offset {
			delta += (s.origEnd - s.origStart) - (s.expEnd - s.expStart)
		}
	}
	if exclusiveEnd {
		return offset + delta + 1
	}
	return offset + delta
}

func Expand(d Dialect, query string, env Env) (Expansion, error) {
	dsType, err := datasourceType(d)
	if err != nil {
		return Expansion{}, err
	}
	if err := validateEnv(env); err != nil {
		return Expansion{}, err
	}
	return expandText(d, dsType, query, env)
}

func expandText(d Dialect, dsType spec.DatasourceType, query string, env Env) (Expansion, error) {
	refs, err := spec.ScanQuery(query)
	if err != nil {
		return Expansion{}, scanFailure(err)
	}
	lx, err := newLexer(d, query, refs)
	if err != nil {
		return Expansion{}, err
	}
	exp, err := substitute(d, dsType, query, refs, lx, env)
	if err != nil {
		return Expansion{}, err
	}
	if err := verifyExpansion(d, query, lx.units, exp); err != nil {
		return Expansion{}, err
	}
	return exp, nil
}

func validateEnv(env Env) error {
	if err := env.Range.Validate(); err != nil {
		return err
	}
	if env.Interval <= 0 {
		return datasource.Errorf(datasource.ErrInvalidQuery, "interval must be positive")
	}
	return nil
}

func substitute(d Dialect, dsType spec.DatasourceType, query string, refs []spec.Reference, lx lexer, env Env) (Expansion, error) {
	var b strings.Builder
	var spans []span
	last := 0
	for _, ref := range refs {
		if lx.startsInDelimiter(ref) {
			continue
		}
		c := lx.contextAt(ref.Start)
		if !staysWithinOneContext(lx, ref, c) {
			return Expansion{}, refErrorf(ref, "reference %s crosses a string, comment or identifier boundary", refLabel(ref))
		}
		if c.kind == ctxComment {
			continue
		}
		out, err := expandRef(d, dsType, query, ref, c, env)
		if err != nil {
			return Expansion{}, err
		}
		b.WriteString(query[last:ref.Start])
		expStart := b.Len()
		b.WriteString(out)
		spans = append(spans, span{
			expStart:     expStart,
			expEnd:       b.Len(),
			origStart:    ref.Start,
			origEnd:      ref.End,
			fromVariable: ref.Kind == spec.VariableRef || isGlobalVariable(ref.Name),
		})
		last = ref.End
	}
	b.WriteString(query[last:])
	return Expansion{Text: b.String(), spans: spans}, nil
}

func staysWithinOneContext(lx lexer, ref spec.Reference, start ctx) bool {
	end := lx.contextAt(ref.End - 1)
	if macro, ok := spec.LookupMacro(ref.Name); ok && ref.Kind == spec.MacroRef && macro.Args == spec.ConditionAndVariable {
		if start.kind == ctxCode && end.kind == ctxCode {
			return true
		}
	}
	return end.region == start.region
}

func expandRef(d Dialect, dsType spec.DatasourceType, query string, ref spec.Reference, c ctx, env Env) (string, error) {
	if c.kind == ctxOpaque {
		return "", refErrorf(ref, "reference %s is not allowed in a heredoc or unicode quoted literal", refLabel(ref))
	}
	if unquotedCode(d, ref, c) && fusesWithNeighbors(d, query, ref) {
		return "", refErrorf(ref, "reference %s must not touch a word character, \".\" or \"$\"", refLabel(ref))
	}
	if ref.Kind == spec.MacroRef {
		return expandMacro(d, dsType, query, ref, c, env)
	}
	return expandVariable(d, query, ref, c, env.Variables)
}

func ExpandForLint(d Dialect, query string) (Expansion, error) {
	refs, err := spec.ScanQuery(query)
	if err != nil {
		return Expansion{}, scanFailure(err)
	}
	var vars []datasource.Variable
	for _, ref := range spec.FlattenReferences(refs) {
		if ref.Kind != spec.VariableRef || slices.ContainsFunc(vars, func(v datasource.Variable) bool { return v.Name == ref.Name }) {
			continue
		}
		vars = append(vars, datasource.Variable{Name: ref.Name, Values: []string{"x"}})
	}
	now := time.Now()
	return Expand(d, query, Env{
		Range:          datasource.TimeRange{From: now.Add(-time.Hour), To: now},
		Interval:       time.Minute,
		ScrapeInterval: defaultScrapeInterval,
		Variables:      vars,
		Dashboard:      "dashboard",
		User:           datasource.QueryUser{ID: "user", Login: "user", Email: "user@example.com", Name: "user"},
	})
}

func datasourceType(d Dialect) (spec.DatasourceType, error) {
	switch d {
	case PromQL:
		return spec.Prometheus, nil
	case ClickHouse:
		return spec.ClickHouse, nil
	}
	return "", datasource.Errorf(datasource.ErrInvalidQuery, "unknown dialect %d", int(d))
}

func refLabel(ref spec.Reference) string {
	return strconv.Quote("$" + ref.Name)
}

func refErrorf(ref spec.Reference, format string, args ...any) error {
	return offsetErrorf(ref.Start, fmt.Sprintf(format, args...)+" at offset ", "")
}

type offsetError struct {
	*datasource.UpstreamError
	before, after string
	offset        int
}

func offsetErrorf(offset int, before, after string) error {
	return &offsetError{
		UpstreamError: &datasource.UpstreamError{Kind: datasource.ErrInvalidQuery, Message: before + strconv.Itoa(offset) + after},
		before:        before,
		after:         after,
		offset:        offset,
	}
}

func (e *offsetError) Unwrap() error { return e.UpstreamError }

func shiftOffset(err error, by int) error {
	var oe *offsetError
	if errors.As(err, &oe) {
		return offsetErrorf(oe.offset+by, oe.before, oe.after)
	}
	return err
}

func scanFailure(err error) error {
	var se *spec.ScanError
	if errors.As(err, &se) {
		return offsetErrorf(se.Offset, se.Prefix, "")
	}
	return datasource.Errorf(datasource.ErrInvalidQuery, "%s", err)
}

func expandMacro(d Dialect, dsType spec.DatasourceType, query string, ref spec.Reference, c ctx, env Env) (string, error) {
	macro, args, err := validateMacro(dsType, ref, c)
	if err != nil {
		return "", err
	}
	if value, ok := globalVariable(ref.Name, env); ok {
		return expandVariable(d, query, ref, c, []datasource.Variable{{Name: ref.Name, Values: []string{value}}})
	}
	if out, ok, err := globalMacro(ref, env); ok || err != nil {
		return out, err
	}
	if macro.Args == spec.ConditionAndVariable {
		return expandConditionalAll(d, dsType, ref, args, env)
	}
	var out string
	var ok bool
	switch d {
	case PromQL:
		out, ok = promMacro(ref.Name, env)
	case ClickHouse:
		out, ok = clickHouseMacro(ref.Name, args.Values, env)
	}
	if !ok {
		return "", refErrorf(ref, "macro %s is not implemented", refLabel(ref))
	}
	return out, nil
}

func validateMacro(dsType spec.DatasourceType, ref spec.Reference, c ctx) (spec.Macro, spec.MacroArguments, error) {
	idx := slices.IndexFunc(spec.Macros(dsType), func(m spec.Macro) bool { return m.Name == ref.Name })
	if idx < 0 {
		return spec.Macro{}, spec.MacroArguments{}, refErrorf(ref, "unknown macro %s", refLabel(ref))
	}
	macro := spec.Macros(dsType)[idx]
	if ref.Format != "" {
		if err := macro.CheckFormat(ref.Format); err != nil {
			return macro, spec.MacroArguments{}, refErrorf(ref, "%s", err)
		}
	}
	args, err := macro.ParseArguments(ref)
	if err != nil {
		return macro, spec.MacroArguments{}, refErrorf(ref, "%s", err)
	}
	switch {
	case !macro.Function():
		if c.kind != ctxCode && c.kind != ctxString {
			return macro, spec.MacroArguments{}, refErrorf(ref, "macro %s is not allowed in a quoted identifier", refLabel(ref))
		}
	case c.kind != ctxCode:
		return macro, spec.MacroArguments{}, refErrorf(ref, "macro %s is only allowed in code", refLabel(ref))
	}
	return macro, args, nil
}

func expandConditionalAll(d Dialect, dsType spec.DatasourceType, ref spec.Reference, args spec.MacroArguments, env Env) (string, error) {
	condition, name := args.Values[0], args.Values[1]
	idx := slices.IndexFunc(env.Variables, func(v datasource.Variable) bool { return v.Name == name })
	if idx < 0 {
		return "", refErrorf(ref, "unknown variable %s", strconv.Quote("$"+name))
	}
	v := env.Variables[idx]
	if !v.All && len(v.Values) == 0 {
		return "", refErrorf(ref, "no value selected for %s", strconv.Quote("$"+name))
	}
	probe := env
	if v.All {
		probe.Variables = slices.Clone(env.Variables)
		probe.Variables[idx] = datasource.Variable{Name: name, Values: []string{allPlaceholder}}
	}
	exp, err := expandText(d, dsType, condition, probe)
	if err != nil {
		return "", shiftOffset(err, ref.Start+1+len(ref.Name)+1+args.ConditionOffset)
	}
	out := "(" + exp.Text + ")"
	if !wrapsExactly(out) {
		return "", refErrorf(ref, "the condition of %s must have balanced parentheses and no comment that reaches the end of it", refLabel(ref))
	}
	if v.All {
		return "1=1", nil
	}
	return out, nil
}

func wrapsExactly(text string) bool {
	tokens, err := TokenizeClickHouse(text)
	if err != nil {
		return false
	}
	depth := 0
	for i, tok := range tokens {
		if tok.Kind != TokenPunct {
			continue
		}
		switch text[tok.Start:tok.End] {
		case "(":
			depth++
		case ")":
			depth--
		}
		if depth < 0 || (depth == 0 && i < len(tokens)-1) {
			return false
		}
	}
	return depth == 0
}

var globalValues = map[string]func(Env) string{
	"__dashboard":  func(env Env) string { return env.Dashboard },
	"__user.login": func(env Env) string { return env.User.Login },
	"__user.email": func(env Env) string { return env.User.Email },
	"__user.name":  func(env Env) string { return env.User.Name },
	"__user.id":    func(env Env) string { return env.User.ID },
}

func isGlobalVariable(name string) bool {
	macro, ok := spec.LookupMacro(name)
	return ok && macro.Global
}

func globalVariable(name string, env Env) (string, bool) {
	value, ok := globalValues[name]
	if !ok || !isGlobalVariable(name) {
		return "", false
	}
	return value(env), true
}

func globalMacro(ref spec.Reference, env Env) (string, bool, error) {
	switch ref.Name {
	case "__interval_ms":
		return strconv.FormatInt(max(env.Interval.Milliseconds(), 1), 10), true, nil
	case "__range_ms":
		return strconv.FormatInt(env.Range.To.Sub(env.Range.From).Milliseconds(), 10), true, nil
	case "__range_s":
		return strconv.FormatInt((env.Range.To.Sub(env.Range.From).Milliseconds()+500)/1000, 10), true, nil
	case "__from", "__to":
		at := env.Range.From
		if ref.Name == "__to" {
			at = env.Range.To
		}
		out, err := spec.FormatTimeMacro(at, ref.Format)
		if err != nil {
			return "", false, refErrorf(ref, "invalid date format %q for macro %s", ref.Format, refLabel(ref))
		}
		return out, true, nil
	}
	return "", false, nil
}

func promMacro(name string, env Env) (string, bool) {
	switch name {
	case "__interval":
		return promDuration(env.Interval), true
	case "__rate_interval":
		scrape := env.ScrapeInterval
		if scrape <= 0 {
			scrape = defaultScrapeInterval
		}
		return promDuration(max(env.Interval+scrape, 4*scrape)), true
	case "__range":
		return promDuration(env.Range.To.Sub(env.Range.From)), true
	}
	return "", false
}

func clickHouseMacro(name string, args []string, env Env) (string, bool) {
	from, to := env.Range.From, env.Range.To
	switch name {
	case "__interval":
		return strconv.FormatInt(intervalSeconds(env.Interval), 10), true
	case "__interval_s":
		return strconv.FormatInt(wholeSeconds(env.Interval), 10), true
	case "__timeFilter", "__timeFilter_ms":
		return timeFilter(args[0], env), true
	case "__timeGroup":
		return fmt.Sprintf("toStartOfInterval(%s, INTERVAL %d SECOND)", args[0], intervalSeconds(env.Interval)), true
	case "__dateFilter":
		return dateFilter(args[0], env), true
	case "__dateTimeFilter", "__dt":
		return fmt.Sprintf("(%s AND %s)", dateFilter(args[0], env), timeFilter(args[1], env)), true
	case "__fromTime":
		return fmt.Sprintf("toDateTime(%d)", from.Unix()), true
	case "__toTime":
		return fmt.Sprintf("toDateTime(%d)", to.Unix()), true
	case "__fromTime_ms":
		return fmt.Sprintf("fromUnixTimestamp64Milli(toInt64(%d))", from.UnixMilli()), true
	case "__toTime_ms":
		return fmt.Sprintf("fromUnixTimestamp64Milli(toInt64(%d))", to.UnixMilli()), true
	case "__timeInterval":
		return fmt.Sprintf("toStartOfInterval(toDateTime(%s), INTERVAL %d second)", args[0], wholeSeconds(env.Interval)), true
	case "__timeInterval_ms":
		return fmt.Sprintf("toStartOfInterval(toDateTime64(%s, 3), INTERVAL %d millisecond)", args[0], max(env.Interval.Milliseconds(), 1)), true
	}
	return "", false
}

func timeFilter(column string, env Env) string {
	return fmt.Sprintf("(%[1]s >= fromUnixTimestamp64Milli(toInt64(%[2]d), 'UTC') AND %[1]s <= fromUnixTimestamp64Milli(toInt64(%[3]d), 'UTC'))",
		column, env.Range.From.UnixMilli(), env.Range.To.UnixMilli())
}

func dateFilter(column string, env Env) string {
	const layout = "2006-01-02"
	return fmt.Sprintf("(%[1]s >= toDate('%[2]s') AND %[1]s <= toDate('%[3]s'))",
		column, env.Range.From.UTC().Format(layout), env.Range.To.UTC().Format(layout))
}

func promDuration(d time.Duration) string {
	ms := max(d.Milliseconds(), 1)
	if ms%1000 == 0 {
		return strconv.FormatInt(ms/1000, 10) + "s"
	}
	return strconv.FormatInt(ms, 10) + "ms"
}

func wholeSeconds(d time.Duration) int64 {
	return max(int64(d/time.Second), 1)
}

func intervalSeconds(d time.Duration) int64 {
	secs := int64((d + time.Second - 1) / time.Second)
	return max(secs, 1)
}
