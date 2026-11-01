---
title: Known limitations
description: What SlashObserve does not do yet, so you can check it against your needs before you adopt it.
---

SlashObserve is pre-1.0.
These are the gaps that most often decide whether it fits.

## Datasources

Prometheus and ClickHouse are the only datasource types.
A datasources file entry with any other `type` is refused (see [Datasources file](../reference/datasources-file.md)).
When you import a Grafana panel that queries another type, such as Loki, the report says "Loki is not supported yet." and the datasource cannot be mapped.
The agent finds metrics, labels, tables and columns on these two only.

## No alerting, and no separate logs or traces pages

There are no alert rules, no notifications and no Alertmanager integration.
An alert list panel shows the firing and pending alerts that a Prometheus `ALERTS` query returns, and that is all it does with alerts.

The web app has no log search page and no trace explorer.
Logs and traces appear as panels on a dashboard, fed by ClickHouse queries, and you change them through the panel editor or a prompt (see [Edit a panel](../use/edit-panel.md)).

## Sign-in needs an OIDC provider

People sign in through an OpenID Connect provider, and there are no local passwords.
The server refuses to start without the provider's settings and an allowed email domain list (see [Sign-in with OIDC](../connect/oidc.md)).
The demo stack includes Dex for this, with fixed development logins.
The CLI and MCP clients use service-account keys, which an Admin creates after signing in through OIDC.

## Grafana import

The import reads dashboard JSON that you provide.
It does not connect to a Grafana server, so it cannot list or fetch dashboards through Grafana's HTTP API.

Alert rules and annotations are not imported.
The report marks them as excluded by decision, and a panel whose only report lines are such decisions still counts as imported.
Other options the ObserveSpec format cannot hold are reported at their path (see [Import Grafana dashboards](../use/import-grafana.md)).

## The ObserveSpec format may change

The dashboard format is ObserveSpec version `0`.
Before 1.0 it can change between releases.
An earlier change already rewrote every stored dashboard when the server started (see [Upgrades and backups](../operate/upgrades-and-backups.md)), so back up Postgres before you upgrade.
