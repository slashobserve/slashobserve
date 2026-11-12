---
title: Upgrades and backups
description: Back up Postgres, upgrade the server and return to an older version.
---

## Back up Postgres

Postgres is the only state.
The datasources file, the OIDC settings and the LLM key are configuration you keep elsewhere; keep them in your own secret and config management.

Take a dump before every upgrade, and on a schedule:

```sh
pg_dump --format=custom --file slashobserve.dump --dbname "<the SLASHOBSERVE_DATABASE_URL value>"
```

Restore into an empty database with `pg_restore --dbname "<database URL>" slashobserve.dump`.
For the demo stack, which runs Postgres in Compose:

```sh
docker compose -f deploy/compose/compose.yaml exec -T postgres pg_dump -U slashobserve slashobserve > slashobserve.sql
```

If your connection string carries pgx-only parameters such as `pool_max_conns`, remove them first, because `pg_dump` does not understand them.

Dumps contain session and key hashes and your dashboards, so protect them like the database itself.

## Upgrade

1. Take a backup.
2. Pull the new image tag (or build your own image) and replace the container.
3. Watch the log: the server applies pending migrations at startup, each logged as `applied migration` with its version, and `/readyz` returns 200 once they are done.

The image now ships `observe-agent` and runs on `distroless/base-nossl`, so it is about 44 MB larger to pull (19 MB to 63 MB compressed on arm64) and about 107 MB larger unpacked: the 82 MB binary and a 24 MB C library layer.
The compose stack raises the server memory limit from 256M to 512M for the same reason.
The default agent runtime is now `sidecar`.
Set `SLASHOBSERVE_AGENT_RUNTIME=go` to keep the old runtime.
Memory per server grows by about 50 MiB, the idle size of the `observe-agent` child.
Run sessions of connected agents now get the built-in agent's guards.
The server does not start with `sidecar` when `observe-agent` is missing from `PATH` or when `SLASHOBSERVE_HTTP_ADDR` names a host other than a loopback address or all addresses.

### Migrations

Migrations run under a Postgres lock, so starting several servers at once is safe: one migrates and the others wait.
With `SLASHOBSERVE_DATABASE_DIRECT_URL` set, migrations and their lock run on that connection.

The server runs migrations forward only; it never runs a migration's down section.
To go back to an older version, restore the backup you took before upgrading rather than running the older image against a migrated database.

Before 1.0, a release never edits a migration that an earlier release shipped, and the server applies every pending migration in order.
So you can upgrade from any 0.x release directly to a later one without stopping at the releases between them.
Going backward is the one jump that is not supported.
The [changelog](https://github.com/slashobserve/slashobserve/blob/main/CHANGELOG.md) names a release whose migration rewrites existing data.

### Upgrading to the run states release

Migration 35 adds a `state` column to agent requests and checks it against `status`.
A server older than this release fails its queue writes against it, so stop every old server, or at least its agent workers, before the first new server starts.
To go back, run `goose down-to 34` against the database before starting an older binary.

### Upgrading to the persisted Grafana imports release

Migration 36 moves Grafana imports into the database and copies each connection's last import into it.
An import that an older server runs during the upgrade stays on that server and is not shown by the new ones, so finish or avoid imports while servers of both versions run.
To go back, run `goose down-to 35` before starting an older binary; it writes each connection's newest finished import back to the connection.

### Upgrading to the release that lets outside agents take runs

Migration 37 adds the assignment columns on runs, `run_tokens`, `agent_workers`, `oauth_grants.mode` and `service_accounts.takes_runs`.
A server older than this release claims runs without looking at who they are assigned to, so stop the old servers before any run is assigned to an outside agent.
To go back, check that no run is assigned to an outside agent, then run `goose down-to 36` against the database before starting an older binary.

### Upgrading from an early release

Upgrading from an early release changes the dashboard format: migration 10 rewrites every stored dashboard version from the earlier ObserveSpec v0 shape to evolved v0, in one transaction at startup.
Take the backup first; this migration cannot be reverted, and an older image cannot read the rewritten versions.
The server refuses to start if a stored version cannot be upgraded to a valid Spec, for example one that grows past the size limit; the log shows `migration_failed` with `migration_version` 10, the `dashboard_id` and the `dashboard_version` that failed (never the title or any Spec content), the database is left unchanged, and you either restore the backup or fix that version.

### Upgrading from a release before roles and folders

Upgrading from a release before roles and folders also changes access.
The migrations put every existing dashboard in the General folder and make every existing person and service account an Editor, and the first start then applies the rules in [Roles](../teams/roles.md).
Set `SLASHOBSERVE_ADMIN_EMAILS` before that first start; otherwise the earliest person who signed in becomes the Admin.

### Shutdown and leases

On `SIGTERM` the server stops accepting requests, gives running requests 15 seconds, and hands a running agent request back to the queue without using up an attempt.
A worker holds a 60-second lease on its request and renews it every 5 seconds while the run is in flight.
A crash instead leaves the lease to expire, so the next process picks the request up within a minute.
