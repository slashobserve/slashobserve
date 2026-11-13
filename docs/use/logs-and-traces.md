---
title: Logs and traces
description: Search logs from Loki, ClickHouse, Elasticsearch or OpenSearch and traces from Tempo or ClickHouse with a query or a plain-language question, follow a line to its trace, tail new lines and keep a query as a dashboard panel.
---

The logs page and the trace view let you look at raw lines and spans without building a dashboard first.
They read from the datasources an Admin configured, as described in [Connect Loki](../connect/loki.md), [Connect Tempo](../connect/tempo.md) and [Connect ClickHouse](../connect/clickhouse.md).

## Logs page

### Choose a datasource

Open **Logs** in the sidebar and pick one datasource in the toolbar: Elasticsearch, OpenSearch, Loki or ClickHouse.
The datasource control shows the name and type, and on Elasticsearch and OpenSearch also the index pattern and time field.
It opens a list of the datasources grouped by type, with a search field, and **Apply** switches to the one you chose.
Switching keeps the time range and clears the query, unless both datasources are Elasticsearch or OpenSearch, which both speak KQL.
The page searches that one datasource in its own language, which the mode switch names: KQL on Elasticsearch and OpenSearch, LogQL on Loki and SQL on ClickHouse.
On Elasticsearch and OpenSearch a query starting with `lucene:` is Lucene, a query that is a `{...}` body is Query DSL, and anything else is KQL, as [Query languages on Elasticsearch and OpenSearch](#query-languages-on-elasticsearch-and-opensearch) describes.
The address keeps `ds`, `q`, `from` and `to`, so **Copy link** copies a search that opens the same way.

### What the page shows first

The page starts as soon as it opens.
On ClickHouse it runs a template that reads `otel_logs` and shows the newest 1,000 lines of the time range at once.
On Loki it opens the label browser, which the **Builder** segment of the mode switch toggles, and waits for a stream selector.
The label browser lists the labels of the datasource and their values, and a click inserts a matcher at the cursor.
With a model configured on the server, the query box opens in Ask mode, as [Ask on the logs page](#ask-on-the-logs-page) describes.

### Query modes

The mode switch in the top right corner of the query box has up to three segments.
**Ask** takes a question.
**Builder** edits the query as filter chips, and the last segment is named for the datasource's language (**KQL**, **LogQL** or **SQL**) and edits the text.
ClickHouse has no Builder segment, because its query is SQL.
On Loki the Builder segment is the label browser.

On Elasticsearch and OpenSearch the Builder is a row of chips with an **Add a filter** button.
A filter picks a field from the index mapping, an operator (is, is not, is one of, exists, greater than, at least, less than, at most or contains) and a value, which the page suggests from the newest lines of the time range.
Choosing a filter or removing a chip rewrites the query and runs it.
The Builder shows only a flat list of clauses joined by `and`, with `not` in front of a single field match.
A query with an `or` at the top, a group, a form prefix or text that does not parse greys the segment, which says "The builder cannot show this query. Edit it as KQL.", and **Edit as KQL** in the Builder switches to the text without changing it.
The page opens in the language segment when the address carries a query.

### Write and run a query

In the language segment you write the query yourself.
On Loki, write a LogQL query such as `{service_name="checkout"} |= "error"`.
On ClickHouse, the editor holds the template; keep `$__timeFilter(Timestamp)` in the `WHERE` clause and a `LIMIT` at the end.
Press **Run query**, or Ctrl+Enter or Cmd+Enter in the editor, to run it over the time range at the top of the page.
The page shows at most 1,000 lines, newest first.

On Loki the query must start with a stream selector.
When a run fails, the box says "No results." with the time and size of the last good run, and on Elasticsearch and OpenSearch a **Check the datasource** link.
When the page shows 1,000 lines it adds the note "Showing the newest 1,000 lines. Narrow the query or the time range."
The results have no page bar and no way to load older lines, so narrow the query or the time range to reach them.
**Live tail**, **Add to dashboard** and **Add to observation** stay disabled until a query has run, and say "Run a query first."

### Results toolbar and columns

The row above the lines holds **Sort**, **Wrap**, **JSON** and **Columns**.
**Sort** picks newest first or oldest first, and clicking the **Time** heading flips it.
**Wrap** wraps long lines, and **JSON** reformats a line whose body is JSON over several rows.
**Columns** lists the fields found in the loaded lines, and each field you tick becomes a column before the line, written to the address as `cols`.
On Elasticsearch and OpenSearch the `service` field is a column until you choose others.
The menu on a column heading sorts the loaded lines A to Z or Z to A, moves the column left or right, or removes it.

### Log volume

The volume strip above the lines counts every line the query matches over the whole time range by level, not only the lines shown.
It comes from its own log volume request, so the lines never wait for it, and it hides when that request fails or while live tail shows its lines.

- On Loki a line's level is the first of its `level`, `lvl`, `severity_text`, `severity` and `detected_level` labels, including a `level` that a `| json` or `| logfmt` stage extracts.
- On ClickHouse the level is the `SeverityText` column, counted per `Timestamp`, so the strip hides when the query does not select both.
- On Elasticsearch and OpenSearch one aggregation request counts the matching documents per time bucket and per value of the level field, the field the datasource's level role names; with no level field every document counts as other, and a query body with its own `aggs` is refused.

Levels such as `err`, `fatal` and `critical` count as errors, `warn` as warnings, `info` and `notice` as info, and anything else, or no level, as other.

### Facet counts

Facets count the top values of each label, field or column the query matches over the whole time range, through `POST /api/v1/datasources/{uid}/log-facets` and the `log_facets` tool.
Each field says whether its counts are exact or come from a sample of the newest lines.

- On Loki the index labels of the matched streams are counted exactly from the index volume API, in bytes rather than lines, so those fields carry `unit: bytes`; parsed fields (`kind: field`) and structured-metadata fields (`kind: metadata`, the detected fields that no parser produced) are counted from the newest 1,000 lines by default, up to 5,000.
- On ClickHouse every field is counted exactly with one `GROUP BY` per field: each result column except `Timestamp`, `Body`, `TraceId` and `SpanId`, plus the 20 most frequent `LogAttributes` keys as `LogAttributes.<key>`.
- On Elasticsearch and OpenSearch one aggregation request counts every field exactly over the whole range, with `sampled: false` and `lines_sampled: 0`, and `sample` is ignored.

Without `fields`, the level and service fields come first, then the keyword, boolean, IP and number fields by name, and text fields through their `.keyword` subfield, 20 fields at most.
A field named in `fields` that the mapping lacks answers `kind: unknown`, and a text field without a keyword subfield, an object, a nested field or a field whose type differs between the matched indices answers its kind with no values.
A field says `exact: false` when the server reports a count error bound for it, and `truncated: true` when it has more values than were returned.

### Pattern templates

Patterns group the lines a query matches into line templates with counts, the variable tokens written `<*>`, through `POST /api/v1/datasources/{uid}/log-patterns` and the `log_patterns` tool, so a noisy service reads as a few templates instead of thousands of lines.

- On a Loki whose pattern ingester is enabled, a query that is a bare stream selector answers from Loki's pattern index over the whole range, and the response says `source: loki`.
- Otherwise, on a Loki without the pattern ingester, a Loki query with filters or stages, or ClickHouse, the server reads the newest 2,000 lines by default, up to 5,000, masks numbers, identifiers, addresses and timestamps, clusters lines that share at least 60% of their tokens, and answers `source: server` with each pattern's newest line as its sample.
- On Elasticsearch the cluster's `categorize_text` aggregation answers over the whole range with `source: elasticsearch`, each pattern being the words its lines share; OpenSearch has no such aggregation, and a cluster without it or without the licence for it falls back to the `source: server` clustering above.

Patterns come by count descending, 20 by default and at most 100, and `truncated` says when more exist.
Every pattern carries a `trend` of 30 equal buckets over the range, empty ones included with a count of 0, and a `level` (`error`, `warning`, `info`, `other` or empty): for `source: loki` it is the first level word in the pattern, for `source: server` the level most of the pattern's lines carry.

### Line details

Click a line, or focus it and press Enter, to open its details; press `j` and `k` in the list to move to the next and previous line.
Opening a line adds `line` to the address, closing it removes `line`, and **Copy link** in the line's details copies an address that opens that line.
When the linked line is not in the results, the page says so and **Clear** removes `line` from the address.

Open a line to see it in the panel on the right, beside the results, or inline under the line on a screen 760 pixels wide or narrower.
The panel has a **Table** tab, a **JSON** tab that prints the whole line, and a **Context** tab; press Escape to close it and return to the line.
On Elasticsearch and OpenSearch the panel is titled **Document** and the table groups **Core fields** (time, level, service, trace id, span id and document id) apart from **Other fields**, which hold the rest of the document and its index as `_index`.
A field whose value is a JSON object, such as the ClickHouse `LogAttributes` map, shows one row per key, named `<field>.<key>`, one level deep.
The panel offers **Show context**, **Copy line**, which copies the line's text, and **Copy link**, and an **Open trace** link when the line carries a trace id.

Each field in the table has **Filter in** and **Filter out** buttons, shown when you hover or focus the row, that rewrite the query and run it, and Back returns to the previous query.
On Loki a stream label goes into the stream selector and a parsed field or structured metadata value into a `|` stage at the end of the query; on ClickHouse a column or a `LogAttributes` key goes into the `WHERE` clause.
On Elasticsearch and OpenSearch the buttons append a KQL clause to the query, **Require** adds `field:*` for a field that exists, and the buttons are disabled on a Lucene or Query DSL query; the core fields have no buttons.
A button is disabled when the query already holds the filter, when the query has no shape the button can edit (a Loki query with no stream selector, or a ClickHouse query with no `WHERE` clause or with a subquery), or when the value is longer than 200 characters.

### Share a search

The address keeps the search as `/logs?ds=<datasource uid>&q=<query>&from=<from>&to=<to>`, so you can share it or bookmark it.
A query written from a question also keeps the question as `ask` beside `q`, and the link opens the query with its question above it.

### Query languages on Elasticsearch and OpenSearch

An Elasticsearch or OpenSearch datasource runs three query forms.
KQL is the default and what a Kibana user types; a `lucene:` prefix switches to Lucene, the syntax of Kibana's Lucene mode and of Grafana's Elasticsearch targets; text that starts with `{` is a Query DSL body, for aggregations and for a body pasted from Kibana or Grafana.
A `kql:` prefix, or `queryLanguage: lucene` on the datasource, changes which form an unprefixed query is.
The [Elasticsearch](../connect/elasticsearch.md#queries) and [OpenSearch](../connect/opensearch.md#queries) pages describe what each form returns, the aggregations that fold into panels, ad hoc filters, lint and paging.

#### KQL on Elasticsearch and OpenSearch

The server parses a strict subset of Kibana KQL and refuses the rest with a message at the offset of the construct, so a query that Kibana accepts may not paste.
The query box shows the same check as you type: a caret under the offending character, the message in plain words and, when one edit makes the query valid, a button that applies it.
A missing closing parenthesis is closed before the next top-level `and`, or at the end.
A query that reads as Lucene, such as one with `+`, `-` or `~` operators, offers **Switch to Lucene**, which adds the `lucene:` prefix and runs the text as written.
**Fix it with Ask** asks the model to repair the text, **Open in the builder** shows it as chips when it can, and **KQL reference** opens this section.
Nothing runs while the query cannot be read.
The **Language** control under the box switches between KQL, Lucene and Query DSL: it adds or removes the `lucene:` prefix, and picking Query DSL on an empty box inserts a `match_all` body.
The editor highlights JSON as soon as the text starts with `{`.

| Construct                        | Example                  | Message                                      | Why it is out                                                                                    |
| -------------------------------- | ------------------------ | -------------------------------------------- | ------------------------------------------------------------------------------------------------ |
| Nested field queries             | `items:{ name:x and qty > 1 }` | "nested queries are not supported"     | They need the `nested` mapping type and a `nested` query.                                        |
| Leading wildcard in a field name | `*name:x`                | "a field name cannot start with \*"          | Kibana expands it from the data view, and the server has no data view.                           |
| Escaped whitespace in a bare word | `message:a\ b`          | "quote a value that contains spaces"         | One escaping rule, quotes, keeps the explorer's filter buttons and the query builder simple.     |
| A `kql:` prefix inside a query   | `kql:a`                  | "kql: is a form prefix, not a query"         | The prefix is read before the parser runs.                                                       |

Date math in a range value, such as `@timestamp >= now-15m`, is passed to the cluster as written, so the server cannot lint it.
A field the mapping does not have is reported as a warning in lint, and the query still runs.

## Filters

A filter pill narrows the query without retyping it.
The row under the query box shows one pill for each filter, such as `service.name: checkout` or `NOT host.name: canary-*`, and **Add filter** opens a form with a field, an operator and a value.
The field and the value suggest what the datasource holds in the time range, and the operators are is, is not, is one of, exists and between.
A pill is applied in the language of the datasource: on Elasticsearch and OpenSearch it adds a KQL clause to the query, on Loki it goes into the stream selector or a `|` stage, and on ClickHouse into the `WHERE` clause.

Pick a pill to change it:

- **Exclude** turns it into its negation and **Include** turns it back.
- **Pin** keeps it when you change the query, and **Unpin** lets the next query clear it.
- **Pause** keeps the pill in the row but leaves it out of the query until you pick **Resume**.
- **Edit** opens the form with the pill's values.
- **Delete** removes it.

The pills are not part of the text in the query box.
The results, the log volume, the fields and the patterns all read the query with the active pills added, and the address keeps the pills as `f` beside `q`, so a shared link carries both.
When you run a different query, the pills that are not pinned are cleared and the page says how many.
A saved search stores the query with its pills folded in as text.

On Elasticsearch and OpenSearch a pill needs a KQL query, so the row asks you to switch the query to KQL while the text starts with `lucene:` or `{`.
On Loki and ClickHouse only the is and is not operators apply.
A pill the datasource cannot express is marked "not applied" and left out of the query.

## Fields

The column on the left of the results lists the fields of the datasource: the mapped fields of the index on Elasticsearch and OpenSearch, the stream labels and the fields the query shows on Loki, and the columns of the query on ClickHouse.
The note under **Fields** says where the list comes from, and the search box narrows it by name.
Columns you chose for the results come first under **Selected**; the rest are under **Available**.
A badge beside each name says its kind: keyword, text, number, date, boolean, IP, or on Loki a label, a parsed field or structured metadata.

Open a field to see its top five values with their counts and a bar scaled to the largest.
The line under the values says how they were counted.
Where the datasource counts, the counts are exact and cover every matching line.
Where it cannot, the counts come from the newest 1,000 lines and the line says so; on Loki that is the parsed fields and the structured metadata.
Loki counts its index labels in bytes, not lines, and shows a size.
A field the query does not return, or that has no countable values, says "No values in this range."

Click a value to keep only the lines that have it, or its exclude button to drop them.
The page rewrites the query and runs it, so **Back** returns to the previous one.
On Elasticsearch and OpenSearch the page adds a KQL clause; a Lucene or Query DSL query is not rewritten, and its buttons stay disabled.
On Loki the page adds a `| field="value"` stage to a log query.
On ClickHouse the page adds a condition to the `WHERE` clause, and a query without one keeps its buttons disabled.

**Hide fields** folds the column to a strip with **Show fields**, and the page remembers the choice.
Below 1,000 pixels wide the column is hidden.

## Patterns

The **Patterns** tab beside **Documents** groups the lines the query matches into templates and counts them.
The tab shows how many patterns there are once they have loaded, and it stays disabled until a query has run.
A template keeps the fixed words of a line and writes each number or id as `<*>`.
Each row shows the template, its count, its share of the matching lines, a small chart of how often it occurred across the time range with its change over the range ("new" for a pattern that appeared late), and a dot for its level.
The heading says how many patterns there are and what share of the documents they cover, and **Sort patterns** orders the rows by most lines or by newest.

Where the patterns come from depends on the datasource.
Loki answers from its pattern index over the whole range when the pattern ingester is on and the query is a bare stream selector; otherwise, and on ClickHouse, the page groups the newest 2,000 lines itself and the counts are for that sample only.
Elasticsearch groups the matching documents with its `categorize_text` aggregation when the cluster has it; OpenSearch and a cluster without it are grouped by the page like ClickHouse.
The note under the table says which applies.

**Filter** keeps only the lines of a pattern and **Filter out** drops them, and both return to **Documents** with the rewritten query, so **Back** returns to the previous one.
On Loki the page adds a regular expression line filter (`|~` or `!~`) after the existing stages, and on ClickHouse a `match(Body, ...)` condition in the `WHERE` clause.
On Elasticsearch and OpenSearch it adds KQL clauses on `message` for the fixed words of the pattern.
A query the page cannot rewrite, such as a Lucene or Query DSL query or a ClickHouse query without a `WHERE`, keeps both buttons disabled.
**Samples** shows the newest line of a pattern when the server read the lines itself.

The table shows the nine largest patterns and every pattern with at least 1% of the lines.
The rest fold under "9 of 14 patterns shown, 5 cover under 1% each" until you press **Show all**.
The page asks for the 20 largest patterns; **Show 100** asks for more when there are more.

## Ask on the logs page

The mode switch in the top right corner of the query box switches between a question, the builder and the query.
Ctrl+I or Cmd+I steps through the modes the datasource offers from anywhere on the page, the editor included, and skips a mode that is not available.
The page remembers the mode you last chose with the switch, the shortcut or **Ask again**, and opens in the query when the address carries one.

Write a question and press Enter or **Ask**, and press Shift+Enter to add a line.
Esc cancels a question that is still running.

The server writes the query as [Ask in plain language](#ask-in-plain-language) describes, and the page runs it at once.
On Elasticsearch and OpenSearch the model answers in KQL, and the Builder then shows the answer as chips when it can.
The explanation and any warnings show under the question, and the query shows in the editor below them, where you can still change it.
Editing the query switches to the query segment, with a "From: <question>" line and **Ask again**, which brings the question back.
A label from the label browser also switches to the query segment.
When the model cannot turn the question into a query, the page says "I could not turn that into a query." with the reason, and switches to the query segment.

While the field is empty it lists your 5 latest questions under **Recent**, and Up at the start of the field steps back through earlier ones.
This browser keeps the recent questions, apart from the chat's.
Without a model, the page stays out of Ask, and the Ask segment is disabled and says "Add a model key to ask in plain language".
[LLM providers](../connect/llm-providers.md) describes how to configure one.

## Context around a line

Open a line and choose **Context** (**Surrounding documents** on Elasticsearch and OpenSearch) to see the 50 lines before it and the 50 lines after it, with the line itself marked between them.
The server reads both sides in one request, anchored on the line's nanosecond time, so the context is exact: the lines just before are the newest ones older than the line, the lines just after are the oldest ones newer than it, within an hour on each side.
The lines come from the same stream labels on Loki, the same `ServiceName` on ClickHouse, and the whole index on Elasticsearch and OpenSearch.
When more lines exist on a side than it shows, **Load 10 older** or **Load 10 newer** adds 10 to that side, up to 500.
**Window** widens the search from an hour to 10 minutes or 24 hours, and a side with nothing in the window says so.
On an Elasticsearch or OpenSearch time field of type `date`, which keeps milliseconds, context is exact to the millisecond, and the lines in the anchor's own millisecond are left out on both sides.
Scripts call the same endpoint, `POST /api/v1/datasources/{uid}/log-context`, with the line's `TimeNs` as `anchor`.

## Live tail

**Live tail** polls the datasource through the explore query every 2 seconds, asking for the lines from the newest one it holds onward, and adds what comes back to the list.
Each poll returns at most 1,000 lines, and the poll runs as the signed-in person, under the explore query's limits.
The page keeps the newest 1,000 lines of the last 5 minutes and drops older ones off the top, and the note under the list says so.

The tail lists them oldest first with the newest at the bottom, so the sort is fixed while it runs and the search order comes back when you leave it.
Lines that just arrived are marked for a moment, and a status line above the list tells when the tail started and how many lines it received.
Scrolling up pauses the list so it stops moving under the pointer, and a button counts what arrived meanwhile.
**Jump to newest**, **Resume** or scrolling back to the bottom resumes it.
**Clear** empties the lines shown until you start the tail again.

A tail stops when you press **Stop live tail**, when you run a query or change the time range, or when a poll fails.
A failed poll shows "Live tail stopped" with the reason and **Start again**.

The browser polls and does not read the server's stream.
Scripts can read that stream with `GET /api/v1/datasources/{uid}/tail?query=...`, which sends Server-Sent Events, polls the datasource every second and ends after 30 minutes; see the API reference.

## Add to dashboard

**Add to dashboard** keeps the query as a logs panel on a dashboard you can edit.
You need Edit on the target dashboard.
The panel is full width and goes at the bottom of the dashboard, inside the last row when that row is collapsed, and the save makes a new version.
The save runs the query over the dashboard's time range and refuses a panel that returns no lines there.
The panel's menu offers the way back with **Open in Logs** or **Open in Traces**, which opens the explorer with the panel's query and the dashboard's time range.

## Add to an observation

**Add to observation** copies the search you ran into an [observation](observations.md) as a logs panel named Logs, with the datasource, the query and the time range the page shows.
The side panel offers an open observation or a new one, the same as **Add to observation** in a dashboard panel's menu, and describes what is added and what is refused in [Observations](observations.md#add-to-an-observation-from-a-panel-or-from-logs).
When the page was opened from **Logs** in an observation's row of **Add a block**, its address carries `observation=<id>`, and that observation is preselected when it is open.
The block stays live on the page, so it runs the query when the page is open.

## Trace view

A trace opens at `/t/<trace id>?ds=<datasource uid>`.
The title reads the root service and the root span, such as **frontend POST /checkout**, and under it a summary gives the duration, the number of spans, services and errors, the start time, and the slowest service on the critical path with the time it spent there.
**Copy trace id** copies the whole id.
**Open trace** on a log line opens it when the line carries a `TraceId`, `traceID`, `trace_id` or `traceId` field.
A Loki line opens in the first Tempo datasource, and a ClickHouse line opens in the same ClickHouse datasource.

### Waterfall

The view draws the spans as a waterfall under a ruler that steps by 1, 2, 2.5 or 5 times a power of ten, so its labels are round numbers.
The view fills the window, and the waterfall scrolls inside it.

- **Find in trace** matches a span's name, service, id or attribute values; the matching text is highlighted, the field counts the matches, and Enter opens the next one, Shift+Enter the previous one.
- **Critical path** outlines the spans on the longest chain, the ones that add up to the trace's duration, and dims the rest; the line under the waterfall says how many spans are on it.
- **Errors only** dims every span that neither failed nor contains a failed span.
- **Collapse all** folds every span with children, and **Expand all** opens them again.
- The services legend lists each service with its colour and span count; click a service to dim its spans, and click it again to show them.
- The overview strip above the ruler draws the whole trace in one line per span; drag on it, or on the ruler, to zoom the time axis to that window, and **Reset zoom** shows the whole trace again.

### Span details

A click on a span shows its details in the pane beside it, or inline under the span on a screen 760 pixels wide or narrower: the service and the span's kind (server, client, producer, consumer or internal), its name, its status, duration, share of the trace and start offset, then its span attributes, its resource attributes and its events.
Each attribute row offers **Copy value** and, on Tempo, **Search traces where** the attribute is, or is not, that value, which opens the trace search with that TraceQL filter.
An event, such as an `exception`, shows its name, its offset from the span's start and its attributes, and the span's bar carries a dot at each event.
A span's links are listed after its events: each shows the linked trace id, which opens that trace on the same datasource at the linked span, the linked span id and the link's attributes.

While a span is open the address carries `span=<span id>`, so a copied link opens on that span with its row scrolled into view.
The keys the waterfall answers are listed in [Keyboard shortcuts](keyboard-shortcuts.md).

### Logs for a span or trace

A span's details end with **Logs for this span**: the first lines that carry its trace id and span id, read from the Loki or ClickHouse logs datasource, with the count of lines found.
**Open in Logs** continues with those lines on the logs page, filtered on the trace id and span id over the span's window plus a minute on each side.
When a span logged nothing, **Try the whole trace** opens the trace's logs instead.
**Logs for this trace** in the header opens a panel with every line of the trace, oldest first, up to the newest 500, and the same **Open in Logs** link.
A ClickHouse trace reads its logs from a ClickHouse datasource and a Tempo trace from Loki; with more than one logs datasource, a select in the section picks one and the choice is remembered in the browser.

### Time range and large traces

The view looks for the spans in the last 24 hours, or within an hour of the line when you came from the logs page; a trace outside the range says so and offers **Last 7 days** to widen it.
The view loads the whole trace up to 20,000 spans.

A trace with many repeated sibling spans, such as a batch job's, folds every set of more than 20 siblings with the same service and operation into one hatched row that carries their count, their total and slowest durations and how many failed, so the timeline still adds up.
A banner says the trace folds, the find field searches folded spans too, and the services legend counts them.
Open a folded row to see its slowest 50 spans, and **Show 50 more** for the rest; a search match or a linked span inside a folded group opens the group to it.

A trace over 20,000 spans opens with its earliest 20,000 spans by start time and a banner that says how many the trace holds and that the rest are not loaded; **Critical path** is off for it, because the path needs every span.
When Tempo did not search every block, a banner says that some spans may be missing.

### Export and share

**Download JSON** saves the trace as the browser holds it, the trace endpoint's response, as `trace-<trace id>.json`.
**Copy link** copies the address of the trace with its datasource, time range and open span, and **Search traces** opens the trace search on the same datasource.

## Trace search

Open **Traces** in the sidebar to find traces by service, span name, minimum and maximum duration, and errors only.
The search runs as soon as the page opens, with the fields the address carries, or with none.
The page opens on the last datasource you used in this browser, else Tempo when one is configured; the datasource and the time range sit in the title row, and the **Search traces** button sits under the fields with Ctrl+Enter or Cmd+Enter as its shortcut.

Durations take a number and a unit, such as `250ms` or `1.5s`.
On Tempo you can also write TraceQL, which replaces the fields.
Press **Search traces**, or Enter in Service, Span name or a duration field, to run it; Enter in the TraceQL field adds a line.

### Results

A count line above the results says how many traces are on the page.
A scatter above the table plots each trace's duration over time on a log scale, red for traces with errors, with the p95 as a dashed line; a dot opens its trace on click, and hovering a dot highlights its row.

That p95 is of every trace the search matches in the range, not only the listed ones: the page asks the server for the duration distribution of the matching spans and reads the p95 as the upper edge of the bucket that holds it, so a service filter changes it.
While that answer is loading, or when it fails, the line and the ticks fall back to the p95 of the listed traces, and a failed answer shows a warning icon beside the count line with the reason.

The table opens slowest first, and a column header sorts by that column, ascending, descending, then back to the original order.
Each row shows **Root service**, **Root span**, **Duration** with a bar scaled to the slowest trace shown and a tick at the same p95 as the scatter's line, **Spans**, **Errors** and **Started**, which opens the trace.
**Columns** hides a column.
**Errors** reads OK, or Failed with the number of error spans.
**Spans** and **Errors** show only when every row carries the count.

- ClickHouse lists the newest traces with a matching span, shows each trace's longest matching span, and counts only the matching spans.
- Tempo shows the root span of each trace, and sums the span and error span counts from Tempo's per-service stats; when Tempo does not report those stats, both columns are left out.

When Tempo stopped searching at its time limit, a note says older traces may be missing and which times the found traces cover, with **Keep only** to narrow the range to them.
On a phone the table becomes a list.

### Paging

The table pages 50 traces at a time, and the bar under it sets 50, 100, 250 or 500 per page.
ClickHouse offers numbered pages and an exact total of matching traces.
Tempo pages by start time from the last trace shown, with **Previous** and **Next** and a running total that reads "at least N" until the last page makes it exact, and no total while a search is partial.
The page and size are in the address as `page` and `size`, and sorting a column sorts only the traces on the current page.
A Tempo address beyond page 1 reopens on page 1 with a note, because a Tempo page is reached through the page before it.

### Compare selected traces

Drag across the duration scatter to select traces by time and duration; the dots outside the selection fade, and the selection's start, end and duration floor are handles that the arrow keys move.
The table then shows only the selection, and **Keep only the selection** turns it into the search: the time range and the duration bounds go into the fields, or on Tempo into the written TraceQL, where the button is disabled when the query has no spanset to add them to.
**Clear** brings every trace back.

While a selection exists, the **What is different about the selected traces** panel ranks the attribute values that appear in a larger share of the selected traces than of the other matching traces: span names, kinds, statuses and the span and resource attributes, each with its share of both sets.
It reads every span of the newest 50 selected and 150 other loaded traces, says when it could not read them all, and keeps the weaker differences behind **Show weaker differences**.
**Filter** on a row adds that attribute value to the search: on Tempo it appends the condition to the TraceQL, and on ClickHouse it fills the Span name or Service field, so the other attributes have no **Filter** there.

## Ask on the trace search

With a model configured on the server, the trace search has the same **Ask** and **Query** toggle as the logs page, in the row above the search box.
Ctrl+I or Cmd+I switches it, Enter asks, Shift+Enter adds a line and Esc cancels a question that is still running.
The mode you last chose on either page is the one both pages open in, and the trace search opens in Query when the address carries a search field.
**Recent** and Up bring back earlier questions, kept apart from those of the logs page.

The answer fills Service, Span name, the durations and Errors only, or on Tempo the TraceQL field when the question needs TraceQL.
A duration the form cannot read is left empty.
The search runs at once, and the address keeps the question as `ask`.
The explanation and any warnings show under the question, with **Edit the query**, which switches to Query with the answer's values in the form.
The Run row then shows a "From: <question>" line and **Ask again**, which brings the question back.

When the model cannot turn the question into a query, the page says "I could not turn that into a query." with the reason, switches to Query and puts the cursor in Service.
Changing the datasource drops the question, because it was answered for the other one.
Without a model, the page stays in Query, and the Ask side of the toggle is disabled and says "Add a model key to ask in plain language".

## Ask in plain language

Ask turns a question such as "checkout errors in the last hour" into a query for one datasource.
On Loki it writes LogQL, on ClickHouse logs it writes ClickHouse SQL, and on Elasticsearch and OpenSearch it writes KQL, with the field names and values the datasource really has.
It writes a `lucene:` query only when KQL cannot say what the question needs.
For traces it fills the trace search fields: service, span name, minimum and maximum duration, and errors only.
On Tempo it writes TraceQL instead when the question needs more than those fields.

The server first reads the datasource's labels, attributes or columns and some of their values, so the query names real ones.
Then it asks the model once.
It checks the answer with the query lint and one sample run over the last 15 minutes of the time range, and never runs the full query.
When that check fails, it asks the model once more to fix the query, so a question costs at most two model calls.

The answer comes with a one-sentence explanation and warnings, such as a query that reads every stream or every document, a Lucene query that the explorer's Builder cannot edit, a KQL field the mapping does not have, a ClickHouse query without `$__timeFilter` or `LIMIT`, or a sample with no matches.
When the model cannot write a query for the question, or the fixed query still fails its check, the answer says it could not turn that into a query and why.

Each person can have 3 questions in flight, and each question stops after 60 seconds.
Without a model configured on the server, Ask is off.

## Save a search

A saved search keeps what the logs page or the trace view shows so that anyone with access can open it again: the datasource, the query as it runs with its filter pills folded in, the visible columns of a logs search or the filters of a trace search, and the range as written, such as `now-1h`.
A range written like that stays relative, so the search opens on the latest hour and not on the hour it was saved in.
A logs search takes a Loki, ClickHouse, Elasticsearch or OpenSearch datasource and needs a query.
A trace search takes a Tempo or ClickHouse datasource, and its query is optional, except on ClickHouse, which takes filters only.

On the logs page and the trace search, **Save search** asks for a title and a folder and saves the search you are looking at, and the **Saved searches** box next to it opens one of the searches you can view.
The logs page saves the query that runs, so the filter pills are folded into the query text and reopening the search shows them as query text, not as pills.
A trace search on a ClickHouse datasource saves its filters only, because the trace search ignores a query there.
The folder list offers only the folders where you can create, and a refused save shows the server's message.

A search is saved in a folder, which needs the Create permission there.
Without a folder it goes to your My Dashboards, and a service account saves it in General.
Everyone who can view the folder can open the search, and the folder's permissions decide who may change it, as [Folders](../teams/folders.md) describes.
Renaming, moving and deleting a saved search need Edit on it, and a move needs Edit on the target folder too.
A search cannot move into a personal folder, because that would make it private without asking: save it there instead.
The kind, datasource, query, filters and range never change, so save a new search to change one of them.

`POST /api/v1/saved-searches` saves a search, `GET /api/v1/saved-searches` lists the ones in the folders you can view (`kind`, `folder_id`, `limit` and `cursor` narrow and page the list), `PATCH /api/v1/saved-searches/{id}` renames or moves one and `DELETE` removes it.
Each saved search carries the `url` of the page that reopens it, and each change is recorded in [Activity](activity.md).
Deleting a folder that still holds a saved search is refused, and deleting a user deletes the searches in their personal folder.

## Who can use them and limits

Every signed-in role from Viewer can use the logs page, the trace view, the trace search and Ask.
Each query stops after 15 seconds and returns at most 1,000 rows.
The queries run on the server, so datasource credentials never reach the browser.

## Scripts and the CLI

Scripts run the same queries with `POST /api/v1/datasources/{uid}/query` and a service-account key:

```sh
curl -fsS -X POST "$OBSERVE_SERVER/api/v1/datasources/<datasource uid>/query" \
  -H "Authorization: Bearer $OBSERVE_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"query":"{service_name=\"checkout\"}","kind":"instant","from":"now-15m","to":"now","limit":100}'
```

`limit` lowers the row limit and never raises it above 1,000.
`POST /api/v1/datasources/{uid}/log-volume` takes `query`, `from`, `to` and an optional `buckets` (60 by default, at most 200) and returns the line counts by level per bucket over the range, for Loki and ClickHouse.
[The CLI](cli.md) has `observe logs` and `observe traces` for the same searches, and `--ask` for questions.

Scripts ask a question with `POST /api/v1/datasources/{uid}/draft-query`:

```sh
curl -fsS -X POST "$OBSERVE_SERVER/api/v1/datasources/<datasource uid>/draft-query" \
  -H "Authorization: Bearer $OBSERVE_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"question":"checkout errors","from":"now-1h","to":"now"}'
```

The answer has `query`, `language` (`logql`, `clickhouse_sql`, `traceql` or `filters`), `trace_filters`, `explanation` and `warnings`.
A ClickHouse datasource needs `"signal":"logs"` or `"signal":"traces"`.
A question the model cannot turn into a query answers `422` with code `cannot_translate` and a `reason`, and a server with no model answers `503` `agent_unavailable`.
