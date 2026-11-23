---
title: Upgrades and backups
description: Back up Postgres, upgrade the server and return to an older version.
---

SlashObserve is under active development, and releases before 1.0 can include breaking changes.
Before you upgrade, read the [changelog](https://github.com/slashobserve/slashobserve/blob/main/CHANGELOG.md) for each release between your version and the new one; a release that has breaking changes lists them under "Breaking changes".

## Back up Postgres

Postgres is the only state.
It holds everything people make in the app: dashboards, observations, comments, folders, people, teams and invites, webhooks, the datasources added in the app and the activity log.
The datasources file, the OIDC settings, the LLM key and `SLASHOBSERVE_SECRET_KEY` are configuration you keep elsewhere; keep them in your own secret and config management.
Keep the secret key with the backup: the credentials of datasources added in the app, the Grafana connection tokens and the webhook signing secrets are in the dump encrypted with it, and they cannot be read after a restore without the same key.

Take a dump before every upgrade, and on a schedule:

```sh
pg_dump --format=custom --file slashobserve.dump --dbname "<the SLASHOBSERVE_DATABASE_URL value>"
```

Restore into an empty database with `pg_restore --dbname "<database URL>" slashobserve.dump`.
For the [demo](demo.md), which runs Postgres in Compose:

```sh
docker exec slashobserve-astronomy-postgres-1 pg_dump -U slashobserve slashobserve > slashobserve.sql
```

If your connection string carries pgx-only parameters such as `pool_max_conns`, remove them first, because `pg_dump` does not understand them.

Dumps contain session and key hashes and your dashboards, so protect them like the database itself.

## Upgrade

1. Take a backup.
2. Pull the new image tag (or build your own image) and replace the container.
3. Watch the log: the server applies pending migrations at startup, each logged as `applied migration` with its version, and `/readyz` returns 200 once they are done.

### Upgrading from 0.1.1 or earlier

Release 0.1.2 ships `observe-agent` in the image, on `distroless/base-nossl`, so the image is about 44 MB larger to pull (19 MB to 63 MB compressed on arm64).
The compose stack raised the server memory limit from 256M to 512M for the same reason.
The built-in agent runs as the `observe-agent` sidecar, a separate process the server starts, instead of inside the server process.
Memory per server grows by about 50 MiB, the idle size of the `observe-agent` child.
Run sessions of connected agents get the built-in agent's guards.
The server does not start with a model provider when `observe-agent` is missing from `PATH` or when `SLASHOBSERVE_HTTP_ADDR` names a host other than a loopback address or all addresses.
It also adds migrations 26 to 48, and the notes below on migrations 35, 36, 37, 41 and 43 apply to that jump.

### Migrations

Migrations run under a Postgres lock, so starting several servers at once is safe: one migrates and the others wait.
With `SLASHOBSERVE_DATABASE_DIRECT_URL` set, migrations and their lock run on that connection.

The server runs migrations forward only; it never runs a migration's down section.
To go back to an older version, restore the backup you took before upgrading rather than running the older image against a migrated database.
The `goose down-to <version>` commands below use the goose command line tool, run against the database from a checkout of the release that applied the migrations, with `-dir internal/db/migrations` and the database URL; the table of applied versions is `goose_db_version`.

Before 1.0, a release never edits a migration that an earlier release shipped, and the server applies every pending migration in order.
So you can upgrade from any 0.x release directly to a later one without stopping at the releases between them.
Going backward is the one jump that is not supported.
The [changelog](https://github.com/slashobserve/slashobserve/blob/main/CHANGELOG.md) names a release whose migration rewrites existing data.

### Migration 35, run states

Migration 35 adds a `state` column to agent requests and checks it against `status`.
A server older than this release fails its queue writes against it, so stop every old server, or at least its agent workers, before the first new server starts.
To go back, run `goose down-to 34` against the database before starting an older binary.

### Migration 36, persisted Grafana imports

Migration 36 moves Grafana imports into the database and copies each connection's last import into it.
An import that an older server runs during the upgrade stays on that server and is not shown by the new ones, so finish or avoid imports while servers of both versions run.
To go back, run `goose down-to 35` before starting an older binary; it writes each connection's newest finished import back to the connection.

### Migration 37, outside agents taking runs

Migration 37 adds the assignment columns on runs, `run_tokens`, `agent_workers`, `oauth_grants.mode` and `service_accounts.takes_runs`.
A server older than this release claims runs without looking at who they are assigned to, so stop the old servers before any run is assigned to an outside agent.
To go back, check that no run is assigned to an outside agent, then run `goose down-to 36` against the database before starting an older binary.

### Migration 41, findings become observations

Migration 41 gives every finding an observation and makes `findings.observation_id` required.
A finding that an investigation saved keeps the observation of its run.
Every other finding becomes a resolved observation with the finding's id, filed in the folder the finding was in, else its dashboard's folder, else the saver's My Dashboards, else General, with the finding's text as a text block and each evidence panel as a panel block.
The comment threads of a finding are merged into one discussion on its observation, which is reopened when any of them was open, and the activity of a finding moves to its observation and that observation's folder.
Take the backup first.
Stop every old server before the first new one starts, because an older server saves findings without an observation and opens threads on a finding, which the new schema refuses.

To go back, run `goose down-to 40` against the database before starting an older binary.
The down section has costs:

- Threads that were merged stay merged, and are not split into their original threads again.
- The observations that were made from findings are deleted with everything on them, so edits, blocks and resolves made on them since the upgrade are lost; the observation of a run is kept.
- A finding with no dashboard comes back filed in the folder of its observation, so a finding that a service account saved with no folder comes back filed in General.

### Migration 43, the inbox

Migration 43 adds `inbox_cursors`, one read position per person, the `inbox_reason` SQL function and the partial index `activity_inbox_idx` on the activity log.
The index is built with a plain `CREATE INDEX`, which blocks writes to the activity log, and so every change that records activity, until it finishes.
At the default 180 day retention the build is short, but a log that holds millions of events can take a minute or more, so upgrade when little is changing.
An older server ignores the new objects, so servers of both versions can run together.
To go back, run `goose down-to 42` against the database before starting an older binary; everyone's read position is lost, and the next inbox shows the last 30 days as unread.

### Upgrading from 0.1.3

Migrations 74 to 90 are new, and there are no migrations 76 to 78.
They add the audit trail and its export position, the audience of a datasource, Only Admins folders, anchors and agent columns on threads, chats, the record of starter dashboards, observation block heights, run intent, read restrictions on runs and a delete trigger for dashboards.
Take the backup first.

Migration 74 copies every activity event into the new audit table, keeping only the data keys each audit action allows.
It puts events from the 24 months before the migration into one partition per UTC month and older events into one per UTC year, and leaves out an activity event dated after the migration, which it counts in a notice.
Retention drops a partition only once its end is older than `SLASHOBSERVE_AUDIT_RETENTION`, so backfilled history older than two years is kept until the end of its year.
During a rolling upgrade, servers still on the older version keep writing activity events without an audit event, so for 7 days after the migration the `audit_partitions` job, and every server at start, records an audit event for each activity event that has none.

Migrations 81, 82, 84 and 87 run outside a transaction, add each check constraint and foreign key as `NOT VALID` and validate it afterwards, and build their indexes concurrently, so none of them holds a write lock for its whole run.
Migration 82 backfills chats for earlier requests and migration 87 sets the intent and kind of every run, both in batches of 1000 rows.
Migrations 82 and 87 also add triggers that fill the chat, the intent and the kind of a request that an older server writes while it is still running.

The server records an audit event for every sign-in, change, refusal, tool call and query, and keeps them for `SLASHOBSERVE_AUDIT_RETENTION`, one year by default, so the database grows with its use.
Set it to the history you want before the first start, read [Audit trail](audit.md), and set `SLASHOBSERVE_TRUSTED_PROXIES` when a load balancer or ingress controller sits in front of the server, so the recorded client address is the caller's and not the balancer's.
[Configuration](../reference/configuration.md) lists both settings and `SLASHOBSERVE_AUDIT_OTLP_ENDPOINT`, which copies the trail to a log pipeline and sends nothing while it is unset.
On `SIGTERM` the server now also ends the requests that are still open after 10 seconds, see [Shutdown and leases](#shutdown-and-leases).

To go back, run `goose down-to 73` against the database before starting an older binary.
The down sections undo the new tables and columns, and some also delete data the older schema cannot hold:

- Migration 74 drops the audit table with every audit event in it, its detached partitions included, and migration 85 drops the export position.
- Migration 87 puts each run's `source` back from its intent and deletes the investigation requests that continue another.
- Migration 82 drops the chats table, which holds each chat's title, pin and archive mark.
- Migration 81 drops the anchors and agent columns of threads and deletes the threads of observations that have an anchor or a block.
- Migration 79 drops the Only Admins mark of folders, so such a folder, which has no access list, reads as open again, and migration 75 drops the audience of datasources, so every datasource can be queried by everyone again.
- Migration 84 drops block heights and sets a block of a third or two thirds width to half width.
- Migration 88 drops the read restriction marks of runs and the flags on copied answers.

### Upgrading from 0.1.2

Migrations 49 to 73 are new, and there is no migration 66.
They add tables and columns for chats, inbox item states, webhook formats, per-person thread read marks, dashboard viewers and view days, home dashboards, datasources added in the app, people invites and deactivation, teams, chat-only findings, observation pins, run time zones and access requests.
Take the backup first.
Migrations 74 to 90 came with 0.1.4, so read [Upgrading from 0.1.3](#upgrading-from-013) as well.

Migrations 59, 62 and 73 replace a check constraint on the activity log, which scans the whole log under a lock that blocks reads and writes.
Migrations 50 and 73 rebuild `activity_inbox_idx` with a plain `CREATE INDEX`, which blocks writes to the log, and so every change that records activity, until it finishes.
Like migration 43, the cost grows with the log, so when it holds millions of events, upgrade when little is changing.
Migration 63 records the earliest person to have signed in as the first user, the person the first-user rule of [Roles](../teams/roles.md) names.
Migration 71 sets a reply time on every finished request that was started from a thread message, in one `UPDATE`.

To go back, run `goose down-to 48` against the database before starting an older binary; that also runs the down sections of migrations 74 to 90 listed above.
The down sections undo the new tables and columns, and some also delete data the older schema cannot hold:

- Migration 73 drops access requests and deletes their activity events, and migration 70 drops each request's time zone.
- Migration 65 deletes every finding that no observation holds, which is a chat answer nobody saved.
- Migration 64 drops teams and their members, so folder entries that named a team match no one.
- Migration 63 drops invites and the deactivation marks, so a person an Admin deactivated can sign in again.
- Migration 62 drops the datasources added in the app, with their stored credentials, and deletes their activity events; dashboards that use them then have no datasource to read from.
- Migration 59 deletes the activity events of admin changes, which is the Audit log.
- Migration 58 deletes saved metrics searches.
- Migration 52 drops each webhook's format, so Slack and Teams webhooks send the generic payload again, and deletes the deliveries skipped by an Admin.
- Migrations 50 and 72 drop the inbox's per-item read and done marks, 53 drops the thread read marks and the dashboard viewers, and 60, 61 and 68 drop dashboard view days, home dashboards and observation pins and dashboard links.
- Migrations 49, 51, 54, 55, 57, 67, 69 and 71 only drop the columns they added, such as the chat a request belongs to (49), the link from a retry to its request (54), the reason a proposal was discarded (55) and the label of a public link (57); migration 56 drops the hourly count of query drafts.

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
After 10 seconds it ends the requests still open, such as live tails, event streams and `/mcp` streams, so each one records its audit event before the server writes its last buffered audit events and exits.
A worker holds a 60-second lease on its request and renews it every 5 seconds while the run is in flight.
A crash instead leaves the lease to expire, so the next process picks the request up within a minute.
