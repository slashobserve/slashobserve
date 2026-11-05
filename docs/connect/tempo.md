---
title: Connect Tempo
description: Add a Tempo server as a datasource for traces panels, read by trace id or by a TraceQL search.
---

A Tempo entry takes the fields common to every datasource, described in the [datasources file](../reference/datasources-file.md), plus the fields below.

A Tempo behind basic auth references the password by variable name:

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

## Fields

| Field                               | Notes                                                                                                                                         |
| ----------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------- |
| `url`                               | Required. `http` or `https`, with a host, and no user info, query or fragment.                                                                |
| `headers`                           | Plain request headers. Not for secrets. `Accept`, `Host` and `Content-Length` are managed by the server. A multi-tenant Tempo takes its tenant as an `X-Scope-OrgID` header here. |
| `headersFromEnv`, `headersFromFile` | Maps of header name to an environment variable name or a file path, for header values that are secrets.                                       |
| `basicAuth`                         | `username` plus one of `passwordEnv` or `passwordFile`. The password is required.                                                             |
| `bearerTokenEnv`, `bearerTokenFile` | A bearer token from an environment variable or a file. Not combinable with `basicAuth`.                                                       |

The server does not follow redirects, so give the final address, and it refuses a response over 64 MiB.

## Queries

A query that is a trace id, 32 or 16 hexadecimal characters, reads that trace and returns one row per span.
The row has the fields `TraceId`, `SpanId`, `ParentSpanId`, `SpanName`, `ServiceName`, `StartTime`, `Duration` (nanoseconds), `StatusCode` (`Ok`, `Error` or `Unset`), `SpanAttributes` and `ResourceAttributes` (both JSON objects), which the traces panel draws as a span tree.
A trace with more spans than the datasource's row limit is refused, and a trace Tempo does not know returns no data.

Any other query is a TraceQL search, such as `{ resource.service.name = "checkout" && status = error }`, over the dashboard's time range.
It returns one row per matching trace, newest first and at most the row limit, with `TraceId`, `SpanName` and `ServiceName` of the root span, `StartTime` and `Duration`.
A search result names no spans, so its `SpanId` is the trace id and each trace shows as one bar.
Tempo refuses a search longer than its own maximum search duration, 168 hours by default.

Queries take dashboard variables, escaped as in PromQL.
A variable that holds a trace id goes in quotes, such as `"${trace_id}"`, because a bare trace id that starts with a digit is refused.
They also take the PromQL time macros `$__interval`, `$__interval_ms`, `$__range`, `$__range_s`, `$__range_ms`, `$__from` and `$__to`, plus `$__dashboard` and the `$__user` macros.

## Variables and discovery

Tempo has no query variables; use a custom or text box variable for trace ids or service names.

The agent can list Tempo's tags, scoped as TraceQL writes them (`resource.service.name`, `span.http.method`, or an intrinsic such as `name` or `status`), and the values of one tag, optionally narrowed by a TraceQL query.
`get_schema` on Tempo lists every service with its span names and the attributes TraceQL can filter on.
The query editor's lint accepts a trace id and checks that a TraceQL query has a spanset and that its brackets and quotes balance.
