package tools

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/clickhouse"
	"github.com/slashobserve/slashobserve/internal/datasource/elasticsearch"
	"github.com/slashobserve/slashobserve/internal/datasource/jaeger"
	"github.com/slashobserve/slashobserve/internal/datasource/loki"
	"github.com/slashobserve/slashobserve/internal/datasource/tempo"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	draftContextBytes = 12 << 10
	draftLabelsLimit  = 200
	draftValuesLimit  = 100
	draftTablesLimit  = 100
	draftMetricsLimit = 150
	draftKeywordLimit = 40
	draftKeywords     = 4
	minKeywordRunes   = 3
	draftFieldsShown  = 60
	draftValueBytes   = 200
	moreNotShown      = " (more not shown)"

	textFacetNote         = "text (facet via .keyword)"
	clickhouseLogsTable   = "otel_logs"
	clickhouseTracesTable = "otel_traces"
)

var (
	lokiLevelLabels = []string{"detected_level", "level", "severity_text", "severity", "lvl"}

	prometheusServiceLabels = []string{"service", "service_name", "app", "job"}

	questionStopWords = map[string]bool{
		"the": true, "and": true, "for": true, "per": true, "how": true, "what": true, "are": true, "was": true,
		"with": true, "from": true, "show": true, "over": true, "last": true, "many": true, "much": true,
		"all": true, "any": true, "that": true, "this": true, "has": true, "have": true, "does": true,
		"which": true, "when": true, "where": true, "who": true, "give": true, "tell": true, "our": true,
	}
)

const lokiStructuredLevel = "detected_level"

type fieldRoleSource interface {
	Roles(ctx context.Context) (elasticsearch.FieldRoles, error)
}

type draftFacts struct {
	Text             string
	Services         []string
	ServicesComplete bool
	Reads            int
}

type factsBuilder struct {
	r         *Registry
	ctx       context.Context
	uid       string
	text      strings.Builder
	facts     draftFacts
	first     *DatasourceError
	succeeded bool
}

func (r *Registry) draftContext(ctx context.Context, inst datasource.Instance, signal string, rng datasource.TimeRange, question string) (draftFacts, error) {
	dsType := typeOf(inst)
	b := &factsBuilder{r: r, ctx: ctx, uid: inst.UID}
	b.line("datasource: " + inst.UID + " (" + string(dsType) + "), signal: " + signal)
	b.line(languageRule(dsType, signal))
	b.line("time range: " + rng.From.UTC().Format(time.RFC3339) + " to " + rng.To.UTC().Format(time.RFC3339))
	var err error
	switch dsType {
	case spec.Prometheus:
		err = b.prometheus(question)
	case spec.Loki:
		err = b.loki()
	case spec.Tempo:
		err = b.tempo()
	case spec.Jaeger:
		err = b.jaeger()
	case spec.ClickHouse:
		err = b.clickhouse(signal)
	case spec.Elasticsearch, spec.OpenSearch:
		err = b.search(inst)
	}
	b.facts.Text = b.text.String()
	switch {
	case err != nil:
		return b.facts, err
	case !b.succeeded && b.first != nil:
		return b.facts, b.first
	}
	return b.facts, nil
}

func (b *factsBuilder) loki() error {
	cat, err := b.read("labels", datasource.DiscoverRequest{Kind: datasource.DiscoverLabels, Limit: draftLabelsLimit})
	if err != nil || cat == nil {
		return err
	}
	b.list("labels", names(cat.Labels), cat.Truncated)
	if label := firstPresent(cat.Labels, loki.ServiceLabels); label != "" {
		if err := b.services("values of "+label, datasource.DiscoverRequest{Kind: datasource.DiscoverLabelValues, Label: label, Limit: draftValuesLimit}); err != nil {
			return err
		}
	}
	if label := firstPresent(cat.Labels, lokiLevelLabels); label != "" {
		return b.values("values of "+label, datasource.DiscoverRequest{Kind: datasource.DiscoverLabelValues, Label: label, Limit: draftValuesLimit})
	}
	return b.structuredLevel()
}

func (b *factsBuilder) structuredLevel() error {
	b.facts.Reads++
	cat, _, ok, err := b.r.discover(b.ctx, b.uid, datasource.DiscoverRequest{Kind: datasource.DiscoverLabelValues, Label: lokiStructuredLevel, Limit: draftValuesLimit}, "Could not read the structured level")
	if err != nil || !ok || len(cat.Values) == 0 {
		return err
	}
	b.list("values of "+lokiStructuredLevel+" (structured metadata read from a sample of the newest lines, so a level can exist that is not listed; not an index label: filter after the selector with | "+lokiStructuredLevel+"=\"value\", never inside it)", quoted(cat.Values), cat.Truncated)
	return nil
}

func (b *factsBuilder) prometheus(question string) error {
	labels, err := b.read("labels", datasource.DiscoverRequest{Kind: datasource.DiscoverLabels, Limit: draftLabelsLimit})
	if err != nil {
		return err
	}
	if labels != nil {
		b.list("labels", names(labels.Labels), labels.Truncated)
	}
	if err := b.metrics(question); err != nil {
		return err
	}
	if labels == nil {
		return nil
	}
	if label := firstPresent(labels.Labels, prometheusServiceLabels); label != "" {
		return b.values("values of "+label, datasource.DiscoverRequest{Kind: datasource.DiscoverLabelValues, Label: label, Limit: draftValuesLimit})
	}
	return nil
}

func (b *factsBuilder) metrics(question string) error {
	all, err := b.read("metrics", datasource.DiscoverRequest{Kind: datasource.DiscoverMetrics, Limit: draftMetricsLimit})
	if err != nil || all == nil {
		return err
	}
	shown, more := all.Metrics, all.Truncated
	if all.Truncated {
		found, err := b.metricsMatching(question)
		if err != nil {
			return err
		}
		if len(found) > 0 {
			shown = found
		}
	}
	b.list("metrics", metricItems(shown), more)
	return nil
}

func (b *factsBuilder) metricsMatching(question string) ([]datasource.Metric, error) {
	var found []datasource.Metric
	seen := map[string]bool{}
	for _, word := range questionKeywords(question) {
		cat, err := b.read("metrics matching "+word, datasource.DiscoverRequest{Kind: datasource.DiscoverMetrics, Search: word, Limit: draftKeywordLimit})
		if err != nil {
			return nil, err
		}
		if cat == nil {
			continue
		}
		for _, m := range cat.Metrics {
			if !seen[m.Name] {
				seen[m.Name] = true
				found = append(found, m)
			}
		}
	}
	return found, nil
}

func questionKeywords(question string) []string {
	words := strings.FieldsFunc(strings.ToLower(question), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	var keywords []string
	for _, word := range words {
		if len([]rune(word)) >= minKeywordRunes && !questionStopWords[word] && !slices.Contains(keywords, word) {
			keywords = append(keywords, word)
		}
	}
	return keywords[:min(len(keywords), draftKeywords)]
}

func metricItems(metrics []datasource.Metric) []string {
	items := make([]string, len(metrics))
	for i, m := range metrics {
		items[i] = strings.TrimSpace(factName(m.Name) + " " + factName(m.Type))
	}
	return items
}

func (b *factsBuilder) tempo() error {
	cat, err := b.read("attributes", datasource.DiscoverRequest{Kind: datasource.DiscoverLabels, Limit: draftLabelsLimit})
	if err != nil {
		return err
	}
	if cat != nil {
		b.list("attributes", names(cat.Labels), cat.Truncated)
	}
	return b.services("values of "+tempo.ServiceTag, datasource.DiscoverRequest{Kind: datasource.DiscoverLabelValues, Label: tempo.ServiceTag, Limit: draftValuesLimit})
}

func (b *factsBuilder) jaeger() error {
	return b.services("values of "+jaeger.ServiceKey, datasource.DiscoverRequest{Kind: datasource.DiscoverLabelValues, Label: jaeger.ServiceKey, Limit: draftValuesLimit})
}

func (b *factsBuilder) clickhouse(signal string) error {
	cat, err := b.read("tables", datasource.DiscoverRequest{Kind: datasource.DiscoverTables, Limit: draftTablesLimit})
	if err != nil || cat == nil {
		return err
	}
	tables := make([]string, len(cat.Tables))
	for i, t := range cat.Tables {
		tables[i] = t.Name
	}
	b.list("tables", names(tables), cat.Truncated)
	table := clickhouseLogsTable
	if signal == signalTraces {
		table = clickhouseTracesTable
	}
	if !slices.Contains(tables, table) {
		return nil
	}
	columns, err := b.read("columns of "+table, datasource.DiscoverRequest{Kind: datasource.DiscoverColumns, Table: table})
	if err != nil {
		return err
	}
	if columns != nil {
		items := make([]string, len(columns.Columns))
		for i, c := range columns.Columns {
			items[i] = strings.TrimSpace(factName(c.Name) + " " + factName(c.Type))
		}
		b.list("columns of "+table, items, columns.Truncated)
	}
	return b.services("values of "+clickhouse.ServiceColumn+" in "+table, datasource.DiscoverRequest{Kind: datasource.DiscoverLabelValues, Table: table, Label: clickhouse.ServiceColumn, Limit: draftValuesLimit})
}

func (b *factsBuilder) search(inst datasource.Instance) error {
	cat, err := b.read("fields", datasource.DiscoverRequest{Kind: datasource.DiscoverLabels, Limit: draftLabelsLimit})
	if err != nil || cat == nil {
		return err
	}
	kinds, err := b.read("field kinds", datasource.DiscoverRequest{Kind: datasource.DiscoverColumns, Limit: datasource.MaxDiscoverLimit})
	if err != nil {
		return err
	}
	shown := cat.Labels[:min(len(cat.Labels), draftFieldsShown)]
	items := make([]string, len(shown))
	for i, name := range shown {
		items[i] = factName(name)
		if kinds != nil {
			items[i] = strings.TrimSpace(items[i] + " " + fieldKind(kinds.Columns, name))
		}
	}
	b.list("fields", items, cat.Truncated || len(shown) < len(cat.Labels))
	source, ok := inst.Datasource.(fieldRoleSource)
	if !ok {
		return nil
	}
	roles, err := source.Roles(b.ctx)
	if err != nil {
		if b.ctx.Err() != nil {
			return b.ctx.Err()
		}
		b.line("field roles: unavailable")
		return nil
	}
	if roles.Service != "" {
		if err := b.services("values of "+roles.Service, datasource.DiscoverRequest{Kind: datasource.DiscoverLabelValues, Label: roles.Service, Limit: draftValuesLimit}); err != nil {
			return err
		}
	}
	if roles.Level != "" {
		return b.values("values of "+roles.Level, datasource.DiscoverRequest{Kind: datasource.DiscoverLabelValues, Label: roles.Level, Limit: draftValuesLimit})
	}
	return nil
}

func fieldKind(columns []datasource.Column, name string) string {
	for _, c := range columns {
		if c.Name != name {
			continue
		}
		if c.Type == string(elasticsearch.KindText) {
			return textFacetNote
		}
		return factName(c.Type)
	}
	return ""
}

func (b *factsBuilder) read(what string, req datasource.DiscoverRequest) (*datasource.Catalog, error) {
	b.facts.Reads++
	cat, res, ok, err := b.r.discover(b.ctx, b.uid, req, "Could not read the "+what)
	if err != nil {
		return nil, err
	}
	if !ok {
		body, _ := res.Content.(errorBody)
		if b.first == nil {
			b.first = &DatasourceError{Code: body.Code, Message: body.Message}
		}
		b.line(what + ": unavailable (" + body.Code + ")")
		return nil, nil
	}
	b.succeeded = true
	return cat, nil
}

func (b *factsBuilder) values(what string, req datasource.DiscoverRequest) error {
	cat, err := b.read(what, req)
	if err != nil || cat == nil {
		return err
	}
	b.list(what, quoted(cat.Values), cat.Truncated)
	return nil
}

func (b *factsBuilder) services(what string, req datasource.DiscoverRequest) error {
	cat, err := b.read(what, req)
	if err != nil || cat == nil {
		return err
	}
	cut := b.list(what, quoted(cat.Values), cat.Truncated)
	b.facts.Services = slices.Clone(cat.Values)
	b.facts.ServicesComplete = !cat.Truncated && !cut
	return nil
}

func (b *factsBuilder) list(what string, items []string, more bool) (cut bool) {
	var line strings.Builder
	line.WriteString(what + ":")
	budget := draftContextBytes - b.text.Len() - len(moreNotShown) - 1
	kept := 0
	for _, item := range items {
		sep := " "
		if kept > 0 {
			sep = ", "
		}
		if line.Len()+len(sep)+len(item) > budget {
			break
		}
		line.WriteString(sep + item)
		kept++
	}
	cut = kept < len(items)
	if cut || more {
		line.WriteString(moreNotShown)
	}
	b.line(line.String())
	return cut
}

func (b *factsBuilder) line(s string) {
	if b.text.Len()+len(s)+1 > draftContextBytes {
		return
	}
	b.text.WriteString(s + "\n")
}

func firstPresent(have, preferred []string) string {
	for _, name := range preferred {
		if slices.Contains(have, name) {
			return name
		}
	}
	return ""
}

func names(items []string) []string {
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = factName(item)
	}
	return out
}

func quoted(values []string) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = strconv.Quote(clip(v, draftValueBytes))
	}
	return out
}

func factName(s string) string {
	return clip(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s), maxNameBytes)
}
