---
title: Run more than one server
description: What to set for two or more servers on one Postgres, behind a pooler, what each server counts on its own, and how to check they work as one.
---

Run two or more servers on one Postgres when you want the app to stay up through a crash, a node drain or an upgrade.
The servers behave as one for scheduled jobs, events, agent runs, webhooks, OAuth tokens and provisioning.

Each server heartbeats into the database and counts the live servers.
It divides the limits that cap a resource all servers share, and keeps every other limit for itself.

## Add a second server

These steps run two servers on Kubernetes against a Postgres behind a pooler.
[Run on Kubernetes](kubernetes.md) covers the rest of the chart.

1. Create the Secret `slashobserve-database`.
   It holds the pooled address under `url` and the direct address under `direct-url`.
2. Set the [Helm values for two or more servers](#helm-values-for-two-or-more-servers).
3. Upgrade the release.
   A rolling upgrade migrates on the first new pod while the others wait on the lock.
4. Check that both servers report in, as [Check it is working](#check-it-is-working) shows.

Without Helm, run each server with the same settings and put a load balancer with no session affinity in front.

## What to set

Set these on every server:

- The same `SLASHOBSERVE_SECRET_KEY`, OIDC settings and `SLASHOBSERVE_PUBLIC_URL`.
- The same datasources file, because each server reads it only at startup.
  A datasource you add in the app is stored in Postgres, and the other servers pick it up within 10 seconds.
- `SLASHOBSERVE_AGENT_WORKERS`, which defaults to 8.
  The fleet runs the replica count times this many agent requests at once.
- `SLASHOBSERVE_DATABASE_DIRECT_URL`, whenever `SLASHOBSERVE_DATABASE_URL` goes through a transaction pooler.

Set these once for the fleet:

- One Postgres that every server uses, with a [backup](upgrades-and-backups.md).
- `limits.maxConcurrentQueries` on each datasource, as the total number of queries the datasource can take from all servers.
  It defaults to 32, and you set it in the datasources file or on the datasource's form in the app; see [the datasources file](../reference/datasources-file.md).

A datasource's fleet total goes in the datasources file the chart renders:

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

## Decide whether you need another server

A second server is for availability first.
Capacity per server depends on your hardware, so [measure it](#measure-your-own-hardware) before you size a fleet by it.
The ceiling of the agent is usually the rate limit of your model provider, not the number of servers.

Each server with a model provider runs one `observe-agent` child process that works that server's runs, so the fleet runs one per server.
Measured on a compose stack with the replay model, an idle child holds about 50 MiB resident (`docker top`).
The whole server container, child included, shows about 48 MiB in `docker stats`, and the compose server limit is 512M.

## Connections

Each server opens a pool on `SLASHOBSERVE_DATABASE_URL`.
The pool holds at most the largest of 16, `2 x workers + 8` and the machine's CPU count.
That is 24 at the default 8 workers on a host with 24 or fewer CPUs.
A `pool_max_conns` in the URL replaces that size.

Each server also keeps 3 connections on the direct URL for `LISTEN` (events, run claims and live agent text).
It uses one more while it migrates or runs the provisioning and service-account locks.

Plan Postgres' `max_connections` like this:

- Without a pooler, about `replicas x (24 + 4)` connections at the defaults, plus your own headroom for backups and people.
- With a pooler, `replicas x 4` direct connections, and the size of the pooler's pool is yours to set.

`pg_stat_activity` shows what Postgres sees, and `SHOW POOLS` in the pooler's admin console shows the pooled side.

## Run behind a pooler

[Behind a connection pooler](production.md#behind-a-connection-pooler) has the rules: point the pooled URL at the pooler and the direct URL straight at Postgres.
Without the direct URL, live updates and the migration lock do not work.
The locks that make provisioning and the service-account file run on one server at a time do not work either.
The two-server check runs PgBouncer 1.26.1 in transaction mode with `max_prepared_statements = 200`.

## Check it is working

Query these in Prometheus, or read the first three on the "Are the servers sharing the work?" row of the [internals dashboard](observe-slashobserve.md):

- `max(slashobserve_fleet_servers)` and `min(slashobserve_fleet_servers)` both equal the replica count, because every server reports the same count.
- `slashobserve_datasource_slots_limit` on each server is the datasource's total divided by the live servers, rounded up.
- `sum by (datasource) (slashobserve_datasource_slots_in_use) / sum by (datasource) (slashobserve_datasource_slots_limit)` close to 1, together with `429` answers, means the datasource is the limit.
  Raise `maxConcurrentQueries` only if the datasource can take more.
- `histogram_quantile(0.95, sum by (le) (rate(slashobserve_agent_queue_wait_seconds_bucket[15m])))` rising means agent requests wait for a worker.
  Add workers or servers if your model provider allows it.

To list the servers, run this in Postgres; no HTTP endpoint does:

```sql
SELECT id, version, started_at, seen_at FROM server_instances;
```

[Metrics](metrics.md) lists the series.

## Measure your own hardware

Run `make loadtest` in a checkout.
It starts its own stack and never touches another one.

It runs dashboard viewers, activity streams, MCP clients and simultaneous agent builds against one server and against two, then the builds alone with 2 and 16 workers.
The agent builds use a replay model, so they measure the queue and the database, not your provider.
It takes about 50 minutes and writes `loadtest/reports/<time>-<label>/report.md`.
The report has one table per scenario, the peak CPU and memory of every container, and a pass or fail for each gate.

## Reference

### Helm values for two or more servers

These values run two servers against a Postgres behind a pooler.

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

### Per server and fleet-wide limits

A limit is divided among the live servers only when it caps a total over every caller of a resource that all servers share and that gives no backpressure of its own.
Every other limit protects one server's own capacity or one caller, and stays per server.
A caller whose requests spread over N servers can have up to N times a per-server cap in flight.

| Limit | Value | Scope |
| --- | --- | --- |
| Queries running against one datasource | `maxConcurrentQueries`, default 32 | Fleet-wide |
| Queries running on one server, all datasources | 32 | Per server |
| OAuth registrations, authorizations, token requests and revocations | 30, 120, 1200 and 120 per minute | Fleet-wide |
| OAuth token requests of one client | 120 per minute | Per server |
| MCP requests in flight | 8 per person or service account, 32 per server | Per server |
| Dashboard event streams | 20 per person, 1000 per server | Per server |
| Activity streams | 5 per person, 500 per server | Per server |
| Live tails | 5 per person, 200 per server | Per server |
| Panel data requests in flight | 8 per caller | Per server |
| Query drafts in flight | 3 per caller, 10 per server | Per server |
| Query drafts started | 60 per caller per clock hour | Fleet-wide |
| Grafana imports | 1 per caller, 4 per server | Per server |
| Agent requests running at once | `SLASHOBSERVE_AGENT_WORKERS` | Per server |
| New dashboard requests queued or running | 20 in all, 3 per person | Fleet-wide |
| Investigations queued or running | 10 in all, 3 per person | Fleet-wide |

Details of why a limit has its scope:

- Datasource queries are fleet-wide because the datasource is shared, so each server gets its share, rounded up.
- OAuth limits that count every client are fleet-wide because they cap a total over every client.
- Query drafts started are fleet-wide because Postgres counts them, so the model spend stays capped however many servers there are.
- New dashboard requests and investigations are fleet-wide because Postgres counts them.
- The agent request limit is per server because the fleet runs the replica count times `SLASHOBSERVE_AGENT_WORKERS`.
- The other per-server limits protect the server or cap one caller.

### Known limits

- A Grafana import on a server that stops is handed to another server, or continues after the restart, within about 10 seconds.
  One whose server is killed waits for its 60 second lease, and an import whose server dies twice ends failed.
- After a crash, the other servers count the dead one for up to 30 seconds, so their shares are briefly smaller.
  A new server is counted within 10 seconds.
- Another server retries a killed server's running agent requests once their 60 second lease expires, up to 2 attempts in all.
- Streams on a stopped server end, and the client reconnects.
  An activity stream resumes from `Last-Event-ID`.
- A rolling upgrade migrates on the first new pod while the others wait on the lock; see [Upgrades and backups](upgrades-and-backups.md).
