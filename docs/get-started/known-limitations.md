---
title: Known limitations
description: What SlashObserve does not do yet, so you can check it against your needs before you adopt it.
---

SlashObserve is pre-1.0.
These are the gaps that most often decide whether it fits.

## Datasources

SlashObserve reads seven datasource types: Prometheus, ClickHouse, Loki, Tempo, Jaeger, Elasticsearch and OpenSearch.
Elasticsearch and OpenSearch are logs datasources.
A datasources file entry with any other `type` is refused (see [Datasources file](../reference/datasources-file.md)), and so is a datasource of another type in the app.
There is no InfluxDB, CloudWatch or SQL database datasource such as MySQL or PostgreSQL.
When you import a Grafana panel that queries another type, such as InfluxDB, it imports as a placeholder that says "Influxdb is not supported yet." and the datasource cannot be mapped.
Tempo search builder queries and TraceQL metrics queries are dropped from an import by decision; write the search as a TraceQL query instead.
The agent finds metrics, labels, tables, columns, log labels and trace attributes on these seven only.

## No alerting

There are no alert rules, no contact points, no silences, no notifications and no Alertmanager integration, and none is built.
Keep alerting where it runs today: Grafana alerting, or Prometheus rules with Alertmanager, which work without Grafana's dashboards.
An alert list panel shows the firing and pending alerts that a Prometheus `ALERTS` query returns, and that is all it does with alerts.

## Who can query which datasource

Datasources come from the server's [datasources file](../reference/datasources-file.md) or are [added in the app](../connect/datasources-in-the-app.md) by an Admin, and every signed-in person from Viewer up, and every service-account key, may run a query against any of them through the explore query that the logs page and the trace search use.
There is no per-team or per-folder datasource permission.
A restricted folder hides its dashboards, not the datasources they read (see [Folders and access](../teams/folders.md)).
To keep a team away from data, give the datasource account only the access you want every person here to have.
A run keeps [Admin-only data](../use/chat.md#answers-that-read-admin-only-data) out of what others read only for what the run itself reads: an Admin's own connected agent can read the audit trail outside a run and paste it into a run whose answer others read, and the server cannot see that.
Content that leaves the server and comes back is not traced either: an Admin who exports a dashboard that holds such an answer, or copies a panel out of it by hand, and imports or pastes it into a folder others read is not stopped, because the server sees only a new dashboard.
Runs from before the server traced Admin-only data are marked only when the audit trail recorded a query of theirs against the built-in audit trail or an app datasource set to `audience: admins`: a run that only read content in a folder only Admins can read is not marked, because nothing recorded that read, and neither is a run that queried a datasources file entry set to `audience: admins` or whose audit events were already removed by the retention window.

## Sharing, snapshots and reports

A public link opens one dashboard read-only and always shows its latest version, so there is no stored-data snapshot of a moment in time (see [Public links and TV mode](../use/public-links.md)).
Every response carries `X-Frame-Options: DENY` and `frame-ancestors 'none'`, so neither a dashboard nor a public link can be framed in another page, and there is no embeddable panel link.
There are no scheduled reports: nothing emails a dashboard on a schedule.
For a screen that must stay current, use a public link or [TV mode](../use/public-links.md#show-it-on-a-wall).

## Exploring data

The [logs page and the trace search](../use/logs-and-traces.md) have gaps.
The logs page reads Loki, ClickHouse, Elasticsearch and OpenSearch, and the trace search reads Tempo, Jaeger and ClickHouse.
A Jaeger datasource is searched by service, operation, tags and duration, not TraceQL, and has no trace statistics or trace comparison.
The [metrics page](../use/metrics.md) reads Prometheus; a SQL query on ClickHouse metrics runs in the panel editor's live preview (see [Edit a panel](../use/edit-panel.md)).

## Sign-in needs an OIDC provider

People sign in through an OpenID Connect provider, and there are no local passwords.
The server refuses to start without the provider's settings and an allowed email domain list (see [Sign-in with OIDC](../connect/oidc.md)).
The demo stack includes Dex for this, with fixed development logins.
The CLI and MCP clients use service-account keys, which an Admin creates after signing in through OIDC.

## Grafana import

The import reads dashboard JSON that you provide.
It also fetches dashboards from a running Grafana, with their folders (see [Switch from Grafana](../use/switch-from-grafana.md)).
Panel types and options the ObserveSpec format cannot hold are reported at their path (see [Import Grafana dashboards](../use/import-grafana.md)); the [import coverage table](https://slashobserve.com/grafana) on slashobserve.com shows which Grafana features map, change or wait.

Alert rules and annotation queries are not imported.
The report marks them as excluded by decision, and a panel whose only report lines are such decisions still counts as imported.
A library panel imports as a normal panel when the export carries its definition and as a placeholder when it does not, and a panel from a plugin imports as a placeholder.
There is no shared panel object and no plugin mechanism: SlashObserve draws the panel types it ships, and each dashboard holds its own copy of a panel.
Users, teams, folder permissions and playlists stay in Grafana (see [Switch from Grafana](../use/switch-from-grafana.md#what-maps-what-changes-and-what-waits)).

## Wall screens

TV mode is part of the signed-in app, so a wall screen signs in as someone who can view the dashboard.
A public link's view has no TV mode and cannot cycle through rows or dashboards (see [Show it on a wall](../use/public-links.md#show-it-on-a-wall)).

## The ObserveSpec format may change

The dashboard format is ObserveSpec version `0`.
Before 1.0 it can change between releases.
An earlier change already rewrote every stored dashboard when the server started (see [Upgrades and backups](../operate/upgrades-and-backups.md)), so back up Postgres before you upgrade.
