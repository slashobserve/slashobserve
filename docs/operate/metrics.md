---
title: Metrics
description: The Prometheus metrics the server serves at /metrics, what each series counts and how to scrape it.
---

The server serves Prometheus metrics at `/metrics` in the text exposition format.
Use them to watch the API, the agent, the query proxy, the database pool, the scheduler and the audit recorder, and to alert on them.
The series carry route templates, codes and model names, never dashboard ids, user names, query text or prompt text.

## Scrape it

1. Point a Prometheus scrape job at the server's address with the path `/metrics`:

   ```yaml
   scrape_configs:
     - job_name: slashobserve
       metrics_path: /metrics
       static_configs:
         - targets: ["slashobserve.internal:8080"]
   ```

2. Reload Prometheus, and check that the target is up.
3. Chart the series with the [internals dashboard](observe-slashobserve.md), or query them as in [Useful queries](#useful-queries).

A series with labels appears after its first observation, so a server that has run no agent request serves no `slashobserve_agent_runs_total` yet.
On Kubernetes, the Helm chart adds `prometheus.io` scrape annotations to the pod and can create a Prometheus Operator ServiceMonitor; see [Run on Kubernetes](kubernetes.md#metrics).

## Keep the route private

The route needs no sign-in, so keep it off the public internet.
Let Prometheus reach the server on an internal network, or block `/metrics` at the reverse proxy in front of the server.
Scrapes are logged at `debug` only, like health checks.

## Turn the route off

`SLASHOBSERVE_METRICS` controls the route.
It is on by default.
`false` removes it, so the path answers like any unknown page.

## Read audit counts

The `slashobserve_audit_*` series describe only the audit recorder's own health.
Counts of audit events by action, person or outcome are not exported here, because `/metrics` is open to anyone and those counts are for Admins only.
Queries on the built-in audit datasource (`slashobserve-audit`) are left out of `slashobserve_proxy_query_duration_seconds` and the slot gauges for the same reason, since they would show when Admins read the trail.
Read audit counts through that datasource instead; see [Audit queries](../reference/audit-queries.md).

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

## Reference

### Series

| Series | Type | Labels | What it counts |
| --- | --- | --- | --- |
| `slashobserve_http_request_duration_seconds` | histogram | `route`, `method`, `status` | Time to answer an HTTP request. |
| `slashobserve_agent_runs_total` | counter | `kind`, `outcome` | Agent runs that ended. |
| `slashobserve_agent_queue_wait_seconds` | histogram | `source` | Time an agent request waited before the agent claimed it. |
| `slashobserve_thread_replies_abandoned_total` | counter | none | Replies of finished agent runs that could not be posted in their comment thread within 24 hours, so the server stopped trying; the run reads `reply.state` `abandoned`. |
| `slashobserve_model_call_duration_seconds` | histogram | `provider`, `model`, `outcome` | Time of one model call, including its retries. |
| `slashobserve_model_tokens_total` | counter | `direction` | Tokens the model provider reported. |
| `slashobserve_proxy_query_duration_seconds` | histogram | `datasource_type`, `outcome` | Time of one datasource call from the query proxy. |
| `slashobserve_proxy_cache_total` | counter | `result` | Panel data lookups in the query proxy's shared result cache. |
| `slashobserve_scheduler_jobs_total` | counter | `job`, `outcome` | Scheduled job runs that ended. |
| `slashobserve_scheduler_job_running` | gauge | `job` | Runs of a scheduled job in flight on this server. |
| `slashobserve_db_pool_connections` | gauge | `state` | Database pool connections that are `acquired`, `idle` or `constructing`. |
| `slashobserve_db_pool_max_connections` | gauge | none | The most connections the database pool opens. |
| `slashobserve_db_pool_acquires_total` | counter | none | Connections taken from the database pool. |
| `slashobserve_db_pool_empty_acquires_total` | counter | none | Acquires that waited because the pool had no idle connection. |
| `slashobserve_db_pool_acquire_wait_seconds_total` | counter | none | Time spent waiting to acquire a pool connection. |
| `slashobserve_datasource_slots_in_use` | gauge | `datasource` | Queries running against a datasource on this server. |
| `slashobserve_datasource_slots_limit` | gauge | `datasource` | Queries this server allows against a datasource at once. |
| `slashobserve_fleet_servers` | gauge | none | Servers that heartbeat into this database in the last 30 seconds, this one included. |
| `slashobserve_audit_dropped_total` | counter | `reason` | Audit events not stored, or data keys dropped. |
| `slashobserve_audit_write_seconds` | histogram | none | Time of one attempt to write a batch of read and refusal events. |
| `slashobserve_audit_buffer_events` | gauge | none | Read and refusal events waiting in this server's buffer at the last flush. |
| `slashobserve_audit_partitions_ahead` | gauge | none | Daily audit partitions after today. |
| `slashobserve_audit_export_lag_seconds` | gauge | none | Seconds between the newest audit event and the last one the audit export sent. |
| `slashobserve_audit_export_rejected_total` | counter | none | Audit events the receiver accepted the request for but refused in its answer. |
| `slashobserve_audit_export_dropped_total` | counter | `reason` | Audit events the audit export gave up on. |

The Go runtime series (`go_*`) and the process series (`process_*`) come from the Prometheus client library.

Details of the series that need more than a cell:

- `slashobserve_agent_queue_wait_seconds` does not count a retry after a restart or a lost lease.
- `slashobserve_proxy_query_duration_seconds` times panel data, variable values and save checks alike.
- `slashobserve_datasource_slots_limit` is this server's share of the datasource's `maxConcurrentQueries`.
- `slashobserve_audit_partitions_ahead` drops below 14 when the `audit_partitions` job stops.

### Labels

- `route` is the route template, such as `/api/v1/dashboards/:id`, never the path a caller sent, so dashboard ids stay out of the labels.
  A request that matched no route, a page of the web app included, is `unmatched`.
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
- `job` on the scheduler series is the scheduled job's name.
  The scrape adds its own `job` label, so Prometheus stores the scheduler series' value as `exported_job`, unless the scrape job sets `honor_labels: true`.
  The internals dashboard groups by `exported_job`.

### Audit series

`slashobserve_audit_dropped_total` counts events and data keys by `reason`:

- `buffer_full`: the buffer was full.
- `write_failed`: a batch write failed for a reason a retry cannot fix, or was still failing when the server stopped.
- `record_failed`: the direct write of a change's request event failed.
- `undeclared_key`: data keys an event did not declare.

`slashobserve_audit_write_seconds` includes a timed out attempt.

`slashobserve_audit_export_lag_seconds` is `0` when the [audit export](audit-export.md) has caught up.
It stays above `0` while the export is stuck, a long transaction hides new events from it, or no server runs the job.
Every server that has the export set reads it from the database every 15 seconds, so any of those servers' scrape shows it.
A server without the export set exposes the series at `0` and never updates it.

`slashobserve_audit_export_rejected_total` counts events the receiver of the [audit export](audit-export.md) accepted the request for but refused in its partial success answer.
They are not sent again.

`slashobserve_audit_export_dropped_total` counts by `reason`:

- `refused`: events the receiver refused on their own with `400`, `413` or `422`.
- `retention`: gaps where events were deleted before they were exported.

### How the audit recorder retries a write

A batch that times out, loses its connection, waits for a lock or meets a database that is read-only during a failover is kept and written again every second.
It is retried for as long as the database stays unavailable, so a slow database moment loses nothing while the buffer has room.
A batch that an earlier attempt already stored is not written twice or counted as dropped.
A batch the server cancels for taking too long, or that times out three times in a row, is split in half and written as two.

While a batch is kept, new events queue up behind it.
Once the buffer is full they are dropped as `buffer_full`, and one `audit.gap` event records how many once writing resumes.
During that time each new event can wait up to 100 milliseconds for room.
A change's own event is written straight to the database rather than in a batch.
When that write fails in one of the ways above, it is handed to the same retry, up to the size of the buffer, and a refused row costs only itself.
Events still unwritten when the server stops count as `write_failed`.
