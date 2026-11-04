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
  Every person, Viewers included, can ask the agent for dashboards in their My Dashboards, which spends model tokens and lets the agent query every configured datasource for them.
  New people are Editors by default, so allow into SlashObserve only the people you would trust to create and change dashboards.
- **Folders hide dashboards.**
  A restricted folder (see [Folders and access](../teams/folders.md)) is hidden from everyone who is not on its list, did not create it and is not an Admin, in lists, search, tags, datasource dashboard counts, panel data, comments, events, import records and the agent.
  The server filters lists by the folders the caller can see, so a failed lookup shows nothing rather than everything, and re-checks access on each request, including at every ping of an event stream.
  A key matches no folder entry, so only an Admin key reaches a restricted folder.
  A My Dashboards folder is hidden from everyone but its owner, Admins included.
- **Share links need sign-in.**
  The Share pane copies a dashboard or panel address with the time range and variable values, and grants nothing: someone without access to the folder sees "You don't have access to this dashboard" after they sign in.
- **Datasource credentials stay on the server.**
  They never reach the browser.
  The datasources API returns health only, with no address, user or secret.
  The browser gets panel data from the server's query proxy, which runs only the queries stored in a saved dashboard and rejects any query text sent to it with `403 query_not_in_spec`.
  There are two exceptions, and in both the browser still sends no query text.
  The draft preview lets a signed-in user run an unsaved panel's queries before saving, under the same limits as the agent's tools, and service-account keys cannot.
  The build data routes (`/api/v1/observe/{id}/build/panels/{panel_id}/data` and `/api/v1/observe/{id}/build/variables/{name}/values`) run the queries of the dashboard an agent request is still assembling, which is not a saved version yet.
  Only the signed-in user who made the request can call them, and for a revision only while they can still view the dashboard; anyone else gets 404 and a service-account key gets `403 user_session_required`.
  They load the stored build on the server, run under the same limits as the agent's tools, and never cache or share results.
  People who can see a dashboard see how far its revision has come, never the build's panels.
  The revisions list (`GET /api/v1/dashboards/{id}/revisions`) is visible to every viewer of the dashboard, and carries a stopped build's progress counts, besides the continue note a person gave; the build's title is shown only to the person who asked, and is null for everyone else.
- **Only the requester saves a stopped build.**
  When an agent run fails, its build is kept for 7 days.
  The person or key that made the request can save it as one version with `POST /api/v1/observe/{id}/build/save`, if its role is Editor or higher and it can edit the dashboard (or create dashboards, for a new one); nobody else can, and the route answers them 404.
  No query runs when it is saved, because the agent checked each step as it made it.
- **Only the requester cancels or continues a request.**
  `POST /api/v1/observe/{id}/cancel` and `POST /api/v1/observe/{id}/continue` answer anyone else 403, or 404 for a new-dashboard request they cannot see, and no role or folder level overrides that.
  Continue also needs the access that starting a run needs, and it answers 409 when someone saved a newer version of the dashboard after the build started.
  A run that does not end holds back other editors' revisions on its dashboard until its limits, its requester or a run time limit end it.
- **Every query is bounded.**
  Whatever a prompt or a client asks for, each query has a timeout and limits on series, points and rows, applied on the server.
  ClickHouse is queried only as a read-only user.
- **The agent can only save dashboards.**
  Its only product is a dashboard definition that the server validates, and no code or markup written by the model is rendered.
  A save from the agent or an MCP client is refused when it writes HTML in a text panel (`html_not_allowed`) or adds or changes a canvas or field action (`action_not_allowed`), so only people and imports can create them.
  Through MCP, a key can also list datasources, run bounded queries and sample rows, and create and update dashboards.
- **Outbound requests are allowlisted.**
  The server fetches content from other sites only for domains on the allowlist, which starts empty, and only over `https` to public addresses; private, loopback and metadata addresses are refused even when a name resolves to one.
  This covers news feeds, images, map tiles, GeoJSON files and actions.
  Redirects are checked hop by hop, and no cookies or credentials are sent.
  A canvas or field action also needs Edit on the dashboard and a confirmation in the browser, and the server never follows a redirect for it.
  See [Outbound access](outbound-access.md).
- **The browser loads nothing from other sites.**
  Every response carries a strict `Content-Security-Policy`: scripts, stylesheets, images, fonts and connections come only from the server's own origin (images may also be `data:` or `blob:` URLs, and inline styles are allowed), and plugins, frames, media and `<base>` are refused.
  So even if a sanitizer were ever bypassed, injected script could not run or send data to another site.
  Images and GeoJSON from other sites reach the browser only through the server, which fetches them from allowlisted domains (see [Outbound access](outbound-access.md#images-and-geojson)).
- **HTML in text panels is sanitized.**
  HTML written by a person or imported from Grafana passes through one allowlist in the browser before it is drawn.
  Scripts, styles, forms, embedded frames, event handler attributes, links and images with any scheme but http, https and (for links) mailto are removed, and so are inline style declarations that load a resource or read a variable.
  Every image loads through the server, so the policy above still decides what the browser fetches.
- **Framing is refused.**
  Responses carry `X-Frame-Options: DENY` and `frame-ancestors 'none'` in the policy.
- **Keep keys short-lived.**
  Prefer an expiry, one service account per tool or person, and revoke on the API keys page when a laptop or pipeline is retired.
