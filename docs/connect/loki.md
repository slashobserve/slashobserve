---
title: Connect Loki
description: Add a Loki server as a datasource for logs panels and log metrics, with its tenant and authentication, a first LogQL query, and the rules for queries and discovery.
---

Connect a Loki server to search logs on the Logs page, draw logs and log metrics in dashboards, and let the agent read them.
An Admin adds it to the datasources file, or in the app as [Add datasources in the app](datasources-in-the-app.md) describes.

## Connect a Loki server

1. Add an entry to the [datasources file](../reference/datasources-file.md) that `SLASHOBSERVE_DATASOURCES_FILE` names:

   ```yaml
   datasources:
     - uid: loki
       type: loki
       name: Loki
       loki:
         url: http://loki:3100
   ```

2. Restart the server.
   It reads the file once, at startup, and refuses to start when an entry is invalid.
3. Check the connection.
   The **Datasources** page shows the entry as "From file" with its health, and the CLI prints the same:

   ```sh
   observe datasources list
   observe datasources recheck loki
   ```

   The health check reads `/ready`.
4. Run a first query:

   ```sh
   observe logs '{service_name=~".+"}' --datasource loki --from now-15m
   ```

   You see the newest lines of the last 15 minutes, oldest first, with their time, labels and message.
   The same query works on the [Logs page](../use/logs-and-traces.md).

## Connect a multi-tenant Loki behind a token

Name the tenant and reference the token by variable name.
The variable name must start with `SLASHOBSERVE_`, and it must not be one the server uses for its own configuration.
It goes in the same env file as the server's other settings.

```yaml
- uid: loki
  type: loki
  name: Loki
  loki:
    url: https://loki.example.com
    tenantId: shop
    bearerTokenEnv: SLASHOBSERVE_LOKI_TOKEN
```

`tenantId` is sent as the `X-Scope-OrgID` header on every request.
Leave it out for a Loki that runs with `auth_enabled: false`.
For basic auth, give `basicAuth` with a `username` and one of `passwordEnv` or `passwordFile`.
Basic auth and a bearer token cannot be combined.

## Write a log or metric query

A query that starts with a stream selector, such as `{service_name="checkout"} |= "error"`, is a log query.
Any other query, such as `sum by (service_name) (count_over_time({service_name=~".+"}[5m]))`, is a metric query.
[Queries](#queries) lists what each one returns and which variables and macros it takes.

## Find labels, values and services

A query variable on a Loki datasource takes `label_values(label)` or `label_values({selector}, label)`.
From the CLI, `observe datasources labels`, `label-values` and `services` name what Loki holds.
[Variables and discovery](#variables-and-discovery) lists what the agent can read.

## Reference

### Fields

Every entry also takes the [fields common to every datasource](../reference/datasources-file.md).

| Field | Notes |
| --- | --- |
| `url` | Required. `http` or `https`, with a host, and no user info, query or fragment. |
| `tenantId` | Optional. Sent as the `X-Scope-OrgID` header. |
| `headers` | Plain request headers. Not for secrets. |
| `headersFromEnv`, `headersFromFile` | Maps of header name to an environment variable name or a file path. |
| `basicAuth` | `username` plus one of `passwordEnv` or `passwordFile`. The password is required. |
| `bearerTokenEnv`, `bearerTokenFile` | A bearer token from an environment variable or a file. |

Details of `headers`:

- `Accept`, `Host` and `Content-Length` are managed by the server and refused.
- `X-Scope-OrgID` is set through `tenantId` and refused here.
- `Authorization` is refused; use `basicAuth` or a bearer token.

Details of `bearerTokenEnv` and `bearerTokenFile`:

- They cannot be combined with `basicAuth`.

The server does not follow redirects, so give the final address, and it refuses a response over 64 MiB.

### Queries

A log query returns one frame per stream, with the stream's labels and the fields `Time`, `Body` and `level`.
The `level` field is present when the stream has a `level`, `lvl`, `severity_text`, `severity` or `detected_level` label.
Lines come newest first, and one query returns at most the datasource's row limit or 5,000 lines, whichever is lower.
Each line counts as one point against the points limit.
Log patterns use Loki's pattern ingester (`pattern_ingester: enabled: true` in Loki's config) when it is enabled and the query is a bare stream selector.
Otherwise the server samples the newest lines and computes the patterns itself.

A metric query returns series with `Time` and `Value` fields, like a Prometheus query, and the series and points limits apply.

Queries take dashboard variables, escaped for the place they appear in, as in PromQL.
Inside a regex matcher several values become one alternation.
The value All becomes `.+` inside the stream selector, because Loki refuses a selector whose every matcher can match an empty value.
After the selector, in a label or line filter, All becomes `.*`, so lines without the label stay.
A variable's own `allValue` replaces that text, so set it to `.+` there, never `.*`.

Queries also take these macros, so `count_over_time({service_name="checkout"}[$__interval])` follows the panel's step:

- `$__interval`, `$__interval_ms`, `$__range`, `$__range_s`, `$__range_ms`, `$__from` and `$__to`
- `$__dashboard` and the `$__user` macros

`$__rate_interval` is Prometheus only, because it depends on a scrape interval.

### Variables and discovery

A query variable on a Loki datasource returns that label's values over the dashboard's time range.

The agent can do these on Loki:

- List label names and the values of one label, both optionally narrowed by a stream selector.
- Sample up to 20 recent lines with their labels.
- List the stream label keys of a scope with the number of streams that carry each.

Loki has no metric names, so the metric list is empty.
The services list takes the first of the labels `service_name`, `service`, `app` and `job` that has values over a recent window.
The query editor's lint checks three things:

- The query has a stream selector.
- Brackets and quotes balance.
- Every stage after `|` is a known parser, formatter or label filter.
