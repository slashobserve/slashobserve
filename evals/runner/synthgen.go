package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	synthSeriesFile  = "series.json"
	synthKindCounter = "counter"
	synthKindGauge   = "gauge"

	synthLabelValue    = "synthetic"
	synthSecondSuffix  = "-2"
	variableStandIn    = "evalvar"
	durationStandIn    = "[5m]"
	scalarStandIn      = "1"
	labelLE            = "le"
	labelJob           = "job"
	labelMetricName    = "__name__"
	bucketSuffix       = "_bucket"
	exposedFilePerm    = 0o644
	exposedDirPerm     = 0o755
	requestsTotalName  = "http_requests_total"
	requestDurationKey = "http_request_duration_seconds"
	upName             = "up"
)

var (
	counterSuffixes = []string{"_total", "_count", "_sum", bucketSuffix}
	bucketBounds    = []string{"0.005", "0.01", "0.025", "0.05", "0.1", "0.25", "0.5", "1", "2.5", "5", "10", "+Inf"}

	bracketVariable = regexp.MustCompile(`\[[^\]]*\$[^\]]*\]`)
	quotedString    = regexp.MustCompile("\"(?:[^\"\\\\]|\\\\.)*\"|'(?:[^'\\\\]|\\\\.)*'|`[^`]*`")
	variableRef     = regexp.MustCompile(`\$\{[^}]*\}|\$[A-Za-z_][A-Za-z0-9_]*`)
	labelValuesCall = regexp.MustCompile(`(?s)^\s*label_values\s*\((.*)\)\s*$`)
	simpleAlternate = regexp.MustCompile(`^[A-Za-z0-9_.:/-]+$`)

	promParser = parser.NewParser(parser.Options{})
)

type synthMetric struct {
	Name   string              `json:"name"`
	Kind   string              `json:"kind"`
	Labels []map[string]string `json:"labels"`
}

type synthSummary struct {
	Metrics, Series, SkippedQueries int
}

type selector struct {
	metric   string
	matchers []*labels.Matcher
	label    string
}

func parserSafe(query string) string {
	q := bracketVariable.ReplaceAllString(query, durationStandIn)
	q = quotedString.ReplaceAllStringFunc(q, func(s string) string { return variableRef.ReplaceAllString(s, variableStandIn) })
	return variableRef.ReplaceAllString(q, scalarStandIn)
}

func exprSelectors(query string) ([]selector, error) {
	expr, err := promParser.ParseExpr(parserSafe(query))
	if err != nil {
		return nil, err
	}
	var out []selector
	parser.Inspect(expr, func(node parser.Node, _ []parser.Node) error {
		vs, ok := node.(*parser.VectorSelector)
		if !ok {
			return nil
		}
		if name := vectorName(vs); name != "" {
			out = append(out, selector{metric: name, matchers: vs.LabelMatchers})
		}
		return nil
	})
	return out, nil
}

func vectorName(vs *parser.VectorSelector) string {
	if vs.Name != "" {
		return vs.Name
	}
	for _, m := range vs.LabelMatchers {
		if m.Name == labelMetricName && m.Type == labels.MatchEqual {
			return m.Value
		}
	}
	return ""
}

func variableSelectors(query string) ([]selector, error) {
	m := labelValuesCall.FindStringSubmatch(query)
	if m == nil {
		return nil, errors.New("not a label_values variable query")
	}
	head, label, ok := splitLastArgument(m[1])
	if !ok {
		return nil, nil
	}
	sels, err := exprSelectors(head)
	if err != nil {
		return nil, err
	}
	for i := range sels {
		sels[i].label = strings.TrimSpace(label)
	}
	return sels, nil
}

func splitLastArgument(args string) (head, last string, ok bool) {
	depth, comma := 0, -1
	for i := 0; i < len(args); i++ {
		switch c := args[i]; {
		case isQuote(c):
			i = skipQuoted(args, i) - 1
		case c == '(' || c == '{' || c == '[':
			depth++
		case c == ')' || c == '}' || c == ']':
			depth--
		case c == ',' && depth == 0:
			comma = i
		}
	}
	if comma < 0 {
		return "", "", false
	}
	return args[:comma], args[comma+1:], true
}

var systemDatabases = map[string]bool{"system": true, "information_schema": true}

func chTables(sql string) []string {
	var tables []string
	for _, m := range fromTable.FindAllStringSubmatch(sql, -1) {
		qualifier, table := "", m[1]
		if dot := strings.LastIndex(m[1], "."); dot >= 0 {
			qualifier, table = m[1][:dot], m[1][dot+1:]
		}
		if !strings.HasPrefix(table, "$") && !systemDatabases[strings.ToLower(qualifier)] {
			tables = append(tables, table)
		}
	}
	return tables
}

type metricBook struct {
	values map[string]string
	le     map[string]bool
}

func (m *metricBook) use(label, value string) {
	if m.values[label] == "" {
		m.values[label] = value
	}
}

func (m *metricBook) addMatcher(matcher *labels.Matcher) {
	if matcher.Name == labelMetricName || (matcher.Type == labels.MatchEqual && matcher.Value == "") {
		return
	}
	value := literalValue(matcher)
	if matcher.Name == labelLE && value != "" {
		m.le[value] = true
	}
	m.use(matcher.Name, value)
}

func literalValue(m *labels.Matcher) string {
	if strings.Contains(m.Value, variableStandIn) {
		return ""
	}
	switch m.Type {
	case labels.MatchEqual:
		return m.Value
	case labels.MatchRegexp:
		first, _, _ := strings.Cut(m.Value, "|")
		if simpleAlternate.MatchString(first) {
			return first
		}
	case labels.MatchNotEqual, labels.MatchNotRegexp:
	}
	return ""
}

type seriesBook struct {
	metrics map[string]*metricBook
}

func excludedMetric(name string) bool {
	return name == upName || name == requestsTotalName || strings.HasPrefix(name, requestDurationKey)
}

func (b *seriesBook) add(s selector) {
	if excludedMetric(s.metric) {
		return
	}
	m, ok := b.metrics[s.metric]
	if !ok {
		m = &metricBook{values: map[string]string{}, le: map[string]bool{}}
		b.metrics[s.metric] = m
	}
	for _, matcher := range s.matchers {
		m.addMatcher(matcher)
	}
	if s.label != "" && s.label != labelMetricName {
		m.use(s.label, "")
	}
}

func (b *seriesBook) list() []synthMetric {
	names := slices.Sorted(maps.Keys(b.metrics))
	out := make([]synthMetric, 0, len(names))
	for _, name := range names {
		out = append(out, b.metrics[name].render(name))
	}
	return out
}

func (m *metricBook) render(name string) synthMetric {
	metric := synthMetric{Name: name, Kind: synthKindGauge}
	if slices.ContainsFunc(counterSuffixes, func(suffix string) bool { return strings.HasSuffix(name, suffix) }) {
		metric.Kind = synthKindCounter
	}
	bucket := strings.HasSuffix(name, bucketSuffix)
	if bucket {
		delete(m.values, labelLE)
	}
	sets := m.labelSets()
	if !bucket {
		metric.Labels = sets
		return metric
	}
	bounds := slices.Clone(bucketBounds)
	for le := range m.le {
		if !slices.Contains(bounds, le) {
			bounds = append(bounds, le)
		}
	}
	slices.Sort(bounds)
	for _, set := range sets {
		for _, le := range bounds {
			withLE := maps.Clone(set)
			withLE[labelLE] = le
			metric.Labels = append(metric.Labels, withLE)
		}
	}
	return metric
}

func (m *metricBook) labelSets() []map[string]string {
	first := map[string]string{}
	for name, value := range m.values {
		if value == "" {
			value = synthLabelValue
		}
		first[name] = value
	}
	names := slices.Sorted(maps.Keys(first))
	varied := slices.IndexFunc(names, func(n string) bool { return n != labelJob })
	if varied < 0 {
		return []map[string]string{first}
	}
	second := maps.Clone(first)
	second[names[varied]] += synthSecondSuffix
	return []map[string]string{first, second}
}

func generateSynth(out string) error {
	summary, err := writeSynth(out)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(prepareOutput, "synthetic series: %d metrics, %d series, %d queries skipped\n", summary.Metrics, summary.Series, summary.SkippedQueries)
	return err
}

func writeSynth(out string) (synthSummary, error) {
	dashboards, err := loadCorpus(corpusDir, nil)
	if err != nil {
		return synthSummary{}, err
	}
	book := &seriesBook{metrics: map[string]*metricBook{}}
	summary := synthSummary{}
	for _, d := range dashboards {
		dash, err := convertSource(d.raw, stackOptions())
		if err != nil {
			return synthSummary{}, fmt.Errorf("%s: %w", d.key, err)
		}
		for _, ref := range queryRefs(dash) {
			if ref.kind != spec.Prometheus {
				continue
			}
			sels, err := promSelectors(ref)
			if err != nil {
				summary.SkippedQueries++
				continue
			}
			for _, s := range sels {
				book.add(s)
			}
		}
	}
	metrics := book.list()
	summary.Metrics = len(metrics)
	for _, m := range metrics {
		summary.Series += len(m.Labels)
	}
	body, err := json.Marshal(metrics)
	if err != nil {
		return synthSummary{}, err
	}
	if err := writeShared(filepath.Join(out, synthSeriesFile), body); err != nil {
		return synthSummary{}, err
	}
	return summary, nil
}

func promSelectors(ref queryRef) ([]selector, error) {
	if ref.variable {
		return variableSelectors(ref.text)
	}
	return exprSelectors(ref.text)
}

func writeShared(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), exposedDirPerm); err != nil {
		return err
	}
	return os.WriteFile(path, content, exposedFilePerm) //nolint:gosec // bind-mounted read-only into containers that run as another user
}
