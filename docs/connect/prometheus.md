---
title: Connect Prometheus
description: Add a Prometheus server as a datasource and set its authentication.
---

A Prometheus entry takes the fields common to every datasource, described in the [datasources file](../reference/datasources-file.md), plus the fields below.

A Prometheus behind a bearer token references the token by variable name, and the variable goes in the same env file as the other settings:

```yaml
prometheus:
  url: https://prometheus.example.com
  bearerTokenEnv: SLASHOBSERVE_PROMETHEUS_TOKEN
```

## Fields

| Field                               | Notes                                                                                                   |
| ----------------------------------- | ------------------------------------------------------------------------------------------------------- |
| `url`                               | Required. `http` or `https`, with a host, and no user info, query or fragment.                          |
| `scrapeInterval`                    | A Go duration, default `15s`. Feeds the `$__rate_interval` macro, and is the smallest step of a range query that sets no `interval`. |
| `headers`                           | Plain request headers. Not for secrets. `Content-Type`, `Accept`, `Host` and `Content-Length` are managed by the server and refused. |
| `headersFromEnv`, `headersFromFile` | Maps of header name to an environment variable name or a file path, for header values that are secrets. |
| `basicAuth`                         | `username` plus one of `passwordEnv` or `passwordFile`. The password is required.                       |
| `bearerTokenEnv`, `bearerTokenFile` | A bearer token from an environment variable or a file. Not combinable with `basicAuth`.                 |

The server does not follow redirects, so give the final address, and it refuses a response over 64 MiB.

## Step

A range query asks for about as many points per series as its panel is wide in pixels, and never at a step shorter than `scrapeInterval`, because a shorter step only repeats the same samples.
Over the last hour at the default `15s` that is at most 240 points per series, however wide the panel.
A query's or panel's `interval` replaces the floor, and `$__interval` and `$__rate_interval` follow the step the query ran with, which Inspect shows on its Query tab.
The explore query API, `observe metrics query` and the `query_range` MCP tool ask for 200 points per series unless told otherwise.

## Warnings

Thanos and Mimir answer with a `warnings` list when a store did not answer.
SlashObserve marks that result partial, so `meta.partial` is true even when the result is empty, and `meta.warning` carries the text, with a server address replaced by a generic line.
The agent's query tools report the same text, so an empty answer is not read as "no data".

## Exemplars

A time series or heatmap query with `exemplar: true` also asks Prometheus for exemplars over the same range, through `/api/v1/query_exemplars`.
Prometheus returns them only when it runs with `--enable-feature=exemplar-storage` and the scraped application exposes them.
SlashObserve keeps at most one exemplar per step, the one with the highest value, and at most 2,000 per query.
If the exemplar request fails, the panel still shows its series.
