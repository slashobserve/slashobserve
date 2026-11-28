---
title: Upgrades and backups
description: Back up Postgres, upgrade the server and return to an older version.
---

This page shows how to back up Postgres, move to a newer version, and go back when you must.
SlashObserve is under active development, and releases before 1.0 can include breaking changes.
Before you upgrade, read the [changelog](https://github.com/slashobserve/slashobserve/blob/main/CHANGELOG.md) for each release between your version and the new one.
A release that has breaking changes lists them under "Breaking changes".

## Back up Postgres

Postgres is the only state.
It holds everything people make in the app: dashboards, observations, comments, folders, people, teams and invites, webhooks, the datasources added in the app and the activity log.

1. Take a dump before every upgrade, and on a schedule:

   ```sh
   pg_dump --format=custom --file slashobserve.dump --dbname "<the SLASHOBSERVE_DATABASE_URL value>"
   ```

   If your connection string carries pgx-only parameters such as `pool_max_conns`, remove them first, because `pg_dump` does not understand them.
2. Keep `SLASHOBSERVE_SECRET_KEY` with the dump.
   The credentials of datasources added in the app, the Grafana connection tokens and the webhook signing secrets are in the dump encrypted with it.
   They cannot be read after a restore without the same key.
3. Keep the rest of the configuration in your own secret and config management.
   That is the datasources file, the OIDC settings and the LLM key.
4. Protect the dump like the database itself, because it contains session and key hashes and your dashboards.

To restore, create an empty database and run `pg_restore --dbname "<database URL>" slashobserve.dump`.

For the [demo](demo.md), which runs Postgres in Compose:

```sh
docker exec slashobserve-astronomy-postgres-1 pg_dump -U slashobserve slashobserve > slashobserve.sql
```

## Upgrade

1. Take a backup.
2. Pull the new image tag (or build your own image) and replace the container.
   On Kubernetes, see [Upgrade and roll back](kubernetes.md#upgrade-and-roll-back).
3. Watch the log.
   The server applies pending migrations at startup, each logged as `applied migration` with its version.
4. Wait for `/readyz` to return 200, which it does once the migrations are done.

Migrations run under a Postgres lock, so starting several servers at once is safe: one migrates and the others wait.
With `SLASHOBSERVE_DATABASE_DIRECT_URL` set, migrations and their lock run on that connection.
[Run more than one server](scaling.md) covers a rolling upgrade.

### The migration baseline

The migration history of the 0.1.x releases is replaced by one baseline migration that creates the whole schema.
A database created by a 0.1.x release, or by a build from before the baseline, is not upgraded.
The server refuses to start on it with `migration_failed` and the message "this database was created before the migration baseline".

To move such an installation, keep its dashboards as files, start the new version on a new, empty database, and load them again:

```sh
observe dashboards export <folder> --dir ./dashboards
observe dashboards apply --dir ./dashboards
```

See [Keep dashboards as files](../use/cli.md#keep-dashboards-as-files).
Observations, comments, runs and the activity log stay with the old database.
For the [demo](demo.md), `make demo-reset` drops its volumes, and `scripts/e2e.sh down` does the same for the e2e stack.

## Go back to an older version

The server runs migrations forward only and never runs a migration's down section.
To go back, restore the backup you took before upgrading rather than running the older image against a migrated database.

## Reference

### Shutdown and leases

On `SIGTERM` the server answers 503 on `/readyz` (`status` and `phase` are `stopping`) for 5 seconds while it keeps serving, so a load balancer or Kubernetes Service stops sending new requests, then it stops accepting requests, gives running requests the rest of 15 seconds from the signal, and hands a running agent request back to the queue without using up an attempt.
After 10 seconds it ends the requests still open, such as live tails, event streams and `/mcp` streams.
Each one records its audit event before the server writes its last buffered audit events and exits.

A worker holds a 60-second lease on its request and renews it every 5 seconds while the run is in flight.
A crash instead leaves the lease to expire, so the next process picks the request up within a minute.
The built-in agent gives a run up when the lease ends without a renewal the server answered: it makes no further model call or report for that run and logs `lease lapsed without a successful report` at `WARN`.
