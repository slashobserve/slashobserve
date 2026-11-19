---
title: Edit a panel
description: Change a panel's queries, settings or JSON in the panel editor and save the result as a new version.
---

The panel editor changes one panel by hand and saves the dashboard as a new version.
Use it when you know the change you want, or when the agent's panel is close but not right; for a broader change, ask the agent as described in [Chat and follow-ups](chat.md).

What you edit is a draft.
Nothing changes for anyone until you save.

## Open the editor

Open the menu of a panel ("Panel menu: `<title>`") and choose **Edit**.
The **Activity** card turns into the editor, headed "Edit `<panel title>`" with the line "A draft. Nothing changes until you save."
The address gains `panel=<id>` and `edit=query`, so you can bookmark it, and **Close panel** or **Back to activity** returns to the timeline.
When the dashboard and the editor do not fit side by side, the editor opens as a sheet over the bottom of the screen, and on a phone the preview is shorter so the query has room.

**Edit** is missing from the menu, or the editor is read-only, when:

- you do not have Edit on the dashboard's folder;
- you are on a pinned older version, where the editor says "Open the latest version to edit this panel." with an **Open latest version** button;
- the panel is a row or a placeholder, which says "This panel cannot be edited.".

Data panels, text panels, dashboard lists and news panels can be edited.
Pressing Escape, the browser's Back or Forward, or leaving the page, with unsaved changes asks "Discard your changes?" with **Keep editing** and **Discard**.
**Keep editing** keeps the draft and its address.
An editor opened from a bookmark or a pasted link adds the dashboard without the editor behind it in the browser history, so Back asks the same question instead of leaving the app.
If your session ends while you have unsaved changes, for example another tab signed you out, the editor keeps them in this tab's session storage.
After you sign in and reopen the panel, "Your unsaved changes were kept." offers **Restore my changes**, which lays them over the panel as it is now, and **Discard them**; nothing is kept when you close the editor in any other way, and nothing reaches the server.

The **Editor view** switch at the top changes between **Editor** and **JSON**.

## Add a panel

On a dashboard, open **More** and choose **Add panel**.
The editor opens headed "Add a panel" with the line "A new panel. Nothing changes until you save.", on a time series panel titled "New panel" below the last panel, using the datasource of the dashboard's first data panel.
Pick the type, datasource and query as for any panel; the save adds the panel and its summary is `Add "<title>"` unless you add a note.
**Add panel** needs Edit on the dashboard, is missing when no datasource is configured, and is disabled while you are on a pinned older version or have unsaved layout moves.

## Start from a blank dashboard

On the Dashboards page, **Create dashboard** offers **Ask the agent**, which opens the chat and is marked "needs a model" when the server has no model provider, and **Blank dashboard**.
**Blank dashboard** asks for a title and a folder, starting in My Dashboards, and creates a dashboard holding one text panel that says how to add panels.
Remove that note from its panel menu once you have added your own panels.

## Edit queries

For a data panel, the **Preview** section sits at the top and the query rows below it.

**Datasource** picks the default datasource of the panel, from your configured datasources or from a datasource variable the dashboard defines.
Each query row can use another datasource through its own menu.

A row is "Query A", "Query B" and so on, up to 26.
Type the query in the editor; a query is at most 20000 characters.
The server lints the query, and each problem shows once, under the line that has it, with a marker in the gutter.
A screen reader hears it as "Error: `<message>` (line 2, column 14)", or "Warning: ..." for a lesser one.
A clean query shows "Syntax is valid.": the lint checks only how the query is written, and the preview is what runs it.
"Lint is unavailable right now." means the check did not run, not that the query is wrong.

A Prometheus range query also has an **Exemplars** checkbox; when it is on, a time series or a heatmap marks the exemplars Prometheus stored for the query as diamonds on the chart, and hovering one lists its labels (a trace id first) in the tooltip.
**Legend** sets the name of each series (up to 200 characters), and **Add query** adds a row.
**Remove query A** appears when the panel has more than one query.
Rows carry a "New" or "Changed" marker against the saved version.

A query that is an expression shows as "Expression B", with its kind (math, reduce, resample or threshold), "hidden" when it is, and a one-line summary such as the math text.
The editor does not change an expression: edit it in the JSON view, or remove it with **Remove expression B**.
A panel that reuses the results of another panel shows "Reused panel A" with "Results of `<title>`", or "Results of `<title>`, after its transforms".
That row is read-only, and **Add query** is disabled on such a panel.
See [Expressions](../concepts/observespec.md#expressions) for what an expression does.

**Metrics and labels** opens a browser for the datasource of the active query.
Search a metric (or on ClickHouse, a table), select it to see its labels, then a label to see its values.
**Insert metric** or **Insert table** puts the name in the query, and choosing a value inserts a matcher at the cursor.
It lists the first 100 names, and "Showing the first 100. Type to narrow." tells you to search.
If the names cannot be loaded, it says "Could not load names." with **Retry**.

### Live preview

The preview runs the draft on the real datasource shortly after you stop typing, with the time range and variable selections the dashboard page has right now.
It draws the panel in the type you chose, and reports its state:

- "The draft is not valid yet." while a query has an error;
- "Waiting for variables." until the variables the queries use have values;
- "Running the draft.";
- "Draft ran `<time>`, 3 series" when it finished;
- "The draft did not run." in place of the chart, with the reason under it, such as "The datasource is not answering."; when the lint already shows a query's parse error under its line, the preview does not repeat it.

When the draft's queries or datasource differ from the saved panel, a dotted line labelled "Saved in v3" shows the saved panel's data next to yours.

A preview is limited like the agent's draft queries: 15 seconds, 200 series, 100,000 points and 1,000 rows, and it is never cached.
It runs for a signed-in person who can comment on the dashboard, so a Commenter sees it too while suggesting an edit, and not for service-account keys.
If the datasource is slow or busy, the preview says so ("The datasource is not answering.", "Too many requests right now. The preview runs again in a moment.", or "Query A failed: it took too long to run.") and the draft stays as it is.
When the draft itself is at fault, such as a query that does not parse, the footer reads "Fix the query to save." and the save button is disabled until the preview runs again; a slow or unreachable datasource does not disable it.

### Ask the agent for a query

Below the preview, **Ask the agent** has a box "Ask the agent to change this query", for example "only 5xx, by route", and a **Suggest** button.
The agent reads the panel and proposes a change, shown as "Proposed change" with a diff of the queries.
**Accept** applies it to the draft ("Applied the agent's change. Save to keep it."), **Discard** drops it and **Try again** asks for another.
A suggestion never saves anything.
The box is hidden when the server has no model, and unavailable on an older version, or while a save is running.

## Panel settings

The settings column has:

- **Panel type**, which can be Time series, Stat, Gauge, Bar gauge, Table, Pie chart, Heatmap, Histogram, State timeline, Logs, Bar chart, Status history, Trend, XY chart, Candlestick, Node graph, Traces, Flame graph, Geomap, Canvas or Alert list.
  Switching type keeps the queries, redraws the preview and drops display options the new type does not have.
- **Unit**, from the groups it lists, or custom text: a prefix, a suffix, a date format, an SI symbol, a count text or a currency symbol.
- **Thresholds**, up to 10 rows, each with a colour (OK, Info, Warning, Critical, Text for the theme's text colour, or a custom hex colour) and the value it starts from; the first row can leave the value empty ("Base").
- **Title**, up to 200 characters.
- The display options that belong to the chosen type, listed under [Display options by panel type](#display-options-by-panel-type).
- The field and panel options that the chosen type draws, in groups you can open and close, listed under [Field and panel options](#field-and-panel-options).

### Display options by panel type

- For Time series and Trend panels the legend options are **Legend** (bottom, right or hidden), **Legend values**, **Legend mode** (List or Table, "Table when values are shown" until you choose), **Sort legend by** (Name or one of the legend values, "Series order" until you choose), **Sort descending**, **Legend width** (pixels, for a legend on the right), **Hide series that are all zero** and **Hide series with no values**.
  A series hidden from the legend is still drawn.
- Time series, Trend and Candlestick panels also have **Tooltip sort** (Series order, Ascending or Descending, which is the default), **Hide zero values in the tooltip**, **Show stacked totals in the tooltip** and **Tooltip max width** and **Tooltip max height** (pixels, 100 to 2000); the sort, zero and total options apply to a tooltip that lists all series.
- For Stat, Gauge, Bar gauge and Pie chart panels, **Calculation** picks how each field is reduced to one value.
  **Calculation**, **Legend values**, **Sort legend by** and a table's **Footer** list the common reducers, and a **More** search box below each one finds any other reducer by name or id; **Legend values** and **Footer** call the last non-empty value "Last (not null)" and the last value, empty or not, "Last".
  **Show every value** shows one value per row instead, **Row limit** (1 to 5000) caps the rows shown per field, and **Fields** shows a field by name or by a pattern such as `/^cpu/` instead of every number field.
- A Stat panel also has **Color mode** (Value, Background or None), **Show graph** with its **Graph style** (Area or Line), **Text** (Auto, Value, Name, Value and name or None), **Alignment** (Auto or Center), **Title size** and **Value size** (pixels from 1 to 200, empty for the default), **Show percent change** with its **Percent change size** (pixels from 1 to 200) and **Percent change color** (Standard, Inverted or Same as value, applied to the arrow only), and **Wide layout**, which puts the name beside the value on wide tiles.
- Pie chart panels add **Pie type**, **Slice order** (Largest first, Smallest first or Data order), **Slice labels** (Name, Value and Percent, written on the slices that are big enough), **Label name size** and **Label value size** (pixels, empty for 12), and the legend and tooltip options: **Legend** (bottom, right or hidden), **Legend values** (Value and Percent, both until you choose), **Legend mode** (List or Table), **Sort legend by** (Name or Value, "Series order" until you choose), **Sort descending** and **Legend width**.
  Clicking an entry in a pie chart's legend hides its slice, and the rest share the circle.
  A pie chart's tooltip has **Tooltip** (the hovered slice, all series or hidden), **Tooltip sort**, **Hide zero values in the tooltip**, **Tooltip max width** and **Tooltip max height**; the sort and zero options apply to a tooltip that lists all series.
- Bar chart panels add **Color by field** (colors each category by that number field's thresholds), **Value text size** (6 to 100 pixels, empty for 12), **Bar radius** (0 to 0.5 of the bar width), **Highlight the whole category**, **X label rotation** (-90 to 90 degrees), **X label spacing** (the minimum pixels between labels, 0 for automatic) and **X label max length** (characters before an ellipsis), next to the legend and tooltip options.
- Histogram panels add **Bucket offset**, **Stacking** (Stacked, Percent or Overlaid) and **Combine all series**, next to the bucket size, bucket count, legend and tooltip options.
- A bar chart or histogram tooltip shows the hovered series or all series, in the order of **Tooltip sort**; **Hide zero values in the tooltip** is on by default for a histogram.
  Clicking an entry in a bar chart or histogram legend hides its series, and the rest are redrawn.
- XY chart panels add **Smallest point size** and **Largest point size**, which a series with a size field scales between, and the same legend and tooltip options as a histogram; the series mapping (the x, y, size and color fields and the name of each series) is edited in the JSON view.
  An XY chart draws a point in the color of its color field when the series has one, clicking a legend entry hides the series, and the tooltip lists the size and color values of the hovered point.
- Gauge panels add **Show threshold labels**, **Minimum gauge width**, **Minimum gauge height**, **Title size** and **Value size**.
- Bar gauge panels add **Bar style** (Basic or LCD), **Value** (With threshold state, Plain text or Hidden), **Name placement**, **Show unfilled area**, **Minimum bar width**, **Minimum bar height**, **Maximum bar height**, **Title size** and **Value size**; sizes are in pixels.
- Traces panels add **Search**, **Services** and **Operations** (comma-separated names, each with an **Exclude** switch), **Shortest span** and **Longest span** (milliseconds), **Show matching spans only** and **Show the critical path only**; without the matching switch the spans that do not match are dimmed.
- Logs panels add **Show unique labels**, **Show common labels**, **Log details**, **Details placement** (under the line or in a side panel), **Prettify JSON messages**, **Deduplication**, **Controls bar**, **Font size** and **Highlight key=value and levels**.
  They also have **Show line attributes**, which hides the label chips on each line when it is off, **Line fields** (up to 20 field names that build the line instead of the message) and **Time precision** (Milliseconds or Nanoseconds).
- Table panels have **Show header**, **Row height** (Small, Medium, Large or Fit content), **Maximum row height** (20 to 1000 pixels, for rows that fit their content), **Frame** (0 to 99, to show only that frame when the data has several), **Show column type icons**, **Paginate rows**, **Frozen columns** (0 to 10), **Sort by** (a column name), **Sort descending**, **Footer** (up to 5 calculations), **Footer columns** (up to 50 names) and **Footer row count**.
  Column settings such as width and alignment are under **Table columns**; the cell type is edited in the JSON view.
- Heatmap panels have **Calculate buckets** with **Time buckets** and **Value buckets** (each with a scale and a log base), **Pre-bucketed rows** with a **Pre-bucketed value field** for data that is already bucketed, and **Y scale**; **Color mode** (Opacity or Scheme) with **Color scheme**, **Opacity color**, **Color steps**, **Opacity scale**, **Opacity exponent**, **Color range start**, **Color range end** and **Reverse colors**; **Cell gap**, **Cell radius**, **Cell value unit** and **Cell value decimals**; **Color legend**; **Hide cells at or below** and **Hide cells at or above**; **Y axis** (Left, Right or Hidden) with its label, width, unit, decimals, minimum and maximum, and **Reverse Y axis**; **Tooltip** (Hovered cell, Every row of the column or Hidden) with **Tooltip histogram**, **Tooltip color scale** and the max width and height; and **Show exemplars** with **Exemplar color**.
- State timeline panels have **Merge equal values**, **Show values** (Auto, Always or Never), **Row height** (a share of the row, 0.1 to 1), **Rows per page** (1 to 100, empty for every row), **Align values** (Left, Center or Right), **Legend**, **Legend mode**, **Legend width**, **Tooltip**, **Tooltip sort**, **Hide zero values in the tooltip** and the tooltip max width and height.
- Status history panels have the same, with **Column width** (a share of the cell, 0.1 to 1) in place of **Merge equal values** and **Align values**.
- Candlestick panels have **Mode** (Candles and volume, Candles or Volume), **Candle style** (Candles or OHLC bars), **Color strategy** (Open to close or Close to close), **Up color**, **Down color**, **Flat color**, the **Open**, **High**, **Low**, **Close** and **Volume field** names, **Draw other fields as lines**, **X axis**, and the legend and tooltip options of a time series, except hiding all-zero and empty series.
- Node graph panels have **Layout** (Layered, Force or Grid) and **Zoom with the scroll wheel**, and flame graph panels have no display options.
- Geomap panels have **View** (Fit to data, World, Coordinates or a region), with **Latitude**, **Longitude** and **Zoom** for Coordinates; **Location** (Auto, Coordinates or Geohash) with the latitude, longitude and geohash field names; **Size field**, **Color field**, **Marker size** and **Largest marker size**; **Basemap** (Outline, CARTO, Esri, OpenStreetMap, Custom XYZ or None) with the settings of the one you pick, such as **CARTO theme**, **Esri map**, **Custom XYZ URL**, **Basemap attribution**, **Basemap opacity**, **Lowest tile zoom** and **Highest tile zoom**; **Draw the world once**; **Lowest zoom**, **Highest zoom** and **Fit padding**; the controls **Show zoom buttons**, **Zoom with the mouse wheel**, **Show attribution**, **Show scale bar**, **Show measure tool** and **Show zoom and center**; and **Tooltip** (Details or None).
  The editor has no layer settings; edit `layers` in the JSON view.
- Canvas panels have **Calculation**, **Background color**, **Border color**, **Border width** (0 to 20 pixels), **Border radius** (0 to 100 pixels), **Pan and zoom**, **Pan beyond the canvas** and **Tooltip** (Details or None).
- Alert list panels have **Limit** (1 to 100), **Sort** (A to Z, Z to A, Importance, Oldest first or Newest first) and **View** (List or Count).

### Field and panel options

Below the display options the column lists the ObserveSpec field and panel options that the chosen panel type draws, grouped as the Spec groups them.
Each group shows how many of its options are set, and only **Standard options** starts open.
An empty field shows the Spec default, or "Auto" when there is none, and a choice list starts on "Default", naming the default value when the Spec has one.
Choosing the default value removes the option from the panel, so the saved JSON holds only what you changed.
A number or text that the Spec would refuse is marked with what it accepts, such as "A whole number from 0 to 10.", and is not applied until it fits.

- **Standard options**: **Decimals**, **Min**, **Max**, **Display name**, **No value text**, **Color mode**, **Color scheme**, **Color** and **Threshold mode**, for every type that draws field values; heatmaps leave out the color options, which their own color settings replace, and flame graphs have only **Decimals**.
- **Range**: **Range each field by its own values** for gauges and bar gauges, and **Neutral value** for gauges.
- **Series**: draw style, line width, interpolation and style, fill opacity, points and point size, stacking and its group, value labels, line, fill and point colors, bar alignment, width factor and max width, **Show thresholds**, **Transform**, **Fill below to**, **Treat nulls as zero** and the three hide options, for Time series, Trend and Candlestick panels; Time series also has **Color by value**.
- **Axis**: placement, label, soft min and max, scale, log base, linear threshold, centered zero, width, grid lines, border and color, for Time series, Trend, Candlestick, Bar chart, Histogram and XY chart panels.
- **Bars** for Bar chart and Histogram panels, **Cells** for State timeline and Status history panels, and **Points and lines** for XY chart panels hold the fill, line width, stacking, point and hide options those types draw.
- **Series visibility**: the hide options for pie charts, and **Hide in chart** for heatmaps, geomaps and canvases.
- **Table columns**: column width and min width, cell alignment, column filter, cell inspect, hide and wrap header, field description and the cell tooltip field and placement.
- **Panel**, for every data panel: **Description**, **Transparent background**, **Hide title**, **Relative time** (like `now-1h`), **Time shift** (like `1h`), **Hide time info**, **Min interval** and **Max data points**; Time series, State timeline and Status history panels add **Time zone**.
- **Repeat**: **Repeat for variable**, **Repeat direction** and **Max per row**.

Value mappings, links, actions, cell types, dash patterns, gap and null handling, overrides and transformations are edited in the JSON view; **Edit in JSON** under the groups opens it.

### Text and dashboard list panels

A text panel has a **Content** box instead of queries: Markdown up to 20000 characters, which may contain HTML.
The box notes that HTML is cleaned: scripts, styles, forms and embeds are removed.
The panel's `mode` is set in the JSON view (see [Panels and layout](../concepts/observespec.md#panels-and-layout)): `markdown` is the default, `html` reads the content as HTML only, and `code` shows it as code with a `code.language` and optional line numbers.
Only people and Grafana imports can write HTML; the agent and MCP cannot, so an agent request can leave an HTML panel as it is but not change its text.
A dashboard list panel has the title, **Search**, **Tags** (up to 10), **Limit** (1 to 100), **Keep the time range** and **Include variable values**, and a news panel has the title and **Feed URL**.
Canvas elements, connections and background images are edited in the JSON view.

## Edit as JSON

Choose **JSON** in **Editor view** to see "Panel JSON": the whole panel object, position included, exactly as it will be saved in the ObserveSpec.
Edits there update the editor at once when they are valid, and the editor's changes show up in the JSON.

An edit that is not valid never reaches the draft.
The problems are listed with the line or the path they are about, and "and 2 more" when there are many.
While any remain the summary reads "Fix the JSON to save." and the save button is disabled.
Leaving the view with errors asks "Discard the JSON edits?" with **Keep editing** and **Discard JSON edits**.

Some edits are refused whatever the JSON says:

- The `id` must stay the same ("The panel id must stay "`<id>`"").
- A panel cannot become a row or a placeholder, or move between data, text, dashboard list and news kinds.
- An unknown type reports only that ("`"timeseriesx"` is not a panel type."), not the fields another kind of panel would need.
  Use the **Panel type** picker to change a data panel's type.

## Save as a new version

The footer states what you changed: "No changes", "2 changes: query, title", "Fix the JSON to save." or "Fix the query to save.".
While you type a title, the editor heading and the panel on the dashboard show it; the dashboard goes back to the saved title when you close the editor without saving.
**Add a note** opens "Note for the timeline", up to 500 characters, which becomes the version's summary; without one the summary is `Changed <what> in "<title>"`, where `<title>` is the panel's saved title before your change.
**Discard** drops the draft.

The save button reads "Save as version 4" when you are on the latest version, 3.
It saves the whole dashboard with your panel replaced, as the next version after the one you opened.
The save runs only your panel's queries, as the agent's `update_panels` saves do, and a panel that fails is refused.

A panel whose queries run but return no data for the dashboard's time range is refused too, with a notice that says so and **Save anyway**.
Save anyway keeps the panel and marks it "Unverified" with the empty result as the reason, which suits a query that is empty while things are healthy.
Only a signed-in person can do this: the agent, MCP clients, the CLI and service-account keys still get the refusal, and a request that sets `accept_empty` with a key answers 403 `user_session_required`.
A panel that was already unverified and still fails is kept, marked "Unverified" with the reason it failed when it was saved.
On success the toast reads "Saved version 4" and you return to the timeline.
When the check cannot finish, the editor says "The panel could not be checked right now." and names each query and datasource that did not answer, for example "Query B on tempo: query timed out after 15s", with **Try again**.

If someone saves a version while you edit, the editor says who saved it ("Ada saved version 4 while you were editing.") and the button becomes "Save my changes as version 5".
The header keeps the title the panel had when you started, so it agrees with your draft.
When you and that version changed the same things, the notice lists them, with both values for a title, datasource, panel type or unit ("title: version 4 has "A", yours is "B""); saving keeps your values for those and version 4's for everything else.
When nothing overlaps, it says so.
The button saves at once: it applies your change to that version's copy of the panel, and if the panel no longer exists there it says "This panel no longer exists in the latest version.", and if your change does not fit it says "Your changes do not fit version 4.".

If someone deletes the dashboard while you have unsaved changes, the editor stays open with your draft and says "<name> deleted this dashboard. Your draft stays here until you close the editor, but it cannot be saved. Copy what you need from the JSON view."
When you close the editor, or when you had no unsaved changes, the page reads "Dashboard deleted" instead of "Dashboard not found".

A save made on an older version is merged by the server when the newer versions changed other panels, variables or settings than you did.
The toast then reads "Saved version 5, merged with changes made since version 3".
Moving one panel to a different place than a newer version moved it to counts as the same thing, while a move of a different field of the panel, such as its width, merges.
When the newer versions changed the same things, nothing is saved: the editor lists them under "Version 4 changed the same things", each as the panel title, "Variable <name>" or "Dashboard settings" with what clashed, and **Reload version 4** replaces your draft with that version's panel.

A refused save lists "This panel was rejected:" with each query and its problem, and "Other problems in this dashboard:" for panels you did not touch.
"The panel could not be checked right now." with **Try again** means the check itself failed, for instance a datasource did not answer, rather than the panel being wrong.
Your draft stays in place after any of these.

The new version shows up in the timeline and in the version history, where you can compare it with the previous one or restore an earlier one; see [Comments, agent requests and versions](review.md).

## Who can edit

Editing a panel needs Edit on the dashboard's folder.
By default that means Editors and Admins in an open folder.
A Commenter or Viewer gets it only when a restricted folder's list gives them Edit.
See [Roles](../teams/roles.md) and [Folders](../teams/folders.md).

Without Edit you can still open the dashboard, and a Commenter can [comment on a panel](review.md) for an Editor to act on.

A Commenter can also open a panel's editor with **Suggest an edit** in the panel menu.
The save button reads "Propose change" and the footer says "An editor merges your change".
Saving creates a [proposal](../concepts/proposals.md) instead of a version, and the toast reads "Proposed for an editor to merge" with **View proposal**; saving again while it is open reads "Added to your open proposal (revision 2)".
The dashboard's top row shows "1 proposal" while one is open, listing each with its author and "Behind" when the dashboard has moved on.
