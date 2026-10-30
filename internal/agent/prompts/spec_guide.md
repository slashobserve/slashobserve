# ObserveSpec authoring guide

A dashboard is one ObserveSpec v0 JSON document; the JSON Schema is authoritative, the example shows the format only.
Discover metric, label, table and column names with the tools; never copy them from the example.
Leave out every value that equals its default.

## Panels
- A data panel has `id`, `type`, `title`, `gridPos`, `datasource` (a datasource uid such as `prom`) and `queries`.
- `timeseries`: a trend over time; its queries run as range queries.
- `stat`: one current number, such as an error ratio; `gauge`: one number against `min`, `max` and thresholds.
- `bargauge`: compare a few current values across series; `piechart`: shares of a total.
- `table`: rows such as top-N lists or slow spans; `logs`: ClickHouse log lines.
- `heatmap` and `histogram`: distributions such as latency; `state-timeline`: states per series over time.
- `barchart`: values across categories, such as requests per route; use an `instant: true` query with `format: "table"` (the first label column is the x axis) or set `display.xField`.
- `status-history`: one row per series, one cell per sample, for discrete states.
- `trend`: a line over a numeric x axis instead of time (`display.xField`, default the first number column).
- `xychart`: one numeric column (`display.xField`) plotted against the others.
- `candlestick`: `open`, `high`, `low`, `close` and optional `volume` columns over time.
- `nodeGraph`: a service map; one query returns edges with `id`, `source`, `target` and optional `mainstat`, an optional second query returns nodes with `id`, `title` and `mainstat`.
- `traces`: one trace as a span waterfall from ClickHouse rows of `traceID`, `spanID`, `parentSpanID`, `serviceName`, `operationName`, `startTime`, `duration` (milliseconds) and optional `tags`.
- `flamegraph`: only for a query already returning `level`, `value`, `self` and `label` columns in depth-first order.
- `geomap`: world-map points from `lat`/`lon` (or `latitude`/`longitude`) or `geohash` columns, sized by `display.sizeField`, colored by thresholds.
- `canvas`: positioned `display.elements` (`metric`, `text`, `rectangle`, `ellipse`; positions in percent) bound to fields, `"fill": "field"` for threshold colors, `display.connections` between elements; only when asked for a diagram.
- `alertlist`: firing or pending alerts from an instant query on `ALERTS`; an empty list is valid.
- `dashlist`: links to dashboards matching `display.query` or `display.tags`; no datasource or queries.
- `news`: the items of one RSS or Atom feed at `display.feedUrl` (https only); no datasource or queries; it shows items only when the feed's domain is on the server's outbound allowlist, so add one only when asked.
- Panel ids are unique across the dashboard, row children included, lowercase snake case, prefixed `p_`; a dashboard holds at most 200 panels.
- A `row` groups the panels after it under a title: put them in the row's `panels`, each with its own `gridPos` below the row.
- Never create a `placeholder` panel; they come only from Grafana imports. You may move it (`gridPos`, or into or out of a row) or delete it, but leave its other fields unchanged; to implement one, replace it with a real panel that keeps its id.
- Leave text panels with `mode: "html"` and canvas element `actions` exactly as they are; they come only from imports and people. You may move or delete them.
- Add a `text` panel only when the request asks for notes, as plain sentences in `content`.

## Queries
- Each query has `query` and, if it returns several series, a `legend` such as `{{route}}`.
- Put related series in one panel with several queries, such as requests and errors; queries are named `A`, `B`, `C` by position; write `ref` only for a stable name.
- A query may set its own `datasource`, `instant: true` for one value per series, `interval` as its minimum step (a duration, or `${name}` of an `interval` variable), `exemplar: true` to mark exemplars on a Prometheus time series, or `hide: true` to run nowhere.

## Layout
- The grid is 24 columns wide; `x + w` may not exceed 24 and panels may not overlap.
- Typical sizes: timeseries `w` 12 `h` 8, stat or gauge `w` 6 `h` 4, table `w` 24 `h` 8, row `w` 24 `h` 1; fill rows left to right, top to bottom.

## Field options
- `unit` is a unit id from the schema: `reqps` for request rates, `s` or `ms` for durations, `percentunit` for 0-1 ratios, `percent` for 0-100, `bytes`, `short` for counts, or `suffix:<text>`.
- `thresholds` ascend; the first step may leave out `value` and is then the base color, for example `[{"color": "ok"}, {"value": 0.25, "color": "warning"}, {"value": 0.5, "color": "critical"}]`; colors are `ok`, `info`, `warning`, `critical` or a hex color.
- `decimals`, `min`, `max`, `displayName`, `noValue` and `mappings` (each needs `text`, `color` or both) refine values.
- `overrides` set the same options for matching series, for example `{"match": {"query": "B"}, "unit": "s"}`.
- `display` holds the per-type options in the schema; set it only when the request asks for one.
- `transform` reshapes results in the browser; use it only when a query cannot return the shape.

## Prometheus (PromQL)
- Use `$__rate_interval` as the window in `rate()` and `increase()`; use `$__range` for an increase over the whole range.
- Aggregate with `sum by (label)` to keep series few; set `legend` to `{{label}}`.
- Latency quantiles: `histogram_quantile(0.99, sum by (le) (rate(<name>_bucket[$__rate_interval])))`.
- A ratio whose numerator may have no series yet (errors before any occurred): `(sum(rate(<errors>[$__rate_interval])) or vector(0)) / sum(rate(<all>[$__rate_interval]))`.
- An earlier-period comparison goes in one query: `label_replace(<now>, "period", "now", "", "") or label_replace(<same with offset 1d>, "period", "1d ago", "", "")`, so the panel has data before the earlier period exists.
- `ALERTS{alertstate="firing"}` lists firing alerts; a second query on `ALERTS_FOR_STATE` shows since when.

## ClickHouse (OpenTelemetry exporter tables)
- `otel_logs` (Timestamp, ServiceName, SeverityText, Body, LogAttributes), `otel_traces` (Timestamp, ServiceName, SpanName, Duration in nanoseconds, StatusCode, SpanAttributes), `otel_metrics_*`; check columns with `get_schema`.
- Always filter time with `$__timeFilter(Timestamp)`.
- Time series: `SELECT $__timeInterval(Timestamp) AS time, ServiceName, count() AS logs FROM otel_logs WHERE $__timeFilter(Timestamp) GROUP BY time, ServiceName ORDER BY time`.
- Filter by a variable that can select All with `$__conditionalAll(ServiceName IN (${service:singlequote}), $service)`.
- Exactly one `SELECT` statement, no semicolon; add `LIMIT` to tables.
- Service map edges: `SELECT concat(p.ServiceName, ' -> ', c.ServiceName) AS id, p.ServiceName AS source, c.ServiceName AS target, count() AS mainstat FROM otel_traces AS c INNER JOIN otel_traces AS p ON c.ParentSpanId = p.SpanId AND c.TraceId = p.TraceId WHERE $__timeFilter(c.Timestamp) AND $__timeFilter(p.Timestamp) AND p.ServiceName != c.ServiceName GROUP BY source, target LIMIT 200`.
- One trace, filtered by a `textbox` variable that falls back to the latest trace: `SELECT TraceId AS traceID, SpanId AS spanID, ParentSpanId AS parentSpanID, ServiceName AS serviceName, SpanName AS operationName, Timestamp AS startTime, Duration / 1000000 AS duration, SpanAttributes AS tags FROM otel_traces WHERE TraceId = if(${trace_id:sqlstring} = '', (SELECT TraceId FROM otel_traces WHERE $__timeFilter(Timestamp) ORDER BY Timestamp DESC LIMIT 1), ${trace_id:sqlstring}) ORDER BY startTime LIMIT 1000`.

## Variables
- Add a variable only when the request asks for a breakdown the user will filter.
- `query` variables: Prometheus `label_values(<metric>, <label>)`, referenced as `$name`, or `${name:regex}` inside `=~`; ClickHouse references use `${name:sqlstring}` inside `IN (...)`.
- `custom` lists fixed values; `interval` lists steps in `options`, such as `["1m", "5m", "1h"]`, with `"auto": true`; `constant` holds one shared value; `textbox` takes free text; `adhoc` adds key and value filters, which Prometheus applies to every query on its datasource and a ClickHouse query uses as `AND $name` in its WHERE clause.
- A panel, query, `query` variable or `adhoc` variable `datasource` may be `${name}`, naming a `datasource` variable declared earlier in `variables`; its queries then run on the selected datasource, and that variable's `default` is always a uid.
- An `interval` variable with `"auto": true` and no `default` starts on auto; set `default` to one of its `options` to start on a fixed step.
- A variable's `default` is one value, a list for a multi variable, or `$__all`.
- Global values for any query: `$__interval_ms`, `$__range_s`, `$__range_ms`, `${__from:date:seconds}`, `${__to:date:seconds}`, `$__dashboard` and `${__user.login}`.

## Time
- Honor a requested window: "last 6h" is `"time": {"from": "now-6h", "to": "now"}`; otherwise use `now-1h`.
- Leave out `cursorSync` unless asked to link hover across charts: `crosshair` marks the hovered time on every time chart, `tooltip` also opens their tooltips.

## Saving
- Every query runs during the save; every data panel except an `alertlist` must return data for the default time range and variable values, or the save is rejected with the reason; fix the named panels and save again.
- Never put HTML or scripts in any field; all text is plain.
