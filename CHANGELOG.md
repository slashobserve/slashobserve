# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
SlashObserve is pre-1.0: the dashboard format, the HTTP API and the settings can change between minor releases.

## [0.1.2] - 2026-11-16

This release adds features and database migrations despite its patch number.
Migrations 26 to 48 are new, and no later migration exists.
They add tables and columns for the activity log, comment threads, webhooks, MCP sign-in, saved searches, workspace search, observations, proposals, the inbox and the live servers, and they keep Grafana import state in Postgres.
Migration 28 moves every dashboard comment into a thread, and migration 41 gives every finding an observation and merges its comment threads into that observation's discussion.
Back up Postgres before you upgrade.
When you run more than one server, stop every old server before you start the first new one, because an older server writes queue rows and findings in a shape that migrations 35 and 41 refuse.
Migration 43 builds an index on the activity log with a plain `CREATE INDEX`, which blocks writes to it until it finishes, so upgrade when little is changing.
The server never runs a migration backwards, so going back to 0.1.1 means restoring the backup you took before upgrading.
ObserveSpec, the dashboard format, is unchanged, and the 0.1.1 `observe` commands still work.
The first five items under Changed can break an upgrade.

### Added

- Observations.
  An observation is a page about one question, in a folder, built from text, panel, snapshot and run blocks.
  A panel block runs live, and a snapshot block freezes the data at one moment.
  Editors add blocks, and a person who can only comment suggests one that an editor keeps or discards.
  Each observation has one discussion, where a message can belong to a block and a reply can quote the message it answers.
  Resolve an observation with a one-line summary, reopen it, or move it to another folder.
  **Add to observation** in a panel menu and on the logs page copies a panel or a search into one.
  An observation opens at `/o/<id>` and has `observe o`, `create_observation`, `add_block` and the other observation MCP tools.
- RCAs.
  Publishing freezes an observation into a numbered revision with its text, snapshots and a fresh snapshot of every live panel, and leaves out the query text.
  A person with Manage on the folder shares one revision read-only without sign-in at `/rca/<token>`, through a link that can expire and be revoked.
  No agent or MCP tool creates a link.
- Comment threads on dashboards, annotations, observations and proposals.
  You can edit your own comments and mention people, `observe comments` has `add`, `edit` and `reopen`, and MCP clients open, edit and reopen comments.
- Runs from a mention.
  A message that mentions exactly one agent starts a run for it, and the run answers in the same thread with a status line.
  You can mention `@observe`, your own connected agents and service accounts that take runs.
  From the CLI or MCP, send the comment with `--start-runs` or `start_runs`.
- Home and the inbox.
  The app opens on Home, which asks Observe Agent, starts an observation with a first note or imports a dashboard, and lists what needs you, the runs working now and what you opened last.
  The inbox lists what asks something of you, a proposal to merge, a block to keep, a mention, a reply, a failed run or a saved health repair, and its unread count follows you across browsers.
  The MCP tool `list_inbox` reads it.
- Connected agents.
  An MCP client such as Claude Code connects over OAuth at `/mcp`, you approve it once in the browser, and it acts as you with your role and folder access.
  You can have 20 connections, and you disconnect them in Settings.
  An Admin sees and disconnects everyone's, and signing a person out everywhere disconnects their agents.
  A service-account key still works for bots.
- The Agents page.
  It shows the Observe Agent, your connected agents and the service accounts that take runs, each with its status and its runs of the last seven days.
  A connected agent has a direct or propose mode, and its page lists its recent work.
- Runs for outside agents.
  An Admin turns on **Takes agent runs** for a service account in the web app, with `takes_runs` or with `takesRuns` in the service-accounts file.
  A connected agent or such an account claims the runs mentioned to it with `claim_run`, under the guards of the built-in agent.
  A run that no outside agent takes within 10 minutes fails with `agent_unavailable`.
- Proposals.
  A save from a person who can comment but not edit, or from a connection in propose mode, waits as a proposal until someone who can edit merges or discards it.
  An author has one open proposal per dashboard, and a merge is refused while the proposal and newer versions changed the same panel, variable or setting.
  Proposals work over HTTP, MCP, `observe proposals` and the page at `/proposals/<id>`.
  A save with `merge: auto` merges a stale save onto the latest version when nothing overlaps.
- The activity log.
  `GET /api/v1/activity`, its live stream, `observe activity --follow` and the MCP tool `list_activity` record who changed what on dashboards, annotations, observations, proposals, threads, saved searches and agent runs, and each person sees only the folders they can view.
  `SLASHOBSERVE_ACTIVITY_RETENTION` sets how long events stay, 180 days by default.
- Webhooks.
  An Admin sends activity events to an HTTPS address as signed requests that retry in order and never skip an event, from Settings, the API or `observe webhooks`.
  Webhooks need `SLASHOBSERVE_SECRET_KEY` and a domain on the outbound allowlist, and no agent or MCP tool manages one.
- Search.
  The command palette, `GET /api/v1/search`, `observe search` and the MCP tool `search_workspace` find dashboards, observations, annotations, comments and saved searches by their words, and show each person only what they can open.
- Saved searches.
  A logs or trace search saves into a folder, follows that folder's access, and reopens from the logs and traces pages, `observe saved-searches` and MCP.
- Logs and traces per source.
  Every log source and every trace source has its own page, listed in the sidebar, with `/logs` and `/traces` listing the sources and saved searches.
  Filter pills narrow a logs query without retyping it, you can select a range on the volume strip, the results have a page bar, and live tail reads a Server-Sent Events stream.
  Paste a Kibana or OpenSearch Dashboards Discover link into the query box and the page fills in the query, range and filters.
  The trace search has **Live** and **Copy link**.
- Grafana import from a link.
  An Admin pastes a dashboard link on Home, and the server fetches the dashboard through a stored Grafana connection.
  An Admin can also stop a running import.
- Run cost.
  Each finished run reports an estimated cost in US dollars from a built-in price table, shown in the web app, `observe status` and the activity log.
  `SLASHOBSERVE_LLM_PRICES_FILE` prices other models, and `SLASHOBSERVE_AGENT_RUN_MAX_COST_USD` stops a run that reaches a limit.
- The Chats page.
  It lists all your chats with search and a state filter, and an Admin can read everyone's chats read only.
- Pinned dashboards.
  A pin puts a dashboard in the sidebar under Dashboards.
- More than one server.
  Two or more servers share one Postgres and behave as one for jobs, events, agent runs, webhooks, OAuth tokens and provisioning, and the Run more than one server page says what to set.
  `SLASHOBSERVE_AGENT_WORKERS` sets the agent requests each server runs at once, 8 by default, and `maxConcurrentQueries` caps the queries one datasource takes from the whole fleet.
  `SLASHOBSERVE_DATABASE_DIRECT_URL`, and `database.directUrlKey` in the Helm chart, send `LISTEN` and migrations around a transaction-mode pooler such as PgBouncer.
  The internals dashboard and `/metrics` show the live servers, datasource slots and how long agent requests wait.
- More commands.
  `observe` can move, duplicate, restore and diff dashboards and manage folders, annotations and observations, and `observe skill install` installs the `/observe` skill.
  MCP serves the same guide as the `observe` prompt.

### Changed

- The built-in agent now runs as `observe-agent`, a separate process that the server starts whenever a model provider is configured.
  The image carries it, runs on `distroless/base-nossl` and is about 107 MB larger unpacked, and each server needs about 50 MiB more memory.
  The compose stack raises the server memory limit from 256M to 512M.
- The server does not start with a model provider when `observe-agent` is missing from `PATH`, or when `SLASHOBSERVE_HTTP_ADDR` names a host other than a loopback address or all addresses, because the agent reaches the server on loopback.
- Findings are observations.
  The MCP tools `create_finding` and `list_findings` are gone, so an MCP client saves an answer with `create_observation` and `add_block` and reads earlier ones with `list_observations`.
  The routes under `/api/v1/findings` still answer, with a `Deprecation` header, and the next release removes them.
  A finding's page, `/findings/<id>`, redirects to its observation.
- A prompt that starts with `activity`, `o`, `proposals`, `retry`, `saved-searches`, `search`, `skill` or `webhooks` is now a command name, so quote it as one argument.
- The metric `slashobserve_agent_runs_total` no longer has the outcome `lease_lost`, and `SLASHOBSERVE_AGENT_LOG_TOOL_INPUTS` is no longer read.
- Investigations, and chat messages that only ask a question, write their answer into an observation, in the dashboard's folder when you can edit it and otherwise in your My Dashboards.
  `POST /api/v1/observe` takes `observation_id` to write into an existing one.
- The chat composer's **Send to agent** button is gone.
  Mention `@observe` in a comment or a reply to ask the agent.
  A Commenter can ask the agent for a change, and the result arrives as a proposal.
- MCP tools need Viewer, as the web app does for the same work, so a Viewer key can now call the discovery and query tools that needed Editor before.
  The folder's access still decides what a call may change.
- Home replaces the Observe page at `/`, and the chat moved to `/ask`.
  The Health page is part of the dashboards list now, and `/health` opens the list filtered to dashboards with a problem.
  A dashboard whose last health check failed is checked again on the next hourly run.
- Folders hold observations and saved searches as well as dashboards, and a folder with any of them cannot be deleted.
- The sidebar starts at Home and folds to an icon rail, `⌘\` or `Ctrl+\` collapses it, and phones get one top bar with Menu, the page title, Search and Inbox.

### Fixed

- Under many concurrent queries a server could use up its ports to one datasource and answer every call with `datasource_unavailable: connection failed`.
  The Prometheus, Loki, Tempo and Elasticsearch clients now keep an idle connection for each concurrent call.
- A cached panel result no longer holds up to twice its size in memory, which got a server killed for running out of memory under 200 viewers on distinct time ranges.
- The next and previous links of the dashboards list and a folder page no longer return to page 1.
- The composer and the panel menu no longer take focus from a picker you opened.
- The demo Tempo no longer runs out of memory during compaction.

## [0.1.1] - 2026-11-09

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

[0.1.2]: https://github.com/slashobserve/slashobserve/releases/tag/v0.1.2
[0.1.1]: https://github.com/slashobserve/slashobserve/releases/tag/v0.1.1
[0.1.0]: https://github.com/slashobserve/slashobserve/releases/tag/v0.1.0
