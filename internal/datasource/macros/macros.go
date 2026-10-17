package macros

import (
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

const defaultScrapeInterval = 15 * time.Second

type Env struct {
	Range          datasource.TimeRange
	Interval       time.Duration
	ScrapeInterval time.Duration
	Variables      []datasource.Variable
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
	if err := env.Range.Validate(); err != nil {
		return Expansion{}, err
	}
	if env.Interval <= 0 {
		return Expansion{}, datasource.Errorf(datasource.ErrInvalidQuery, "interval must be positive")
	}
	refs, err := spec.ScanQuery(query)
	if err != nil {
		return Expansion{}, datasource.Errorf(datasource.ErrInvalidQuery, "%s", err)
	}
	lx := newLexer(d, query)
	var b strings.Builder
	var spans []span
	last := 0
	for _, ref := range refs {
		c := lx.contextAt(ref.Start)
		if lx.contextAt(ref.End-1).region != c.region {
			return Expansion{}, refErrorf(ref, "reference %s crosses a string, comment or identifier boundary", refLabel(ref))
		}
		if c.kind == ctxComment {
			continue
		}
		var out string
		switch ref.Kind {
		case spec.MacroRef:
			out, err = expandMacro(d, dsType, ref, c, env)
		default:
			out, err = expandVariable(d, query, ref, c, env.Variables)
		}
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
			fromVariable: ref.Kind == spec.VariableRef,
		})
		last = ref.End
	}
	b.WriteString(query[last:])
	return Expansion{Text: b.String(), spans: spans}, nil
}

func ExpandForLint(d Dialect, query string) (Expansion, error) {
	refs, err := spec.ScanQuery(query)
	if err != nil {
		return Expansion{}, datasource.Errorf(datasource.ErrInvalidQuery, "%s", err)
	}
	var vars []datasource.Variable
	for _, ref := range refs {
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
	return datasource.Errorf(datasource.ErrInvalidQuery, "%s at offset %d", fmt.Sprintf(format, args...), ref.Start)
}

func expandMacro(d Dialect, dsType spec.DatasourceType, ref spec.Reference, c ctx, env Env) (string, error) {
	args, err := validateMacro(dsType, ref, c)
	if err != nil {
		return "", err
	}
	var out string
	var ok bool
	switch d {
	case PromQL:
		out, ok = promMacro(ref.Name, env)
	case ClickHouse:
		out, ok = clickHouseMacro(ref.Name, args, env)
	}
	if !ok {
		return "", refErrorf(ref, "macro %s is not implemented", refLabel(ref))
	}
	return out, nil
}

func validateMacro(dsType spec.DatasourceType, ref spec.Reference, c ctx) (string, error) {
	idx := slices.IndexFunc(spec.Macros(dsType), func(m spec.Macro) bool { return m.Name == ref.Name })
	if idx < 0 {
		return "", refErrorf(ref, "unknown macro %s", refLabel(ref))
	}
	if ref.Format != "" {
		return "", refErrorf(ref, "macro %s does not take a format", refLabel(ref))
	}
	args := strings.TrimSpace(ref.Args)
	if !spec.Macros(dsType)[idx].Function {
		switch {
		case ref.HasArgs:
			return "", refErrorf(ref, "macro %s does not take arguments", refLabel(ref))
		case c.kind != ctxCode && c.kind != ctxString:
			return "", refErrorf(ref, "macro %s is not allowed in a quoted identifier", refLabel(ref))
		}
		return "", nil
	}
	switch {
	case !ref.HasArgs || args == "":
		return "", refErrorf(ref, "macro %s requires an argument", refLabel(ref))
	case strings.ContainsAny(args, "$;"):
		return "", refErrorf(ref, "macro %s argument must not contain \"$\" or \";\"", refLabel(ref))
	case c.kind != ctxCode:
		return "", refErrorf(ref, "macro %s is only allowed in code", refLabel(ref))
	}
	return args, nil
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

func clickHouseMacro(name, args string, env Env) (string, bool) {
	switch name {
	case "__interval":
		return strconv.FormatInt(intervalSeconds(env.Interval), 10), true
	case "__timeFilter":
		return fmt.Sprintf("(%[1]s >= fromUnixTimestamp64Milli(toInt64(%[2]d), 'UTC') AND %[1]s <= fromUnixTimestamp64Milli(toInt64(%[3]d), 'UTC'))",
			args, env.Range.From.UnixMilli(), env.Range.To.UnixMilli()), true
	case "__timeGroup":
		return fmt.Sprintf("toStartOfInterval(%s, INTERVAL %d SECOND)", args, intervalSeconds(env.Interval)), true
	}
	return "", false
}

func promDuration(d time.Duration) string {
	ms := max(d.Milliseconds(), 1)
	if ms%1000 == 0 {
		return strconv.FormatInt(ms/1000, 10) + "s"
	}
	return strconv.FormatInt(ms, 10) + "ms"
}

func intervalSeconds(d time.Duration) int64 {
	secs := int64((d + time.Second - 1) / time.Second)
	return max(secs, 1)
}
