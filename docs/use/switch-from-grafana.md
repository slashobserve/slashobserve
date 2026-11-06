---
title: Switch from Grafana
description: Move a Grafana team over in order, from connecting a Grafana and importing every dashboard to repairing, checking health, sharing, keeping dashboards in Git and opening old links.
---

This page walks a team through the move from Grafana in the order that works: each step leaves the dashboards in a better state for the next one.

1. [Connect](#connect) the server to a running Grafana.
2. [Import](#import) every dashboard with its folders in one run.
3. [Repair](#repair) the panels whose queries did not verify.
4. [Check health](#check-health) to find what still returns no data, and what nobody uses.
5. [Share](#share-with-public-links-and-tv-mode) dashboards with people outside the team and on wall screens.
6. [Keep dashboards in Git](#keep-dashboards-in-git) once they are right.
7. [Open old links](#open-old-links) to Grafana dashboards at their new address.

[What maps, what changes and what waits](#what-maps-what-changes-and-what-waits) compares the two in one table, and [The API](#the-api) does every step without the web app.
Screenshots of each step come with a later release.

## Before you start

- Set `SLASHOBSERVE_SECRET_KEY` (see [Configuration](../reference/configuration.md#stored-secrets)).
  The server encrypts each token with a key derived from it, and adding a connection without it fails with `409 secret_key_missing`.
- In Grafana, create a service account with the Viewer role on the folders you want to bring over, and a token for it.
  The server only reads from Grafana: `/api/health`, `/api/folders`, `/api/search`, `/api/datasources` and `/api/dashboards/uid/<uid>`.

## Connect

An Admin stores a Grafana URL and a service-account token once and lets the server import every dashboard from that Grafana, instead of importing one file at a time.
Each dashboard goes through the same importer as [Import Grafana dashboards](import-grafana.md), with the same datasource matching, the same report and one import record per dashboard.


Open Settings and choose the Grafana tab; only Admins see it.
Give the connection a name, the Grafana URL and the service account token, and choose Add connection.
The token is stored encrypted and is never shown again, not in the web app and not in the API.
Choose Test on the connection to check it: the outcome names how many top-level folders and dashboards the server found, for example `Connected: 12 top-level folders, 340 dashboards.`
A Grafana that cannot be reached or refuses the token shows a fixed message such as `Grafana refused the token (status 401).`
To change the token, delete the connection and add it again; imported dashboards stay when a connection is deleted.

### Who can connect, and why private hosts are allowed

Only Admins can add, change, test, list or delete a connection, or start an import from one; a key needs the Admin role too.
The token is never returned by the API, never logged, and only ever sent to the host of the connection's URL.
A new URL needs the token again in the same request, so a stored token never follows a URL change to another host.

Other outbound fetches, such as news feeds and images, reach only `https` on port 443 and refuse private addresses (see [Outbound access](../operate/outbound-access.md)).
A Grafana connection is the one exception: its URL may be `http` or `https` on any port, and its host may resolve to a private or loopback address, because Grafana usually runs inside the same network as SlashObserve.
This is safe to allow only because the person adding the URL is an Admin, who already controls the server's datasources and settings.
The host must still be a domain name, not an IP address, and it must be on the outbound allowlist: adding a connection allows its domain when the list is managed in Settings, and is refused with `403 outbound_domain_not_allowed` when `SLASHOBSERVE_OUTBOUND_DOMAINS` locks the list and does not name it.
Every request to Grafana keeps the outbound timeout and an 8 MiB size cap, and a redirect is never followed.

## Import

Choose Import on a connection.
Pick All folders, or Choose folders and tick the ones you want; a folder brings its subfolders with it, and General stands for the dashboards outside any folder.
Then check the datasource matches.
Each Grafana datasource of a type SlashObserve queries (Prometheus, ClickHouse, Loki or Tempo) is listed with a choice of your datasources of the same type, prefilled by the importer's automatic rules, so a datasource of the wrong type is never offered.
A datasource with no match is flagged: dashboards that use it fail until you pick one.
A datasource whose type has no configured datasource here is listed without a choice, and its panels import as placeholders that say which type is missing.
Datasources of other types are counted in one line, and their panels import as placeholders.

Choose Start import to begin.
The page shows how many dashboards are done out of the total, and how many failed or were skipped, while the import runs, and it keeps showing the run after a reload.
When it ends, the results are grouped:

- Failed: the dashboards that could not be imported, each with its Grafana uid and the reason.
- Need repair: the dashboards that were imported with unverified or placeholder panels, with their counts and a link to each.
- Imported: the dashboards that imported cleanly.
- Skipped: the dashboards already imported at the same Grafana version.

The server fetches at most 4 dashboards from Grafana at a time and saves them one after another, and a dashboard that fails never stops the run.
A connection runs one import at a time.
The server keeps the last 20 runs in memory until it restarts, and the connection's last import keeps the summary of the last finished one.
A restart stops a running import; start it again and it continues by skipping what it already imported.

## Repair

The repair queue is the **Imported, not verified** filter on the [Health page](dashboard-health.md), at `/health?kind=imported`; it lists every dashboard whose latest version still has unverified panels or variables or placeholder panels, in the folders you can see.
The old address `/import/repair` opens it too.
Editors and Admins can open it from Import from Grafana, and from the Need repair group of an import's results.
Each row shows the dashboard's folder, its unverified panels and variables, its placeholders, when it was imported, and its latest repair.
A dashboard leaves the list once nothing in it fails.

Repair with the agent queues one revision on that dashboard with this fixed prompt:

```text
Repair every panel and variable listed as unverified so its query runs and returns data; keep every other panel unchanged and never delete an unverified panel.
```

Repair all queues one such revision for every listed dashboard in the chosen folder.
A dashboard with a revision already queued or running is skipped, and so is one with only placeholders, because the agent never edits a placeholder; replace those by hand.
Each dashboard still runs one revision at a time with at most 5 waiting, and each person can have 20 repairs queued or running, so on a large import run Repair all again when the first ones finish.
A repair that ran but left failures shows as repaired and still failing, and you can send it again.

From an MCP client, read the dashboard with `get_dashboard`, whose `unverified` list names the failing panels and variables, and fix those panels with `update_panels`.

## Check health

Repair fixes what failed at import time, but a query that runs can still return nothing, and a move is a good time to drop dashboards nobody opens.
Every dashboard is checked about once a day, and the **Health** page lists each one with a panel that fails or returns no data over the last 7 days, a variable that resolves to nothing, a duplicate title in its folder, or no opens in 90 days.
After a large import, an Admin can choose **Run now** instead of waiting for the hourly run, and an Editor can choose **Repair** on a row to queue the agent on its failing panels.
Delete or merge the duplicates and the unopened dashboards before the next step, so nobody shares or commits them.
See [Dashboard health](dashboard-health.md) for the rules and the API.

## Share with public links and TV mode

Grafana's public dashboards and kiosk mode have their counterparts here.

- A [public link](public-links.md) lets someone without an account, such as a vendor or a status screen, open one dashboard at `/p/<token>`.
  A person who manages the dashboard's folder creates it in the dashboard's **Share** pane, chooses whether viewers may change the variables, can set an expiry, and can revoke it at any time.
  A public view runs only that dashboard's saved queries and never shows query text, annotations, comments or the agent, and a dashboard in My Dashboards or one whose queries read the viewer cannot get a link.
- TV mode shows a dashboard on a wall screen without the top row, the variables bar, the links or the activity card, and Esc leaves it.
  Add `kiosk=true` to the dashboard's address, `cycle=rows` to scroll through its rows or `cycle=folder` to move through its folder's dashboards, and `interval=<seconds>` to set the pace (60 by default).
  Cycling through a folder takes the place of a Grafana playlist; see [the web app](web.md).

## Keep dashboards in Git

Once a folder is right, export it and commit the files, so a change goes through review like code:

```sh
observe dashboards export Payments --dir dashboards
observe dashboards apply --dir dashboards
```

Apply saves each file into the dashboard it came from, adopts a dashboard with the same title in the folder, and never deletes one, so a CI job can run it on every merge.
It takes the place of Grafana's provisioning files when you would rather not mount a directory on the server; the server's own [provisioning directory](../operate/provisioning.md) reads the same layout.
See [Dashboards as code](dashboards-as-code.md) for the layout, the matching rule and a GitHub Actions workflow.

## Open old links

A Grafana dashboard link keeps working with its host changed to SlashObserve's: `/d/<grafana uid>/<slug>` redirects with `302 Found` to `/d/<id>` of the imported dashboard, keeping the slug and the query string, as [Old addresses](web.md#old-addresses) describes.
`GET /api/v1/grafana/resolve/{uid}` answers the `dashboard_id` and the web `url` of the dashboard imported from a Grafana uid.
Any signed-in person can call it, and a uid whose dashboard is in a folder they cannot see answers `404`, like a uid nobody imported.

## What maps, what changes and what waits

| In Grafana | Here | |
| --- | --- | --- |
| Folders, nested folders | Folders keep their Grafana titles, and a nested folder becomes one folder titled `Parent / Child`. A missing folder is created, and an existing one with the same title, in any case, is reused. | Maps |
| Dashboards outside any folder | General, not your own My Dashboards. | Maps |
| Tags and dashboard links | Imported with the dashboard, as they are for a single import. | Maps |
| Dashboard uid and version | Kept on the import record as `grafana_uid` and the Grafana `version`, so old links resolve. | Maps |
| A dashboard changed in Grafana after the import | Run the import again: a dashboard whose Grafana `version` is newer is saved as a new version of the same dashboard, and the others are skipped. A new version runs every query like any later save, so one whose queries now fail is reported as failed and keeps its last version. | Changes |
| A folder titled `My Dashboards` | Lands in the importing Admin's own My Dashboards, because that title names the default folder here; rename it in Grafana before the import to keep it shared. | Changes |
| A dashboard imported from a file earlier | Holds its Grafana uid, since only the first import of a uid holds it, so the run updates that dashboard instead of making a copy. | Changes |
| Panels whose queries fail | Imported as unverified, then repaired by the agent from the Health page. | Changes |
| Public dashboards | [Public links](public-links.md) from the Share pane. | Changes |
| Kiosk mode and playlists | TV mode, which cycles through a dashboard's rows or a folder's dashboards. Playlists themselves stay in Grafana. | Changes |
| Provisioning files | The server's provisioning directory, or `observe dashboards apply` from Git. | Changes |
| Alerting rules, contact points and silences | Not imported, and an alert list panel imports as a placeholder. | Waits |
| Library panels | Imported as a placeholder, as in a single import. | Waits |
| Users, teams and folder permissions | Stay in Grafana; set [folder permissions](../teams/folders.md) here after the import. | Waits |

## The API

Everything in the Grafana tab is also in the HTTP API for Admins and Admin keys, and the repair queue for Editors and Admins.
The health list and apply have their own APIs, described in [Dashboard health](dashboard-health.md#api) and [Dashboards as code](dashboards-as-code.md#api).

### Connections

```sh
curl -X POST "$SERVER/api/v1/grafana" -H "Authorization: Bearer $OBSERVE_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"Production","url":"https://grafana.example.com","token":"<grafana token>"}'
```

The answer is the connection with its `id`.
`POST /api/v1/grafana/{id}/test` reads Grafana's health, its top-level folders and the first page of its dashboards, and stores the outcome in `last_test`.
A Grafana that cannot be reached or refuses the token still answers `200`, with `last_test.ok` false.

### Datasources and folders

`GET /api/v1/grafana/{id}/datasources` lists Grafana's datasources, and `matched_uid` names the datasource of yours each one matches by the importer's automatic rules.
Each one also names its `kind`, the type of yours it maps to (`prometheus`, `clickhouse`, `loki` or `tempo`), and its `candidates`, the uids of your datasources of that kind, which are the only mapping targets an import keeps.
A null `matched_uid` means the datasource needs a mapping, or has a type SlashObserve does not query, in which case its `kind` is null and its panels import as placeholders.
A `kind` with empty `candidates` means no datasource of that type is configured here: it needs no mapping, and its panels import as placeholders.
Give the open matches in the `mapping` of the import, keyed by the Grafana datasource uid or name, or add the Grafana uid to `grafanaUids` in the [datasources file](../reference/datasources-file.md) so every later import matches it too.
A mapping entry that a dashboard does not use is ignored for that dashboard.

`GET /api/v1/grafana/{id}/folders` lists Grafana's folders with how many dashboards each holds; `general` stands for the dashboards outside any folder.

### Imports

Start the import with every folder, or with the folders you choose (each with its subfolders):

```sh
curl -X POST "$SERVER/api/v1/grafana/$ID/imports" -H "Authorization: Bearer $OBSERVE_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"folders":"all","mapping":{"Prometheus EU":"prom-eu"}}'
```

The answer is `202` with a `Location` header; `GET` it to follow `total`, `done`, `failed` and `skipped`, and the `results` with each dashboard's uid, title, outcome, dashboard id, unverified and placeholder panel counts and error.
The connection's `last_sync` keeps the summary of the last finished run.

### Repairs

`GET /api/v1/repairs` lists every dashboard, in folders you can see, whose latest version still has unverified panels or variables or placeholder panels, with their counts, when it was imported and its latest repair.
Editors and Admins can call it, and `folder_id` limits it to one folder.
A dashboard leaves the list once nothing in it fails.

`POST /api/v1/repairs` queues the agent on the dashboards you name, or on every listed dashboard:

```sh
curl -X POST "$SERVER/api/v1/repairs" -H "Authorization: Bearer $OBSERVE_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"all":true,"folder_id":"<folder id>"}'
```

Each dashboard gets one ordinary revision with this prompt:

```text
Repair every panel and variable listed as unverified so its query runs and returns data; keep every other panel unchanged and never delete an unverified panel.
```

The answer is `202` with the `queued` request ids and the `skipped` dashboards, each with a reason:

- `active`: a revision is already queued or running on it.
- `nothing_to_repair`: it holds only placeholders, which the agent never edits.
- `not_found`: it does not exist or is in a folder you cannot see.
- `forbidden_role`: you can see it but not edit it.
- `queue_full`: you already have 20 repairs queued or running; send the request again when they finish.
