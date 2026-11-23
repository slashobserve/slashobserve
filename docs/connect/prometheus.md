---
title: Connect Prometheus
description: Add a Prometheus server as a datasource, set its authentication, check its health and run a first PromQL query, with the step, warnings and exemplars rules.
---

Connect a Prometheus server to run PromQL queries from dashboards, the Metrics page, the CLI and the agent.
An Admin adds it to the datasources file, or in the app as [Add datasources in the app](datasources-in-the-app.md) describes.

## Connect a Prometheus server

1. Add an entry to the [datasources file](../reference/datasources-file.md) that `SLASHOBSERVE_DATASOURCES_FILE` names:

   ```yaml
   datasources:
     - uid: prom
       type: prometheus
       name: Prometheus
       prometheus:
         url: http://prometheus:9090
         scrapeInterval: 15s
   ```

2. Restart the server.
   It reads the file once, at startup, and refuses to start when an entry is invalid.
3. Check the connection.
   The **Datasources** page shows the entry as "From file" with its health, and the CLI prints the same:

   ```sh
   observe datasources list
   observe datasources recheck prom
   ```

   The health check sends the query `1` to `/api/v1/query`.
   A 401 or 403 answer shows as "authentication failed (HTTP 401)".
4. Run a first query:

   ```sh
   observe metrics query 'up' --datasource prom --instant
   ```

   You see one line per series with its labels and its value.
   The same query works on the [Metrics](../use/metrics.md) page, and `query_range` is the equivalent MCP tool.

## Connect a Prometheus that needs a login

Reference a secret by variable name or by file, never by value.
The variable name must start with `SLASHOBSERVE_`, and it goes in the same env file as the server's other settings.

```yaml
prometheus:
  url: https://prometheus.example.com
  bearerTokenEnv: SLASHOBSERVE_PROMETHEUS_TOKEN
```

For basic auth, give `basicAuth` with a `username` and one of `passwordEnv` or `passwordFile`.
Basic auth and a bearer token cannot be combined.
Other headers go under `headers`, or under `headersFromEnv` or `headersFromFile` when the value is a secret.
A header name may appear in only one of the three, and `Authorization` is refused there.

## Reference

### Fields

Every entry also takes the [fields common to every datasource](../reference/datasources-file.md).

| Field | Notes |
| --- | --- |
| `url` | Required. `http` or `https`, with a host, and no user info, query or fragment. |
| `scrapeInterval` | A Go duration, default `15s`. |
| `headers` | Plain request headers. Not for secrets. |
| `headersFromEnv`, `headersFromFile` | Maps of header name to an environment variable name or a file path. |
| `basicAuth` | `username` plus one of `passwordEnv` or `passwordFile`. The password is required. |
| `bearerTokenEnv`, `bearerTokenFile` | A bearer token from an environment variable or a file. |

Details of `scrapeInterval`:

- It feeds the `$__rate_interval` macro.
- It is the smallest step of a range query that sets no `interval`.

Details of `headers`:

- `Content-Type`, `Accept`, `Host` and `Content-Length` are managed by the server and refused.

Details of `bearerTokenEnv` and `bearerTokenFile`:

- They cannot be combined with `basicAuth`.

The server does not follow redirects, so give the final address, and it refuses a response over 64 MiB.

### Step

A range query asks for about as many points per series as its panel is wide in pixels.
It never uses a step shorter than `scrapeInterval`, because a shorter step only repeats the same samples.
Over the last hour at the default `15s`, that is at most 240 points per series, however wide the panel.
A query's or panel's `interval` replaces the floor.
`$__interval` and `$__rate_interval` follow the step the query ran with, which Inspect shows on its Query tab.
The explore query API, `observe metrics query` and the `query_range` MCP tool ask for 200 points per series unless told otherwise.

### Warnings

Thanos and Mimir answer with a `warnings` list when a store did not answer.
SlashObserve marks that result partial, so `meta.partial` is true even when the result is empty.
`meta.warning` carries the text, with a server address replaced by a generic line.
The agent's query tools report the same text, so an empty answer is not read as "no data".

### Exemplars

A time series or heatmap query with `exemplar: true` also asks Prometheus for exemplars over the same range, through `/api/v1/query_exemplars`.
Prometheus returns them only when it runs with `--enable-feature=exemplar-storage` and the scraped application exposes them.
SlashObserve keeps at most one exemplar per step, the one with the highest value, and at most 2,000 per query.
If the exemplar request fails, the panel still shows its series.
