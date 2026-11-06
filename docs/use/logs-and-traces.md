---
title: Logs and traces
description: Search logs from Loki or ClickHouse, follow a line to its trace, tail new lines and keep a query as a dashboard panel.
---

The logs page and the trace view let you look at raw lines and spans without building a dashboard first.
They read from the datasources an Admin configured, as described in [Connect Loki](../connect/loki.md), [Connect Tempo](../connect/tempo.md) and [Connect ClickHouse](../connect/clickhouse.md).

## Logs page

Open **Logs** in the sidebar and pick a Loki or ClickHouse datasource.
On Loki, write a LogQL query such as `{service_name="checkout"} |= "error"`.
On ClickHouse, the editor starts with a template that reads `otel_logs`; keep `$__timeFilter(Timestamp)` in the `WHERE` clause and a `LIMIT` at the end.
**Label browser** lists the labels of the datasource and their values, and a click inserts a matcher at the cursor.
Press **Run query**, or Ctrl+Enter or Cmd+Enter in the editor, to run it over the time range at the top of the page.
The page shows at most 1,000 lines, newest first.
The volume strip above the lines counts every line the query matches over the whole time range by level, not only the lines shown.
On Loki the query must start with a stream selector, and a line's level is the first of its `level`, `lvl`, `severity_text`, `severity` and `detected_level` labels, including a `level` that a `| json` or `| logfmt` stage extracts.
On ClickHouse the level is the `SeverityText` column, counted per `Timestamp`, so the strip hides when the query does not select both.
Levels such as `err`, `fatal` and `critical` count as errors, `warn` as warnings, `info` and `notice` as info, and anything else, or no level, as other.
The address keeps the search as `/logs?ds=<datasource uid>&q=<query>&from=<from>&to=<to>`, so you can share it or bookmark it.

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
**Open trace** on a log line opens it when the line carries a `TraceId`, `traceID`, `trace_id` or `traceId` field.
A Loki line opens in the first Tempo datasource, and a ClickHouse line opens in the same ClickHouse datasource.
The view draws the spans as a waterfall, and a click on a span shows its attributes in the pane beside it.
On ClickHouse the view looks for the spans in the last 24 hours, or within an hour of the line when you came from the logs page; widen the time range for an older trace.
Tempo finds a trace by its id at any time.
**Copy link** copies the address of the trace with its datasource and time range.

## Trace search

Open **Traces** in the sidebar to find traces by service, span name, minimum and maximum duration, and errors only.
Durations take a number and a unit, such as `250ms` or `1.5s`.
On Tempo you can also write TraceQL, which replaces the fields.
ClickHouse lists the newest 100 traces with a matching span and shows that trace's longest matching span.
Tempo shows the root span of each trace.
Tempo results also carry each trace's span count and error span count, summed from Tempo's per-service stats; when Tempo does not report those stats, both counts are left out.

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

Every signed-in role from Viewer can use the logs page, the trace view and the trace search.
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
