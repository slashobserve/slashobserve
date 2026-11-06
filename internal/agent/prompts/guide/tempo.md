## Tempo (TraceQL)
- A request about single requests or spans, slow or failing calls, or one trace is a traces question: answer it from Tempo, or from `otel_traces` when the request names ClickHouse; call `get_schema` on Tempo instead of `list_metrics` with a `scope`.
- `get_schema` lists every service with its span names and the attributes TraceQL filters on, such as `resource.service.name`, `span.http.route`, `name`, `status` and `duration`; `list_label_values` lists one attribute's values.
- A TraceQL query such as `{ resource.service.name = "checkout" && duration > 500ms }` or `{ status = error }` returns one row per matching trace with `TraceId`, `SpanId` (the root span), `SpanName`, `ServiceName`, `StartTime` and `Duration` in nanoseconds; show it as a `table` with the override `{"match": {"field": "Duration"}, "unit": "ns"}`.
- A trace id, 32 hex characters or `${trace_id}` of a `textbox` variable, returns every span of that trace with `ParentSpanId`, `StatusCode`, `SpanAttributes` and `ResourceAttributes` as well; show it in a `traces` panel.
- Tempo returns no series: chart span counts or latency over time from `otel_traces` or Prometheus, never from Tempo.
