---
title: Connect Loki
description: Add a Loki server as a datasource for logs panels and log metrics, with its tenant and authentication.
---

A Loki entry takes the fields common to every datasource, described in the [datasources file](../reference/datasources-file.md), plus the fields below.

A multi-tenant Loki behind a bearer token names the tenant and references the token by variable name:

```yaml
- uid: loki
  type: loki
  name: Loki
  loki:
    url: https://loki.example.com
    tenantId: shop
    bearerTokenEnv: SLASHOBSERVE_LOKI_TOKEN
```

## Fields

| Field                               | Notes                                                                                                                                      |
| ----------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------ |
| `url`                               | Required. `http` or `https`, with a host, and no user info, query or fragment.                                                             |
| `tenantId`                          | Optional. Sent as the `X-Scope-OrgID` header on every request; leave it out for a Loki that runs with `auth_enabled: false`.               |
| `headers`                           | Plain request headers. Not for secrets. `Accept`, `Host` and `Content-Length` are managed by the server, and `X-Scope-OrgID` is set through `tenantId`. |
| `headersFromEnv`, `headersFromFile` | Maps of header name to an environment variable name or a file path, for header values that are secrets.                                    |
| `basicAuth`                         | `username` plus one of `passwordEnv` or `passwordFile`. The password is required.                                                          |
| `bearerTokenEnv`, `bearerTokenFile` | A bearer token from an environment variable or a file. Not combinable with `basicAuth`.                                                    |

The server does not follow redirects, so give the final address, and it refuses a response over 64 MiB.

## Queries

A query that starts with a stream selector, such as `{service_name="checkout"} |= "error"`, is a log query.
It returns one frame per stream, with the stream's labels and the fields `Time`, `Body` and, when the stream has a `level`, `lvl`, `severity_text`, `severity` or `detected_level` label, `level`.
Lines come newest first, and one query returns at most the datasource's row limit or 5,000 lines, whichever is lower.
Each line counts as one point against the points limit.

Any other query, such as `sum by (service_name) (count_over_time({service_name=~".+"}[5m]))`, is a metric query.
It returns series with `Time` and `Value` fields, like a Prometheus query, and the series and points limits apply.

Queries take dashboard variables, escaped for the place they appear in, as in PromQL: inside a regex matcher several values become one alternation.
They also take the PromQL time macros `$__interval`, `$__interval_ms`, `$__range`, `$__range_s`, `$__range_ms`, `$__from` and `$__to`, plus `$__dashboard` and the `$__user` macros, so `count_over_time({service_name="checkout"}[$__interval])` follows the panel's step.
`$__rate_interval` is Prometheus only, because it depends on a scrape interval.

## Variables and discovery

A query variable on a Loki datasource takes `label_values(label)` or `label_values({selector}, label)` and returns that label's values over the dashboard's time range.

The agent can list Loki's label names and the values of one label, optionally narrowed by a stream selector.
Loki has no metric names, so the metric list is empty.
The query editor's lint checks that a query has a stream selector, that brackets and quotes balance, and that every stage after `|` is a known parser, formatter or label filter.
