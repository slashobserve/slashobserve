---
title: Logs and traces
description: Search logs from Loki, ClickHouse, Elasticsearch or OpenSearch and traces from Tempo, Jaeger or ClickHouse with a query or a plain-language question, follow a line to its trace, tail new lines and keep a query as a dashboard panel.
---

The logs page and the trace view let you read raw log lines and spans without building a dashboard first.
Use them to see what a service logged, follow one request through its trace, or keep a search as a dashboard panel.
They read from the datasources an Admin configured, as described in [Connect Loki](../connect/loki.md), [Connect Tempo](../connect/tempo.md), [Connect Jaeger](../connect/jaeger.md) and [Connect ClickHouse](../connect/clickhouse.md).

Every signed-in role from Viewer can use them.
Queries run on the server, so datasource credentials never reach the browser.

## Search logs

1. Choose **Logs** in the sidebar, then a log source.
   A log source is an Elasticsearch, OpenSearch, Loki or ClickHouse datasource.
   The page opens at `/logs/<datasource uid>`, and its title names the source and its language, such as **Logs / ClickHouse** with an **SQL** tag.
2. Read the lines the page shows at once.
   It runs a starting query over the time range at the top and lists the newest lines, so you see what the source holds before you narrow it.
3. Write a query in the language segment of the query card, then press **Run query**, or press Ctrl+Enter (Cmd+Enter on a Mac) in the editor.
   On Loki, for example:

   ```logql
   {service_name="checkout"} |= "error"
   ```

   You see one page of lines, newest first, with a volume chart above them and the fields of the source on the left.
4. Click a value under **Fields**, or choose **Add filter**, to narrow the search without retyping it.
5. Click a line to open it, then choose **Show context** to read the lines around it, or **Open trace** to follow it to its trace.
6. Choose **Copy link** to share the search, **Save search** to keep it, or **Add to dashboard** to turn it into a panel.

If you would rather describe what you want, switch the mode switch to **Write a query**, as [Ask in plain language](#ask-in-plain-language) describes.
Outside a text field, `/` focuses the query and `?` lists the keys of the page, which [Keyboard shortcuts](keyboard-shortcuts.md#logs-page-keys) describes.

### Choose a source

`/logs` lists the log sources, each with its query language and whether it answers.
Under them it lists the saved logs searches you can view, most recently changed first, with their source, folder, who saved them and when.
A source or a search opens on its own page.

Each source in the list also shows the services it has seen in the last hour.
The list shows the first three names and then "+N more", and a click on "+N more" shows the rest, with **Show fewer** to fold them again.
A service name opens that source with the service filter applied.
On logs that is a pill on the field that holds the service, such as `service_name` on Loki, and for traces it is the service filter.
The box above the list filters the sources by the text in a source name or in a service name, so typing `checkout` leaves the sources that hold a checkout service.
Every source loads its services on its own, so a slow source shows "Services unavailable" for itself and the others fill in as they answer.

Switching to another source keeps the time range and the filter pills and clears the query, unless both are Elasticsearch or OpenSearch, which both speak KQL.
An address that carries a search but no source, such as `/logs?q=...`, opens the source you used last on this browser, else the first by name.
An old `/logs?ds=<datasource uid>` address opens `/logs/<datasource uid>` with the rest of its query string.
[Sources and the sidebar](#sources-and-the-sidebar) describes how the sidebar lists them.

### What the page shows first

The page starts as soon as it opens, with a query for the datasource.

- On ClickHouse it runs a template that reads `otel_logs` and shows the newest lines of the time range.
- On Elasticsearch and OpenSearch it runs `*`, which matches every document, so you can see which services the source holds in the **Fields** column before you narrow the query.
- On Loki it runs `{service_name=~".+"}`, which reads every stream that has a `service_name` label.
  A Loki whose streams carry no `service_name` answers no lines until you pick a label.

With a model configured on the server, the query box opens in Write a query mode instead, as [Ask in plain language](#ask-in-plain-language) describes.

### Query modes

The mode switch at the top left of the query card has up to three segments.

- **Builder** edits the query as filter chips.
- The next segment is named for the datasource's language (**KQL**, **LogQL** or **SQL**) and edits the text.
- **Write a query**, always last, takes a question and writes the query; it does not explain the results.
  To ask what a set of results means, use **Add to observation**, then **Ask** in the block's header, as [Observations](observations.md#comment-and-ask-on-the-blocks) describes.

ClickHouse has no Builder segment, because its query is SQL.
On Loki the Builder segment is the label browser.
It lists the labels of the datasource and their values, and a click on a value puts that matcher into the stream selector the query already holds, replacing the value of the same label, so the rest of the query stays as it is.
On ClickHouse, or when the query has no selector yet, the click inserts the matcher at the cursor.

On Elasticsearch and OpenSearch the Builder is a row of chips with an **Add a filter** button.
A filter picks a field from the index mapping, an operator and a value.
The operators are is, is not, is one of, exists, greater than, at least, less than, at most and contains.
The page suggests values from the newest lines of the time range.
Choosing a filter or removing a chip rewrites the query and runs it.

The Builder shows only a flat list of clauses joined by `and`, with `not` in front of a single field match.
A query with an `or` at the top, a group, a form prefix or text that does not parse greys the segment, which says "The builder cannot show this query. Edit it as KQL."
**Edit as KQL** in the Builder switches to the text without changing it.
The page opens in the language segment when the address carries a query.

### Write and run a query

In the language segment you write the query yourself.
The page shows one page of lines at a time, newest first, 50 by default.
The link under the query opens the reference for the language of the source: LogQL on Loki, SQL on ClickHouse, and KQL on Elasticsearch and OpenSearch.

- On Loki, write a LogQL query that starts with a stream selector, such as `{service_name="checkout"} |= "error"`.
- On ClickHouse, the editor holds the template.
  Keep `$__timeFilter(Timestamp)` in the `WHERE` clause and a `LIMIT` at the end.
- On Elasticsearch and OpenSearch, write KQL, or one of the other two forms in [Query languages on Elasticsearch and OpenSearch](#query-languages-on-elasticsearch-and-opensearch).

Ctrl+Space in the editor completes names.
It completes the columns of `otel_logs` and the SQL keywords on ClickHouse, the stream labels on Loki, and the mapped fields on Elasticsearch and OpenSearch.
Press **Run query**, or Ctrl+Enter or Cmd+Enter in the editor, to run it over the time range at the top of the page.

When a run fails, the box says "No results." with the time and size of the last good run, and on Elasticsearch and OpenSearch a **Check the datasource** link.
Under the lines a page bar sets 50, 100, 250 or 500 lines per page and moves between pages, as [Paging](#paging) describes.
**Live tail**, **Add to dashboard** and **Add to observation** stay disabled until a query has run, and say "Run a query first."

#### ClickHouse rules

A ClickHouse query must read the time range through a macro such as `$__timeFilter(...)`.
When it does not, the server refuses it with a sentence that names the macro with the table's time column, such as `$__timeFilter(Timestamp)` on `otel_logs` and `otel_traces`.
A macro inside a comment does not count, because it never runs.
So no ClickHouse request reads a table beyond the range at the top of the page.

A query that selects no `Timestamp` runs too, and the rows show as a plain table in your own order with the volume chart hidden:

```sql
SELECT ServiceName, count() FROM otel_logs WHERE $__timeFilter(Timestamp) GROUP BY ServiceName ORDER BY count() DESC
```

The field counts follow the same rule, except that a query selecting `Timestamp` is counted over the time range even without the macro.
Without an `ORDER BY` the rows are ordered by all their columns, so a page never repeats or skips a row.

A syntax error names the line and column in the query as you wrote it and the word there, such as `failed at line 2, column 32 of the query, near "GROUP"`.
It can also say the query ends too early.
It never shows the SQL the server wraps around the query to page it, and the trace SQL view reports errors the same way.

### Query languages on Elasticsearch and OpenSearch

An Elasticsearch or OpenSearch datasource runs three query forms.

- KQL is the default and what a Kibana user types.
- A `lucene:` prefix switches to Lucene, the syntax of Kibana's Lucene mode and of Grafana's Elasticsearch targets.
- Text that starts with `{` is a Query DSL body, for aggregations and for a body pasted from Kibana or Grafana.

A `kql:` prefix, or `queryLanguage: lucene` on the datasource, changes which form an unprefixed query is.
The [Elasticsearch](../connect/elasticsearch.md#queries) and [OpenSearch](../connect/opensearch.md#queries) pages describe what each form returns, the aggregations that fold into panels, ad hoc filters, lint and paging.
The **Language** control under the query box switches between KQL, Lucene and Query DSL.
It adds or removes the `lucene:` prefix, and picking Query DSL on an empty box inserts a `match_all` body.
The editor highlights JSON as soon as the text starts with `{`.

#### KQL on Elasticsearch and OpenSearch

The server parses a strict subset of Kibana KQL and refuses the rest with a message at the offset of the construct.
A query that Kibana accepts may therefore not paste.

The query box shows the same check as you type.
It puts a caret under the offending character, says the problem in plain words and, when one edit makes the query valid, shows a button that applies it.

- A missing closing parenthesis is closed before the next top-level `and`, or at the end.
- A query that reads as Lucene, such as one with `+`, `-` or `~` operators, offers **Switch to Lucene**, which adds the `lucene:` prefix and runs the text as written.
- **Rewrite with a query** asks the model to repair the text.
- **Open in the builder** shows it as chips when it can.
- **KQL reference** opens this section.

Nothing runs while the query cannot be read, also when a link opens the page with such a query.
The box shows the same card, and the volume strip and the results say nothing ran instead of showing a datasource failure.

These constructs are refused:

| Construct | Example | Message | Why it is out |
| --- | --- | --- | --- |
| Nested field queries | `items:{ name:x and qty > 1 }` | "nested queries are not supported" | They need the `nested` mapping type and a `nested` query. |
| Leading wildcard in a field name | `*name:x` | "a field name cannot start with \*" | Kibana expands it from the data view, and the server has no data view. |
| Escaped whitespace in a bare word | `message:a\ b` | "quote a value that contains spaces" | One escaping rule, quotes, keeps the explorer's filter buttons and the query builder simple. |
| A `kql:` prefix inside a query | `kql:a` | "kql: is a form prefix, not a query" | The prefix is read before the parser runs. |

Details of what the parser accepts:

- Inside a quoted value, `\n`, `\t` and `\r` stand for a newline, a tab and a carriage return, `\"` for a quote and `\\` for a backslash.
- A query may hold 200 terms, and the values of one `field:(a or b or ...)` list count separately, up to 5,000, so a large multi-value selection still parses.
- Date math in a range value, such as `@timestamp >= now-15m`, is passed to the cluster as written, so the server cannot lint it.
- A field the mapping does not have is reported as a warning in lint, and the query still runs.

When exactly one mapped field ends the same way as the unknown one, the lint message adds a suggestion.
An example is `resource.service.name` for `service.name` after you move a query from Elasticsearch to OpenSearch.
The message then adds `did you mean "resource.service.name"?`, and the warning under the editor offers **Use resource.service.name**, which renames the field in the query.
The suggestion ignores capitals, so `Body` offers `body` and `Service.Name` offers `resource.service.name`.

A query whose text requires a field the mapping does not have, and which finds no lines, says so in `meta.hint`, such as `Field "Body" is not in the mapping; did you mean "body"?`.
When no field is close, the hint lists up to five text fields to try instead, starting with the message field: `Field "Body" is not in the mapping; text fields to try: message, error.stack_trace.`
The hint comes with an instant result, the first page of a paged read and a count over time whose counts are all zero.
A field under `not` or in one branch of an `or` is not named, and neither is a field of an ad-hoc filter added outside the query text.
The agent's `query_instant` and `query_range` show that sentence beside the result, so it names the real field instead of reading the empty result as an absence of data.

KQL values are case sensitive, so a query such as `service.name:checkout and log.level:error` can find no lines when the data says `ERROR`.
The server then checks the keyword fields its clauses name, three at most, against the values they hold in the range.

- When one of them holds `ERROR` where the query says `error`, the empty result says no document has that value, that the field has the other spelling, and lists up to five of the values it does hold.
- When the field has few enough values to list them all and none matches, it lists them without the case remark.

The server writes that sentence as `meta.hint` of the query response.
The HTTP API and the MCP tool `query_instant` give the same words, so an agent reads it in the step text and in the result's `hint`.

### Share a search

The time range, **Live tail**, **Saved searches**, **Save search** and **Copy link** sit in the top bar.
On a phone the bar keeps the time range and **Live tail**, and **Save search**, **Saved searches** and **Copy link** are under **More actions**.
The address keeps the search as `/logs/<datasource uid>?q=<query>&from=<from>&to=<to>`, so you can share it or bookmark it.
A query written from a question also keeps the question as `ask` beside `q`, and the link opens the query with its question above it.
Times on the page are in the time zone of your browser, and a range between two moments reads with that zone after it, such as `Nov 18, 19:53 to 19:59 GMT+5:30`.

## Narrow the lines with filters

A filter pill narrows the query without retyping it.
The row under the query box shows one pill for each filter, such as `service.name: checkout` or `NOT host.name: canary-*`.

1. Choose **Add filter** under the query box.
2. Pick a field, an operator and a value.
   The field and the value suggest what the datasource holds in the time range, and the operators are is, is not, is one of, exists and between.
3. Choose **Add filter** in the form.
   The page runs the query again with the pill.

The pills are not part of the text in the query box.
The results, the log volume, the fields and the patterns all read the query with the active pills added.
The address keeps the pills as `f` beside `q`, so a shared link carries both.

### Change a pill

Pick a pill to change it.

- **Exclude** turns it into its negation and **Include** turns it back.
- **Pin** keeps it when you change the query, and **Unpin** lets the next query clear it.
- **Pause** keeps the pill in the row but leaves it out of the query until you pick **Resume**.
- **Edit** opens the form with the pill's values.
- **Delete** removes it.

When you run a different query, the pills that are not pinned are cleared and a message says how many, with **Undo** to bring them back and run the query again.
A long pill shortens with an ellipsis inside the card, and hovering it or moving focus to it shows the whole text.
A saved search stores the query with its pills folded in as text.

### How a pill is applied

A pill is applied in the language of the datasource.

- On Elasticsearch and OpenSearch it adds a KQL clause to the query.
  It needs a KQL query, so the row asks you to switch the query to KQL while the text starts with `lucene:` or `{`.
- On Loki it goes into the stream selector or a `|` stage.
  An exclusion on a stream label stays beside the matcher the query already holds, so `{service_name=~".+"}` with `NOT service_name: frontend` runs as `{service_name=~".+", service_name!="frontend"}`.
- On ClickHouse it goes into the `WHERE` clause.
- On Loki and ClickHouse only the is and is not operators apply.

A pill the datasource cannot express, or the query cannot take, is marked "not applied" and left out of the query.
The row holds at most 50 pills, and the address holds the pills in about 4,000 characters.
A form that would go past either limit says so under the row and adds nothing.

## Read the volume chart

The volume strip above the lines counts every line the query matches over the whole time range by level, not only the lines shown.
Hover a bar, or focus the strip and walk it with the arrow keys, to see the time and the count for each level in that bucket.

The level chips above the bars show a count for each level.
A chip hides or shows that level in the bars and in the lines of the page you are on, with a note "Hiding Info lines" beside **Sort**.
The count of the other chips and the query do not change, and **All** shows every level again.
The chips filter the lines already loaded, not the query, so another page can hold lines of the hidden level.
To search for a level, add a filter.

Levels such as `err`, `fatal` and `critical` count as errors, `warn` as warnings, `info` and `notice` as info, and anything else, or no level, as other.

### Zoom to a time

1. Drag across the strip to select a time.
2. Choose **Zoom to selection** to narrow the time range to it, or **Clear** to drop the selection.

Move either edge of the selection with the arrow keys on its handle, or hold Shift while walking the strip to extend it.
On a touch screen the strip shows counts but does not select, because a drag there scrolls the page.

### When the volume is slow or partial

The strip comes from its own log volume request, which starts once the lines have answered or failed, so one slow source gets one request at a time.
The fields list on the logs page and the trace search statistics wait for the results in the same way.
The strip hides while live tail shows its lines.

- When the lines time out, the strip says the source is slow instead of starting more requests.
- When the volume request does not finish, the strip says so with the server's reason in plain words about your query, never the position in the query the server wrapped.
  It also says the lines below are complete only when they loaded.
  **Retry volume** asks again and **Try the last 15 minutes** narrows the range.
- On Loki, when the next day would not finish in time, the strip shows the days it counted and shades the older bars as not counted.
  It says "N lines since <time>" and explains that the datasource ran out of time, and hovering a shaded bar says "Not counted".
  The log volume request then returns `partial: true` with `counted_from`, and `observe datasources log-volume` prints the same note, so an agent knows the older buckets read zero because they were not counted.

[How the volume is counted](#how-the-volume-is-counted) has the exact rules for each datasource.

## Browse fields and their values

The column on the left of the results lists the fields of the datasource.
The note under **Fields** says where the list comes from, and the search box narrows it by name.

- On Elasticsearch and OpenSearch it lists the mapped fields of the index.
- On Loki it lists the stream labels and the fields the query shows.
- On ClickHouse it lists the columns of the query.

Columns you chose for the results come first under **Selected**, and the rest are under **Available**.
A badge beside each name says its kind: keyword, text, number, date, boolean, IP, or on Loki a label, a parsed field or structured metadata.
A query that matches no lines on Loki shows an empty list, not an error.

### See the top values of a field

Open a field to see its top five values with their counts and a bar scaled to the largest.
The line under the values says how they were counted.

- Where the datasource counts, the counts are exact and cover every matching line.
- Where it cannot, the counts come from the newest 1,000 lines and the line says so.
  On Loki that is the parsed fields and the structured metadata.
- Loki counts its index labels in bytes, not lines, and shows a size.
- A field the query does not return, or that has no countable values, says "No values in this range."

The counts are read after the lines have arrived, so a wide range shows the lines first and the value counts a moment later.
On a range wider than six hours the counts also wait for the volume strip to answer, so the two heaviest requests on a busy source do not run together.
The label names are asked for after the lines have arrived.
When the count times out, the column says so with **Retry** and suggests a shorter range.

### Keep or drop the lines that have a value

Click a value to keep only the lines that have it, or its exclude button to drop them.
The page rewrites the query and runs it, so **Back** returns to the previous one.

- On Elasticsearch and OpenSearch the page adds a KQL clause.
  A Lucene or Query DSL query is not rewritten, and its buttons stay disabled.
- On Loki the page adds a `| field="value"` stage to a log query.
- On ClickHouse the page adds a condition to the `WHERE` clause, and a query without one keeps its buttons disabled.

On Elasticsearch and OpenSearch a `.keyword` subfield is not listed beside the text field it belongs to.
A field filter replaces a query that is only `*` instead of appending to it.

### Resize or hide the column

**Hide fields** folds the column to a strip with **Show fields**, and the page remembers the choice.
Drag the edge of the column, or focus it and press the left and right arrow keys, to make it wider when long field names are cut off.
Double-click the edge to reset it, and the page remembers the width.
Below 1,000 pixels wide the column is hidden, and a **Fields** button in the results header opens the same list in a drawer.
Choosing a value closes the drawer and runs the query.

## Read the lines

The lines the query and its filters match are listed under the volume strip, newest first, 50 to a page.
A row shows the time, the level and the message only.
A line's stream labels and structured metadata are not written in front of the message, and the open line lists them all.
Each row opens as [Open a line](#open-a-line) describes.
Live tail swaps the list for the tail, as [Follow new lines with live tail](#follow-new-lines-with-live-tail) describes.

### Results toolbar and columns

The row above the lines holds **Sort**, **Wrap**, **JSON** and **Columns**.

- **Sort** picks newest first or oldest first, and clicking the **Time** heading flips it.
- **Wrap** wraps long lines, and starts on when the page opens on a phone.
- **JSON** reformats a line whose body is JSON over several rows.
- **Columns** lists the fields found in the loaded lines.
  Each field you tick becomes a column before the line, written to the address as `cols`.

On Elasticsearch and OpenSearch the `service` field is a column until you choose others.
The menu on a column heading sorts the loaded lines A to Z or Z to A, moves the column left or right, or removes it.

**Sort**, **Wrap** and **JSON** are your own view of the lines, so **Copy link** does not carry them, and neither a running **Live tail**.
The person who opens the link sees the same query, filters, columns, time range, page and open line with their own view settings.

### Paging

The bar under the lines says which lines are on the page, such as "51-100 of 48,210 hits in range", and sets how many lines a page holds.
The header above the lines gains ", page 3" from the second page on.

- ClickHouse, Elasticsearch and OpenSearch offer numbered pages and an exact total of the lines that match.
- Elasticsearch and OpenSearch stop at the largest offset the index pages to, `max_result_window`.
  A note under the bar says which page that is and to narrow the time range to reach older lines.
  An address that names a page past it opens the last page that can be reached, with a note saying which page was asked for, and keeps the page bar.
- Loki pages by time from the last line shown, with **Previous** and **Next**.
  Its running total reads "at least N" until the last page makes it exact, because a Loki page is reached through the page before it.

The page and size are in the address as `page` and `size`, so a link to a page opens on it.
A Loki address beyond page 1 reopens on page 1 with a note, and a query, filter, datasource or time range change returns to page 1.
When a page cannot be read, the bar keeps the last page that loaded and the error shows above it.
On Elasticsearch and OpenSearch a note above the bar says "Some shards did not answer; counts may be low" when the store reports a partial answer.
Live tail replaces the lines and hides the bar.

## Open a line

Click a line, or focus it and press Enter, to open its details.
Press `j` and `k` in the list to move to the next and previous line, and press `j` with nothing focused to land on the first line.
Only one line is a Tab stop, the one you last had focus on, so Tab crosses the list in one press.
Press Escape to close the details and return to the line.

The details open in the side panel on the right, beside the results, or inline under the line on a screen narrower than 900 pixels.
The panel has three tabs.

- **Table** lists the fields of the line.
- **JSON** prints the whole line.
- **Context** shows the lines around it, as [Context around a line](#context-around-a-line) describes.

The panel also offers **Show context**, **Copy line**, which copies the line's text, **Copy link**, and an **Open trace** link when the line carries a trace id.
Opening a line adds `line` to the address, closing it removes `line`, and **Copy link** in the line's details copies an address that opens that line.
When the linked line is not in the results, the page says so and **Clear** removes `line` from the address.

On Elasticsearch and OpenSearch the panel is titled **Document**.
Its table groups **Core fields** (time, level, service, trace id, span id and document id) apart from **Other fields**, which hold the rest of the document and its index as `_index`.
A field whose value is a JSON object, such as the ClickHouse `LogAttributes` map, shows one row per key, named `<field>.<key>`, one level deep.

### Filter from a field of the line

Each field in the table has **Include** and **Exclude** buttons, shown when you hover or focus the row.
They add a [filter pill](#narrow-the-lines-with-filters) for the field and its value, and the page runs the query again with it.

- On Elasticsearch and OpenSearch **Require** adds an exists pill for a field that has a value.
  The core fields have no buttons.
- On Loki a stream label becomes a pill that goes into the stream selector.
  A parsed field or structured metadata value becomes a pill that goes into a `|` stage at the end of the query.
- On ClickHouse a column or a `LogAttributes` key goes into the `WHERE` clause.

A button is disabled in these cases:

- The row's pill is already in the row of filters.
- The row holds 50 filters.
- The value is longer than 200 characters.
- On Elasticsearch and OpenSearch, the query is Lucene or Query DSL.

## Context around a line

Open a line and choose **Context** (**Surrounding documents** on Elasticsearch and OpenSearch) to see the 50 lines before it and the 50 lines after it, with the line itself marked between them.

- When more lines exist on a side than it shows, **Load 10 older** or **Load 10 newer** adds 10 to that side, up to 500.
- **Window** widens the search from an hour to 10 minutes or 24 hours, and a side with nothing in the window says so.

The server reads both sides in one request, anchored on the line's nanosecond time, so the context is exact.
The lines just before are the newest ones older than the line, and the lines just after are the oldest ones newer than it, within an hour on each side.
The lines come from the same stream labels on Loki, the same `ServiceName` on ClickHouse, and the whole index on Elasticsearch and OpenSearch.

On an Elasticsearch or OpenSearch time field of type `date`, which keeps milliseconds, context is exact to the millisecond.
The lines in the anchor's own millisecond are left out on both sides.

Scripts call `POST /api/v1/datasources/{uid}/log-context` with the line's `TimeNs` as `anchor`, as [Scripts and the CLI](#scripts-and-the-cli) describes.
The MCP tool `log_context` does the same for an agent.

## Follow new lines with live tail

**Live tail** shows new lines as they arrive, without rerunning the query by hand.

1. Run a query.
2. Choose **Live tail** in the top bar.
3. Watch the lines arrive.
   The tail lists them oldest first with the newest at the bottom, so the sort is fixed while it runs and the search order comes back when you leave it.
4. Press **Stop live tail** to end it.

Lines that just arrived are marked for a moment, and a status line above the list tells when the tail started and how many lines it received.
**Clear** empties the lines shown until you start the tail again.
The page keeps the newest 1,000 lines of the last 5 minutes and drops older ones off the top, and the note under the list says so.

Scrolling up with the wheel, a touch, the keyboard or the scrollbar pauses the list, so it stops moving under the pointer.
The list growing as lines arrive never pauses it, and a button counts what arrived meanwhile.
**Jump to newest**, **Resume** or scrolling back to the bottom resumes it.
A "New since" divider then marks where the lines that arrived during the pause begin, and a screen reader reads each of them as "new".

A tail stops when you press **Stop live tail**, when the query, a filter or the time range changes, or when the server cannot run the query.
A tail that cannot start or run shows "Live tail stopped" with the reason, such as the limit of 5 tails, and **Start again**.

### How a tail runs

**Live tail** opens one stream to the server, which watches the query and sends new lines as they arrive, so the browser runs no query of its own.
The page reads `GET /api/v1/datasources/{uid}/tail?query=...` on a Loki, ClickHouse, Elasticsearch or OpenSearch datasource.
The stream sends Server-Sent Events, polls the datasource every second and ends after 30 minutes.
[Live tail over HTTP](#live-tail-over-http) lists what a script can read from the same stream.

- You may hold 5 tails at a time.
- A tail reconnects by itself after a dropped connection or at the server's 30 minute limit, from the newest line it already holds, so no line is lost or repeated.
- On every one of these datasources the server reads the last 5 seconds again at each poll and drops the lines it already sent.
  A line that arrives out of order or is indexed late still appears, and a line stamped in the future does not hide the lines that follow it.
- When the store says a poll did not cover everything, such as a failed shard, the stream flags that poll as partial with the store's text.
  The next polls ask again from the same point for up to three polls, so the missing lines arrive without repeating the ones already shown.

## Group lines into patterns

The **Patterns** tab beside **Documents** groups the lines the query matches into templates and counts them.
Use it to read a noisy service as a few templates instead of thousands of lines.
The tab shows how many patterns there are once they have loaded, and it stays disabled until a query has run.

A template keeps the fixed words of a line and writes each number or id as `<*>`.
Each row shows these:

- the template and its count;
- its share of the matching lines;
- a small chart of how often it occurred across the time range, with its change over the range ("new" for a pattern that appeared late);
- a dot for its level.

The heading says how many patterns there are and what share of the documents they cover.
**Sort patterns** orders the rows by most lines or by newest.

The table shows the nine largest patterns and every pattern with at least 1% of the lines.
The rest fold under "9 of 14 patterns shown, 5 cover under 1% each" until you press **Show all**.
The page asks for the 20 largest patterns, and when the datasource holds more, **Show 100** appears beside **Show all** and asks for up to 100.

### Keep or drop the lines of a pattern

**Filter** keeps only the lines of a pattern and **Filter out** drops them.
Both return to **Documents** with the rewritten query, so **Back** returns to the previous one.

- On Loki the page adds a regular expression line filter (`|~` or `!~`) after the existing stages.
- On ClickHouse it adds a `match(Body, ...)` condition in the `WHERE` clause.
- On Elasticsearch and OpenSearch it adds KQL clauses on `message` for the fixed words of the pattern.

A query the page cannot rewrite, such as a Lucene or Query DSL query or a ClickHouse query without a `WHERE`, keeps both buttons disabled.
**Samples** shows the newest line of a pattern when the server read the lines itself.

### Where patterns come from

The note under the table says which source applies.

- Loki answers from its pattern index over the whole range when the pattern ingester is on and the query is a bare stream selector.
- Otherwise, and on ClickHouse, the page groups the newest 2,000 lines itself, and the counts are for that sample only.
- Elasticsearch groups the matching documents with its `categorize_text` aggregation when the cluster has it.
- OpenSearch, and a cluster without `categorize_text`, are grouped by the page like ClickHouse.

[Pattern templates](#pattern-templates) describes the same grouping for scripts and agents.

## Add to dashboard

**Add to dashboard**, in the header of the results, keeps the query as a logs panel on a dashboard you can edit.
You need Edit on the target dashboard.
The panel is full width and goes at the bottom of the dashboard, inside the last row when that row is collapsed, and the save makes a new version.
The save runs the query over the dashboard's time range and refuses a panel that returns no lines there.
The panel's menu offers the way back with **Open in Logs**, **Open in Traces** or **Open in Metrics**, which opens the explorer with the panel's query and the dashboard's time range.

## Add to an observation

**Add to observation**, in the header of the results beside **Add to dashboard**, copies the search you ran into an [observation](observations.md) as a logs panel named Logs.
The panel keeps the datasource, the query and the time range the page shows.
The block stays live on the page, so it runs the query when the page is open.

The side panel offers an open observation or a new one, the same as **Add to observation** in a dashboard panel's menu.
[Observations](observations.md#add-to-an-observation-from-a-panel-or-from-logs) describes what is added and what is refused.
When the page was opened from **Logs** in an observation's row of **Add a block**, its address carries `observation=<id>`, and that observation is preselected when it is open.

## Paste a Discover link

Paste a Kibana or OpenSearch Dashboards Discover address into the query box or the **Write a query** box of an Elasticsearch or OpenSearch datasource.
You can also type it into **Write a query** and press **Write query**, and the address is never sent to the model.
The page fills in the query, the time range, the columns and the filters that match one value or a list of values.

The note under the box says what was filled and how many filters from the address were not applied.
Range, exists and custom filters are left out.
A Lucene query gets the `lucene:` prefix, the source you are on stays selected, and nothing is sent to the server, because the browser reads the address itself.
On a Loki or ClickHouse datasource the address is pasted as plain text.

## Ask in plain language

Ask turns a question such as "checkout errors in the last hour" into a query for one datasource.
Use it when you know what you want to see but not the query language.
It works on the logs page, the trace search and the [metrics page](metrics.md#ask-in-plain-language).

Ask is off without a model configured on the server.
[LLM providers](../connect/llm-providers.md) describes how to configure one.
Without one, the Write a query segment is disabled and says "Add a model key to ask in plain language".

### Ask on the logs page

1. Switch the mode switch at the top left of the query card to **Write a query**.
2. Write a question and press Enter or **Write query**.
   Shift+Enter adds a line, and Esc cancels a question that is still running.
3. The page runs the answer at once.
   The explanation and any warnings show under the question, and the query shows in the editor below them, where you can still change it.

On Elasticsearch and OpenSearch the model answers in KQL, and the Builder then shows the answer as chips when it can.
Ctrl+I or Cmd+I steps through the modes the datasource offers from anywhere on the page, the editor included, and skips a mode that is not available.
The page remembers the mode you last chose with the switch, the shortcut or **Write again**, and opens in the query when the address carries one.

Editing the query switches to the query segment, with a "From: <question>" line and **Write again**, which brings the question back.
A label from the label browser also switches to the query segment.
When the model cannot turn the question into a query, the page says "I could not turn that into a query." with the reason, and switches to the query segment.

While the field is empty it lists your 5 latest questions under **Recent**, and Up at the start of the field steps back through earlier ones.
This browser keeps the recent questions, apart from the chat's.

### Ask on the trace search

With a model configured, the trace search has the same toggle at the top left of the query card, with the question field beside it.
The toggle reads **TraceQL** and **Write a query** on Tempo, **Search** and **Write a query** on Jaeger, and on ClickHouse **Search**, **SQL** and **Write a query**.

1. Switch to **Write a query**, or press Ctrl+I or Cmd+I.
2. Write a question and press Enter.
   Shift+Enter adds a line, and Esc cancels a question that is still running.
3. The search runs at once.

The answer fills Service, Span name, the durations and Errors only, or on Tempo the TraceQL field when the question needs TraceQL.
On ClickHouse it replaces any SQL in the address with those fields.
A duration the form cannot read is left empty.

The explanation, the fields the answer filled as chips, such as **Service** with its value or **Min span duration** with `500ms`, and any warnings show under the question.
**Edit the query** switches to Query with the answer's values in the form.
The row under the form then shows a "From: <question>" line and **Write again**, which brings the question back.
The address keeps the question as `ask` until you change a field and search again, which drops it because the fields are then your own query.

The mode you last chose on either page is the one both pages open in.
The trace search opens in Query when the address carries a search field.
On ClickHouse it opens in **Search**, or in **SQL** when the address carries SQL.
**Recent** and Up bring back earlier questions, kept apart from those of the logs page.

When the model cannot turn the question into a query, the page says "I could not turn that into a query." with the reason, switches to Query and puts the cursor in Service.
Switching to another source drops the question, because it was answered for the other one.
Without a model, the page stays in Query, and the Ask side of the toggle is disabled.

### What Ask writes

- On Loki it writes LogQL.
- On ClickHouse logs it writes ClickHouse SQL.
- On Elasticsearch and OpenSearch it writes KQL, and a `lucene:` query only when KQL cannot say what the question needs.
- On a Prometheus datasource it writes PromQL, as the [Metrics](metrics.md#ask-in-plain-language) page describes.
- For traces it fills the trace search fields: service, span name, minimum and maximum duration, and errors only.
  On Tempo it writes TraceQL instead when the question needs more than those fields.

The query uses the field names and values the datasource really has.

### How Ask checks the answer

The server first reads the datasource's metric names, labels, attributes or columns and some of their values, so the query names real ones.
On Loki it also offers the values of `detected_level`, the structured metadata level that Loki adds, when no level label is listed.
A question about errors then filters after the selector with `| detected_level="error"`.

Then it asks the model once.
It checks the answer with the query lint and one sample run over the last 15 minutes of the time range, and never runs the full query.
When that check fails, it asks the model once more to fix the query, so a question costs at most two model calls.

The answer comes with a one-sentence explanation and warnings.
A warning can say any of these:

- The query reads every stream or every document.
- A Lucene query cannot be edited in the explorer's Builder.
- A KQL field is not in the mapping.
- A ClickHouse query has no `$__timeFilter` or `LIMIT`.
- The sample has no matches or, on Prometheus, no series.

When the model cannot write a query for the question, or the fixed query still fails its check, the answer says it could not turn that into a query and why.

### Limits on questions

- Each person can have 3 questions in flight and ask 60 questions per clock hour.
- After the 60th question the answer says to wait, with the minutes left in the hour.
- A service-account key counts the same way.
- Each question stops after 60 seconds.
- A question is at most 2,000 characters.
- The server answers at most 10 questions at once, and past that the answer is `agent_busy`.

[Ask over HTTP](#ask-over-http) lists the error codes.

## Trace view

A trace opens at `/t/<trace id>?ds=<datasource uid>`.
Use it to see where a request spent its time, which span failed, and what the service logged while it ran.

1. Open a trace from a row of the [trace search](#trace-search), from **Open trace** on a log line, or from a traces panel.
2. Read the summary under the title.
   It gives the duration, the number of spans, services and errors, the start time, and the slowest service on the critical path with the time it spent there.
3. Click a span in the waterfall to read its details in the side panel.
4. Choose **Logs for this trace** in the top bar to read what the services logged.

The title in the top bar reads **Traces / <source> / <root service> <root span>**, such as **Traces / Tempo / frontend POST /checkout**.
The summary ends with the short trace id, and **Copy trace id** beside it copies the whole id.

**Open trace** on a log line opens it when the line carries a `TraceId`, `traceID`, `trace_id` or `traceId` field.
The value must be a trace id of 16 or 32 hexadecimal characters that is not all zeros.
A Loki, Elasticsearch or OpenSearch line opens in the first Tempo datasource, or in the first Jaeger datasource when there is no Tempo, so the trace has to be in that datasource too.
A ClickHouse line opens in the same ClickHouse datasource.
When the line also carries a `SpanId`, `spanID`, `span_id` or `spanId` field holding a span id of 16 hexadecimal characters that is not all zeros, the trace opens with that span selected.
An address with a trace id and no `ds` opens the only trace datasource when there is one, and otherwise asks which datasource to use.

### Waterfall

The view draws the spans as a waterfall under a ruler, and the waterfall scrolls inside the window.
Each row is 32 pixels high with the service and operation on the left, the bar in the middle and the span's duration at the right.
A dashboard's traces panel draws the same rows.
When the query returns more than one trace, the panel shows one at a time with **Previous trace** and **Next trace** buttons and a count such as "Trace 2 of 8".
**Open trace** in the panel opens it in the trace view over the dashboard's time range.

The controls above and beside the waterfall:

- **Find in trace** matches a span's name, service, id, status message or attribute values.
  The matching text is highlighted, the field counts the matches, and Enter opens the next one, Shift+Enter the previous one.
- **Critical path** outlines the chain that decides when the trace ends and dims the rest, and the line under the waterfall says how many spans are on it.
  The chain starts at the span that finishes last and includes a span that outlives its parent or starts after the parent ended, such as work the parent started and did not wait for.
- **Errors only** dims every span that neither failed nor contains a failed span.
- **Collapse all** folds every span with children, and **Expand all** opens them again.
- The services legend lists each service with its colour and span count.
  Click a service to dim its spans, and click it again to show them.
- The overview strip above the ruler draws the whole trace in one line per span, on the same time columns as the ruler, so a span has the same x in both.
  Drag on it, or on the ruler, to zoom the time axis to that window, and **Reset zoom** shows the whole trace again.

The ruler steps by 1, 2, 2.5 or 5 times a power of ten, so its labels are round numbers, also after you zoom, when the first label is then not at the left edge.
A very short window shows three round labels, or two or one on a narrow ruler, rather than its two edges.

### Span details

A click on a span shows its details in the side panel on the right, or inline under the span on a screen narrower than 900 pixels.
The details list these, in order:

- the service and the span's kind (server, client, producer, consumer or internal);
- its name;
- its status with the message the span set, such as "card declined";
- its duration, share of the trace and start offset;
- its span attributes, its resource attributes and its events.

Each attribute row offers **Copy value** and, on Tempo, **Search traces where** the attribute is, or is not, that value, which opens the trace search with that TraceQL filter.
A value that reads as a number matches both the number and the same text, such as `(span.order.id = 503 || span.order.id = "503")`, because the page cannot tell whether Tempo stored the attribute as a number or as a string.

An event, such as an `exception`, shows its name, its offset from the span's start and its attributes, and the span's bar carries a dot at each event.
A span's links are listed after its events.
Each link shows the linked trace id, which opens that trace on the same datasource at the linked span, the linked span id and the link's attributes.

While a span is open the address carries `span=<span id>`, so a copied link opens on that span with its row scrolled into view.
The waterfall keeps its full width until a span is open, `j` and `k` move between spans while the panel stays open, and Escape closes it.
[Keyboard shortcuts](keyboard-shortcuts.md) lists the keys the waterfall answers.

On a phone each row stacks the service and operation, with the duration at the right, above a thinner bar, so the whole name shows instead of its first few characters.
**Logs for this trace**, **Add to dashboard** and **Add to observation** move out of the top bar into a row under the title, and the legend hint wraps instead of scrolling sideways.

### Logs for a span or trace

A span's details end with a **Logs for this span** section.
It shows the first lines that carry the span's trace id and span id, read from the Loki or ClickHouse logs datasource, with the count of lines found.
A datasource that has not answered after 15 seconds ends in an error with **Retry**, never in placeholders that stay.

- **Open in Logs** continues with those lines on the logs page, filtered on the trace id and span id over the span's window plus a minute on each side.
- When a span logged nothing, **Try the whole trace** opens the trace's logs instead.
- **Logs for this trace** in the top bar opens a side panel with every line of the trace, oldest first, up to the newest 500, and the same **Open in Logs** link.
  Messages wrap in the panel instead of being cut off.
  The panel replaces the span details while it is open, because the page shows one side panel at a time, and opening a span closes it.

A ClickHouse trace reads its logs from a ClickHouse datasource, and a Tempo trace from Loki.
With more than one logs datasource, a select in the section picks one, and the browser remembers the choice.
The built-in audit datasource is never one of them, so a server with no other logs datasource shows the prompt to connect one.

### Time range and large traces

The view looks for the spans in the last 24 hours, or within an hour of the line when you came from the logs page.
A trace outside the range says so and offers **Last 7 days** to widen it.
The view loads the whole trace up to 20,000 spans.

A trace with many repeated sibling spans, such as a batch job's, folds every set of more than 20 siblings with the same service and operation into one hatched row.
The row carries their count, their total and slowest durations and how many failed, so the timeline still adds up.

- A banner says the trace folds, the find field searches folded spans too, and the services legend counts them.
- Open a folded row to see its slowest 50 spans, and **Show 50 more** for the rest.
- A search match or a linked span inside a folded group opens the group to it.

A trace over 20,000 spans opens with its earliest 20,000 spans by start time.
A banner says how many the trace holds and that the rest are not loaded, and **Critical path** is off for it, because the path needs every span.
When Tempo did not search every block, a banner says that some spans may be missing.

### Export and share

A trace opened from the Logs page or from a trace search shows **Back to logs** or **Back to traces** under the top bar, which returns to that page with the same query and time range.
A trace opened from a pasted link, a dashboard or after a reload has no such link, because the page it came from is not known.

- **Download JSON**, an icon button in the top bar beside the time range, saves the trace as the browser holds it, the trace endpoint's response, as `trace-<trace id>.json`.
- **Copy link** copies the address of the trace with its datasource, time range and open span.
- **Add to dashboard** adds a full-width traces panel for the trace under the last row of a dashboard you can edit, with the datasource and the trace id.
- **Add to observation** copies the trace into an [observation](observations.md) as a traces panel, with the datasource, the trace id and the time range the page shows.

The source name in the title opens the trace search on the same source at `/traces/<datasource uid>`.
A trace opened without a known source shows **Search traces** in the top bar instead.

## Trace search

The trace search finds traces by service, span name, minimum and maximum duration, and errors only.
Use it to find the slow or failing requests of a service, then open one in the trace view.

1. Open **Traces** in the sidebar, then a trace source: a Tempo, Jaeger or ClickHouse datasource.
2. Fill in **Service**, **Span name**, a duration or **Errors only**, or leave them empty.
3. Press **Search traces**, or Ctrl+Enter or Cmd+Enter.
   The search also runs as soon as the page opens, with the fields the address carries, or with none.
4. Read the table, slowest first.
   Click the **Started** time of a trace, or its dot in the scatter, to open it.

Each trace source has its own page at `/traces/<datasource uid>`.
The title names the source and its language.
On Tempo you can write TraceQL instead of using the fields, and on ClickHouse you can write SQL.

`/traces` lists the trace sources and the saved trace searches like `/logs`.
That includes the services each source has seen and the box that filters sources by source or service name.
A service opens the trace search on that source with the service filter set.
An address that carries a search but no source opens the last source you used in this browser, else Tempo when one is configured, else the first by name.
An old `/traces?ds=<datasource uid>` address opens `/traces/<datasource uid>`.

The time range, **Live tail**, **Saved searches**, **Save search** and **Copy link** sit in the top bar.
On a phone the bar keeps the time range and **Live tail**, while **Save search**, **Saved searches** and **Copy link** are under **More actions**.
The fields and the **Search traces** button sit in the query card.

### Set the filters

Durations take a number and a unit, such as `250ms` or `1.5s`.
The minimum and maximum are span durations.
A trace matches when a span that also fits the service and span name is that long.
A span name with a minimum of `1s` therefore finds nothing when that span runs for 100 ms, even if a later span makes the whole trace slower.

On ClickHouse and Jaeger the **Service** field is a box with the services in `otel_traces` or in Jaeger.
Open it for the list, type to narrow it, or type a name that is not listed.
Press **Search traces**, or Enter in Service, Span name or a duration field, to run it.

### Write TraceQL on Tempo

On Tempo you can write TraceQL, which replaces the fields.
While the TraceQL field has text the other fields are disabled, and only the TraceQL goes into the address.
The link under the TraceQL field opens the TraceQL reference.
Enter in the TraceQL field adds a line.

A TraceQL syntax error reads "Invalid TraceQL at line 1, column 9 after \"{ name =\": unexpected end of the query."
It names the position and the text before it.
The API, the CLI and MCP return the same sentence, without the parser's own `$end` and doubled prefix.

### Search a Jaeger source

On Jaeger the same place takes a **Search** written as `key=value` pairs, such as `service=checkout http.status_code=500`.
It replaces the fields the same way and is how you search on any tag.
[Connect Jaeger](../connect/jaeger.md#queries) lists the keys.

- Jaeger searches an operation or a tag only within one service, so a search with `operation` or a tag and no `service` shows the problem instead of running.
- With no filter set the page searches with `minDuration=0s`, which Jaeger answers with its newest traces.

### Keep the search live

**Live tail**, named as on the Logs page, runs the search and its statistics again every 10 seconds while the page is open and the range is relative, such as the last 15 minutes.
Press **Stop live tail** to end it.
It is a refresh and not a stream, because the trace sources have no tail, and the address does not carry it, so a shared link opens without it.

On an absolute range **Live** is disabled and says "Live works on a relative time range, like the last 15 minutes."
Choosing an absolute range while it runs turns it off.

### Ranges the sources refuse

A range that starts in the future runs no search and says so.
A call that reaches the datasource anyway, from the API, the CLI or MCP, is refused with `time_range_in_future` and the same sentence.

Tempo refuses a search over a longer range than its `max_duration`, 7 days by default.
The page then reads "Tempo searches at most 7 days at a time; pick a shorter time range" and offers **Last 7 days**.
The API answers `time_range_too_long` with the same text, so a caller can read the limit from the message and retry with a shorter range.
The limit is named in days when it is a whole number of days, in hours when it is a whole number of hours and otherwise in minutes, and the button uses the same unit.

### SQL on ClickHouse

On ClickHouse the query card has a **Search** and **SQL** switch beside **Write a query**, and Ctrl+I or Cmd+I moves through them.
**SQL** opens an editor with the search of the fields written as SQL over `otel_traces`, so you can start from it.
Ctrl+Space completes the columns of `otel_traces` and the SQL keywords.
Press **Run query**, or Ctrl+Enter or Cmd+Enter, to run it.
The address keeps it as `q`, and a link or a saved search with SQL opens in the SQL view.

It runs through the same path and checks as the SQL of the logs page.

- It is one read-only `SELECT`.
- It reads the time range at the top of the page through a macro such as `$__timeFilter(Timestamp)`.
- The 15 second query limit and the page size apply.
- A query that breaks a rule is refused with a sentence that names it, and one ClickHouse cannot run shows the database's message.

The rows show as a table with a column per selected column, in the order and up to the `LIMIT` the SQL gives, paged like the search.
When the query selects a trace id column, `TraceId` or `trace_id` in any case, each id opens its trace.

**Add to dashboard** adds a full-width table panel with the SQL as written under the last row of a dashboard you can edit.
**Add to observation** copies it into an [observation](observations.md) as the same table panel over the time range the page shows.
The scatter, the statistics and **Compare** belong to the search, so the SQL view does not show them.
Switch back to **Search** and press **Search traces** to run the fields again.

The label beside the source name in the top bar says **SQL** while the SQL view is open and **Search** otherwise, Ask included, because an answer fills the search fields.

To draw the spans of one trace from your own SQL, use a traces panel on a dashboard or in an observation.
Its query is SQL with `format: "table"` that returns these columns:

- `TraceId AS traceID`
- `SpanId AS spanID`
- `ParentSpanId AS parentSpanID`
- `ServiceName AS serviceName`
- `SpanName AS operationName`
- `Timestamp AS startTime`
- `Duration / 1000000 AS duration`, in milliseconds
- `SpanAttributes AS tags`

### Results

A count line above the results says how many traces are on the page.
A scatter above the table plots each trace's duration over time on a log scale, red for traces with errors, with the p95 as a dashed line.
A dot opens its trace on click, and hovering a dot highlights its row.
The dots are not keyboard stops, so Tab goes straight to the table, where each trace has one link.

That p95 is of every trace the search matches in the range, not only the listed ones, so a service filter changes it.
The page asks the server for the duration distribution of the matching spans and reads the p95 as the upper edge of the bucket that holds it.
While that answer is loading, or when it fails, the line and the ticks fall back to the p95 of the listed traces.
A failed answer shows a warning icon beside the count line with the reason.

The table opens slowest first, and a column header sorts by that column, ascending, descending, then back to the original order.
Each row shows these columns:

- **Root service** and **Root span**.
- **Duration**, with a bar scaled to the slowest trace shown and a tick at the same p95 as the scatter's line.
  When that p95 is above the slowest trace shown, the bars have no tick.
  Every duration takes the width of the longest one, up to 16 characters, so the bar starts after the number and never runs under it.
- **Spans** and **Errors**.
  **Errors** reads OK, or Failed with the number of error spans.
  Both show only when every row carries the count.
- **Started**, which opens the trace.

**Columns** hides a column.
The arrow keys move between the cells of the table, and Enter on a **Started** cell opens that trace.
On Jaeger the search shows no duration tick and has no comparison, because Jaeger has no trace statistics.
On a phone the table becomes a list of cards: the root service on top, then each other column with its name.
The column headers are hidden there, but a screen reader still reads them and sorts by them.
Above 100 traces the table scrolls in its own box, which fills the rest of the window and is at least 384 pixels tall, and draws only the rows in view.

On ClickHouse the server sorts every trace the search matches before it pages them, so the first page holds the slowest traces of the range.
The sort is in the address as `sort`, such as `sort=-spans`.
On Tempo and Jaeger only the traces on the current page are sorted, and the line above the table says so.

**Add to dashboard**, beside **Columns**, adds a full-width traces panel with the same query under the last row of a dashboard you can edit, as it does for logs.
**Add to observation** copies the search you ran into an [observation](observations.md) as a traces panel over the time range the page shows.
That panel holds the TraceQL on Tempo, or on ClickHouse a query for the spans of the latest matching trace.

#### What each source puts in a row

- ClickHouse lists each trace that has a matching span as a whole: the root span's service and name, the number of spans in the whole trace, and the full length of the trace from its first start to its last end, so a row agrees with the trace it opens.
  With a filter set, a smaller "N matching" under the span count says how many of those spans matched.
- Jaeger shows the root span of each trace with the span and error span counts of the whole trace, from the summary Jaeger keeps for it.
- Tempo shows the root span of each trace, and sums the span and error span counts from Tempo's per-service stats.
  When Tempo does not report those stats, both columns are left out.

#### Partial results on Tempo

When Tempo did not search every block, a **Partial** note says why and which times the found traces cover, with **Keep only** to narrow the range to them.
There are two causes.

- The page filled up.
  The note says "Showing the newest 50 traces that match", because the server asks Tempo for the most recent matches first, and a reload shows the same traces.
  It adds `with (most_recent=true)` unless the TraceQL carries its own `with` hints.
- Tempo stopped at its time limit, so some matching traces may be missing.

On a long range the server searches Tempo newest first in growing windows, the last hour, then the two hours before it, then four, and so on.
It stops once the page is full, so a broad search costs about as much as the newest window that fills it.

When the 15 second query limit comes before the range is covered, the server returns the traces it found as a partial result instead of a timeout.
The note says these are the newest matches it found and **Next** searches further back.
When it found none, the page says Tempo stopped at its time limit before it found a trace, rather than that nothing matched.
The explore query and `query_instant` return the same result with `meta.partial` set and the note that some matches may be missing, which `observe traces --ask` prints under the table.
The scatter then plots only the times the found traces cover, so a full page that sits in a few seconds is not squeezed against the edge of the range.

#### Count spans over time

The `trace_stats` MCP tool counts spans and errors per time bucket over the same traces, on Tempo and ClickHouse.

- On Tempo its query is TraceQL.
- On ClickHouse it is a SQL condition over `otel_traces`, such as `ServiceName = 'checkout' AND Duration > 1000000`.
  A statement such as `SELECT 1`, a second statement, a comment or unbalanced parentheses is refused with that example.
  A condition ClickHouse cannot run is reported without the SQL the server built around it.

### Paging

The table pages 50 traces at a time, and the bar under it sets 50, 100, 250 or 500 per page.
The page and size are in the address as `page` and `size`, and sorting a column resets to page 1.

- ClickHouse offers numbered pages and an exact total of matching traces.
- Tempo pages by start time from the last trace shown, with **Previous** and **Next**.
  Its running total reads "at least N" until the last page makes it exact, and there is no total while a search is partial.
  A Tempo address beyond page 1 reopens on page 1 with a note, because a Tempo page is reached through the page before it.
  So **Copy link** on a Tempo source leaves the page out, and a shared link opens on page 1.
- Jaeger pages by position, like ClickHouse, but only through the newest traces up to the datasource's row limit.
  A page past that is refused with a message that says to narrow the range or the search.

### Compare selected traces

Use a selection to find what the slow or failing traces have in common.

1. Drag across the duration scatter to select traces by time and duration.
   The dots outside the selection fade, and the selection's start, end and duration floor are handles that the arrow keys move.
2. Read the table, which now shows only the selection.
3. Choose **Keep only the selection** to turn it into the search.
   **Clear** brings every trace back, and both buttons sit in the header of the scatter.

**Keep only the selection** puts the time range and the duration bounds into the fields.
On Tempo it puts them into the written TraceQL, and the button is disabled when the query has no spanset to add them to.
On Jaeger the bounds replace the `minDuration` and `maxDuration` pairs of the written search.

While a selection exists, the **What is different about the selected traces** panel opens beside the scatter, and under it on a screen 1,100 pixels wide or narrower.
Jaeger does not offer it.
It ranks the attribute values that appear in a larger share of the selected traces than of the other matching traces.
Those are span names, kinds, statuses and the span and resource attributes, each with its share of both sets.
It reads every span of the newest 50 selected and 150 other loaded traces, says when it could not read them all, and keeps the weaker differences behind **Show N weaker differences**.

**Filter** on a row adds that attribute value to the search.
On Tempo it appends the condition to the TraceQL, and on ClickHouse it fills the Span name or Service field, so the other attributes have no **Filter** there.
Applying a filter, or **Keep only the selection**, switches the query card to Query so the written TraceQL or the filled fields are visible, and drops the Ask question from the address.

## Save a search

A saved search keeps what the logs page, the trace search or the metrics page shows, so that anyone with access can open it again.
It stores these:

- the datasource;
- the query as it runs, with its filter pills folded in;
- the visible columns of a logs search, the filters of a trace search, or the graph or table choice of a metrics search;
- the range as written, such as `now-1h`.

A range written like that stays relative, so the search opens on the latest hour and not on the hour it was saved in.

1. Run the search on the logs page, the trace search or the metrics page.
2. Choose **Save search** in the top bar.
3. Enter a title and choose a folder.
   The title starts as the query with all of it selected, so typing a name replaces it.
4. Save it.
   To open it again, choose **Saved searches** next to it, or open it from the Logs, Traces or Metrics list or from a folder.

A search opened that way shows its title and folder above the query, and the address carries its id as `saved`.
On the logs page and the metrics page, opening a saved search from **Saved searches** while the query bar is on **Ask** switches it to the query, so the saved query shows, including when it is the search already open.
**Close the saved search** removes it.
The title shows only for a search among the 100 most recently changed that the list holds.

### Change a saved search

When you edit the query, the range, the columns, the trace filters or the metrics view, the line says so.
To someone with Edit on the search it offers **Update saved search**, which replaces the saved query, filters and range with what the page shows and keeps the title and folder.
**Save search** still saves a new search.
Someone without Edit sees only that **Save search** saves a new search, and the open one stays as it was saved.

The Logs and Traces lists show the saved searches of their kind, up to the 100 most recently changed.
They are grouped under their folders the way the dashboards list is.
Your My Dashboards comes first, then the other folders you can view, each with its access and a chevron that collapses it.
A folder with no saved search of that kind is not listed, and a folder's header counts its searches unless the list is cut at 100.
Each row's menu offers **Rename...**, **Move to folder** and **Delete...** to those with Edit on it.

### Who can save and change one

A search is saved in a folder, which needs the Create permission there.
The folder list offers only the folders where you can create, and a refused save shows the server's message.
On the trace search, **Save search** is disabled and says why when you cannot create in any folder.
Without a folder a search goes to your My Dashboards, and a service account saves it in General.

- Everyone who can view the folder can open the search, and the folder's permissions decide who may change it, as [Folders](../teams/folders.md) describes.
- Renaming, moving, changing what it searches and deleting a saved search need Edit on it, and a move needs Edit on the target folder too.
- A search cannot move into a personal folder, because that would make it private without asking.
  Save it there instead.
- The kind and the datasource never change, so save a new search to change either.
  The query, filters and range can be replaced, and the result must pass the same rules as a save.
- Deleting a folder that still holds a saved search is refused, and deleting a user deletes the searches in their personal folder.

### What a saved search holds

A logs search takes a Loki, ClickHouse, Elasticsearch or OpenSearch datasource and needs a query.
The logs page saves the query that runs, so the filter pills are folded into the query text and reopening the search shows them as query text, not as pills.

A trace search takes a Tempo, Jaeger or ClickHouse datasource, and its query is optional.
The query is TraceQL on Tempo, `key=value` pairs on Jaeger, or on ClickHouse the SQL of the SQL view.
A trace search on a ClickHouse datasource saves its SQL while the SQL view shows a result, and reopens in the SQL view.
Otherwise it saves the fields of the search.
A trace search whose minimum span duration is above its maximum is refused with 422 `invalid_saved_search` and "min must not be greater than max", because it could only return nothing.

A metrics search takes a Prometheus datasource, as [Metrics](metrics.md#save-a-search) describes.

The server answers 422 `invalid_saved_search` with the rule in `error` when a save breaks one of these rules:

| Field | Rule |
| --- | --- |
| `title` | At most 120 characters. |
| `query` | Not blank, at most 8,000 characters, no NUL character. It may span lines. |
| `columns` | At most 20 names of a logs search, each 1 to 256 characters with no comma or control character. |
| Trace filters | Only `service`, `span`, `min`, `max` and `errors`. `service` and `span` are at most 200 characters with no control character, and `min` and `max` are durations such as `250ms` or `2s`. |
| `from`, `to` | At most 64 characters. |

### Saved searches over HTTP

- `POST /api/v1/saved-searches` saves a search.
- `GET /api/v1/saved-searches` lists the ones in the folders you can view.
  `kind`, `folder_id`, `limit` (1 to 100, 20 by default) and `cursor` narrow and page the list.
- `PATCH /api/v1/saved-searches/{id}` renames or moves one, or replaces its `query`, `filters`, `from` and `to`.
- `DELETE /api/v1/saved-searches/{id}` removes it.

The filters of a logs search hold its `columns`, those of a trace search its service, span, duration and errors-only choices, and those of a metrics search its `mode`.
A `filters` value replaces all of them, and `null` clears them.
Each saved search carries the `url` of the page that reopens it, and each change is recorded in [Activity](activity.md).

A save with a service-account key also lints the query and answers 422 `query_invalid` when it has an error.
A repeat of a search that key already saved answers 200 with `replayed` true instead of making a second copy.

`observe saved-searches save`, `list`, `update`, `rename`, `move` and `delete` reach the same operations.
So do the MCP tools `save_search`, `list_saved_searches` and `update_saved_search`, and no MCP tool deletes, renames or moves one.

## Reference

### Sources and the sidebar

With two or more log sources they are listed under **Logs** in the sidebar, each with its query language.
The languages are SQL on ClickHouse, LogQL on Loki and KQL on Elasticsearch and OpenSearch.
The trace sources work the same way under **Traces**, with TraceQL on Tempo and Search on Jaeger and ClickHouse.

- The five you used last on this browser come first and the rest follow by name, and **More** opens the Logs list or the Traces list.
- The order is set when the app loads and stays fixed until you reload, so opening a source does not move it, and the next load puts it first.
- A source added or removed meanwhile goes by name without moving the others.
- With one source nothing is listed under it, and **Logs** or **Traces** still opens the list.
- On the collapsed sidebar the Logs icon opens a menu with a **Logs** link and the same sources.
  The Traces icon does the same.
- On a phone the source line under the page title opens the sources.

For an Admin the built-in audit datasource, `slashobserve-audit`, is in none of the sidebar's lists, the collapsed sidebar's menu and the `/logs` list.
The default source for a Logs link without one never is it.
These open it:

- **Open in Logs** on the page under **Audit**, then **Log**;
- its address `/logs/slashobserve-audit`;
- the phone's source menu;
- the command palette.

The services of each source come from `GET /api/v1/datasources/{uid}/services`, as [Scripts and the CLI](#scripts-and-the-cli) describes, and the server caches them for about a minute.

A trace source's list at `/traces` also shows its services.
The same list and filter box apply, and a service opens the trace search on that source with the service filter set.

### Limits and roles

- Every signed-in role from Viewer can use the logs page, the trace view, the trace search and Ask.
- Each query stops after 15 seconds and returns at most 1,000 rows.
- The queries run on the server, so datasource credentials never reach the browser.

### How the volume is counted

The volume strip counts per level, and each datasource counts in its own way.

- On Elasticsearch, OpenSearch and ClickHouse it counts exactly the lines the results list, so the strip total equals the hits or lines total.
  The first and last bar hold partial buckets.
- Its buckets are whole steps that start at or before the start of the range and end at or after its end, so the bars never leave out the newest lines.
  On Loki the total can run a little above the line count, by the lines in the two edge buckets that fall outside the range.
- On Loki a range of a day or less is counted in one request.
  A longer range is counted one day at a time, newest first and one request after another, so a week does not have to fit in one request.
- On Loki a bar wider than 30 minutes is counted as the sum of equal windows of at most 30 minutes, because Loki reads a wider window far more slowly.
- On Loki a line's level is the first of its `level`, `lvl`, `severity_text`, `severity` and `detected_level` labels, including a `level` that a `| json` or `| logfmt` stage extracts.
- On ClickHouse the level is the `SeverityText` column, counted per `Timestamp` within the time range whatever the query says.
  The volume request fails when the query does not select both, and the failure names the column to add.
- On Elasticsearch and OpenSearch one aggregation request counts the matching documents per time bucket and per value of the level field, the field the datasource's level role names.
  With no level field every document counts as other, and a query body with its own `aggs` is refused.

A datasource's error text reaches the page without the expanded query, the index names behind a data stream and the exception class names, so a typo reads as the problem and its position.

`POST /api/v1/datasources/{uid}/log-volume` takes `query`, `from`, `to` and an optional `buckets` (60 by default, at most 200).
It returns `step_ms`, `total`, the `buckets` with their `error`, `warning`, `info` and `other` counts, and `partial` with `counted_from`.
It works for Loki, ClickHouse, Elasticsearch and OpenSearch.

### Facet counts

Facets count the top values of each label, field or column the query matches over the whole time range.
They run through `POST /api/v1/datasources/{uid}/log-facets` and the `log_facets` tool, and `observe logs --facets` prints them.
Each field says whether its counts are exact or come from a sample of the newest lines.

| Parameter | Meaning |
| --- | --- |
| `query`, `from`, `to`, `timezone` | The query and its range. |
| `fields` | Count only these fields, at most 50, each at most 200 bytes. |
| `limit` | Values per field, 1 to 50, 10 by default. |
| `sample` | Lines to sample where the datasource cannot count exactly, 100 to 5,000, 1,000 by default. |

The counting differs by datasource.

- On Loki the index labels of the matched streams are counted exactly from the index volume API, in bytes rather than lines, so those fields carry `unit: bytes`.
  Parsed fields (`kind: field`) and structured-metadata fields (`kind: metadata`, the detected fields that no parser produced) are counted from the newest 1,000 lines by default, up to 5,000.
- Those Loki lines are read from the newest five minutes first and then from windows twice as wide further back, until the sample is full.
  Loki's detected fields are asked for over only the time the sample covers, so a week does not have to be read whole.
  When time runs short, the fields are counted over the lines read so far, and `lines_sampled` and the note under each field say how many.
- The Loki sample starts at once, beside the index label counts rather than after them.
  An index label whose count does not finish in time is counted over the sample instead, with `exact: false` and no unit, and its note says "Counted over the newest N lines".
  A slow index still leaves every field with counts.
- On ClickHouse every field is counted exactly with one `GROUP BY` per field.
  That is each result column except `Timestamp`, `Body`, `TraceId` and `SpanId`, plus the 20 most frequent `LogAttributes` keys as `LogAttributes.<key>`.
- On Elasticsearch and OpenSearch one aggregation request counts every field exactly over the whole range, with `sampled: false` and `lines_sampled: 0`, and `sample` is ignored.

Without `fields`, the level and service fields come first, then the keyword, boolean, IP and number fields by name, and text fields through their `.keyword` subfield, 20 fields at most.
A field named in `fields` that the mapping lacks answers `kind: unknown`.
A text field without a keyword subfield, an object, a nested field or a field whose type differs between the matched indices answers its kind with no values.
A field says `exact: false` when the server reports a count error bound for it, and `truncated: true` when it has more values than were returned.

### Pattern templates

Patterns group the lines a query matches into line templates with counts, the variable tokens written `<*>`.
They run through `POST /api/v1/datasources/{uid}/log-patterns` and the `log_patterns` tool, and `observe logs --patterns` prints them.
A noisy service reads as a few templates instead of thousands of lines.

| Parameter | Meaning |
| --- | --- |
| `query`, `from`, `to`, `timezone` | The query and its range. |
| `limit` | Patterns to return, 1 to 100, 20 by default. |
| `sample` | Lines the server reads itself, 100 to 5,000, 2,000 by default. |

The `source` of the answer says where the patterns came from.

- `source: loki`.
  On a Loki whose pattern ingester is enabled, a query that is a bare stream selector answers from Loki's pattern index over the whole range.
  If the index counts fewer lines than the sample size, the server groups the newest lines as below instead, because an ingester that has seen a small share of the lines would claim full coverage.
- `source: server`.
  On a Loki without the pattern ingester, a Loki query with filters or stages, or ClickHouse, the server reads the newest 2,000 lines by default, up to 5,000.
  It masks numbers, identifiers, addresses and timestamps, and clusters lines that share at least 60% of their tokens.
  Each pattern's newest line is its sample.
- `source: elasticsearch`.
  The cluster's `categorize_text` aggregation answers over the whole range, each pattern being the words its lines share.
  OpenSearch has no such aggregation, and a cluster without it or without the licence for it falls back to the `source: server` clustering.

Patterns come by count descending, and `truncated` says when more exist.
It also says when the server clustering folded lines into the nearest of its 500 clusters or dropped lines past its 5,000.
Every pattern carries a `trend` of 30 equal buckets over the range, empty ones included with a count of 0, and a `level` (`error`, `warning`, `info`, `other` or empty).
For `source: loki` the level is the first level word in the pattern, and for `source: server` it is the level most of the pattern's lines carry.

### Scripts and the CLI

Each page has an HTTP route, and most have a CLI command and an MCP tool.

| What | HTTP | CLI | MCP |
| --- | --- | --- | --- |
| Run a query | `POST /api/v1/datasources/{uid}/query` | `observe logs`, `observe traces`, `observe metrics query` | `query_range`, `query_instant` |
| List services | `GET /api/v1/datasources/{uid}/services` | `observe datasources services` | `list_services` |
| Count by level | `POST /api/v1/datasources/{uid}/log-volume` | `observe datasources log-volume` | none |
| Top values | `POST /api/v1/datasources/{uid}/log-facets` | `observe logs --facets` | `log_facets` |
| Group into patterns | `POST /api/v1/datasources/{uid}/log-patterns` | `observe logs --patterns` | `log_patterns` |
| Read around a line | `POST /api/v1/datasources/{uid}/log-context` | `observe logs --context` | `log_context` |
| Follow new lines | `GET /api/v1/datasources/{uid}/tail` | `observe logs --tail` | none |
| Ask a question | `POST /api/v1/datasources/{uid}/draft-query` | `--ask` on `observe logs`, `observe traces` and `observe metrics query` | `draft_query` |

`observe logs --tail` polls every 2 seconds and does not read the HTTP stream.
[The CLI](cli.md) describes `observe logs` and `observe traces`, with `--ask` for questions.

#### Run a query

Scripts run the same queries as the page with `POST /api/v1/datasources/{uid}/query` and a service-account key:

```sh
curl -fsS -X POST "$OBSERVE_SERVER/api/v1/datasources/<datasource uid>/query" \
  -H "Authorization: Bearer $OBSERVE_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"query":"{service_name=\"checkout\"}","kind":"instant","from":"now-15m","to":"now","limit":100}'
```

The request takes these fields.

| Field | Meaning |
| --- | --- |
| `query` | The query text, 1 byte to 64 KiB. |
| `kind` | `range` or `instant`. |
| `from`, `to` | The range, both required. |
| `timezone` | A time zone name for relative times. |
| `max_data_points` | The most points of a range query. Not allowed with `instant`. |
| `limit` | Lowers the row limit, 1 to 1,000. It never raises it. |
| `page_size` | Pages an instant query, 50 to 500. It cannot be combined with `limit`. |
| `page`, `cursor` | The page to read, with `page_size`. |
| `sort` | A result column, with a minus in front for descending, such as `-Duration`. Needs `page_size`, and applies to ClickHouse only. |
| `as_written` | ClickHouse only. Returns the rows as the query wrote them, with its own `ORDER BY`, `LIMIT` and columns and no `TimeNs` field. Needs `page_size`. |

On ClickHouse the explore query refuses a query that does not read the time range through a macro such as `$__timeFilter(Timestamp)` unless it sends `page_size`.
`query_range` and `query_instant` without `page_size` follow the same rule.
Only a paged query is cut to the range by the server, so a query that selects `Timestamp` still needs the macro there.
The refusal names the macro with the time column of the first table the query reads.

With `page_size` on ClickHouse, `sort` orders the whole result before it is paged, and the MCP tool `query_instant` takes the same `sort`.
A cursor remembers the sort it was made with, so send the same `sort` with every cursor of a walk.
A cursor sent without it, or with another, is refused with a message naming the sort.

#### List the services of a source

`GET /api/v1/datasources/{uid}/services` lists the services a Loki, Tempo, Jaeger, ClickHouse, Elasticsearch or OpenSearch datasource has seen over a recent window, with the label, column or attribute that holds them:

```sh
curl -fsS "$OBSERVE_SERVER/api/v1/datasources/<datasource uid>/services?signal=logs&window=1h" \
  -H "Authorization: Bearer $OBSERVE_TOKEN"
```

The answer has `datasource`, `signal`, `field`, `window_seconds`, `services` and `truncated`.

- `field` is the one the server reads as the service.
  On Loki it is the first of the labels `service_name`, `service`, `app` and `job` that has values.
  On Tempo it is the `resource.service.name` attribute, and on Jaeger its list of services.
  On ClickHouse it is the `ServiceName` column.
  On Elasticsearch and OpenSearch it is the service field of the datasource, empty when its mapping has none.
- `signal` is `logs` or `traces` and defaults to what the datasource holds, with `logs` first on ClickHouse, which holds both.
- `window` is a duration such as `15m`, one minute to six hours, and defaults to one hour.
  Jaeger ignores it and lists every service it retains.
- It lists at most 200 services and sets `truncated` when more exist.

The server asks the source once, over that window only, and waits at most 5 seconds for it.
It keeps the answer for about a minute, so reloading the page does not ask the source again.
These errors are not kept:

- `504` `query_timeout` or `502` `datasource_unavailable` when a source does not answer in time.
- `429` `too_many_requests` when the server's shared upstream slots were busy.
- `422` `query_invalid` when the datasource does not hold the signal.

Every signed-in role from Viewer can read it, because any Viewer can already query any datasource.
In the CLI, `observe datasources services <uid>` lists them, and `observe datasources services --service checkout` asks every source and prints the ones that hold a service whose name contains `checkout`.
The MCP tool `list_services` does the same for an agent.

#### Read the lines around one line

`POST /api/v1/datasources/{uid}/log-context` takes these fields.

| Field | Meaning |
| --- | --- |
| `query` | The query of the line. |
| `anchor` | The line's `TimeNs`. |
| `before`, `after` | Lines on each side, 0 to 500, 50 by default. |
| `window_ms` | How far each side looks, 1,000 to 86,400,000, 3,600,000 (one hour) by default. |

The answer has the `anchor` and a `before` and an `after` side, each with its `frames` and `truncated`.

#### Live tail over HTTP

`GET /api/v1/datasources/{uid}/tail` sends new lines as Server-Sent Events.
It takes `query`, `from` (the last 10 seconds by default), `limit` (1 to 1,000, 500 by default) and `timezone`, each given once.

- It sends `retry: 3000`, then `event: ready`, and then an `event: lines` frame for each poll with `frames`, `newest`, `truncated`, `partial` and `warning`.
- A `: ping` comment arrives every 15 seconds.
- It sends `event: error` with a `code` and an `error` when it cannot continue.
- It ends after 30 minutes.
- A person or key may hold 5 tails, and the server 200.
  Beyond that the answer is 429 or 503 `too_many_streams`.

#### Ask over HTTP

Scripts ask a question with `POST /api/v1/datasources/{uid}/draft-query`:

```sh
curl -fsS -X POST "$OBSERVE_SERVER/api/v1/datasources/<datasource uid>/draft-query" \
  -H "Authorization: Bearer $OBSERVE_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"question":"checkout errors","from":"now-1h","to":"now"}'
```

The answer has `query`, `language` (`logql`, `clickhouse_sql`, `traceql` or `filters`), `trace_filters`, `explanation` and `warnings`.
A ClickHouse datasource needs `"signal":"logs"` or `"signal":"traces"`.

| Status | Code | Meaning |
| --- | --- | --- |
| 422 | `cannot_translate` | The model cannot turn the question into a query, with a `reason`. |
| 503 | `agent_unavailable` | The server has no model. |
| 429 | `too_many_requests` | The caller has 3 questions in flight, or has used the 60 of the hour. |
| 429 | `agent_busy` | The server is answering 10 questions already. |
| 504 | `draft_timeout` | The question did not finish within 60 seconds. |
| 502 | `provider_error` | The model provider failed. |
| 400 | `invalid_request` | The request is malformed, or the question is over 2,000 characters. |
| 404 | `not_found` | The datasource does not exist. |
