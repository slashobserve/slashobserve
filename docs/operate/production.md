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

Do not buffer or time out the three Server-Sent Events streams: `GET /api/v1/dashboards/{id}/events`, `GET /api/v1/activity/stream` and `GET /api/v1/datasources/{uid}/tail`.
The dashboard events stream sends a ping every 15 seconds, tells the page within 5 seconds when the person's own role changes, ends at the first ping after its session or key stops being valid or the person can no longer see the dashboard, and ends after 30 minutes; browsers reconnect on their own.
Each person can hold 20 streams and one server process 1000; past either limit the server answers `429` or `503` with the code `too_many_streams`.
A log tail ends after 30 minutes too, and each person can hold 5 tails and one server process 200.
Allow request bodies of at least 8 MiB, the size limit of a Grafana import and of an MCP request.
Keep `/metrics` off the public internet, because it needs no sign-in (see [Metrics](metrics.md)).
A proxy on the same host as the server must send `X-Forwarded-For` or another forwarding header, so the server can tell its traffic from the built-in agent's; see [The agent process](../concepts/agent.md#the-agent-process).

Set `SLASHOBSERVE_TRUSTED_PROXIES` to the addresses or CIDR ranges your proxy connects from, so the audit trail records the person's address and not the proxy's.
Unset, which is the default, the client address is the address the connection came from, and the server ignores `X-Forwarded-For`.
Set, the server reads `X-Forwarded-For` only on a connection from a listed address, and takes the rightmost address in it that is not itself listed, so a client cannot choose its own address by sending the header.
List every proxy that appends to the header, such as a CDN and the load balancer behind it; an address you leave out is taken for the client.
List the narrowest range your proxy or Ingress controller connects from, never the whole pod network, because a client inside a listed range can send any address it likes.
The server refuses a range with no network bits, such as `0.0.0.0/0` or `::/0`, at startup.
A load balancer that writes `ip:port` into `X-Forwarded-For`, as some cloud ones do, always records the proxy's own address, because the server does not accept an entry with a port.
The check that keeps the built-in agent's token off forwarded requests still uses the connection's own address, and refuses the token on any request that carries a forwarding header.

Serve the proxy over HTTP/2.
The server itself speaks HTTP/1.1, which lets a browser fetch about six files at once, and the first visit loads the app's script files (about 35 on the entry page, 270 KB gzipped).
Over HTTP/2 they load in one wave.
Files under `/assets/` ship brotli-compressed at build time and the server sends that copy to a browser that accepts `br`, which makes the largest file, the shared vendor chunk, about 19 percent smaller than gzip (200 KB against 246 KB); every other response is compressed with gzip.
A proxy that recompresses responses must pass `Content-Encoding: br` through and key its cache on `Accept-Encoding`, which the server lists in `Vary`.
Files under `/assets/` carry a content hash in their name and `Cache-Control: immutable`, so a proxy can cache them for a year.

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

The image carries `observe-agent` at `/usr/local/bin/observe-agent` beside `slashobserve-server`; the server starts it whenever a model provider is configured.
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
