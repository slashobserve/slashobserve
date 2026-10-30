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
    {
      "name": "pod",
      "type": "query",
      "datasource": "prom",
      "multi": true,
      "includeAll": true,
      "query": "label_values(http_requests_total{service=\"checkout\"}, pod)"
    }
  ],
  "panels": [
    {
      "id": "p_rps",
      "type": "timeseries",
      "title": "Requests per second by pod",
      "gridPos": { "x": 0, "y": 0, "w": 12, "h": 8 },
      "datasource": "prom",
      "unit": "reqps",
      "queries": [
        {
          "query": "sum by (pod) (rate(http_requests_total{service=\"checkout\",pod=~\"$pod\"}[$__rate_interval]))",
          "legend": "{{pod}}"
        }
      ]
    },
    {
      "id": "p_p99",
      "type": "timeseries",
      "title": "p99 latency",
      "gridPos": { "x": 12, "y": 0, "w": 12, "h": 8 },
      "datasource": "prom",
      "unit": "s",
      "queries": [
        {
          "query": "histogram_quantile(0.99, sum by (le) (rate(http_request_duration_seconds_bucket{service=\"checkout\"}[$__rate_interval])))"
        }
      ],
      "thresholds": [
        { "value": 0.25, "color": "warning" },
        { "value": 0.5, "color": "critical" }
      ]
    },
    {
      "id": "p_errlogs",
      "type": "table",
      "title": "Error logs by message (ClickHouse)",
      "gridPos": { "x": 0, "y": 8, "w": 24, "h": 8 },
      "datasource": "ch",
      "queries": [
        {
          "query": "SELECT Body AS message, count() AS n FROM otel_logs WHERE ServiceName = 'checkout' AND SeverityText = 'ERROR' AND $__timeFilter(Timestamp) GROUP BY message ORDER BY n DESC LIMIT 20"
        }
      ]
    }
  ]
}
```

## The dashboard

`specVersion`, `title`, `time` and `panels` are required.
The rest is optional, and an absent field means its default.

| Field                  | Meaning                                                                                                                                                                                                             |
| ---------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `title`                | 1 to 200 characters.                                                                                                                                                                                                |
| `description`          | Up to 2000 characters.                                                                                                                                                                                              |
| `tags`                 | Up to 20 lowercase tags of 1 to 40 characters, each starting with a letter or digit.                                                                                                                                |
| `time`                 | The default range, `from` and `to`. A value is relative, such as `now-6h`, `now-1d/d` or `now/fy`, or an RFC 3339 time.                                                                                             |
| `refresh`              | An auto-refresh interval such as `30s`, `1m`, `1h` or `1d`.                                                                                                                                                         |
| `timePicker`           | What the time picker offers: `hidden` removes it, `refreshIntervals` (1 to 20) replaces the auto-refresh choices and `quickRanges` (1 to 30 of `display`, `from` and `to`) the preset ranges.                       |
| `liveNow`              | `true` refreshes the dashboard every 5 seconds until someone picks another interval.                                                                                                                                |
| `timezone`             | `browser`, `utc` or an IANA zone such as `Asia/Kolkata`. Relative times resolve in this zone.                                                                                                                       |
| `weekStart`            | The first day of the week for `now/w`: `monday` (the default), `sunday` or `saturday`.                                                                                                                              |
| `fiscalYearStartMonth` | The month the fiscal year starts in for `now/fy` and `now/fQ`, from 0 (January, the default) to 11 (December).                                                                                                      |
| `nowDelay`             | How far behind the real time `now` is for every relative time, such as `30s` or `1m`.                                                                                                                               |
| `cursorSync`           | How hovering one time chart shows on the others: `off`, `crosshair` or `tooltip`.                                                                                                                                   |
| `variables`            | Up to 20 [variables](datasources-and-variables.md#variables).                                                                                                                                                       |
| `links`                | Up to 20 links: a `title` with a `url` (and optionally `newTab`, `oneClick`, `keepTime`, `includeVars`), or `"type": "dashboards"` with `tags` to list the other dashboards that carry every tag (`asDropdown` shows one menu). |
| `panels`               | 1 to 200 panels, counting the children of rows.                                                                                                                                                                     |

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
  An image in the Markdown loads through the server, and only from an https address on an allowed domain (see [Outbound access](../operate/outbound-access.md#images-and-geojson)); otherwise the panel shows its alt text.
  Its optional `mode` is `markdown` (the default), `html` (sanitized HTML) or `code` (the content shown as code, with `code.language` and `code.lineNumbers`).
  People and Grafana imports can write `html`; the agent and MCP clients cannot (see [Maps, canvases and actions](#maps-canvases-and-actions)).
  In `markdown` and `html` modes HTML is cleaned before it is drawn: scripts, styles, forms, embedded frames, SVG and event handlers are removed, links keep only http, https and mailto addresses, and every image loads through the server.
  Inline `style` attributes keep colours, fonts, spacing, borders, sizes and simple layout, and drop positioning, backgrounds with images, `var()` and anything that loads a resource.
  Whatever the HTML draws stays inside its panel.
  Variable values are inserted as text in all three modes, never as HTML.
- `dashlist` and `news`, which list other dashboards and the items of an RSS or Atom feed.
  They have no datasource and no queries.
- `row`, described below.
- `placeholder`, which only the importer writes.
  The agent may move or delete one but never creates or edits one.

A data panel can also set `timeFrom` and `timeShift` to override the dashboard's time range, `interval` (the same forms as a query's) and `maxDataPoints` to shape the step of its queries, and `display`, a small set of options that depend on its type, such as the legend of a `timeseries` or the `header` of a `table`.
The legend of a `timeseries` or `trend` panel is a `list` or a `table` (`legendMode`), may show reducer `legendValues`, sorts by a value or by name (`legendSortBy`, `legendSortDesc`), takes a width when it sits on the right (`legendWidth`), and hides all-zero or empty series (`legendHideZero`, `legendHideEmpty`).
A `timeseries`, `trend` or `candlestick` tooltip that lists all series is sorted by `tooltipSort` (`none` for series order, `asc` or `desc`, which is the default), can drop zero values with `tooltipHideZeros`, can show the running total of a stacked series with `tooltipCumulative`, and is clipped to `tooltipMaxWidth` and `tooltipMaxHeight` pixels (100 to 2000).
A `candlestick` panel draws only its open, high, low, close and volume fields unless `includeAllFields` is `true`, which draws its other number fields as lines.
A `timeseries`, `trend` or `candlestick` panel can hide its x axis with `xAxis: "hidden"` (`shown` by default).
When `legendMode` is absent the legend is a table if `legendValues` is not empty and a list otherwise.
A `table` panel's `display` also takes `cellHeight` (`sm`, the default, `md` or `lg` row padding), `typeIcons` (an icon for the column type before each header), `pagination` (pages of rows sized to the panel, with previous and next buttons), `frozenColumns` (0 to 10 columns from the left that stay in view while the table scrolls sideways, which also fixes the column widths), `sortBy` (a column name, as written in the header) with `sortDesc`, which set the sort the table starts with, `footerFields` (column names; the footer then covers only those columns) and `footerCountRows` (a footer row with the number of rows).
The footer and the row count cover the rows left after filtering.
The agent prompt does not list these options.
A `heatmap` panel colors its cells with `colorMode`: `opacity` (the default) varies the opacity of `colorFill` (a semantic color; the first series color when absent), and `scheme` takes the color of the ramp `colorScheme` names (the same names as the field prop, `blues` when absent).
`colorSteps` (2 to 128) quantizes the colors into that many steps and is continuous when absent, `colorMin` and `colorMax` fix the counts that get the first and last color (the data otherwise; a scheme starts at the lowest count and opacity at zero), and `colorReverse` flips the ramp.
`colorScale` (`linear`, the default, or `exponential`) and `colorExponent` (0.1 to 2, 0.5 when absent) bend the opacity of the opacity mode only.
`cellGap` (0 to 25 pixels) and `cellRadius` (0 to 10) shape the cells, `showValues` (`auto` where the text fits, `always` or `never`, the default) writes each count in its cell, and `cellUnit` and `cellDecimals` format the counts in the cells, the tooltip and the color legend.
`hideBelow` (0 when absent) and `hideAbove` leave out cells whose count is at or below, or at or above, them, and those cells do not count toward the color range.
`legend: "hidden"` removes the color legend under the chart.
The agent prompt does not list these options.
A `state-timeline` or `status-history` panel sizes its bars with `rowHeight`, the share of each row the bar fills (0.1 to 1), and a `status-history` panel sizes its cells with `columnWidth`, the share of each cell's width (0.1 to 1).
When either is absent the bar leaves a 2 pixel gap under it and a cell a 1 pixel inset on each side.
`alignValue` (`left`, `center` or `right`) places a `state-timeline` segment's text.
These two types take the same `legend`, `legendMode` and `legendWidth` as a time series, where the legend lists the states and clicking one hides its segments, and `tooltip` (`single`, `all` or `hidden`), `tooltipSort`, `tooltipHideZeros`, `tooltipMaxWidth` and `tooltipMaxHeight`.
`legendValues`, legend sorting and legend calculations do not apply to them.
Their `fill` field prop (0 to 100) sets how opaque the bars are, and `lineWidth` (0 to 10) draws a border around them.
A timeline written without `fill` is opaque and one written without `lineWidth` has no border, and a `fill` of 0 is drawn opaque.
`spanNulls` joins a gap between two values into the earlier state: `true` joins every gap and a number joins gaps up to that many milliseconds.
A `stat`, `gauge`, `bargauge` or `piechart` panel shows one value per number field, reduced by `reduce`.
Three optional `display` keys change that.
`allValues: true` shows one value per row of each field instead, at most `limit` rows per field (1 to 5000, 5000 when absent), labelled by the first text field of the row, else the row's time, else its row number.
`fields` picks the fields to show: a field name, or a pattern written between slashes such as `/^cpu/`, and `/.*/` for every field, text fields included.
When `fields` is absent the panel shows the number fields.
Time fields are never shown as values.
A `stat` panel has more optional `display` keys.
`justify: center` centers the name and the value in each tile.
`titleSize` and `valueSize` set the font size of the name and the value in pixels (6 to 200), where the tile otherwise uses 13 and 28.
`percentChange: true` shows the change between the first and last value of the range next to the value, with an arrow and, for screen readers, the words "up" or "down".
`percentChangeColor` colors only the arrow: `standard` (up is green, down is red), `inverted` (the reverse) or `value` (the color of the value).
`wideLayout: true` puts the name and the value on one row when a tile is at least 200px wide and twice as wide as it is tall; when it is absent or `false` the name sits above the value.
A `piechart` panel has more optional `display` keys.
`sliceLabels` is a list of `name`, `value` and `percent` written on the slices that are big enough, with `titleSize` for the name and `valueSize` for the rest (1 to 200 pixels, 12 by default).
Its legend takes `legend` (`bottom`, `right` or `hidden`), `legendMode` (`list` or `table`), `legendShow` (a list of `value` and `percent`, both by default), `legendSortBy` (`name` or `value`), `legendSortDesc` and `legendWidth`, and clicking an entry hides its slice.
Its tooltip takes `tooltip` (`single`, `all` or `hidden`), `tooltipSort` (`none`, `asc` or `desc`), `tooltipHideZeros`, `tooltipMaxWidth` and `tooltipMaxHeight`.
A `gauge` panel's `thresholdLabels: true` writes the value of each threshold, formatted with the field's unit, along the arc.
A `bargauge` panel draws its bars as `barStyle` `basic` (a filled track) or `lcd` (equal cells lit up to the value).
Its `valueMode` is `color` or `text`, which both draw the value in the foreground color with the threshold icon and word beside it, or `hidden`, which leaves the value out and keeps the threshold icon and word.
Its `namePlacement` is `auto` (the name over a horizontal bar and under a vertical one), `top` (over both), `left` (in front of a horizontal bar) or `hidden`, and `showUnfilled: false` leaves out the unfilled part of the track.
`minWidth` and `minHeight` keep each gauge or bar at least that many pixels (1 to 2000), the panel scrolls when they do not fit, and a bar gauge's `maxHeight` caps the height of a horizontal bar.
`titleSize` and `valueSize` set the size in pixels (1 to 200) of the name and the value text of a gauge or a bar gauge.
A `logs` panel shows its lines newest first (`order`), with `wrap` and `showTime` off, and has more optional `display` keys.
`showLabels: true` writes each line's unique labels after the time, and `showCommonLabels: true` writes the labels shared by every line once above the list; with one series every label is common.
`details: false` turns off expanding a line, and `detailsMode: sidebar` shows the fields of the chosen line in a column beside the list instead of under the line.
`prettify: true` indents a message that is a JSON object or array.
`dedup` shows one line for messages the strategy counts as equal and writes "Repeated N times" beside it: `exact`, `numbers` (numbers are ignored) or `signature` (letters and numbers are ignored), `none` by default.
`controls: true` adds a bar above the list to change the order, wrapping, time and deduplication while looking at the panel; the choices are not saved.
`fontSize: small` draws the lines at 11px, and `highlight: true` colors `key=value` pairs and upper case level words in a message.

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
{
  "title": "Deploy",
  "method": "POST",
  "url": "https://deploy.example.com/api/deploy",
  "body": "{\"env\": \"${env}\"}",
  "confirm": "Deploy to ${env}?"
}
```

- A `url` is an `https` address with the host written out; `${name}` references may appear only after the host.
- `headers` and `query` take `name` and `value` pairs.
  Credentials and transport headers (such as `Authorization`, `Cookie` or a name containing `token`, `secret` or `password`) are refused with `action_header_not_allowed`, because the server adds what it needs.
- The `url`, `body` and header and query values may reference declared variables, `__from`, `__to`, `__user.login`, `__user.email`, `__user.name`, `__user.id`, `__value.raw`, `__value.text`, `__value.numeric` and `__data.fields.<field>`.
  Any other name is refused with `unknown_variable`.
- `oneClick` on an element (`action` or `link`) needs at least one action or link.
- A person with Edit on the dashboard runs an action, and the server sends it on their behalf to an allowed domain; see [Canvas actions](../operate/outbound-access.md#canvas-actions).
  An action is sent as the latest saved version describes it, and it carries no credentials.

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
- `exemplar`, on a Prometheus range query, to also fetch its exemplars.
- `hide`, to keep the query out of the panel's data.

The query language follows the datasource: PromQL for Prometheus and a single SQL `SELECT` for ClickHouse.
`$name`, `${name}` and `${name:format}` insert the value of a variable.
Macros such as `$__rate_interval` for Prometheus and `$__timeFilter(Timestamp)` for ClickHouse expand when the query runs, from the time range and step of the request.
A datasource is a uid from the datasources file or `${name}`, naming a datasource variable.
An `adhoc` variable filters Prometheus queries automatically, and filters ClickHouse queries that leave out `$name` only when it sets `applyToAll`.
See [Datasources and variables](datasources-and-variables.md).

## Expressions

A query can be an expression instead of datasource text: it has a required `ref` and an `expression` object, and no `query` or `datasource`.
The server computes it from other queries of the same panel, after they have run.
The `type` of the expression says what it does:

- `math` takes `math`, arithmetic over query refs written `$A` or `${A}`, such as `$A * 2` or `abs($A) / $B`.
- `reduce` takes an `input` ref and a `reducer` (`mean`, `min`, `max`, `sum`, `count`, `last` or `median`), and turns each series into one number.
  `mode` says what happens to a value that is not a number: `strict` (the default) gives no number for a series that has one, `dropNonNumbers` leaves it out, and `replaceNonNumbers` swaps it for `replaceWith`, which that mode requires and every other mode refuses.
- `resample` takes an `input` ref, a `window` such as `1m`, a `downsampler` (`mean`, `min`, `max`, `sum` or `last`) and an `upsampler` (`pad`, `backfill` or `fillNA`), and puts the series on a regular grid of that width.
- `threshold` takes an `input` ref, an `evaluator` (`gt`, `lt`, `withinRange`, `outsideRange`, `withinRangeIncluded` or `outsideRangeIncluded`) and `params`, one number for `gt` and `lt` and a lower and an upper number for the range evaluators, and turns each number into 1 or 0.

A ref that the panel does not have, or expressions that depend on each other in a circle, are refused when the document is saved.
A query that an expression reads can set `hide` to stay out of the panel's data.
The server runs a hidden query only when an expression that is itself shown names it.

An expression that cannot be computed, such as a `resample` whose input is a single number instead of a time series, fails with status `422` and the code `expression_error`.

## Field options, overrides and transforms

A data panel carries its display options directly, and an override repeats the same options for the series or fields it matches.
The options are `unit`, `decimals`, `min`, `max`, `fieldMinMax`, `thresholds` and `thresholdMode`, `color`, `colorMode`, `colorScheme`, `colorBy`, `displayName`, `noValue`, `mappings`, `links`, and for charts `style`, `fill`, `lineStyle`, `lineInterpolation`, `lineWidth`, `points`, `pointSize`, `spanNulls`, `insertNulls`, `nullAsZero`, `barAlignment`, `barWidthFactor`, `barMaxWidth`, `thresholdsStyle`, `stack`, `axis`, `hidden`, and for the axes `axisLabel`, `axisSoftMin`, `axisSoftMax`, `scale`, `logBase`, `linearThreshold`, `axisCenteredZero`, `axisWidth`, `axisGrid`, `axisBorder` and `axisColor`.
`fieldMinMax: true` makes a gauge or bar gauge range each field by its own values when it has no `min` or `max`, instead of one range shared by every field; other panel types already range each field by itself.
`displayName` may use `${__field.name}`, `${__field.displayName}`, `${__series.name}`, `${__data.refId}`, `${__field.labels}` and `${__field.labels.<label>}`, and dashboard variables as `$name`, `${name}`, `[[name]]` and `${name:format}` (`raw`, `csv`, `text`, `pipe`, `json`, `percentencode` or `queryparam`); several selected values are joined with a comma and a space, and All reads `All`.
A name that is not known is left as written.
A link with `oneClick: true` is a data link on stat, gauge, bar gauge and table panels: when it is the only data link, the whole tile, bar, gauge or cell is the link.
`lineInterpolation` is `linear`, `smooth`, `stepBefore` or `stepAfter`, and stacked series always draw linear.
`lineWidth` is 0 to 10 pixels (2 by default; 0 draws no line, so a filled series shows only its fill).
`points` is `auto` (a marker on samples with no neighbour, the default), `always` or `never`, and `pointSize` is the marker diameter in pixels, 1 to 40 (4 by default), which also sizes the markers of the `points` style.
`spanNulls` is `false` (the default, a gap breaks the line), `true` (every gap is joined) or a number of milliseconds (gaps up to that long are joined).
`insertNulls` is a number of milliseconds: a series with a longer pause between two samples breaks its line there, which a series that also sets `spanNulls` then joins only up to its own limit.
`nullAsZero` draws a series' missing values as 0 (`false` by default).
`barAlignment` puts a bar `before`, at the `center` (the default) or `after` its sample, `barWidthFactor` is the share of the space between samples a bar fills, 0.05 to 1 (0.6 by default), and `barMaxWidth` caps a bar at 1 to 200 pixels (64 by default).
`thresholdsStyle` draws the thresholds on the chart as `off`, `line`, `dashed` (the default), `area`, `line+area`, `dashed+area` or `series` (the line takes the color of the threshold band its value is in), and the first series on the left axis decides it for the panel.
The axis options belong to the axis a series draws on, and the first series on an axis decides its settings.
`axisLabel` is up to 100 characters of text beside the axis.
`axisSoftMin` and `axisSoftMax` widen the automatic range to include them but never cut data, while `min` and `max` are hard limits that win over them.
`scale` is `linear` (the default), `log` (with `logBase` 2 or 10, 10 by default; values at or below zero are not drawn) or `symlog` (linear within `linearThreshold` of zero, 1 by default, and logarithmic outside).
`axisCenteredZero` makes the range symmetric around zero.
`axisWidth` fixes the width of the axis in pixels (0 to 400), `axisGrid` shows or hides its grid lines (by default only the left axis, or the right one when there is no left axis, draws a grid), `axisBorder` draws a line along the axis, and `axisColor` is `text` (the default) or `series` (the axis takes the color of its first series).
`valueTransform` draws a series `negativeY` (below zero, mirrored, while the legend and tooltip keep its real values) or `constant` (a flat line at its first value).
`hideInLegend` and `hideInTooltip` leave a series out of the legend or the tooltip but keep it drawn, and `hidden` leaves it out of the chart.
`stackGroup` (up to 50 characters, `A` by default) makes series stack with the others of the same group only, and `fillBelowTo` is the name of another series: the area between the two is filled, in the series' color at its `fill` (20% when `fill` is 0), and it is ignored for a stacked series or a name no series has.
Only `timeseries` and `trend` panels draw these five options.
A percent stack always uses a 0 to 100% axis whatever the scale.
A `table` panel also takes `cell`, which sets how a column draws its cells: its `type` is `auto` (the plain cell, colored by thresholds and colored mappings), `text` (colored by the threshold, mapping or field `color`), `background` (filled with that color, with text chosen for contrast; `wholeRow` fills the whole row), `gauge` (a bar over the column's range, `gaugeStyle` `basic` or `lcd`), `sparkline` (a mini chart of an array of numbers, such as the `Trend` column of a `timeSeriesTable` transform, drawn with the column's `style`, `fill`, `lineInterpolation`, `lineWidth`, `points`, `pointSize`, `spanNulls`, `min`, `max`, `axisSoftMin` and `axisSoftMax`), `json` (a disclosure with the pretty-printed value), `image` (the value is an https address, drawn through the server, with `imageAlt` and `imageTitle`), `links` (the field's data links as anchors) or `pill` (the value split on commas into labels).
`valueMode` (`color`, `text`, the default, or `hidden`) sets how a gauge or sparkline cell shows its value, and `wrap` lets the cell's text wrap onto several lines.
Other panel types ignore `cell`, and the agent prompt does not list it.
A `table` column also takes `width` and `minWidth` (20 to 2000 pixels), `align` (`auto`, the default, right-aligns numbers and left-aligns the rest, or `left`, `center`, `right`), `filterable` (a button in the header opens a checklist of the column's distinct values and keeps the rows that match), `inspect` (a button in every cell opens its full value in a dialog) and `hideHeader` (the header text is hidden visually but still read by screen readers).
A column with `width` has exactly that width, the other columns share what is left and never get narrower than their `minWidth`, and the table scrolls sideways when they do not fit; with none of them set the browser sizes the columns from their content.
`colorMode` chooses how a color is picked: `palette` (the series palette, the default when no `color` is set), `paletteByName` (a palette color from a stable hash of the series name, so a series keeps its color when others come and go), `fixed` (the `color`, the default when one is set), `shades` (the `color` in a step lighter or darker per series), `thresholds` (each series takes the color of the threshold its value reaches) or `scheme` (each value or series takes a color from the ramp `colorScheme` names, by its place between the field's `min` and `max`, else the data range).
`colorScheme` is one of `greenYellowRed`, `redYellowGreen`, `blueYellowRed`, `yellowRed`, `bluePurple`, `yellowBlue`, `blues`, `reds`, `greens`, `purples`, `oranges`, `viridis`, `magma`, `plasma`, `inferno` or `cividis`; every ramp is built from the theme's own colors, so light and dark themes follow it.
`colorBy` is the value of a series that picks its color under `thresholds` and `scheme`: `last` (the default), `min` or `max`.
Time series and trend panels draw every `colorMode`, while stat, gauge, bar gauge, table cells with colored values, bar chart and XY chart draw `scheme`; stat, gauge, bar gauge, table, bar chart, pie chart and XY chart already color values by their thresholds.
Only `timeseries`, `trend` and `candlestick` panels draw the line style and axis options, so other panel types ignore them and a Grafana import reports them there.
Thresholds ascend, and the first step may leave out `value` to act as the base color.
A color is `ok`, `info`, `warning`, `critical` or a `#rrggbb` hex value.
A unit is any Grafana unit id such as `reqps`, `s`, `bytes`, `percentunit` or `radbq`, or custom `suffix:`, `prefix:`, `time:`, `si:`, `count:` or `currency:` text.
The text after `time:` is a date format, `si:` a symbol that scales by 1000 with SI prefixes (`si:mF` starts at milli), `count:` text after a K, Mil or Bil scaled number, and `currency:` a symbol before a scaled number.
The agent prompt lists only the common ids and `suffix:` and `prefix:`, but a Spec may use any of them.

An override has a `match`, which selects by `query` (a `ref`), `series`, `seriesRegex`, `field`, `fieldType` or `value`, and at least one option to apply.
A `value` match selects the number fields whose `reducer` result compares to `value` with `op` (`eq`, `neq`, `lt`, `lte`, `gt` or `gte`).
A panel has at most 50 overrides.

`transform` is an ordered list of up to 20 steps that reshape the result in the browser after the query runs.
The steps are `merge`, `seriesToRows`, `organize`, `reduce`, `joinByField`, `filterFieldsByName`, `calculateField`, `sortBy` and `limit`.
Seven more steps reshape single fields and filter rows: `convertFieldType` (`conversions` of `field` and `to`: `number`, `string`, `time`, `boolean`, `enum` or `other`, with `dateFormat` and `joinWith`), `formatString` (`field`, `format` and, for `substring`, `start` and `end`), `formatTime` (`field`, a date `format` made of `YYYY YY MM M DD D HH H hh h mm ss SSS A a`, and `utc`), `renameByRegex` (`regex` and `renameTo`, where `$1` is a captured group), `filterByValue` (`keep` and `match`, and `filters` of `field`, `op` and `value`, or `from` and `to` for `range`), `filterByRefId` (`include` lists the query refs to keep) and `extractFields` (`field`, `format` of `auto`, `json`, `kvp`, `delimiter` or `regexp`, `paths`, `delimiter`, `regex`, `replace` and `keepTime`).
Eight more steps reshape frames by their labels and layout:
`labelsToFields` (`mode` of `columns` or `rows`, `include` to keep some labels, `valueLabel` to name the value field after a label),
`joinByLabels` (`value` is the label whose values become columns and `on` the labels to join on),
`partitionByValues` (`fields`, `keepFields`, and `naming` of `labels`, `name` or `append`),
`prepareTimeSeries` (`format` of `wide`, `many` or `long`),
`timeSeriesTable` (one row per series with a trend and a reduced value, `stats` choosing the reducer per query ref, `lastNotNull` by default),
`rowsToFields` (each row becomes a field, with `mappings` of a column `field` to `name`, `value`, `unit`, `min`, `max`, `decimals`, `displayName`, `noValue` or `color`),
`configFromData` (reads field settings from the frame of the `configQuery` ref, with `mappings` of `field` to `unit`, `min`, `max`, `decimals`, `displayName`, `noValue` or `color` and an optional `reducer`, and applies them to the number fields or to the fields `applyTo` matches) and
`concatenate` (puts the fields of every frame side by side, and `nameMode` of `drop`, `field` or `label` with `nameLabel` says where the frame name goes).
Field names match a field's name or its series name, and a step that names a field the data does not have changes nothing.
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
