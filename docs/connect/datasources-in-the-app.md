---
title: Add datasources in the app
description: An Admin adds, edits, tests and removes datasources on the Datasources page or with the CLI, and how they differ from entries in the datasources file.
---

An Admin can add a datasource without editing a file or restarting the server.
Open the Datasources page, choose Add datasource, pick the type, fill in the address and any credentials, test the connection, then save.
The datasource answers queries as soon as it is saved, and other servers in a fleet pick it up within 10 seconds.

The Datasources page is in the sidebar for Admins only.
Each row shows the name and id, the type, whether the datasource is "From file" or "Added in the app", its status, how many dashboards use it and when it was last checked.
The page refreshes itself every 30 seconds, and **Recheck** on a row runs a fresh health check at once.
A row also links to the page that reads it: **Explore logs**, **Search traces** or **Explore metrics**.
**Edit** and **Remove** appear only on datasources added in the app.
The app also reloads its datasource list every 15 seconds while any datasource is unhealthy, which the sidebar and the Logs, Traces and Metrics source lists read, so a source that was still starting when the page loaded shows as available once it answers.

## Two sources

Entries in the [datasources file](../reference/datasources-file.md) show as "From file".
They are read once at startup and are read-only in the app.
Datasources added in the app show as "Added in the app".
When a file entry and an app datasource share a uid, the file entry wins and the app one is refused at save time with `409 uid_taken`.

## Who can query it

A datasource added in the app is open to everyone from Viewer up by default.
Set its audience to `admins` with `"audience": "admins"` on `POST /api/v1/datasources` or `PATCH /api/v1/datasources/{uid}`, or `--audience admins` on `observe datasources add` and `edit`, and only Admins and Admin keys can then list, query or health-check it.
Every other role, key and public link gets the same answer for it as for a datasource that does not exist.
A change that leaves the audience out keeps it, and `"audience": "everyone"` opens the datasource again.
The audience is returned with the rest of the definition on `GET /api/v1/datasources/{uid}/config`, which only Admins can call, and the Datasources page does not show or change it yet.
For an Admin, each row of the MCP tool `list_datasources` also carries `audience`; other callers never see an `admins` datasource, so their rows leave it out.

A dashboard, proposal, observation or saved search that reads an Admins-only datasource lives only in a [folder only Admins can read](../teams/folders.md).
That counts a panel or query that names it, a datasource variable whose default or fixed options name it, and a snapshot or RCA panel that read it when it was frozen, the datasource a variable picked included.
Saving, importing, applying or proposing one anywhere else is refused with `409 restricted_content`, which names the datasource and says to use a folder only Admins can read, and so is moving it out to another kind of folder.
Every version of a dashboard and every revision of its proposals count for a move, because the history stays readable.
The same answer reaches the web app, the agent, MCP tools, the CLI, files applied with `observe dashboards apply`, the provisioning directory and the Grafana import.

Content in any other folder is read as everyone reads it, by Admins too: a panel, variable, snapshot or RCA there never queries an Admins-only datasource, and a datasource variable with no default picks among the datasources everyone can query.
Only content in a folder only Admins can read, the explore query, panel drafts and agent builds read with the Admin's own role.

Changing the audience from `everyone` to `admins` is refused with `409 restricted_content` while such content sits in a folder other people can read.
`items` lists up to 20 of them outside people's My Dashboards with their kind, id, title and folder, so an Admin can move them first, the message says how many more are not listed, and `hidden` counts the ones in people's My Dashboards, which only their owners can move.
`observe datasources edit` prints the same list.

## Credentials are write-only

A password, bearer token, API key or request header value is stored encrypted with `SLASHOBSERVE_SECRET_KEY` and is never shown again.
The edit form shows only whether each one is set, and a field left empty keeps what is stored.
Clear removes a credential.
Without `SLASHOBSERVE_SECRET_KEY` the server refuses to save credentials with `409 secret_key_missing`; a datasource with no credentials still saves.
Every header value counts as a credential, so the app has no plain custom headers.

## Edit a datasource

The id and the type are fixed once a datasource is saved, and a change to either is refused with `422 invalid_datasource`: add a new datasource instead.
Everything else can change, including the name.
The form shows the common settings first and, under **Show advanced settings**, the query timeout, the limits on series, points, rows and concurrent queries, the Grafana datasource ids, and for Elasticsearch and OpenSearch the query language and the index fields that hold the message, level, service, trace id and span id.
These are the same settings as in the [datasources file](../reference/datasources-file.md).

## Addresses

The address is checked against the outbound allowlist like a Grafana connection's.
It must be `http` or `https` with a host and no user info, or `host:port` for ClickHouse.
A domain is added to the outbound allowlist when the allowlist is not locked, and refused with `403 outbound_domain_not_allowed` when `SLASHOBSERVE_OUTBOUND_DOMAINS` locks it.
A bare IP address or a single-label host cannot be listed, so it is accepted only while the allowlist is unlocked; link-local, unspecified and multicast addresses are always refused.
The server checks the allowlist again every 10 seconds and when it starts.
A datasource whose domain was removed, or whose IP address or single-label host is no longer allowed because `SLASHOBSERVE_OUTBOUND_DOMAINS` now locks the list, shows Unavailable with "The address is not allowed by the outbound allowlist" until the domain is allowed again or the address is changed.
The server never connects to a link-local, cloud metadata or unspecified address, even when a name resolves to one.
The server holds at most 100 datasources added in the app, and the 101st is refused with `422 too_many_datasources`.

## Test the connection

Test connection runs a health check against the values in the form, before anything is saved.
When editing, a credential you leave empty is taken from the saved datasource, so the test reflects what saving would do.
A failed test shows the same fixed message and cause as the Datasources page.
A test never changes the outbound allowlist: a domain that is not listed can be tested only while the allowlist is unlocked, and it is added when you save.

## Remove a datasource

A datasource that saved dashboards or saved searches use cannot be removed.
The refusal is `409 datasource_in_use` and lists the users, so you can change them to another datasource first.
Dashboards in folders you cannot see are counted but not named.
Removing one deletes its stored credentials for good, so add it again with the credentials to restore it.

## Audit log

Adding, changing and deleting a datasource is recorded as `datasource.created`, `datasource.updated` and `datasource.deleted` in the [Audit log](../use/activity.md), with the name, the address's host and port and what changed.
A change of the audience is recorded on its own as `datasource.audience_changed`, with the audience before and after, and in the audit trail as `datasource.audience_change`.
A credential and the address path are never recorded, only that credentials were replaced.
Datasources from the datasources file are not recorded.

## Stored datasources that cannot load

If the secret key is missing or changed, or a stored row no longer validates, the datasource still appears, as Unavailable with the reason, and dashboards that use it fail with a clear message.
Fix the cause and use Recheck, or edit the datasource.

## CLI

```sh
printf '%s' "$TOKEN" | bin/observe datasources add prom --type prometheus \
  --address https://prometheus.example.com --bearer-token-file -
bin/observe datasources get prom
bin/observe datasources edit prom --address https://prom2.example.com --clear header:X-Org
bin/observe datasources test prom --saved
bin/observe datasources recheck prom
bin/observe datasources remove prom
```

Credentials are read from a file or from standard input, never from an argument, and only one flag can read standard input.
`--clear` takes `password`, `bearer_token`, `api_key` or `header:<name>`.
Other flags set the database and `--tls` for ClickHouse, the Loki tenant, the Prometheus scrape interval, the `--index`, `--time-field`, `--query-language` and `--field-*` settings of Elasticsearch and OpenSearch, the query limits, and `--grafana-uid`; `observe datasources add --help` lists them all.
`test` and `recheck` exit 1 when the datasource is unhealthy.
These routes are for Admins over HTTP and the CLI only; an agent over MCP can list datasources and recheck them but cannot add, change or remove one.
