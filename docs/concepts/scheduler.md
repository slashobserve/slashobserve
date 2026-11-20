---
title: Scheduled jobs
description: How the server runs background jobs on a schedule, once across all servers, with a lease that a crashed server gives up on its own.
---

The server runs background work, such as maintenance, as scheduled jobs.
Each job has a name and an interval, and it runs once per interval across all the servers that share a database, never once per server.

## Jobs and their schedule

Every job has one row in the `jobs` table of the database, written when a server starts with the job registered.
The row keeps when the job runs next, which server holds it, and when and how it last ran: `last_outcome` is `succeeded` or `failed`, and `last_error` is a short description of a failure, such as its error type, never the error's own text.

A server looks for due jobs when it starts and every 15 seconds after.
Each due job runs in its own goroutine, so a long job such as the dashboard health check never holds up a short one that is due at the same time.
A run starts the next interval: a job that runs every 5 minutes and starts at 10:00 is due again at 10:05, whether it succeeded or failed, so a failing job is retried on its normal schedule.
When a newer server changes a job's interval, the row takes the new interval and keeps its next run time.

## One run per server fleet

A server takes a due job with a lease, written in the job's row in the same statement that claims it, so two servers looking at the same moment never both run it.
The lease lasts the job's interval, at most 10 minutes.

When the lease ends, the job's context is cancelled, so a job must stop when its context is done.
A job that ignores its context is not killed, but once its lease has ended another server may start the job again, and when one does, the late result of the first run is not recorded.

A server that crashes mid-run leaves its lease behind, and the first server that finds the job due after the lease ends takes it over.
A server that shuts down mid-run gives the lease back at once and records no outcome, so another server runs the job without waiting for the lease to end.

## The heartbeat job

Every server registers `scheduler_heartbeat`, a job that does nothing every 5 minutes.
It keeps a row in the `jobs` table and a series in the metrics from the first start, so you can tell the scheduler is running before any other job exists.

## Other jobs

These jobs run hourly:

- `dashboard_health` runs the [dashboard health check](../use/dashboard-health.md#the-health-check).
- `activity_prune` deletes old [activity](../use/activity.md#retention) events.
- `webhook_deliveries_prune` deletes [webhook](../use/webhooks.md#attempts) attempts older than 7 days.
- `oauth_prune` deletes expired OAuth codes, tokens and sign-in requests, grants that expired or were revoked more than 30 days ago, and clients that registered over 24 hours ago and never signed in, which [MCP](../use/mcp.md) clients leave behind.
- `run_tokens_prune` deletes expired run tokens and the records of agent workers whose grant or service account is gone.

`agent_requests_sweep` runs every minute.
It fails agent requests that ran out of attempts, ends requests whose run was canceled, expires requests that no agent claimed within 10 minutes, retries thread replies that did not post in the last 24 hours, and deletes expired build records.

## Metrics

The scheduler reports two series:

- `slashobserve_scheduler_jobs_total`, a counter by `job` and `outcome`: `succeeded`, `failed`, or `interrupted` for a run stopped by a shutdown.
- `slashobserve_scheduler_job_running`, a gauge by `job` that is 1 while this server runs the job.
