---
title: Run in production
description: Put TLS in front of the server and use its health endpoints.
---

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
The server ignores `X-Forwarded-For`.

## Health endpoints

| Endpoint       | Meaning                                                                                                |
| -------------- | ------------------------------------------------------------------------------------------------------ |
| `GET /healthz` | Liveness. It checks nothing else, so use it to decide whether to restart.                              |
| `GET /readyz`  | Readiness. It pings Postgres and confirms no migration is pending; it returns 200 only when both pass. |

Datasources never affect `/readyz`.
An unreachable Prometheus shows up as a datasource health problem, not as a server that is down.
