package clickhouse

import (
	"context"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	ch "github.com/ClickHouse/clickhouse-go/v2"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const (
	discoverTimeout   = 10 * time.Second
	recentWindow      = "now() - INTERVAL 1 HOUR"
	defaultSampleRows = 5
	maxSampleRows     = 20
	maxListedTables   = 10000
)

var (
	mapLabelPattern = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\['([^'\\]*)'\]$`)
	metricsSuffixes = []string{"gauge", "sum", "histogram", "exponential_histogram", "summary"}
)

func (d *Datasource) Discover(ctx context.Context, req datasource.DiscoverRequest) (*datasource.Catalog, error) {
	if err := d.readOnly(ctx); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, discoverTimeout)
	defer cancel()
	var (
		cat *datasource.Catalog
		err error
	)
	switch req.Kind {
	case datasource.DiscoverTables:
		cat, err = d.discoverTables(ctx, req)
	case datasource.DiscoverColumns:
		cat, err = d.discoverColumns(ctx, req)
	case datasource.DiscoverMetrics:
		cat, err = d.discoverMetrics(ctx, req)
	case datasource.DiscoverLabels:
		cat, err = d.discoverLabels(ctx, req)
	case datasource.DiscoverLabelValues:
		cat, err = d.discoverLabelValues(ctx, req)
	case datasource.DiscoverSampleRows:
		cat, err = d.discoverSampleRows(ctx, req)
	default:
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "unsupported discover kind %q", req.Kind)
	}
	if err != nil {
		return nil, mapError(err, discoverTimeout)
	}
	return cat, nil
}

func (d *Datasource) discoverQuery(ctx context.Context, sql string, params ch.Parameters) (*resultSet, error) {
	lim := d.limits
	lim.Timeout = discoverTimeout
	return d.fetch(ctx, sql, lim, params)
}

func tableKind(name string) string {
	switch name {
	case "otel_logs":
		return "logs"
	case "otel_traces":
		return "traces"
	}
	if suffix, ok := strings.CutPrefix(name, "otel_metrics_"); ok && slices.Contains(metricsSuffixes, suffix) {
		return "metrics_" + suffix
	}
	return "table"
}

func timeColumn(kind string, columns []datasource.Column) string {
	preferred := ""
	switch {
	case strings.HasPrefix(kind, "metrics_"):
		preferred = "TimeUnix"
	case kind == "logs" || kind == "traces":
		preferred = "Timestamp"
	}
	if preferred != "" && slices.ContainsFunc(columns, func(c datasource.Column) bool { return c.Name == preferred }) {
		return preferred
	}
	for _, c := range columns {
		if fieldType(c.Type) == datasource.FieldTime {
			return c.Name
		}
	}
	return ""
}

func where(predicates ...string) string {
	predicates = slices.DeleteFunc(predicates, func(p string) bool { return p == "" })
	if len(predicates) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(predicates, " AND ")
}

func recentPredicate(timeCol string) string {
	if timeCol == "" {
		return ""
	}
	return quoteIdent(timeCol) + " >= " + recentWindow
}

func searchPredicate(req datasource.DiscoverRequest, expr string) string {
	if req.Search == "" {
		return ""
	}
	return "positionCaseInsensitive(" + expr + ", {search:String}) > 0"
}

func baseParams(req datasource.DiscoverRequest) ch.Parameters {
	params := ch.Parameters{"limit": strconv.Itoa(req.EffectiveLimit())}
	if req.Search != "" {
		params["search"] = req.Search
	}
	return params
}

func (d *Datasource) listTables(ctx context.Context, search string, limit int) ([]string, error) {
	sql := "SELECT name FROM system.tables WHERE database = currentDatabase() AND NOT is_temporary"
	params := ch.Parameters{"limit": strconv.Itoa(limit)}
	if search != "" {
		sql += " AND positionCaseInsensitive(name, {search:String}) > 0"
		params["search"] = search
	}
	rs, err := d.discoverQuery(ctx, sql+" ORDER BY name LIMIT {limit:UInt32}", params)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(rs.rows))
	for i, row := range rs.rows {
		names[i] = stringOf(row[0])
	}
	return names, nil
}

func (d *Datasource) tableColumns(ctx context.Context, table string) ([]datasource.Column, error) {
	if table == "" {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "table is required")
	}
	rs, err := d.discoverQuery(ctx,
		"SELECT name, type FROM system.columns WHERE database = currentDatabase() AND table = {table:String} ORDER BY position",
		ch.Parameters{"table": table})
	if err != nil {
		return nil, err
	}
	if len(rs.rows) == 0 {
		return nil, datasource.Errorf(datasource.ErrNotFound, "table %q not found", table)
	}
	columns := make([]datasource.Column, len(rs.rows))
	for i, row := range rs.rows {
		columns[i] = datasource.Column{Name: stringOf(row[0]), Type: stringOf(row[1])}
	}
	return columns, nil
}

func (d *Datasource) discoverTables(ctx context.Context, req datasource.DiscoverRequest) (*datasource.Catalog, error) {
	names, err := d.listTables(ctx, req.Search, req.EffectiveLimit())
	if err != nil {
		return nil, err
	}
	tables := make([]datasource.Table, len(names))
	for i, name := range names {
		tables[i] = datasource.Table{Name: name, Kind: tableKind(name)}
	}
	return &datasource.Catalog{Tables: tables}, nil
}

func (d *Datasource) discoverColumns(ctx context.Context, req datasource.DiscoverRequest) (*datasource.Catalog, error) {
	columns, err := d.tableColumns(ctx, req.Table)
	if err != nil {
		return nil, err
	}
	if req.Search != "" {
		needle := strings.ToLower(req.Search)
		columns = slices.DeleteFunc(columns, func(c datasource.Column) bool {
			return !strings.Contains(strings.ToLower(c.Name), needle)
		})
	}
	return &datasource.Catalog{Columns: columns[:min(len(columns), req.EffectiveLimit())]}, nil
}

func (d *Datasource) discoverMetrics(ctx context.Context, req datasource.DiscoverRequest) (*datasource.Catalog, error) {
	names, err := d.listTables(ctx, "", maxListedTables)
	if err != nil {
		return nil, err
	}
	var metrics []datasource.Metric
	for _, name := range names {
		kind := tableKind(name)
		suffix, ok := strings.CutPrefix(kind, "metrics_")
		if !ok {
			continue
		}
		sql := "SELECT MetricName, any(MetricDescription), any(MetricUnit) FROM " + quoteIdent(name) +
			where(recentPredicate("TimeUnix"), searchPredicate(req, "MetricName")) +
			" GROUP BY MetricName ORDER BY MetricName LIMIT {limit:UInt32}"
		rs, err := d.discoverQuery(ctx, sql, baseParams(req))
		if err != nil {
			return nil, err
		}
		for _, row := range rs.rows {
			metrics = append(metrics, datasource.Metric{
				Name:  stringOf(row[0]),
				Type:  suffix,
				Help:  stringOf(row[1]),
				Unit:  stringOf(row[2]),
				Table: name,
			})
		}
	}
	slices.SortStableFunc(metrics, func(a, b datasource.Metric) int { return strings.Compare(a.Name, b.Name) })
	return &datasource.Catalog{Metrics: metrics[:min(len(metrics), req.EffectiveLimit())]}, nil
}

func isMapWithStringKey(typeName string) bool {
	inner, ok := strings.CutPrefix(typeName, "Map(")
	if !ok || !strings.HasSuffix(inner, ")") {
		return false
	}
	inner = inner[:len(inner)-1]
	depth := 0
	for i, r := range inner {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				return unwrapType(strings.TrimSpace(inner[:i])) == "String"
			}
		}
	}
	return false
}

func isMap(typeName string) bool { return strings.HasPrefix(typeName, "Map(") }

func metricPredicate(req datasource.DiscoverRequest, kind string, columns []datasource.Column) string {
	if req.Metric == "" || !strings.HasPrefix(kind, "metrics_") {
		return ""
	}
	if !slices.ContainsFunc(columns, func(c datasource.Column) bool { return c.Name == "MetricName" }) {
		return ""
	}
	return "MetricName = {metric:String}"
}

func (d *Datasource) discoverLabels(ctx context.Context, req datasource.DiscoverRequest) (*datasource.Catalog, error) {
	columns, err := d.tableColumns(ctx, req.Table)
	if err != nil {
		return nil, err
	}
	kind := tableKind(req.Table)
	metric := metricPredicate(req, kind, columns)
	filters := where(recentPredicate(timeColumn(kind, columns)), metric)
	needle := strings.ToLower(req.Search)
	var labels []string
	for _, c := range columns {
		if unwrapType(c.Type) == "String" && strings.Contains(strings.ToLower(c.Name), needle) {
			labels = append(labels, c.Name)
		}
	}
	for _, c := range columns {
		if !isMapWithStringKey(c.Type) {
			continue
		}
		sql := "SELECT k FROM (SELECT DISTINCT arrayJoin(mapKeys(" + quoteIdent(c.Name) + ")) AS k FROM " + quoteIdent(req.Table) + filters + ")" +
			where(searchPredicate(req, "k")) + " ORDER BY k LIMIT {limit:UInt32}"
		params := baseParams(req)
		if metric != "" {
			params["metric"] = req.Metric
		}
		rs, err := d.discoverQuery(ctx, sql, params)
		if err != nil {
			return nil, err
		}
		for _, row := range rs.rows {
			key := stringOf(row[0])
			if strings.ContainsAny(key, `'\`) {
				continue
			}
			labels = append(labels, c.Name+"['"+key+"']")
		}
	}
	return &datasource.Catalog{Labels: labels[:min(len(labels), req.EffectiveLimit())]}, nil
}

func (d *Datasource) discoverLabelValues(ctx context.Context, req datasource.DiscoverRequest) (*datasource.Catalog, error) {
	if req.Label == "" {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "label is required")
	}
	columns, err := d.tableColumns(ctx, req.Table)
	if err != nil {
		return nil, err
	}
	params := baseParams(req)
	var expr string
	if m := mapLabelPattern.FindStringSubmatch(req.Label); m != nil {
		i := slices.IndexFunc(columns, func(c datasource.Column) bool { return c.Name == m[1] })
		if i < 0 || !isMap(columns[i].Type) {
			return nil, datasource.Errorf(datasource.ErrInvalidQuery, "label %q is not a map column of table %q", req.Label, req.Table)
		}
		expr = quoteIdent(m[1]) + "[{key:String}]"
		params["key"] = m[2]
	} else {
		if !slices.ContainsFunc(columns, func(c datasource.Column) bool { return c.Name == req.Label }) {
			return nil, datasource.Errorf(datasource.ErrInvalidQuery, "label %q is not a column of table %q", req.Label, req.Table)
		}
		expr = quoteIdent(req.Label)
	}
	kind := tableKind(req.Table)
	metric := metricPredicate(req, kind, columns)
	if metric != "" {
		params["metric"] = req.Metric
	}
	sql := "SELECT v FROM (SELECT DISTINCT toString(" + expr + ") AS v FROM " + quoteIdent(req.Table) +
		where(recentPredicate(timeColumn(kind, columns)), metric) + ")" +
		where(searchPredicate(req, "v")) + " ORDER BY v LIMIT {limit:UInt32}"
	rs, err := d.discoverQuery(ctx, sql, params)
	if err != nil {
		return nil, err
	}
	values := make([]string, len(rs.rows))
	for i, row := range rs.rows {
		values[i] = stringOf(row[0])
	}
	return &datasource.Catalog{Values: values}, nil
}

func (d *Datasource) discoverSampleRows(ctx context.Context, req datasource.DiscoverRequest) (*datasource.Catalog, error) {
	columns, err := d.tableColumns(ctx, req.Table)
	if err != nil {
		return nil, err
	}
	n := req.Limit
	if n <= 0 {
		n = defaultSampleRows
	}
	n = min(n, maxSampleRows)
	sql := "SELECT * FROM " + quoteIdent(req.Table) + where(recentPredicate(timeColumn(tableKind(req.Table), columns))) + " LIMIT {n:UInt32}"
	rs, err := d.discoverQuery(ctx, sql, ch.Parameters{"n": strconv.Itoa(n)})
	if err != nil {
		return nil, err
	}
	frame := rs.tableFrame()
	return &datasource.Catalog{Sample: &frame}, nil
}
