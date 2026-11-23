---
title: Connect Jaeger
description: Add a Jaeger server as a read-only datasource for traces panels, read by trace id or by a search of service, operation, tags and duration, with a first query and the rules for searches and discovery.
---

Connect a Jaeger server to open traces by id, search them by service, operation, tags and duration, and draw them as span trees in dashboards and on the Traces page.
SlashObserve only reads from Jaeger: it never writes spans or settings to it.
An Admin adds it to the datasources file, or in the app as [Add datasources in the app](datasources-in-the-app.md) describes.

SlashObserve reads Jaeger 2.21 or later through its `/api/v3` query API, which the Jaeger v2 query service serves on port 16686 by default.
An older Jaeger answers with a message that names the version needed.

## Connect a Jaeger server

1. Add an entry to the [datasources file](../reference/datasources-file.md) that `SLASHOBSERVE_DATASOURCES_FILE` names:

   ```yaml
   datasources:
     - uid: jaeger
       type: jaeger
       name: Jaeger
       jaeger:
         url: http://jaeger:16686
   ```

2. Restart the server.
   It reads the file once, at startup, and refuses to start when an entry is invalid.
3. Check the connection.
   The **Datasources** page shows the entry as "From file" with its health, and the CLI prints the same:

   ```sh
   observe datasources list
   observe datasources recheck jaeger
   ```

   The health check reads the list of services.
4. Run a first search:

   ```sh
   observe traces --query 'minDuration=0s' --datasource jaeger
   ```

   You see a table of up to 100 traces, newest first, with each trace's start, service, span, duration, span count and error count.
   The `minDuration=0s` search filters nothing, because a query must have at least one byte.
5. Narrow it to one service:

   ```sh
   observe traces --query 'service=checkout error=true' --datasource jaeger --sort -duration
   ```

   The same searches work on the [Traces page](../use/logs-and-traces.md).

## Connect a Jaeger behind a login

Reference a secret by variable name or by file, never by value.
The variable name must start with `SLASHOBSERVE_`, and it goes in the same env file as the server's other settings.

```yaml
- uid: jaeger
  type: jaeger
  name: Jaeger
  jaeger:
    url: https://jaeger.example.com
    basicAuth:
      username: reader
      passwordEnv: SLASHOBSERVE_JAEGER_PASSWORD
```

Basic auth and a bearer token cannot be combined.
A path prefix in `url`, such as `/jaeger`, is kept.

## Search traces

A query that is a trace id reads that trace, and any other query is a search of `key=value` pairs.
[Queries](#queries) lists the keys, what a search returns and its limits.

## Find services and operations

Jaeger has no query variables; use a custom or text box variable for trace ids or service names.
[Variables and discovery](#variables-and-discovery) lists what the agent can read.

## Read service dependencies

The datasource reads Jaeger's service dependency graph over a time range, as pairs of calling and called service with a call count.
The graph comes from Jaeger's `/api/dependencies` endpoint, which the v3 API does not replace.
No page shows it yet.

## Reference

### Fields

Every entry also takes the [fields common to every datasource](../reference/datasources-file.md).

| Field | Notes |
| --- | --- |
| `url` | Required. `http` or `https`, with a host, and no user info, query or fragment. A path prefix is kept. |
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
- `StatusCode`, which is `Ok`, `Error` or `Unset`, and `StatusMessage`
- `SpanAttributes` and `ResourceAttributes`, both JSON objects
- `SpanKind`, `SpanEvents` and `SpanLinks`

A trace with more spans than the datasource's row limit is cut at the limit and marked truncated.
A trace Jaeger does not know returns no data.

Any other query is a search: `key=value` pairs separated by spaces, over the dashboard's time range, such as `service=checkout operation="POST /pay" minDuration=500ms error=true`.
An empty query lists the newest traces, but the query API and a panel need at least one byte, so write `minDuration=0s` to search with no filter.

| Key | Meaning |
| --- | --- |
| `service` | The service name of a span. |
| `operation` | The span name. |
| `minDuration`, `maxDuration` | Bounds on a span's duration, as a Go duration such as `250ms` or `2s`. |
| any other key | A span or resource tag compared for equality, such as `http.status_code=500` or `error=true`. |

Quote a value that holds a space, as in `operation="GET /cart"`, and escape a quote inside it with a backslash.
A key may appear once.
Jaeger needs a service to search on an operation or a tag.
So `operation` or any other key without `service` is refused with "tags need service=<name>" before the search is sent.
Jaeger's search has no regex, negation or comparison other than the two durations, so the search says what to filter on and the rest is read from the traces.

A search returns one row per trace, newest first and at most the row limit.
Each row has `TraceId`, `SpanName` and `ServiceName` of the root span, `StartTime`, `Duration` and the counts `Spans` and `Errors`.
A search result names no spans, so its `SpanId` is the trace id and each trace shows as one bar.

Details of search limits and paging:

- Jaeger's search is bound by the start time of a trace, and the server asks for at most 10,000 traces.
  A page beyond that window is refused, and the message says to narrow the range or the search.
- Jaeger has no offset, so a page asks again for the first traces up to the end of that page and keeps the last ones.
- That is exact only where the storage returns the newest traces when it cuts a search at its depth.
  Elasticsearch, OpenSearch, Badger and the in-memory store do.
- Cassandra and ClickHouse storage cut the search in no stated order.
  There the newest-first order, the row limit and the pages are approximate, and a page can repeat or skip a trace.

Queries take dashboard variables and these macros:

- `$__interval`, `$__range`, `$__from` and `$__to`
- `$__dashboard` and the `$__user` macros

A variable that holds a trace id works bare or in quotes.

### Variables and discovery

Jaeger has no query variables.

The agent can do these on Jaeger:

- List the values of `service`, and of `operation` for one service when the metric is `service=checkout`.
- Run `get_schema`, which lists every service with its span names, for at most 30 services and 50 span names each, and marks the list as cut when it is longer.

Tags are free-form and cannot be listed.
The services list reads Jaeger's list of services, which holds every service Jaeger retains, so the window of `list_services` and `observe datasources services` does not narrow it.
The query editor's lint checks these:

- Each pair has a key and a value.
- A duration parses.
- No key repeats.
- An `operation` or tag comes with a `service`.

### Not covered

- TraceQL, span metrics and any write to Jaeger are not available on a Jaeger datasource.
- A datasource variable of type `datasource` cannot offer Jaeger datasources yet, so a panel names its Jaeger datasource by uid.
- `trace_stats` and `compare_traces` read Tempo or ClickHouse only.
