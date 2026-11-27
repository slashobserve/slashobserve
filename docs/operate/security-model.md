---
title: Security model
description: What SlashObserve protects, which settings an operator should choose for a safe deployment, and the limits and checks the server applies to sign-in, dashboards, datasources, the agent, outbound requests, keys and connected agents.
---

This page describes what the server protects and what you decide as the person who runs it.
Each section lists the guarantees the server keeps, so you can tell what you still have to configure.

## Harden a new deployment

Work through these settings before you give people access.

1. Put TLS in front and set `SLASHOBSERVE_PUBLIC_URL` to the address people use; see [Run in production](production.md#put-tls-in-front).
2. Set `SLASHOBSERVE_AUTH_ALLOWED_EMAIL_DOMAINS` to your own domains, so only they can sign in; `*` allows every account, so do not use it on a shared provider; see [Sign-in with OIDC](../connect/oidc.md).
   New people are Editors by default, so allow in only people you would trust to create and change dashboards.
3. Set `SLASHOBSERVE_ADMIN_EMAILS` for the first Admins; see [Roles](../teams/roles.md#how-a-role-is-decided).
4. Set `SLASHOBSERVE_SECRET_KEY` before anyone adds a datasource in the app, and keep it with your database backups.
5. Keep `/metrics` on an internal network, or set `SLASHOBSERVE_METRICS=false`; see [Metrics](metrics.md).
6. Leave the outbound allowlist empty until a dashboard needs a domain, or fix it with `SLASHOBSERVE_OUTBOUND_DOMAINS`; see [Outbound access](outbound-access.md).
7. Give each tool its own service account with an expiry; see [Service-account keys](service-account-keys.md).
8. Set `SLASHOBSERVE_TRUSTED_PROXIES` to the narrowest range your proxy connects from, so the [audit trail](audit.md) records the real client address.

## Sign-in and roles

- **Sign-in.**
  Only accounts from your OIDC provider, and only in the allowed email domains, get a session.
  The server stores sessions and keys only as SHA-256 hashes, and they are random tokens.
- **Cookies.**
  Session cookies are `HttpOnly` and `SameSite=Lax`.
  The server refuses a cross-origin write made with a cookie.
  `SLASHOBSERVE_PUBLIC_URL` is the trusted origin.
- **Roles.**
  Every person and service account has an instance role (see [Roles](../teams/roles.md)), and the server checks it on every API route.
- **What only Admins can do.**
  An Admin can see everyone who has signed in, invite people, sign them out, deactivate or remove them, and manage service accounts and keys.
  An Admin can also open the Datasources page, manage Grafana connections and webhooks, read the Audit log, and change the outbound allowlist (unless `SLASHOBSERVE_OUTBOUND_DOMAINS` locks it).
  The activity log records what an Admin changes about people, keys, integrations and access, without any secret; see [Activity](../use/activity.md#admin-changes).
- **What every person can do.**
  Every person, Viewers included, can ask the agent for dashboards in their My Dashboards.
  That spends model tokens and lets the agent query every configured datasource for them.
- **An admin email needs a verified address.**
  An address in `SLASHOBSERVE_ADMIN_EMAILS` makes a person an Admin only when the provider reports it as verified.
  A provider that lets people set an unverified address cannot be used to claim an Admin's address.

## Dashboard access

- **Folders hide dashboards.**
  A restricted folder (see [Folders and access](../teams/folders.md)) is hidden from everyone who is not on its list, did not create it and is not an Admin.
  It is hidden in lists, search, tags, datasource dashboard counts, panel data, comments, events, import records and the agent.
  The server filters lists by the folders the caller can see, so a failed lookup shows nothing rather than everything.
  It re-checks access on each request, including at every ping of an event stream.
- **Keys and folders.**
  A key matches no folder entry, so only an Admin key reaches a restricted folder.
  A My Dashboards folder is hidden from everyone but its owner, Admins included.
- **Share links need sign-in.**
  The Share pane copies a dashboard or panel address with the time range and variable values, and grants nothing.
  Someone without access to the folder sees "Dashboard not found" after they sign in, the same page as for an id that does not exist.
- **Public links open one dashboard without sign-in.**
  Someone who manages a dashboard's folder can create a [public link](../use/public-links.md), the one way to show data to people with no session.
  The next list says what the server limits on that link.

What the server limits on a public link:

- The token is 32 random bytes, shown once and stored only as a SHA-256 hash with a 12 character prefix.
  Request logs replace it with `:token`.
- The routes under `/api/v1/public/` take no query text.
  They return the latest version's Spec with every query's text, the dashboard's tags and its links left out.
- They run only that dashboard's saved queries, under the proxy's limits, with 8 requests in flight per link.
- A free link accepts a variable value only when it is one of the options the server lists for it.
- Links are refused for dashboards in My Dashboards and for dashboards whose queries read the viewer's identity.
- Revoking deletes the link at once, and the server answers every public route with `Cache-Control: no-store`.
- A public link cannot be embedded in another site, because framing stays refused.

## Datasource credentials and queries

### Credentials stay on the server

Datasource credentials never reach the browser.

- The datasources list returns health only, with no address, user or secret.
- Only an Admin can read a datasource's address and login.
- No route returns a stored credential.
  The server seals passwords, tokens, API keys and header values with `SLASHOBSERVE_SECRET_KEY` and writes them, never reads them back.
- An address given in the app passes the outbound allowlist and address checks.
  Only an Admin can add, change or remove a datasource; see [Add datasources in the app](../connect/datasources-in-the-app.md).
- A query error shows the datasource's own error text, truncated and never naming an address, so people can fix a query from it.
  That holds for datasources in the file and in the app, so point a datasource only at a store whose error text everyone who can run a query on it may read.

The browser gets panel data from the server's query proxy.
The proxy runs only the queries stored in a saved dashboard, and it rejects any query text sent to it with `403 query_not_in_spec`.

### The three exceptions to saved queries

- **The draft preview.**
  A signed-in user who can comment on the dashboard can run an unsaved panel's queries before saving, under the same limits as the agent's tools.
  Service-account keys cannot.
  The observation panel editor's draft preview (`POST /api/v1/observations/{id}/blocks/preview`) does the same for a signed-in user who can edit or comment on the observation.
  A datasource only Admins may query answers there only in an Admins-only folder.
- **The build data routes.**
  `/api/v1/observe/{id}/build/panels/{panel_id}/data` and `/api/v1/observe/{id}/build/variables/{name}/values` run the queries of the dashboard an agent request is still assembling, which is not a saved version yet.
  Only the signed-in user who made the request, and a signed-in Admin, can call them, and for a revision only while the caller can still view the dashboard.
  Anyone else gets 404, and a service-account key gets `403 user_session_required`.
  They load the stored build on the server, run under the same limits as the agent's tools, and never cache or share results.
- **The explore query.**
  `POST /api/v1/datasources/{uid}/query`, behind the logs page and trace view, runs a query the caller writes against one datasource.
  It works for any signed-in role from Viewer and for service-account keys, under the same limits, and never caches or shares results.

People who can see a dashboard see how far its revision has come, never the build's panels.
The revisions list (`GET /api/v1/dashboards/{id}/revisions`) is visible to every viewer of the dashboard.
It carries a stopped build's progress counts, besides the continue note a person gave.
The build's title is shown only to the person who asked, and is null for everyone else.

### Every query is bounded

Whatever a prompt or a client asks for, each query has a timeout and limits on series, points and rows, applied on the server.
The server queries ClickHouse only as a read-only user.

## The agent and its builds

- **Only the requester saves a stopped build.**
  When an agent run fails, the server keeps its build for 7 days.
  The person or key that made the request can save it as one version with `POST /api/v1/observe/{id}/build/save`.
  Its role must be Editor or higher, and it must be able to edit the dashboard (or create dashboards, for a new one).
  Nobody else can, and the route answers them 404.
  No query runs when it is saved, because the agent checked each step as it made it.
- **Admins can read every chat, and only read it.**
  `GET /api/v1/observe?scope=all` lists everyone's requests for an Admin and answers anyone else 403.
  `GET /api/v1/observe/{id}` lets an Admin open any request with `read_only: true` and no allowed actions.
  `GET /api/v1/observe/{id}/build` and the build data routes let an Admin read another person's build the same way.
  Cancel, continue, retry, rename, pin, archive and build save stay with the requester.
  An investigation's finding in someone's personal folder stays hidden, as does a build on a dashboard in someone's personal folder.
- **Only the requester cancels or continues a request.**
  `POST /api/v1/observe/{id}/cancel` and `POST /api/v1/observe/{id}/continue` answer anyone else 403, or 404 for a new-dashboard request they cannot see.
  No role or folder level overrides that.
  Continue also needs the access that starting a run needs, and it answers 409 when someone saved a newer version of the dashboard after the build started.
  A run that does not end holds back other editors' revisions on its dashboard until its limits, its requester or a run time limit end it.
- **The built-in agent's token only works beside the server.**
  The agent process claims runs with a token that is new at each start.
  The server accepts it only from a loopback peer on a request with no forwarding header, and it lists only the four claim tools; see [The agent process](../concepts/agent.md#the-agent-process).
- **Draft questions are capped.**
  Each person or key may start 60 draft queries per clock hour, a count every server shares, on top of the limits on concurrent drafts.
  A refused one answers `429 too_many_requests` and calls no model.
- **The agent can only save dashboards.**
  Its only product is a dashboard definition that the server validates, and no code or markup written by the model is rendered.
  The server refuses a save from the agent or an MCP client when it writes HTML in a text panel (`html_not_allowed`) or adds or changes a canvas or field action (`action_not_allowed`), so only people and imports can create them.
  Through MCP, a key can also list datasources, run bounded queries and sample rows, and create and update dashboards.

## Outbound requests and the browser

- **Outbound requests are allowlisted.**
  The server fetches content from other sites only for domains on the allowlist, which starts empty.
  It uses only `https` to public addresses, and refuses private, loopback and metadata addresses even when a name resolves to one.
  This covers news feeds, images, map tiles, GeoJSON files and actions.
  The server checks redirects hop by hop, and sends no cookies or credentials.
  A canvas or field action also needs Edit on the dashboard and a confirmation in the browser, and the server never follows a redirect for it.
  See [Outbound access](outbound-access.md).
- **Two exceptions to public addresses.**
  A Grafana connection an Admin configures may use `http` and a private address, because the Grafana being migrated usually runs inside the same network.
  A datasource an Admin adds in the app may use `http` and a private address, and an IP address or a single-label host while the allowlist is unlocked, because the stores it queries usually run inside the same network.
- **Datasource addresses are re-checked.**
  The server checks the allowlist for these datasources every 10 seconds and at start.
  A datasource whose domain is removed, or whose IP address is no longer allowed once `SLASHOBSERVE_OUTBOUND_DOMAINS` locks the list, turns Unavailable.
  The server never connects to a link-local, cloud metadata or unspecified address, even when a name resolves to one.
- **Webhooks are an Admin's decision.**
  Only an Admin, or a key with the Admin role, can send workspace activity to an outside address, and no agent or MCP tool can.
  A webhook sends only what its owner could read when it sends.
  It signs each request with an HMAC and a timestamp, and keeps its secret sealed with `SLASHOBSERVE_SECRET_KEY`.
  It goes through the same outbound rules, so it cannot reach a private address or an unlisted domain.
  See [Webhooks](../use/webhooks.md).
- **The browser loads nothing from other sites.**
  Every response carries a strict `Content-Security-Policy`.
  Scripts, stylesheets, images, fonts and connections come only from the server's own origin.
  Images may also be `data:` or `blob:` URLs, and inline styles are allowed.
  Plugins, frames, media and `<base>` are refused.
  So even if a sanitizer were ever bypassed, injected script could not run or send data to another site.
  Images and GeoJSON from other sites reach the browser only through the server, which fetches them from allowlisted domains; see [Outbound access](outbound-access.md#images-and-geojson).
- **Links are limited to safe schemes.**
  A link in a dashboard, panel, override or canvas element whose address starts with a scheme other than `http`, `https` or `mailto` is refused when the Spec is saved (`unsafe_link`).
  That holds over the API, the CLI and MCP alike, and the browser drops such a link from a stored Spec as well.
- **HTML in text panels is sanitized.**
  HTML written by a person or imported from Grafana passes through one allowlist in the browser before it is drawn.
  The sanitizer removes scripts, styles, forms, embedded frames and event handler attributes.
  It removes links and images with any scheme but http, https and (for links) mailto.
  It also removes inline style declarations that load a resource or read a variable.
  Every image loads through the server, so the policy above still decides what the browser fetches.
- **Framing is refused.**
  Responses carry `X-Frame-Options: DENY` and `frame-ancestors 'none'` in the policy.
- **Other headers.**
  Responses also carry `X-Content-Type-Options: nosniff` and `Referrer-Policy: strict-origin-when-cross-origin`.
- **HTTPS only.**
  When `SLASHOBSERVE_PUBLIC_URL` is an `https` URL, responses carry `Strict-Transport-Security: max-age=31536000`, so browsers keep to HTTPS for a year.
  With an `http` URL the header is not sent.
- **`/metrics` needs no sign-in.**
  The series hold route templates, codes and model names and no ids, user names or query text.
  Anyone who can reach the route can read them, so keep it on an internal network or block it at the proxy, or set `SLASHOBSERVE_METRICS=false`; see [Metrics](metrics.md).

## Keys

- **Keep keys short-lived.**
  Prefer an expiry and one service account per tool or person.
  Revoke on the API keys page when a laptop or pipeline is retired.
- **Keys from a file are the operator's.**
  `SLASHOBSERVE_SERVICE_ACCOUNTS_FILE` lets the server create service accounts and their keys at boot from files on its own disk; see [Service-account keys](service-account-keys.md#declare-service-accounts-in-a-file).
  Anyone who can write those files or the server's environment can mint a key with the listed role, so protect them like the database URL.
- **The file stays the one place that decides.**
  The server stores only the key's hash and never logs it.
  It never takes over an account that was created in the web app, and it refuses role changes and revocation of a file key through the API.

## Connected agents (OAuth)

People connect their own MCP clients, such as Claude Code, VS Code and Cursor, to `<public url>/mcp`.
Each client then acts as the person who approved it.

- **SlashObserve is its own authorization server.**
  It uses the person's web session, which the identity provider created, for the approval page, and issues its own tokens.
  It never hands the identity provider's tokens to a client.
- **Clients register themselves.**
  Dynamic client registration is open, and a registered redirect URI must be `https` or `http` on `localhost`, `127.0.0.1` or `[::1]`, with no fragment.
  A redirect must match a registered one exactly, except that a loopback address may use any port.
  An unknown client or an unregistered redirect never receives a redirect.
- **PKCE with S256 is required.**
  Codes last 60 seconds and work once, and a code used twice disconnects the connection it produced.
  Clients are public, so no client secret exists.
- **Codes and tokens are stored only as SHA-256 hashes.**
  The server never logs a code, a token, the `state`, a redirect URI or a client name.
- **Access tokens last 1 hour and refresh tokens 30 days, replaced on every use.**
  Using an already replaced refresh token more than 10 seconds later disconnects the connection.
  Inside 10 seconds it only fails, so a client with two processes does not lose its connection.
  A connection ends at 90 days whatever its use, and the person then approves again, which passes through the identity provider's sign-in.
- **`/mcp` takes only a bearer token.**
  A service-account key or an OAuth access token works there, and a session cookie is ignored.
  A request with an `Origin` header other than `SLASHOBSERVE_PUBLIC_URL` is refused with `403 forbidden_origin`.
- **Tokens are bound to `<public url>/mcp` and accepted nowhere else.**
  Changing `SLASHOBSERVE_PUBLIC_URL` therefore disconnects every connected agent.
- **A connected agent has exactly the person's role and folder access, read again on every request.**
  Deleting a person disconnects their agents.
- **The client's name is a self-chosen label.**
  The server freezes it when the client registers, shows it on the approval page with the host the browser returns to, and never uses it in an access decision.
  A client can claim any name, and can only mislabel writes made as the person who approved it.
- **Abuse limits.**
  Registration, authorization, token and revocation requests are rate limited.
  The database caps unused clients at 1000, pending approvals at 10000 and connected agents per person at 20.
  The registration, authorization, token and revocation rate limits are totals divided among the live servers; the per-client token limit is per server.

### Cut someone off

To cut someone off:

1. Remove them at the identity provider.
2. Deactivate them on the People page; see [People](../teams/people.md).
   This ends their sessions, disconnects their agents and refuses their next sign-in.

**Sign out everywhere** only ends sessions and connections, a connection approved but not yet completed included, and the person can sign in again.
