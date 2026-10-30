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

| Field         | Meaning                                                                                                       |
| ------------- | ------------------------------------------------------------------------------------------------------------- |
| `title`       | 1 to 200 characters.                                                                                          |
| `description` | Up to 2000 characters.                                                                                        |
| `tags`        | Up to 20 lowercase tags of 1 to 40 characters, each starting with a letter or digit.                          |
| `time`        | The default range, `from` and `to`. A value is relative, such as `now-6h` or `now-1d/d`, or an RFC 3339 time. |
| `refresh`     | An auto-refresh interval such as `30s`, `1m` or `1h`.                                                         |
| `timezone`    | `browser`, `utc` or an IANA zone such as `Asia/Kolkata`.                                                      |
| `cursorSync`  | How hovering one time chart shows on the others: `off`, `crosshair` or `tooltip`.                             |
| `variables`   | Up to 20 [variables](datasources-and-variables.md#variables).                                                 |
| `links`       | Up to 20 links, each with a `title`, a `url` and optionally `newTab`.                                         |
| `panels`      | 1 to 200 panels, counting the children of rows.                                                               |

## Panels and layout

Every panel has an `id`, a `type` and a `gridPos`.
An `id` starts with a letter, continues with letters, digits, `_` or `-`, is at most 64 characters, and is unique across the whole dashboard, row children included.

`gridPos` places a panel on a grid 24 columns wide with `x`, `y`, `w` and `h`.
`x + w` cannot exceed 24, and panels cannot overlap.

A data panel also has a `title`, a `datasource` and `queries`.
Its `type` is one of `timeseries`, `stat`, `gauge`, `bargauge`, `table`, `piechart`, `heatmap`, `histogram`, `state-timeline`, `logs`, `barchart`, `status-history`, `trend`, `xychart`, `candlestick`, `nodeGraph`, `traces`, `flamegraph`, `geomap`, `canvas` or `alertlist`.
The other panel types are:

- `text`, which holds Markdown in `content`, rendered sanitized.
  Its optional `mode` is `markdown` (the default), `html` (sanitized HTML) or `code` (the content shown as code, with `code.language` and `code.lineNumbers`).
  People and Grafana imports can write `html`; the agent and MCP clients cannot (see [Maps, canvases and actions](#maps-canvases-and-actions)).
- `dashlist` and `news`, which list other dashboards and the items of an RSS or Atom feed.
  They have no datasource and no queries.
- `row`, described below.
- `placeholder`, which only the importer writes.
  The agent may move or delete one but never creates or edits one.

A data panel can also set `timeFrom` and `timeShift` to override the dashboard's time range, `interval` (the same forms as a query's) and `maxDataPoints` to shape the step of its queries, and `display`, a small set of options that depend on its type, such as the legend of a `timeseries` or the `header` of a `table`.

## Maps, canvases and actions

These options are all optional, and a dashboard that leaves them out looks the same as before.
The web app draws them as support lands; an option it does not draw yet is kept and ignored.

A `geomap` panel can load map tiles and draw several layers.
Its `display` takes:

- `view`: `fit`, `world`, `coords` or a region (`north-america`, `south-america`, `europe`, `africa`, `west-asia`, `south-asia`, `south-east-asia`, `east-asia`, `australia` or `oceania`).
- `basemap`: `outline` (the default), `carto`, `esri`, `osm`, `xyz` or `none`, with `basemapTheme`, `basemapLabels`, `esriServer`, `basemapUrl` for `xyz`, `basemapAttribution` and `basemapOpacity`.
- `minZoom`, `maxZoom` and `zoom`: `minZoom` cannot exceed `maxZoom`, and when all three are set `zoom` lies between them.
- Fit and controls: `fitLayer`, `fitLastOnly`, `fitPadding`, `sharedView`, `showZoom`, `mouseWheelZoom`, `showAttribution`, `showScale`, `showMeasure`, `showDebug` and `tooltip` (`details` or `none`).
- `layers`: up to 10 layers, each with a `type` (`markers`, `heatmap`, `geojson`, `dayNight`, `route`, `photos`, `network`, `carto`, `esri`, `osm` or `xyz`) and the options of that type, such as `query`, `location`, `latField`, `lonField`, `rules` and `opacity`.
  When `layers` is set, the list is the whole layer set, so the flat marker options (`location`, `latField`, `lonField`, `geohashField`, `sizeField`, `colorField`, `markerSize`, `markerSizeMax`) are refused with `geomap_layers_conflict`.

A `canvas` panel takes a background (`backgroundColor`, `backgroundImage`, `backgroundSize`), a border (`borderColor`, `borderWidth`, `borderRadius`), `panZoom`, `infinitePan` and `tooltip`.
An image is `{ "url": "https://..." }` or `{ "field": "<field>" }`.
A canvas element can have the kinds `icon`, `button`, `server`, `triangle`, `cloud`, `parallelogram`, `windTurbine`, `droneTop`, `droneFront` and `droneSide` next to `metric`, `text`, `rectangle` and `ellipse`, and options such as `placement`, `rotation`, `valign`, `image`, `links` and `actions`.
`placement` wins over `left`, `top`, `width` and `height` when present: numbers are pixels of the panel body, or percent on an axis whose mode is `scale`.
A connection can set `path`, `vertices`, `lineStyle`, `arrow`, `widthField`, `minWidth` and `maxWidth`.

An element's `actions` are HTTP requests that a person triggers from the panel:

```json
{ "title": "Deploy", "method": "POST", "url": "https://deploy.example.com/api/deploy", "body": "{\"env\": \"${env}\"}", "confirm": "Deploy to ${env}?" }
```

- A `url` is an `https` address with the host written out; `${name}` references may appear only after the host.
- `headers` and `query` take `name` and `value` pairs.
  Credentials and transport headers (such as `Authorization`, `Cookie` or a name containing `token`, `secret` or `password`) are refused with `action_header_not_allowed`, because the server adds what it needs.
- The `url`, `body` and header and query values may reference declared variables, `__from`, `__to`, `__user.login`, `__user.email`, `__user.name`, `__user.id`, `__value.raw`, `__value.text`, `__value.numeric` and `__data.fields.<field>`.
  Any other name is refused with `unknown_variable`.
- `oneClick` on an element (`action` or `link`) needs at least one action or link.

People and imports write HTML text panels and canvas actions.
The agent and MCP clients cannot add or change them, and cannot write HTML tags in any text panel: a save that does is refused with `html_not_allowed` or `action_not_allowed`.
They may still move or delete such a panel or element.

## Queries and datasources

A data panel has 1 to 26 `queries`.
A query has the text in `query` and, optionally:

- `ref`, its name for overrides, which defaults to `A`, `B`, `C` by position.
- `legend`, a template such as `{{pod}}`.
- `datasource`, to run this query on a datasource other than the panel's.
- `instant`, to ask for one value per series instead of a range.
- `format`, one of `timeseries`, `table`, `heatmap` or `logs`.
- `interval`, the smallest step: a duration such as `30s`, or `${name}` naming an `interval` variable, which then gives the step the viewer selected, `$__auto` included.
- `intervalFactor`, a whole number from 2 to 10 that multiplies the query's step, as Grafana's resolution 1/N does.
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
