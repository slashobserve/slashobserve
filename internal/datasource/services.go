package datasource

import (
	"context"
	"slices"
)

const (
	SignalLogs   = "logs"
	SignalTraces = "traces"
)

type ServiceRequest struct {
	Signal string
	Range  TimeRange
	Limit  int
}

type ServiceList struct {
	Signal string
	Field  string
	Values []string
}

type ServiceSource interface {
	Services(ctx context.Context, req ServiceRequest) (ServiceList, error)
}

func ServiceSignals(datasourceType string) []string {
	switch datasourceType {
	case "loki", "elasticsearch", "opensearch":
		return []string{SignalLogs}
	case "tempo", "jaeger":
		return []string{SignalTraces}
	case "clickhouse":
		return []string{SignalLogs, SignalTraces}
	}
	return nil
}

func ResolveServiceSignal(datasourceType, requested string) (string, error) {
	signals := ServiceSignals(datasourceType)
	switch {
	case len(signals) == 0:
		return "", Errorf(ErrInvalidQuery, "%s datasources hold no services", datasourceType)
	case requested == "":
		return signals[0], nil
	case slices.Contains(signals, requested):
		return requested, nil
	}
	return "", Errorf(ErrInvalidQuery, "%s datasources hold %s, not %s", datasourceType, signals[0], requested)
}
