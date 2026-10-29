---
title: Security model
description: What SlashObserve protects and what an operator should know.
---

- **Sign-in.**
  Only accounts from your OIDC provider, and only in the allowed email domains, get a session.
  Sessions and keys are random tokens stored only as SHA-256 hashes.
  Session cookies are `HttpOnly` and `SameSite=Lax`, and a cross-origin write made with a cookie is refused; `SLASHOBSERVE_PUBLIC_URL` is a trusted origin.
- **Roles.**
  Every person and service account has an instance role (see [Roles](../teams/roles.md)), and the server checks it on every API route.
  Only Admins see everyone who has signed in, sign people out, manage service accounts and keys, open the Datasources page, or change the outbound allowlist (unless `SLASHOBSERVE_OUTBOUND_DOMAINS` locks it).
  New people are Editors by default, so allow into SlashObserve only the people you would trust to create and change dashboards.
- **Folders hide dashboards.**
  A restricted folder (see [Folders and access](../teams/folders.md)) is hidden from everyone who is not on its list, did not create it and is not an Admin, in lists, search, tags, datasource dashboard counts, panel data, comments, events, import records and the agent.
  The server filters lists by the folders the caller can see, so a failed lookup shows nothing rather than everything, and re-checks access on each request, including at every ping of an event stream.
  A key matches no folder entry, so only an Admin key reaches a restricted folder.
- **Share links need sign-in.**
  The Share pane copies a dashboard or panel address with the time range and variable values, and grants nothing: someone without access to the folder sees "You don't have access to this dashboard" after they sign in.
- **Datasource credentials stay on the server.**
  They never reach the browser.
  The datasources API returns health only, with no address, user or secret.
  The browser gets panel data from the server's query proxy, which runs only the queries stored in a saved dashboard and rejects any query text sent to it with `403 query_not_in_spec`.
  The one exception is the draft preview: a signed-in user can run an unsaved panel's queries before saving, under the same limits as the agent's tools, and service-account keys cannot.
- **Every query is bounded.**
  Whatever a prompt or a client asks for, each query has a timeout and limits on series, points and rows, applied on the server.
  ClickHouse is queried only as a read-only user.
- **The agent can only save dashboards.**
  Its only product is a dashboard definition that the server validates, and no code or markup written by the model is rendered.
  Through MCP, a key can also list datasources, run bounded queries and sample rows, and create and update dashboards.
- **Outbound requests are allowlisted.**
  The server fetches content from other sites only for domains on the allowlist, which starts empty, and only over `https` to public addresses; private, loopback and metadata addresses are refused even when a name resolves to one.
  Redirects are checked hop by hop, and no cookies or credentials are sent.
  See [Outbound access](outbound-access.md).
- **Framing is refused.**
  Responses carry `X-Frame-Options: DENY` and `Content-Security-Policy: frame-ancestors 'none'`.
- **Keep keys short-lived.**
  Prefer an expiry, one service account per tool or person, and revoke on the API keys page when a laptop or pipeline is retired.
