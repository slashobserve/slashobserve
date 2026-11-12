---
title: Run more than one server
description: What to set for two or more servers on one Postgres, behind a pooler, what each server counts on its own, and how to check they work as one.
---

Several servers can share one Postgres and behave as one server for scheduled jobs, events, agent runs, webhooks, OAuth tokens and provisioning.
Each server heartbeats into the database and counts the live servers.
It divides the limits that cap a resource all servers share, and every other limit it keeps for itself.
This page says what to set, which limit is which, and how to check that the servers share the work.

## When to add a server

A second server is for availability first: a crash, a node drain or an upgrade leaves the other server answering.
Capacity per server depends on your hardware, so [measure it](#measure-your-own-hardware) before you size a fleet by it.
The ceiling of the agent is usually the rate limit of your model provider, not the number of servers.

## What to set

- `replicaCount` in the Helm chart, or the replica count of your own Deployment.
- One Postgres that every server uses, with a [backup](upgrades-and-backups.md).
- `SLASHOBSERVE_AGENT_WORKERS` on each server.
  It defaults to 8, and the fleet runs the replica count times this many agent requests at once.
- With the default `SLASHOBSERVE_AGENT_RUNTIME=sidecar`, each server runs one `observe-agent` child process that works that server's runs, so the fleet runs one per server.
  Measured on a compose stack with the replay model, an idle child holds about 50 MiB resident (`docker top`), and the whole server container, child included, shows about 48 MiB in `docker stats`; the compose server limit is 512M.
- `limits.maxConcurrentQueries` on each datasource, as the total number of queries the datasource can take from the whole fleet.
  It defaults to 32; see [the datasources file](../reference/datasources-file.md).
- `SLASHOBSERVE_DATABASE_DIRECT_URL`, whenever `SLASHOBSERVE_DATABASE_URL` goes through a transaction pooler.
- The same `SLASHOBSERVE_SECRET_KEY`, OIDC settings and `SLASHOBSERVE_PUBLIC_URL` on every server.
- A load balancer with no session affinity.

## Connections

Each server opens a pool on `SLASHOBSERVE_DATABASE_URL`.
The pool holds at most the largest of 16, `2 x workers + 8` and the machine's CPU count, which is 24 at the default 8 workers on a host with 24 or fewer CPUs.
A `pool_max_conns` in the URL replaces that size.
Each server also keeps 2 connections on the direct URL for `LISTEN`, and one more while it migrates.

Without a pooler, Postgres needs `max_connections` for about `replicas x (24 + 3)` connections at the defaults, plus your own headroom for backups and people.
With a pooler, the direct connections are `replicas x 3`, and the size of the pooler's pool is yours to set.
`pg_stat_activity` shows what Postgres sees, and `SHOW POOLS` in the pooler's admin console shows the pooled side.

## Behind a pooler

[Behind a connection pooler](production.md#behind-a-connection-pooler) has the rules: point the pooled URL at the pooler and the direct URL straight at Postgres.
The two-server check runs PgBouncer 1.26.1 in transaction mode with `max_prepared_statements = 200`.
Without the direct URL, live updates, the migration lock and the locks that make provisioning and the service-account file run on one server at a time do not work.

## Per server and fleet-wide

A limit is divided among the live servers only when it caps a total over every caller of a resource that all servers share and that gives no backpressure of its own.
Every other limit protects one server's own capacity or one caller, and stays per server.

| Limit | Value | Scope | Why |
| --- | --- | --- | --- |
| Queries running against one datasource | `maxConcurrentQueries`, default 32 | Fleet-wide | The datasource is shared, so each server gets its share, rounded up. |
| Queries running on one server, all datasources | 32 | Per server | It protects the server. |
| OAuth registrations, authorizations, token requests and revocations | 30, 120, 1200 and 120 per minute | Fleet-wide | They cap a total over every client. |
| OAuth token requests of one client | 120 per minute | Per server | It caps one caller. |
| MCP requests in flight | 8 per key, 32 per server | Per server | They protect the server. |
| Dashboard event streams | 20 per person, 1000 per server | Per server | They protect the server. |
| Activity streams | 5 per person, 500 per server | Per server | They protect the server. |
| Live tails | 5 per person, 200 per server | Per server | They protect the server. |
| Panel data requests in flight | 8 per caller | Per server | It caps one caller. |
| Query drafts | 3 per caller, 10 per server | Per server | They protect the server. |
| Grafana imports | 1 per caller, 4 per server | Per server | They protect the server. |
| Agent requests running at once | `SLASHOBSERVE_AGENT_WORKERS` | Per server | The fleet runs the replica count times this. |
| New dashboard requests queued or running | 20 in all, 3 per person | Fleet-wide | Postgres counts them. |

A caller whose requests spread over N servers can have up to N times a per-server cap in flight.

## Helm values for two or more servers

These values run two servers against a Postgres behind a pooler.
The Secret `slashobserve-database` holds the pooled address under `url` and the direct address under `direct-url`; [Run on Kubernetes](kubernetes.md) shows how to create it.

```yaml
replicaCount: 2
database:
  existingSecret: slashobserve-database
  urlKey: url
  directUrlKey: direct-url
env:
  - name: SLASHOBSERVE_AGENT_WORKERS
    value: "8"
resources:
  requests:
    cpu: "1"
```

A datasource's total for the fleet goes in the datasources file the chart renders:

```yaml
datasources:
  datasources:
    - uid: prom
      type: prometheus
      name: Prometheus
      prometheus:
        url: http://prometheus.monitoring:9090
      limits:
        maxConcurrentQueries: 64
```

## Check it is working

- `max(slashobserve_fleet_servers)` and `min(slashobserve_fleet_servers)` both equal the replica count, because every server reports the same count.
- `slashobserve_datasource_slots_limit` on each server is the datasource's total divided by the live servers, rounded up.
- `sum by (datasource) (slashobserve_datasource_slots_in_use) / sum by (datasource) (slashobserve_datasource_slots_limit)` close to 1 together with `429` answers means the datasource is the limit.
  Raise `maxConcurrentQueries` only if the datasource can take more.
- `histogram_quantile(0.95, sum by (le) (rate(slashobserve_agent_queue_wait_seconds_bucket[15m])))` rising means agent requests wait for a worker.
  Add workers or servers if your model provider allows it.
- `SELECT id, version, started_at, seen_at FROM server_instances` lists the servers; no HTTP endpoint does.
- The row "Are the servers sharing the work?" of the [internals dashboard](observe-slashobserve.md) shows the first three.

[Metrics](metrics.md) lists the series.

## Known limits

- A Grafana import on a server that stops is handed to another server, or continues after the restart, within about 10 seconds; one whose server is killed waits for its 60 second lease, and an import whose server dies twice ends failed.
- After a crash, the other servers count the dead one for up to 30 seconds, so their shares are briefly smaller.
  A new server is counted within 10 seconds.
- A killed server's running agent requests are retried by another server once their 60 second lease expires, up to 2 attempts in all.
- Streams on a stopped server end, and the client reconnects; an activity stream resumes from `Last-Event-ID`.
- A rolling upgrade migrates on the first new pod while the others wait on the lock; see [Upgrades and backups](upgrades-and-backups.md).

## Measure your own hardware

`make loadtest` in a checkout starts its own stack and never touches another one.
It runs dashboard viewers, activity streams, MCP clients and simultaneous agent builds against one server and against two, and then the builds alone with 2 and 16 workers.
The agent builds use a replay model, so they measure the queue and the database, not your provider.
It takes about 50 minutes and writes `loadtest/reports/<time>-<label>/report.md` with one table per scenario, the peak CPU and memory of every container, and a pass or fail for each gate.
