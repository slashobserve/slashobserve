# ObserveSpec authoring guide

A dashboard is one ObserveSpec v0 JSON document; the JSON Schema, which `get_spec_guide` returns, is authoritative; the example shows the format only.
Discover metric, label, table and column names with the tools; never copy them from the example.
Leave out every value that equals its default.
The schema leaves out rare, import-only and purely visual options; when a Spec you revise holds one, keep it unchanged.

## Panels
- A data panel has `id`, `type`, `title`, `gridPos`, `datasource` (a datasource uid such as `prom`) and `queries`.
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
- Add a `text` panel only when the request asks for notes, as Markdown in `content`.

## Panel type by data shape
Decide what the data is before choosing a type: one value, a value against a limit, a trend, a distribution, a ranking, a share, rows of attributes, states, logs or events.
- One current value, such as a count, a rate now or a total: `stat` with an `instant: true` query, or `reduce` `lastNotNull` on a range query; thresholds only when the value has a good and a bad direction, then `colorMode` `value`.
- A state now, such as `up`: `stat` with `mappings` naming each state (0 Down, 1 Up) and no unit; a version, owner or node label: `stat` with `textMode` `name`, or a `table` for several labels; an age: `stat` on `time() - <timestamp>` in `s` or `dtdurations`.
- One ratio against a known maximum, such as used over capacity: `gauge` with `min` 0, `max` 1 and `percentunit` (or `max` 100 and `percent`), thresholds `ok`, `warning` near 0.8, `critical` near 0.9; a value with no natural maximum is a `stat`.
- A trend of up to about 20 series, such as rates, gauges, ratios or latency quantiles (p50, p90 and p99 together, in `s` or `ms`): `timeseries`, as wide as its line's recipe whatever its series count; more: aggregate with `sum by` or `topk` first.
- A distribution over time: `heatmap` of `sum by (le) (rate(<name>_bucket[$__rate_interval]))` with `format: "heatmap"`, the bucket bound's unit and `yScale` log for wide ranges; buckets become a heatmap or quantile lines, never one line per bucket; raw values over the whole range: `histogram`.
- A ranking or top N now: `bargauge` on an instant `topk(N, ...)` or `ORDER BY ... DESC LIMIT N`, `orientation` horizontal for long names; a `table` when entries need several columns; never a `timeseries` with a line per entry.
- Values across categories: `barchart` with an `instant: true` query and `format: "table"` (the first label column is the x axis) or `display.xField`, legend hidden for one series; shares of a whole in up to six parts: `piechart` with `legend` `right`.
- Several attributes per entity, inventories or slowest-N lists: `table` with `format: "table"`, a unit per column in `overrides` and `LIMIT` in SQL.
- Discrete states per entity over time, such as `up` or `ALERTS`: `state-timeline` with mappings or thresholds per state and no unit, or `status-history` (one cell per sample) for periodic checks.
- Show a counter as `rate()` or `increase()`; its raw value only in a stat titled as a lifetime total, or inside `count(...)`.

## Queries
- Each query has `query` and, if it returns several series, a `legend` such as `{{route}}`.
- Put related series in one panel with several queries, such as requests and errors; queries are named `A`, `B`, `C` by position; write `ref` only for a stable name.
- For a request naming two sources, plan a panel per source on its own datasource.
- A query may set its own `datasource`, `instant: true` for one value per series, `interval` as its minimum step (a duration, or `${name}` of an `interval` variable), `exemplar: true` to mark exemplars on a Prometheus time series, or `hide: true` to run nowhere.

## Layout
- The grid is 24 columns wide and a screen about 22 units tall; `x + w` may not exceed 24 and panels may not overlap.
- Default sizes, `w`x`h`: stat and gauge 6x4; timeseries, bargauge, barchart, heatmap and histogram 12x8; piechart 8x8; table 24x8; state-timeline and status-history 24x6; logs, traces, nodeGraph and geomap 24x10; text 24x2; others 12x8; a row is 24x1.
- A line is the panels side by side at one `y`: plan it before its first panel, make its widths add up to exactly 24, and give its panels one height; a stat or gauge beside a chart takes the chart's height, and short panels otherwise get their own line.
- Line recipes: 12/12, 8/8/8, 24, 6/6/6/6, 4/4/4/4/4/4, 16/8, 8/16, 18/6, 6/6/12, 4/4/4/12 or 15/9; equal widths unless one panel is the focus; vary the recipe by purpose: single values in a strip, related charts 12/12, a group of three such as requests, errors and duration 8/8/8.
- Full width only for a table of five or more columns (or unknown), logs, traces, a service map, state timelines, a note, the last panel of an odd line, and one headline chart per row at `h` 7; a table of up to four columns goes 12 or 8 wide beside a chart of its height; never end a row on a lone half or third.
- Open with one strip of three, four or six single values of one width (8, 6 or 4) at `h` 3 or 4, two strips at most, and keep single values out of later rows unless a row has its own strip.
- Order from summary to detail: an optional 24x2 note, the strip, the key charts, one row per subsystem or resource, then drill-down rows; title the first row `Overview` and later rows with a short noun.
- Up to 8 panels need no rows or one or two; 9 to 16 may use rows; above 16, rows of two to six panels; never collapse the first row, and collapse detail rows only from 30 panels.
- Keep five to nine panels in the first 22 units and the total height near 50 units unless the request rebuilds a whole dashboard.

## Field options
- Set `unit` on every timeseries, stat, gauge and bargauge; it is a unit id from the schema: `reqps` for request rates, `s` or `ms` for durations, `percentunit` for 0-1 ratios, `percent` for 0-100, `bytes`, `short` for counts, or `suffix:<text>`.
- `thresholds` ascend; the first step may leave out `value` and is then the base color, for example `[{"color": "ok"}, {"value": 0.25, "color": "warning"}, {"value": 0.5, "color": "critical"}]`; colors are `ok`, `info`, `warning`, `critical`, `text` (the theme's text color) or a hex color.
- `decimals`, `min`, `max`, `displayName`, `noValue` and `mappings` (each needs `text`, `color` or both) refine values.
- `overrides` set the same options for matching series, for example `{"match": {"query": "B"}, "unit": "s"}`.
- `display` holds the per-type options; set one only when the request or a rule in this guide asks for it, after reading its type's options with `get_spec_guide`.
- Legends: a list below the chart, naming each series with its query's `legend`; above five series a table (`display.legendMode` `table`, `legendValues` `["lastNotNull", "max"]`), below the chart up to 12 wide and on the right (`display.legend` `right`) only at full width; `display.legend` `hidden` for one unnamed series.
- A stat draws its sparkline from `h` 4; at `h` 2 or 3 set `display.graph` false.
- Give every stat and gauge a one-sentence `description`: a lone number has no axis or legend to explain it.
- `transform` reshapes results in the browser; use it only when a query cannot return the shape.

## Variables
- Add a variable only when the request asks for a breakdown the user will filter.
- `query` variables: Prometheus `label_values(<metric>, <label>)`, referenced as `$name`, or `${name:regex}` inside `=~`; ClickHouse references use `${name:sqlstring}` inside `IN (...)`.
- `custom` lists fixed values; `interval` lists steps in `options`, such as `["1m", "5m", "1h"]`, with `"auto": true`; `constant` holds one shared value; `textbox` takes free text; `adhoc` adds key and value filters, which Prometheus applies to every query on its datasource and a ClickHouse query uses as `AND $name` in its WHERE clause.
- `groupby` variables (Prometheus only) let viewers pick labels to group by, with saved labels in `default`; queries need no change.
- A panel, query, `query` variable, `adhoc` variable or `groupby` variable `datasource` may be `${name}`, naming a `datasource` variable declared earlier in `variables`; its queries then run on the selected datasource, and that variable's `default` is always a uid.
- An `interval` variable with `"auto": true` and no `default` starts on auto; set `default` to one of its `options` to start on a fixed step.
- A variable's `default` is one value, a list for a multi variable, or `$__all`.
- Global values for any query: `$__interval_ms`, `$__range_s`, `$__range_ms`, `${__from:date:seconds}`, `${__to:date:seconds}`, `$__dashboard` and `${__user.login}`.

## Time
- Honor a requested window: "last 6h" is `"time": {"from": "now-6h", "to": "now"}`; otherwise use `now-1h`.
- Leave out `cursorSync` unless asked to link hover across charts: `crosshair` marks the hovered time on every time chart, `tooltip` also opens their tooltips.

## Links
- Add `links` only when asked: `{"title": "Runbook", "url": "https://..."}` opens an address, and `{"type": "dashboards", "title": "Related", "tags": ["checkout"]}` lists the other dashboards that carry every one of the tags.

## Saving
- Every query runs during the save; every data panel except an `alertlist` must return data for the default time range and variable values, or the save is rejected with the reason; fix the named panels and save again.
- Never put HTML or scripts in any field.

## Loki (LogQL)
- Answer a question about log lines, messages, errors or events from Loki, or from `otel_logs` when the request names ClickHouse.
- `list_labels` lists stream labels and `list_label_values` their values, both narrowed by a stream selector in `metric`; `sample_rows` reads recent lines, of every stream or of a stream selector in `table`.
- A log query starts with a stream selector and returns lines: `{service_name="payments"} |= "timeout"`, or `{service_name="payments"} | severity_text="ERROR"`; show it in a full-width `logs` panel.
- A metric query wraps a selector in a range function, like PromQL: `sum by (service_name) (count_over_time({service_name=~".+"}[$__interval]))` with `legend` `{{service_name}}` and `unit` `short`, or `rate()` for lines per second.
- The window is `$__interval` or `$__range`; Loki has no `$__rate_interval` and no ClickHouse macros.
- A count, share or quantile of lines over a window: `measure_aggregate` with the log query (`total`, `ratio`, or `unwrap` a field for `quantile`, `max` or `avg`).
- `query` variables: `label_values(service_name)` or `label_values({namespace="shop"}, service_name)`.
- `list_metrics` with a stream selector as `scope` counts the streams that carry each label key.

## Tempo (TraceQL)
- A request about single requests or spans, slow or failing calls, or one trace is a traces question: answer it from Tempo, or from `otel_traces` when the request names ClickHouse; call `get_schema` on Tempo instead of `list_metrics` with a `scope`.
- `get_schema` lists the services, span names and attributes TraceQL filters on, such as `resource.service.name`, `span.http.route`, `name` and `status`; `list_label_values` lists one attribute's values.
- A TraceQL query such as `{ resource.service.name = "checkout" && duration > 500ms }` returns one row per trace with `TraceId`, `SpanId` (the root span), `SpanName`, `ServiceName`, `StartTime` and `Duration` in nanoseconds; show it as a `table` with the override `{"match": {"field": "Duration"}, "unit": "ns"}`.
- A trace id, 32 hex characters or `${trace_id}` of a `textbox` variable, returns every span of that trace with `ParentSpanId`, `StatusCode` and attributes; show it in a `traces` panel.
- A Tempo panel lists traces, not series: chart counts or latency over time from `otel_traces` or Prometheus.
- Over a window, read `trace_stats` with `group_by`, `trace_error_origins` and `trace_slow_path`.

## The audit trail (`slashobserve-audit`)
- `slashobserve-audit` is this server's own audit trail, served as an OpenSearch datasource that only Admins can read: one document per sign-in, read, change, agent tool call and refusal.
- The time field is `occurred_at`, not `@timestamp`, and the line is `message`, such as `dashboard.update denied (forbidden_role) by Sam via web on dashboard "Checkout"`; there is no `log.level`, so filter on `outcome` (`success`, `denied`, `failed`, `unknown`).
- Fields are flat and lowercase: `action`, `category`, `outcome`, `reason`, `actor_name`, `actor_kind`, `initiator`, `agent_name`, `origin`, `origin_client`, `tool`, `client_ip`, `auth_method`, `target_kind`, `target_name`, `target_id` (a query's datasource uid), `status`, `duration_ms`; `list_labels` lists them all. Group with `terms` on these names directly, with no `.keyword`.
- KQL reads `outcome:denied`, `action:auth.login_failed`, `action:dashboard.*` and `initiator:agent and action:agent.tool_call`; a range KQL query counts events per interval, so a `stat` of a count takes `display.reduce` `sum`.
- A DSL histogram uses `occurred_at`: `{"aggs":{"over_time":{"date_histogram":{"field":"occurred_at","fixed_interval":"$__interval"},"aggs":{"by_outcome":{"terms":{"field":"outcome","size":5}}}}}}` with `legend` `{{outcome}}`.
- It answers searches sorted by `occurred_at`, `date_histogram` and `terms` buckets, and the `cardinality`, `value_count`, `min`, `max`, `avg`, `sum` and `percentiles` metrics; anything else is refused with a message that names it, and one read returns at most 1,000 lines.
- A run reads it only when everyone who reads what the run writes is an Admin: a private chat, or a thread, dashboard, observation or build in a folder only Admins can read. Anywhere else it is not listed and a query on it answers `admins_only_datasource`; then say in the answer that the audit data can be read in a private chat or in the Audit folder, and never guess it.
- Everything that uses it must live in a folder only Admins can read, such as the Audit folder, which holds the starter dashboards Audit overview, Sign-ins and access, Agent and MCP activity and Data access; a save or a move anywhere else is refused.
- Each query on it is recorded in the trail as `query.run`, so read what the question needs and no more.
