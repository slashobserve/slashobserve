---
title: Add datasources in the app
description: An Admin adds, tests, edits and removes datasources on the Datasources page, with the CLI or over HTTP, and decides who can query them, without editing a file or restarting the server.
---

A datasource is a connection to a system that holds your data, such as Prometheus or Loki.
An Admin can add one in the web app, with no file to edit and no restart.
The datasource answers queries as soon as it is saved, and other servers in a fleet pick it up within 10 seconds.

## Add a datasource

1. Sign in as an Admin and choose **Datasources** in the sidebar.
2. Choose **Add datasource**.
3. Fill in the form:
   - **Type**, one of Prometheus, Loki, Tempo, Jaeger, "ClickHouse, read only", Elasticsearch or OpenSearch.
   - **Id**, which dashboards use to refer to the datasource and which cannot change later.
     It starts with a letter or digit and has at most 40 letters, digits, `_` and `-`.
   - **Name**, shown in lists, at most 100 characters.
     It defaults to the id.
   - **Address**, as [Addresses](#addresses) describes.
   - Any **Credentials** the system needs.
4. Choose **Test connection**.
   A working address shows "Connected. Save to keep these settings."
5. Choose **Add datasource**.
   The new row appears with its status.
6. Run a first query with the link on the row: **Explore metrics**, **Explore logs** or **Search traces**, whichever fits the type.

The CLI does the same.
It reads a credential from a file or from standard input, never from an argument:

```sh
printf '%s' "$TOKEN" | observe datasources add prom --type prometheus \
  --address https://prometheus.example.com --bearer-token-file -
observe datasources recheck prom
observe metrics query 'up' --datasource prom --instant
```

Over HTTP, an Admin session or key posts the definition and its credentials:

```sh
curl -fsS -X POST -H "Authorization: Bearer $OBSERVE_TOKEN" \
  -H "Content-Type: application/json" "$OBSERVE_SERVER/api/v1/datasources" \
  -d '{"uid":"prom","type":"prometheus","address":"https://prometheus.example.com","secrets":{"bearer_token":"..."}}'
```

The answer is `201` with the saved definition, `"source": "app"` and `secrets_set`, the names of the credentials that are set.

## Read the Datasources page

The Datasources page is in the sidebar for Admins only.
Each row shows these columns:

- The name and id.
- The type.
- Where the datasource comes from: "From file", "Added in the app" or "Built in".
- An **Only Admins** badge when only Admins can query it.
- Its status.
- How many dashboards use it.
- When it was last checked.

The page refreshes every 30 seconds.
**Recheck** on a row runs a fresh health check at once.
A row also links to the page that reads it: **Explore logs**, **Search traces** or **Explore metrics**.

**Edit** and **Remove** appear only on datasources added in the app.
A datasource from the file and the built-in audit trail show only **Recheck** and the links.

The app reloads its datasource list every 15 seconds while any datasource is unhealthy.
The sidebar and the Logs, Traces and Metrics source lists read that list, so a source that was still starting when the page loaded shows as available once it answers.

## Edit a datasource

1. On the Datasources page, choose **Edit** on the row.
2. Change the settings and choose **Test connection**.
3. Choose **Save**.

The id and the type are fixed once a datasource is saved.
A change to either is refused with `422 invalid_datasource`, so add a new datasource instead.
Everything else can change, including the name.

The form shows the common settings first.
Under **Show advanced settings** it shows these:

- The query timeout.
- The limits on series, points, rows and concurrent queries.
- The Grafana datasource ids.
- For Elasticsearch and OpenSearch, the query language and the index fields that hold the message, level, service, trace id and span id.

These are the same settings as in the [datasources file](../reference/datasources-file.md).

From the CLI:

```sh
observe datasources get prom
observe datasources edit prom --address https://prom2.example.com --clear header:X-Org
```

`edit` changes only the settings you pass.
Over HTTP, `PATCH /api/v1/datasources/{uid}` takes the same fields, and a field you leave out keeps its value.

## Test a connection before saving

**Test connection** runs a health check against the values in the form, before anything is saved.
When you edit, a credential you leave empty is taken from the saved datasource, so the test reflects what saving would do.
A failed test shows the same fixed message and cause as the Datasources page.
The check times out after 10 seconds.

A test never changes the outbound allowlist.
A domain that is not listed can be tested only while the allowlist is unlocked, and it is added when you save.

From the CLI, `observe datasources test prom --saved --address https://prom2.example.com` starts from the saved datasource and changes only what the flags name.
Without `--saved` it tests a draft you describe in full.
Over HTTP, it is `POST /api/v1/datasources/test`, with `?base=<uid>` to start from a saved datasource.
It answers `{"healthy": true}`, or `healthy: false` with a `message` and a `detail`.
A test is recorded in the audit trail with the type, the host and the result.

## Remove a datasource

1. On the Datasources page, choose **Remove** on the row and confirm.
2. If a dashboard or saved search still uses the datasource, the removal is refused.
   Change those to another datasource first, then remove it.

From the CLI, `observe datasources remove prom`.
Over HTTP, `DELETE /api/v1/datasources/{uid}`.

The refusal is `409 datasource_in_use`.
It lists the users and counts as `hidden` the dashboards in folders you cannot see.
Removing a datasource deletes its stored credentials for good, so add it again with the credentials to restore it.

## Make a datasource Admins only

By default everyone from Viewer up can query a datasource added in the app.
To limit it to Admins:

1. Edit the datasource and choose **Only Admins** under **Audience**.
2. Move any dashboard, observation or saved search that reads it into a folder only Admins can read.
   The save is refused until you do.
3. Save.

From the CLI, `--audience admins` on `observe datasources add` or `edit`.
Over HTTP, `"audience": "admins"` on `POST /api/v1/datasources` or `PATCH /api/v1/datasources/{uid}`.
`"audience": "everyone"` opens it again, and a change that leaves the audience out keeps it.

[Who can query it](#who-can-query-it) has the full rules, including what the server refuses and why.

## Fix a datasource that shows Unavailable

A stored datasource that cannot load still appears, as Unavailable with the reason.
Dashboards that use it fail with a clear message.
These are the causes:

- The secret key is missing or changed, or the sealed credentials cannot be read.
  Set `SLASHOBSERVE_SECRET_KEY` back to the key that sealed them, or edit the datasource and enter the credentials again.
- A stored row no longer validates.
  Edit the datasource.
- The address's domain is no longer allowed.
  The status is "The address is not allowed by the outbound allowlist".
  Allow the domain again or change the address.

After the fix, choose **Recheck**.

## Reference

### Where a datasource comes from

Entries in the [datasources file](../reference/datasources-file.md) show as "From file".
The server reads them once at startup, and they are read-only in the app.
Datasources added in the app show as "Added in the app".
When a file entry and an app datasource share a uid, the file entry wins and the app one is refused at save time with `409 uid_taken`.

Every server also has one built-in datasource, the audit trail.
It shows as "Built in" with the **Only Admins** badge, with the uid `slashobserve-audit` and `source` `builtin` on `GET /api/v1/datasources`.
Only Admins see it, and nobody can edit or remove it.
`PATCH`, `DELETE` and `GET /api/v1/datasources/slashobserve-audit/config` answer `409 datasource_read_only`.
A file entry with that uid stops the server at startup, and an app datasource with it is not loaded.
[Audit queries](../reference/audit-queries.md#the-audit-datasource) describes what it answers.

### Who can query it

A datasource added in the app is open to everyone from Viewer up by default.
With the `admins` audience, only Admins and Admin keys can list, query or health-check it.
Every other role, key and public link gets the same answer for it as for a datasource that does not exist.

The audience is returned with the rest of the definition on `GET /api/v1/datasources/{uid}/config`, which only Admins can call.
The Datasources page shows an **Only Admins** badge on a datasource with the `admins` audience.
For an Admin, each row of the MCP tool `list_datasources` also carries `audience`.
Other callers never see an `admins` datasource, so their rows leave it out.

#### Where its content must live

A dashboard, proposal, observation or saved search that reads an Admins-only datasource lives only in a [folder only Admins can read](../teams/folders.md).
That counts these:

- A panel or query that names the datasource.
- A datasource variable whose default or fixed options name it.
- A snapshot or RCA panel that read it when it was frozen, the datasource a variable picked included.

Saving, importing, applying or proposing such content anywhere else is refused with `409 restricted_content`.
The answer names the datasource and says to use a folder only Admins can read.
Moving the content out to another kind of folder is refused the same way.
Every version of a dashboard and every revision of its proposals count for a move, because the history stays readable.

The same answer reaches the web app, the agent, MCP tools, the CLI, files applied with `observe dashboards apply`, the provisioning directory and the Grafana import.

Content in any other folder is read as everyone reads it, by Admins too.
A panel, variable, snapshot or RCA there never queries an Admins-only datasource, and a datasource variable with no default picks among the datasources everyone can query.
Only content in a folder only Admins can read, the explore query, panel drafts and agent builds read with the Admin's own role.

#### Making a datasource Admins only

Changing the audience from `everyone` to `admins` is refused with `409 restricted_content` while such content sits in a folder other people can read.
The answer has these fields:

- `items` lists up to 20 of them outside people's My Dashboards, with their kind, id, title and folder, so an Admin can move them first.
- `unlisted` and the message say how many more are not listed.
- `hidden` counts the ones in people's My Dashboards, which only their owners can move or remove and an Admin cannot see.

`observe datasources edit` prints the same list.

Each server checks every app datasource that is already `admins` when it starts, for content left outside such folders from before this rule.
It logs one warning per datasource, `admins only datasource read outside admins only folders`, and starts anyway.
The warning has the datasource uid (`datasource_uid`) and the number of items in shared folders (`shared_items`) and in people's My Dashboards (`personal_items`), never their titles.

### Credentials

A password, bearer token, API key or request header value is stored encrypted with `SLASHOBSERVE_SECRET_KEY` and is never shown again.
The edit form shows only whether each one is set, and a field left empty keeps what is stored.
**Clear** on a credential removes it, and **Keep** undoes the clear before you save.
Every header value counts as a credential, so the app has no plain custom headers.

Without `SLASHOBSERVE_SECRET_KEY` the server refuses to save credentials with `409 secret_key_missing`.
A datasource with no credentials still saves.

Over HTTP, `secrets` sets credentials and `clear_secrets` removes them.
`clear_secrets` takes `password`, `bearer_token`, `api_key` or `header:<name>`.
The CLI takes `--clear` with the same names.
Other names are refused with `422 invalid_datasource`.

These limits apply to credentials, and a value over a limit or with a control character is refused with `422 invalid_datasource`:

| Limit | Value |
| --- | --- |
| Request headers per datasource | 20 |
| One credential or header value | 4096 bytes |
| All credentials together | 60000 bytes |

### Addresses

The address must pass the outbound allowlist, like a Grafana connection's.
It must be an `http` or `https` URL with a host and no user info, or `host:port` for ClickHouse.
The server refuses a link-local, unspecified or multicast IP address.
It never connects to a link-local, cloud metadata or unspecified address, even when a name resolves to one.

#### Details of the allowlist rules

- A domain is added to the outbound allowlist when you save, if `SLASHOBSERVE_OUTBOUND_DOMAINS` does not lock the list.
  When it locks the list, a domain that is not on it is refused with `403 outbound_domain_not_allowed`.
- A bare IP address or a single-label host cannot be listed.
  It is accepted only while the allowlist is unlocked.
- Removing a datasource, or changing its address, takes its old domain off the allowlist when nothing else uses it and the list is unlocked.
- The allowlist holds at most 100 domains, and adding one more is refused with `422 too_many_domains`.
- The server checks the allowlist again every 10 seconds and when it starts.
  A datasource whose domain was removed shows Unavailable until the domain is allowed again or the address is changed.
  The same happens when `SLASHOBSERVE_OUTBOUND_DOMAINS` now locks the list and the address is an IP address or a single-label host.
- The server holds at most 100 datasources added in the app, and the 101st is refused with `422 too_many_datasources`.

### CLI

`observe datasources` has these commands for datasources added in the app.
They need an Admin key, except `recheck`, which needs a Viewer key.

| Command | What it does |
| --- | --- |
| `get <uid>` | Prints the saved definition. |
| `add <uid> --type <type> --address <address>` | Adds a datasource. |
| `edit <uid>` | Changes the settings you pass. |
| `test <uid> [--saved]` | Checks a draft and saves nothing. |
| `recheck <uid>` | Runs a fresh health check. Viewer or higher. |
| `remove <uid>` | Removes the datasource. |

`test` and `recheck` exit 1 when the datasource is unhealthy.

#### Details of the flags

- `--name`, `--audience` (`everyone` or `admins`) and `--address` apply to every type.
- `--username` is the basic-auth user, or the ClickHouse user.
- `--password-file`, `--bearer-token-file` and `--api-key-file` read a credential from a file, or from standard input with `-`.
  `--header NAME=FILE` sends header `NAME` with the value read from `FILE`.
  Only one flag can read standard input.
- `--clear` takes `password`, `bearer_token`, `api_key` or `header:<name>`, and repeats for several.
- `--database` and `--tls` apply to ClickHouse.
- `--tenant-id` is the Loki tenant, and `--scrape-interval` is the Prometheus scrape interval.
- `--index`, `--time-field`, `--query-language` (`kql` or `lucene`) and the `--field-message`, `--field-level`, `--field-service`, `--field-trace-id` and `--field-span-id` settings apply to Elasticsearch and OpenSearch.
- `--timeout`, `--max-series`, `--max-points`, `--max-rows` and `--max-concurrent-queries` set the query limits.
- `--grafana-uid` names a Grafana datasource uid this one stands for on import, and repeats for several.

`observe datasources add --help` lists them all.

An agent over MCP can list datasources and recheck them, but cannot add, change or remove one.
These routes are for Admins over HTTP and the CLI only.

### Audit log

These events go to the [Audit log](../use/activity.md), with the name, the address's host and port and what changed:

- `datasource.created`, `datasource.updated` and `datasource.deleted`.
- `datasource.audience_changed`, written on its own for a change of the audience, with the audience before and after.
  In the audit trail it is `datasource.audience_change`.

A credential and the address path are never recorded, only that credentials were replaced.
Datasources from the datasources file are not recorded.
