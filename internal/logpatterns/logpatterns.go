package logpatterns

import (
	"cmp"
	"net/netip"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/logvolume"
)

const (
	Wildcard    = "<*>"
	MaxLines    = 5000
	MaxClusters = 500

	TrendBuckets = 30

	joinSimilarity    = 0.6
	maxPlainQuoteSize = 32
)

type Line struct {
	Time  time.Time
	Text  string
	Level string
}

type TrendBucket struct {
	Start time.Time
	Count int64
}

type Pattern struct {
	Pattern string
	Count   int64
	Sample  string
	First   time.Time
	Last    time.Time
	Trend   []TrendBucket
	Level   string
}

type Trend struct {
	from, to time.Time
	step     time.Duration
	counts   [TrendBuckets]int64
}

func NewTrend(from, to time.Time) *Trend {
	return &Trend{from: from, to: to, step: to.Sub(from) / TrendBuckets}
}

func (t *Trend) Add(at time.Time, n int64) {
	if t.step <= 0 || at.IsZero() || at.Before(t.from) || !at.Before(t.to) {
		return
	}
	t.counts[min(int(at.Sub(t.from)/t.step), TrendBuckets-1)] += n
}

func (t *Trend) Buckets() []TrendBucket {
	out := make([]TrendBucket, TrendBuckets)
	for i, n := range t.counts {
		out[i] = TrendBucket{Start: t.from.Add(time.Duration(i) * t.step).UTC(), Count: n}
	}
	return out
}

func PatternLevel(pattern string) string {
	for token := range strings.FieldsSeq(pattern) {
		if _, value, ok := strings.Cut(token, "="); ok {
			token = value
		}
		token = strings.Trim(token, `[]()",:;'`)
		if kind := logvolume.KindOf(token); kind != logvolume.Other {
			return string(kind)
		}
	}
	return ""
}

var (
	numberToken = regexp.MustCompile(`^[+-]?(\d+\.?\d*|\.\d+)([eE][+-]?\d+)?$`)
	hexToken    = regexp.MustCompile(`^(0[xX])?[0-9a-fA-F]{8,}$`)
	uuidToken   = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	dateToken   = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	clockToken  = regexp.MustCompile(`^\d{2}:\d{2}:\d{2}(\.\d+)?$`)
)

func Tokenize(line string) []string {
	tokens := strings.FieldsFunc(line, func(r rune) bool { return r == ' ' || r == '\t' })
	for i, token := range tokens {
		tokens[i] = mask(token)
	}
	return tokens
}

func mask(token string) string {
	if variable(token) {
		return Wildcard
	}
	if key, value, ok := strings.Cut(token, "="); ok && key != "" && value != "" && variable(value) {
		return key + "=" + Wildcard
	}
	return token
}

func variable(token string) bool {
	switch {
	case numberToken.MatchString(token), hexToken.MatchString(token), uuidToken.MatchString(token):
		return true
	case dateToken.MatchString(token), clockToken.MatchString(token):
		return true
	case isAddress(token), isTimestamp(token):
		return true
	}
	return len(token) > maxPlainQuoteSize && len(token) >= 2 && token[0] == '"' && token[len(token)-1] == '"'
}

func isAddress(token string) bool {
	_, err := netip.ParseAddr(token)
	return err == nil
}

func isTimestamp(token string) bool {
	_, err := time.Parse(time.RFC3339Nano, token)
	return err == nil
}

type cluster struct {
	template []string
	count    int64
	sample   string
	first    time.Time
	last     time.Time
	trend    *Trend
	levels   map[logvolume.Kind]int64
}

type groupKey struct {
	size  int
	first string
}

func Cluster(lines []Line, limit int, from, to time.Time) ([]Pattern, bool) {
	if len(lines) > MaxLines {
		lines = lines[:MaxLines]
	}
	var clusters []*cluster
	groups := map[groupKey][]*cluster{}
	for _, line := range lines {
		tokens := Tokenize(line.Text)
		if len(tokens) == 0 {
			continue
		}
		key := groupKey{size: len(tokens), first: firstPlainToken(tokens)}
		c := firstSimilar(groups[key], tokens)
		if c == nil && len(clusters) >= MaxClusters {
			c = nearest(clusters, tokens)
		}
		if c == nil {
			c = &cluster{template: slices.Clone(tokens), trend: NewTrend(from, to), levels: map[logvolume.Kind]int64{}}
			clusters = append(clusters, c)
			groups[key] = append(groups[key], c)
		} else {
			c.absorb(tokens)
		}
		c.add(line)
	}
	slices.SortStableFunc(clusters, func(a, b *cluster) int {
		return cmp.Or(cmp.Compare(b.count, a.count), strings.Compare(a.pattern(), b.pattern()))
	})
	truncated := len(clusters) > limit
	if truncated {
		clusters = clusters[:limit]
	}
	out := make([]Pattern, len(clusters))
	for i, c := range clusters {
		out[i] = Pattern{Pattern: c.pattern(), Count: c.count, Sample: c.sample, First: c.first, Last: c.last, Trend: c.trend.Buckets(), Level: c.level()}
	}
	return out, truncated
}

func firstPlainToken(tokens []string) string {
	for _, t := range tokens {
		if t != Wildcard {
			return t
		}
	}
	return ""
}

func firstSimilar(candidates []*cluster, tokens []string) *cluster {
	for _, c := range candidates {
		if similarity(c.template, tokens) >= joinSimilarity {
			return c
		}
	}
	return nil
}

func nearest(candidates []*cluster, tokens []string) *cluster {
	var best *cluster
	bestScore := -1.0
	for _, c := range candidates {
		if score := similarity(c.template, tokens); score > bestScore {
			best, bestScore = c, score
		}
	}
	return best
}

func similarity(template, tokens []string) float64 {
	longer := max(len(template), len(tokens))
	if longer == 0 {
		return 0
	}
	equal := 0
	for i := range min(len(template), len(tokens)) {
		if template[i] == tokens[i] {
			equal++
		}
	}
	return float64(equal) / float64(longer)
}

func (c *cluster) absorb(tokens []string) {
	for i := range min(len(c.template), len(tokens)) {
		if c.template[i] != tokens[i] {
			c.template[i] = Wildcard
		}
	}
}

func (c *cluster) add(line Line) {
	c.count++
	c.trend.Add(line.Time, 1)
	if strings.TrimSpace(line.Level) != "" {
		c.levels[logvolume.KindOf(line.Level)]++
	}
	if c.count == 1 || line.Time.After(c.last) {
		c.sample, c.last = line.Text, line.Time
	}
	if c.count == 1 || line.Time.Before(c.first) {
		c.first = line.Time
	}
}

func (c *cluster) pattern() string {
	return strings.Join(c.template, " ")
}

func (c *cluster) level() string {
	best, votes := logvolume.Kind(""), int64(0)
	for _, kind := range logvolume.Kinds {
		if n := c.levels[kind]; n > votes {
			best, votes = kind, n
		}
	}
	return string(best)
}
