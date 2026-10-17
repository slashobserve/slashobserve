# ObserveSpec authoring guide

A dashboard is one ObserveSpec v0 JSON document; the JSON Schema is authoritative and the example shows the format only.
Discover real metric, label, table and column names with the tools before writing queries; never copy names from the example.

## Panels
- `timeseries`: a trend over time; the server runs it as a range query.
- `stat`: one current number, such as an error ratio or a p99.
- `table`: rows such as top-N lists, log messages or slow spans.
- `bargauge`: compare a few current values across series.
- Panel ids are unique, lowercase snake case, prefixed `p_`.

## Layout
- The grid is 24 columns wide; `x + w` must not exceed 24 and panels must not overlap.
- Typical sizes: timeseries `w` 12 `h` 8, stat `w` 6 `h` 4, table `w` 24 `h` 8; fill rows left to right, top to bottom.

## Prometheus (PromQL)
- Use `$__rate_interval` as the window in `rate()` and `increase()`; use `$__range` for an increase over the whole range.
- Aggregate with `sum by (label)` so the series count stays small, and set `legend` to `{{label}}`.
- Latency quantiles: `histogram_quantile(0.99, sum by (le) (rate(<name>_bucket[$__rate_interval])))`.
- A ratio whose numerator may have no series yet (errors before any error happened): `(sum(rate(<errors>[$__rate_interval])) or vector(0)) / sum(rate(<all>[$__rate_interval]))`.
- Comparing with an earlier period: put both periods in one query, `label_replace(<now>, "period", "now", "", "") or label_replace(<same with offset 1d>, "period", "1d ago", "", "")`, so the panel has data before the earlier period was recorded.

## ClickHouse (OpenTelemetry exporter tables)
- `otel_logs` (Timestamp, ServiceName, SeverityText, Body, LogAttributes), `otel_traces` (Timestamp, ServiceName, SpanName, Duration in nanoseconds, StatusCode, SpanAttributes), `otel_metrics_*`; check columns with `get_schema`.
- Always filter time with `$__timeFilter(Timestamp)`.
- Time series: `SELECT $__timeGroup(Timestamp) AS time, ... GROUP BY time ORDER BY time`.
- Exactly one `SELECT` statement, no semicolon; add `LIMIT` to tables.

## Variables
- Add a variable only when the request asks for a breakdown the user will filter.
- Prometheus query variables use `label_values(<metric>, <label>)`; reference them as `$name`, or `${name:regex}` inside `=~`.
- ClickHouse references use `${name:sqlstring}` inside `IN (...)`.

## Units and thresholds
- Use a unit id from the schema's enum: `reqps` for request rates, `s` or `ms` for durations, `percentunit` for 0-1 ratios, `percent` for 0-100, `bytes`, `short` for counts.
- Threshold values ascend; colors are `warning`, `critical` or a hex color.

## Time
- Honor a requested window: "last 6h" is `{"from": "now-6h", "to": "now"}`; otherwise use `now-1h`.

## Saving
- Every panel must return data for the default time range and variable values, or the save is rejected with the reason; fix the named panels and save again.
- Never put HTML, scripts or Markdown in any field; titles and descriptions are plain text.
