## Tempo (TraceQL)
- A request about single requests or spans, slow or failing calls, or one trace is a traces question: answer it from Tempo, or from `otel_traces` when the request names ClickHouse; call `get_schema` on Tempo instead of `list_metrics` with a `scope`.
- `get_schema` lists the services, span names and attributes TraceQL filters on, such as `resource.service.name`, `span.http.route`, `name` and `status`; `list_label_values` lists one attribute's values.
- A TraceQL query such as `{ resource.service.name = "checkout" && duration > 500ms }` returns one row per trace with `TraceId`, `SpanId` (the root span), `SpanName`, `ServiceName`, `StartTime` and `Duration` in nanoseconds; show it as a `table` with the override `{"match": {"field": "Duration"}, "unit": "ns"}`.
- A trace id, 32 hex characters or `${trace_id}` of a `textbox` variable, returns every span of that trace with `ParentSpanId`, `StatusCode` and attributes; show it in a `traces` panel.
- A Tempo panel lists traces, not series: chart counts or latency over time from `otel_traces` or Prometheus.
- Over a window, read `trace_stats` with `group_by`, `trace_error_origins` and `trace_slow_path`.
