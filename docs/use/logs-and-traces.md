---
title: Logs and traces
description: Search logs from Loki, ClickHouse, Elasticsearch or OpenSearch and traces from Tempo or ClickHouse with a query or a plain-language question, follow a line to its trace, tail new lines and keep a query as a dashboard panel.
---

The logs page and the trace view let you look at raw lines and spans without building a dashboard first.
They read from the datasources an Admin configured, as described in [Connect Loki](../connect/loki.md), [Connect Tempo](../connect/tempo.md) and [Connect ClickHouse](../connect/clickhouse.md).

## Logs page

### Choose a source

Each log source, an Elasticsearch, OpenSearch, Loki or ClickHouse datasource, has its own page at `/logs/<datasource uid>`.

`/logs` lists the log sources, each with its query language and whether it answers, and under them the saved logs searches you can view, most recently changed first, with their source, folder, who saved them and when; a source or a search opens on its own page.

Each source in the list also shows the services it has seen in the last hour, the first three names and then "+N more", and a click on "+N more" shows the rest, with **Show fewer** to fold them again.
A service name opens that source with the service filter applied: a pill on the field that holds the service for logs, such as `service_name` on Loki, and the service filter for traces.
The box above the list filters the sources by the text in a source name or in a service name, so typing `checkout` leaves the sources that hold a checkout service.
Every source loads its services on its own, so a slow source shows "Services unavailable" for itself and the others fill in as they answer.
The services come from `GET /api/v1/datasources/{uid}/services`, as [Scripts and the CLI](#scripts-and-the-cli) describes, and are cached on the server for about a minute.

With two or more log sources they are listed under **Logs** in the sidebar, each with its query language: SQL on ClickHouse, LogQL on Loki and KQL on Elasticsearch and OpenSearch.
The five you used last on this browser come first and the rest follow by name, and **More** opens the Logs list.
The order is set when the app loads and stays fixed until you reload, so opening a source does not move it; the next load puts it first.
A source added or removed meanwhile goes by name without moving the others.
With one log source nothing is listed under it, and **Logs** still opens the list.
For an Admin the built-in audit datasource, `slashobserve-audit`, is in none of the sidebar's lists, the collapsed sidebar's menu and the `/logs` list, and the default source for a Logs link without one never is it; **Audit**, then **Explore**, opens it, and so does its address `/logs/slashobserve-audit`, the phone's source menu and the command palette.
On the collapsed sidebar the Logs icon opens a menu with a **Logs** link and the same sources, and on a phone the source line under the page title opens the sources.
The title names the source and its language, such as **Logs / ClickHouse** with an **SQL** tag.

Switching to another source keeps the time range and the filter pills and clears the query, unless both are Elasticsearch or OpenSearch, which both speak KQL.
An address that carries a search but no source, such as `/logs?q=...`, opens the source you used last on this browser, else the first by name, and an old `/logs?ds=<datasource uid>` address opens `/logs/<datasource uid>` with the rest of its query string.
The page searches that one source in its own language, which the mode switch names: KQL on Elasticsearch and OpenSearch, LogQL on Loki and SQL on ClickHouse.
On Elasticsearch and OpenSearch a query starting with `lucene:` is Lucene, a query that is a `{...}` body is Query DSL, and anything else is KQL, as [Query languages on Elasticsearch and OpenSearch](#query-languages-on-elasticsearch-and-opensearch) describes.
The address keeps the source, `q`, `from` and `to`, so **Copy link** in the top bar copies a search that opens the same way.
Times on the page are in the time zone of your browser, and a range between two moments reads with that zone after it, such as `Nov 18, 19:53 to 19:59 GMT+5:30`.

### What the page shows first

The page starts as soon as it opens.
On ClickHouse it runs a template that reads `otel_logs` and shows the newest lines of the time range at once.
On Elasticsearch and OpenSearch it runs `*`, which matches every document, and shows the newest lines of the time range, so you can see which services the source holds in the Fields column before you narrow the query.
On Loki it runs `{service_name=~".+"}`, which reads every stream that has a `service_name` label and shows the newest lines of the time range; a Loki whose streams carry no `service_name` answers no lines until you pick a label.
The **Builder** segment of the mode switch opens the label browser.
The label browser lists the labels of the datasource and their values, and a click on a value puts that matcher into the stream selector the query already holds, replacing the value of the same label, so the rest of the query stays as it is.
On ClickHouse, or when the query has no selector yet, the click inserts the matcher at the cursor.
With a model configured on the server, the query box opens in Ask mode, as [Ask on the logs page](#ask-on-the-logs-page) describes.

### Query modes

The mode switch at the top left of the query card has up to three segments.
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
A query that selects no `Timestamp`, such as `SELECT ServiceName, count() FROM otel_logs WHERE $__timeFilter(Timestamp) GROUP BY ServiceName ORDER BY count() DESC`, runs too: the rows show as a plain table in your own order and the volume chart is hidden.

Such a query must read the time range through a macro such as `$__timeFilter(...)`, and when it does not, the server refuses it with a sentence that names the macro with the table's time column, such as `$__timeFilter(Timestamp)` on `otel_logs` and `otel_traces`; a macro inside a comment does not count, because it never runs.
The field counts follow the same rule, except that a query selecting `Timestamp` is counted over the time range even without the macro.
So no ClickHouse request reads a table beyond the range at the top of the page.
A syntax error names the line and column in the query as you wrote it and the word there, such as `failed at line 2, column 32 of the query, near "GROUP"`, or says the query ends too early, and never shows the SQL the server wraps around it to page; the trace SQL view reports it the same way.
Without an `ORDER BY` the rows are ordered by all their columns, so a page never repeats or skips a row.
Ctrl+Space in the editor completes names: the columns of `otel_logs` on ClickHouse, the stream labels on Loki and the mapped fields on Elasticsearch and OpenSearch, and on ClickHouse also the SQL keywords.
Press **Run query**, or Ctrl+Enter or Cmd+Enter in the editor, to run it over the time range at the top of the page.
The page shows one page of lines at a time, newest first, 50 by default.

On Loki the query must start with a stream selector.
The link under the query opens the reference for the language of the source: LogQL on Loki, SQL on ClickHouse, and KQL on Elasticsearch and OpenSearch.
When a run fails, the box says "No results." with the time and size of the last good run, and on Elasticsearch and OpenSearch a **Check the datasource** link.
Under the lines a page bar sets 50, 100, 250 or 500 lines per page and moves between pages, as [Paging](#paging) describes.
**Live tail**, **Add to dashboard** and **Add to observation** stay disabled until a query has run, and say "Run a query first."

Outside a text field, `/` focuses the query and `?` lists the keys of the page, which [Keyboard shortcuts](keyboard-shortcuts.md#logs-page-keys) describes.

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
Nothing runs while the query cannot be read, also when a link opens the page with such a query: the box shows the same card, and the volume strip and the results say nothing ran instead of showing a datasource failure.
The **Language** control under the box switches between KQL, Lucene and Query DSL: it adds or removes the `lucene:` prefix, and picking Query DSL on an empty box inserts a `match_all` body.
The editor highlights JSON as soon as the text starts with `{`.

| Construct                        | Example                  | Message                                      | Why it is out                                                                                    |
| -------------------------------- | ------------------------ | -------------------------------------------- | ------------------------------------------------------------------------------------------------ |
| Nested field queries             | `items:{ name:x and qty > 1 }` | "nested queries are not supported"     | They need the `nested` mapping type and a `nested` query.                                        |
| Leading wildcard in a field name | `*name:x`                | "a field name cannot start with \*"          | Kibana expands it from the data view, and the server has no data view.                           |
| Escaped whitespace in a bare word | `message:a\ b`          | "quote a value that contains spaces"         | One escaping rule, quotes, keeps the explorer's filter buttons and the query builder simple.     |
| A `kql:` prefix inside a query   | `kql:a`                  | "kql: is a form prefix, not a query"         | The prefix is read before the parser runs.                                                       |

Inside a quoted value, `\n`, `\t` and `\r` stand for a newline, a tab and a carriage return, `\"` for a quote and `\\` for a backslash.
A query may hold 200 terms, and the values of one `field:(a or b or ...)` list count separately, up to 5,000, so a large multi-value selection still parses.
Date math in a range value, such as `@timestamp >= now-15m`, is passed to the cluster as written, so the server cannot lint it.
A field the mapping does not have is reported as a warning in lint, and the query still runs.
When exactly one mapped field ends the same way as the unknown one, such as `resource.service.name` for `service.name` after you move a query from Elasticsearch to OpenSearch, the lint message adds `did you mean "resource.service.name"?` and the warning under the editor offers **Use resource.service.name**, which renames the field in the query.
KQL values are case sensitive, so when a query such as `service.name:checkout and log.level:error` finds no lines, the server checks the keyword fields its clauses name, three at most, against the values they hold in the range.
When one of them holds `ERROR` where the query says `error`, the empty result says no document has that value, that the field has the other spelling, and lists up to five of the values it does hold; when the field has few enough values to list them all and none matches, it lists them without the case remark.
The server writes that sentence, as `meta.hint` of the query response, so the HTTP API and the MCP tool `query_instant` give the same words: an agent reads it in the step text and in the result's `hint`.

### Share a search

The time range, **Live tail**, **Saved searches**, **Save search** and **Copy link** sit in the top bar.
On a phone the bar keeps the time range and **Live tail**, and **Save search**, **Saved searches** and **Copy link** are under **More actions**.
The address keeps the search as `/logs/<datasource uid>?q=<query>&from=<from>&to=<to>`, so you can share it or bookmark it.
A query written from a question also keeps the question as `ask` beside `q`, and the link opens the query with its question above it.

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
When you run a different query, the pills that are not pinned are cleared and a message says how many, with **Undo** to bring them back and run the query again.
A long pill shortens with an ellipsis inside the card, and hovering it or moving focus to it shows the whole text.
A saved search stores the query with its pills folded in as text.

On Elasticsearch and OpenSearch a pill needs a KQL query, so the row asks you to switch the query to KQL while the text starts with `lucene:` or `{`.
On Loki and ClickHouse only the is and is not operators apply.
A pill the datasource cannot express, or the query cannot take, is marked "not applied" and left out of the query.
On Loki, an exclusion on a stream label stays beside the matcher the query already holds, so `{service_name=~".+"}` with `NOT service_name: frontend` runs as `{service_name=~".+", service_name!="frontend"}`.
The row holds at most 50 pills, and the address holds the pills in about 4,000 characters, so a form that would go past either limit says so under the row and adds nothing.

## Volume

The volume strip above the lines counts every line the query matches over the whole time range by level, not only the lines shown.
On Elasticsearch, OpenSearch and ClickHouse it counts exactly the lines the results list, so the strip total equals the hits or lines total; the first and last bar hold partial buckets.
Its buckets are whole steps that start at or before the start of the range and end at or after its end, so the bars never leave out the newest lines; on Loki the total can run a little above the line count by the lines in the two edge buckets that fall outside the range.
It comes from its own log volume request, which starts once the lines have answered or failed, so one slow source gets one request at a time; when the lines time out the strip says the source is slow instead of starting more requests, and it hides while live tail shows its lines.
The fields list on the logs page and the trace search statistics wait for the results in the same way.
Hover a bar, or focus the strip and walk it with the arrow keys, to see the time and the count for each level in that bucket.
The level chips above the bars show a count for each level, and a chip hides or shows that level in the bars and in the lines of the page you are on, with a note "Hiding Info lines" beside **Sort**; the count of the other chips and the query do not change, and **All** shows every level again.
The chips filter the lines already loaded, not the query, so another page can hold lines of the hidden level; to search for a level, add a filter.

Drag across the strip to select a time, then **Zoom to selection** narrows the time range to it, and **Clear** drops the selection.
Move either edge of the selection with the arrow keys on its handle, or hold Shift while walking the strip to extend it.
On a touch screen the strip shows counts but does not select, because a drag there scrolls the page.
On Loki a range of a day or less is counted in one request, and a longer range one day at a time, newest first and one request after another, so a week does not have to fit in one request.
A bar wider than 30 minutes is counted as the sum of equal windows of at most 30 minutes, because Loki reads a wider window far more slowly.
When the next day would not finish in time, the strip shows the days it counted, shades the older bars as not counted, says "N lines since <time>" and explains that the datasource ran out of time; hovering a shaded bar says "Not counted".
The log volume request then returns `partial: true` with `counted_from`, and `observe datasources log-volume` prints the same note, so an agent knows the older buckets read zero because they were not counted.
When the volume request does not finish, the strip says so with the server's reason in plain words about your query, never the position in the query the server wrapped, and says the lines below are complete only when they loaded, **Retry volume** asks again and **Try the last 15 minutes** narrows the range.

- On Loki a line's level is the first of its `level`, `lvl`, `severity_text`, `severity` and `detected_level` labels, including a `level` that a `| json` or `| logfmt` stage extracts.
- On ClickHouse the level is the `SeverityText` column, counted per `Timestamp` within the time range whatever the query says, so the volume request fails when the query does not select both, and the failure names the column to add.
A datasource's error text reaches the page without the expanded query, the index names behind a data stream and the exception class names, so a typo reads as the problem and its position.
- On Elasticsearch and OpenSearch one aggregation request counts the matching documents per time bucket and per value of the level field, the field the datasource's level role names; with no level field every document counts as other, and a query body with its own `aggs` is refused.

Levels such as `err`, `fatal` and `critical` count as errors, `warn` as warnings, `info` and `notice` as info, and anything else, or no level, as other.

## Fields

The column on the left of the results lists the fields of the datasource: the mapped fields of the index on Elasticsearch and OpenSearch, the stream labels and the fields the query shows on Loki, and the columns of the query on ClickHouse.
The note under **Fields** says where the list comes from, and the search box narrows it by name.
The counts are read after the lines have arrived, so a wide range shows the lines first and the value counts a moment later.
On a range wider than six hours the counts also wait for the volume strip to answer, so the two heaviest requests on a busy source do not run together, and the label names are asked for after the lines have arrived.
When the count times out, the column says so with **Retry** and suggests a shorter range.

A query that matches no lines on Loki shows an empty list, not an error.
Columns you chose for the results come first under **Selected**; the rest are under **Available**.
On Elasticsearch and OpenSearch a `.keyword` subfield is not listed beside the text field it belongs to, and a field filter replaces a query that is only `*` instead of appending to it.
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
Drag the edge of the column, or focus it and press the left and right arrow keys, to make it wider when long field names are cut off; double-click the edge to reset it, and the page remembers the width.
On a window narrower than 1,000 pixels the column does not fit, so a **Fields** button in the results header opens the same list in a drawer, and choosing a value closes the drawer and runs the query.
Below 1,000 pixels wide the column is hidden.

### Facet counts

Facets count the top values of each label, field or column the query matches over the whole time range, through `POST /api/v1/datasources/{uid}/log-facets` and the `log_facets` tool.
Each field says whether its counts are exact or come from a sample of the newest lines.

- On Loki the index labels of the matched streams are counted exactly from the index volume API, in bytes rather than lines, so those fields carry `unit: bytes`; parsed fields (`kind: field`) and structured-metadata fields (`kind: metadata`, the detected fields that no parser produced) are counted from the newest 1,000 lines by default, up to 5,000.
  Those lines are read from the newest five minutes first and then from windows twice as wide further back, until the sample is full, and Loki's detected fields are asked for over only the time the sample covers, so a week does not have to be read whole.
  When time runs short, the fields are counted over the lines read so far, and `lines_sampled` and the note under each field say how many.
  The sample starts at once, beside the index label counts rather than after them, and an index label whose count does not finish in time is counted over the sample instead, with `exact: false` and no unit, and its note says "Counted over the newest N lines", so a slow index still leaves every field with counts.
- On ClickHouse every field is counted exactly with one `GROUP BY` per field: each result column except `Timestamp`, `Body`, `TraceId` and `SpanId`, plus the 20 most frequent `LogAttributes` keys as `LogAttributes.<key>`.
- On Elasticsearch and OpenSearch one aggregation request counts every field exactly over the whole range, with `sampled: false` and `lines_sampled: 0`, and `sample` is ignored.

Without `fields`, the level and service fields come first, then the keyword, boolean, IP and number fields by name, and text fields through their `.keyword` subfield, 20 fields at most.
A field named in `fields` that the mapping lacks answers `kind: unknown`, and a text field without a keyword subfield, an object, a nested field or a field whose type differs between the matched indices answers its kind with no values.
A field says `exact: false` when the server reports a count error bound for it, and `truncated: true` when it has more values than were returned.

## Documents

The documents are the lines the query and its filters match, listed under the volume strip, newest first, 50 to a page.
A row shows the time, the level and the message only: a line's stream labels and structured metadata are not written in front of the message, and the open line lists them all.
Each one is a row you can open, as [Reading a line](#reading-a-line) describes.
Live tail swaps the list for the tail, as [Live tail](#live-tail) describes.

### Results toolbar and columns

The row above the lines holds **Sort**, **Wrap**, **JSON** and **Columns**.
**Sort** picks newest first or oldest first, and clicking the **Time** heading flips it.
**Wrap** wraps long lines and starts on when the page opens on a phone, and **JSON** reformats a line whose body is JSON over several rows.
**Sort**, **Wrap** and **JSON** are your own view of the lines, so **Copy link** does not carry them, and neither a running **Live tail**: the person who opens the link sees the same query, filters, columns, time range, page and open line with their own view settings.
**Columns** lists the fields found in the loaded lines, and each field you tick becomes a column before the line, written to the address as `cols`.
On Elasticsearch and OpenSearch the `service` field is a column until you choose others.
The menu on a column heading sorts the loaded lines A to Z or Z to A, moves the column left or right, or removes it.

### Paging

The bar under the lines says which lines are on the page, such as "51-100 of 48,210 hits in range", and sets how many lines a page holds.
The header above the lines gains ", page 3" from the second page on.
ClickHouse, Elasticsearch and OpenSearch offer numbered pages and an exact total of the lines that match.
Elasticsearch and OpenSearch stop at the largest offset the index pages to, `max_result_window`, and a note under the bar says which page that is and to narrow the time range to reach older lines.
An address that names a page past it opens the last page that can be reached, with a note saying which page was asked for, and keeps the page bar.
Loki pages by time from the last line shown, with **Previous** and **Next** and a running total that reads "at least N" until the last page makes it exact, because a Loki page is reached through the page before it.

The page and size are in the address as `page` and `size`, so a link to a page opens on it; a Loki address beyond page 1 reopens on page 1 with a note, and a query, filter, datasource or time range change returns to page 1.
When a page cannot be read, the bar keeps the last page that loaded and the error shows above it.
On Elasticsearch and OpenSearch a note above the bar says "Some shards did not answer; counts may be low" when the store reports a partial answer.
Live tail replaces the lines and hides the bar.

## Reading a line

Click a line, or focus it and press Enter, to open its details; press `j` and `k` in the list to move to the next and previous line, and press `j` with nothing focused to land on the first line.
Only one line is a Tab stop, the one you last had focus on, so Tab crosses the list in one press.
Opening a line adds `line` to the address, closing it removes `line`, and **Copy link** in the line's details copies an address that opens that line.
When the linked line is not in the results, the page says so and **Clear** removes `line` from the address.

Open a line to see it in the side panel on the right, beside the results, or inline under the line on a screen narrower than 900 pixels.
The panel has a **Table** tab, a **JSON** tab that prints the whole line, and a **Context** tab; press Escape to close it and return to the line.
On Elasticsearch and OpenSearch the panel is titled **Document** and the table groups **Core fields** (time, level, service, trace id, span id and document id) apart from **Other fields**, which hold the rest of the document and its index as `_index`.
A field whose value is a JSON object, such as the ClickHouse `LogAttributes` map, shows one row per key, named `<field>.<key>`, one level deep.
The panel offers **Show context**, **Copy line**, which copies the line's text, and **Copy link**, and an **Open trace** link when the line carries a trace id.

Each field in the table has **Include** and **Exclude** buttons, shown when you hover or focus the row, that add a [filter pill](#filters) for the field and its value, and the page runs the query again with it.
On Elasticsearch and OpenSearch **Require** adds an exists pill for a field that has a value; the core fields have no buttons.
On Loki a stream label becomes a pill that goes into the stream selector, and a parsed field or structured metadata value a pill that goes into a `|` stage at the end of the query; on ClickHouse a column or a `LogAttributes` key goes into the `WHERE` clause.
A button is disabled when the row's pill is already in the row of filters, when the row holds 50 filters, when the value is longer than 200 characters, or on Elasticsearch and OpenSearch when the query is Lucene or Query DSL.

## Context around a line

Open a line and choose **Context** (**Surrounding documents** on Elasticsearch and OpenSearch) to see the 50 lines before it and the 50 lines after it, with the line itself marked between them.
The server reads both sides in one request, anchored on the line's nanosecond time, so the context is exact: the lines just before are the newest ones older than the line, the lines just after are the oldest ones newer than it, within an hour on each side.
The lines come from the same stream labels on Loki, the same `ServiceName` on ClickHouse, and the whole index on Elasticsearch and OpenSearch.
When more lines exist on a side than it shows, **Load 10 older** or **Load 10 newer** adds 10 to that side, up to 500.
**Window** widens the search from an hour to 10 minutes or 24 hours, and a side with nothing in the window says so.
On an Elasticsearch or OpenSearch time field of type `date`, which keeps milliseconds, context is exact to the millisecond, and the lines in the anchor's own millisecond are left out on both sides.
Scripts call the same endpoint, `POST /api/v1/datasources/{uid}/log-context`, with the line's `TimeNs` as `anchor`.

## Live tail

**Live tail** opens one stream to the server, which watches the query and sends new lines as they arrive, so the browser runs no query of its own.
You may hold 5 tails at a time, and a tail reconnects by itself after a dropped connection or at the server's 30 minute limit, from the newest line it already holds, so no line is lost or repeated.
On every datasource the server reads the last 5 seconds again at each poll and drops the lines it already sent, so a line that arrives out of order or is indexed late still appears, and a line stamped in the future does not hide the lines that follow it.
When the store says a poll did not cover everything, such as a failed shard, the stream flags that poll as partial with the store's text, and the next polls ask again from the same point for up to three polls, so the missing lines arrive without repeating the ones already shown.
The page keeps the newest 1,000 lines of the last 5 minutes and drops older ones off the top, and the note under the list says so.

The tail lists them oldest first with the newest at the bottom, so the sort is fixed while it runs and the search order comes back when you leave it.
Lines that just arrived are marked for a moment, and a status line above the list tells when the tail started and how many lines it received.
Scrolling up with the wheel, a touch, the keyboard or the scrollbar pauses the list so it stops moving under the pointer, and the list growing as lines arrive never pauses it, and a button counts what arrived meanwhile.
**Jump to newest**, **Resume** or scrolling back to the bottom resumes it, and a "New since" divider then marks where the lines that arrived during the pause begin, with each of them read out as "new" by a screen reader.
**Clear** empties the lines shown until you start the tail again.

A tail stops when you press **Stop live tail**, when the query, a filter or the time range changes, or when the server cannot run the query.
A tail that cannot start or run shows "Live tail stopped" with the reason, such as the limit of 5 tails, and **Start again**.

On a Loki, ClickHouse, Elasticsearch or OpenSearch datasource the page reads `GET /api/v1/datasources/{uid}/tail?query=...`, which sends Server-Sent Events, polls the datasource every second and ends after 30 minutes; scripts can read the same stream, see the API reference.

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
The page asks for the 20 largest patterns, and when the datasource holds more, **Show 100** appears beside **Show all** and asks for up to 100.

### Pattern templates

Patterns group the lines a query matches into line templates with counts, the variable tokens written `<*>`, through `POST /api/v1/datasources/{uid}/log-patterns` and the `log_patterns` tool, so a noisy service reads as a few templates instead of thousands of lines.

- On a Loki whose pattern ingester is enabled, a query that is a bare stream selector answers from Loki's pattern index over the whole range, and the response says `source: loki`, unless the index counts fewer lines than the sample size, because an ingester that has seen a small share of the lines would claim full coverage; then the server groups the newest lines as below.
- Otherwise, on a Loki without the pattern ingester, a Loki query with filters or stages, or ClickHouse, the server reads the newest 2,000 lines by default, up to 5,000, masks numbers, identifiers, addresses and timestamps, clusters lines that share at least 60% of their tokens, and answers `source: server` with each pattern's newest line as its sample.
- On Elasticsearch the cluster's `categorize_text` aggregation answers over the whole range with `source: elasticsearch`, each pattern being the words its lines share; OpenSearch has no such aggregation, and a cluster without it or without the licence for it falls back to the `source: server` clustering above.

Patterns come by count descending, 20 by default and at most 100, and `truncated` says when more exist, or when the server clustering folded lines into the nearest of its 500 clusters or dropped lines past its 5,000.
Every pattern carries a `trend` of 30 equal buckets over the range, empty ones included with a count of 0, and a `level` (`error`, `warning`, `info`, `other` or empty): for `source: loki` it is the first level word in the pattern, for `source: server` the level most of the pattern's lines carry.

## Add to dashboard

**Add to dashboard**, in the header of the results, keeps the query as a logs panel on a dashboard you can edit.
You need Edit on the target dashboard.
The panel is full width and goes at the bottom of the dashboard, inside the last row when that row is collapsed, and the save makes a new version.
The save runs the query over the dashboard's time range and refuses a panel that returns no lines there.
The panel's menu offers the way back with **Open in Logs** or **Open in Traces**, which opens the explorer with the panel's query and the dashboard's time range.

## Add to an observation

**Add to observation**, in the header of the results beside **Add to dashboard**, copies the search you ran into an [observation](observations.md) as a logs panel named Logs, with the datasource, the query and the time range the page shows.
The side panel offers an open observation or a new one, the same as **Add to observation** in a dashboard panel's menu, and describes what is added and what is refused in [Observations](observations.md#add-to-an-observation-from-a-panel-or-from-logs).
When the page was opened from **Logs** in an observation's row of **Add a block**, its address carries `observation=<id>`, and that observation is preselected when it is open.
The block stays live on the page, so it runs the query when the page is open.

## Paste a Discover link

Paste a Kibana or OpenSearch Dashboards Discover address into the query box or the **Ask** box of an Elasticsearch or OpenSearch datasource, or type it into **Ask** and press Ask (the address is never sent to the model), and the page fills in the query, the time range, the columns and the filters that match one value or a list of values.
The note under the box says what was filled and how many filters from the address were not applied: range, exists and custom filters are left out.
A Lucene query gets the `lucene:` prefix, the source you are on stays selected, and nothing is sent to the server: the browser reads the address itself.
On a Loki datasource or a ClickHouse datasource the address is pasted as plain text.

## Ask on the logs page

The mode switch at the top left of the query card switches between a question, the builder and the query.
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

## Trace view

A trace opens at `/t/<trace id>?ds=<datasource uid>`.
The title in the top bar reads **Traces / <source> / <root service> <root span>**, such as **Traces / Tempo / frontend POST /checkout**, and under it a summary gives the duration, the number of spans, services and errors, the start time, and the slowest service on the critical path with the time it spent there.
The summary ends with the short trace id, and **Copy trace id** beside it copies the whole id.
**Open trace** on a log line opens it when the line carries a `TraceId`, `traceID`, `trace_id` or `traceId` field.
A Loki, Elasticsearch or OpenSearch line opens in the first Tempo datasource, so the trace has to be in that Tempo too, and a ClickHouse line opens in the same ClickHouse datasource.
An address with a trace id and no `ds` opens the only trace datasource when there is one, and otherwise asks which datasource to use.

### Waterfall

The view draws the spans as a waterfall under a ruler that steps by 1, 2, 2.5 or 5 times a power of ten, so its labels are round numbers, also after you zoom, when the first label is then not at the left edge; a very short window shows three round labels, or two or one on a narrow ruler, rather than its two edges.
The view fills the window, and the waterfall scrolls inside it.
Each row is 32 pixels high with the service and operation on the left, the bar in the middle and the span's duration at the right, and a dashboard's traces panel draws the same rows.
When the query returns more than one trace, the panel shows one at a time with **Previous trace** and **Next trace** buttons and a count such as "Trace 2 of 8", and **Open trace** opens it in the trace view over the dashboard's time range.

- **Find in trace** matches a span's name, service, id, status message or attribute values; the matching text is highlighted, the field counts the matches, and Enter opens the next one, Shift+Enter the previous one.
- **Critical path** outlines the chain that decides when the trace ends and dims the rest; the line under the waterfall says how many spans are on it.
  The chain starts at the span that finishes last and includes a span that outlives its parent or starts after the parent ended, such as work the parent started and did not wait for.
- **Errors only** dims every span that neither failed nor contains a failed span.
- **Collapse all** folds every span with children, and **Expand all** opens them again.
- The services legend lists each service with its colour and span count; click a service to dim its spans, and click it again to show them.
- The overview strip above the ruler draws the whole trace in one line per span, on the same time columns as the ruler so a span has the same x in both; drag on it, or on the ruler, to zoom the time axis to that window, and **Reset zoom** shows the whole trace again.

### Span details

A click on a span shows its details in the side panel on the right, or inline under the span on a screen narrower than 900 pixels: the service and the span's kind (server, client, producer, consumer or internal), its name, its status with the message the span set, such as "card declined", its duration, share of the trace and start offset, then its span attributes, its resource attributes and its events.
Each attribute row offers **Copy value** and, on Tempo, **Search traces where** the attribute is, or is not, that value, which opens the trace search with that TraceQL filter.
A value that reads as a number matches both the number and the same text, such as `(span.order.id = 503 || span.order.id = "503")`, because the page cannot tell whether Tempo stored the attribute as a number or as a string.
An event, such as an `exception`, shows its name, its offset from the span's start and its attributes, and the span's bar carries a dot at each event.
A span's links are listed after its events: each shows the linked trace id, which opens that trace on the same datasource at the linked span, the linked span id and the link's attributes.

While a span is open the address carries `span=<span id>`, so a copied link opens on that span with its row scrolled into view.
The waterfall keeps its full width until a span is open, j and k move between spans while the panel stays open, and Escape closes it.
The keys the waterfall answers are listed in [Keyboard shortcuts](keyboard-shortcuts.md).
On a phone each row stacks the service and operation, with the duration at the right, above a thinner bar, so the whole name shows instead of its first few characters; **Logs for this trace**, **Add to dashboard** and **Add to observation** move out of the top bar into a row under the title, and the legend hint wraps instead of scrolling sideways.

### Logs for a span or trace

A span's details end with **Logs for this span**: the first lines that carry its trace id and span id, read from the Loki or ClickHouse logs datasource, with the count of lines found; a datasource that has not answered after 15 seconds ends in an error with **Retry**, never in placeholders that stay.
**Open in Logs** continues with those lines on the logs page, filtered on the trace id and span id over the span's window plus a minute on each side.
When a span logged nothing, **Try the whole trace** opens the trace's logs instead.
**Logs for this trace** in the top bar opens a side panel with every line of the trace, oldest first, up to the newest 500, and the same **Open in Logs** link.
Messages wrap in the panel instead of being cut off.
The panel replaces the span details while it is open, because the page shows one side panel at a time, and opening a span closes it.
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

A trace opened from the Logs page or from a trace search shows **Back to logs** or **Back to traces** under the top bar, which returns to that page with the same query and time range.
A trace opened from a pasted link, a dashboard or after a reload has no such link, because the page it came from is not known.
**Download JSON** and **Copy link** are icon buttons in the top bar, beside the time range and **Logs for this trace**.
**Download JSON** saves the trace as the browser holds it, the trace endpoint's response, as `trace-<trace id>.json`.
**Copy link** copies the address of the trace with its datasource, time range and open span.
**Add to dashboard** and **Add to observation**, in the top bar beside **Logs for this trace**, keep the trace: the first adds a full-width traces panel for it under the last row of a dashboard you can edit, with the datasource and the trace id, and the second copies the trace into an [observation](observations.md) as a traces panel, with the datasource, the trace id and the time range the page shows.
The source name in the title opens the trace search on the same source at `/traces/<datasource uid>`, and a trace opened without a known source shows **Search traces** in the top bar instead.

## Trace search

Open **Traces** in the sidebar to see the trace sources and the saved trace searches, then open a source to find traces by service, span name, minimum and maximum duration, and errors only.
The search runs as soon as the page opens, with the fields the address carries, or with none.
Each trace source, a Tempo or ClickHouse datasource, has its own page at `/traces/<datasource uid>`.
With two or more trace sources they are listed under **Traces** in the sidebar with their language, TraceQL on Tempo and Search on ClickHouse, the five you used last on this browser first and then by name, and **More** opens the Traces list; with one, nothing is listed.
As for logs, the order is set when the app loads and stays fixed until you reload.
The collapsed sidebar and the phone title open the sources as they do for logs, and the title names the source and its language.

`/traces` lists the trace sources and the saved trace searches like `/logs`, including the services each source has seen and the box that filters sources by source or service name, and a service opens the trace search on that source with the service filter set; an address that carries a search but no source opens the last source you used in this browser, else Tempo when one is configured, else the first by name, and an old `/traces?ds=<datasource uid>` address opens `/traces/<datasource uid>`.
The time range, **Live tail**, **Saved searches**, **Save search** and **Copy link** sit in the top bar, and on a phone the bar keeps the time range and **Live tail**, while **Save search**, **Saved searches** and **Copy link** are under **More actions**.
**Live tail**, named as on the Logs page, runs the search and its statistics again every 10 seconds while the page is open and the range is relative, such as the last 15 minutes; press **Stop live tail** to end it.
It is a refresh and not a stream, because the trace sources have no tail, and the address does not carry it, so a shared link opens without it.
A range that starts in the future runs no search and says so; a call that reaches the datasource anyway, from the API, the CLI or MCP, is refused with `time_range_in_future` and the same sentence.

Tempo refuses a search over a longer range than its `max_duration`, 7 days by default; the page then reads "Tempo searches at most 7 days at a time; pick a shorter time range" and offers **Last 7 days**, and the API answers `time_range_too_long` with the same text, so a caller can read the limit from the message and retry with a shorter range.
The limit is named in days when it is a whole number of days, in hours when it is a whole number of hours and otherwise in minutes, and the button uses the same unit.
On an absolute range **Live** is disabled and says "Live works on a relative time range, like the last 15 minutes.", and choosing an absolute range while it runs turns it off.
The fields and the **Search traces** button sit in the query card, with Ctrl+Enter or Cmd+Enter as the button's shortcut.

Durations take a number and a unit, such as `250ms` or `1.5s`.
The minimum and maximum are span durations: a trace matches when a span that also fits the service and span name is that long, so a span name with a minimum of `1s` finds nothing when that span runs for 100 ms, even if a later span makes the whole trace slower.
On Tempo you can also write TraceQL, which replaces the fields: while the TraceQL field has text the other fields are disabled, and only the TraceQL goes into the address.
The link under the TraceQL field opens the TraceQL reference.
On ClickHouse the **Service** field is a box with the services in `otel_traces`: open it for the list, type to narrow it, or type a name that is not listed.
Press **Search traces**, or Enter in Service, Span name or a duration field, to run it; Enter in the TraceQL field adds a line.
A TraceQL syntax error reads "Invalid TraceQL at line 1, column 9 after \"{ name =\": unexpected end of the query.", which names the position and the text before it; the API, the CLI and MCP return the same sentence, without the parser's own `$end` and doubled prefix.

### SQL on ClickHouse

On ClickHouse the query card has a **Search** and **SQL** switch beside **Ask**, and Ctrl+I or Cmd+I moves through them.
**SQL** opens an editor with the search of the fields written as SQL over `otel_traces`, so you can start from it, and Ctrl+Space completes the columns of `otel_traces` and the SQL keywords.
Press **Run query**, or Ctrl+Enter or Cmd+Enter, to run it; the address keeps it as `q`, and a link or a saved search with SQL opens in the SQL view.
It runs through the same path and checks as the SQL of the logs page: one read-only `SELECT`, the time range at the top of the page through a macro such as `$__timeFilter(Timestamp)`, the 15 second query limit and the page size; a query that breaks a rule is refused with a sentence that names it, and one ClickHouse cannot run shows the database's message.
The rows show as a table with a column per selected column, in the order and up to the `LIMIT` the SQL gives, paged like the search, and when the query selects a trace id column, `TraceId` or `trace_id` in any case, each id opens its trace.
**Add to dashboard** adds a full-width table panel with the SQL as written under the last row of a dashboard you can edit, and **Add to observation** copies it into an [observation](observations.md) as the same table panel over the time range the page shows.
The scatter, the statistics and **Compare** belong to the search, so the SQL view does not show them; switch back to **Search** and press **Search traces** to run the fields again.
To draw the spans of one trace from your own SQL, use a traces panel on a dashboard or in an observation, whose query is SQL with `format: "table"` that returns `TraceId AS traceID`, `SpanId AS spanID`, `ParentSpanId AS parentSpanID`, `ServiceName AS serviceName`, `SpanName AS operationName`, `Timestamp AS startTime`, `Duration / 1000000 AS duration` in milliseconds and `SpanAttributes AS tags`.

The label beside the source name in the top bar says **SQL** while the SQL view is open and **Search** otherwise, Ask included, because an answer fills the search fields.

### Results

A count line above the results says how many traces are on the page.
A scatter above the table plots each trace's duration over time on a log scale, red for traces with errors, with the p95 as a dashed line; a dot opens its trace on click, and hovering a dot highlights its row; the dots are not keyboard stops, so Tab goes straight to the table, where each trace has one link.
That p95 is of every trace the search matches in the range, not only the listed ones: the page asks the server for the duration distribution of the matching spans and reads the p95 as the upper edge of the bucket that holds it, so a service filter changes it.
While that answer is loading, or when it fails, the line and the ticks fall back to the p95 of the listed traces, and a failed answer shows a warning icon beside the count line with the reason.

The table opens slowest first, and a column header sorts by that column, ascending, descending, then back to the original order.
On ClickHouse the server sorts every trace the search matches before it pages them, so the first page holds the slowest traces of the range and the sort is in the address as `sort`, such as `sort=-spans`; on Tempo only the traces on the current page are sorted, and the line above the table says so.
Each row shows **Root service**, **Root span**, **Duration** with a bar scaled to the slowest trace shown and a tick at the same p95 as the scatter's line, **Spans**, **Errors** and **Started**, which opens the trace.
**Columns** hides a column.
**Add to dashboard**, beside **Columns**, adds a full-width traces panel with the same query under the last row of a dashboard you can edit, as it does for logs; **Add to observation** copies the search you ran into an [observation](observations.md) as a traces panel over the time range the page shows: the TraceQL on Tempo, or on ClickHouse a query for the spans of the latest matching trace.
**Errors** reads OK, or Failed with the number of error spans.
**Spans** and **Errors** show only when every row carries the count.

- ClickHouse lists each trace that has a matching span as a whole: the root span's service and name, the number of spans in the whole trace, and the full length of the trace from its first start to its last end, so a row agrees with the trace it opens.
  With a filter set, a smaller "N matching" under the span count says how many of those spans matched.
- Tempo shows the root span of each trace, and sums the span and error span counts from Tempo's per-service stats; when Tempo does not report those stats, both columns are left out.

When Tempo did not search every block, a **Partial** note says why and which times the found traces cover, with **Keep only** to narrow the range to them: either the page filled up, which says "Showing the newest 50 traces that match" because the server asks Tempo for the most recent matches first (`with (most_recent=true)`, unless the TraceQL carries its own `with` hints) and a reload shows the same traces, or Tempo stopped at its time limit, so some matching traces may be missing.
On a long range the server searches Tempo newest first in growing windows, the last hour, then the two hours before it, then four, and so on, and stops once the page is full, so a broad search costs about as much as the newest window that fills it.
When the 15 second query limit comes before the range is covered, the server returns the traces it found as a partial result instead of a timeout: the note says these are the newest matches it found and **Next** searches further back, and when it found none the page says Tempo stopped at its time limit before it found a trace, rather than that nothing matched.
The explore query and `query_instant` return the same result with `meta.partial` set and the note that some matches may be missing, which `observe traces --ask` prints under the table.
The scatter then plots only the times the found traces cover, so a full page that sits in a few seconds is not squeezed against the edge of the range.
On a phone the table becomes a list.

The `trace_stats` MCP tool counts spans and errors per time bucket over the same traces.
On Tempo its query is TraceQL; on ClickHouse it is a SQL condition over `otel_traces`, such as `ServiceName = 'checkout' AND Duration > 1000000`.
A statement such as `SELECT 1`, a second statement, a comment or unbalanced parentheses is refused with that example, and a condition ClickHouse cannot run is reported without the SQL the server built around it.

### Paging

The table pages 50 traces at a time, and the bar under it sets 50, 100, 250 or 500 per page.
ClickHouse offers numbered pages and an exact total of matching traces.
Tempo pages by start time from the last trace shown, with **Previous** and **Next** and a running total that reads "at least N" until the last page makes it exact, and no total while a search is partial.
The page and size are in the address as `page` and `size`, and sorting a column resets to page 1.
A Tempo address beyond page 1 reopens on page 1 with a note, because a Tempo page is reached through the page before it, so **Copy link** on a Tempo source leaves the page out and a shared link opens on page 1.

### Compare selected traces

Drag across the duration scatter to select traces by time and duration; the dots outside the selection fade, and the selection's start, end and duration floor are handles that the arrow keys move.
The table then shows only the selection, and **Keep only the selection** turns it into the search: the time range and the duration bounds go into the fields, or on Tempo into the written TraceQL, where the button is disabled when the query has no spanset to add them to.
**Clear** brings every trace back, and both buttons sit in the header of the scatter.

While a selection exists, the **What is different about the selected traces** panel opens beside the scatter, and under it on a screen 1,100 pixels wide or narrower.
It ranks the attribute values that appear in a larger share of the selected traces than of the other matching traces: span names, kinds, statuses and the span and resource attributes, each with its share of both sets.
It reads every span of the newest 50 selected and 150 other loaded traces, says when it could not read them all, and keeps the weaker differences behind **Show weaker differences**.
**Filter** on a row adds that attribute value to the search: on Tempo it appends the condition to the TraceQL, and on ClickHouse it fills the Span name or Service field, so the other attributes have no **Filter** there.
Applying a filter, or **Keep only the selection**, switches the query card to Query so the written TraceQL or the filled fields are visible, and drops the Ask question from the address.

## Ask on the trace search

With a model configured on the server, the trace search has the same **Ask** and **Query** toggle as the logs page, at the top left of the query card, with the question field beside it; on ClickHouse the toggle reads **Ask**, **Search** and **SQL**.
Ctrl+I or Cmd+I switches it, Enter asks, Shift+Enter adds a line and Esc cancels a question that is still running.
The mode you last chose on either page is the one both pages open in, and the trace search opens in Query when the address carries a search field; on ClickHouse it opens in **Search**, or in **SQL** when the address carries SQL.
**Recent** and Up bring back earlier questions, kept apart from those of the logs page.

The answer fills Service, Span name, the durations and Errors only, or on Tempo the TraceQL field when the question needs TraceQL; on ClickHouse it replaces any SQL in the address with those fields.
A duration the form cannot read is left empty.
The search runs at once, and the address keeps the question as `ask` until you change a field and search again, which drops it because the fields are then your own query.
The explanation, the fields the answer filled as chips, such as **Service** with its value or **Min span duration** with `500ms`, and any warnings show under the question, with **Edit the query**, which switches to Query with the answer's values in the form.
The row under the form then shows a "From: <question>" line and **Ask again**, which brings the question back.

When the model cannot turn the question into a query, the page says "I could not turn that into a query." with the reason, switches to Query and puts the cursor in Service.
Switching to another source drops the question, because it was answered for the other one.
Without a model, the page stays in Query, and the Ask side of the toggle is disabled and says "Add a model key to ask in plain language".

## Ask in plain language

Ask turns a question such as "checkout errors in the last hour" into a query for one datasource.
On Loki it writes LogQL, on ClickHouse logs it writes ClickHouse SQL, and on Elasticsearch and OpenSearch it writes KQL, with the field names and values the datasource really has.
It writes a `lucene:` query only when KQL cannot say what the question needs.
For traces it fills the trace search fields: service, span name, minimum and maximum duration, and errors only.
On Tempo it writes TraceQL instead when the question needs more than those fields.
On a Prometheus datasource it writes PromQL, as the [Metrics](metrics.md#ask-in-plain-language) page describes.

The server first reads the datasource's metric names, labels, attributes or columns and some of their values, so the query names real ones.
On Loki it also offers the values of `detected_level`, the structured metadata level that Loki adds, when no level label is listed, so a question about errors filters after the selector with `| detected_level="error"`.
Then it asks the model once.
It checks the answer with the query lint and one sample run over the last 15 minutes of the time range, and never runs the full query.
When that check fails, it asks the model once more to fix the query, so a question costs at most two model calls.

The answer comes with a one-sentence explanation and warnings, such as a query that reads every stream or every document, a Lucene query that the explorer's Builder cannot edit, a KQL field the mapping does not have, a ClickHouse query without `$__timeFilter` or `LIMIT`, or a sample with no matches or, on Prometheus, no series.
When the model cannot write a query for the question, or the fixed query still fails its check, the answer says it could not turn that into a query and why.

Each person can have 3 questions in flight and ask 60 questions per clock hour, and each question stops after 60 seconds.
After the 60th question the answer says to wait, with the minutes left in the hour, and a service-account key counts the same way.
Without a model configured on the server, Ask is off.

## Save a search

A saved search keeps what the logs page or the trace view shows so that anyone with access can open it again: the datasource, the query as it runs with its filter pills folded in, the visible columns of a logs search or the filters of a trace search, and the range as written, such as `now-1h`.
A range written like that stays relative, so the search opens on the latest hour and not on the hour it was saved in.
A logs search takes a Loki, ClickHouse, Elasticsearch or OpenSearch datasource and needs a query.
The query may span lines but holds no NUL character, and a column name holds no control characters; the server answers 422 `invalid_saved_search` with the rule in `error`.
A trace search takes a Tempo or ClickHouse datasource, and its query is optional: TraceQL on Tempo, or on ClickHouse the SQL of the SQL view.

On the logs page and the trace search, **Save search** in the top bar asks for a title and a folder and saves the search you are looking at, and the **Saved searches** box next to it opens one of the searches you can view.
The title starts as the query with all of it selected, so typing a name replaces it.
The logs page saves the query that runs, so the filter pills are folded into the query text and reopening the search shows them as query text, not as pills.
A search opened from the box, from the Logs, Traces or Metrics list or from a folder shows its title and folder above the query, and the address carries its id as `saved`; **Close the saved search** removes it.

When you edit the query, the range, the columns, the trace filters or the metrics view, the line says so and, to someone with Edit on the search, offers **Update saved search**, which replaces the saved query, filters and range with what the page shows and keeps the title and folder; **Save search** still saves a new search.
Someone without Edit sees only that **Save search** saves a new search and the open one stays as it was saved.
The title shows only for a search among the 100 most recently changed that the list holds.
A trace search on a ClickHouse datasource saves its SQL while the SQL view shows a result, and reopens in the SQL view; otherwise it saves the fields of the search.
A trace search whose minimum span duration is above its maximum is refused with 422 `invalid_saved_search` and "min must not be greater than max", because it could only return nothing.

The Logs and Traces lists show the saved searches of their kind, up to the 100 most recently changed, grouped under their folders the way the dashboards list is: your My Dashboards first, then the other folders you can view, each with its access and a chevron that collapses it.
A folder with no saved search of that kind is not listed, and a folder's header counts its searches unless the list is cut at 100.
Each row's menu offers **Rename...**, **Move to folder** and **Delete...** to those with Edit on it.
The folder list offers only the folders where you can create, and a refused save shows the server's message.
On the trace search, **Save search** is disabled and says why when you cannot create in any folder.

A search is saved in a folder, which needs the Create permission there.
Without a folder it goes to your My Dashboards, and a service account saves it in General.
Everyone who can view the folder can open the search, and the folder's permissions decide who may change it, as [Folders](../teams/folders.md) describes.
Renaming, moving, changing what it searches and deleting a saved search need Edit on it, and a move needs Edit on the target folder too.
A search cannot move into a personal folder, because that would make it private without asking: save it there instead.
The kind and the datasource never change, so save a new search to change either; the query, filters and range can be replaced, and the result must pass the same rules as a save.

`POST /api/v1/saved-searches` saves a search, `GET /api/v1/saved-searches` lists the ones in the folders you can view (`kind`, `folder_id`, `limit` and `cursor` narrow and page the list), `PATCH /api/v1/saved-searches/{id}` renames or moves one, or replaces its `query`, `filters`, `from` and `to`, and `DELETE` removes it.
The filters of a logs search hold its `columns`, those of a trace search its service, span, duration and errors-only choices, and those of a metrics search its `mode`; a `filters` value replaces all of them, and `null` clears them.
A save with a service-account key also lints the query and answers 422 `query_invalid` when it has an error, and a repeat of a search that key already saved answers 200 with `replayed` true instead of making a second copy.
`observe saved-searches save`, `list`, `update`, `rename`, `move` and `delete` and the MCP tools `save_search`, `list_saved_searches` and `update_saved_search` reach the same operations; no MCP tool deletes, renames or moves one.
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

`GET /api/v1/datasources/{uid}/services` lists the services a Loki, Tempo, ClickHouse, Elasticsearch or OpenSearch datasource has seen over a recent window, with the label, column or attribute that holds them:

```sh
curl -fsS "$OBSERVE_SERVER/api/v1/datasources/<datasource uid>/services?signal=logs&window=1h" \
  -H "Authorization: Bearer $OBSERVE_TOKEN"
```

The answer has `datasource`, `signal`, `field`, `window_seconds`, `services` and `truncated`.
`field` is the one the server reads as the service: on Loki the first of the labels `service_name`, `service`, `app` and `job` that has values, on Tempo the `resource.service.name` attribute, on ClickHouse the `ServiceName` column, and on Elasticsearch and OpenSearch the service field of the datasource, empty when its mapping has none.
`signal` is `logs` or `traces` and defaults to what the datasource holds, with `logs` first on ClickHouse, which holds both.
`window` is a duration such as `15m`, one minute to six hours, and defaults to one hour.
The server asks the source once, over that window only, and waits at most 5 seconds for it, and keeps the answer for about a minute, so reloading the page does not ask the source again; a source that does not answer in time gives `504` `query_timeout` or `502` `datasource_unavailable` and the failure is not kept, `429` `too_many_requests` means the server's shared upstream slots were busy, and a datasource that does not hold the signal gives `422` `query_invalid`.
It lists at most 200 services and sets `truncated` when more exist.
Every signed-in role from Viewer can read it, because any Viewer can already query any datasource.
In the CLI, `observe datasources services <uid>` lists them, and `observe datasources services --service checkout` asks every source and prints the ones that hold a service whose name contains `checkout`; the MCP tool `list_services` does the same for an agent.

On ClickHouse the explore query refuses a query that does not read the time range through a macro such as `$__timeFilter(Timestamp)` unless it sends `page_size`, and `query_range` and `query_instant` without `page_size` follow the same rule: only a paged query is cut to the range by the server, so a query that selects `Timestamp` still needs the macro there.
The refusal names the macro with the time column of the first table the query reads.
`limit` lowers the row limit and never raises it above 1,000.
With `page_size` on ClickHouse, `sort` names a result column, with a minus in front for descending, such as `-Duration`, and orders the whole result before it is paged; the MCP tool `query_instant` takes the same `sort`.
A cursor remembers the sort it was made with, so send the same `sort` with every cursor of a walk; a cursor sent without it, or with another, is refused with a message naming the sort.
`POST /api/v1/datasources/{uid}/log-volume` takes `query`, `from`, `to` and an optional `buckets` (60 by default, at most 200) and returns the line counts by level per bucket over the range, for Loki, ClickHouse, Elasticsearch and OpenSearch.
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
