---
title: Run in production
description: Put TLS in front of the server, use its health endpoints and run it behind a connection pooler.
---

Everything on this page applies to every install; [Run on Kubernetes](kubernetes.md) covers the Helm chart, and [Observe SlashObserve](observe-slashobserve.md) the dashboard that watches the server.

## Put TLS in front

The server speaks plain HTTP; it has no TLS settings.
Terminate TLS in a reverse proxy or load balancer and set `SLASHOBSERVE_PUBLIC_URL` to the address people use in the browser, scheme and host included.

That address matters beyond links:

- It is the OIDC redirect URI base.
  Register `<public url>/api/v1/auth/callback` with your identity provider.
- With an `https` address, session cookies are marked `Secure` and use the `__Host-` prefix.
  With `http` they are not, so use `http` only for local development.
- It is the one origin the MCP endpoint accepts in an `Origin` header.
- It is the trusted origin for the cross-origin check on cookie-authenticated writes.

Do not buffer or time out `GET /api/v1/dashboards/{id}/events`.
It is a Server-Sent Events stream that sends a ping every 15 seconds, ends at the first ping after its session or key stops being valid or the person can no longer see the dashboard, and ends after 30 minutes; browsers reconnect on their own.
Each person can hold 20 streams and one server process 1000; past either limit the server answers `429` or `503` with the code `too_many_streams`.
Allow request bodies of at least 8 MiB, the size limit of a Grafana import and of an MCP request.
The server ignores `X-Forwarded-For`.

## Health endpoints

| Endpoint       | Meaning                                                                                                |
| -------------- | ------------------------------------------------------------------------------------------------------ |
| `GET /healthz` | Liveness. It checks nothing else, so use it to decide whether to restart.                              |
| `GET /readyz`  | Readiness. It pings Postgres and confirms no migration is pending; it returns 200 only when both pass, and 503 otherwise. |

The `/readyz` body names each check, as in `{"status":"ok","checks":{"database":"ok","migrations":"ok"}}`, and a failing check shows `error` and logs `readiness_check_failed`.
The image's Docker health check runs `slashobserve-server healthcheck`, which requests `/readyz` on the listen address and exits 1 unless it answers 200.

Datasources never affect `/readyz`.
An unreachable Prometheus shows up as a datasource health problem, not as a server that is down.

## The image

The image carries `observe-agent` at `/usr/local/bin/observe-agent` beside `slashobserve-server`; the server starts it only when `SLASHOBSERVE_AGENT_RUNTIME=sidecar`.
Its base is `distroless/base-nossl`, which has glibc for the `observe-agent` binary and still runs as the non-root user `nonroot`.

## Behind a connection pooler

A transaction-mode pooler such as PgBouncer breaks the two things that need one Postgres session for a long time: `LISTEN`, which carries live dashboard updates and wakes the agent workers, and the lock that makes concurrent migrations safe.
Point `SLASHOBSERVE_DATABASE_URL` at the pooler and `SLASHOBSERVE_DATABASE_DIRECT_URL` straight at Postgres.
Live updates and migrations with their lock use the direct URL, and every other query uses the pooled one.

Each server keeps 2 connections open on the direct URL, and one more while it migrates, so count them in Postgres' `max_connections`.
The direct URL must reach the same database as the pooled one.
If it reaches another database, the server stops before it migrates and logs `database_direct_url_mismatch`.
An unreadable direct URL logs `invalid_database_direct_url`, and one that does not connect logs `database_unavailable` with `connection` set to `direct`.

With PgBouncer 1.21 or newer, set `max_prepared_statements` above `0` so the server's prepared statements work in transaction mode.
With an older PgBouncer, add `default_query_exec_mode=exec` to `SLASHOBSERVE_DATABASE_URL`.

Without the direct URL behind a transaction pooler, live updates and the migration lock do not work.
For several servers, see [Run more than one server](scaling.md).
