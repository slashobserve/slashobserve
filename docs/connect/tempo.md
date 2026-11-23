---
title: Connect Tempo
description: Add a Tempo server as a datasource for traces panels, read by trace id or by a TraceQL search, with its authentication, a first query and the rules for queries and discovery.
---

Connect a Tempo server to open traces by id, search them with TraceQL, and draw them as span trees in dashboards and on the Traces page.
An Admin adds it to the datasources file, or in the app as [Add datasources in the app](datasources-in-the-app.md) describes.

## Connect a Tempo server

1. Add an entry to the [datasources file](../reference/datasources-file.md) that `SLASHOBSERVE_DATASOURCES_FILE` names:

   ```yaml
   datasources:
     - uid: tempo
       type: tempo
       name: Tempo
       tempo:
         url: http://tempo:3200
   ```

2. Restart the server.
   It reads the file once, at startup, and refuses to start when an entry is invalid.
3. Check the connection.
   The **Datasources** page shows the entry as "From file" with its health, and the CLI prints the same:

   ```sh
   observe datasources list
   observe datasources recheck tempo
   ```

   The health check reads `/ready`.
4. Run a first search:

   ```sh
   observe traces --query '{ resource.service.name =~ ".+" }' --datasource tempo
   ```

   You see a table of up to 100 traces, newest first, with each trace's start, service, span name and duration.
5. Open one trace with an id from that table:

   ```sh
   observe traces 4bf92f3577b34da6a3ce929d0e0e4736 --datasource tempo
   ```

   You see the trace as a span tree.
   The same queries work on the [Traces page](../use/logs-and-traces.md).

## Connect a Tempo behind a login

Reference a secret by variable name or by file, never by value.
The variable name must start with `SLASHOBSERVE_`, and it goes in the same env file as the server's other settings.

```yaml
- uid: tempo
  type: tempo
  name: Tempo
  tempo:
    url: https://tempo.example.com
    basicAuth:
      username: reader
      passwordEnv: SLASHOBSERVE_TEMPO_PASSWORD
```

Basic auth and a bearer token cannot be combined.
A multi-tenant Tempo takes its tenant as an `X-Scope-OrgID` header under `headers`.

## Find a trace by id or by TraceQL

A query that is a trace id reads that trace, and any other query is a TraceQL search.
[Queries](#queries) lists what each one returns, how long a search can run and which variables and macros it takes.

## Find tags and services

Tempo has no query variables; use a custom or text box variable for trace ids or service names.
[Variables and discovery](#variables-and-discovery) lists what the agent can read.

## Reference

### Fields

Every entry also takes the [fields common to every datasource](../reference/datasources-file.md).

| Field | Notes |
| --- | --- |
| `url` | Required. `http` or `https`, with a host, and no user info, query or fragment. |
| `headers` | Plain request headers. Not for secrets. |
| `headersFromEnv`, `headersFromFile` | Maps of header name to an environment variable name or a file path. |
| `basicAuth` | `username` plus one of `passwordEnv` or `passwordFile`. The password is required. |
| `bearerTokenEnv`, `bearerTokenFile` | A bearer token from an environment variable or a file. |

Details of `headers`:

- `Accept`, `Host` and `Content-Length` are managed by the server and refused.
- `Authorization` is refused; use `basicAuth` or a bearer token.

Details of `bearerTokenEnv` and `bearerTokenFile`:

- They cannot be combined with `basicAuth`.

The server does not follow redirects, so give the final address, and it refuses a response over 64 MiB.

### Queries

A query that is a trace id, 32 or 16 hexadecimal characters, reads that trace and returns one row per span.
The row has these fields, which the traces panel draws as a span tree:

- `TraceId`, `SpanId`, `ParentSpanId`, `SpanName` and `ServiceName`
- `StartTime` and `Duration` in nanoseconds
- `StatusCode`, which is `Ok`, `Error` or `Unset`
- `StatusMessage`, the text the span set with its status, such as "card declined", or empty
- `SpanAttributes` and `ResourceAttributes`, both JSON objects

A trace with more spans than the datasource's row limit is refused, and a trace Tempo does not know returns no data.

Any other query is a TraceQL search, such as `{ resource.service.name = "checkout" && status = error }`, over the dashboard's time range.
It returns one row per matching trace, newest first and at most the row limit.
Each row has `TraceId`, `SpanName` and `ServiceName` of the root span, `StartTime` and `Duration`.
A search result names no spans, so its `SpanId` is the trace id and each trace shows as one bar.
Tempo refuses a search longer than its own maximum search duration, 168 hours by default.

The server adds `with (most_recent=true)` to a search unless the query has its own `with` hints.
On a long range it searches the newest hour first, then wider windows further back.
It stops when the row limit is full or the query's time runs out, and in that case the traces found come back marked partial.
[Logs and traces](../use/logs-and-traces.md#results) describes what the page shows.

Queries take dashboard variables, escaped as in PromQL.
A variable that holds a trace id goes in quotes, such as `"${trace_id}"`, because a bare trace id that starts with a digit is refused.
Queries also take these macros:

- `$__interval`, `$__interval_ms`, `$__range`, `$__range_s`, `$__range_ms`, `$__from` and `$__to`
- `$__dashboard` and the `$__user` macros

### Variables and discovery

Tempo has no query variables.

The agent can do these on Tempo:

- List tags, scoped as TraceQL writes them (`resource.service.name`, `span.http.method`, or an intrinsic such as `name` or `status`).
- List the values of one tag, optionally narrowed by a TraceQL query.
- Run `get_schema`, which lists every service with its span names and the attributes TraceQL can filter on.

The services list reads the values of `resource.service.name` over a recent window.
The query editor's lint accepts a trace id and checks that a TraceQL query has a spanset and that its brackets and quotes balance.
