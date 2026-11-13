---
title: Import Grafana dashboards
description: Import a Grafana dashboard from the web page, the CLI or MCP, and how datasources and folders are matched.
---

You can bring a Grafana dashboard in three ways: the web page Import from Grafana, `observe import`, and the `import_grafana_dashboard` MCP tool.
The page and `observe import` call `POST /api/v1/imports` with the dashboard JSON as pasted or uploaded, the tool runs the same import, and `POST /api/v1/imports/preview` shows the result first without saving anything.
The JSON can be the classic dashboard model, an export that shares a dashboard externally, or the response of Grafana's dashboard API.
Home's **Import a dashboard** tab starts the web page: paste the JSON or choose a file there and Home opens the page with it; see [Home](home.md).

To bring over every dashboard of a running Grafana in one run, with its folders, see [Switch from Grafana](switch-from-grafana.md).

## Match datasources

Each datasource the dashboard uses is matched to one of yours, in the order described under `grafanaUids` in the [datasources file](../reference/datasources-file.md).
An import cannot be saved while a datasource has no match, and SlashObserve never guesses between two datasources of the same type; give the match with the mapping that comes with the request, or add the Grafana uid to `grafanaUids`.

SlashObserve imports Prometheus, ClickHouse, Loki, Tempo, Elasticsearch (the `elasticsearch` plugin) and OpenSearch (the `grafana-opensearch-datasource` plugin) datasources.
A Grafana datasource whose type has no configured datasource here does not block the import: its panels import as placeholders that say so, such as "No Loki datasource is configured.".
A datasource of another type, such as InfluxDB, imports its panels as placeholders that say "Influxdb is not supported yet.".

## Choose the folder

The dashboard goes into a folder, chosen in this order:

1. The request's `folder` title: `--folder` for `observe import`, `folder` for the API and the MCP tool.
   The web page sends the folder you pick in its **Folder** card, and sends none when the choice is **Automatic**.
2. The `meta.folderTitle` of an export taken from Grafana's dashboard API, which names the folder the dashboard had in Grafana.
3. Your default folder: your My Dashboards; for a key, General.

A title matches an existing folder without regard to case.
A title of `General` or `My Dashboards` means your default folder.
An export title that is empty or not a valid folder title means the default too.
An invalid `folder` in the request is refused with `400 invalid_request`.

When no folder has the title, an Editor or Admin creates it.
When a folder has the title but you cannot create dashboards in it, the dashboard goes to your default folder instead, and the response carries a `folder_note` saying so (`observe import` prints it on standard error, even with `-q`).

The **Folder** card on the web page lists the folders where you may create dashboards, except General, because a title of `General` means your default folder, which **Automatic** already gives you.
After an import the page says "In folder `<title>`" and shows the `folder_note` when the dashboard did not go where the request or the export asked.
The folder is chosen before the dashboard is saved, so an import that then fails can leave a new, empty folder behind; delete it from its settings page.

## Saving and the import report

### Unverified panels and later saves

The request body, with the dashboard JSON as a string, can be at most 8 MiB.
It saves the dashboard as version 1 and runs every query once.
Failing queries mark their panels and variables unverified in the import report instead of failing the import.
The **Imported, not verified** problem filter on the [dashboards list](dashboard-health.md#in-the-dashboards-list) lists every dashboard that still has unverified panels or variables and sends them to the agent.

Later saves keep a failing panel or variable unverified, instead of failing, for as long as its queries and the variables they use are unchanged, and so does its panel type, time override (`timeFrom` and `timeShift`), `maxDataPoints` and `interval`; a new or changed query must return data, and a panel whose queries pass again is no longer unverified.
A save with `update_dashboard` runs every query.
A save with `update_panels` runs only the queries it changed, unless it changes the dashboard's default `time`, which runs every query; either way a panel the save did not change stays unverified while it still fails, and an unverified panel that now returns data leaves the list.

### The import report

Below the panel list, **Show every option, block by block** opens the report grouped by block: the dashboard settings, each variable, and each panel with its queries, transformations, field config and overrides.
Each block is collapsed until you open it, and lists every Grafana option it carried as mapped, with the ObserveSpec fields it became, excluded by decision, with the reason SlashObserve leaves it out, or not imported, with the reason.
An option mapped without a field name is one no catalog describes yet, such as the options of legacy panels.
Options that need no setting (Grafana defaults, options with no visible effect and editor bookkeeping) are shown as a count that you can open to see each of them.
A very large dashboard lists at most 200 options per block and 10,000 in all, keeping excluded, then not imported, then mapped options before those that need no setting.

A value SlashObserve does not support, such as a delay in days or a time zone name the server does not know, is left out and listed in the import report.

### The stored original and Grafana links

The original JSON is stored with the import so a newer importer can convert it again, and it is never shown to the agent.
The import record's `grafana_uid` holds the dashboard's Grafana uid, so `GET /api/v1/grafana/resolve/{uid}` finds the dashboard from an old Grafana link.
Only the first import of a uid holds it, and only a uid of 1 to 40 letters, digits, `-` or `_`; importing the same uid again makes a separate dashboard whose record has an empty `grafana_uid`, until the dashboard holding it is deleted.

## Dashboard settings

A dashboard's time settings, links and editability import as follows.

| Grafana setting | Imports as | Notes |
| --- | --- | --- |
| Time zone, week start and fiscal year start month | The [dashboard fields](../concepts/observespec.md#the-dashboard) `timezone`, `weekStart` and `fiscalYearStartMonth` | |
| The time picker's now delay | The `nowDelay` dashboard field | |
| Time range | The same range | It may use `now/fy` and `now/fQ`, and a range written in epoch milliseconds carries over. |
| The time picker's hidden flag, refresh intervals, quick ranges, live now switch and refresh intervals in days | Carried over | A refresh interval of `500ms` is left out and reported. |
| Links to other dashboards by tag | Links that list the matching dashboards | |
| Links to an address | Links that keep their keep-time and include-variables options | |
| A link's tooltip | Imported as written | It is cut to 200 characters and reported when cut. |
| Link icons other than Grafana's default, and placement in the controls menu | Reported | The link is shown inline. |
| A dashboard that is not editable in Grafana (`editable: false`) | Editable by anyone with Edit access to its folder | The report says so. |

## Variables

### Custom, text box and interval variables

A query's min step written as a variable (`$name`, `${name}` or `[[name]]`) imports as `${name}`, so the query uses the step selected in that variable.
A `custom` variable of durations that is single-select, has no All and is used as a min step becomes an `interval` variable, and the import report says so.

A `custom` variable's `text : value` pairs import as `{ text, value }` options, so the picker shows the text and the selection uses the value.
A text box imports its saved value as its default, else its default text.

### Query variables

A `query` variable's sort imports as `alpha`, `num`, `alphaCaseInsensitive` or `natural`, each in its two directions, and `allowCustomValue` imports as written, with Grafana's own default of true when it is missing.
A `query` variable's static options import as `staticOptions`, with Grafana's placement as `staticOrder`, and a query variable that Grafana never refreshes imports as `refresh: "never"` with its saved options as the static options, so only those are offered; one that never refreshes and has no saved options keeps loading and the report says so.
A query variable's `regexApplyTo` imports without a note at its default `value`; at `text` the regular expression is not applied, because options here carry a value only, and the report says so.

A `query` variable whose query uses a variable the dashboard does not declare is dropped and reported, and the panels that use it become placeholders.
The exception is a Prometheus query whose undeclared `$name`, `${name}` or `${name:format}` is the label that exactly one other `query` variable lists with `label_values(selector, name)`, as in VictoriaMetrics' per-tenant dashboard after its rename of `accountID` to `account`: the reference imports as that variable, the report says so on the variable, and the panels that use it import whole.
A variable that lists the label without a selector, a variable that depends on the one being repaired, or two variables that list the label keep the drop.

### URL sync, hidden variables and datasource variables

A variable's `skipUrlSync` imports as written, so it stays out of the URL and starts from its default on every load.
A variable that Grafana hides in the controls menu (`hide` 3) imports shown and is reported, as is a datasource variable's `multi`, `includeAll` or `allowCustomValue: false`, because a datasource variable selects one datasource from the ones it lists.

### Ad hoc variables

A saved ad hoc filter list imports as the variable's `default`, and its base filters as `baseFilters`, with each filter's key label and first value label, and a list of values that is the regular expression's alternatives joined with `|`.
On ClickHouse the operators `<` and `>` and the condition OR import too; on Prometheus those three are reported, because label matchers can only be ANDed and have no comparison.
A filter with another operator, an invalid key or regular expression, or another condition is reported, in the base filters as in the saved ones.

An ad hoc variable's `defaultKeys` import as `keys` (up to 100), the keys the filter picker offers instead of asking the datasource; Grafana's key texts are shown as the keys themselves and the report says so.
On Prometheus, `allowCustomValue: false` imports as `listedValuesOnly`, so a filter's key and value must be picked from the lists the picker shows; on another datasource the picker lists no values, so it is reported.
A ClickHouse ad hoc variable imports with `applyToAll`, so its filters apply to every query on its datasource (see [Ad hoc filters](../concepts/datasources-and-variables.md#ad-hoc-filters)), and for the Altinity plugin the report notes that Grafana filters only the outermost query.

### Group-by variables

A Grafana group-by variable on Prometheus imports as a `groupby` variable, with its current labels (else its default value) as the `default`, up to 20 labels that are valid label names.
Like Grafana, it adds the selected labels to the grouping of every aggregation in the queries on its datasource, so the queries need no change.
Its saved label options import as `keys` (up to 100 valid label names), the labels the picker offers instead of asking the datasource.
A group-by variable on any other datasource is dropped and reported.

A Grafana 13.1 ad hoc variable with `enableGroupBy` on Prometheus imports as the ad hoc variable plus a `groupby` variable on the same datasource and with the same visibility, named `<name>_groupby` (or the first free name that follows that pattern), without saved labels; it keeps the ad hoc variable's `skipUrlSync` and its `defaultKeys` as `keys`.
On another datasource, or when the dashboard already holds 20 variables, `enableGroupBy` is reported.

### Loki and Tempo variables

A Loki query variable imports as written, and one saved in the label values form becomes `label_values(<stream>, <label>)`; a Loki label names variable and any Tempo query variable keep their saved values as a fixed list, and an ad hoc variable on Loki or Tempo is dropped and reported.

## Queries

### Prometheus

A min step of `$__interval` or `$__rate_interval` imports as no minimum, and a resolution factor from 2 to 10 is kept as `intervalFactor`.
A Prometheus query that Grafana runs as both an instant and a range query (`instant` and `range` both on) imports as two queries: the range query keeps its ref, and an `instant` copy without the min step or resolution factor takes the next free ref, which the import report lists as `refId A (instant) -> B`.
Both queries are hidden together, and a style override or a `filterByRefId` transformation that names the original ref covers the copy too.
A query with `instant` on and `range` unset or off stays one instant query.
A panel that already holds 26 queries keeps only the range query and reports the instant one.

A Prometheus target's `exemplar` flag is kept on a time series and a heatmap, and dropped without a note on an instant query and on every other panel type, which draw no exemplars.

### Loki

A Loki query imports its LogQL as written, on the matched Loki datasource.
A Loki metric query keeps its legend, runs as an instant query when its `queryType` is `instant` (or, with no `queryType`, when `instant` is on), and keeps a resolution from 2 to 10 as `intervalFactor`.
A Loki `step` of `$__interval` or `$__auto` imports as no minimum, and another step becomes the smallest step and is reported, because Grafana uses that step as is.
`$__auto` in a Loki query becomes `$__range` in an instant query and `$__interval` otherwise, and the import report lists the rewrite.

A Loki log query, one that starts with a stream selector, always reads the whole time range, so its legend, resolution and step have no effect, and an instant log query is reported.
A Loki query whose `queryType` is `stream` imports as a range query and is reported.

### Tempo

A Tempo query imports its TraceQL or trace id as written, on the matched Tempo datasource, and a trace id held in a variable, such as `${trace_id}`, is looked up after the variable is expanded.
The Tempo search form state saved beside a TraceQL query (filters, group-by, service names, durations and the metrics options) is editor state and imports without a note.
A Tempo query of the search builder or the legacy search, the service graph, an uploaded trace file, or a TraceQL metrics pipeline such as `| rate()` is dropped, and a panel left with no query becomes a placeholder.

### Elasticsearch and OpenSearch

An Elasticsearch or OpenSearch query is compiled from its Lucene query, metrics and bucket aggregations on the matched datasource.
A query whose only metric is a count over an automatic date histogram on the time field imports as `lucene:<query>`, and a logs, raw data or raw document query (by `queryType`, by its first metric, or by the OpenSearch `luceneQueryType`) imports as `lucene:<query>` run as an instant query that returns lines; a raw document query is reported, because it shows `_source` and SlashObserve shows lines.

Every other query imports as a Query DSL body: a `query_string` of the Lucene query, and the bucket aggregations nested in their order (date histograms with `$__interval` or the fixed interval, `$__from` and `$__to` as extended bounds; terms with their size, order and missing value, a size of 0 meaning 500; filters labelled as saved; histograms) around the metrics, keyed by their Grafana ids.

Metrics import as follows.

| Metric | Imports as |
| --- | --- |
| `avg`, `sum`, `min`, `max`, `cardinality`, `percentiles`, `extended_stats` and `rate` | The same metric, with its fields and settings |
| `moving_avg` | A `moving_fn` with an unweighted average over its window |
| `derivative`, `cumulative_sum`, `serial_diff` and `bucket_script` | The same metric, with its bucket paths and scripts |
| A metric of a type SlashObserve does not know, and a bucket aggregation other than those listed above | Reported |

An empty Lucene query matches everything, as in Grafana.
A query's `alias` imports as the legend, with `{{term <field>}}` rewritten to `{{<field>}}` and `{{field}}` written out as the metric's field name.
A `timeField` on a query is editor state: Grafana copies the datasource's time field onto every query, and a date histogram without a field uses it.

An Elasticsearch or OpenSearch query variable saved as `{"find": "terms", "field": <field>, "query": <lucene>}` becomes `label_values(lucene:<query>, <field>)`, or `label_values(<field>)` without a query, and its `size` is reported; a variable query in any other form keeps its saved values as a fixed list.
An ad hoc variable on Elasticsearch or OpenSearch imports as on ClickHouse: with `applyToAll`, the operators `<` and `>`, the condition OR, and dotted field names as keys.

### ClickHouse queries from the Altinity plugin

A ClickHouse query from the Altinity plugin that starts with a whole-query macro (`$rate`, `$perSecond`, `$delta`, `$increase`, `$columns`, their `Columns`, `Ms` and `ColumnsAggregated` forms, and `$lttb` and `$lttbMs`) is written out as plain ClickHouse SQL that uses SlashObserve's time macros, so the saved query no longer needs the Altinity macros.

A macro that is not at the start of the query, or whose arguments cannot be read, is still left out and listed in the import report.

Inside a query, these macros import as follows.

| Macro | Imports as | Notes |
| --- | --- | --- |
| `$naturalTimeSeries` | Buckets by year, month, week or day for long ranges | |
| `$timeFilter64ByColumn` | The millisecond time filter | |
| `$timeFilterByColumn(column)` | `$__timeFilter(column)` | |
| `$adhoc` | The dashboard's ad hoc variable, when exactly one ad hoc variable uses the query's datasource | It becomes `1` without a note when the dashboard declares no ad hoc variable, and `1` with the note "Ad hoc filters inside this query are not applied." when ad hoc variables exist but not exactly one uses the query's datasource. |
| `$conditionalTest(condition, $variable)` and its three-argument form `$conditionalTest(condition, otherwise, $variable)` | `$__conditionalAll`, which writes the otherwise condition when the variable selects All, as the plugin does | See the keyword rule below. |

A condition that starts with `AND` or `OR`, as in `$conditionalTest(AND service IN ($service), $service)`, keeps that keyword outside the macro (`AND $__conditionalAll(service IN ($service), $service)`), because the macro writes the condition inside parentheses; with an otherwise condition, both conditions must start with the same keyword.
A call whose two conditions start with different keywords, or only one of them with a keyword, is left out and listed in the import report.

### Time columns and counter macros

The query's time column type decides how the time macros read the column: `DATETIME64` keeps its sub-second precision, while `TIMESTAMP`, `FLOAT` and `TIMESTAMP64_3`, `_6` and `_9` (seconds, milliseconds, microseconds and nanoseconds since the epoch) are compared as numbers and bucketed as times.
The `round` option rounds the start of the range down and the end up in the time filter, `$from` and `$to`; the first bucket of a counter macro has no value when `extrapolate` is off, because by default it is compared with a virtual earlier bucket.

The `Columns` forms return one row per time and key (time, key, value), which SlashObserve draws as one series per key, named by the key.
Counter macros always use window functions (`lagInFrame`), whatever the plugin's `useWindowFuncForMacros` says, because `runningDifference` is deprecated in current ClickHouse versions.
The first point of a counter series has nothing to subtract from: a rate treats the previous point as one step earlier, a delta or increase is 0, and a per-second value is empty.

### Options the plugin carries

The plugin's `extrapolate` option, which the plugin applies in the browser to the edges of a rate series, is reported on the rate family; `round` is dropped silently only when it is `$step`, `0s` or divides the query's explicit minimum step, and reported otherwise.
A ClickHouse query that keeps its generated SQL drops the query builder's settings without a note.

### Server-side expressions

A server-side expression (math, reduce, resample or threshold) imports as an expression query that reads the queries it names (see [Expressions](../concepts/observespec.md#expressions)).
A threshold keeps its main condition without its recovery threshold, because Grafana applies the recovery threshold only when it evaluates alert rules.
A classic condition is dropped because it belongs to alert rules, which SlashObserve does not import, and a SQL expression is dropped because SlashObserve does not support it.
So are a math function SlashObserve does not support, math that reads a dashboard variable or a `$__` global, an expression whose input query did not import, and expressions that depend on each other in a circle.
The import report gives the reason for each, and a panel keeps at least one data query or becomes a placeholder.

## Field options and overrides

### Units, names and descriptions

A unit imports as itself, and unit text Grafana does not know imports as the suffix Grafana shows, up to 20 characters.
A display name keeps `${__field.name}`, `${__field.displayName}`, `${__series.name}`, `${__data.refId}`, `${__field.labels.<label>}` and dashboard variables in all four syntaxes; other `__` tokens, such as `${__data.fields[0]}`, are reported.

A field description (`fieldConfig.defaults.description`, also as an override property) imports on tables, where it is the tooltip of the column header, cut to 2000 characters; on every other panel type it is reported, because they draw no field description.
A field's `fieldMinMax` imports on gauges and bar gauges, where fields otherwise share one range, and is left out silently elsewhere, where every field already has its own range.

### Colors and value mappings

A field's color mode imports as the classic palette (the default), a palette by series name, a fixed color, shades or a named color scheme, and a series-colored mode imports on time series and trend panels, a scheme also on stat, gauge, bar gauge, bar chart, XY chart, state timeline and status history (a state is colored by its place between the field's min and max, else the data's, and a value mapping color wins).
A histogram colored by `thresholds` whose thresholds are only the base step imports as a fixed color, the base step's, because Grafana draws a histogram series in the color of the step its last value meets; with more steps it is reported.
A color mode a panel type does not draw is reported as "The color mode X is not drawn on Y panels."

A value mapping keyed by the empty string imports as the empty-value mapping.
A value mapping's icon is reported, and its text and color are kept.

### Overrides

An override imports its matcher (by name, regular expression, type, numeric fields, time fields, a list of types, a regular expression or names, query ref, list of names, value, first field, first time field or every field) and its properties like the panel's own; a list of types becomes one override per type.
A value matcher may reduce with `allIsNull` or `allIsZero`, which is how many dashboards hide series that are empty.
On time series and trend panels, and on the extra series of a candlestick, the series properties import too: negative-Y and constant transforms, hiding from the legend or tooltip, stack group and fill below to, and a legacy graph's series overrides import the same way.
A stack group written as `false`, `true` or a number becomes that text, so its series stack together and apart from the default group `A`, and a group on a series whose stacking is `none` has no effect.

### Panel links and data links

A data link's `oneClick` imports on stat, gauge, bar gauge and table panels and is reported on every other type, where the link stays a panel link.
A panel link to another dashboard, named by its title, imports as a link that opens that dashboard when it is clicked; imported dashboards get new ids, so the title is looked up at click time.
When several dashboards you can see share the title, or none does, the link opens the dashboards list filtered by the title instead.
The link carries the time range when Grafana's keep-time option was on and the variable selections when include-variables was on, and a panel link to an address keeps both options the same way, as the link tokens `${__url_time_range}` and `${__all_variables}`.
A panel link to a dashboard without a title, with a title over 200 characters or of any other type is reported and not kept.

### Field actions

Field actions (HTTP fetch actions in a field's defaults and overrides, with their inputs, button color, confirmation and one-click) import on stat, gauge, bar gauge and table panels, the types that draw them, and are reported on other types.
An action that is not an HTTP fetch is reported, and a one-click action stays one-click only when it is the field's only data link or action.

## Panel types

### Titles, time overrides and time zones

A panel with no title imports with an internal name (`Panel <id>`) that menus and screen readers use but that is not drawn, as in Grafana, and a transparent panel is drawn without a border or background.
A panel that hides its time override (`hideTimeOverride`) imports as `hideTimeOverride`, which leaves the override note out of the panel header while the override still applies to the queries.

A time series, state timeline or status history panel's single time zone (`options.timezone`: `utc`, `browser` or an IANA zone; `default` and an empty entry mean the dashboard's) imports as the panel's `timezone`, so its times are drawn in that zone; a list of several zones is reported, because Grafana draws one axis per zone and a panel draws in one.

### Layout and panels that reuse another panel

Panels in a collapsed row keep their layout relative to the topmost one, whatever order they were saved in.

Grafana's `-- Dashboard --` query, which shows another panel's results, imports as a query that reuses that panel (see [Reusing another panel's results](../concepts/observespec.md#reusing-another-panels-results)), and panels that reuse another are converted after all the others.
Its `withTransforms` setting is kept, and `adHocFiltersEnabled` imports as `adHocFilters`, which filters the reused rows by the dashboard's ad hoc filters.
A source panel that is missing, is not a data panel, repeats or itself reuses another panel drops the query with "The panel this one reuses could not be imported.", and a panel left with no query becomes a placeholder.
A reusing panel takes its time range and step from its source, so its own `interval`, `maxDataPoints`, `timeFrom` and `timeShift` are reported, and a reusing query that sits beside other queries is dropped and reported.

### Time series, trend and candlestick

A time series, trend, candlestick or legacy graph panel imports its legend (placement, list or table mode, values, sort, width, and hiding series that are all zero or empty) and its tooltip (sort, hide zeros, maximum width and height, and a legacy graph's cumulative values).
It also imports the axis options, the line style (interpolation, width, dots, dashes with a custom dash and gap length from 1 to 100 pixels, square dots, points), the fixed line, fill and point colors, the point symbol (circle or square), value labels, how gaps draw (spanning nulls, or inserted nulls given in milliseconds or at the query interval), the bar alignment and width, the threshold style and a candlestick's extra number fields.

A named Grafana color becomes its hex color, Grafana's `text` color (the theme's text color) becomes the Spec color `text`, any other CSS color name (such as `gray`) becomes its hex color, and a point symbol other than a circle or square, a dash pattern that is not one dash and one gap length, and an axis placement of top or bottom are reported.

A point size on a time series with automatic points imports and sizes the isolated points that automatic points draw, except Grafana's own size 5, which draws them at the usual size, and a size of 4, which is reported because it cannot be told from no size.
An axis color mode other than `series` is Grafana's default, `text`.

A candlestick imports its up, down and flat candle colors; a flat candle, whose open equals its close, otherwise takes the up color.

The annotation lane options of a time series panel (`options.annotations.multiLane` and `clustering`) import without a note, because Grafana 12.3 does not define them and SlashObserve draws no annotations.

### State timeline and status history

A state timeline and a status history import their row height (and a status history's column width), legend, tooltip (including hiding a series from it), line width, fill, null spanning and inserted nulls, and a state timeline imports its value alignment, and both import the rows per page (`perPage`, from 1 to 100), which pages the rows with previous and next buttons; a legend sort or calculation, hiding a series from the legend, and the value axis options import without a note, because these panels have no value axis and their legend lists states, and a legacy discrete panel imports as a state timeline with its value, range and color maps and its unit, while the keys that Grafana's own discrete panel migration does not read (row height, text sizes, colors, time axis, time precision and the legend value choices) import without a note.

### Value options of stat, gauge, bar gauge and pie chart

A stat, gauge, bar gauge or pie chart imports its value options: the first reducer, "Show" as every value (`allValues`, with `limit` when it is not 5000) and the `fields` name or pattern.
Grafana uses only the first reducer of these panels, so extra reducers are dropped without a note.

### Stat

A stat panel imports its text alignment (`justifyMode`), title and value sizes (`text.titleSize` and `text.valueSize`, from 1 to 200 pixels), percent change (`showPercentChange` and `percentChangeColorMode`) and `wideLayout`; a size outside that range is reported.
The percent size (`text.percentSize`) imports like the other text sizes, and the line graph mode (`graphMode: line`) draws the sparkline as a line without the area under it.

### Gauge and bar gauge

A gauge imports its threshold labels and its neutral value (`custom.neutral`, the value the bar grows from) and a bar gauge its `lcd` display mode, value mode, name placement and unfilled area; both import their text sizes (`text.titleSize` and `text.valueSize`, from 1 to 200 pixels).
The `basic` and `gradient` bar gauge display modes import as the basic bar, because bar gauges are drawn flat.
When the sizing is Manual, a gauge or bar gauge imports its minimum width and height (and a bar gauge its maximum height), using Grafana's own defaults when they are not set; with Auto sizing Grafana ignores these limits, so they are left out without a note.
A bar gauge also imports its orientation: horizontal, vertical or auto.

### Pie chart

A pie chart also imports its slice labels (name, value and percent), their text sizes (`text.titleSize` and `text.valueSize`, from 1 to 200 pixels), its legend and its tooltip.
The legend imports its placement, list or table mode, the values it shows (value and percent), the sort by name or value, the sort order and the width.
A legend sorted by a calculation such as Mean, the pie chart's orientation and its legend calculations are imported without a note, because a pie has no orientation and its legend shows no calculations.

A pie chart's slice order (`options.sort`) imports: largest first by default, smallest first, or the order of the data.
The tooltip imports its mode, sort, "Hide zeros" and maximum width and height.
A series hidden from the legend or the tooltip in the field options imports on a pie chart as it does on a time series, and stays drawn.

### Bar chart and histogram

A bar chart imports its label rotation, spacing and maximum length (`xTickLabelRotation`, `xTickLabelSpacing` and `xTickLabelMaxLength`), `barRadius` (up to 0.5), `fullHighlight`, `colorByField`, the value text size (`text.valueSize`, 6 to 100 pixels), and its legend and tooltip like a time series does, including the table mode, legend calculations, sort, width and the tooltip sort, "Hide zeros" and maximum sizes.
A histogram imports `bucketOffset`, `combine`, its stacking mode (normal, percent or none), and the same legend and tooltip options; Grafana's own `none` stacking and `single` tooltip are written explicitly, because a histogram without them stacks series and lists all of them.

On both, the axis options (label, soft limits, log and symlog scales, centered zero, width, grid, border and color mode) apply to the value axis, `lineWidth` is the bar border and `fillOpacity` the bar fill, and a series hidden from the legend or the tooltip in the field options imports and stays drawn.
A bar chart also imports the threshold style (lines, areas or both over the value axis), and a histogram the stack group of each series.
A fully transparent fill is reported because bars are drawn opaque at 0, and a histogram's `thresholdsStyle` and the bottom or top axis placement are still reported.

### Heatmap

A heatmap imports its color mode, scheme, steps, scale and exponent, fill, range (`color.min` and `color.max`) and reverse, the cell gap and radius, the cell value unit and decimals, the legend toggle, the low and high value filters (`filterValues.le` and `ge`) and `showValue`, the calculated buckets (`calculation.xBuckets` and `yBuckets`: mode, value and the y scale with its log base), the y axis (placement left, right or hidden, unit, decimals, range, width, label and reverse), the rows layout (`rowsFrame.layout`), the tooltip (`single`, `multi` as all rows, `none` as hidden, the y histogram, the color scale and the size limits) and the exemplar color, and it writes Grafana's own defaults when the panel does not set them (scheme mode with the Oranges scheme, 64 steps, a gap of 1 and values shown where they fit).

Grafana ignores the scale, exponent and fill in scheme mode and the scheme in opacity mode, so they are left out without a note.
A scheme with no color ramp (such as Cool) or a fill color with no matching theme color is reported, and the heatmap then uses the default scheme or the first series color.
A time bucket size is read in seconds when it has no unit, a lone bucket value without a mode is taken as a size, and the selection modes `xy` and `y` are reported; hiding a series from the heatmap legend or tooltip imports without a note, because a heatmap draws one field.

A heatmap that Grafana still runs through its own Angular migration (a `dataFormat` with a schema version below 37, or with empty `options`) imports whole: pre-bucketed or calculated rows, bucket bound as the rows layout, bucket sizes and counts, the log base, y axis unit, decimals, range and visibility, color mode, scheme and scale, card padding and rounding, hidden zero buckets, reverse, tooltip decimals as the cell decimals, tooltip and legend; the highlight of the hovered card is not drawn, and a hidden time axis, `splitFactor` and a log base other than 2 and 10 are reported.
A heatmap that already has `options` keeps the Angular keys it was saved with (`dataFormat`, `yAxis`, `color`, `cards`, `tooltip` and the rest) without a note, because Grafana never reads them again; its `options.tooltip.show` decides the tooltip as Grafana's migration does, and a `fieldConfig` color is not drawn, since a heatmap colors its cells from `options.color`.

### Table

A table column imports how its cells are drawn (`custom.cellOptions`, or the older `custom.displayMode`) as the field option `cell`: colored text, a colored background (with `applyToRow` as `wholeRow`), a bar gauge (basic or `lcd`, with its value display), a sparkline (with its line style, fill, points, null spanning and soft axis limits as the column's field options), JSON, an image (with its alt text and title), data links, pills and markdown (with `dynamicHeight`), and `wrapText` as `wrap`.
Gradient backgrounds and gauges, and the sparkline gradient mode, are drawn flat.
Map cells are reported by decision, and the `actions` cell type imports.
Thresholds are kept for the columns whose cells they color.

A table column also imports these options, and the older table panel's column alignment.

| Grafana key | Imports as |
| --- | --- |
| `custom.width` and `custom.minWidth` | Width and minimum width |
| `custom.align` | Alignment |
| `custom.filterable` | Filter |
| `custom.inspect` | Inspect |
| `custom.hideHeader` | Hidden header |
| `custom.wrapHeaderText` | Wrapped header |
| `custom.tooltip` | The cell tooltip, with its field and placement |

A table imports its row height (`sm`, `md`, `lg` and `auto`, which fits each row to its content), the maximum row height (20 to 1000 pixels), the frame it shows (`frameIndex`, 0 to 99), type icons, pagination, frozen columns, the first column of `sortBy` with its direction, and its footer: the reducers (a footer that is shown without a reducer sums), the columns it covers and the row count.
A table panel's `showSearch` and `filterable` keys are not table options in Grafana, and they import without a note.
A second sort column, more than 10 frozen columns and the footer reducers of a single column's override are still reported.

The older table panel's saved sort is dropped silently when it is Grafana's own default (the first column, descending) and reported otherwise, because it names a column by position.
A table with `styles` and no `fieldConfig` is read as Grafana reads it, as the older table panel, so its styles import as column units, decimals, names, alignment, thresholds and hidden columns; a style's `dateFormat` on a column that is not a date and its link options when `link` is off are Grafana's own no-ops and import without a note.
A table that already has a `fieldConfig` keeps the Angular keys it was saved with (`styles`, `transform`, `columns`, `sort`, `showHeader`, `fontSize` and leftover graph options such as `yaxes`, `lines` and `legend`) without a note, because Grafana never reads them again.

On a table, `custom.hidden` (written `true` or `"true"`) and `custom.hideFrom.viz` hide the column, in the panel's defaults or in an override.

### Logs

A logs panel imports its unique and common labels, log details and where they open (`detailsMode`), prettified JSON, deduplication, controls bar, font size and syntax highlighting; the displayed fields (at most 20, shown as the line instead of the message), the log attributes switch (turning it off hides the per-line labels) and the nanosecond timestamp resolution import too; infinite scrolling, the field selector and the context toggle are still reported.

### Traces and flame graph

A traces panel imports its span filters: the search (`spanFilters.query`), the service and span name lists with their `!=` operators, the minimum and maximum duration (`from` and `to`, written with a unit from `ns` to `h`, up to 24 hours), `matchesOnly` and `criticalPathOnly`.
A filter that is not valid, such as a duration without a unit or a list of more than 50 names, and the tag filters are reported.
A flame graph has no options of its own to import.

### Text

A text panel imports its HTML mode and its code mode with the language (JSON, YAML, XML, SQL, Go, TypeScript, Markdown or HTML) and the line numbers; another language is shown as plain text and reported, and the code view has no minimap.
Only people can write HTML text: an agent or the MCP server cannot, and the sanitizer still removes scripts and styles.

### Geomap

A geomap imports its basemap (the CARTO map Grafana draws by default, OpenStreetMap, an Esri server, or an `xyz` tile address that is an https address written out in full with `{z}`, `{x}` and `{y}`), the basemap opacity, theme and labels, the zoom levels an `xyz` tile server serves (tiles are never requested outside them), the view (every named region, the zoom limits, the fit padding, fitting to one layer or only the last points, the shared view, and whether the world is drawn only once), the map controls and the tooltip.
A tile address over http is changed to https and reported, and an address with a variable in the host, a `maplibre` basemap and its access token are reported and never imported.

The mouse wheel zoom is imported as on, because Grafana's default is on.
The map draws tiles only when the server may fetch the basemap's host, so allow it under [Images and GeoJSON](../operate/outbound-access.md#images-and-geojson); until then the map shows country outlines.

A geomap imports every layer, each with its opacity, tooltip and the query that feeds it.

| Layer | Imported options |
| --- | --- |
| Markers | Size, color, fill opacity, outline width, symbol or a symbol from a field with its alignment, text or a fixed label with its offsets and alignment, fixed and clamped rotation, and a WKT location column |
| Heatmap | Radius, blur, and a fixed weight or the weight range of a weight field |
| GeoJSON | The bundled country and state maps, an https address and its rules |
| Day and night | The night color, the sun, and whether the night is drawn at the start or the end of the time range |
| Photos | Border color and size, crop and shadow |
| Network | Node color and edge color |
| Route and tile | The options every layer has |

A markers layer without a fill opacity imports Grafana's own 0.4, so the markers are as light as they are in Grafana.
A markers layer imports its legend, which Grafana shows unless it is turned off: a color swatch under the layer name for a fixed color, one entry per threshold step when the color field has thresholds, else the smallest and largest color values.
The zoom levels of a tile layer are still reported.
Locating points by name uses the bundled country and state gazetteers; an airport lookup, other gazetteers, other GeoJSON files, dynamic GeoJSON, the last-point tracker, photo frames, MapLibre basemaps and the other layer types are reported.

### Canvas

A canvas imports icons (a bundled icon, an https image or an image taken from a field), buttons, server, triangle, cloud, parallelogram, wind turbine and drone elements, the constraint and pixel placement, rotation, vertical alignment, border width and radius, an icon's outline color and width, template text, background images and the background, border, pan and zoom and tooltip of the canvas.
Pixel placement inside a group is still converted to percent of the panel and reported.
A template text shows the values of the element's data, and a reference to a field the data lacks shows as nothing; an outline width that is not a whole number from 0 to 20 is reported.

Element links import with their open-in-new-tab setting, and an element's one-click link or action imports as its one-click behavior.

A connection imports its path, line style, arrow, and a width that follows a field with its range, and its bends (at most 20); widths or directions that follow a rule are reported.

### Canvas actions

A canvas element action and a button's API call import when the address is an https address written out in full; the action needs Edit to run, nothing is sent without a confirmation, and only people can add or change actions (the agent and MCP cannot).
An action with a credential in a header or query (such as `Authorization`, a token or an API key) is kept without that header, and the report says so.
An action whose address is not https, has a variable in the host, refers to a variable the dashboard does not declare, or has a body over 10,000 characters is left out and reported, as is an infinity action.
A canvas action also imports its inputs (`variables`), and its button color (`style`) is reported because a canvas action draws none.

### XY chart

An XY chart imports its series mapping: with `Manual` mapping each series keeps its x, y, size and color fields (chosen by name, and y also by names or a regular expression), its frame or query and its name, and the older `seriesMapping` and `dims` options import the same way.
Point size and color follow the size and color fields, `custom.pointSize.min` and `custom.pointSize.max` set the size range, and the point shape (circle or square), point stroke width, line width, fill opacity, axis options, legend and tooltip options import too; the square line style and custom dash patterns import as on a time series, and an ordinal scale is still reported.

### Node graph

A node graph imports its layout (`layered`, the default, `force` or `grid`) and the greedy zoom mode as mouse wheel zoom.
Its stat units and arcs import as overrides on the main stat, secondary stat and arc fields, and node and edge units that differ are reported.

### Placeholders and special panel types

These Grafana panel types import as follows.

| Grafana panel | Imports as | Notes |
| --- | --- | --- |
| Annotation list, live panel and Alertmanager panel | A placeholder that gives the reason | Annotations, live streaming and alert rules are not part of SlashObserve. |
| `welcome` | A text panel with a one-line note | |
| `gettingstarted` | A text checklist that links these docs | |
| `datagrid` | A table that shows the frame its `selectedSeries` names | It shows the first frame when `selectedSeries` is empty. |
| `debug` | A table over its own targets, or a text panel without targets | The debug modes are not part of SlashObserve. |
| Alert list | One Prometheus `ALERTS` query | A Grafana-managed or legacy alert list becomes a placeholder. |
| Dashboard list | A static panel | It needs no datasource match. |
| News | A news panel with the feed address, changed from http to https and reported | An empty address gets Grafana's blog feed. The feed's domain must be on the [outbound allowlist](../operate/outbound-access.md#news-feeds) before the panel shows items. |

### Legacy panel types

Grafana's renamed and retired panel types import as their modern type, such as `graph`, `table-old` and `singlestat`, and `grafana-worldmap-panel` imports as a geomap, with its circle sizes, thresholds and colors, view and wheel zoom.

A world map whose `locationData` is `countries`, `countries_3letter` or `states` imports as a markers layer that looks places up by name in the countries or US states gazetteer, and a `valueName` other than `current` becomes a `reduce` transform of the queries, which a table-format query makes unnecessary because its rows already hold one value each.
On a world map with table data the `valueName` has no effect, as in Grafana, and the keys Grafana's own world map migration does not read (`esMetric`, `hideEmpty`, `hideZero`, `stickyLabels`, the legend, the unit words and the label field) import without a note.

Graph options that Grafana's own time series migration drops, such as `paceLength` and the tooltip's millisecond resolution, thresholds without a value and `percentage` without stacking import without a note, and a legacy graph alert is excluded with alert rules.
A legacy graph's threshold fill and line choose the threshold style the way Grafana's migration does (lines, areas, or both), and `dashLength` and `spaceLength` set the pattern of a dashed line.

A legacy singlestat imports without a note for the keys Grafana's own singlestat-to-stat migration drops: the prefix and postfix colors, the font sizes, the sparkline fill and full width, the mapping editor choices, a prefix or postfix beside a unit (the unit is kept, as in Grafana) and a null mode, except that a connected null mode turns the first value into the first value that is not null.
The sparkline line color imports without a note too, because the sparkline is drawn in the series color.

## Transformations

Transformations import in order, up to 20 per panel, and a disabled one is skipped.
These import: `merge`, `seriesToRows`, `organize` (and the older `rename`, `order` and `filterFields`), `reduce`, `joinByField` (and `seriesToColumns`), `filterFieldsByName`, `calculateField`, `sortBy`, `limit`, `convertFieldType`, `formatString`, `formatTime`, `renameByRegex`, `filterByValue`, `filterByRefId`, `extractFields`, `labelsToFields`, `joinByLabels`, `partitionByValues`, `prepareTimeSeries`, `timeSeriesTable`, `rowsToFields`, `configFromData`, `concatenate`, `groupBy`, `groupToNestedTable`, `groupingToMatrix`, `histogram`, `regression`, `transpose`, `heatmap`, `spatial` and `fieldLookup` (see [Field options, overrides and transforms](../concepts/observespec.md#field-options-overrides-and-transforms)).
Any other transformation, one past the 20th, and an option the Spec cannot hold are reported at their path.

### Grouping steps and calculateField

The grouping steps keep only their `by` and `aggregate` fields, as Grafana's output does, and take every reducer, including the true or false and list reducers (`allIsNull`, `allIsZero`, `allValues` and `uniqueValues`); a reducer name Grafana does not have is reported.

`calculateField` imports its binary, row reduce (including its list of fields), unary, index, cumulative and window modes; the unary `percent`, a row reduce `nullValueMode` and `timeSeries: false` are reported.
A binary calculation over field names with spaces imports (a name that holds a spaced operator, or a matcher other than `byName`, is reported), and a `convertFieldType` with no conversions imports as nothing, since Grafana changes no field either.

### Limiting a transformation to some queries

A transformation that Grafana limits to some queries (a `filter` of `byRefId`) imports as `queries`, matched against the refs the panel imported: one ref, refs joined with `|`, a list or a `/regex/` pattern.
A frame filter (`byFrameRefID`) imports the same way.
A ref that did not import, a pattern that matches none and any other filter are reported at `filter`, and the transformation then applies to every query.
`filterByRefId` imports `include` and `exclude`, `filterByValue` imports `between`, `substring` and, as a transformation's only filter, `notSubstring`, and `formatTime` imports the `utc` time zone; any other time zone is reported.

### Heatmap, spatial and lookups

The `heatmap` transformation imports its x and y buckets, the log, symlog or ordinal row scale, the log or symlog time scale and `keepOriginalData`, and a heatmap panel draws its cells directly; an ordinal time scale, a log base other than 2 and 10, and `rowsFrame` are reported.
`spatial` imports its action (prepare, calculate or modify), its source and target locations (automatic, coordinates, geohash or lookup) and the line builder.
A lookup in `spatial` or `fieldLookup` uses the built-in countries or US states gazetteer; an airports gazetteer or one from a URL drops the transformation and is reported ("Airport lookup is not available." or "Only the built-in countries and US states gazetteers are supported.").

## Options that are not imported

Some Grafana options are reported and not imported by decision, because SlashObserve does not have the feature:

- Grafana services and links: Grafana scopes on Prometheus queries, images served by a Grafana server, the Infinity datasource (canvas actions and field actions), Explore links, editing data from a panel, CSS taken from a field, recently viewed dashboards and Grafana folder ids in dashboard lists, alert rules in alert lists (inactive alerts, instances, the normal state filter, alert grouping), and the modes and counters of the debug panel.
- Altinity queries and ClickHouse: Grafana Live streaming on Altinity queries, the log context window of Altinity queries, hidden log labels in Altinity logs settings, a group-by variable on ClickHouse, and the Altinity ad hoc values query.
- Logs panels: the log context toggle, infinite log scrolling and the log field selector.
- Axes and scales: vertical value axes placed at the top or bottom and ordinal value scales on time series, trend, candlestick, histogram, bar chart and XY chart panels, and the time comparison and horizontal orientation of time series.
- Gauges, heatmaps and tables: bar gauge legends (turning one off imports without a note), the horizontal and vertical orientation of a gauge (a bar gauge imports its orientation), the heatmap selection modes `xy` and `y`, map cells in tables, and the `anyMatch`, `allMatch` and `invertMatch` override matchers and the `byType` matcher for other fields.
- Canvas: server element animation (blink rate and bulb color), data-driven canvas connections, canvas zoom to content, the one-click canvas tooltip and the canvas background image mapping mode.
- Loki: Grafana Live streaming and the line limit of Loki queries (a logs panel shows up to the datasource's row limit), and reading Loki lines oldest first.
- Tempo: the search builder, the service graph, uploaded trace files, TraceQL metrics queries, and the trace limit, span-set limit and span tables of Tempo queries (a search lists one row per trace).
- Elasticsearch and OpenSearch: PPL queries (rewrite them in KQL or Query DSL), `geohash_grid` and `nested` bucket aggregations, trimmed histogram edges, the `top_metrics` metric, inline scripts on a metric, hiding one metric of a query, and field-name variables (`{"find": "fields"}`), which have no equivalent.
- Transformation options: the annotation and time region frame conversions, the exemplar frame conversion, the percent unary operator, the max window reducer, enum and time zone conversions of field types, ordering by label or in descending name order and by mode `auto`, the semicolon and pipe separators and names of partition by values, the ignore and threshold modes of rows to fields with its name and value fields, and the inline statistic of time series table.

The import report marks these options as excluded by decision, apart from options that are not supported yet.
A panel whose only report lines are such decisions counts as imported, and its report line still lists them.

### Imported without a note

Options that Grafana itself does not draw or read where they are written (the legend options of a stat panel, a log base on a query, the unit scale of a field, a Prometheus exemplar flag on an instant query or on a panel that draws no exemplars), editor state such as a query's help toggle or saved ad hoc filters, and inputs Grafana rejects are imported without a note.
A query with no text is dropped without a note, because Grafana sends no request for it and the panel shows the same data without it; a panel left with no query becomes a placeholder.
