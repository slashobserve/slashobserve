---
title: Switch from Grafana
description: Connect SlashObserve to a running Grafana, import every dashboard with its folders in one run, and keep links to the old dashboards working.
---

Instead of importing one file at a time, an Admin can store a Grafana URL and a service-account token once and let the server import every dashboard from that Grafana.
Each dashboard goes through the same importer as [Import Grafana dashboards](import-grafana.md), with the same datasource matching, the same report and one import record per dashboard.

## Before you start

- Set `SLASHOBSERVE_SECRET_KEY` (see [Configuration](../reference/configuration.md#stored-secrets)).
  The server encrypts each token with a key derived from it, and adding a connection without it fails with `409 secret_key_missing`.
- In Grafana, create a service account with the Viewer role on the folders you want to bring over, and a token for it.
  The server only reads from Grafana: `/api/health`, `/api/folders`, `/api/search`, `/api/datasources` and `/api/dashboards/uid/<uid>`.

## Who can connect, and why private hosts are allowed

Only Admins can add, change, test, list or delete a connection, or start an import from one; a key needs the Admin role too.
The token is never returned by the API, never logged, and only ever sent to the host of the connection's URL.
A new URL needs the token again in the same request, so a stored token never follows a URL change to another host.

Other outbound fetches, such as news feeds and images, reach only `https` on port 443 and refuse private addresses (see [Outbound access](../operate/outbound-access.md)).
A Grafana connection is the one exception: its URL may be `http` or `https` on any port, and its host may resolve to a private or loopback address, because Grafana usually runs inside the same network as SlashObserve.
This is safe to allow only because the person adding the URL is an Admin, who already controls the server's datasources and settings.
The host must still be a domain name, not an IP address, and it must be on the outbound allowlist: adding a connection allows its domain when the list is managed in Settings, and is refused with `403 outbound_domain_not_allowed` when `SLASHOBSERVE_OUTBOUND_DOMAINS` locks the list and does not name it.
Every request to Grafana keeps the outbound timeout and an 8 MiB size cap, and a redirect is never followed.

## Connect and test

```sh
curl -X POST "$SERVER/api/v1/grafana" -H "Authorization: Bearer $OBSERVE_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"Production","url":"https://grafana.example.com","token":"<grafana token>"}'
```

The answer is the connection with its `id`.
`POST /api/v1/grafana/{id}/test` reads Grafana's health, its top-level folders and the first page of its dashboards, and stores the outcome in `last_test`, for example `Connected: 12 top-level folders, 340 dashboards.`
A Grafana that cannot be reached or refuses the token still answers `200`, with `last_test.ok` false and a fixed message such as `Grafana refused the token (status 401).`

## Map datasources

`GET /api/v1/grafana/{id}/datasources` lists Grafana's datasources, and `matched_uid` names the datasource of yours each one matches by the importer's automatic rules.
Each one also names its `kind`, the type of yours it maps to (`prometheus` or `clickhouse`), and its `candidates`, the uids of your datasources of that kind, which are the only mapping targets an import keeps.
A null `matched_uid` means the datasource needs a mapping, or has a type SlashObserve does not query, in which case its `kind` is null and its panels import as placeholders.
Give the open matches in the `mapping` of the import, keyed by the Grafana datasource uid or name, or add the Grafana uid to `grafanaUids` in the [datasources file](../reference/datasources-file.md) so every later import matches it too.
A mapping entry that a dashboard does not use is ignored for that dashboard.

## Import all

`GET /api/v1/grafana/{id}/folders` lists Grafana's folders with how many dashboards each holds; `general` stands for the dashboards outside any folder.
Start the import with every folder, or with the folders you choose (each with its subfolders):

```sh
curl -X POST "$SERVER/api/v1/grafana/$ID/imports" -H "Authorization: Bearer $OBSERVE_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"folders":"all","mapping":{"Loki logs":"clickhouse"}}'
```

The answer is `202` with a `Location` header; `GET` it to follow `total`, `done`, `failed` and `skipped`, and the `results` with each dashboard's uid, title, outcome, dashboard id, unverified and placeholder panel counts and error.
The server fetches at most 4 dashboards from Grafana at a time and saves them one after another, and a dashboard that fails never stops the run.
A connection runs one import at a time.
The server keeps the last 20 runs in memory until it restarts, and the connection's `last_sync` keeps the summary of the last finished one.
A restart stops a running import; start it again and it continues by skipping what it already imported.

## What maps

- Folders keep their Grafana titles, and a nested folder becomes one folder titled `Parent / Child`.
  A missing folder is created, and an existing one with the same title, in any case, is reused.
- Dashboards outside any Grafana folder go to General, not to your own My Dashboards.
- Tags and dashboard links import with the dashboard, as they do for a single import.
- Each import record holds the dashboard's Grafana uid in `grafana_uid` and its Grafana `version` in the summary.

## What changes

- Run the import again to pick up changes: a dashboard whose Grafana `version` is newer than the one imported is saved as a new version of the same dashboard, and the others are skipped.
  A new version runs every query like any later save, so a dashboard whose queries now fail is reported as failed and keeps its last version.
- A Grafana folder titled `My Dashboards` lands in the importing Admin's own My Dashboards, because that title names the default folder here; rename it in Grafana before the import to keep it shared.
- Only the first import of a Grafana uid holds it.
  A dashboard imported from a file before the connection holds its uid, and the run updates that dashboard instead of making a copy.

## Open old links

`GET /api/v1/grafana/resolve/{uid}` answers the `dashboard_id` and the web `url` of the dashboard imported from a Grafana uid.
Any signed-in person can call it, and a uid whose dashboard is in a folder they cannot see answers `404`, like a uid nobody imported.

## What waits

- Alerting rules, contact points and silences are not imported, and an alert list panel imports as a placeholder.
- A library panel imports as a placeholder, as it does in a single import.
- Playlists, users, teams and Grafana's folder permissions stay in Grafana; set [folder permissions](../teams/folders.md) here after the import.
