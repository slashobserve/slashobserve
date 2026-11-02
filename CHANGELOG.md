# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
SlashObserve is pre-1.0: the dashboard format, the HTTP API and the settings can change between minor releases.

## [Unreleased]

### Changed

- The demo shop image is no longer published to GHCR.
  `make demo` builds it from `demo/shop` in the clone and pulls only the server image.

## [0.1.0] - 2026-11-02

First public release.

### Added

- Prompt to dashboard.
  Describe what you want to see in the web chat, with the `observe` CLI, or from an MCP client such as Claude Code, OpenCode or Cursor.
  The agent discovers real metric, label, table and column names, drafts the dashboard, and saves it only when every panel query runs and returns data.
  Follow-up prompts revise the same dashboard.
- LLM providers: Anthropic, and any server that implements the OpenAI Chat Completions API with tool calling, including a local Ollama.
  The server runs with the agent off when no provider is set, and MCP clients can still build dashboards with their own model.
- Dashboards as ObserveSpec version `0`, a JSON document checked against a closed schema:
  - 21 data panel types, among them time series, stat, gauge, table, heatmap, logs, traces, geomap and canvas, plus text, dashboard list and news panels and collapsible rows.
  - Eight variable types, including datasource, ad hoc filter and group-by variables.
  - Thresholds, value mappings, links, field overrides, data transformations, expression queries, panel repeats and time zones.
- Versions.
  Every save is an immutable version.
  Compare any two versions, restore an old one, and duplicate a dashboard.
- Review.
  Comment on a panel or a dashboard, send a comment to the agent as a revision request, and resolve the thread.
  Open dashboards update live.
- A panel editor for queries, settings and JSON, with a live preview on the real datasource and an option to ask the agent to change one query.
- Datasources: Prometheus, including exemplars, and ClickHouse queried as a read-only user.
  A Datasources page shows each one's health.
- Import of Grafana dashboard JSON from a web page, `observe import` or MCP.
  The import matches datasources, picks a folder, and reports what carried over, what was left out by decision and what needs attention, option by option.
- Access control.
  Sign-in through an OIDC provider, four roles (Admin, Editor, Commenter and Viewer), folders with access lists, a People page, and service-account keys for the CLI and MCP.
  Editors and Admins can save prompts and share them.
- Operations.
  One server binary with the web app embedded, plus Postgres.
  The server applies database migrations at startup, serves `/healthz` and `/readyz`, logs JSON without secrets, prompt text or query text, and fetches content from other sites only for domains on an allowlist that starts empty.
  The server sends no telemetry.
- Distribution.
  The container image `ghcr.io/slashobserve/slashobserve` is tagged `0.1.0`, `0.1` and `latest`.
  `observe` binaries and a `checksums.txt` are attached to the GitHub release.
  `make demo` and `docker compose -f deploy/compose/compose.yaml up -d --wait` start a local demo stack with Dex, Prometheus, ClickHouse, a demo shop and sample history from the published images.
  The demo is for local use only, with fixed logins and published ports bound to 127.0.0.1.

[0.1.0]: https://github.com/slashobserve/slashobserve/releases/tag/v0.1.0
