---
title: Known limitations
description: What SlashObserve does not do yet, so you can check it against your needs before you adopt it.
---

SlashObserve is pre-1.0.
This page lists the gaps that most often decide whether it fits, so you can check them before you adopt it.

## Check these before you adopt it

1. Your data sits in one of seven datasource types: Prometheus, ClickHouse, Loki, Tempo, Jaeger, Elasticsearch or OpenSearch.
2. You do not need SlashObserve to send alerts.
3. Everyone who signs in may query every datasource that is open to everyone, and you accept that.
4. You can sign people in through OpenID Connect (OIDC), or use only service-account keys.
5. You can back up Postgres before each upgrade, because the dashboard format can change before 1.0.

The sections below give the detail.

## Datasources

SlashObserve reads seven datasource types: Prometheus, ClickHouse, Loki, Tempo, Jaeger, Elasticsearch and OpenSearch.
Elasticsearch and OpenSearch are logs datasources.
The agent finds metrics, labels, tables, columns, log labels and trace attributes on these seven only.

There is no InfluxDB, CloudWatch or SQL database datasource such as MySQL or PostgreSQL.
A datasources file entry with any other `type` is refused, and so is a datasource of another type in the app.
See [Datasources file](../reference/datasources-file.md).

When you import a Grafana panel that queries another type, such as InfluxDB, it imports as a placeholder that says "Influxdb is not supported yet." and the datasource cannot be mapped.
Tempo search builder queries and TraceQL metrics queries are dropped from an import by decision.
Write the search as a TraceQL query instead.

## No alerting

There are no alert rules, no contact points, no silences, no notifications and no Alertmanager integration, and none is built.
Keep alerting where it runs today: Grafana alerting, or Prometheus rules with Alertmanager, which work without Grafana's dashboards.
An alert list panel shows the firing and pending alerts that a Prometheus `ALERTS` query returns, and that is all it does with alerts.

## Who can query which datasource

Datasources come from the server's [datasources file](../reference/datasources-file.md) or are [added in the app](../connect/datasources-in-the-app.md) by an Admin.
By default, every signed-in person from Viewer up, and every service-account key, may run a query against any of them.
They do it through the explore query that the logs page and the trace search use.

A datasource can instead be set to `audience: admins`, and then only Admins and Admin keys can list or query it.
There is no per-team or per-folder datasource permission.
A restricted folder hides its dashboards, not the datasources they read (see [Folders and access](../teams/folders.md)).
To keep a team away from data, give the datasource account only the access you want every person here to have.

### Admin-only data in agent answers

A run keeps [Admin-only data](../use/chat.md#answers-that-read-admin-only-data) out of what others read, but only for what the run itself reads.
The server cannot see two kinds of leak:

- An Admin's own connected agent can read the audit trail outside a run and paste it into a run whose answer others read.
- Content that leaves the server and comes back is not traced.
  An Admin who exports a dashboard that holds such an answer, or copies a panel out of it by hand, and imports or pastes it into a folder others read is not stopped.
  The server sees only a new dashboard.

Runs from before the server traced Admin-only data are marked only when the audit trail recorded a query of theirs against the built-in audit trail or an app datasource set to `audience: admins`.
These runs are not marked:

- a run that only read content in a folder only Admins can read, because nothing recorded that read;
- a run that queried a datasources file entry set to `audience: admins`;
- a run whose audit events were already removed by the retention window.

## Sharing, snapshots and reports

A public link opens one dashboard read-only and always shows its latest version, so there is no stored-data snapshot of a moment in time.
See [Public links and TV mode](../use/public-links.md).

Every response carries `X-Frame-Options: DENY` and `frame-ancestors 'none'`.
So neither a dashboard nor a public link can be framed in another page, and there is no embeddable panel link.

There are no scheduled reports, so nothing emails a dashboard on a schedule.
For a screen that must stay current, use a public link or [TV mode](../use/public-links.md#show-it-on-a-wall).

## Exploring data

The [logs page and the trace search](../use/logs-and-traces.md) have gaps.

- The logs page reads Loki, ClickHouse, Elasticsearch and OpenSearch.
- The trace search reads Tempo, Jaeger and ClickHouse.
- A Jaeger datasource is searched by service, operation, tags and duration, not TraceQL, and has no trace statistics or trace comparison.
- The [metrics page](../use/metrics.md) reads Prometheus.
  A SQL query on ClickHouse metrics runs in the panel editor's live preview; see [Edit a panel](../use/edit-panel.md).

## Sign-in needs an OIDC provider or keys

People sign in through an OpenID Connect provider, and there are no local passwords.
Browser sign-in needs the provider's issuer, client ID and client secret together, and an allowed email domain list; see [Sign-in with OIDC](../connect/oidc.md).
The server refuses to start when only some of the provider settings are set, or when the domain list is missing.

A server with none of the provider settings runs with no browser sign-in, and only service-account keys authenticate; see [Run with no browser sign-in](../operate/service-account-keys.md#run-with-no-browser-sign-in).
The demo stack includes Dex for browser sign-in, with fixed development logins.
The CLI and MCP clients use service-account keys, which an Admin creates after signing in through OIDC.

## Grafana import

The import reads dashboard JSON that you provide.
It also fetches dashboards from a running Grafana, with their folders; see [Switch from Grafana](../use/switch-from-grafana.md).
Panel types and options the ObserveSpec format cannot hold are reported at their path; see [Import Grafana dashboards](../use/import-grafana.md).
The [import coverage table](https://slashobserve.com/grafana) on slashobserve.com shows which Grafana features map, change or wait.

- Alert rules and annotation queries are not imported.
  The report marks them as excluded by decision, and a panel whose only report lines are such decisions still counts as imported.
- A library panel imports as a normal panel when the export carries its definition, and as a placeholder when it does not.
- A panel from a plugin imports as a placeholder.
  There is no shared panel object and no plugin mechanism, so SlashObserve draws the panel types it ships, and each dashboard holds its own copy of a panel.
- Users, teams, folder permissions and playlists stay in Grafana; see [Switch from Grafana](../use/switch-from-grafana.md#what-maps-what-changes-and-what-waits).

## Wall screens

TV mode is part of the signed-in app, so a wall screen signs in as someone who can view the dashboard.
A public link's view has no TV mode and cannot cycle through rows or dashboards.
See [Show it on a wall](../use/public-links.md#show-it-on-a-wall).

## The ObserveSpec format may change

The dashboard format is ObserveSpec version `0`.
Before 1.0 it can change between releases.
An earlier change already rewrote every stored dashboard when the server started, so back up Postgres before you upgrade.
See [Upgrades and backups](../operate/upgrades-and-backups.md).
