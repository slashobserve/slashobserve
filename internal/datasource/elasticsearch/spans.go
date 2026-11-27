package elasticsearch

import (
	"context"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const (
	SpansJaeger = "jaeger"

	JaegerTimeField = "startTimeMillis"

	spanServiceField   = "process.serviceName"
	spanOperationField = "operationName"
	spanTraceIDField   = "traceID"
	spanIDField        = "spanID"
	spanStartField     = "startTime"
	spanDurationField  = "duration"
)

// SpanIndex is an index of Jaeger span documents; only it answers the trace interfaces, so a plain index never claims to.
type SpanIndex struct {
	*Datasource
}

var (
	_ datasource.SignalSource    = (*SpanIndex)(nil)
	_ datasource.SpanIndexSource = (*SpanIndex)(nil)
	_ datasource.ServiceSource   = (*SpanIndex)(nil)
	_ datasource.TraceLogSource  = (*SpanIndex)(nil)
)

func checkSpans(name string, opts *Options) error {
	switch opts.Spans {
	case "":
		return nil
	case SpansJaeger:
	default:
		return datasource.ConfigErrorf("%s: spans must be %s", name, SpansJaeger)
	}
	switch opts.TimeField {
	case "":
		opts.TimeField = JaegerTimeField
	case JaegerTimeField:
	default:
		return datasource.ConfigErrorf("%s: an index of Jaeger spans uses the time field %s", name, JaegerTimeField)
	}
	return nil
}

// Open builds the datasource the options describe: a SpanIndex when they name a span format, else a plain index.
func Open(opts Options) (datasource.Datasource, error) {
	d, err := New(opts)
	if err != nil {
		return nil, err
	}
	if d.opts.Spans != "" {
		return &SpanIndex{Datasource: d}, nil
	}
	return d, nil
}

func (s *SpanIndex) SpanFormat() string { return s.opts.Spans }

func (s *SpanIndex) Signals() []string { return []string{datasource.SignalTraces} }

func (s *SpanIndex) Services(ctx context.Context, req datasource.ServiceRequest) (datasource.ServiceList, error) {
	if req.Signal != "" && req.Signal != datasource.SignalTraces {
		return datasource.ServiceList{}, datasource.Errorf(datasource.ErrInvalidQuery, "an index of Jaeger spans holds traces, not %s", req.Signal)
	}
	ctx, cancel := bound(ctx, discoverTimeout)
	defer cancel()
	values, _, err := s.termsValues(ctx, spanServiceField, req.Range, req.Limit, nil)
	if err != nil {
		return datasource.ServiceList{}, err
	}
	return datasource.ServiceList{Signal: datasource.SignalTraces, Field: spanServiceField, Values: filterSortLimit(values, "", req.Limit)}, nil
}

func (s *SpanIndex) TraceLogs(context.Context, datasource.TraceLogsRequest) ([]datasource.Frame, error) {
	return nil, datasource.Errorf(datasource.ErrInvalidQuery, "this datasource holds Jaeger spans, not log lines; read the lines of a trace from a logs datasource")
}
