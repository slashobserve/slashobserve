---
title: Import Grafana dashboards
description: Import a Grafana dashboard from the web page, the CLI or MCP, and how datasources and folders are matched.
---

You can bring a Grafana dashboard in three ways: the web page Import from Grafana, `observe import`, and the `import_grafana_dashboard` MCP tool.
The page and `observe import` call `POST /api/v1/imports` with the dashboard JSON as pasted or uploaded, the tool runs the same import, and `POST /api/v1/imports/preview` shows the result first without saving anything.
The JSON can be the classic dashboard model, an export that shares a dashboard externally, or the response of Grafana's dashboard API.

Each datasource the dashboard uses is matched to one of yours, in the order described under `grafanaUids` in the [datasources file](../reference/datasources-file.md).
An import cannot be saved while a datasource has no match, and SlashObserve never guesses between two datasources of the same type; give the match with the mapping that comes with the request, or add the Grafana uid to `grafanaUids`.

The dashboard goes into a folder, chosen in this order:

1. The request's `folder` title: `--folder` for `observe import`, `folder` for the API and the MCP tool.
   The web page sends the folder you pick in its **Folder** card, and sends none when the choice is **Automatic**.
2. The `meta.folderTitle` of an export taken from Grafana's dashboard API, which names the folder the dashboard had in Grafana.
3. Your default folder: the folder you last created a dashboard in, else General.

A title matches an existing folder without regard to case.
A title of `General`, or an export title that is empty or not a valid folder title, means the default; an invalid `folder` in the request is refused with `400 invalid_request`.
When no folder has the title, an Editor or Admin creates it.
When a folder has the title but you cannot create dashboards in it, the dashboard goes to your default folder instead, and the response carries a `folder_note` saying so (`observe import` prints it on standard error, even with `-q`).
The **Folder** card on the web page lists the folders where you may create dashboards, except General, because a title of `General` means your default folder, which **Automatic** already gives you.
After an import the page says "In folder `<title>`" and shows the `folder_note` when the dashboard did not go where the request or the export asked.
The folder is chosen before the dashboard is saved, so an import that then fails can leave a new, empty folder behind; delete it from its settings page.

The request body, with the dashboard JSON as a string, can be at most 8 MiB.
It saves the dashboard as version 1 and runs every query once.
Failing queries mark their panels and variables unverified in the import report instead of failing the import.
Later saves run every query too, but keep a failing panel or variable unverified, instead of failing, for as long as its queries and the variables they use are unchanged; a new or changed query must return data, and a panel whose queries pass again is no longer unverified.
The original JSON is stored with the import so a newer importer can convert it again, and it is never shown to the agent.

A query's min step written as a variable (`$name`, `${name}` or `[[name]]`) imports as `${name}`, so the query uses the step selected in that variable.
A `custom` variable of durations that is single-select, has no All and is used as a min step becomes an `interval` variable, and the import report says so.
A `custom` variable's `text : value` pairs import as `{ text, value }` options, so the picker shows the text and the selection uses the value.
A `query` variable's sort imports as `alpha`, `num`, `alphaCaseInsensitive` or `natural`, each in its two directions, and `allowCustomValue` imports as written, with Grafana's own default of true when it is missing.
A saved ad hoc filter list imports as the variable's `default` when its operators are `=`, `!=`, `=~` and `!~`; a filter with another operator, an invalid key or regular expression, or a condition other than AND is reported, and base filters are not kept.
A ClickHouse ad hoc variable imports with `applyToAll`, so its filters apply to every query on its datasource, and for the Altinity plugin the report notes that Grafana filters only the outermost query.
A min step of `$__interval` or `$__rate_interval` imports as no minimum, and a resolution factor from 2 to 10 is kept as `intervalFactor`.
A Prometheus query that Grafana runs as both an instant and a range query (`instant` and `range` both on) imports as two queries: the range query keeps its ref, and an `instant` copy without the min step or resolution factor takes the next free ref, which the import report lists as `refId A (instant) -> B`.
Both queries are hidden together, and a style override or a `filterByRefId` transformation that names the original ref covers the copy too.
A query with `instant` on and `range` unset or off stays one instant query.
A panel that already holds 26 queries keeps only the range query and reports the instant one.
A panel with no title imports with an internal name (`Panel <id>`) that menus and screen readers use but that is not drawn, as in Grafana, and a transparent panel is drawn without a border or background.
A dashboard's time settings carry over to the [dashboard fields](../concepts/observespec.md#the-dashboard) `timezone`, `weekStart`, `fiscalYearStartMonth` and `nowDelay` (from the time picker's now delay), and its time range may use `now/fy` and `now/fQ`.
The time picker's hidden flag, refresh intervals and quick ranges, the live now switch, refresh intervals in days, and a time range written in epoch milliseconds carry over too (a refresh interval of `500ms` is left out and reported).
Links to other dashboards by tag import as links that list the matching dashboards, and links to an address keep their keep-time and include-variables options.
A unit imports as itself, and unit text Grafana does not know imports as the suffix Grafana shows, up to 20 characters.
A stat, gauge, bar gauge or pie chart imports its value options: the first reducer, "Show" as every value (`allValues`, with `limit` when it is not 5000) and the `fields` name or pattern.
Grafana uses only the first reducer of these panels, so extra reducers are dropped without a note.
A pie chart also imports its slice labels (name, value and percent), their text sizes (`text.titleSize` and `text.valueSize`, from 1 to 200 pixels), its legend and its tooltip.
The legend imports its placement, list or table mode, the values it shows (value and percent), the sort by name or value, the sort order and the width.
A legend sorted by a calculation such as Mean is still reported, and so are the pie chart's sort order, orientation and legend calculations.
The tooltip imports its mode, sort, "Hide zeros" and maximum width and height.
A series hidden from the legend or the tooltip in the field options imports on a pie chart as it does on a time series, and stays drawn.
A bar chart imports its label rotation, spacing and maximum length (`xTickLabelRotation`, `xTickLabelSpacing` and `xTickLabelMaxLength`), `barRadius` (up to 0.5), `fullHighlight`, `colorByField`, the value text size (`text.valueSize`, 6 to 100 pixels), and its legend and tooltip like a time series does, including the table mode, legend calculations, sort, width and the tooltip sort, "Hide zeros" and maximum sizes.
A histogram imports `bucketOffset`, `combine`, its stacking mode (normal, percent or none), and the same legend and tooltip options; Grafana's own `none` stacking and `single` tooltip are written explicitly, because a histogram without them stacks series and lists all of them.
An XY chart imports its series mapping: with `Manual` mapping each series keeps its x, y, size and color fields (chosen by name, and y also by names or a regular expression), its frame or query and its name, and the older `seriesMapping` and `dims` options import the same way.
Point size and color follow the size and color fields, `custom.pointSize.min` and `custom.pointSize.max` set the size range, and the point shape (circle or square), point stroke width, line width, fill opacity, axis options, legend and tooltip options import too; line dash patterns, the square line style and an ordinal scale are still reported.
On both, the axis options (label, soft limits, log and symlog scales, centered zero, width, grid, border and color mode) apply to the value axis, `lineWidth` is the bar border and `fillOpacity` the bar fill, and a series hidden from the legend or the tooltip in the field options imports and stays drawn.
A fully transparent fill is reported because bars are drawn opaque at 0, and a legend calculation SlashObserve does not have, `thresholdsStyle` and the bottom or top axis placement are still reported.
A heatmap imports its color mode, scheme, steps, scale and exponent, fill, range (`color.min` and `color.max`) and reverse, the cell gap and radius, the cell value unit and decimals, the legend toggle, the low and high value filters (`filterValues.le` and `ge`) and `showValue`, and it writes Grafana's own defaults when the panel does not set them (scheme mode with the Oranges scheme, 64 steps, a gap of 1 and values shown where they fit).
Grafana ignores the scale, exponent and fill in scheme mode and the scheme in opacity mode, so they are left out without a note.
A scheme with no color ramp (such as Warm) or a fill color with no matching theme color is reported, and the heatmap then uses the default scheme or the first series color.
A stat panel imports its text alignment (`justifyMode`), title and value sizes (`text.titleSize` and `text.valueSize`, from 6 to 200 pixels), percent change (`showPercentChange` and `percentChangeColorMode`) and `wideLayout`; a size outside that range is reported, and the percent size (`text.percentSize`) is still reported.
A gauge imports its threshold labels and a bar gauge its `lcd` display mode, value mode, name placement and unfilled area; both import their text sizes (`text.titleSize` and `text.valueSize`, from 1 to 200 pixels).
The `basic` and `gradient` bar gauge display modes import as the basic bar, because bar gauges are drawn flat.
When the sizing is Manual, a gauge or bar gauge imports its minimum width and height (and a bar gauge its maximum height), using Grafana's own defaults when they are not set; with Auto sizing Grafana ignores these limits, so they are left out without a note.
A table column imports how its cells are drawn (`custom.cellOptions`, or the older `custom.displayMode`) as the field option `cell`: colored text, a colored background (with `applyToRow` as `wholeRow`), a bar gauge (basic or `lcd`, with its value display), a sparkline (with its line style, fill, points, null spanning and soft axis limits as the column's field options), JSON, an image (with its alt text and title), data links and pills, and `wrapText` as `wrap`.
Gradient backgrounds and gauges, and the sparkline gradient mode, are drawn flat.
Cell actions and the markdown and geo cell types are still reported.
Thresholds are kept for the columns whose cells they color.
A table column also imports its width and minimum width, alignment, filter, inspect and hidden header (`custom.width`, `custom.minWidth`, `custom.align`, `custom.filterable`, `custom.inspect` and `custom.hideHeader`), and the older table panel's column alignment.
A table imports its row height (`sm`, `md` and `lg`; `auto` is reported), type icons, pagination, frozen columns, the first column of `sortBy` with its direction, and its footer: the reducers (a footer that is shown without a reducer sums), the columns it covers and the row count.
A second sort column, more than 10 frozen columns, the per-column footer reducers, the cell tooltip, the wrapped header, the shown frame and the maximum row height are still reported.
The older table panel's saved sort is dropped silently when it is Grafana's own default (the first column, descending) and reported otherwise, because it names a column by position.
A field's `fieldMinMax` imports on gauges and bar gauges, where fields otherwise share one range, and is left out silently elsewhere, where every field already has its own range.
A display name keeps `${__field.name}`, `${__field.displayName}`, `${__series.name}`, `${__data.refId}`, `${__field.labels.<label>}` and dashboard variables in all four syntaxes; other `__` tokens, such as `${__data.fields[0]}`, are reported.
A data link's `oneClick` imports on stat, gauge, bar gauge and table panels and is reported on every other type, where the link stays a panel link.
A logs panel imports its unique and common labels, log details and where they open (`detailsMode`), prettified JSON, deduplication, controls bar, font size and syntax highlighting; infinite scrolling, the field selector, the context toggle, the timestamp resolution, the log attributes switch and the displayed fields are still reported.
A value SlashObserve does not support, such as a delay in days or a time zone name the server does not know, is left out and listed in the import report.
A ClickHouse query from the Altinity plugin that starts with a whole-query macro (`$rate`, `$perSecond`, `$delta`, `$increase`, `$columns`, their `Columns`, `Ms` and `ColumnsAggregated` forms, and `$lttb`) is written out as plain ClickHouse SQL that uses SlashObserve's time macros, so the saved query no longer needs the Altinity macros.
The `Columns` forms return one row per time and key (time, key, value), which SlashObserve draws as one series per key, named by the key.
Counter macros always use window functions (`lagInFrame`), whatever the plugin's `useWindowFuncForMacros` says, because `runningDifference` is deprecated in current ClickHouse versions.
The first point of a counter series has nothing to subtract from: a rate treats the previous point as one step earlier, a delta or increase is 0, and a per-second value is empty.
A macro that is not at the start of the query, or whose arguments cannot be read, is still left out and listed in the import report.
`$adhoc` becomes the dashboard's ad hoc variable when exactly one ad hoc variable uses the query's datasource, and `1` (with a note in the report) otherwise.
`$timeFilterByColumn(column)` becomes `$__timeFilter(column)`.
The plugin's `extrapolate` option, which the plugin applies in the browser to the edges of a rate series, is reported on the rate family; `round` is dropped silently only when it is `$step`, `0s` or divides the query's explicit minimum step, and reported otherwise.
A ClickHouse query that keeps its generated SQL drops the query builder's settings without a note.
