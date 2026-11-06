package tools

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	draftContextBytes = 12 << 10
	draftLabelsLimit  = 200
	draftValuesLimit  = 100
	draftTablesLimit  = 100
	draftValueBytes   = 200
	moreNotShown      = " (more not shown)"

	tempoServiceAttribute   = "resource.service.name"
	clickhouseServiceColumn = "ServiceName"
	clickhouseLogsTable     = "otel_logs"
	clickhouseTracesTable   = "otel_traces"
)

var (
	lokiServiceLabels = []string{"service_name", "service", "app", "job"}
	lokiLevelLabels   = []string{"detected_level", "level", "severity_text", "severity", "lvl"}
)

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

func (r *Registry) draftContext(ctx context.Context, inst datasource.Instance, signal string, rng datasource.TimeRange) (draftFacts, error) {
	dsType := typeOf(inst)
	b := &factsBuilder{r: r, ctx: ctx, uid: inst.UID}
	b.line("datasource: " + inst.UID + " (" + string(dsType) + "), signal: " + signal)
	b.line(languageRule(dsType, signal))
	b.line("time range: " + rng.From.UTC().Format(time.RFC3339) + " to " + rng.To.UTC().Format(time.RFC3339))
	var err error
	switch dsType {
	case spec.Loki:
		err = b.loki()
	case spec.Tempo:
		err = b.tempo()
	case spec.ClickHouse:
		err = b.clickhouse(signal)
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
	if label := firstPresent(cat.Labels, lokiServiceLabels); label != "" {
		if err := b.services("values of "+label, datasource.DiscoverRequest{Kind: datasource.DiscoverLabelValues, Label: label, Limit: draftValuesLimit}); err != nil {
			return err
		}
	}
	if label := firstPresent(cat.Labels, lokiLevelLabels); label != "" {
		return b.values("values of "+label, datasource.DiscoverRequest{Kind: datasource.DiscoverLabelValues, Label: label, Limit: draftValuesLimit})
	}
	return nil
}

func (b *factsBuilder) tempo() error {
	cat, err := b.read("attributes", datasource.DiscoverRequest{Kind: datasource.DiscoverLabels, Limit: draftLabelsLimit})
	if err != nil {
		return err
	}
	if cat != nil {
		b.list("attributes", names(cat.Labels), cat.Truncated)
	}
	return b.services("values of "+tempoServiceAttribute, datasource.DiscoverRequest{Kind: datasource.DiscoverLabelValues, Label: tempoServiceAttribute, Limit: draftValuesLimit})
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
	return b.services("values of "+clickhouseServiceColumn+" in "+table, datasource.DiscoverRequest{Kind: datasource.DiscoverLabelValues, Table: table, Label: clickhouseServiceColumn, Limit: draftValuesLimit})
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
