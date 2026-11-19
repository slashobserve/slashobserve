---
title: Add datasources in the app
description: An Admin adds, edits, tests and removes datasources on the Datasources page or with the CLI, and how they differ from entries in the datasources file.
---

An Admin can add a datasource without editing a file or restarting the server.
Open the Datasources page, choose Add datasource, pick the type, fill in the address and any credentials, test the connection, then save.
The datasource answers queries as soon as it is saved, and other servers in a fleet pick it up within 10 seconds.

## Two sources

Entries in the [datasources file](../reference/datasources-file.md) show as "from file".
They are read once at startup and are read-only in the app.
Datasources added in the app show as "added in the app".
When a file entry and an app datasource share a uid, the file entry wins and the app one is refused at save time.

## Credentials are write-only

A password, bearer token, API key or request header value is stored encrypted with `SLASHOBSERVE_SECRET_KEY` and is never shown again.
The edit form shows only whether each one is set, and a field left empty keeps what is stored.
Clear removes a credential.
Without `SLASHOBSERVE_SECRET_KEY` the server refuses to save credentials with `409 secret_key_missing`; a datasource with no credentials still saves.
Every header value counts as a credential, so the app has no plain custom headers.

## Addresses

The address is checked against the outbound allowlist like a Grafana connection's.
It must be `http` or `https` with a host and no user info, or `host:port` for ClickHouse.
A domain is added to the outbound allowlist when the allowlist is not locked, and refused with `403 outbound_domain_not_allowed` when `SLASHOBSERVE_OUTBOUND_DOMAINS` locks it.
A bare IP address or a single-label host cannot be listed, so it is accepted only while the allowlist is unlocked; link-local, unspecified and multicast addresses are always refused.
The server checks the allowlist again every 10 seconds and when it starts.
A datasource whose domain was removed, or whose IP address or single-label host is no longer allowed because `SLASHOBSERVE_OUTBOUND_DOMAINS` now locks the list, shows Unavailable with "The address is not allowed by the outbound allowlist" until the domain is allowed again or the address is changed.
The server never connects to a link-local, cloud metadata or unspecified address, even when a name resolves to one.
The server holds at most 100 datasources added in the app.

## Test the connection

Test connection runs a health check against the values in the form, before anything is saved.
When editing, a credential you leave empty is taken from the saved datasource, so the test reflects what saving would do.
A failed test shows the same fixed message and cause as the Datasources page.
A test never changes the outbound allowlist: a domain that is not listed can be tested only while the allowlist is unlocked, and it is added when you save.

## Remove a datasource

A datasource that saved dashboards or saved searches use cannot be removed.
The refusal is `409 datasource_in_use` and lists the users, so you can change them to another datasource first.
Dashboards in folders you cannot see are counted but not named.

## Audit log

Adding, changing and deleting a datasource is recorded as `datasource.created`, `datasource.updated` and `datasource.deleted` in the [Audit log](../use/activity.md), with the name, the address's host and port and what changed.
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
bin/observe datasources remove prom
```

Credentials are read from a file or from standard input, never from an argument, and only one flag can read standard input.
`test` exits 1 when the datasource is unhealthy.
These routes are for Admins over HTTP and the CLI only; an agent over MCP can list datasources and recheck them but cannot add, change or remove one.
