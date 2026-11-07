---
title: Known limitations
description: What SlashObserve does not do yet, so you can check it against your needs before you adopt it.
---

SlashObserve is pre-1.0.
These are the gaps that most often decide whether it fits.

## Datasources

SlashObserve reads six datasource types: Prometheus, ClickHouse, Loki, Tempo, Elasticsearch and OpenSearch.
Elasticsearch and OpenSearch are logs datasources.
A datasources file entry with any other `type` is refused (see [Datasources file](../reference/datasources-file.md)).
There is no Jaeger, InfluxDB, CloudWatch or SQL database datasource such as MySQL or PostgreSQL.
When you import a Grafana panel that queries another type, such as InfluxDB, it imports as a placeholder that says "Influxdb is not supported yet." and the datasource cannot be mapped.
Tempo search builder queries and TraceQL metrics queries are dropped from an import by decision; write the search as a TraceQL query instead.
The agent finds metrics, labels, tables, columns, log labels and trace attributes on these six only.

## No alerting

There are no alert rules, no notifications and no Alertmanager integration; alerting is the next stage of the roadmap.
An alert list panel shows the firing and pending alerts that a Prometheus `ALERTS` query returns, and that is all it does with alerts.

## Exploring data

The [logs page and the trace search](../use/logs-and-traces.md) have gaps.
The logs page reads Loki, ClickHouse, Elasticsearch and OpenSearch, and the trace search reads Tempo and ClickHouse.
You can add filter pills on the logs page, but they do not narrow the results yet, because the page never adds them to the query it runs; write the condition in the query instead.
The logs results have no page bar yet; the page shows the newest 1,000 lines of the time range, so narrow the query or the time range to reach older lines.
Live tail in the browser polls the datasource every 2 seconds and does not use the server's Server-Sent Events stream, which scripts can read.
Pasting a Kibana or OpenSearch Dashboards Discover URL is not supported.
Metrics have no explore page of their own; try a PromQL or SQL query in the panel editor's live preview (see [Edit a panel](../use/edit-panel.md)), or ask the agent.

## Sign-in needs an OIDC provider

People sign in through an OpenID Connect provider, and there are no local passwords.
The server refuses to start without the provider's settings and an allowed email domain list (see [Sign-in with OIDC](../connect/oidc.md)).
The demo stack includes Dex for this, with fixed development logins.
The CLI and MCP clients use service-account keys, which an Admin creates after signing in through OIDC.

## Grafana import

The import reads dashboard JSON that you provide.
It also fetches dashboards from a running Grafana, with their folders (see [Switch from Grafana](../use/switch-from-grafana.md)).
Panel types and options the ObserveSpec format cannot hold are reported at their path (see [Import Grafana dashboards](../use/import-grafana.md)); the [import coverage table](https://slashobserve.com/grafana) on slashobserve.com shows which Grafana features map, change or wait.

Alert rules and annotations are not imported.
The report marks them as excluded by decision, and a panel whose only report lines are such decisions still counts as imported.
Library panels import as placeholders, and users, teams, folder permissions and playlists stay in Grafana (see [Switch from Grafana](../use/switch-from-grafana.md#what-maps-what-changes-and-what-waits)).

## Wall screens

TV mode is part of the signed-in app, so a wall screen signs in as someone who can view the dashboard.
A public link's view has no TV mode and cannot cycle through rows or dashboards (see [Show it on a wall](../use/public-links.md#show-it-on-a-wall)).

## The ObserveSpec format may change

The dashboard format is ObserveSpec version `0`.
Before 1.0 it can change between releases.
An earlier change already rewrote every stored dashboard when the server started (see [Upgrades and backups](../operate/upgrades-and-backups.md)), so back up Postgres before you upgrade.
