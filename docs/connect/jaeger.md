---
title: Connect Jaeger
description: Add a Jaeger server as a read-only datasource for traces panels, read by trace id or by a search of service, operation, tags and duration.
---

SlashObserve reads Jaeger 2.21 or later through its `/api/v3` query API, which the Jaeger v2 query service serves on port 16686 by default.
An older Jaeger answers with a message that names the version needed.
It only reads: it never writes spans or settings to Jaeger.

A Jaeger entry takes the fields common to every datasource, described in the [datasources file](../reference/datasources-file.md), plus the fields below.

A Jaeger behind basic auth references the password by variable name:

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

## Fields

| Field                               | Notes                                                                                                                                  |
| ----------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| `url`                               | Required. `http` or `https`, with a host, and no user info, query or fragment. A path prefix, such as `/jaeger`, is kept.              |
| `headers`                           | Plain request headers. Not for secrets. `Accept`, `Host` and `Content-Length` are managed by the server.                               |
| `headersFromEnv`, `headersFromFile` | Maps of header name to an environment variable name or a file path, for header values that are secrets.                                |
| `basicAuth`                         | `username` plus one of `passwordEnv` or `passwordFile`. The password is required.                                                      |
| `bearerTokenEnv`, `bearerTokenFile` | A bearer token from an environment variable or a file. Not combinable with `basicAuth`.                                                |

The server does not follow redirects, so give the final address, and it refuses a response over 64 MiB.
The health check reads the list of services.

## Queries

A query that is a trace id, 32 or 16 hexadecimal characters, reads that trace and returns one row per span.
The row has the fields `TraceId`, `SpanId`, `ParentSpanId`, `SpanName`, `ServiceName`, `StartTime`, `Duration` (nanoseconds), `StatusCode` (`Ok`, `Error` or `Unset`), `StatusMessage`, `SpanAttributes` and `ResourceAttributes` (both JSON objects), `SpanKind`, `SpanEvents` and `SpanLinks`, which the traces panel draws as a span tree.
A trace with more spans than the datasource's row limit is cut at the limit and marked truncated, and a trace Jaeger does not know returns no data.

Any other query is a search: `key=value` pairs separated by spaces, over the dashboard's time range, such as `service=checkout operation="POST /pay" minDuration=500ms error=true`.
A query that is empty lists the newest traces, but the query API and a panel need at least one byte, so write `minDuration=0s` to search with no filter.

| Key                           | Meaning                                                                                                        |
| ----------------------------- | -------------------------------------------------------------------------------------------------------------- |
| `service`                     | The service name of a span.                                                                                    |
| `operation`                   | The span name.                                                                                                 |
| `minDuration`, `maxDuration`  | Bounds on a span's duration, as a Go duration such as `250ms` or `2s`.                                         |
| any other key                 | A span or resource tag compared for equality, such as `http.status_code=500` or `error=true`.                  |

Quote a value that holds a space, as in `operation="GET /cart"`, and escape a quote inside it with a backslash.
A key may appear once.
Jaeger's search has no regex, negation or comparison other than the two durations, so the search says what to filter on and the rest is read from the traces.

A search returns one row per trace, newest first and at most the row limit, with `TraceId`, `SpanName` and `ServiceName` of the root span, `StartTime`, `Duration` and the counts `Spans` and `Errors`.
A search result names no spans, so its `SpanId` is the trace id and each trace shows as one bar.
Jaeger's search is bound by the start time of a trace, and the server asks for at most 10,000 traces, so a page beyond that window is refused and the message says to narrow the range or the search.

Queries take dashboard variables and the time macros `$__interval`, `$__range`, `$__from` and `$__to`, plus `$__dashboard` and the `$__user` macros.
A variable that holds a trace id works bare or in quotes.

## Variables and discovery

Jaeger has no query variables; use a custom or text box variable for trace ids or service names.

The agent lists the values of `service`, and of `operation` for one service when the metric is `service=checkout`; tags are free-form and cannot be listed.
`get_schema` lists every service with its span names, for at most 30 services and 50 span names each, and marks the list as cut when it is longer.
The services list reads Jaeger's list of services.
The query editor's lint checks that each pair has a key and a value, that a duration parses and that no key repeats.

## Service dependencies

The datasource reads Jaeger's service dependency graph over a time range, as pairs of calling and called service with a call count.
The graph comes from Jaeger's `/api/dependencies` endpoint, which the v3 API does not replace.
No page shows it yet.

## Not covered

TraceQL, span metrics and any write to Jaeger are not available on a Jaeger datasource.
A datasource variable of type `datasource` cannot offer Jaeger datasources yet, so a panel names its Jaeger datasource by uid.
`trace_stats` and `compare_traces` read Tempo or ClickHouse only.
