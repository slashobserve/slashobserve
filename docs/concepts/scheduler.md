---
title: Scheduled jobs
description: How the server runs background jobs on a schedule, once across all servers, with a lease that a crashed server gives up on its own, and the list of every job with its interval.
---

The server runs recurring background work, such as deleting old activity events, as scheduled jobs.
Each job has a name and an interval, and it runs once per interval across all the servers that share a database, never once per server.

For example, `activity_prune` runs every hour.
If you run three servers against one Postgres, one of them runs it at 10:00, one of them runs it at 11:00, and the others skip each run.
You do not configure the schedule, and you do not start a second scheduler.

You see jobs in three places.
An Admin lists them with `GET /api/v1/jobs`, which gives each job's `state`: `running`, `interrupted`, `due` or `waiting`.
The `jobs` table of the database has one row per job, and [two metrics](#metrics) report runs.
A failed run also leaves a `job.run` event in the audit trail.

## How a job runs

Every server registers the same jobs when it starts.
It then looks for due jobs at start and every 15 seconds after.

1. The server writes or updates one row per job in the `jobs` table.
   A job seen for the first time is due at once.
   When a newer server changes a job's interval, the row takes the new interval and keeps its next run time.
2. A server claims a due job with a lease.
   The claim is one database statement, so two servers looking at the same moment never both get the job.
3. The server runs the job in its own goroutine.
   A long job, such as the dashboard health check, never holds up a short one that is due at the same time.
4. When the job ends, the server records the outcome and releases the lease.
   The next run is due one interval after this run started.

A job that runs every 5 minutes and starts at 10:00 is due again at 10:05, whether it succeeded or failed.
A failing job is retried on its normal schedule.

## What happens when a server stops

The lease lasts the job's interval, at most 10 minutes.
When the lease ends, the job's context is cancelled, so a job must stop when its context is done.

- A server that crashes mid-run leaves its lease behind.
  The first server that finds the job due after the lease ends takes it over, and records the crashed run's outcome as `interrupted` with no error.
- A server that shuts down mid-run gives the lease back at once and records no outcome.
  Another server runs the job without waiting for the lease to end.
- A job that ignores its context is not killed.
  Once its lease has ended another server may start the job again, and the late result of the first run is not recorded.

## Run the health check now

An Admin can start the `dashboard_health` job from the dashboards list with **Run now**; see [Run now](../use/dashboard-health.md#run-a-check-now).
The run takes the job's lease like a scheduled run and is recorded with a `trigger` of `manual`.
No other job can be started by hand.
When the job already holds a lease, the request records a `job.run` event with the outcome `denied` and the reason `already_running`.

## Reference

### Columns of a job row

| Column | Holds |
| --- | --- |
| `name` | The job name, the primary key. |
| `every_seconds` | The interval. |
| `next_run_at` | When the job is next due. |
| `leased_by`, `lease_until` | The server that holds the job and when its lease ends. |
| `last_started_at`, `last_finished_at` | When the last run began and ended. |
| `last_outcome` | `succeeded` or `failed`, or `interrupted` after a server took over the lease of a run that crashed. |
| `last_error` | A short description of a failure. |

### States of a job

`GET /api/v1/jobs` computes a job's `state` from its row on every read; nothing stores it.
The [states reference](../reference/states.md#scheduled-job) lists the same four.

| State | When |
| --- | --- |
| `running` | A server holds its lease and is running it. |
| `interrupted` | Its last run did not finish, because the server crashed or shut down. The next server that finds it due runs it again. |
| `due` | Its next run time has passed and no server has claimed it yet. |
| `waiting` | Its last run finished, or it never ran, and the next run time is ahead. |

Details of `last_error`:

- It holds `key=value` pairs: `error_type`, and when they apply `sqlstate`, `errno` and `panic_type`.
- It never holds the error's own text, which can echo queries or credentials.
- A job that panics records `error_type=scheduler.panicError` and `panic_type=<Go type of the panic value>`.

### The audit event of a run

The transaction that writes the outcome also records one `job.run` audit event in two cases.
It records one when a person started the run, whatever its outcome, and when the run failed, whoever started it.
A scheduled run that succeeds records none, and neither does a job stopped by a shutdown.

| Field | Value |
| --- | --- |
| Target | Kind `job`, with the job name as the id. |
| Actor | The person who started a manual run, or the system for a scheduled failure. |
| `trigger` | `schedule` or `manual`. |
| `duration_ms` | How long the run took. |
| Outcome | `success`, or `failed` with the reason `job_failed`, or `panic` when the job panicked. |

### Jobs

| Job | Interval | What it does |
| --- | --- | --- |
| `scheduler_heartbeat` | 5 minutes | Does nothing. |
| `dashboard_health` | 1 hour | Runs the [dashboard health check](../use/dashboard-health.md#the-health-check). |
| `activity_prune` | 1 hour | Deletes old [activity](../use/activity.md#retention) events. |
| `webhook_deliveries_prune` | 1 hour | Deletes [webhook](../use/webhooks.md#attempts) attempts older than 7 days. |
| `oauth_prune` | 1 hour | Deletes expired OAuth data. |
| `run_tokens_prune` | 1 hour | Deletes expired run tokens and stale agent worker records. |
| `agent_requests_sweep` | 1 minute | Settles stuck agent requests. |
| `audit_partitions` | 24 hours | Keeps the audit table's partitions. |
| `audit_export` | 10 minutes | Posts audit events to a receiver, when configured. |

Details of `scheduler_heartbeat`:

- Every server registers it.
- It keeps a row in the `jobs` table and a series in the metrics from the first start, so you can tell the scheduler runs before any other job exists.

Details of `oauth_prune`:

- It deletes expired OAuth codes, tokens and sign-in requests.
- It deletes grants that expired or were revoked more than 30 days ago.
- It deletes clients that registered over 24 hours ago and never signed in, which [MCP](../use/mcp.md) clients leave behind.

Details of `run_tokens_prune`:

- It deletes expired run tokens.
- It deletes the records of agent workers whose grant or service account is gone.

Details of `agent_requests_sweep`:

- It fails agent requests that ran out of attempts, which is 2.
- It ends requests whose run was canceled.
- It expires requests that no agent claimed within 10 minutes.
- It retries thread replies that did not post in the last 24 hours.
- It deletes expired build records.

Details of `audit_partitions`:

- It keeps the audit table's daily partitions 14 days ahead and drops those older than `SLASHOBSERVE_AUDIT_RETENTION`; see [Configuration](../reference/configuration.md#audit).
- Every server also runs it once at start.
- For 7 days after the audit table's migration, it also records an audit event for each activity event an older server wrote without one; see [Upgrades and backups](../operate/upgrades-and-backups.md).

Details of `audit_export`:

- It exists only when `SLASHOBSERVE_AUDIT_OTLP_ENDPOINT` is set.
- It keeps one server posting audit events to the receiver for the whole lease, and the next run starts as that one ends; see [Audit export](../operate/audit-export.md).

### Metrics

| Series | Type | Labels | Meaning |
| --- | --- | --- | --- |
| `slashobserve_scheduler_jobs_total` | counter | `job`, `outcome` | Runs that ended. |
| `slashobserve_scheduler_job_running` | gauge | `job` | 1 while this server runs the job. |

The `outcome` is `succeeded`, `failed`, or `interrupted` for a run stopped by a shutdown.
