---
title: Metrics
description: The Prometheus metrics the server serves at /metrics, what each series counts and how to scrape it.
---

The server serves Prometheus metrics at `/metrics` in the text exposition format.
`SLASHOBSERVE_METRICS` controls the route: it is on by default, and `false` removes it, so the path answers like any unknown page.
Scrapes are logged at `debug` only, like health checks.
The route needs no sign-in, so keep it off the public internet: let Prometheus reach the server on an internal network, or block `/metrics` at the reverse proxy in front of the server.
The series carry route templates, codes and model names, never dashboard ids, user names, query text or prompt text.

## Scrape it

Point a Prometheus scrape job at the server's address with the path `/metrics`.

```yaml
scrape_configs:
  - job_name: slashobserve
    metrics_path: /metrics
    static_configs:
      - targets: ["slashobserve.internal:8080"]
```

The [demo stack](../get-started/quick-start.md) already scrapes its server this way, as the `slashobserve` job of its Prometheus.
On Kubernetes, the Helm chart adds `prometheus.io` scrape annotations to the pod and can create a Prometheus Operator ServiceMonitor; see [Run on Kubernetes](kubernetes.md#metrics).
The [internals dashboard](observe-slashobserve.md) charts these series.

## Series

A series with labels appears after its first observation, so a server that has run no agent request serves no `slashobserve_agent_runs_total` yet.

| Series                                            | Type      | Labels                         | What it counts                                                                                           |
| ------------------------------------------------- | --------- | ------------------------------ | -------------------------------------------------------------------------------------------------------- |
| `slashobserve_http_request_duration_seconds`      | histogram | `route`, `method`, `status`    | Time to answer an HTTP request.                                                                          |
| `slashobserve_agent_runs_total`                   | counter   | `kind`, `outcome`              | Agent runs that ended.                                                                                   |
| `slashobserve_agent_queue_wait_seconds`           | histogram | `source`                       | Time an agent request waited before the agent claimed it; a retry after a restart or a lost lease is not counted. |
| `slashobserve_model_call_duration_seconds`        | histogram | `provider`, `model`, `outcome` | Time of one model call, including its retries.                                                           |
| `slashobserve_model_tokens_total`                 | counter   | `direction`                    | Tokens the model provider reported.                                                                      |
| `slashobserve_proxy_query_duration_seconds`       | histogram | `datasource_type`, `outcome`   | Time of one datasource call from the query proxy, for panel data, variable values and save checks alike. |
| `slashobserve_proxy_cache_total`                  | counter   | `result`                       | Panel data lookups in the query proxy's shared result cache.                                             |
| `slashobserve_scheduler_jobs_total`               | counter   | `job`, `outcome`               | Scheduled job runs that ended.                                                                           |
| `slashobserve_scheduler_job_running`              | gauge     | `job`                          | Runs of a scheduled job in flight on this server.                                                        |
| `slashobserve_db_pool_connections`                | gauge     | `state`                        | Database pool connections that are `acquired`, `idle` or `constructing`.                                 |
| `slashobserve_db_pool_max_connections`            | gauge     |                                | The most connections the database pool opens.                                                            |
| `slashobserve_db_pool_acquires_total`             | counter   |                                | Connections taken from the database pool.                                                                |
| `slashobserve_db_pool_empty_acquires_total`       | counter   |                                | Acquires that waited because the pool had no idle connection.                                            |
| `slashobserve_db_pool_acquire_wait_seconds_total` | counter   |                                | Time spent waiting to acquire a pool connection.                                                         |
| `slashobserve_datasource_slots_in_use`            | gauge     | `datasource`                   | Queries running against a datasource on this server.                                                     |
| `slashobserve_datasource_slots_limit`             | gauge     | `datasource`                   | Queries this server allows against a datasource at once: its share of the datasource's `maxConcurrentQueries`. |
| `slashobserve_fleet_servers`                      | gauge     |                                | Servers that heartbeat into this database in the last 30 seconds, this one included.                     |

The Go runtime series (`go_*`) and the process series (`process_*`) come from the Prometheus client library.

## Labels

- `route` is the route template, such as `/api/v1/dashboards/:id`, never the path a caller sent, so dashboard ids stay out of the labels; a request that matched no route, a page of the web app included, is `unmatched`.
- `method` is the HTTP method, or `other` for a method the server does not serve.
- `status` is the HTTP status code the server answered.
- `kind` is `create` for a new dashboard, `revise` for a change to an existing one, `suggest` for a single suggested panel, and `investigate` for an investigation.
- `source` on `slashobserve_agent_queue_wait_seconds` is the request's source: `comment`, `prompt`, `observe`, `suggest` or `investigate`.
- `outcome` on `slashobserve_agent_runs_total` is `succeeded` or the failure code the request shows, such as `rate_limited`, `run_timeout` or `canceled`.
- `outcome` on `slashobserve_model_call_duration_seconds` is `ok`, `rate_limited`, `call_timeout`, `provider_error`, `canceled`, or `error` for any other failure.
- `direction` is `input`, `output`, `cache_read` or `cache_write`.
- `datasource_type` is the datasource type, such as `prometheus` or `clickhouse`.
- `outcome` on `slashobserve_proxy_query_duration_seconds` is `ok` or the query error code a panel shows, such as `query_invalid`, `query_timeout`, `limit_exceeded` or `datasource_unavailable`.
- `result` is `hit` or `miss`.

## Useful queries

The share of API requests that failed with a server error over the last 5 minutes:

```promql
sum(rate(slashobserve_http_request_duration_seconds_count{status=~"5.."}[5m]))
  / sum(rate(slashobserve_http_request_duration_seconds_count[5m]))
```

The number of live servers:

```promql
max(slashobserve_fleet_servers)
```

How much of each datasource's slots the whole fleet uses:

```promql
sum by (datasource) (slashobserve_datasource_slots_in_use) / sum by (datasource) (slashobserve_datasource_slots_limit)
```

How much of each datasource's slots this server uses:

```promql
slashobserve_datasource_slots_in_use / slashobserve_datasource_slots_limit
```

The 95th percentile time an agent request waited for a worker:

```promql
histogram_quantile(0.95, sum by (le) (rate(slashobserve_agent_queue_wait_seconds_bucket[15m])))
```

The 95th percentile time of a model call, by model:

```promql
histogram_quantile(0.95, sum by (model, le) (rate(slashobserve_model_call_duration_seconds_bucket[15m])))
```

Agent runs that failed in the last day, by failure code:

```promql
sum by (outcome) (increase(slashobserve_agent_runs_total{outcome!="succeeded"}[1d]))
```

The share of panel data requests the cache answered:

```promql
sum(rate(slashobserve_proxy_cache_total{result="hit"}[15m])) / sum(rate(slashobserve_proxy_cache_total[15m]))
```
