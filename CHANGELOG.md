# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
SlashObserve is pre-1.0: the dashboard format, the HTTP API and the settings can change between minor releases.

## [0.1.4] - 2026-11-22

This release adds features and database migrations despite its patch number.
Migrations 74 to 90 are new, there are no migrations 76 to 78, and no later migration exists.
They add the audit trail and its export position, datasource audiences, Only Admins folders, anchors and agent columns on threads, chats, the starter content record, observation block heights, run intent and read restrictions, and a delete trigger for dashboards.
Migration 74 creates the audit table and copies every activity event into it, which takes longer the more activity the log holds.
Migrations 81, 82, 84 and 87 run outside a transaction, add each check constraint and foreign key as `NOT VALID` and validate it afterwards, and build their indexes concurrently, so they do not hold a write lock for the whole run.
Migration 82 gives every earlier chat a row, and migration 87 gives every run an intent and a kind, both in batches of 1000.
Back up Postgres before you upgrade.
The server never runs a migration backwards, so going back to 0.1.3 means restoring the backup you took before upgrading.
The server now records every sign-in, change, refusal, tool call and query in an audit trail in your own Postgres, kept for a year by default, so size the database for it and read the upgrade notes in the operations guide.
The Breaking changes below can break a script, an integration or a CLI call.

### Breaking changes

- `observe comments list <dashboard-id> --json` prints the thread page of `observe comments list --on dashboard:<id>`, and the `comments` key is gone (#241).
  Before, it printed `{"comments": [...], "next_cursor": ...}` from the older dashboard comments route.
  Now it prints `threads`, `messages`, `next_cursor` and `can_comment`.
  A script that read `jq '.comments[]'` reads `jq '.messages[]'` instead, as `docs/use/cli.md` documents.
  A message's `thread_id` names its thread in `threads` and replaces `parent_id`, a reply being a message whose `thread_id` is not its own `id`.
  `actor.label` replaces `author.name`, and `status`, `panel_id`, `panel_present` and `resolved_by_version` move from the comment to its thread.
  In the table, the AUTHOR column shows the actor label, such as `Ada (via Claude Code)`, and a run's reply shows the answer's title after `Answered.`.
  The route `GET /api/v1/dashboards/{id}/comments` is unchanged.
- The retired spellings for asking an agent and for naming a chat are removed (#232).
  `start_runs` is gone from `POST /api/v1/threads` and `POST /api/v1/threads/{thread_id}/messages`, from the MCP tools `create_comment` and `reply_to_comment`, and as `--start-runs` from `observe comments add` and `observe comments reply`.
  Send `to: agent`, or pass `--to agent`, which asks the agent the text mentions, else the thread's agent, else `@observe`.
  `follows_id` is gone from `POST /api/v1/observe` and the MCP tool `start_run`, and `--follows` is gone from `observe`.
  Send `chat_id`, or pass `--chat`, with the id of one of your chats from `GET /api/v1/chats`, `list_chats` or `observe chats list`.
  No database migration is involved, because neither name was a column.
- A message to the agent in a thread now starts a read-only ask, and `kind` and `source` of a run changed (#179, #188).
  Before, `start_runs` on a dashboard thread started a comment run that saved a version or opened a proposal, and on an observation thread an investigation that wrote into the page.
  Now `to: agent` on either starts an ask that answers in the thread and changes nothing, for an Editor as for a Commenter.
  To change the dashboard from the answer, apply its suggested change with `POST /api/v1/threads/{thread_id}/apply`, `apply_suggested_change` or `observe comments apply`, or start a revision.
  `kind` on `POST /api/v1/observe`, `start_run` and the CLI is now `ask` (the default), `revise` or `investigate`, where it was `build` or `investigate`, and a request with `kind` `build` is refused with 400 `invalid_request`.
  A request with `dashboard_id` and no `kind` no longer revises that dashboard.
  It asks about it privately, and only `kind` `revise` changes it, as in `observe --revise --dashboard <id> '<prompt>'`, where `observe --dashboard <id> '<prompt>'` used to revise.
- Runs carry `intent` and `investigation` where they carried `source` (#188).
  `source` and its values `observe`, `prompt`, `comment`, `suggest` and `investigate` are gone from a run in `GET /api/v1/observe`, `GET /api/v1/observe/{id}`, `list_runs`, `get_run` and a claimed run, and `intent` is `ask`, `apply` or `suggest`.
  `investigation` is true for a run that opens or writes into an observation and false for an ask.
  The `source` filter is gone from `GET /api/v1/observe`, from `list_runs` and from `observe requests list`, whose table shows an intent column in its place.
  The `data.source` of a `run.*` event in the activity feed, an inbox item or a webhook payload is now `data.intent`, and migration 87 rewrites the events already stored.
- The admin scope of the activity feed is removed (#177).
  `GET /api/v1/activity?scope=admin`, `observe activity --admin` and the `scope` argument of `list_activity` no longer exist, and the old Settings, Audit log page that used them is replaced by the Audit module.
  An Admin key still gets the admin changes in the ordinary activity list, and `GET /api/v1/audit/events`, `observe audit search` and `search_audit_events` read the audit trail.
- `chat_id` of a run that belongs to no chat is null, where `GET /api/v1/observe` answered the run's own id (#187).
- Upgrade notes that need a decision before the first start:
  Set `SLASHOBSERVE_AUDIT_RETENTION` (default `8760h`, one year) to the history you want to keep, because the audit table grows with every sign-in, change, tool call and query, and an Admin cannot delete an event.
  Set `SLASHOBSERVE_TRUSTED_PROXIES` to the addresses or CIDR ranges of your load balancer or ingress controller when it sits in front of the server, or every audit event records the balancer's address as the client address; the Helm chart takes `trustedProxies`.
  An invalid entry stops the server at start and names the entry.
  Set `SLASHOBSERVE_AUDIT_OTLP_ENDPOINT` and `SLASHOBSERVE_AUDIT_OTLP_HEADERS_FILE` only if you want a copy of the trail in a log pipeline; unset, nothing is sent.
  On `SIGTERM` the server now ends the requests still open after 10 seconds, such as live tails, event streams and `/mcp` streams, so each one records its audit event before the server exits, and a proxy or client of such a stream sees it close then.
  Upgrading from 0.1.2 reads both sets of migrations, 49 to 73 from the 0.1.3 notes and 74 to 90 here.

### Added

- The audit trail.
  The server records who did what, through what, to what and how it ended, in the table `audit_events` of its own Postgres, one partition for each UTC day.
  A change is recorded in the same transaction as the change, so the two commit or roll back together, and the database refuses to update, delete or truncate an event.
  Sign-ins and failed sign-ins, credentials and CSRF tokens that were refused, OAuth and MCP sign-in, webhook deliveries, folder and team changes, Grafana imports, provisioning, scheduled job runs, the server's start and stop and every migration it applies are recorded, and so is every request that is refused with 401, 403 or 429.
  Every MCP tool call, every datasource query with its query text cut at 4 KiB, and views of a dashboard, an observation, a public link, a run and a chat are events too.
  An audit search and a read of someone else's chat or run are recorded once in 10 minutes, and repeated refusals of a sign-in, a credential or a CSRF token are folded into one event a minute.
  A run's start, finish and release name the agent that took it, and a run that failed or was canceled is recorded as failed.
  A write that times out is kept and written again, and a loss is recorded as `audit.gap`, with `slashobserve_audit_dropped_total` and `slashobserve_audit_partitions_ahead` to watch.
  Retention drops a whole partition once its end is older than `SLASHOBSERVE_AUDIT_RETENTION`, and migration 74 copies the activity log into the table so the trail starts with your history.
- Reading the audit trail.
  An Admin reads it with `GET /api/v1/audit/events`, `/audit/events/{id}`, `/audit/facets`, `/audit/histogram`, `/audit/aggregate`, `/audit/fields`, `/audit/export` and `/audit/stream`, with `observe audit search`, `show`, `facets`, `histogram`, `aggregate`, `fields` and `export`, and with the MCP tools `search_audit_events`, `get_audit_event`, `list_audit_facets`, `get_audit_histogram`, `aggregate_audit_events` and `list_audit_fields`.
  Every other caller gets 403 `forbidden_role`, and every read is recorded as `audit.search`.
  A query is KQL, Lucene after a `lucene:` prefix, or a Query DSL body of `bool`, `term`, `terms`, `match`, `match_phrase` and the aggregations the API description lists, over at most 366 days, and the CLI marks the failing part of a query.
  `observe audit search --follow` and the stream tail new events and resume after a dropped connection.
  An MCP answer holds at most 16 KiB and says `truncated: true` with a `next_cursor` when it cut a page.
- The Audit module and the audit datasource.
  Admins get an Audit item in the sidebar with Log, Dashboards and Observations.
  Log searches the trail with a query box that completes field names, facet chips, a volume chart, a detail panel with the other events of the same request and agent run, Live follow and an export as NDJSON or CSV.
  The trail is also a built-in datasource, `slashobserve-audit`, which only Admins can query, so the Logs page, log, table, stat and time series panels and saved searches read it like an OpenSearch datasource.
  At first start the server creates the folder Audit, which only Admins can read, with four starter dashboards: Audit overview, Sign-ins and access, Agent and MCP activity and Data access.
  The starters are read-only and a release updates them, and an Admin duplicates one to change it.
  `observe folders list`, `list_folders` and the folders API mark that folder with `audit`, and the dashboards list takes `exclude_audit`.
- Audit export over OTLP.
  With `SLASHOBSERVE_AUDIT_OTLP_ENDPOINT` set, the server posts each audit event as an OTLP log record to `<endpoint>/v1/logs`, in the order the events committed and at least once, from a position stored in the database.
  A receiver should drop repeats by `slashobserve.audit.id`, and `slashobserve_audit_export_lag_seconds` shows how far the export is behind.
  The server refuses a redirect and a link-local address, and request headers come only from the file in `SLASHOBSERVE_AUDIT_OTLP_HEADERS_FILE`.
- Admins-only datasources and folders.
  An Admin sets a datasource's audience to Everyone or Only Admins on the form, with `observe datasources add --audience` and `edit --audience`, in the datasources file, or over the API.
  Every datasource lookup now checks who is asking, so a person who is not an Admin gets no data, no values, no tail and no public link from an Admins-only datasource.
  Content that reads one can live only in an Only Admins folder, which non-Admins cannot see, its creator included, and is set in the folder's settings, with `PUT /api/v1/folders/{id}/permissions` or with `observe folders permissions admins-only <folder-id> --yes`.
  A save, a move, a share or a public link that would put such content where non-Admins read it is refused with 409 `restricted_content`.
  An answer from a run that read Admin-only data is kept where only Admins read it, and sharing it elsewhere is refused with 409 `restricted_answer`.
- Asks and private chats.
  Every prompt is a turn of a chat that you and Admins can read, and nobody else sees it.
  A private ask about a dashboard or an observation needs only Comment on it, reads data and answers without writing anything shared, and has no per-person or total cap.
  The asker shares the answer into a comment thread, which on an observation also writes it onto the page, with `POST /api/v1/observe/{id}/share`, `share_answer` and `observe share-answer`, and takes it back with `DELETE /api/v1/threads/{thread_id}/share` or `observe comments withdraw`.
  An answer may carry a one-sentence `suggested_change`, which `POST /api/v1/threads/{thread_id}/apply`, `apply_suggested_change`, `observe comments apply` and `applies_finding_id` with `kind` `revise` turn into a run that saves a version, or a proposal for someone who can only comment.
  `GET /api/v1/chats`, `GET /api/v1/chats/{id}`, `DELETE /api/v1/chats/{id}`, `list_chats`, `get_chat` and `observe chats list`, `show`, `pin`, `unpin` and `archive` read and manage chats, and an Admin reads anyone's chat read only.
  A revision on a dashboard joins your latest chat there unless `new_chat` is true, and a resend with the same `client_id` returns the first run.
  The dashboard side panel has Chat and Activity tabs, and Chats and Home list one row for each chat.
  A thread's reply from a run carries the run's answer, so a thread view no longer fetches each one.
- Anchors.
  An anchor names the spot a comment or a question points at: a time, a range, a series, a table row or cell, a bar, a log line, a span or a position on a panel or an observation block.
  `anchor` and `block_id` are accepted by `POST /api/v1/threads`, `POST /api/v1/observe`, `create_comment` and `start_run`, `observe --at` and `observe comments add --at` take it as JSON or as `row:`, `time:`, `range:`, `series:` and `pos:` short forms, and the agent's brief names the spot in one sentence.
  `list_comments` and the comment tools return a thread's `anchor` and `at`, and `observe requests list --anchored` and `list_runs` with `anchored` keep the questions that point at a spot.
- Comment and Ask on the dashboard canvas.
  **Comment** and **Ask** in the tool bar are modes: a click on a spot of a panel, or a drag across its time axis, opens a box there that names the panel, the part you clicked, the value and the time window.
  A comment posts a thread with that anchor and shows a pin on the panel, which opens the thread in place, and a question goes to the agent as a private turn of your chat and shows a pin that only you see.
  Hovering a panel shows Ask and Comment buttons, `c` and `a` act on the panel under the pointer, `Esc` leaves the mode, and the two modes have their own pointers.
  Every panel type with a meaningful target publishes the layout it draws, so a pin finds its spot again, and the boxes can be resized.
- Comment and Ask on observation blocks.
  The observation page has the same tool bar and keys, and selecting text in a text block offers Comment and Ask on the words you selected.
  An ask or an investigation can name a block with `block_id`, over the API, `start_run` and `observe investigate --block`.
  A run asked on a block puts its answer under that block.
- Editing and arranging observation blocks.
  A panel block has **Edit panel**, or **Suggest an edit** for a commenter, which opens the dashboard's panel editor with a preview over the block's window, and **Panel** offers **From a dashboard** and **New query**.
  A panel block is drawn as a dashboard panel with its panel menu, follows the observation's time range unless pinned, and `POST /api/v1/observations/{id}/blocks/preview` runs an unsaved panel under the draft limits.
  Blocks are dragged by a grip, snap to a third, a half, two thirds or the full width, take a height of 120 to 960 pixels on a panel or snapshot block, and move with the arrow keys.
  `arrange_blocks`, `observe o arrange`, `PUT /api/v1/observations/{id}/layout`, `width` and `height` on `add_block` and `update_block`, and `follow_page` do the same without a version, because a layout change never makes a text edit stale.
- Moving and resizing panels.
  A title bar you can arrange shows a grab cursor and a faint grip, a press and a drag selects and moves in one gesture, the selected panel has handles on its right edge, bottom edge and corner, and the drop spot is a dashed outline.
  The Activity or chat pane closes while you arrange, so the layout shows at full width.
- The CLI and MCP reach the same work.
  The server has 110 tools, 11 more than 0.1.3, and `observe chats`, `observe audit`, `observe share-answer`, `observe o arrange` and `observe chats pin` are new.
  `observe chats list` shows a PINNED and a DASHBOARD column, `observe o show` shows the height and the range of each block, and `--exclude-audit` leaves the Audit folder out of `observe dashboards list` and `tags`.

### Changed

- `GET /api/v1/datasources` gives the audit trail the `source` `builtin`, next to `file` and `app`, and lists it only to Admins, so a script that switches on `source` must handle `builtin`.
  The Datasources page labels it Built in and offers no edit or remove.
- Replying to an observation that has a discussion with `create_comment` adds the comment to it, where it answered 409 `observation_thread_exists`, unless `anchor` opens a thread of its own on a spot.
- The status line a run posts in a thread reads `Answered.` and the thread carries the answer, where it said `Answered in the chat of the person who asked.` and linked to the chat.
- `observe dashboards duplicate` and `duplicate_dashboard` put the copy in the dashboard's own folder when you may create there, else in your default folder.
- A dashboard that changed since its last health check is no longer shown as healthy.
  Its row says a recheck is pending and its old problems stay out of the failing counts and the problem filter, and `list_dashboard_health` rows carry `can_edit`, which is false on a starter.
  Repair is not offered on a starter, and health repair answers `provisioned` for one.
- The investigate prompt and `save_finding` tell the agent to leave `suggested_change` out when no change is needed and never to write it into the answer's Markdown.
- Dependencies: Prometheus 0.316.0, kin-openapi 0.153.0, openai-go 3.96.0, testcontainers-go 0.45.0 and golang.org/x/oauth2 0.38.0 on the Go side, 15 web packages including react-query 5.107.1, zustand 5.1.0, lucide-react 1.70.0 and vite 8.3.6, and docker/login-action 4.8.0 in the release workflow (#245, #246, #247).

### Fixed

- Dashboard saves stalled in the browser after a few page loads, because pages kept in the back and forward cache held their event streams open and used up the six connections a browser allows.
- A layout save could finish without its toast, or crash the page when the fitted layout differed from the saved one.
  A delete, a move or a removal that finished after its row had left the page lost its toast, focus move or navigation.
- A dashboard moved out of someone's view took 5 to 6 seconds to say Dashboard not found, a move made while the event stream was closed never reached the page, and the stream sent a move to a person who had lost access to it.
- On a dashboard that shows its defaults, Open in Logs, Open in Traces and Add to observation ignored the dashboard's range and variables, and a panel whose datasource is a variable could not open Logs or Traces.
- The observation page did not show the agent's reply until a reload, Add a panel opened beside the discussion, Add from Logs or Metrics left the person there, and an agent's backticks showed literally in comments and replies.
- A page first opened in a background tab never caught up on what changed while it was hidden.
- In the Activity pane, Enter with the Investigate chip posted a comment where the send button started the run, the filter overflowed on a long panel name, and the PromQL completion popup misplaced itself.
- A stat panel whose series was constantly zero drew a half-height block, and now sits on the baseline.
- A refused `create_observation` left an empty observation behind, and now creates nothing.

## [0.1.3] - 2026-11-20

This release adds features and database migrations despite its patch number.
Migrations 49 to 73 are new, there is no migration 66, and no later migration exists.
They add tables and columns for chats, inbox item states, webhook formats, thread read marks, dashboard viewers and view days, home dashboards, datasources added in the app, people invites and deactivation, teams, chat-only findings, observation pins, run time zones and access requests.
Migration 62 creates the table that holds datasources added in the app, with their sealed credentials.
Migration 65 lets a finding exist without an observation, so a chat answer stays in its chat until someone saves it.
Migrations 59, 62 and 73 replace a check constraint on the activity log, and migrations 50 and 73 rebuild its inbox index with a plain `CREATE INDEX`, which blocks writes to the log until it finishes, so upgrade when little is changing.
Back up Postgres before you upgrade.
The server never runs a migration backwards, so going back to 0.1.2 means restoring the backup you took before upgrading.
The first items under Changed can break a script or an integration.

### Added

- Datasources in the app.
  An Admin adds, edits, tests and removes a datasource on the Datasources page, with `observe datasources add`, `edit`, `test`, `remove` and `recheck`, without a file edit or a restart.
  Credentials are write-only and stored sealed with `SLASHOBSERVE_SECRET_KEY`.
  A datasource that a saved dashboard or saved search uses cannot be removed, and the address goes through the outbound allowlist.
  Entries of the datasources file stay read-only and win over an app datasource with the same id.
  MCP and the built-in agent list datasources with their health and can call `recheck_datasource`, and neither adds, changes or removes one.
- Invites, deactivation and teams.
  An Admin invites people by email with a role and copies the sign-in link, because no email is sent.
  An Admin deactivates a person, which ends their sessions and disconnects their agents, reactivates them, or removes them, and deletes a service account.
  A team is a named group of people that a folder access list can name like an identity-provider group.
  `observe people` and `observe teams` do the same with an Admin key, and no MCP tool manages people.
- The Audit log.
  Admin changes to roles, sessions, service accounts and keys, webhooks, outbound domains, agent connections, Grafana connections, datasources, people and public links go into the activity log and show in Settings as sentences, with filters by person, kind and time.
  An Admin key reads the same record with `GET /api/v1/activity?scope=admin`, `observe activity --admin` and `list_activity`, and any other key gets `forbidden_role`.
- Requests for comment access.
  A Viewer of a dashboard or an observation asks for Comment access with an optional note, from the activity pane, the discussion, `POST /api/v1/access-requests`, `observe access request` or the MCP tool `request_comment_access`.
  The people who manage the folder get an inbox item, or the Admins do when nobody manages it, and marking the item done declines the request.
  `GET /api/v1/access-requests`, `observe access requests` and `list_access_requests` list the open requests on folders the caller manages.
- A Metrics page.
  `/explore/metrics` runs PromQL on a Prometheus datasource, asks the agent for a query, browses metric names, labels and values, saves searches, and adds the result to a dashboard or an observation.
  `observe metrics query` and `draft_query` do the same from a terminal, and a saved search can have the kind `metrics`.
- Panel layout editing.
  The panel menu duplicates and removes a panel, the dashboard menu adds a panel, opens a blank dashboard and edits the Spec JSON for variables, links and time settings, and the panel editor shows the options of the panel type.
  A query that returns no data can be saved as Unverified from the web app.
  `POST /api/v1/dashboards/{id}/panels` and `observe dashboards panels` apply a panel patch.
- Annotations from the dashboard.
  A person adds an annotation with a button or by Cmd or Ctrl click on a chart, and the creator or an Admin edits or deletes it.
  `PATCH /api/v1/annotations/{id}`, `observe annotations update` and the MCP tool `update_annotation` change its text and tags.
- Dashboard views and the home dashboard.
  The dashboards list and header show how many people opened a dashboard in the last 30 days, the list sorts by name or by most viewed, and a bar moves several selected dashboards to a folder.
  A person picks a home dashboard that opens after sign-in, and `get_home_dashboard`, `set_home_dashboard` and `clear_home_dashboard` do it over MCP.
  A dashboard prints, saves as a PDF or downloads as one image from the browser.
- Mentions, deletion and a triaged inbox.
  `@` opens the mention picker in every comment box, a person deletes a comment, and nobody edits an agent's reply.
  The inbox names who did what, marks items done, filters by Open, Unread and Done, and groups by dashboard or observation.
  `list_mentionable_people`, `list_dashboard_viewers`, `list_inbox` with a `filter`, `set_inbox_item_done` and `observe comments delete` cover the same work for agents.
- Observations grow up.
  An observation is deleted with `observe o delete --yes` or from its page, and the list searches by title, sorts, pages and resolves or deletes several at once.
  An editor withdraws a published RCA revision, moves blocks up and down, pins an observation to a dashboard as an annotation, and makes a dashboard from its panels.
  A panel block keeps the dashboard variables it runs with.
  The MCP tools `pin_observation` and `make_dashboard` and the commands `observe o pin`, `dashboard`, `remove-block` and `withdraw` match.
- Chats as runs.
  A chat has a Save as observation button, Copy answer and Ask again, a message counter, and one row per conversation in Home, Ask and Chats.
  The MCP tools `list_runs`, `rename_run`, `pin_run`, `unpin_run` and `archive_run` manage a person's own chats, and `save_run_answer`, `start_run`, `cancel_run`, `retry_run` and `get_run_build` start, stop and read runs.
  `observe save-answer`, `observe --follows <run id>` and `observe requests archive <id> --delete-observation --yes` do the same from a shell.
  Deleting a chat in the web app offers to delete the observation saved from it.
  A chat and the observation made from it share one title.
- Logs and traces.
  Each source on the Logs and Traces lists shows its services and a filter box, from `GET /api/v1/datasources/{uid}/services`, `observe datasources services` and the MCP tool `list_services`.
  The ClickHouse trace search has an SQL view over `otel_traces`, and `observe traces --query` runs it.
  A trace search or a trace adds to a dashboard or an observation, and `observe traces` lists whole traces sorted with `--sort`.
  A saved search updates its query, columns and range with **Update saved search**, `PATCH /api/v1/saved-searches/{id}`, `observe saved-searches update` and `update_saved_search`.
  A Kibana Discover link pasted into the Ask box imports like one pasted into the query box.
- Trace SQL rows as written.
  The explore query request and the MCP tool `query_instant` take `as_written` together with `page_size` on a ClickHouse datasource, and return the rows as the SQL wrote them, with its own `ORDER BY`, `LIMIT` and columns and no `TimeNs` column.
  With it the query must read its time range through `$__timeFilter`, even when it selects `Timestamp`, because the server no longer cuts the rows to the range for you.
  Without it, a query that selects `Timestamp` still comes back as log lines, newest first, and `as_written` without `page_size` is refused.
  The logs page does not send it.
  The trace SQL view and `observe traces --query` send it, so a query for the three slowest spans returns those three.
- The CLI reaches what the web app does.
  `observe logs` prints lines from Elasticsearch and OpenSearch and rows from ClickHouse SQL.
  `observe folders permissions`, `observe dashboards schema` and `sample`, `observe datasources schema` and `sample`, `observe import --update` and `--as-new`, and `--all` on list commands are new.
  Every list command prints its total and checks `--limit` before it sends a request.
  With `--json` an error is a JSON object on standard error with a stable code.
- Webhooks for Slack and Teams.
  A webhook has a format, generic, Slack or Teams, and an Admin can skip the one event a failing webhook is stuck on.
- Import fidelity.
  A Grafana import can update the dashboard an earlier import made, a Spec exported from SlashObserve imports on the import page, and the report names annotation queries, alert rules and library panels that are not imported.
- Time zones.
  The time range picker has a calendar and a time zone box carried by the `tz` URL parameter, **Save as default** stores the range in the Spec, and Settings has a default time zone.
  The built-in agent writes times in the zone the page shows.
- Panes and first load.
  Share, Activity, Chat and Dashboard JSON take focus on their heading when opened and return it to their button on Escape.
  The server serves script and style files brotli-compressed to browsers that accept it.
- More MCP tools.
  The server has 99 tools, 27 more than 0.1.2, among them `create_folder`, `rename_folder` and `run_dashboard_health` for an Admin.
  `create_dashboard` and `save_search` are safe to retry: the same caller with the same content gets the first result back with `replayed: true`.
  `tools/list` hides the tools that need a person from a service-account key.
- Query results say what ran.
  A panel data response and the MCP results `query_instant` and `query_range` carry the query text after variables and macros as `meta.queries[].executed`, and a public link never shows it.
  Results carry `meta.partial` and the warning text when Prometheus warns, Tempo or Loki stops at its time limit, or a series cap drops series.
- The built-in agent.
  It shows a short sentence of live text while it works, which `observe status` prints and `get_run` returns as `live_text`, and which is not kept once the run ends.
  It builds a new dashboard in a folder you name, reads earlier observations as evidence, reads annotations in every run as the person who asked, and says which datasources it could not check.
  It writes text panels in Markdown.

### Changed

- A plain question asked in a chat no longer saves an observation.
  The answer stays in the chat, readable by the person who asked and by Admins, until they choose **Save as observation**, call `save_run_answer` or run `observe save-answer`.
  `/investigate` still opens an observation, and `get_run` returns an unsaved answer with a null `observation_id`.
- `result_kind` is `answer` for a run whose answer was never saved, in `GET /api/v1/observe`, `get_run`, `list_runs` and `observe requests list --json`.
  A script that switches on `result_kind` must handle `answer`, and `observation_id` is null until the answer is saved.
- The routes under `/api/v1/findings` still answer in 0.1.3, each with a `Deprecation` header, and a later release removes them.
  The 0.1.2 notes said the next release would remove them.
  Use the observation routes and `list_observations` instead.
- A dashboard that the caller cannot see answers 404 `not_found` on every route, where some routes answered 403.
  A visible one with too low a level still answers 403, and the body of every `forbidden_role` refusal now names `needed_access` and `folder_title` or `needed_role`.
- A list route answers 400 `invalid_request` for a parameter it does not take or a value it cannot read, and names the parameter.
  Other routes still serve a request with an unknown query parameter and add the header `X-Unknown-Query-Parameters`, and `SLASHOBSERVE_API_CONTRACT_CHECK` refuses it instead.
- `SLASHOBSERVE_ADMIN_EMAILS` grants Admin only at a sign-in where the provider reports `email_verified`.
  People who already hold Admin keep it, and the startup reconcile never grants it.
- A link in a Spec may use only http, https or mailto, or be relative, and a save with another scheme fails with the issue `unsafe_link`.
  A stored version that already holds one fails validation when it is restored.
- `observe dashboards apply` needs `--dir`, and a file argument is refused.
  A prompt that is one bare word, or whose first word is within one or two edits of a command name, exits 2 before any request, where it used to start a paid run.
  Quote the words as one argument to send them.
- Every MCP tool accepts one alias for a documented name, such as `datasource` for `datasource_uid`, and refuses a call that gives both with different values.
  A success result no longer carries `"code":"ok"`, and a failed call carries `code` and `message` in one shape.
  The tools that overwrite or discard what someone saved are advertised as destructive.
- `accept_empty` on a panel save is for signed-in people, and a key, the CLI, MCP and the built-in agent get 403 `user_session_required`.
  `observe add --panel-file` and the HTTP route behind it run the panel check of `add_block` and exit non-zero for a broken query.
- ClickHouse SQL run through `query_instant`, `query_range` or `observe logs` without `page_size` must contain the time range macro `$__timeFilter(Timestamp)`, and a macro inside an SQL comment no longer counts.
- A paged ClickHouse result that is not read as log lines no longer carries the `TimeNs` field, which a result gets only when it is ordered by time.
- A Prometheus range query that sets no `interval` never runs at a step shorter than the datasource's `scrapeInterval`, 15 seconds by default, so a panel's newest point can be up to 15 seconds old.
  The explore query API defaults `max_data_points` to 200.
- A log volume over Elasticsearch, OpenSearch and ClickHouse counts exactly the requested range, so the first and last bars hold partial buckets.
  A Loki volume over a week and a broad Tempo search return a partial result with `meta.partial` and, for Loki, `counted_from`, instead of timing out.
- A Loki variable set to All expands to `.+` inside a stream selector, because Loki refuses a selector whose matchers all match the empty value.
- Elasticsearch and OpenSearch Query DSL that carries a script is refused, except the script of `bucket_script` and `moving_fn`.
  A ClickHouse trace filter with unbalanced parentheses or a trailing comment is refused.
- `draft_query` allows 60 drafts per caller per clock hour, `import_grafana_dashboard` refuses a conversion that would add an HTML text panel, a placeholder panel or a panel action, and a run token stops working when its grant is revoked.
- The Helm chart keeps `/metrics` out of the Ingress, and `ingress.exposeMetrics: true` restores it.
  The server answers 404 to a percent-encoded spelling of `/metrics`.
- A title, name, comment, prompt or annotation that holds only spaces or invisible characters, or text that flips the direction of its neighbours, is refused with a reason such as `text_invisible_only` or `text_bidi_override`.
- A time series or trend panel draws at most 50 series per query, the 50 with the highest peaks, and its header says how many there are.
- The demo stack gives Loki and Tempo 1 GB of memory instead of 512 MB and caps their Go heap at 768 MiB.

### Fixed

- An agent connected over OAuth was refused with 401 at `/mcp`, because the built-in agent's token shared the prefix `soa_` with OAuth access tokens, and that token now starts with `sob_`.
- A deleted agent reply came back about three seconds later as a new message.
- A follow-up in a chat lost the earlier turns, a stopped run could leave a chat on Running, and Try again opened a second chat.
- A Kibana Discover link lost its time range, columns and filters on the logs page, and an exclude pill on a Loki label that the query selects with a regex replaced the matcher and was dropped.
- The built-in agent reported a read that timed out as no data, said a metric was missing after searching only by name, and wrote times in UTC under a page that showed another zone.
  It now names each datasource it could not check, reads deploy markers in every run, and uses the page's zone.
- `observe cancel` and `observe continue` dropped the error code when someone other than the requester was refused, and now end the message with it.
- The span count of a Tempo trace stats read was wrong when a sample was NaN.
- A Prometheus answer with warnings was not marked partial, and ClickHouse integers above 2^53 lost digits in a table.
- A live tail that polled a partial result did not fetch the lines it missed.
- Two sessions moving one panel to different places both saved, concurrent creates could pass the public link cap, and an RCA published after the title changed while panels froze.
- The dashboard event stream kept sending to a person whose role was lowered.
- A Grafana import dropped more than the one query that used an unsupported variable format and did not report skipped entries, and the Spec import refused a Spec that SlashObserve exported.
- The Metrics table reported 0 rows for an instant query.
- Focus fell to the page body after opening a trace, closing a dashboard pane or navigating, and the trace ruler showed the window's edges as ticks after zooming.
- A time series drew one straight line across a gap with no samples.
- A mention typed straight through, such as `@observe add a stat panel`, lost the text after the `@`.

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

[0.1.4]: https://github.com/slashobserve/slashobserve/releases/tag/v0.1.4
[0.1.3]: https://github.com/slashobserve/slashobserve/releases/tag/v0.1.3
[0.1.2]: https://github.com/slashobserve/slashobserve/releases/tag/v0.1.2
[0.1.1]: https://github.com/slashobserve/slashobserve/releases/tag/v0.1.1
[0.1.0]: https://github.com/slashobserve/slashobserve/releases/tag/v0.1.0
