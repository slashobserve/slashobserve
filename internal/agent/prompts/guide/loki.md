## Loki (LogQL)
- Answer a question about log lines, messages, errors or events from Loki, or from `otel_logs` when the request names ClickHouse.
- `list_labels` lists stream labels and `list_label_values` their values, both narrowed by a stream selector in `metric`; `sample_rows` reads recent lines, of every stream or of a stream selector in `table`.
- A log query starts with a stream selector and returns lines: `{service_name="payments"} |= "timeout"`, or `{service_name="payments"} | severity_text="ERROR"`; show it in a full-width `logs` panel.
- A metric query wraps a selector in a range function, like PromQL: `sum by (service_name) (count_over_time({service_name=~".+"}[$__interval]))` with `legend` `{{service_name}}` and `unit` `short`, or `rate()` for lines per second.
- The window is `$__interval` or `$__range`; Loki has no `$__rate_interval` and no ClickHouse macros.
- A count, share or quantile of lines over a window: `measure_aggregate` with the log query (`total`, `ratio`, or `unwrap` a field for `quantile`, `max` or `avg`).
- `query` variables: `label_values(service_name)` or `label_values({namespace="shop"}, service_name)`.
- `list_metrics` with a stream selector as `scope` counts the streams that carry each label key.
