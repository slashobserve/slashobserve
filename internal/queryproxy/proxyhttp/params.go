package proxyhttp

import (
	"cmp"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	paramFrom          = "from"
	paramTo            = "to"
	paramMaxDataPoints = "max_data_points"
	variablePrefix     = "var-"
	allSelection       = "$__all"

	defaultMaxDataPoints = 1000
	minMaxDataPoints     = 10
	maxMaxDataPoints     = 11000
	maxTimeParamBytes    = 128
	maxRange             = 400 * 24 * time.Hour
	instantResolution    = 5 * time.Second
)

var (
	forbiddenParams = []string{"query", "expr", "sql", "q"}

	errQueryNotInSpec = errors.New("query text is not accepted")
)

type params struct {
	from, to      string
	maxDataPoints int
	selections    map[string]queryproxy.Selection
}

func hasForbiddenParam(q url.Values) bool {
	for _, name := range forbiddenParams {
		if _, ok := q[name]; ok {
			return true
		}
	}
	return false
}

func hasForbiddenRawKey(raw string) bool {
	for pair := range strings.SplitSeq(raw, "&") {
		key, _, _ := strings.Cut(pair, "=")
		if decoded, err := url.QueryUnescape(key); err == nil {
			key = decoded
		}
		if slices.Contains(forbiddenParams, key) {
			return true
		}
	}
	return false
}

func parseQuery(raw string) (url.Values, error) {
	q, err := url.ParseQuery(raw)
	if hasForbiddenParam(q) || hasForbiddenRawKey(raw) {
		return nil, errQueryNotInSpec
	}
	if err != nil {
		return nil, invalidf("the query string is malformed")
	}
	return q, nil
}

func invalidf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", queryproxy.ErrInvalidRequest, fmt.Sprintf(format, args...))
}

func parseParams(q url.Values, allowMaxDataPoints bool) (params, error) {
	p := params{maxDataPoints: defaultMaxDataPoints, selections: map[string]queryproxy.Selection{}}
	for name, values := range q {
		switch {
		case name == paramFrom || name == paramTo:
			text, err := singleTimeParam(name, values)
			if err != nil {
				return params{}, err
			}
			if name == paramFrom {
				p.from = text
			} else {
				p.to = text
			}
		case name == paramMaxDataPoints && allowMaxDataPoints:
			n, err := parseMaxDataPoints(values)
			if err != nil {
				return params{}, err
			}
			p.maxDataPoints = n
		case strings.HasPrefix(name, variablePrefix):
			variable := strings.TrimPrefix(name, variablePrefix)
			if variable == "" {
				return params{}, invalidf("a variable parameter needs a name after %q", variablePrefix)
			}
			p.selections[variable] = selectionOf(values)
		default:
			return params{}, invalidf("unknown query parameter")
		}
	}
	return p, nil
}

func singleTimeParam(name string, values []string) (string, error) {
	if len(values) != 1 {
		return "", invalidf("%s must be given once", name)
	}
	if values[0] == "" || len(values[0]) > maxTimeParamBytes {
		return "", invalidf("%s must be a time expression of 1 to %d bytes", name, maxTimeParamBytes)
	}
	return values[0], nil
}

func parseMaxDataPoints(values []string) (int, error) {
	if len(values) != 1 {
		return 0, invalidf("%s must be given once", paramMaxDataPoints)
	}
	n, err := strconv.Atoi(values[0])
	if err != nil || strconv.Itoa(n) != values[0] || n < minMaxDataPoints || n > maxMaxDataPoints {
		return 0, invalidf("%s must be an integer from %d to %d", paramMaxDataPoints, minMaxDataPoints, maxMaxDataPoints)
	}
	return n, nil
}

func selectionOf(values []string) queryproxy.Selection {
	var sel queryproxy.Selection
	for _, value := range values {
		if value == allSelection {
			sel.All = true
			continue
		}
		sel.Values = append(sel.Values, value)
	}
	return sel
}

func (p params) timeRange(d *spec.Dashboard, now time.Time) (datasource.TimeRange, error) {
	from, err := spec.ResolveTime(cmp.Or(p.from, d.Time.From), now, false)
	if err != nil {
		return datasource.TimeRange{}, invalidf("%s is not a valid time", paramFrom)
	}
	to, err := spec.ResolveTime(cmp.Or(p.to, d.Time.To), now, true)
	if err != nil {
		return datasource.TimeRange{}, invalidf("%s is not a valid time", paramTo)
	}
	if !from.Before(to) {
		return datasource.TimeRange{}, invalidf("%s must be before %s", paramFrom, paramTo)
	}
	if to.Sub(from) > maxRange {
		return datasource.TimeRange{}, invalidf("the time range must be at most 400 days")
	}
	return datasource.TimeRange{From: from, To: to}, nil
}

type alignment struct {
	rng           datasource.TimeRange
	step          time.Duration
	maxDataPoints int
}

func align(rng datasource.TimeRange, kind queryproxy.Kind, maxDataPoints int) alignment {
	if kind == queryproxy.KindInstant {
		to := floor(rng.To, instantResolution)
		return alignment{rng: datasource.TimeRange{From: to.Add(-rng.To.Sub(rng.From)), To: to}}
	}
	step := queryproxy.StepFor(rng, maxDataPoints)
	from, to := floor(rng.From, step), floor(rng.To, step)
	if !to.After(from) {
		to = from.Add(step)
	}
	points := int(to.Sub(from) / step)
	if points < minMaxDataPoints && step > time.Second {
		from, points = to.Add(-minMaxDataPoints*step), minMaxDataPoints
	}
	return alignment{rng: datasource.TimeRange{From: from, To: to}, step: step, maxDataPoints: points}
}

func floor(t time.Time, unit time.Duration) time.Time {
	ms, u := t.UnixMilli(), unit.Milliseconds()
	return time.UnixMilli(ms - ((ms%u)+u)%u).UTC()
}
