package clickhouse

import (
	"context"
	"slices"
	"strconv"
	"strings"

	ch "github.com/ClickHouse/clickhouse-go/v2"
	"golang.org/x/sync/errgroup"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/macros"
)

const (
	logAttributesColumn = "LogAttributes"
	attributeFieldSep   = "."
	maxAttributeKeys    = 20
	facetConcurrency    = 4
)

var facetSkippedColumns = []string{"Timestamp", "Body", "TraceId", "SpanId"}

var _ datasource.LogFacetSource = (*Datasource)(nil)

func (d *Datasource) LogFacets(ctx context.Context, req datasource.FacetRequest) (datasource.Facets, error) {
	if err := req.Range.Validate(); err != nil {
		return datasource.Facets{}, err
	}
	if err := d.readOnly(ctx); err != nil {
		return datasource.Facets{}, err
	}
	exp, err := macros.Expand(macros.ClickHouse, req.Query.Text, macros.Env{Range: req.Range, Interval: instantInterval(req.Range), Variables: req.Query.Variables, Dashboard: req.Query.Dashboard, User: req.Query.User})
	if err != nil {
		return datasource.Facets{}, err
	}
	text, err := d.guard(exp.Text)
	if err != nil {
		return datasource.Facets{}, err
	}
	inner, err := volumeInner(text)
	if err != nil {
		return datasource.Facets{}, err
	}
	lim := d.limits.Tighten(req.Query.Limits)
	lim.Timeout = d.clampTimeout(lim.Timeout)
	timeout := effectiveTimeout(ctx, lim.Timeout)
	ctx, cancel := context.WithTimeout(ctx, lim.Timeout)
	defer cancel()
	columns, err := d.resultColumns(ctx, inner, lim)
	if err != nil {
		return datasource.Facets{}, mapError(ctx, err, timeout)
	}
	fields, err := d.facetFields(ctx, inner, lim, columns, req.Fields)
	if err != nil {
		return datasource.Facets{}, mapError(ctx, err, timeout)
	}
	out := datasource.Facets{Fields: make([]datasource.FacetField, len(fields))}
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(facetConcurrency)
	for i, name := range fields {
		group.Go(func() error {
			field, err := d.columnFacet(groupCtx, inner, lim, name, req.Limit)
			out.Fields[i] = field
			return err
		})
	}
	if err := group.Wait(); err != nil {
		return datasource.Facets{}, mapError(ctx, err, timeout)
	}
	return out, nil
}

func (d *Datasource) resultColumns(ctx context.Context, inner string, lim datasource.Limits) ([]string, error) {
	rs, err := d.fetchWith(ctx, "SELECT * FROM (\n"+inner+"\n) LIMIT 0", lim, nil, nil)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(rs.columns))
	for i, c := range rs.columns {
		names[i] = c.name
	}
	return names, nil
}

func (d *Datasource) facetFields(ctx context.Context, inner string, lim datasource.Limits, columns, requested []string) ([]string, error) {
	hasAttributes := slices.Contains(columns, logAttributesColumn)
	if requested != nil {
		for _, name := range requested {
			if !facetFieldOf(name, columns, hasAttributes) {
				return nil, datasource.Errorf(datasource.ErrInvalidQuery, "field %q is not a column of the query", name)
			}
		}
		return slices.Compact(slices.Sorted(slices.Values(requested))), nil
	}
	var fields []string
	for _, name := range columns {
		if name != logAttributesColumn && !slices.Contains(facetSkippedColumns, name) {
			fields = append(fields, name)
		}
	}
	if !hasAttributes {
		return fields, nil
	}
	keys, err := d.attributeKeys(ctx, inner, lim)
	if err != nil {
		return nil, err
	}
	for _, key := range keys {
		fields = append(fields, logAttributesColumn+attributeFieldSep+key)
	}
	return fields, nil
}

func (d *Datasource) attributeKeys(ctx context.Context, inner string, lim datasource.Limits) ([]string, error) {
	text := "SELECT arrayJoin(mapKeys(" + quoteIdent(logAttributesColumn) + ")) AS K, count() FROM (\n" + inner + "\n) GROUP BY K ORDER BY count() DESC LIMIT " + strconv.Itoa(maxAttributeKeys)
	rs, err := d.fetchWith(ctx, text, lim, nil, nil)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(rs.rows))
	for _, row := range rs.rows {
		if row[0] != nil {
			keys = append(keys, stringOf(row[0]))
		}
	}
	return keys, nil
}

func (d *Datasource) columnFacet(ctx context.Context, inner string, lim datasource.Limits, name string, limit int) (datasource.FacetField, error) {
	expr := quoteIdent(name)
	var params ch.Parameters
	if key, ok := strings.CutPrefix(name, logAttributesColumn+attributeFieldSep); ok {
		expr = quoteIdent(logAttributesColumn) + "[{key:String}]"
		params = ch.Parameters{"key": key}
	}
	text := "SELECT toString(" + expr + ") AS Value, count() AS Count FROM (\n" + inner + "\n) GROUP BY Value ORDER BY Count DESC, Value LIMIT " + strconv.Itoa(limit+1)
	rs, err := d.fetchWith(ctx, text, lim, params, nil)
	if err != nil {
		return datasource.FacetField{}, err
	}
	field := datasource.FacetField{Name: name, Kind: datasource.FacetKindColumn, Exact: true, Values: make([]datasource.FacetValue, 0, len(rs.rows))}
	for _, row := range rs.rows {
		if len(row) < 2 {
			continue
		}
		var value string
		if row[0] != nil {
			value = stringOf(row[0])
		}
		count, _ := toFloat(row[1])
		field.Values = append(field.Values, datasource.FacetValue{Value: value, Count: int64(count)})
	}
	field.Cardinality = len(field.Values)
	if len(field.Values) > limit {
		field.Values, field.Truncated = field.Values[:limit], true
	}
	return field, nil
}

func facetFieldOf(name string, columns []string, hasAttributes bool) bool {
	if slices.Contains(columns, name) {
		return true
	}
	key, isAttribute := strings.CutPrefix(name, logAttributesColumn+attributeFieldSep)
	return isAttribute && hasAttributes && key != ""
}
