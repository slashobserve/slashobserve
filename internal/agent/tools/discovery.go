package tools

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

const (
	discoveryTimeout  = 10 * time.Second
	maxDiscoverItems  = 500
	defaultSampleRows = 5
	maxSampleRows     = 20
	maxNameBytes      = 256
	maxCellBytes      = 300
	maxSampleColumns  = 50
	maxStepNameBytes  = 64
	columnsBudget     = 3 << 10
	sampleRowsBudget  = 10 << 10
)

func (r *Registry) discoveryEntries() []entry {
	return []entry{
		{Tool{Name: nameListDatasources, Description: descListDatasources, InputSchema: json.RawMessage(schemaEmpty), ReadOnly: true, MinRole: auth.RoleEditor}, r.listDatasources},
		{Tool{Name: nameListMetrics, Description: descListMetrics, InputSchema: json.RawMessage(schemaListMetrics), ReadOnly: true, MinRole: auth.RoleEditor}, r.listMetrics},
		{Tool{Name: nameListLabels, Description: descListLabels, InputSchema: json.RawMessage(schemaListLabels), ReadOnly: true, MinRole: auth.RoleEditor}, r.listLabels},
		{Tool{Name: nameListLabelValues, Description: descListLabelValues, InputSchema: json.RawMessage(schemaListLabelValues), ReadOnly: true, MinRole: auth.RoleEditor}, r.listLabelValues},
		{Tool{Name: nameGetSchema, Description: descGetSchema, InputSchema: json.RawMessage(schemaGetSchema), ReadOnly: true, MinRole: auth.RoleEditor}, r.getSchema},
		{Tool{Name: nameSampleRows, Description: descSampleRows, InputSchema: json.RawMessage(schemaSampleRows), ReadOnly: true, MinRole: auth.RoleEditor}, r.sampleRows},
	}
}

type datasourceOut struct {
	UID     string `json:"uid"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Healthy bool   `json:"healthy"`
	Message string `json:"message,omitempty"`
}

type datasourcesOut struct {
	Datasources []datasourceOut `json:"datasources"`
	Truncated   bool            `json:"truncated,omitempty"`
}

func (r *Registry) listDatasources(ctx context.Context, input json.RawMessage) (Result, error) {
	var in struct{}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	statuses := r.deps.Datasources.Health(ctx)
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	items := make([]datasourceOut, len(statuses))
	for i, s := range statuses {
		items[i] = datasourceOut{UID: s.UID, Type: s.Type, Name: s.Name, Healthy: s.Healthy, Message: clip(s.Message, maxCellBytes)}
	}
	items, truncated := fit(items, listBudget)
	return success("Listed "+plural(len(items), "datasource", "datasources"), datasourcesOut{Datasources: items, Truncated: truncated}), nil
}

func (r *Registry) instance(uid string) (datasource.Instance, Result, bool) {
	if uid == "" {
		return datasource.Instance{}, invalid("datasource is required; call list_datasources"), false
	}
	inst, ok := r.deps.Datasources.Get(uid)
	if !ok {
		code := string(queryproxy.CodeUnknownDatasource)
		return datasource.Instance{}, toolError(code, fmt.Sprintf("unknown datasource %q; call list_datasources", clip(uid, maxStepNameBytes))), false
	}
	return inst, Result{}, true
}

func (r *Registry) discover(ctx context.Context, uid string, req datasource.DiscoverRequest, failStep string) (*datasource.Catalog, Result, bool, error) {
	inst, res, ok := r.instance(uid)
	if !ok {
		return nil, res, false, nil
	}
	ctx, cancel := context.WithTimeout(ctx, discoveryTimeout)
	defer cancel()
	cat, err := inst.Datasource.Discover(ctx, req)
	if err != nil {
		res, err := datasourceFailure(failStep+" on "+inst.UID, err)
		return nil, res, false, err
	}
	if cat == nil {
		cat = &datasource.Catalog{}
	}
	return cat, Result{}, true, nil
}

func datasourceFailure(step string, err error) (Result, error) {
	_, code, message := queryproxy.Classify(err)
	switch code {
	case string(queryproxy.CodeCanceled), string(queryproxy.CodeInternal):
		return Result{}, err
	}
	return failure(step+": "+code, errorBody{Code: code, Message: message}), nil
}

func boundedInt(p *int, def, lo, hi int, field string) (int, Result, bool) {
	if p == nil {
		return def, Result{}, true
	}
	if *p < lo || *p > hi {
		return 0, invalid("%s must be between %d and %d", field, lo, hi), false
	}
	return *p, Result{}, true
}

func checkLength(field, value string, limit int) (Result, bool) {
	if len(value) > limit {
		return invalid("%s must be at most %d bytes", field, limit), false
	}
	return Result{}, true
}

type listMetricsInput struct {
	Datasource string `json:"datasource"`
	Search     string `json:"search"`
	Limit      *int   `json:"limit"`
}

type listLabelsInput struct {
	Datasource string `json:"datasource"`
	Metric     string `json:"metric"`
	Table      string `json:"table"`
	Search     string `json:"search"`
	Limit      *int   `json:"limit"`
}

type listLabelValuesInput struct {
	Datasource string `json:"datasource"`
	Label      string `json:"label"`
	Metric     string `json:"metric"`
	Table      string `json:"table"`
	Search     string `json:"search"`
	Limit      *int   `json:"limit"`
}

func namesLimit(p *int) int {
	if p == nil {
		return 0
	}
	return cmp.Or(*p, -1)
}

func (r *Registry) listNames(ctx context.Context, req queryproxy.NamesRequest, noun string) (queryproxy.Names, Result, bool, error) {
	if req.Datasource == "" {
		return queryproxy.Names{}, invalid("datasource is required; call list_datasources"), false, nil
	}
	names, err := r.deps.Runner.Names(ctx, req)
	switch {
	case err == nil:
		return names, Result{}, true, nil
	case errors.Is(err, queryproxy.ErrInvalidRequest):
		_, _, message := queryproxy.Classify(err)
		return queryproxy.Names{}, invalid("%s", message), false, nil
	case errors.Is(err, queryproxy.ErrUnknownDatasource):
		code := string(queryproxy.CodeUnknownDatasource)
		return queryproxy.Names{}, toolError(code, fmt.Sprintf("unknown datasource %q; call list_datasources", clip(req.Datasource, maxStepNameBytes))), false, nil
	}
	res, err := datasourceFailure("Listing "+noun+" on "+req.Datasource, err)
	return queryproxy.Names{}, res, false, err
}

type metricsOut struct {
	Datasource string              `json:"datasource"`
	Metrics    []datasource.Metric `json:"metrics"`
	Truncated  bool                `json:"truncated"`
}

func (r *Registry) listMetrics(ctx context.Context, input json.RawMessage) (Result, error) {
	var in listMetricsInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	names, res, ok, err := r.listNames(ctx, queryproxy.NamesRequest{
		Datasource: in.Datasource,
		Kind:       datasource.DiscoverMetrics,
		Search:     in.Search,
		Limit:      namesLimit(in.Limit),
	}, "metrics")
	if !ok || err != nil {
		return res, err
	}
	metrics, cut := fit(names.Metrics, listBudget)
	return success(fmt.Sprintf("Listed %s on %s", plural(len(metrics), "metric", "metrics"), names.Datasource), metricsOut{Datasource: names.Datasource, Metrics: metrics, Truncated: names.Truncated || cut}), nil
}

type labelsOut struct {
	Datasource string   `json:"datasource"`
	Labels     []string `json:"labels"`
	Truncated  bool     `json:"truncated"`
}

func (r *Registry) listLabels(ctx context.Context, input json.RawMessage) (Result, error) {
	var in listLabelsInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	names, res, ok, err := r.listNames(ctx, queryproxy.NamesRequest{
		Datasource: in.Datasource,
		Kind:       datasource.DiscoverLabels,
		Search:     in.Search,
		Metric:     in.Metric,
		Table:      in.Table,
		Limit:      namesLimit(in.Limit),
	}, "labels")
	if !ok || err != nil {
		return res, err
	}
	labels, cut := fit(names.Labels, listBudget)
	return success(fmt.Sprintf("Listed %s on %s", plural(len(labels), "label", "labels"), names.Datasource), labelsOut{Datasource: names.Datasource, Labels: labels, Truncated: names.Truncated || cut}), nil
}

type labelValuesOut struct {
	Datasource string   `json:"datasource"`
	Label      string   `json:"label"`
	Values     []string `json:"values"`
	Truncated  bool     `json:"truncated"`
}

func (r *Registry) listLabelValues(ctx context.Context, input json.RawMessage) (Result, error) {
	var in listLabelValuesInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	names, res, ok, err := r.listNames(ctx, queryproxy.NamesRequest{
		Datasource: in.Datasource,
		Kind:       datasource.DiscoverLabelValues,
		Search:     in.Search,
		Metric:     in.Metric,
		Label:      in.Label,
		Table:      in.Table,
		Limit:      namesLimit(in.Limit),
	}, "label values")
	if !ok || err != nil {
		return res, err
	}
	values, cut := fit(names.Values, listBudget)
	return success(fmt.Sprintf("Listed %s on %s", plural(len(values), "label value", "label values"), names.Datasource), labelValuesOut{Datasource: names.Datasource, Label: names.Label, Values: values, Truncated: names.Truncated || cut}), nil
}

func cutList[T any](items []T, limit int) ([]T, bool) {
	truncated := len(items) > limit
	if truncated {
		items = items[:limit]
	}
	items, cut := fit(items, listBudget)
	return items, truncated || cut
}

type schemaInput struct {
	Datasource string `json:"datasource"`
	Table      string `json:"table"`
}

type tablesOut struct {
	Tables    []datasource.Table `json:"tables"`
	Truncated bool               `json:"truncated,omitempty"`
}

type columnsOut struct {
	Table     string              `json:"table"`
	Columns   []datasource.Column `json:"columns"`
	Truncated bool                `json:"truncated,omitempty"`
}

func (r *Registry) getSchema(ctx context.Context, input json.RawMessage) (Result, error) {
	var in schemaInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	if res, ok := checkLength("table", in.Table, maxNameBytes); !ok {
		return res, nil
	}
	if in.Table == "" {
		cat, res, ok, err := r.discover(ctx, in.Datasource, datasource.DiscoverRequest{Kind: datasource.DiscoverTables, Limit: maxDiscoverItems + 1}, "Listing tables")
		if !ok || err != nil {
			return res, err
		}
		tables, truncated := cutList(append([]datasource.Table{}, cat.Tables...), maxDiscoverItems)
		return success(fmt.Sprintf("Listed %s on %s", plural(len(tables), "table", "tables"), in.Datasource), tablesOut{Tables: tables, Truncated: truncated}), nil
	}
	cat, res, ok, err := r.discover(ctx, in.Datasource, datasource.DiscoverRequest{Kind: datasource.DiscoverColumns, Table: in.Table, Limit: maxDiscoverItems + 1}, "Reading the "+clip(in.Table, maxStepNameBytes)+" schema")
	if !ok || err != nil {
		return res, err
	}
	columns, truncated := cutList(append([]datasource.Column{}, cat.Columns...), maxDiscoverItems)
	return success(fmt.Sprintf("Read the %s schema on %s", clip(in.Table, maxStepNameBytes), in.Datasource), columnsOut{Table: in.Table, Columns: columns, Truncated: truncated}), nil
}

type sampleInput struct {
	Datasource string `json:"datasource"`
	Table      string `json:"table"`
	Limit      *int   `json:"limit"`
}

type sampleOut struct {
	Table     string   `json:"table"`
	Columns   []string `json:"columns"`
	Rows      [][]any  `json:"rows"`
	Truncated bool     `json:"truncated,omitempty"`
}

func (r *Registry) sampleRows(ctx context.Context, input json.RawMessage) (Result, error) {
	var in sampleInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	limit, res, ok := boundedInt(in.Limit, defaultSampleRows, 1, maxSampleRows, "limit")
	if !ok {
		return res, nil
	}
	if in.Table == "" {
		return invalid("table is required"), nil
	}
	if res, ok := checkLength("table", in.Table, maxNameBytes); !ok {
		return res, nil
	}
	cat, res, ok, err := r.discover(ctx, in.Datasource, datasource.DiscoverRequest{Kind: datasource.DiscoverSampleRows, Table: in.Table, Limit: limit}, "Sampling "+clip(in.Table, maxStepNameBytes))
	if !ok || err != nil {
		return res, err
	}
	out := frameRows(cat.Sample, limit)
	out.Table = in.Table
	return success(fmt.Sprintf("Sampled %s from %s on %s", plural(len(out.Rows), "row", "rows"), clip(in.Table, maxStepNameBytes), in.Datasource), out), nil
}

func frameRows(f *datasource.Frame, limit int) sampleOut {
	out := sampleOut{Columns: []string{}, Rows: [][]any{}}
	if f == nil {
		return out
	}
	fields := f.Fields
	if len(fields) > maxSampleColumns {
		fields = fields[:maxSampleColumns]
		out.Truncated = true
	}
	for _, field := range fields {
		out.Columns = append(out.Columns, clip(field.Name, maxStepNameBytes))
	}
	var cut bool
	if out.Columns, cut = fit(out.Columns, columnsBudget); cut {
		out.Truncated = true
		fields = fields[:len(out.Columns)]
	}
	rows := min(f.Rows(), limit)
	for i := range rows {
		row := make([]any, len(fields))
		for j, field := range fields {
			if i < len(field.Values) {
				row[j] = cellValue(field.Values[i], maxCellBytes)
			}
		}
		out.Rows = append(out.Rows, row)
	}
	if out.Rows, cut = fit(out.Rows, sampleRowsBudget); cut {
		out.Truncated = true
	}
	return out
}

func cellValue(v any, maxBytes int) any {
	switch x := v.(type) {
	case nil:
		return nil
	case time.Time:
		return x.UTC().Format(time.RFC3339Nano)
	case string:
		return clip(x, maxBytes)
	case bool:
		return x
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return nil
		}
		return x
	default:
		return clip(fmt.Sprint(x), maxBytes)
	}
}
