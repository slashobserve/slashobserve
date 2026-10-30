---
title: ObserveSpec
description: The JSON document SlashObserve stores for each dashboard version, and what it contains.
---

ObserveSpec is the JSON format of a SlashObserve dashboard.
Every saved version of a dashboard is one ObserveSpec document, and the web app, the agent, MCP clients, the API and the Grafana importer all read and write the same one.
The current format is version `0`, and each document says so in `specVersion`.

## Why a spec

The server checks a document against one closed JSON Schema (unknown fields are refused) and against a set of rules the schema cannot express before it stores anything.
Because every writer produces the same document, there is one validator and one stored shape, whether a person, the agent or an MCP client wrote the dashboard.

Three choices in the format follow from how dashboards are used here:

- Datasources are named by the `uid` you give them in the [datasources file](../reference/datasources-file.md).
  A document holds no URL, address or credential for a datasource.
- Every panel has a string `id` that stays the same across versions.
  [Comments](versions.md#comments) anchor to it, and the diff between two versions names the panels that changed.
- The agent and MCP clients write a whole document in one call.
  The agent reads the JSON Schema and an authoring guide first, and an MCP client gets both from `get_spec_guide`.
  The agent cannot revise a dashboard whose compact JSON is over 128 KiB, and the server refuses any document over 1 MiB.

A Grafana dashboard is not stored as is.
The importer converts it into this format and keeps a `placeholder` panel for each panel it cannot convert yet (see [Import Grafana dashboards](../use/import-grafana.md)).
**Export spec JSON** in the dashboard's **More** menu downloads the document of the version you are viewing.

## An example

This is `spec/examples/checkout-service-health.json` from the repository.
It has one variable, two Prometheus panels and a ClickHouse table:

```json
{
  "specVersion": "0",
  "title": "Checkout service health",
  "description": "RED metrics and error logs for checkout",
  "tags": ["checkout", "red"],
  "time": { "from": "now-6h", "to": "now" },
  "refresh": "30s",
  "variables": [
    { "name": "pod", "type": "query", "datasource": "prom", "multi": true, "includeAll": true,
      "query": "label_values(http_requests_total{service=\"checkout\"}, pod)" }
  ],
  "panels": [
    { "id": "p_rps", "type": "timeseries", "title": "Requests per second by pod",
      "gridPos": { "x": 0, "y": 0, "w": 12, "h": 8 }, "datasource": "prom", "unit": "reqps",
      "queries": [{ "query": "sum by (pod) (rate(http_requests_total{service=\"checkout\",pod=~\"$pod\"}[$__rate_interval]))", "legend": "{{pod}}" }] },
    { "id": "p_p99", "type": "timeseries", "title": "p99 latency",
      "gridPos": { "x": 12, "y": 0, "w": 12, "h": 8 }, "datasource": "prom", "unit": "s",
      "queries": [{ "query": "histogram_quantile(0.99, sum by (le) (rate(http_request_duration_seconds_bucket{service=\"checkout\"}[$__rate_interval])))" }],
      "thresholds": [{ "value": 0.25, "color": "warning" }, { "value": 0.5, "color": "critical" }] },
    { "id": "p_errlogs", "type": "table", "title": "Error logs by message (ClickHouse)",
      "gridPos": { "x": 0, "y": 8, "w": 24, "h": 8 }, "datasource": "ch",
      "queries": [{ "query": "SELECT Body AS message, count() AS n FROM otel_logs WHERE ServiceName = 'checkout' AND SeverityText = 'ERROR' AND $__timeFilter(Timestamp) GROUP BY message ORDER BY n DESC LIMIT 20" }] }
  ]
}
```

## The dashboard

`specVersion`, `title`, `time` and `panels` are required.
The rest is optional, and an absent field means its default.

| Field                  | Meaning                                                                                                                 |
| ---------------------- | ----------------------------------------------------------------------------------------------------------------------- |
| `title`                | 1 to 200 characters.                                                                                                    |
| `description`          | Up to 2000 characters.                                                                                                  |
| `tags`                 | Up to 20 lowercase tags of 1 to 40 characters, each starting with a letter or digit.                                    |
| `time`                 | The default range, `from` and `to`. A value is relative, such as `now-6h`, `now-1d/d` or `now/fy`, or an RFC 3339 time. |
| `refresh`              | An auto-refresh interval such as `30s`, `1m` or `1h`.                                                                   |
| `timezone`             | `browser`, `utc` or an IANA zone such as `Asia/Kolkata`. Relative times resolve in this zone.                           |
| `weekStart`            | The first day of the week for `now/w`: `monday` (the default), `sunday` or `saturday`.                                  |
| `fiscalYearStartMonth` | The month the fiscal year starts in for `now/fy` and `now/fQ`, from 0 (January, the default) to 11 (December).          |
| `nowDelay`             | How far behind the real time `now` is for every relative time, such as `30s` or `1m`.                                   |
| `cursorSync`           | How hovering one time chart shows on the others: `off`, `crosshair` or `tooltip`.                                       |
| `variables`            | Up to 20 [variables](datasources-and-variables.md#variables).                                                           |
| `links`                | Up to 20 links, each with a `title`, a `url` and optionally `newTab`.                                                   |
| `panels`               | 1 to 200 panels, counting the children of rows.                                                                         |

### Relative times

A relative time is `now`, optionally moved by an offset such as `-6h` or `+1d`, and optionally rounded with a slash and a unit, such as `/d`.
The units are `s`, `m`, `h`, `d`, `w`, `M` and `y`; rounding also takes `fy` (the start of the fiscal year) and `fQ` (the start of the fiscal quarter).
Rounding a `from` goes to the start of the period and rounding a `to` goes to its last millisecond, so `now/d` to `now/d` is the whole of today.

Days, weeks, months, years and every rounding are calendar steps in the dashboard's `timezone`, so `now/d` starts at midnight in that zone and a day that changes the clock lasts 23 or 25 hours.
Seconds, minutes and hours are exact durations.
With `browser`, the web app sends the browser's zone to the server; without a zone, and with `utc`, times resolve in UTC.
`nowDelay` moves `now` back before anything else, for both ends of the range.

Relative times used to resolve in UTC whatever the `timezone` said, so a dashboard whose zone is not UTC now shows different ranges for `now/d`, `now/w`, `now/M`, `now/y` and day, week, month and year offsets than it did before.

## Panels and layout

Every panel has an `id`, a `type` and a `gridPos`.
An `id` starts with a letter, continues with letters, digits, `_` or `-`, is at most 64 characters, and is unique across the whole dashboard, row children included.

`gridPos` places a panel on a grid 24 columns wide with `x`, `y`, `w` and `h`.
`x + w` cannot exceed 24, and panels cannot overlap.

A data panel also has a `title`, a `datasource` and `queries`.
Its `type` is one of `timeseries`, `stat`, `gauge`, `bargauge`, `table`, `piechart`, `heatmap`, `histogram`, `state-timeline`, `logs`, `barchart`, `status-history`, `trend`, `xychart`, `candlestick`, `nodeGraph`, `traces`, `flamegraph`, `geomap`, `canvas` or `alertlist`.
The other panel types are:

- `text`, which holds Markdown in `content`, rendered sanitized.
- `dashlist` and `news`, which list other dashboards and the items of an RSS or Atom feed.
  They have no datasource and no queries.
- `row`, described below.
- `placeholder`, which only the importer writes.
  The agent may move or delete one but never creates or edits one.

A data panel can also set `timeFrom` and `timeShift` to override the dashboard's time range, `interval` and `maxDataPoints` to shape the step of its queries, and `display`, a small set of options that depend on its type, such as the legend of a `timeseries` or the `header` of a `table`.

## Queries and datasources

A data panel has 1 to 26 `queries`.
A query has the text in `query` and, optionally:

- `ref`, its name for overrides, which defaults to `A`, `B`, `C` by position.
- `legend`, a template such as `{{pod}}`.
- `datasource`, to run this query on a datasource other than the panel's.
- `instant`, to ask for one value per series instead of a range.
- `format`, one of `timeseries`, `table`, `heatmap` or `logs`.
- `interval`, the smallest step.
- `hide`, to keep the query out of the panel's data.

The query language follows the datasource: PromQL for Prometheus and a single SQL `SELECT` for ClickHouse.
`$name`, `${name}` and `${name:format}` insert the value of a variable.
Macros such as `$__rate_interval` for Prometheus and `$__timeFilter(Timestamp)` for ClickHouse expand when the query runs, from the time range and step of the request.
A datasource is a uid from the datasources file or `${name}`, naming a datasource variable.
An `adhoc` variable filters Prometheus queries automatically but does not filter ClickHouse queries, which must include `$name` themselves.
See [Datasources and variables](datasources-and-variables.md).

## Field options, overrides and transforms

A data panel carries its display options directly, and an override repeats the same options for the series or fields it matches.
The options are `unit`, `decimals`, `min`, `max`, `thresholds` and `thresholdMode`, `color`, `displayName`, `noValue`, `mappings`, `links`, and for charts `style`, `fill`, `lineStyle`, `stack`, `axis` and `hidden`.
Thresholds ascend, and the first step may leave out `value` to act as the base color.
A color is `ok`, `info`, `warning`, `critical` or a `#rrggbb` hex value.
A unit is a Grafana unit id such as `reqps`, `s`, `bytes` or `percentunit`, or a custom `suffix:` or `prefix:` unit.

An override has a `match`, which selects by `query` (a `ref`), `series`, `seriesRegex`, `field` or `fieldType`, and at least one option to apply.
A panel has at most 50 overrides.

`transform` is an ordered list of up to 20 steps that reshape the result in the browser after the query runs.
The steps are `merge`, `seriesToRows`, `organize`, `reduce`, `joinByField`, `filterFieldsByName`, `calculateField`, `sortBy` and `limit`.
Prefer a query that returns the shape you need, and use a transform when it cannot.

## Rows and repeats

A `row` panel has a `title`, a `gridPos`, a `collapsed` flag and its child panels in `panels`.
The children are always nested in the row, whether it is collapsed or not, and each keeps its own `gridPos`.

`repeat` on a data panel or a row names a variable and draws one copy for each selected value, with All meaning every value of the variable.
On a data panel, `repeatDirection` is `h` or `v` and `maxPerRow` (default 4) sets how many copies share a row.
The web app draws at most 50 copies and adds a note when it leaves values out.
The variable has to be declared.

## Validation

A save checks a document in layers.
The schema rejects unknown or malformed fields.
Further rules reject what a schema cannot express, such as duplicate panel ids or variable names, overlapping panels, thresholds that do not ascend, and references to undeclared variables.
The server then checks that every datasource uid is configured and runs the queries (see [How the agent builds a dashboard](agent.md)).

A document that fails a check is refused with `422` and the code `invalid_spec`.
The body lists every problem in `issues`, each with a path into the document, a code and a message.

## Where the schema is

The schema is `spec/schema/observespec.v0.json` in the repository.
Its `$id` is `https://slashobserve.com/schemas/observespec.v0.json`, and the documentation site publishes it at that address, so editors and validators can fetch it there.
The `get_spec_guide` MCP tool returns the authoring guide, the schema and an example for a model that has not seen them (see [MCP](../use/mcp.md)).

A document cannot carry a `$schema` field, because the schema refuses unknown fields.
