# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
SlashObserve is pre-1.0: the dashboard format, the HTTP API and the settings can change between minor releases.

## [0.1.1] - 2026-11-08

This release adds features and database migrations despite its patch number.
Migration 25 is the only new one, and no later migration exists.
It adds tables and constraints, gives every person a private My Dashboards folder and fills in the Grafana uid of dashboards you imported before.
Back up Postgres before you upgrade.
The server never runs a migration backwards, so going back to 0.1.0 means restoring the backup you took before upgrading.

### Added

- Loki and Tempo datasources.
  Loki feeds logs panels and log metrics, and Tempo feeds traces panels, read by trace id or by a TraceQL search.
- Elasticsearch and OpenSearch as logs datasources.
  Each takes an index pattern, a time field and field roles.
  You query them in KQL, in Lucene with a `lucene:` prefix, or with a Query DSL body, and the agent finds their fields like the names on any other datasource.
- A logs page for Loki, ClickHouse, Elasticsearch and OpenSearch.
  It searches one datasource at a time in that datasource's own query language, or answers a plain-language question when a model is set up.
  A volume strip counts matching lines by level, a side column counts the values of each field, and patterns group similar lines.
  A line opens its details and its surrounding lines, and live tail follows new lines.
  **Add to dashboard** keeps a search as a logs panel, and the address of a search reopens it.
- A trace search and a trace view for Tempo and ClickHouse.
  Search by service, span, duration and errors or with TraceQL, open one trace with its spans, go from a log line to its trace and from a span to its logs.
- Import of Loki, Tempo, Elasticsearch and OpenSearch panels from Grafana.
- Switching from a running Grafana.
  An Admin stores a Grafana URL and a service-account token, encrypted with `SLASHOBSERVE_SECRET_KEY`, and imports every dashboard with its folders in one run, from the web app or with `observe grafana`.
  A link to an old Grafana dashboard, `/d/<grafana uid>`, redirects to the imported one.
- Dashboard health.
  The server checks every dashboard about once a day for panels that stopped working, and the Health page, `observe health` and the `list_dashboard_health` MCP tool list the dashboards with a problem, including a repair queue for imported panels that did not verify.
- Investigations.
  `/investigate` in a chat box, `observe investigate` and MCP clients ask the agent a question about your data and get a finding, a short written answer with up to four evidence panels.
  A chat message that only asks a question gets a finding too, and leaves the dashboard as it is.
- Annotations.
  Mark a deploy, an incident or a maintenance window with the CLI, the HTTP API or an MCP client, see it as a marker on charts, and let the agent read it during an investigation.
- Public links and TV mode.
  Someone who manages a folder can share one dashboard read-only without sign-in through a link that can expire and be revoked, and any signed-in person can show a dashboard on a wall screen in TV mode.
- Dashboards as code.
  `observe dashboards export` writes a folder to ObserveSpec files, and `observe dashboards apply` saves them back.
  `SLASHOBSERVE_PROVISION_DIR` makes the server turn a directory of such files into dashboards at startup and whenever a file changes.
  `SLASHOBSERVE_SERVICE_ACCOUNTS_FILE` creates service accounts and their keys from a file at startup.
- A My Dashboards folder for every person, private to its owner, so a Viewer or Commenter can ask the agent for dashboards there.
- Live builds.
  A new dashboard appears panel by panel on its build page while the agent works, you can cancel a run, keep a stopped build with **Save what was built**, or continue it.
- Short addresses.
  A dashboard opens at `/d/<id>/<title slug>`, and `/f`, `/c`, `/b`, `/logs`, `/traces` and `/t` open a folder, a chat, a build, the logs page, the trace search and one trace.
  The old `/dashboards/<id>`, `/folders/<id>`, `/observe/<id>` and `/builds/<id>` addresses redirect.
- Keyboard shortcuts on dashboards, shown with `?`, and on the logs page and the trace search.
- Operations.
  The server serves Prometheus metrics at `/metrics`, which `SLASHOBSERVE_METRICS=false` turns off, and the demo stack and the Helm chart provision an internals dashboard that charts them.
  A Helm chart in `deploy/helm/slashobserve` runs the server on Kubernetes with an external Postgres and every secret taken from a Secret you create.
- Demo.
  The demo stack adds Loki, Tempo, an OpenTelemetry Collector and three provisioned dashboards, and `make demo-search` adds Elasticsearch and OpenSearch.

### Changed

- New dashboards from the chat, a stopped build you keep and imports that name no folder go to your My Dashboards instead of a shared folder.
  Publish one to a folder from its **Share** pane.
  A service-account key still saves into General unless it names a folder.
- An agent run has no cap of 3 minutes and 20 model calls any more.
  It stops when it makes no progress, after three rejected saves in a row, or when its requester cancels it, and `SLASHOBSERVE_AGENT_RUN_TIMEOUT` sets an optional time limit.
- Model calls retry up to 4 times with backoff, `SLASHOBSERVE_LLM_REQUEST_TIMEOUT` defaults to `10m` instead of `120s`, and `SLASHOBSERVE_LLM_IDLE_TIMEOUT` retries a call that receives no bytes for 180 seconds.
  `SLASHOBSERVE_LLM_EFFORT` now also takes `none`, `minimal`, `low`, `medium`, `high` and `xhigh` on an OpenAI-compatible provider, where unset sends nothing.
- Panels reflow on narrow screens, and a panel whose size you set keeps it.

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

[0.1.1]: https://github.com/slashobserve/slashobserve/releases/tag/v0.1.1
[0.1.0]: https://github.com/slashobserve/slashobserve/releases/tag/v0.1.0
