package tools

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
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

func init() { registerEntries((*Registry).discoveryEntries) }

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

func typeOf(inst datasource.Instance) spec.DatasourceType {
	return spec.DatasourceType(inst.Datasource.Type())
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
	Scope      string `json:"scope"`
	Limit      *int   `json:"limit"`
	Offset     *int   `json:"offset"`
}

type listLabelsInput struct {
	Datasource string `json:"datasource"`
	Metric     string `json:"metric"`
	Table      string `json:"table"`
	Search     string `json:"search"`
	From       string `json:"from"`
	To         string `json:"to"`
	Limit      *int   `json:"limit"`
	Offset     *int   `json:"offset"`
}

type listLabelValuesInput struct {
	Datasource string `json:"datasource"`
	Label      string `json:"label"`
	Metric     string `json:"metric"`
	Table      string `json:"table"`
	Search     string `json:"search"`
	From       string `json:"from"`
	To         string `json:"to"`
	Limit      *int   `json:"limit"`
	Offset     *int   `json:"offset"`
}

func namesRange(from, to string) (datasource.TimeRange, Result, bool) {
	switch {
	case from == "" && to == "":
		return datasource.TimeRange{}, Result{}, true
	case from == "" || to == "":
		return datasource.TimeRange{}, invalid("from and to must be given together"), false
	}
	return resolveRange(from, to, time.Now())
}

func namesLimit(p *int) int {
	if p == nil {
		return 0
	}
	return cmp.Or(*p, -1)
}

func pagedNamesLimit(p *int, offset int) int {
	limit := namesLimit(p)
	if limit == 0 {
		limit = queryproxy.DefaultNamesLimit
	}
	if limit < 1 || limit > queryproxy.MaxNamesLimit {
		return limit
	}
	return min(limit+offset, queryproxy.MaxNamesLimit)
}

func offsetOf(p *int) (int, Result, bool) {
	return boundedInt(p, 0, 0, queryproxy.MaxNamesLimit-1, "offset")
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

func discovered(res Result, datasource, of string, names []string) Result {
	res.Discovered = &Discovery{Datasource: datasource, Of: of, Names: names}
	return res
}

func described(noun, joiner, part string) string {
	if part == "" {
		return noun
	}
	return noun + " " + joiner + " " + clip(part, maxStepNameBytes)
}

type metricsOut struct {
	Datasource string              `json:"datasource"`
	Metrics    []datasource.Metric `json:"metrics"`
	truncation
}

func (r *Registry) listMetrics(ctx context.Context, input json.RawMessage) (Result, error) {
	var in listMetricsInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	offset, res, ok := offsetOf(in.Offset)
	if !ok {
		return res, nil
	}
	if in.Scope != "" {
		return r.listInventory(ctx, in, offset)
	}
	names, res, ok, err := r.listNames(ctx, queryproxy.NamesRequest{
		Datasource: in.Datasource,
		Kind:       datasource.DiscoverMetrics,
		Search:     in.Search,
		Limit:      pagedNamesLimit(in.Limit, offset),
	}, "metrics")
	if !ok || err != nil {
		return res, err
	}
	metrics, cut := page(names.Metrics, offset, names.Truncated, listBudget)
	listed := make([]string, len(metrics))
	for i, m := range metrics {
		listed[i] = m.Name
	}
	res = success(fmt.Sprintf("Listed %s on %s", plural(len(metrics), "metric", "metrics"), names.Datasource), metricsOut{Datasource: names.Datasource, Metrics: metrics, truncation: cut})
	return discovered(res, names.Datasource, described("metrics", "matching", in.Search), listed), nil
}

type familyOut struct {
	Name   string   `json:"name"`
	Type   string   `json:"type,omitempty"`
	Unit   string   `json:"unit,omitempty"`
	Labels []string `json:"labels,omitempty"`
	Series int      `json:"series"`
}

type inventoryOut struct {
	Datasource      string              `json:"datasource"`
	Scope           string              `json:"scope"`
	FamiliesTotal   int                 `json:"families_total"`
	RuntimeFamilies int                 `json:"runtime_families"`
	Families        []familyOut         `json:"families"`
	CommonLabels    []string            `json:"common_labels,omitempty"`
	Grouped         map[string][]string `json:"grouped"`
	truncation
}

const scopeTakesNoFilter = "scope returns every family in one result; leave out search and limit"

func (r *Registry) listInventory(ctx context.Context, in listMetricsInput, offset int) (Result, error) {
	if in.Search != "" || in.Limit != nil {
		return invalid("%s", scopeTakesNoFilter), nil
	}
	names, res, ok, err := r.listNames(ctx, queryproxy.NamesRequest{Datasource: in.Datasource, Kind: datasource.DiscoverMetrics, Scope: in.Scope}, "metric families")
	if !ok || err != nil {
		return res, err
	}
	if inst, ok := r.deps.Datasources.Get(names.Datasource); !ok || typeOf(inst) != spec.Loki {
		inventories.put(ctx, names.Families)
	}
	out := compactInventory(inventoryOut{Datasource: names.Datasource, Scope: in.Scope, FamiliesTotal: names.FamiliesTotal, RuntimeFamilies: names.RuntimeFamilies}, names.Families, offset, names.Truncated)
	listed := make([]string, len(out.Families))
	for i, f := range out.Families {
		listed[i] = f.Name
	}
	of := described("metric families", "in", in.Scope)
	res = success(fmt.Sprintf("Listed %s in %s on %s", plural(len(listed), "metric family", "metric families"), clip(in.Scope, maxStepNameBytes), names.Datasource), out)
	return discovered(res, names.Datasource, of, listed), nil
}

type inventoryShape func(base inventoryOut, families []datasource.Family) inventoryOut

var inventoryShapes = []inventoryShape{withEveryLabel, withSharedLabelsOnce, withSharedLabelsOnly}

func compactInventory(base inventoryOut, families []datasource.Family, offset int, upstreamCut bool) inventoryOut {
	window := families[min(offset, len(families)):]
	for _, shape := range inventoryShapes {
		if out := shape(base, window); inventoryFits(out) {
			if upstreamCut {
				out.truncation = truncation{Truncated: true, TruncatedBy: truncatedByLimit}
			}
			return out
		}
	}
	kept := sort.Search(len(window), func(n int) bool { return !inventoryFits(withSharedLabelsOnly(base, window[:n+1])) })
	out := withSharedLabelsOnly(base, window[:kept])
	out.truncation = truncation{Truncated: true, TruncatedBy: truncatedByBytes}
	if next := offset + kept; next < queryproxy.MaxNamesLimit {
		out.NextOffset = &next
	}
	return out
}

func inventoryFits(out inventoryOut) bool {
	body, err := json.Marshal(out)
	return err == nil && len(body) <= listBudget
}

func withEveryLabel(base inventoryOut, families []datasource.Family) inventoryOut {
	return inventoryOf(base, families, nil, true)
}

func withSharedLabelsOnce(base inventoryOut, families []datasource.Family) inventoryOut {
	return inventoryOf(base, families, sharedLabels(families), true)
}

func withSharedLabelsOnly(base inventoryOut, families []datasource.Family) inventoryOut {
	return inventoryOf(base, families, sharedLabels(families), false)
}

func inventoryOf(base inventoryOut, families []datasource.Family, shared []string, ownLabels bool) inventoryOut {
	out := base
	out.Families = make([]familyOut, len(families))
	out.CommonLabels = shared
	out.Grouped = map[string][]string{}
	members := map[string][]string{}
	for i, f := range families {
		out.Families[i] = familyOut{Name: f.Name, Type: f.Type, Unit: f.Unit, Series: f.Series}
		if ownLabels {
			out.Families[i].Labels = slices.DeleteFunc(slices.Clone(f.Labels), func(l string) bool { return slices.Contains(shared, l) })
		}
		prefix := familyPrefix(f.Name)
		members[prefix] = append(members[prefix], f.Name)
	}
	for prefix, names := range members {
		if len(names) > 1 {
			out.Grouped[prefix] = names
		}
	}
	return out
}

func sharedLabels(families []datasource.Family) []string {
	if len(families) < 2 {
		return nil
	}
	shared := slices.Clone(families[0].Labels)
	for _, f := range families[1:] {
		shared = slices.DeleteFunc(shared, func(l string) bool { return !slices.Contains(f.Labels, l) })
	}
	return shared
}

func familyPrefix(name string) string {
	first := strings.IndexByte(name, '_')
	if first < 0 {
		return name
	}
	second := strings.IndexByte(name[first+1:], '_')
	if second < 0 {
		return name
	}
	return name[:first+1+second]
}

type labelsOut struct {
	Datasource string   `json:"datasource"`
	Labels     []string `json:"labels"`
	truncation
}

func (r *Registry) listLabels(ctx context.Context, input json.RawMessage) (Result, error) {
	var in listLabelsInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	offset, res, ok := offsetOf(in.Offset)
	if !ok {
		return res, nil
	}
	rng, res, ok := namesRange(in.From, in.To)
	if !ok {
		return res, nil
	}
	names, res, ok, err := r.listNames(ctx, queryproxy.NamesRequest{
		Datasource: in.Datasource,
		Kind:       datasource.DiscoverLabels,
		Search:     in.Search,
		Metric:     in.Metric,
		Table:      in.Table,
		Limit:      pagedNamesLimit(in.Limit, offset),
		Range:      rng,
	}, "labels")
	if !ok || err != nil {
		return res, err
	}
	labels, cut := page(names.Labels, offset, names.Truncated, listBudget)
	res = success(fmt.Sprintf("Listed %s on %s", plural(len(labels), "label", "labels"), names.Datasource), labelsOut{Datasource: names.Datasource, Labels: labels, truncation: cut})
	return discovered(res, names.Datasource, described("labels", "of", cmp.Or(in.Metric, in.Table)), labels), nil
}

type labelValuesOut struct {
	Datasource string   `json:"datasource"`
	Label      string   `json:"label"`
	Values     []string `json:"values"`
	truncation
}

func (r *Registry) listLabelValues(ctx context.Context, input json.RawMessage) (Result, error) {
	var in listLabelValuesInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	offset, res, ok := offsetOf(in.Offset)
	if !ok {
		return res, nil
	}
	rng, res, ok := namesRange(in.From, in.To)
	if !ok {
		return res, nil
	}
	names, res, ok, err := r.listNames(ctx, queryproxy.NamesRequest{
		Datasource: in.Datasource,
		Kind:       datasource.DiscoverLabelValues,
		Search:     in.Search,
		Metric:     in.Metric,
		Label:      in.Label,
		Table:      in.Table,
		Limit:      pagedNamesLimit(in.Limit, offset),
		Range:      rng,
	}, "label values")
	if !ok || err != nil {
		return res, err
	}
	values, cut := page(names.Values, offset, names.Truncated, listBudget)
	res = success(fmt.Sprintf("Listed %s on %s", plural(len(values), "label value", "label values"), names.Datasource), labelValuesOut{Datasource: names.Datasource, Label: names.Label, Values: values, truncation: cut})
	of := described("values", "of", names.Label)
	if in.Metric != "" {
		of += " on " + clip(in.Metric, maxStepNameBytes)
	}
	return discovered(res, names.Datasource, of, values), nil
}

func pageSchema[T any](items []T, offset int) ([]T, truncation) {
	end := offset + maxDiscoverItems
	return page(items[:min(len(items), end)], offset, len(items) > end, listBudget)
}

type schemaInput struct {
	Datasource string `json:"datasource"`
	Table      string `json:"table"`
	Offset     *int   `json:"offset"`
}

type tablesOut struct {
	Tables []datasource.Table `json:"tables"`
	truncation
}

type columnsOut struct {
	Table   string              `json:"table"`
	Columns []datasource.Column `json:"columns"`
	truncation
}

func (r *Registry) getSchema(ctx context.Context, input json.RawMessage) (Result, error) {
	var in schemaInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	if res, ok := checkLength("table", in.Table, maxNameBytes); !ok {
		return res, nil
	}
	offset, res, ok := offsetOf(in.Offset)
	if !ok {
		return res, nil
	}
	inst, res, ok := r.instance(in.Datasource)
	if !ok {
		return res, nil
	}
	if typeOf(inst) == spec.Tempo {
		return r.spanSchema(ctx, in, offset)
	}
	limit := offset + maxDiscoverItems + 1
	if in.Table == "" {
		cat, res, ok, err := r.discover(ctx, in.Datasource, datasource.DiscoverRequest{Kind: datasource.DiscoverTables, Limit: limit}, "Listing tables")
		if !ok || err != nil {
			return res, err
		}
		tables, cut := pageSchema(cat.Tables, offset)
		names := make([]string, len(tables))
		for i, t := range tables {
			names[i] = t.Name
		}
		res = success(fmt.Sprintf("Listed %s on %s", plural(len(tables), "table", "tables"), in.Datasource), tablesOut{Tables: tables, truncation: cut})
		return discovered(res, in.Datasource, "tables", names), nil
	}
	cat, res, ok, err := r.discover(ctx, in.Datasource, datasource.DiscoverRequest{Kind: datasource.DiscoverColumns, Table: in.Table, Limit: limit}, "Reading the "+clip(in.Table, maxStepNameBytes)+" schema")
	if !ok || err != nil {
		return res, err
	}
	columns, cut := pageSchema(cat.Columns, offset)
	names := make([]string, len(columns))
	for i, c := range columns {
		names[i] = c.Name
	}
	res = success(fmt.Sprintf("Read the %s schema on %s", clip(in.Table, maxStepNameBytes), in.Datasource), columnsOut{Table: in.Table, Columns: columns, truncation: cut})
	return discovered(res, in.Datasource, described("columns", "of", in.Table), names), nil
}

type spanSchemaOut struct {
	Datasource string   `json:"datasource"`
	Columns    []string `json:"columns"`
	Rows       [][]any  `json:"rows"`
	Attributes []string `json:"attributes"`
	Truncated  bool     `json:"truncated,omitempty"`
}

func (r *Registry) spanSchema(ctx context.Context, in schemaInput, offset int) (Result, error) {
	if in.Table != "" || offset > 0 {
		return invalid("tempo has no tables; leave out table and offset"), nil
	}
	cat, res, ok, err := r.discover(ctx, in.Datasource, datasource.DiscoverRequest{Kind: datasource.DiscoverTables}, "Listing services and span names")
	if !ok || err != nil {
		return res, err
	}
	rows := frameRows(cat.Sample, maxDiscoverItems)
	attributes, cut := fit(cat.Labels, listBudget-sampleRowsBudget)
	out := spanSchemaOut{
		Datasource: in.Datasource,
		Columns:    rows.Columns,
		Rows:       rows.Rows,
		Attributes: attributes,
		Truncated:  rows.Truncated || cut || cat.Truncated,
	}
	services, spanNames := distinctColumns(rows.Rows)
	names := slices.Concat(services, spanNames, attributes)
	res = success(fmt.Sprintf("Listed %s and their span names on %s", plural(len(services), "service", "services"), in.Datasource), out)
	return discovered(res, in.Datasource, "services, span names and attributes", names), nil
}

func distinctColumns(rows [][]any) (first, second []string) {
	add := func(seen []string, v any) []string {
		if s, ok := v.(string); ok && !slices.Contains(seen, s) {
			return append(seen, s)
		}
		return seen
	}
	for _, row := range rows {
		if len(row) >= 2 {
			first, second = add(first, row[0]), add(second, row[1])
		}
	}
	return first, second
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
	if res, ok := checkLength("table", in.Table, maxNameBytes); !ok {
		return res, nil
	}
	inst, res, ok := r.instance(in.Datasource)
	if !ok {
		return res, nil
	}
	if in.Table == "" && typeOf(inst) != spec.Loki {
		return invalid("table is required"), nil
	}
	source := clip(cmp.Or(in.Table, "every stream"), maxStepNameBytes)
	cat, res, ok, err := r.discover(ctx, in.Datasource, datasource.DiscoverRequest{Kind: datasource.DiscoverSampleRows, Table: in.Table, Limit: limit}, "Sampling "+source)
	if !ok || err != nil {
		return res, err
	}
	out := frameRows(cat.Sample, limit)
	out.Table = in.Table
	return success(fmt.Sprintf("Sampled %s from %s on %s", plural(len(out.Rows), "row", "rows"), source, in.Datasource), out), nil
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
