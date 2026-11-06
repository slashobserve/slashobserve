---
title: Logs and traces
description: Search logs and traces from Loki, Tempo or ClickHouse with a query or a plain-language question, follow a line to its trace, tail new lines and keep a query as a dashboard panel.
---

The logs page and the trace view let you look at raw lines and spans without building a dashboard first.
They read from the datasources an Admin configured, as described in [Connect Loki](../connect/loki.md), [Connect Tempo](../connect/tempo.md) and [Connect ClickHouse](../connect/clickhouse.md).

## Logs page

Open **Logs** in the sidebar and pick a Loki or ClickHouse datasource.
The picker names each one with its type, such as "checkout-logs (ClickHouse)", or only "ClickHouse" when the name is the type.
The page starts as soon as it opens.
On ClickHouse it runs a template that reads `otel_logs` and shows the newest 1,000 lines of the time range at once.
On Loki it opens **Label browser** and waits for a stream selector.
**Label browser** lists the labels of the datasource and their values, and a click inserts a matcher at the cursor.
With a model configured on the server, the query box opens in Ask mode, as [Ask on the logs page](#ask-on-the-logs-page) describes.

In Query mode you write the query yourself.
On Loki, write a LogQL query such as `{service_name="checkout"} |= "error"`.
On ClickHouse, the editor holds the template; keep `$__timeFilter(Timestamp)` in the `WHERE` clause and a `LIMIT` at the end.
Press **Run query**, or Ctrl+Enter or Cmd+Enter in the editor, to run it over the time range at the top of the page.
The page shows at most 1,000 lines, newest first.
On Loki the query must start with a stream selector.
When more lines match than the page shows, the page says so and **Load older** fetches the lines before the oldest one shown.
**Live tail** and **Add to dashboard** stay disabled until a query has run, and say "Run a query first."

The volume strip above the lines counts every line the query matches over the whole time range by level, not only the lines shown.
It comes from its own log volume request, so the lines never wait for it, and it hides when that request fails or while live tail shows its lines.
On Loki a line's level is the first of its `level`, `lvl`, `severity_text`, `severity` and `detected_level` labels, including a `level` that a `| json` or `| logfmt` stage extracts.
On ClickHouse the level is the `SeverityText` column, counted per `Timestamp`, so the strip hides when the query does not select both.
Levels such as `err`, `fatal` and `critical` count as errors, `warn` as warnings, `info` and `notice` as info, and anything else, or no level, as other.

Facets count the top values of each label, field or column the query matches over the whole time range, through `POST /api/v1/datasources/{uid}/log-facets` and the `log_facets` tool.
Each field says whether its counts are exact or come from a sample of the newest lines.
On Loki the index labels of the matched streams are counted exactly from the index volume API, in bytes rather than lines, so those fields carry `unit: bytes`; parsed and structured-metadata fields are counted from the newest 1,000 lines by default, up to 5,000.
On ClickHouse every field is counted exactly with one `GROUP BY` per field: each result column except `Timestamp`, `Body`, `TraceId` and `SpanId`, plus the 20 most frequent `LogAttributes` keys as `LogAttributes.<key>`.

Open a line to see its details in the panel on the right, or inline under the line on a screen 760 pixels wide or narrower.
A field whose value is a JSON object, such as the ClickHouse `LogAttributes` map, shows one row per key, named `<field>.<key>`, one level deep.
The details offer **Show context**, **Open trace** and **Copy line**, which copies the line's text.

The address keeps the search as `/logs?ds=<datasource uid>&q=<query>&from=<from>&to=<to>`, so you can share it or bookmark it.
A query written from a question also keeps the question as `ask` beside `q`, and the link opens the query with its question above it.

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
The lines come from the same stream: the same Loki index labels, or the same ClickHouse `ServiceName`, within an hour of the line.
On Loki, the lines after are found by widening a window step by step, so they may be incomplete when more than 1,000 lines arrived in the 10 seconds after the line.

## Live tail

**Live tail** asks for new lines every 2 seconds, starting from the newest line shown.
It keeps the newest 1,000 lines on the page.
It stops when you press **Stop live tail**, or when the query, the datasource or the time range changes.

## Add to dashboard

**Add to dashboard** keeps the query as a logs panel on a dashboard you can edit.
You need Edit on the target dashboard.
The panel is full width and goes at the bottom of the dashboard, inside the last row when that row is collapsed, and the save makes a new version.
The save runs the query over the dashboard's time range and refuses a panel that returns no lines there.

## Trace view

A trace opens at `/t/<trace id>?ds=<datasource uid>`.
The title reads **Trace** with the first 8 characters of the id, and **Copy trace id** copies the whole id.
**Open trace** on a log line opens it when the line carries a `TraceId`, `traceID`, `trace_id` or `traceId` field.
A Loki line opens in the first Tempo datasource, and a ClickHouse line opens in the same ClickHouse datasource.
A line above the waterfall reads the root service, the root span, the duration and the span count, with the number of errors when any span failed.
The view draws the spans as a waterfall under a ruler that steps by 1, 2, 2.5 or 5 times a power of ten, so its labels are round numbers.
A click on a span shows its span attributes and then its resource attributes in the pane beside it, or inline under the span on a screen 760 pixels wide or narrower.
On ClickHouse the view looks for the spans in the last 24 hours, or within an hour of the line when you came from the logs page; widen the time range for an older trace.
Tempo finds a trace by its id at any time.
The view loads the whole trace up to 20,000 spans.
A trace over 20,000 spans opens with its earliest 20,000 spans and a banner that says how many the trace holds.
**Copy link** copies the address of the trace with its datasource and time range, and **Search traces** opens the trace search on the same datasource.

## Trace search

Open **Traces** in the sidebar to find traces by service, span name, minimum and maximum duration, and errors only.
The search runs as soon as the page opens, with the fields the address carries, or with none.
Durations take a number and a unit, such as `250ms` or `1.5s`.
On Tempo you can also write TraceQL, which replaces the fields.
Press **Search traces**, or Enter in Service, Span name or a duration field, to run it; Enter in the TraceQL field adds a line.
The page shows at most 100 traces.
Each row shows **Started**, which opens the trace, then **Service**, **Span**, **Duration** with a bar scaled to the slowest trace shown, **Spans** and **Status**.
**Status** reads OK, or Failed with the number of error spans.
**Spans** and **Status** show only when every row carries the count.
ClickHouse lists the newest 100 traces with a matching span, shows each trace's longest matching span, and counts only the matching spans.
Tempo shows the root span of each trace, and sums the span and error span counts from Tempo's per-service stats; when Tempo does not report those stats, both columns are left out.
The result is marked partial when Tempo did not search every block of the range, so a trace in an unsearched block is missing; narrow the time range or search again.
On a phone the table becomes a list.

## Ask on the trace search

With a model configured on the server, the trace search has the same **Ask** and **Query** toggle as the logs page, in the top right corner of the search box.
Ctrl+I or Cmd+I switches it, Enter asks, Shift+Enter adds a line and Esc cancels a question that is still running.
The mode you last chose on either page is the one both pages open in, and the trace search opens in Query when the address carries a search field.
**Recent** and Up bring back earlier questions, kept apart from those of the logs page.
The answer fills Service, Span name, the durations and Errors only, or on Tempo the TraceQL field when the question needs TraceQL.
A duration the form cannot read is left empty.
The search runs at once, and the address keeps the question as `ask`.
The explanation and any warnings show under the question, above the filled form.
Editing a field switches to Query, with a "From: <question>" line and **Ask again**, which brings the question back.
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
