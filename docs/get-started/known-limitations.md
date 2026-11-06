---
title: Known limitations
description: What SlashObserve does not do yet, so you can check it against your needs before you adopt it.
---

SlashObserve is pre-1.0.
These are the gaps that most often decide whether it fits.

## Datasources

Prometheus, ClickHouse, Loki and Tempo are the only datasource types.
A datasources file entry with any other `type` is refused (see [Datasources file](../reference/datasources-file.md)).
There is no Jaeger, Elasticsearch, InfluxDB, CloudWatch or SQL database datasource such as MySQL or PostgreSQL.
The agent reads and queries these four types only.

## No alerting

There are no alert rules, no notifications and no Alertmanager integration; alerting is the next stage of the roadmap.
An alert list panel shows the firing and pending alerts that a Prometheus `ALERTS` query returns, and that is all it does with alerts.

## Exploring data

The [logs page and the trace search](../use/logs-and-traces.md) read Loki, Tempo and ClickHouse.
Metrics have no explore page of their own; try a PromQL or SQL query in the panel editor's live preview (see [Edit a panel](../use/edit-panel.md)), or ask the agent.

## Sign-in needs an OIDC provider

People sign in through an OpenID Connect provider, and there are no local passwords.
The server refuses to start without the provider's settings and an allowed email domain list (see [Sign-in with OIDC](../connect/oidc.md)).
The demo stack includes Dex for this, with fixed development logins.
The CLI and MCP clients use service-account keys, which an Admin creates after signing in through OIDC.

## Grafana import

The importer maps Grafana's Prometheus and ClickHouse datasources.
A panel that queries any other type, such as Loki, imports as a placeholder whose report line reads "Loki is not supported yet.", and the datasource cannot be mapped.
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
