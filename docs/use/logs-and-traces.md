---
title: Logs and traces
description: Search logs and traces from Loki, Tempo or ClickHouse with a query or a plain-language question, follow a line to its trace, tail new lines and keep a query as a dashboard panel.
---

The logs page and the trace view let you look at raw lines and spans without building a dashboard first.
They read from the datasources an Admin configured, as described in [Connect Loki](../connect/loki.md), [Connect Tempo](../connect/tempo.md) and [Connect ClickHouse](../connect/clickhouse.md).

## Logs page

Open **Logs** in the sidebar and pick one datasource in the toolbar: Elasticsearch, OpenSearch, Loki or ClickHouse.
The datasource control shows the name and type, and on Elasticsearch and OpenSearch also the index pattern and time field.
It opens a list of the datasources grouped by type, with a search field, and **Apply** switches to the one you chose.
Switching keeps the time range and clears the query, unless both datasources are Elasticsearch or OpenSearch, which both speak KQL.
The page searches that one datasource in its own language, which the mode switch names: KQL on Elasticsearch and OpenSearch, LogQL on Loki and SQL on ClickHouse.
On Elasticsearch and OpenSearch a query starting with `lucene:` is Lucene, a query that is a `{...}` body is Query DSL, and anything else is KQL.
The address keeps `ds`, `q`, `from` and `to`, so **Copy link** copies a search that opens the same way.
The page starts as soon as it opens.
On ClickHouse it runs a template that reads `otel_logs` and shows the newest 1,000 lines of the time range at once.
On Loki it opens the label browser, which the **Labels** button toggles, and waits for a stream selector.
The label browser lists the labels of the datasource and their values, and a click inserts a matcher at the cursor.
With a model configured on the server, the query box opens in Ask mode, as [Ask on the logs page](#ask-on-the-logs-page) describes.

In Query mode you write the query yourself.
On Loki, write a LogQL query such as `{service_name="checkout"} |= "error"`.
On ClickHouse, the editor holds the template; keep `$__timeFilter(Timestamp)` in the `WHERE` clause and a `LIMIT` at the end.
Press **Run query**, or Ctrl+Enter or Cmd+Enter in the editor, to run it over the time range at the top of the page.
The page shows at most 1,000 lines, newest first.
On Loki the query must start with a stream selector.
When more lines match than the page shows, the page says so and **Load older** fetches the lines before the oldest one shown.
**Live tail** and **Add to dashboard** stay disabled until a query has run, and say "Run a query first."

The lines page 100 at a time, and the page size goes from 50 to 500.
On ClickHouse the page shows the exact count of matching lines and jumps to any page number.
On Loki the pages follow time, newest first, so the count reads "at least N" until the last page makes it exact, and a page is reached through the page before it.

The volume strip above the lines counts every line the query matches over the whole time range by level, not only the lines shown.
It comes from its own log volume request, so the lines never wait for it, and it hides when that request fails or while live tail shows its lines.
On Loki a line's level is the first of its `level`, `lvl`, `severity_text`, `severity` and `detected_level` labels, including a `level` that a `| json` or `| logfmt` stage extracts.
On ClickHouse the level is the `SeverityText` column, counted per `Timestamp`, so the strip hides when the query does not select both.
Levels such as `err`, `fatal` and `critical` count as errors, `warn` as warnings, `info` and `notice` as info, and anything else, or no level, as other.

Facets count the top values of each label, field or column the query matches over the whole time range, through `POST /api/v1/datasources/{uid}/log-facets` and the `log_facets` tool.
Each field says whether its counts are exact or come from a sample of the newest lines.
On Loki the index labels of the matched streams are counted exactly from the index volume API, in bytes rather than lines, so those fields carry `unit: bytes`; parsed fields (`kind: field`) and structured-metadata fields (`kind: metadata`, the detected fields that no parser produced) are counted from the newest 1,000 lines by default, up to 5,000.
On ClickHouse every field is counted exactly with one `GROUP BY` per field: each result column except `Timestamp`, `Body`, `TraceId` and `SpanId`, plus the 20 most frequent `LogAttributes` keys as `LogAttributes.<key>`.

Patterns group the lines a query matches into line templates with counts, the variable tokens written `<*>`, through `POST /api/v1/datasources/{uid}/log-patterns` and the `log_patterns` tool, so a noisy service reads as a few templates instead of thousands of lines.
On a Loki whose pattern ingester is enabled, a query that is a bare stream selector answers from Loki's pattern index over the whole range, and the response says `source: loki`.
Otherwise, on a Loki without the pattern ingester, a Loki query with filters or stages, or ClickHouse, the server reads the newest 2,000 lines by default, up to 5,000, masks numbers, identifiers, addresses and timestamps, clusters lines that share at least 60% of their tokens, and answers `source: server` with each pattern's newest line as its sample.
On Elasticsearch the cluster's `categorize_text` aggregation answers over the whole range with `source: elasticsearch`, each pattern being the words its lines share; OpenSearch has no such aggregation, and a cluster without it or without the licence for it falls back to the `source: server` clustering above.
Patterns come by count descending, 20 by default and at most 100, and `truncated` says when more exist.
Every pattern carries a `trend` of 30 equal buckets over the range, empty ones included with a count of 0, and a `level` (`error`, `warning`, `info`, `other` or empty): for `source: loki` it is the first level word in the pattern, for `source: server` the level most of the pattern's lines carry.

Open a line to see its details in the panel on the right, or inline under the line on a screen 760 pixels wide or narrower.
A field whose value is a JSON object, such as the ClickHouse `LogAttributes` map, shows one row per key, named `<field>.<key>`, one level deep.
The details offer **Show context**, **Open trace** and **Copy line**, which copies the line's text.

The address keeps the search as `/logs?ds=<datasource uid>&q=<query>&from=<from>&to=<to>`, so you can share it or bookmark it.
A query written from a question also keeps the question as `ask` beside `q`, and the link opens the query with its question above it.

### Query languages on Elasticsearch and OpenSearch

An Elasticsearch or OpenSearch datasource runs three query forms.
KQL is the default and what a Kibana user types; a `lucene:` prefix switches to Lucene, the syntax of Kibana's Lucene mode and of Grafana's Elasticsearch targets; text that starts with `{` is a Query DSL body, for aggregations and for a body pasted from Kibana or Grafana.
A `kql:` prefix, or `queryLanguage: lucene` on the datasource, changes which form an unprefixed query is.
The [Elasticsearch](../connect/elasticsearch.md#queries) and [OpenSearch](../connect/opensearch.md#queries) pages describe what each form returns, the aggregations that fold into panels, ad hoc filters, lint and paging.

#### KQL on Elasticsearch and OpenSearch

The server parses a strict subset of Kibana KQL and refuses the rest with a message at the offset of the construct, so a query that Kibana accepts may not paste:

| Construct                        | Example                  | Message                                      | Why it is out                                                                                    |
| -------------------------------- | ------------------------ | -------------------------------------------- | ------------------------------------------------------------------------------------------------ |
| Nested field queries             | `items:{ name:x and qty > 1 }` | "nested queries are not supported"     | They need the `nested` mapping type and a `nested` query.                                        |
| Leading wildcard in a field name | `*name:x`                | "a field name cannot start with \*"          | Kibana expands it from the data view, and the server has no data view.                           |
| Escaped whitespace in a bare word | `message:a\ b`          | "quote a value that contains spaces"         | One escaping rule, quotes, keeps the explorer's filter buttons and the query builder simple.     |
| A `kql:` prefix inside a query   | `kql:a`                  | "kql: is a form prefix, not a query"         | The prefix is read before the parser runs.                                                       |

Date math in a range value, such as `@timestamp >= now-15m`, is passed to the cluster as written, so the server cannot lint it.
A field the mapping does not have is reported as a warning in lint, and the query still runs.

## Ask on the logs page

The **Ask** and **Query** toggle in the top right corner of the query box switches between a question and a query.
Ctrl+I or Cmd+I switches it from anywhere on the page, the editor included.
The page remembers the mode you last chose with the toggle, the shortcut or **Ask again**, and opens in Query when the address carries a query.
Write a question and press Enter or **Ask**, and press Shift+Enter to add a line.
Esc cancels a question that is still running.
The server writes the query as [Ask in plain language](#ask-in-plain-language) describes, and the page runs it at once.
The explanation and any warnings show under the question, and the query shows in the editor below them, where you can still change it.
Editing the query switches to Query, with a "From: <question>" line and **Ask again**, which brings the question back.
A label from the label browser also switches to Query.
When the model cannot turn the question into a query, the page says "I could not turn that into a query." with the reason, and switches to Query.
While the field is empty it lists your 5 latest questions under **Recent**, and Up at the start of the field steps back through earlier ones.
This browser keeps the recent questions, apart from the chat's.
Without a model, the page stays in Query, and the Ask side of the toggle is disabled and says "Add a model key to ask in plain language".
[LLM providers](../connect/llm-providers.md) describes how to configure one.

## Context around a line

Open a line and press **Show context** to see the 50 lines before it and the 50 lines after it.
The server reads both sides in one request, anchored on the line's nanosecond time, so the context is exact: the lines just before are the newest ones older than the line, the lines just after are the oldest ones newer than it, within an hour on each side.
The lines come from the same query the page ran.
Each side says when more lines exist in its hour than it shows.
On an Elasticsearch or OpenSearch time field of type `date`, which keeps milliseconds, context is exact to the millisecond, and the lines in the anchor's own millisecond are left out on both sides.
Scripts call the same endpoint, `POST /api/v1/datasources/{uid}/log-context`, with the line's `TimeNs` as `anchor`.

## Live tail

**Live tail** opens one stream from the server, which polls the datasource every second for lines newer than the last one it sent and pushes them to the page.
The browser runs no query of its own and holds no datasource credential.
Each poll returns at most 500 lines, so a tail shows at most 500 lines per second, and a poll that hit that limit is followed by another at once.
The page keeps the newest 1,000 lines.
A tail stops when you press **Stop live tail**, when the query, the datasource or the time range changes, when the datasource fails five polls in a row, or after 30 minutes.
Each person may hold 5 tails at a time, and the server 200.
Documents reach an Elasticsearch or OpenSearch index a few seconds after their timestamp, so each poll there starts 5 seconds before the newest line sent and skips the lines it already sent, and a tail that stops cancels its search because the cluster cancels a search when its connection closes.
Scripts open the same stream with `GET /api/v1/datasources/{uid}/tail?query=...`, which is Server-Sent Events.

## Add to dashboard

**Add to dashboard** keeps the query as a logs panel on a dashboard you can edit.
You need Edit on the target dashboard.
The panel is full width and goes at the bottom of the dashboard, inside the last row when that row is collapsed, and the save makes a new version.
The save runs the query over the dashboard's time range and refuses a panel that returns no lines there.
The panel's menu offers the way back with **Open in Logs** or **Open in Traces**, which opens the explorer with the panel's query and the dashboard's time range.

## Trace view

A trace opens at `/t/<trace id>?ds=<datasource uid>`.
The title reads the root service and the root span, such as **frontend POST /checkout**, and under it a summary gives the duration, the number of spans, services and errors, the start time, and the slowest service on the critical path with the time it spent there.
**Copy trace id** copies the whole id.
**Open trace** on a log line opens it when the line carries a `TraceId`, `traceID`, `trace_id` or `traceId` field.
A Loki line opens in the first Tempo datasource, and a ClickHouse line opens in the same ClickHouse datasource.
The view draws the spans as a waterfall under a ruler that steps by 1, 2, 2.5 or 5 times a power of ten, so its labels are round numbers.
The view fills the window, and the waterfall scrolls inside it.
**Find in trace** matches a span's name, service, id or attribute values; the matching text is highlighted, the field counts the matches, and Enter opens the next one, Shift+Enter the previous one.
**Critical path** outlines the spans on the longest chain, the ones that add up to the trace's duration, and dims the rest; the line under the waterfall says how many spans are on it.
**Errors only** dims every span that neither failed nor contains a failed span.
**Collapse all** folds every span with children, and **Expand all** opens them again.
The services legend lists each service with its colour and span count; click a service to dim its spans, and click it again to show them.
The overview strip above the ruler draws the whole trace in one line per span; drag on it, or on the ruler, to zoom the time axis to that window, and **Reset zoom** shows the whole trace again.
A click on a span shows its details in the pane beside it, or inline under the span on a screen 760 pixels wide or narrower: the service and the span's kind (server, client, producer, consumer or internal), its name, its status, duration, share of the trace and start offset, then its span attributes, its resource attributes and its events.
Each attribute row offers **Copy value** and, on Tempo, **Search traces where** the attribute is, or is not, that value, which opens the trace search with that TraceQL filter.
An event, such as an `exception`, shows its name, its offset from the span's start and its attributes, and the span's bar carries a dot at each event.
A span's links are listed after its events: each shows the linked trace id, which opens that trace on the same datasource at the linked span, the linked span id and the link's attributes.
A span's details end with **Logs for this span**: the first lines that carry its trace id and span id, read from the Loki or ClickHouse logs datasource, with the count of lines found.
**Open in Logs** continues with those lines on the logs page, filtered on the trace id and span id over the span's window plus a minute on each side.
When a span logged nothing, **Try the whole trace** opens the trace's logs instead.
**Logs for this trace** in the header opens a panel with every line of the trace, oldest first, up to the newest 500, and the same **Open in Logs** link.
A ClickHouse trace reads its logs from a ClickHouse datasource and a Tempo trace from Loki; with more than one logs datasource, a select in the section picks one and the choice is remembered in the browser.
While a span is open the address carries `span=<span id>`, so a copied link opens on that span with its row scrolled into view.
The keys the waterfall answers are listed in [Keyboard shortcuts](keyboard-shortcuts.md).
The view looks for the spans in the last 24 hours, or within an hour of the line when you came from the logs page; a trace outside the range says so and offers **Last 7 days** to widen it.
The view loads the whole trace up to 20,000 spans.
A trace with many repeated sibling spans, such as a batch job's, folds every set of more than 20 siblings with the same service and operation into one hatched row that carries their count, their total and slowest durations and how many failed, so the timeline still adds up.
A banner says the trace folds, the find field searches folded spans too, and the services legend counts them.
Open a folded row to see its slowest 50 spans, and **Show 50 more** for the rest; a search match or a linked span inside a folded group opens the group to it.
A trace over 20,000 spans opens with its earliest 20,000 spans by start time and a banner that says how many the trace holds and that the rest are not loaded; **Critical path** is off for it, because the path needs every span.
When Tempo did not search every block, a banner says that some spans may be missing.
**Download JSON** saves the trace as the browser holds it, the trace endpoint's response, as `trace-<trace id>.json`.
**Copy link** copies the address of the trace with its datasource, time range and open span, and **Search traces** opens the trace search on the same datasource.

## Trace search

Open **Traces** in the sidebar to find traces by service, span name, minimum and maximum duration, and errors only.
The search runs as soon as the page opens, with the fields the address carries, or with none.
The page opens on the last datasource you used in this browser, else Tempo when one is configured; the datasource and the time range sit in the title row, and the **Search traces** button sits under the fields with Ctrl+Enter or Cmd+Enter as its shortcut.
Durations take a number and a unit, such as `250ms` or `1.5s`.
On Tempo you can also write TraceQL, which replaces the fields.
Press **Search traces**, or Enter in Service, Span name or a duration field, to run it; Enter in the TraceQL field adds a line.
A count line above the results says how many traces are on the page.
A scatter above the table plots each trace's duration over time on a log scale, red for traces with errors, with the p95 as a dashed line; a dot opens its trace on click, and hovering a dot highlights its row.
That p95 is of every trace the search matches in the range, not only the listed ones: the page asks the server for the duration distribution of the matching spans and reads the p95 as the upper edge of the bucket that holds it, so a service filter changes it.
While that answer is loading, or when it fails, the line and the ticks fall back to the p95 of the listed traces, and a failed answer shows a warning icon beside the count line with the reason.
The table opens slowest first, and a column header sorts by that column, ascending, descending, then back to the original order.
Each row shows **Root service**, **Root span**, **Duration** with a bar scaled to the slowest trace shown and a tick at the same p95 as the scatter's line, **Spans**, **Errors** and **Started**, which opens the trace.
**Columns** hides a column.
**Errors** reads OK, or Failed with the number of error spans.
**Spans** and **Errors** show only when every row carries the count.
ClickHouse lists the newest traces with a matching span, shows each trace's longest matching span, and counts only the matching spans.
Tempo shows the root span of each trace, and sums the span and error span counts from Tempo's per-service stats; when Tempo does not report those stats, both columns are left out.
When Tempo stopped searching at its time limit, a note says older traces may be missing and which times the found traces cover, with **Keep only** to narrow the range to them.
On a phone the table becomes a list.

The table pages 50 traces at a time, and the bar under it sets 50, 100, 250 or 500 per page.
ClickHouse offers numbered pages and an exact total of matching traces.
Tempo pages by start time from the last trace shown, with **Previous** and **Next** and a running total that reads "at least N" until the last page makes it exact, and no total while a search is partial.
The page and size are in the address as `page` and `size`, and sorting a column sorts only the traces on the current page.
A Tempo address beyond page 1 reopens on page 1 with a note, because a Tempo page is reached through the page before it.

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
On Loki it writes LogQL, and on ClickHouse logs it writes ClickHouse SQL.
For traces it fills the trace search fields: service, span name, minimum and maximum duration, and errors only.
On Tempo it writes TraceQL instead when the question needs more than those fields.
The server first reads the datasource's labels, attributes or columns and some of their values, so the query names real ones.
Then it asks the model once.
It checks the answer with the query lint and one sample run over the last 15 minutes of the time range, and never runs the full query.
When that check fails, it asks the model once more to fix the query, so a question costs at most two model calls.
The answer comes with a one-sentence explanation and warnings, such as a query that reads every stream, a ClickHouse query without `$__timeFilter` or `LIMIT`, or a sample with no matches.
When the model cannot write a query for the question, or the fixed query still fails its check, the answer says it could not turn that into a query and why.
Each person can have 3 questions in flight, and each question stops after 60 seconds.
Without a model configured on the server, Ask is off.

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
