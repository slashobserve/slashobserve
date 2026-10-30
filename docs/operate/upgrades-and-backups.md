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
2. Build or pull the new image and replace the container.
3. Watch the log: the server applies pending migrations at startup, each logged as `applied migration` with its version, and `/readyz` returns 200 once they are done.

Migrations run under a Postgres lock, so starting several servers at once is safe: one migrates and the others wait.
The milestone 1 release changes the dashboard format: migration 10 rewrites every stored dashboard version from the earlier ObserveSpec v0 shape to evolved v0, in one transaction at startup.
Take the backup first; this migration cannot be reverted, and an older image cannot read the rewritten versions.
The server refuses to start if a stored version cannot be upgraded to a valid Spec, for example one that grows past the size limit; the error names the dashboard and version, the database is left unchanged, and you either restore the backup or fix that version.
The server runs migrations forward only; it never runs a migration's down section.
To go back to an older version, restore the backup you took before upgrading rather than running the older image against a migrated database.

On `SIGTERM` the server stops accepting requests, gives running requests 15 seconds, and hands a running agent request back to the queue without using up an attempt.
A crash instead leaves the request's lease to expire, so the next process picks it up after the lease ends.
