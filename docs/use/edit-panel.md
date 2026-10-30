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

**Edit** is missing from the menu, or the editor is read-only, when:

- you do not have Edit on the dashboard's folder;
- you are on a pinned older version, where the editor says "Open the latest version to edit this panel." with an **Open latest version** button;
- the panel is a row or a placeholder, which says "This panel cannot be edited.".

Data panels, text panels, dashboard lists and news panels can be edited.
Pressing Escape, or leaving the page, with unsaved changes asks "Discard your changes?" with **Keep editing** and **Discard**.

The **Editor view** switch at the top changes between **Editor** and **JSON**.

## Edit queries

For a data panel, the **Preview** section sits at the top and the query rows below it.

**Datasource** picks the default datasource of the panel, from your configured datasources or from a datasource variable the dashboard defines.
Each query row can use another datasource through its own menu.

A row is "Query A", "Query B" and so on, up to 26.
Type the query in the editor; a query is at most 20000 characters.
The server lints the query, and the problems show under the line with "Error: `<message>` (line 2, column 14)", or "Warning: ..." for a lesser one.
A clean query shows "Query is valid.", and "Lint is unavailable right now." means the check did not run, not that the query is wrong.
**Legend** sets the name of each series (up to 200 characters), and **Add query** adds a row.
**Remove query A** appears when the panel has more than one query.
Rows carry a "New" or "Changed" marker against the saved version.

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
- "Draft ran `<time>`, 3 series" when it finished.

When the draft's queries or datasource differ from the saved panel, a dotted line labelled "Saved in v3" shows the saved panel's data next to yours.

A preview is limited like the agent's draft queries: 15 seconds, 200 series, 100,000 points and 1,000 rows, and it is never cached.
It runs for the signed-in person only, not for service-account keys.
If the datasource is slow or busy, the preview says so ("The datasource is not answering.", "Too many requests right now. The preview runs again in a moment.", or "Query A failed: it took too long to run.") and the draft stays as it is.

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
- **Thresholds**, up to 10 rows, each with a colour and the value it starts from; the first row can leave the value empty ("Base").
- **Title**, up to 200 characters.
- The display options that belong to the chosen type.
  For Time series and Trend panels the legend options are **Legend** (bottom, right or hidden), **Legend values**, **Legend mode** (List or Table, "Table when values are shown" until you choose), **Sort legend by** (Name or one of the legend values, "Series order" until you choose), **Sort descending**, **Legend width** (pixels, for a legend on the right), **Hide series that are all zero** and **Hide series with no values**.
  A series hidden from the legend is still drawn.
  Time series, Trend and Candlestick panels also have **Tooltip sort** (Series order, Ascending or Descending, which is the default), **Hide zero values in the tooltip**, **Show stacked totals in the tooltip** and **Tooltip max width** and **Tooltip max height** (pixels, 100 to 2000); the sort, zero and total options apply to a tooltip that lists all series.
  For Stat, Gauge, Bar gauge and Pie chart panels, **Calculation** picks how each field is reduced to one value.
  **Show every value** shows one value per row instead, **Row limit** (1 to 5000) caps the rows shown per field, and **Fields** shows a field by name or by a pattern such as `/^cpu/` instead of every number field.
  A Stat panel also has **Alignment** (Auto or Center), **Title size** and **Value size** (pixels, empty for the default), **Show percent change** with its **Percent change color** (Standard, Inverted or Same as value, applied to the arrow only) and **Wide layout**, which puts the name beside the value on wide tiles.
  Gauge panels add **Show threshold labels**, **Minimum gauge width**, **Minimum gauge height**, **Title size** and **Value size**.
  Bar gauge panels add **Bar style** (Basic or LCD), **Value** (With threshold state, Plain text or Hidden), **Name placement**, **Show unfilled area**, **Minimum bar width**, **Minimum bar height**, **Maximum bar height**, **Title size** and **Value size**; sizes are in pixels.

A text panel has a **Content** box instead of queries: Markdown up to 20000 characters, with "HTML is removed.".
A dashboard list or news panel has only the title and its display options.
Canvas elements and connections are edited in the JSON view.

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
  Use the **Panel type** picker to change a data panel's type.

## Save as a new version

The footer states what you changed: "No changes", "2 changes: query, title", or "Fix the JSON to save.".
**Add a note** opens "Note for the timeline", up to 500 characters, which becomes the version's summary; without one the summary is `Changed <what> in "<title>"`.
**Discard** drops the draft.

The save button reads "Save as version 4" when you are on the latest version, 3.
It saves the whole dashboard with your panel replaced, as the next version after the one you opened.
The save runs every panel query of the dashboard, not just yours, as the agent's saves do, and a panel that fails or returns no data is refused.
A panel that was already unverified and still fails is kept, marked "Unverified" with the reason it failed when it was saved.
On success the toast reads "Saved version 4" and you return to the timeline.

If someone saves a version while you edit, the editor says "Version 4 was saved while you were editing." and "Your changes stay here until you apply them to that version.", and the button becomes "Apply my changes to version 4".
It applies your change to that version's copy of the panel; if the panel no longer exists there it says "This panel no longer exists in the latest version.", and if your change does not fit it says "Your changes do not fit version 4.".

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
