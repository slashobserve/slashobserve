## Loki (LogQL)
- A request to read or count log lines, messages, errors or events is a logs question: answer it from Loki, or from `otel_logs` when the request names ClickHouse.
- `list_labels` lists stream labels and `list_label_values` their values, both narrowed by a stream selector in `metric`; `sample_rows` reads recent lines with their labels, every stream by default or a stream selector in `table`.
- A log query starts with a stream selector and returns lines: `{service_name="payments"} |= "timeout"`, or `{service_name="payments"} | severity_text="ERROR"`; show it in a full-width `logs` panel, whose rows hold `Time`, `Body`, `level` and the stream labels.
- A metric query wraps a selector in a range function and returns series like PromQL: `sum by (service_name) (count_over_time({service_name=~".+"}[$__interval]))` with `legend` `{{service_name}}` and `unit` `short`, or `rate()` for lines per second.
- The window is `$__interval` or `$__range`; Loki has no `$__rate_interval` and no ClickHouse macros.
- `query` variables: `label_values(service_name)` or `label_values({namespace="shop"}, service_name)`.
- `list_metrics` with a stream selector as `scope` lists each stream label key with the number of streams that carry it.
