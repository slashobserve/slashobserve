package clickhouse

import (
	"context"
	"strconv"

	ch "github.com/ClickHouse/clickhouse-go/v2"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const ServiceColumn = "ServiceName"

var serviceTables = map[string]string{datasource.SignalLogs: "otel_logs", datasource.SignalTraces: "otel_traces"}

var _ datasource.ServiceSource = (*Datasource)(nil)

func (d *Datasource) Services(ctx context.Context, req datasource.ServiceRequest) (datasource.ServiceList, error) {
	signal, err := datasource.ResolveServiceSignal(d.Type(), req.Signal)
	if err != nil {
		return datasource.ServiceList{}, err
	}
	if err := d.readOnly(ctx); err != nil {
		return datasource.ServiceList{}, err
	}
	budget := d.clampTimeout(discoverTimeout)
	timeout := effectiveTimeout(ctx, budget)
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	sql := "SELECT DISTINCT toString(" + ServiceColumn + ") AS v FROM " + serviceTables[signal] +
		" WHERE Timestamp >= {from:DateTime64(9, 'UTC')} AND Timestamp <= {to:DateTime64(9, 'UTC')} AND " + ServiceColumn + " != '' ORDER BY v LIMIT {limit:UInt32}"
	rs, err := d.discoverQuery(ctx, sql, ch.Parameters{
		"from":  parameterTime(req.Range.From),
		"to":    parameterTime(req.Range.To),
		"limit": strconv.Itoa(req.Limit),
	})
	if err != nil {
		return datasource.ServiceList{}, mapError(ctx, err, timeout)
	}
	values := make([]string, len(rs.rows))
	for i, row := range rs.rows {
		values[i] = stringOf(row[0])
	}
	return datasource.ServiceList{Signal: signal, Field: ServiceColumn, Values: values}, nil
}
