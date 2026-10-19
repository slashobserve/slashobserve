# ObserveSpec authoring guide

A dashboard is one ObserveSpec v0 JSON document; the JSON Schema is authoritative and the example shows the format only.
Discover real metric, label, table and column names with the tools before writing queries; never copy names from the example.
Leave out every value that equals its default.

## Panels
- A data panel has `id`, `type`, `title`, `gridPos`, `datasource` (a datasource uid such as `prom`) and `queries`.
- `timeseries`: a trend over time; its queries run as range queries.
- `stat`: one current number, such as an error ratio or a p99; `gauge`: one number against `min`, `max` and thresholds.
- `bargauge`: compare a few current values across series; `piechart`: shares of a total.
- `table`: rows such as top-N lists or slow spans; `logs`: log lines from ClickHouse.
- `heatmap` and `histogram`: distributions such as latency; `state-timeline`: a state per series over time.
- Panel ids are unique across the dashboard, row children included, lowercase snake case, prefixed `p_`.
- A `row` groups the panels after it under a title: put them in the row's `panels`, each with its own `gridPos` below the row.
- Never create a `placeholder` panel; they come only from Grafana imports. You may move one (its `gridPos`, or into or out of a row) or delete it, and must leave its other fields unchanged; to implement one, replace it with a real panel that keeps its id.
- Add a `text` panel only when the request asks for notes, and write plain sentences in `content`.

## Queries
- Each query has `query` and, when it returns several series, a `legend` such as `{{route}}`.
- Put related series in one panel with several queries, such as requests and errors, or p50 and p99; queries are named `A`, `B`, `C` by position, so write `ref` only when you need a stable name.
- A query may set its own `datasource`, `instant: true` for one value per series, `interval` as its minimum step, or `hide: true` to run nowhere.

## Layout
- The grid is 24 columns wide; `x + w` must not exceed 24 and panels must not overlap.
- Typical sizes: timeseries `w` 12 `h` 8, stat or gauge `w` 6 `h` 4, table `w` 24 `h` 8, row `w` 24 `h` 1; fill rows left to right, top to bottom.

## Field options
- `unit` is a unit id from the schema: `reqps` for request rates, `s` or `ms` for durations, `percentunit` for 0-1 ratios, `percent` for 0-100, `bytes`, `short` for counts, or `suffix:<text>`.
- `thresholds` ascend; the first step may leave out `value` and is then the base color, for example `[{"color": "ok"}, {"value": 0.25, "color": "warning"}, {"value": 0.5, "color": "critical"}]`; colors are `ok`, `info`, `warning`, `critical` or a hex color.
- `decimals`, `min`, `max`, `displayName`, `noValue` and `mappings` refine how values read.
- `overrides` set the same options for matching series, for example `{"match": {"query": "B"}, "unit": "s"}`.
- `display` holds the few per-type options in the schema; set it only when the request asks for one.
- `transform` reshapes results in the browser; use it only when a query cannot return the shape directly.

## Prometheus (PromQL)
- Use `$__rate_interval` as the window in `rate()` and `increase()`; use `$__range` for an increase over the whole range.
- Aggregate with `sum by (label)` so the series count stays small, and set `legend` to `{{label}}`.
- Latency quantiles: `histogram_quantile(0.99, sum by (le) (rate(<name>_bucket[$__rate_interval])))`.
- A ratio whose numerator may have no series yet (errors before any error happened): `(sum(rate(<errors>[$__rate_interval])) or vector(0)) / sum(rate(<all>[$__rate_interval]))`.
- Comparing with an earlier period: put both periods in one query, `label_replace(<now>, "period", "now", "", "") or label_replace(<same with offset 1d>, "period", "1d ago", "", "")`, so the panel has data before the earlier period was recorded.

## ClickHouse (OpenTelemetry exporter tables)
- `otel_logs` (Timestamp, ServiceName, SeverityText, Body, LogAttributes), `otel_traces` (Timestamp, ServiceName, SpanName, Duration in nanoseconds, StatusCode, SpanAttributes), `otel_metrics_*`; check columns with `get_schema`.
- Always filter time with `$__timeFilter(Timestamp)`.
- Time series: `SELECT $__timeInterval(Timestamp) AS time, ... GROUP BY time ORDER BY time`.
- Filter by a variable that may select All with `$__conditionalAll(ServiceName IN (${service:singlequote}), $service)`.
- Exactly one `SELECT` statement, no semicolon; add `LIMIT` to tables.

## Variables
- Add a variable only when the request asks for a breakdown the user will filter.
- `query` variables: Prometheus `label_values(<metric>, <label>)`, referenced as `$name`, or `${name:regex}` inside `=~`; ClickHouse references use `${name:sqlstring}` inside `IN (...)`.
- `custom` lists fixed values; `interval` lists steps in `options`, such as `["1m", "5m", "1h"]`, with `"auto": true`; `constant` holds one shared value; `textbox` takes free text; `adhoc` adds key and value filters, which Prometheus applies to every query on its datasource and a ClickHouse query uses as `AND $name` in its WHERE clause.
- An `interval` variable with `"auto": true` and no `default` starts on auto; set `default` to one of its `options` to start on a fixed step.
- Global values for any query: `$__interval_ms`, `$__range_s`, `$__range_ms`, `${__from:date:seconds}`, `${__to:date:seconds}`, `$__dashboard` and `${__user.login}`.

## Time
- Honor a requested window: "last 6h" is `"time": {"from": "now-6h", "to": "now"}`; otherwise use `now-1h`.

## Saving
- Every query runs during the save, and every data panel must return data for the default time range and variable values, or the save is rejected with the reason; fix the named panels and save again.
- Never put HTML or scripts in any field; titles, descriptions and text panels are plain text.
