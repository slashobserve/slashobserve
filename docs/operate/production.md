---
title: Run in production
description: Put TLS in front of the server, set the public URL and trusted proxies, keep live streams open, check the health endpoints and run behind a connection pooler.
---

Use this page when you put SlashObserve behind a reverse proxy or load balancer for real users.
It applies to every install; [Run on Kubernetes](kubernetes.md) covers the Helm chart, and [Observe SlashObserve](observe-slashobserve.md) covers the dashboard that watches the server.

## Put TLS in front

The server speaks plain HTTP and has no TLS settings.
Terminate TLS in a reverse proxy or load balancer, then tell the server the address people use.

1. Set `SLASHOBSERVE_PUBLIC_URL` to that address, scheme and host included, for example `https://observe.example.com`.
2. Register `https://observe.example.com/api/v1/auth/callback` with your identity provider.
3. Start the server and sign in through the proxy.
   The session cookie is now marked `Secure` and uses the `__Host-` prefix.

Use an `http` address only for local development.
With `http` the cookies are not marked `Secure`.

Details of what the public URL controls:

- It is the base of the OIDC redirect URI.
- It is the one origin the MCP endpoint accepts in an `Origin` header.
- It is the trusted origin for the cross-origin check on cookie-authenticated writes.

## Keep live streams open

The server sends four Server-Sent Events streams.
Do not buffer them or time them out in the proxy:

- `GET /api/v1/dashboards/{id}/events`
- `GET /api/v1/activity/stream`
- `GET /api/v1/audit/stream`
- `GET /api/v1/datasources/{uid}/tail`

Every stream ends after 30 minutes, and browsers reconnect on their own.
Past a stream limit the server answers `429` (per person) or `503` (per server process) with the code `too_many_streams`.
On the dashboard, activity and audit streams the body's `reason` is `per_caller` or `server_capacity`.
The audit stream answers `429` for both limits, so read the `reason` there.
Before the server ends a dashboard, activity or audit stream itself, it sends a last `closed` event whose `reason` says why, for example `lifetime` or `stopping`.
A stream that ends without `closed` was cut by the client, the network or a proxy.

Also set your proxy to:

- Allow request bodies of at least 8 MiB, the size limit of a Grafana import and of an MCP request.
- Keep `/metrics` off the public internet, because it needs no sign-in (see [Metrics](metrics.md)).
- Send `X-Forwarded-For` or another forwarding header when it runs on the same host as the server, so the server can tell its traffic from the built-in agent's; see [The agent process](../concepts/agent.md#the-agent-process).

## Record the real client address

Set `SLASHOBSERVE_TRUSTED_PROXIES` to the addresses or CIDR ranges your proxy connects from.
The audit trail then records the person's address and not the proxy's.

```sh
SLASHOBSERVE_TRUSTED_PROXIES=10.0.4.0/28,10.0.9.12
```

Unset, which is the default, the client address is the address the connection came from, and the server ignores `X-Forwarded-For`.
Set, the server reads `X-Forwarded-For` only on a connection from a listed address.
It takes the rightmost address in the header that is not itself listed, so a client cannot choose its own address by sending the header.

Follow these rules:

- List every proxy that appends to the header, such as a CDN and the load balancer behind it.
  An address you leave out is taken for the client.
- List the narrowest range your proxy or Ingress controller connects from, never the whole pod network.
  A client inside a listed range can send any address it likes.
- Do not list a range with no network bits, such as `0.0.0.0/0` or `::/0`.
  The server refuses it at startup.
- Expect a load balancer that writes `ip:port` into `X-Forwarded-For`, as some cloud ones do, to record the proxy's own address.
  The server does not accept an entry with a port.

The check that keeps the built-in agent's token off forwarded requests still uses the connection's own address.
It refuses the token on any request that carries a forwarding header.

## Serve the app over HTTP/2

Serve the proxy over HTTP/2.
The server itself speaks HTTP/1.1, which lets a browser fetch about six files at once.
The first visit loads the app's script files, about 35 on the entry page and 270 KB gzipped, and over HTTP/2 they load in one wave.

Two proxy settings matter:

- A proxy that recompresses responses must pass `Content-Encoding: br` through and key its cache on `Accept-Encoding`, which the server lists in `Vary`.
- A proxy can cache files under `/assets/` for a year, because they carry a content hash in their name and `Cache-Control: immutable`.

Details of compression:

- Files under `/assets/` ship brotli-compressed at build time, and the server sends that copy to a browser that accepts `br`.
- The largest file, the shared vendor chunk, is about 19 percent smaller than its gzip copy (200 KB against 246 KB).
- Every other response is compressed with gzip.

## Check that the server is healthy

Point your load balancer or orchestrator at these endpoints.

| Endpoint | Meaning |
| --- | --- |
| `GET /healthz` | Liveness. It checks nothing else, so use it to decide whether to restart. |
| `GET /readyz` | Readiness. It returns 200 when Postgres answers and no migration is pending, and 503 otherwise, and 503 with `status` `stopping` from the shutdown signal on. |

The `/readyz` body names each check and the phase the server process is in:

```json
{"status":"ok","checks":{"database":"ok","migrations":"ok"},"phase":"ready"}
```

A failing check shows `error` in the body and logs `readiness_check_failed`.
`phase` is `ready` while the server serves and `stopping` from the shutdown signal on.
The same word is the `phase` attribute of the log line each phase starts with (`starting slashobserve-server`, `migrating database`, `provisioning`, `listening`, `shutting down`, `server stopped`) and the `phase` label of the `slashobserve_server_phase` series, so one word finds a phase in a probe, the log and an alert.
The six phases and their order are in [States](../reference/states.md#server-process).
Datasources never affect `/readyz`; an unreachable Prometheus shows up as a datasource health problem, not as a server that is down.

The image's Docker health check runs `slashobserve-server healthcheck`.
It requests `/readyz` on the listen address and exits 1 unless it answers 200.

## Behind a connection pooler

A transaction-mode pooler such as PgBouncer breaks the two things that need one Postgres session for a long time.
They are `LISTEN`, which carries live dashboard updates and wakes the agent workers, and the lock that makes concurrent migrations safe.
To keep them working:

1. Point `SLASHOBSERVE_DATABASE_URL` at the pooler.
2. Point `SLASHOBSERVE_DATABASE_DIRECT_URL` straight at Postgres, at the same database.
3. On PgBouncer 1.21 or newer, set `max_prepared_statements` above `0` so the server's prepared statements work in transaction mode.
   On an older PgBouncer, add `default_query_exec_mode=exec` to `SLASHOBSERVE_DATABASE_URL`.

Live updates and migrations with their lock use the direct URL, and every other query uses the pooled one.
Without the direct URL behind a transaction pooler, live updates and the migration lock do not work.
Each server keeps 3 connections open on the direct URL for `LISTEN`, and one more while it migrates, so count 4 per server in Postgres' `max_connections`.
For several servers, see [Run more than one server](scaling.md).

## Reference

### Direct URL errors

If the direct URL fails, the server stops at startup and logs one of these codes.

| Code | Cause |
| --- | --- |
| `database_direct_url_mismatch` | The direct URL reaches another database than the pooled one. |
| `invalid_database_direct_url` | The direct URL is not a valid PostgreSQL connection string. |
| `database_unavailable` | The direct URL does not connect; the log has `connection` set to `direct`. |

### Stream limits

| Stream | Per person | Per server process | Ping |
| --- | --- | --- | --- |
| Dashboard events | 20 | 1000 | 15 seconds |
| Activity stream | 5 | 500 | 15 seconds |
| Audit stream | 5 | 20 | 5 seconds |
| Log tail | 5 | 200 | 15 seconds |

The dashboard events stream also tells the page within 5 seconds when the person's own role changes.
It ends at the first ping after its session or key stops being valid, or after the person can no longer see the dashboard.

### The image

The image carries `observe-agent` at `/usr/local/bin/observe-agent` beside `slashobserve-server`.
The server starts `observe-agent` whenever a model provider is configured.
The base is `distroless/base-nossl`, which has glibc for the `observe-agent` binary and still runs as the non-root user `nonroot`.
