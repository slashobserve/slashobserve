---
title: MCP
description: Connect Claude Code, Claude Desktop, Cursor, OpenCode and other MCP clients to the server, make a first tool call, work with dashboards, comments and observations, and look up every tool, argument and error.
---

SlashObserve speaks the Model Context Protocol (MCP), so an agent such as Claude Code or Claude Desktop can build dashboards, read and answer comments, investigate a question and keep what it finds, using its own model.
The agent acts as the person who connected it, with that person's role and folder access.

The server exposes its tools over Streamable HTTP at `<public url>/mcp`.
`GET /api/v1/instance` returns the exact address as `mcp_url` next to `public_url`; it needs a signed-in session or a key.
Every request carries `Authorization: Bearer <token>`.
The token is either a person's OAuth access token, which [Connect your own agent](#connect-your-own-agent) sets up, or a [service-account key](../operate/service-account-keys.md) for a headless bot, which [Connect a bot with a service-account key](#connect-a-bot-with-a-service-account-key) sets up.

## Connect your own agent

Add `<public url>/mcp` to an MCP client, approve it once in your browser, and the client acts as you.
The steps below use Claude Code and a server at `https://observe.example.com`.
For the demo stack, use `http://localhost:58080/mcp`.
The **Connect an agent** button on the [Agents page](agents.md) shows the Claude Code command with your address filled in.

1. Add the server to Claude Code:

   ```sh
   claude mcp add --transport http slashobserve https://observe.example.com/mcp
   ```

2. Inside Claude Code, run `/mcp` and choose **Authenticate** for `slashobserve`, or run `claude mcp login slashobserve` in your shell.
   Claude Code opens your browser.
3. Sign in through your identity provider, as for the web app.
   The consent page names the client, shows your name, email and role, shows the host the browser returns to, and asks you to approve or deny.
   When the client runs on your own computer, the page warns that any program on this computer can ask under any name, so approve only a connection you just started.
4. Choose **Approve**.
   Claude Code now lists the server's tools.

### Connect VS Code

Add this to `.vscode/mcp.json`, choose Start on the server, and allow the sign-in:

```json
{
  "servers": {
    "slashobserve": {
      "type": "http",
      "url": "https://observe.example.com/mcp"
    }
  }
}
```

### Connect Cursor

Add a `url`-only entry to `.cursor/mcp.json` and sign in when Cursor asks:

```json
{
  "mcpServers": {
    "slashobserve": {
      "url": "https://observe.example.com/mcp"
    }
  }
}
```

### Connect Claude Desktop or claude.ai

Add the server from the connector settings, entering `<public url>/mcp` as the remote MCP server URL.
These clients reach the server from Anthropic's servers, so `SLASHOBSERVE_PUBLIC_URL` must be an `https` address that is reachable from the internet.
A server on `localhost` cannot be added.

### What an approved connection can do

After you approve, the client acts with your role and your folder access, re-read on every call, so a role change takes effect at once.
Everything it writes shows as `<your name> (via <client>)`.
The client chooses its own name and SlashObserve cannot verify it.
The name is a label and never changes what the client may do.

The connection has these limits:

- An access token lasts 1 hour.
- A refresh token lasts 30 days from when it was issued and is replaced each time it is used, so a client that stays in use keeps working.
- A connection lasts at most 90 days, after which you approve it again.
- You can have at most 20 connected agents, and approving a client you already connected replaces that connection.
- An approval the client has not yet finished lasts 60 seconds and counts toward the 20.

### Disconnect an agent

In the web app, open Settings, Connected agents.
An Admin sees and disconnects everyone's connections in Settings, Agent access.
The [Agents page](agents.md) lists the same connections with their mode and presence.

Disconnecting a client also ends a run it already claimed: the run's token stops working at once, and the run ends when its lease lapses.
In Claude Code, `claude mcp logout slashobserve` clears only the credentials Claude Code stored, so disconnect in Settings too.

Over the HTTP API, any signed-in person lists and disconnects their own agents with `GET /api/v1/me/grants` and `DELETE /api/v1/me/grants/{id}`.
An Admin lists and disconnects everyone's with `GET /api/v1/grants` and `DELETE /api/v1/grants/{id}`.
Each connected agent in the list has a `state`, always `active` there because the list leaves out revoked and expired connections; see [States](../reference/states.md).
Service-account keys cannot call any of these routes.

## Make your first tool call

Once connected, ask the client in plain words, for example "Which datasources does SlashObserve have?".
The client picks the tool and sends a `tools/call` request with the tool name and its arguments:

```json
{ "name": "list_datasources", "arguments": {} }
```

The result has one entry for each datasource, with its `uid`, `type`, `name` and `healthy` flag.
On the demo stack the list starts like this:

```json
{
  "datasources": [
    { "uid": "prom", "type": "prometheus", "name": "Prometheus", "healthy": true },
    { "uid": "loki", "type": "loki", "name": "Loki", "healthy": true }
  ]
}
```

Use a `uid` from this list as the `datasource` argument of the other tools.
From here, try `list_dashboards` to see what exists, or ask the client to build a dashboard; [Build and change a dashboard](#build-and-change-a-dashboard) shows the order it follows.

Two things help when a call fails.
Every refused call returns a `code` you can branch on and a `message` that says what to change; [Errors](#errors) lists them.
A call with a field the tool does not take is refused with `invalid_request` and the list of fields it takes, so a wrong guess costs one call.

## Connect a bot with a service-account key

A service-account key suits a headless bot that has no person to approve it.
Its writes show as the service account, and its role decides what it may do.
Create the key as [Service-account keys](../operate/service-account-keys.md) describes.

Export the key in the shell that starts your client, so the key never lands in a config file:

```sh
export OBSERVE_TOKEN=<your service-account key>
```

### Claude Code with a key

Add this to `.mcp.json` in your project, with your server's address:

```json
{
  "mcpServers": {
    "slashobserve": {
      "type": "http",
      "url": "https://observe.example.com/mcp",
      "headers": { "Authorization": "Bearer ${OBSERVE_TOKEN}" }
    }
  }
}
```

Claude Code expands `${OBSERVE_TOKEN}` from its environment when it starts.
`.mcp.json` is the documented place for that expansion.

`claude mcp add` can create the same entry.
Use single quotes so your shell does not expand the variable and write the key into the config:

```sh
claude mcp add --transport http --scope project slashobserve https://observe.example.com/mcp --header 'Authorization: Bearer ${OBSERVE_TOKEN}'
```

Prefer `.mcp.json`.

The **API keys** page's Claude Code snippet also appends one line to your project's `CLAUDE.md`, only if the file does not already contain `For SlashObserve dashboards`.
The line tells Claude Code to read open comments with `list_comments`, fix them with `update_dashboard`, reply to each with `reply_to_comment` and resolve it with `resolve_comment`, passing the version that fixed it.

### OpenCode with a key

Add this to `opencode.json`:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "slashobserve": {
      "type": "remote",
      "url": "https://observe.example.com/mcp",
      "enabled": true,
      "oauth": false,
      "headers": { "Authorization": "Bearer {env:OBSERVE_TOKEN}" }
    }
  }
}
```

`"oauth": false` stops OpenCode from trying an OAuth flow; SlashObserve takes only the bearer key on this route.

### Cursor with a key

Cursor reads remote MCP servers from `.cursor/mcp.json` (per project) or `~/.cursor/mcp.json`.
This is a generic example that the project has not tested with Cursor.
The `url` and `headers` fields apply to any client that supports remote MCP servers with custom headers.
Cursor defines how it expands environment variables inside `headers`, so check its current MCP documentation before relying on the `${env:OBSERVE_TOKEN}` form below:

```json
{
  "mcpServers": {
    "slashobserve": {
      "url": "https://observe.example.com/mcp",
      "headers": { "Authorization": "Bearer ${env:OBSERVE_TOKEN}" }
    }
  }
}
```

If your Cursor version cannot expand variables there, put the key in the file only for a user-level `~/.cursor/mcp.json` that is never committed.

## The /observe prompt

The server offers one MCP prompt, `observe`, with two optional arguments, `ask` and `kind`.

- Without `kind`, it returns the guide that outside agents follow to build and change dashboards, investigate a question, search logs and traces, answer comments, keep observations, review proposals and take assigned runs.
  It appends the `ask`, what you want, as the last section.
- With `kind` set to `build` or `investigate`, it returns the system text SlashObserve's own agent works from for that kind of run.
  An agent that claims runs reads it before it starts one.
  The `ask` is not used.
- Any other `kind` is refused.

Any key or OAuth connection that can call `/mcp` can list and get the prompt, and the text holds nothing secret.

How a client shows a prompt is up to the client; in Claude Code it appears as an `mcp__` command under the server's name.
To get the same guide as `/observe` in Claude Code and other agents that read Agent Skills, run `observe skill install`; see [CLI](cli.md#agent-skill).

## Build and change a dashboard

On `initialize`, the server sends instructions with the order of work, and a client follows it:

1. Call `get_spec_guide` once before the first save.
   It returns the authoring guide, the JSON Schema and an example.
2. Take a datasource `uid` from `list_datasources`, then find real names with `list_metrics`, `list_labels`, `list_label_values`, `get_schema` and `sample_rows`.
3. Check each query with `lint_query`, then run it with `query_range`, or with `query_instant` for an instant query or a table.
4. Save a new dashboard with `create_dashboard`.
   `list_folders` gives a `folder_id` to put it in.

To change a dashboard, read it with `get_dashboard`, or only the panels you need with `get_panels`.
Then send just the change with `update_panels`, passing the `parent_version` you read.
This call changes only the default time range:

```json
{
  "dashboard_id": "<dashboard id>",
  "parent_version": 3,
  "dashboard": { "time": { "from": "now-6h", "to": "now" } },
  "summary": "Default to the last 6 hours"
}
```

`update_dashboard` rewrites the whole document instead, so use it only when most of the dashboard changes.

Every save runs the panel queries first and rejects a panel whose query fails or returns no data, as [What a save checks](../concepts/agent.md#what-a-save-checks) describes.
A save on an older version merges when it changed other panels, and answers `version_conflict` when it changed the same ones; read the latest version and apply your change again.
When the caller can comment but not edit, the save becomes a [proposal](../concepts/proposals.md) and the answer has the code `proposed`.

No tool deletes a dashboard.
People do that in the web app, through the HTTP API or with `observe dashboards delete`.

Related tasks:

- To read history, use `list_dashboard_versions`, `diff_dashboard_versions` and `restore_dashboard_version`.
- To move or copy a dashboard, use `move_dashboard` and `duplicate_dashboard`.
- To bring in a Grafana dashboard, use `import_grafana_dashboard`; see [Import Grafana dashboards](import-grafana.md).
- To repair an imported dashboard, read it with `get_dashboard`, fix the panels and variables in its `unverified` list with `update_panels`, and leave placeholder panels to people.
  The web repair queue does the same through `POST /api/v1/repairs`.
- To see who else has a dashboard open, use `list_dashboard_viewers`.
- To find broken dashboards, use `list_dashboard_health`, read the panel with `get_panels` and save the fix with `update_panels`.

## Find the right names and check a query

Discovery tools read what a datasource holds, so a query uses real names.
Each datasource type answers a different set; [Discovery by datasource type](#discovery-by-datasource-type) lists them.

1. `list_datasources` shows the datasources and whether each is healthy.
   `recheck_datasource` runs a fresh health check on one.
2. `list_metrics`, `list_labels`, `list_label_values`, `get_schema` and `sample_rows` list names, labels, tables and sample rows.
3. `list_services` finds which log, trace or metric datasources hold a service.
4. `lint_query` checks a query without running it.
5. `query_range` runs a query over a time range, and `query_instant` runs one at a point in time or returns a table.
6. `draft_query` turns a question into a checked query, using the server's own model.
7. `show_panel` draws a query, or a panel of a saved dashboard, as a chart in a host that supports [MCP Apps](#charts-in-mcp-apps-hosts).

After a scoped `list_metrics`, saves tell you which metric families no panel uses yet, as [Save results report what is left uncovered](#save-results-report-what-is-left-uncovered) explains.

## Review a change with proposals and comments

When an agent or person who can only comment changes a dashboard, the change waits as a [proposal](../concepts/proposals.md).
An Editor reviews it:

1. `list_proposals` shows the open proposals of a dashboard.
2. `get_proposal` returns the proposal with its changes, the conflicts a merge would hit and the field-by-field diff.
3. `merge_proposal` saves it as a new version, or `discard_proposal` closes it.

Comments work the same way over MCP as in the web app.
Open comments on a dashboard can be read with `list_comments`, answered with `reply_to_comment` and resolved with `resolve_comment`, passing the version that fixed the comment.
A dashboard that Claude Code built can then be reviewed by the team without a model on the server.
`create_comment` opens a thread, and a comment can ask an agent to answer with `to: agent`; [Comments](#comment-tools) has the rules.

## Investigate and keep what you learn

An observation is a page of blocks about one question: Markdown text, panels that run live, and snapshots that freeze a panel's data.
No tool queues an investigation on the server.
An MCP client investigates with its own model, using the discovery and query tools, and saves the answer into an observation:

1. `create_observation` makes the page, with up to 10 blocks.
2. `add_block` adds text, a live panel or a snapshot, and `update_block` changes one.
3. `resolve_observation` closes it with a one-line summary.
4. `publish_rca` publishes it as the next immutable RCA revision; see [Observations](observations.md#publish-as-rca).

`pin_observation` pins an observation to a dashboard as an annotation, and `make_dashboard` turns its live panels into a dashboard.

## Search logs and traces

Over MCP, `log_context`, `log_patterns`, `log_facets` and `logs_for_trace` read logs, and `get_trace`, `trace_stats`, `trace_error_origins`, `trace_slow_path` and `compare_traces` read traces.
`query_instant` runs your own query, including SQL over `otel_traces` on ClickHouse.
`save_search` saves a search so people can open it from the logs, traces or metrics page.
[Logs and traces](logs-and-traces.md) describes the same features in the web app.

## Organize and find your work

- Folders: `list_folders`, `create_folder` and `rename_folder`.
- Pins: `pin_dashboard`, `unpin_dashboard` and `list_pinned_dashboards` manage the dashboards in a person's sidebar.
- Home dashboard: `get_home_dashboard`, `set_home_dashboard` and `clear_home_dashboard`.
- Search: `search_workspace` runs the workspace [search](search.md).
- Saved searches: `save_search`, `list_saved_searches` and `update_saved_search`.
- Annotations: `list_annotations`, `create_annotation` and `update_annotation`; see [annotations](annotations.md).
- Activity: `list_activity` reads the [activity](activity.md) log.
- Inbox: `list_inbox` and `set_inbox_item_done` read and clear a person's [inbox](inbox.md).
- Access: `request_comment_access` asks for comment access and `list_access_requests` lists open requests.
- Audit trail: six Admin-only tools read the [audit trail](../reference/audit-queries.md).

## Ask SlashObserve's own agent

An MCP client can hand a question to the built-in agent, as the person who connected it.
`start_run` queues a run and answers at once, and `get_run` follows it.
`cancel_run` and `retry_run` control it, `list_runs` and `list_chats` find earlier work, and `save_run_answer` and `share_answer` keep or share an answer.
The built-in agent needs a model provider on the server; without one, `start_run` answers `agent_unavailable`.

## Take runs as an agent

An outside agent can do the runs that people hand to it, with its own model and under the guards of SlashObserve's own agent.
A run reaches it when a comment mentions it and starts a run: `@[Name](grant:<id>)` for a connected agent and `@[Name](service_account:<id>)` for a service account (see [Give an agent a run](agents.md#give-an-agent-a-run)).
The run waits in the queue, assigned to that one agent, and only that agent can claim it.
Only a mention assigns a run to an outside agent, and the built-in agent never takes one.

Two kinds of credential can claim:

- A connected agent, signed in over OAuth, claims the runs assigned to its own connection.
  It works only for the person who connected it, and the run's writes carry that connection's `direct` or `propose` mode.
- A service account claims the runs assigned to it with its key.
  An Admin first turns on **Takes agent runs** for it in the Create key panel, sets `takes_runs` through the API, or sets `takesRuns: true` in the [service-accounts file](../operate/service-account-keys.md).
  Its runs always use the direct mode.
  With **Takes agent runs** off, or `takes_runs` false, `claim_run` answers `not_a_worker`.

The claim tools are `claim_run`, `report_run`, `finish_run` and `get_run_brief`; MCP lists them like any other tool.
Connect as in [Connect your own agent](#connect-your-own-agent) or [Connect a bot with a service-account key](#connect-a-bot-with-a-service-account-key), and poll with `claim_run` while the agent is idle.
An agent that polls shows as `listening` on the [Agents page](agents.md), and as `working` while it holds a run.

The loop of an agent that takes runs:

1. Call `claim_run`, which waits up to `wait_seconds` (0 to 30, 30 by default) and answers `run: null` when nothing is waiting.
   Call it again.
   An agent may keep 2 waiting `claim_run` calls at a time.
   Past that, and when `wait_seconds` is 0, the answer is `run: null` with `retry_after_seconds` of 5, so wait that long before the next call.
2. Read the claim answer.
   Next to `run` it has these top-level fields:
   - `lease_seconds` (60).
   - `session`, with a `url`, a `token` (starting `sot_`) and `expires_at`.
   - `system_prompt`, the prompt to read with the run's `kind`: its `name` (`observe`) and `arguments`.

      The `run` holds these fields:
   - `id`, `attempt`, `kind` (`build` or `investigate`), `intent` (`ask`, `apply` or `suggest`) and `prompt`.
   - Its target: `dashboard_id`, `base_version`, `panel_id`, `from` and `to`, each null when the run has none.
   - `continue_note`, and `requested_by`, the person it acts for, with their `kind`, `id` and `name`.
   - `thread_context`, the earlier messages of the thread oldest first, when a message in a comment thread started the run; read it before you answer.
     It is `null` for any other run.
   - `anchor` and `block_id`, when the question points at a spot on a panel or an observation block.
     `anchor` is shaped as `start_run` takes it.
     `block_id` is the observation block the anchor sits on, or alone the whole block the question is about.
     Both are `null` for a run that points at nothing.
     `get_run_brief` names the same spot in one sentence.

   The server answers in the thread when the run ends, so do not reply there yourself.
3. Read the `observe` prompt with that `kind`; it is the system text SlashObserve's own agent works from for that kind of run.
   `get_run_brief` returns the same text with the first message SlashObserve's own agent starts each model session with, and is optional.
4. Open a second MCP connection to the session `url` with the token as the Bearer, and work with the tools it lists.
5. Call `report_run` at least every 30 seconds with the `run_id` and `attempt`.
   The lease ends 60 seconds after the last call, and `stop: true` in the answer means the run was canceled or has ended.
   `note`, one line of at most 500 characters, shows in the run's steps.
   When the server has no run time limit, the session's `expires_at` is a minute out and every `report_run` moves it a minute on, so a run lasts as long as it keeps reporting.
6. Call `finish_run` with `outcome` `succeeded` or `failed`.

A claim tool called with a run that is not yours at that attempt answers `not_your_run`.

The session lists the tools SlashObserve's own agent has for that kind of run, with the same text, and none above the requester's role.
It lists none of the 22 tools that no run has, such as the claim tools, `start_run` and the comment writes; [the agent](../concepts/agent.md#tools-in-a-run) lists them with their reasons.
It acts for the person who asked and records every call in the run's steps.
Its prompt list is empty, because the `observe` prompt is read on the first connection.

The token stops working when the attempt changes, when the run is canceled or ends, when the lease lapses, or when the run passes its time limit.
A report that arrives after the time limit ends the run as failed.
A run that saved a dashboard version, a [proposal](../concepts/proposals.md) or a finding ends as succeeded whatever `finish_run` says, and one that saved nothing ends as failed.
A run that no outside agent took within 10 minutes fails as `agent_unavailable`.

### The run session enforces the guards

The server enforces the guards of [SlashObserve's own agent](../concepts/agent.md#guards) on every run session, whatever agent works in it:

- The save-now window: after 6 turns without a save, 12 in a suggest run and `SLASHOBSERVE_AGENT_INVESTIGATION_CALLS` (by default the task type's limit, 12 to 30, and 12 in a create run) in an investigate run and in a create run until its first panel is saved, only a save runs and a read in a turn without a save is refused; the call before the limit gets a note.
- Identical reads: a third identical read is refused, and an errored read or a query result runs again.
- One save per turn: a second save in one turn is refused.
- Repeated saves: a save identical to an earlier one is refused.
- Rejected saves in a row: the third ends the run as `validation_exhausted`.
- Saves refused with `restricted_answer` in a row: the third ends the run as `restricted_answer`, and they do not count as rejected saves.
- Stalled turns: two turns in a row in which every call was refused end the run as `no_progress`.
- The hypothesis ledger: with `SLASHOBSERVE_AGENT_INVESTIGATION_LOOP` on, an investigate run has `note_hypotheses`, which a connected agent gets only in an investigate run with a run token and `get_run_brief` gives it the prompt section too, never outside a run; its calls are neither reads nor writes, so they are not refused at the call limit, and a turn with only ledger calls counts as stalled.
  The stages, the stage line after each turn and the call limit that grow with progress (`SLASHOBSERVE_AGENT_INVESTIGATION_CALLS_MAX`) belong to the built-in agent only; a connected agent never sees a stage line and keeps the fixed limit.
- The grounding check: with `SLASHOBSERVE_AGENT_FINDING_GROUNDING` set to `report` or `enforce`, a finding saved in a run is checked against what the run read, for a connected agent as for the built-in one; the measurement, trace, `log_facets` and `log_patterns` reads then carry fact ids in `facts`, `save_finding` cites them as `{{f3}}` and lists `answers`, and the server writes the values into the text before it is stored.
  With `enforce`, `save_finding` is sent back once with the figures and names that were not found, the next call may read past the call limit, and the second save is accepted; the send-back is not a rejected save.
  Each entry of an investigation's `causes` is then scored from the evidence, and `save_finding` answers with each cause's `confidence`, `band` and `stated`; see [Causes and confidence](../concepts/investigate.md#causes-and-confidence).
- Task types: an investigate run whose question the server sorted into a [task type](../concepts/investigate.md#task-types) lists only that type's tools, for a connected agent as for the built-in one, and a call to another tool answers `unknown_tool` with a line naming the run's tools; `list_more_tools`, offered only in such a run, offers every investigation tool for the rest of the run, so list the tools again after it.
- Churn: a save that changes no panel, or changes a panel for the fifth time, gets a note, and the fourth such save in a row ends the run as `no_progress`.
- The cost limit: when `SLASHOBSERVE_AGENT_RUN_MAX_COST_USD` is set and `report_run` or `finish_run` reports `usage` whose cost reaches it, the answer has `stop: true` and the run ends as `budget_exhausted`.
  Without `usage` the server cannot know the cost and never stops a run for it.
- Conflicts: a save that finds a newer version first rebases the build onto it when the panels do not overlap, and otherwise the run restarts once from the latest version; a second conflict ends the run as `conflict`.

Every call may carry `_meta["slashobserve.com/turn"]` with `{"id", "calls", "saves"}`: the id of one model turn, how many calls the turn made and how many of them save.
The server treats the calls with one id as one turn.
Without the key, each call is its own turn, so the stalled-turn and one-save-per-turn guards judge one call at a time.
A key the server cannot trust (an empty or over-long id, more than 64 calls, or more saves than calls) is ignored.
The key may also carry `usage`, the same cumulative object as `report_run`, counting the model call that made the turn, so a run the server ends on the result of that turn stores the usage including it.
The cost limit is checked only on `report_run` and `finish_run`.

Every result of a run session carries `_meta["slashobserve.com/run"]` with `{"settled", "build"}`.
`settled` is `saved`, `proposed`, `finding`, `suggested`, `restarted`, `ended` or `null`:

- `saved`, `proposed`, `finding` or `suggested` once the run has its result.
  `proposed` means the save became a proposal, which ends the run as succeeded with the result kind `proposal`.
- `restarted` when a conflict restarted the run from the latest version.
- `ended` once the run has ended.
- `null` otherwise.

`build` is the saved panel progress, `{"saves", "panels", "planned_panels"}`, or `null` when the run has no build.
Some results also carry a note after the content, which an agent reads like any other tool text.

### Briefs, usage and live text

`get_run_brief` takes `run_id`, `attempt` and an optional `reason` (`restart`, `output_limit`, `pending`, `conflict`, `continue`, `renewed`, `call_timeout`, `no_finding` or `no_answer`) and answers `system`, `message`, `reason` and `build`.
`message` is the first message of a model session.
Calling it starts a session: it resets the read guard and pins the parent version the run's saves are checked against.
The server may replace `reason`, with `conflict` after a rebase and with the resume reason when the run has a build.
An agent that never calls it works from the claim answer and the `observe` prompt.
It answers `run_ended` for a run that has ended.
A request on the run's token after the run ended is answered with HTTP 409 and `run_ended`, not a server error.

`report_run` also takes, all optional:

- `text`, at most 4,000 characters: the end of what the model is writing, shown on the run as live text while the run is running and never stored.
- `usage`: the agent's model use so far, cumulative, with `provider`, `model`, `input_tokens`, `output_tokens`, `cache_read_tokens`, `cache_write_tokens`, `cost_usd`, `model_calls`, `failed_calls`, `retries`, `compactions`, `renewals`, `model_ms` and `max_save_output_tokens`.

Its answer has, next to `state`, `stop` and `lease_until`, the run's `build` progress.

`finish_run` also takes `failure_code`, one of `call_timeout`, `rate_limited`, `provider_error`, `output_limit`, `no_tool_call`, `no_finding`, `refused`, `no_fixture` and `internal`, with `outcome` `failed`.
The server stores its own text for the code, so no `message` is needed.
`commit: true`, with `outcome` `succeeded`, saves the finished build as a version first.
`usage` takes the fields above, and the run shows it as the agent reports it.
The answer has `restarted`, true when the commit found a newer version and the run restarted from it: the run stays claimed, so call `get_run_brief` and go on.
`finish_run` on a run that was canceled settles it as canceled at once.
A `failure_code` with another outcome, a code outside the list, or `commit` with `outcome` `failed` answers `invalid_request`.

### Presence and modes

A connected agent or service account shows its presence, with `last_seen_at`; see [Agent presence](../reference/states.md#agent-presence).
The presence is one of these:

- `working` while it holds a run.
- `listening` when it asked for work in the last 40 seconds.
- `offline` otherwise.

A connection's `mode` is `direct` by default.
Its person can switch it to `propose` with `PATCH /api/v1/me/grants/{id}` and the body `{"mode": "propose"}`, and back with `direct`, or with the **Mode** menu on the [Agents page](agents.md#direct-and-propose-mode).
The change applies from the agent's next request, and no key or agent can change it.

In propose mode:

- `update_dashboard` and `update_panels` save a proposal and answer `proposed` even when the person is an Editor.
- `merge_proposal` answers `propose_mode`, and `discard_proposal` works only on a proposal the agent itself opened.
- A save that removes panels or variables answers `proposal_required` and names them, so the agent asks its person to remove them.
  The same holds when a run ends with `finish` on a build that removed any, and when `finish_run` with `commit` saves such a build, which stays so the agent can put the panels back and finish again.
- `create_dashboard` is not changed by the mode.

## Reference

This section has every rule that applies to all tools, then one entry for each tool.

### Endpoint behavior and limits

- Browser sessions are not accepted on `/mcp`, and an OAuth access token is accepted nowhere else.
- A request without a token, or with one the server refuses, gets `401` with `WWW-Authenticate: Bearer realm="slashobserve", resource_metadata="<public url>/.well-known/oauth-protected-resource/mcp"`.
  That header is how an MCP client finds the sign-in.
  A refused token adds `error="invalid_token"`.
- The endpoint is stateless.
- It refuses an `Origin` header that is not `SLASHOBSERVE_PUBLIC_URL` with `403` and the code `forbidden_origin`.
  A request with no `Origin` header is accepted.
- It answers `413` above 8 MiB, `429` above 8 concurrent requests per key or person, and `503` above 32 across all callers.
  A `claim_run` call does not count toward the 32.
- Those three, and a refused JSON-RPC batch (send one request per POST), come back as JSON-RPC error bodies with the short code in `error.data.code`.
  The `429` and `503` answers carry `Retry-After`, so a client SDK backs off instead of reporting a broken transport.
- `tools/list` has no pages: a `cursor` is ignored.
- For a key, `initialize` returns an `Mcp-Session-Id` that only carries the client's self-reported name and holds no state.
  For an OAuth client, the name it registered under is used and `clientInfo` is ignored.
- Each version, agent request, finding and annotation records its origin (`web`, `cli`, `mcp` or `api`) and, for `mcp`, that client name.
  The web app, the CLI and the API show the author as `<name> (via <client>)`.

### Roles by tool

Each tool has a lowest role, which the server checks when a client calls it.
The server uses the role of the key's service account, or of the person for an OAuth client.

| Tool | Lowest role |
| ---- | ----------- |
| `add_block` | Viewer |
| `aggregate_audit_events` | Admin |
| `apply_suggested_change` | Viewer |
| `archive_run` | Viewer |
| `arrange_blocks` | Viewer |
| `call_mcp_tool` | Viewer |
| `cancel_run` | Viewer |
| `claim_run` | Viewer |
| `clear_home_dashboard` | Viewer |
| `compare_traces` | Viewer |
| `create_annotation` | Viewer |
| `create_comment` | Viewer |
| `create_dashboard` | Viewer |
| `create_folder` | Viewer |
| `create_observation` | Viewer |
| `diff_dashboard_versions` | Viewer |
| `discard_block` | Viewer |
| `discard_proposal` | Viewer |
| `draft_query` | Viewer |
| `duplicate_dashboard` | Viewer |
| `edit_comment` | Viewer |
| `find_mcp_tools` | Viewer |
| `finish_run` | Viewer |
| `get_audit_event` | Admin |
| `get_audit_histogram` | Admin |
| `get_chat` | Viewer |
| `get_dashboard` | Viewer |
| `get_home_dashboard` | Viewer |
| `get_observation` | Viewer |
| `get_panels` | Viewer |
| `get_proposal` | Viewer |
| `get_rca` | Viewer |
| `get_run` | Viewer |
| `get_run_brief` | Viewer |
| `get_run_build` | Viewer |
| `get_schema` | Viewer |
| `get_spec_guide` | Viewer |
| `get_trace` | Viewer |
| `import_grafana_dashboard` | Viewer |
| `keep_block` | Viewer |
| `lint_query` | Viewer |
| `list_access_requests` | Viewer |
| `list_activity` | Viewer |
| `list_annotations` | Viewer |
| `list_audit_facets` | Admin |
| `list_audit_fields` | Admin |
| `list_chats` | Viewer |
| `list_comments` | Viewer |
| `list_dashboard_health` | Viewer |
| `list_dashboard_versions` | Viewer |
| `list_dashboard_viewers` | Viewer |
| `list_dashboards` | Viewer |
| `list_datasources` | Viewer |
| `list_folders` | Viewer |
| `list_inbox` | Viewer |
| `list_label_values` | Viewer |
| `list_labels` | Viewer |
| `list_mcp_sources` | Viewer |
| `list_mentionable_people` | Viewer |
| `list_metrics` | Viewer |
| `list_observations` | Viewer |
| `list_pinned_dashboards` | Viewer |
| `list_proposals` | Viewer |
| `list_rcas` | Viewer |
| `list_runs` | Viewer |
| `list_saved_searches` | Viewer |
| `list_services` | Viewer |
| `log_context` | Viewer |
| `log_facets` | Viewer |
| `log_patterns` | Viewer |
| `logs_for_trace` | Viewer |
| `make_dashboard` | Viewer |
| `measure_aggregate` | Viewer |
| `measure_window` | Viewer |
| `merge_proposal` | Viewer |
| `move_dashboard` | Viewer |
| `move_observation` | Viewer |
| `pin_dashboard` | Viewer |
| `pin_observation` | Viewer |
| `pin_run` | Viewer |
| `publish_rca` | Viewer |
| `query_instant` | Viewer |
| `query_range` | Viewer |
| `recheck_datasource` | Viewer |
| `rename_folder` | Viewer |
| `rename_run` | Viewer |
| `reopen_comment` | Viewer |
| `reopen_observation` | Viewer |
| `reply_to_comment` | Viewer |
| `report_run` | Viewer |
| `request_comment_access` | Viewer |
| `resolve_comment` | Viewer |
| `resolve_observation` | Viewer |
| `restore_dashboard_version` | Viewer |
| `retry_run` | Viewer |
| `run_dashboard_health` | Admin |
| `sample_rows` | Viewer |
| `save_run_answer` | Viewer |
| `save_search` | Viewer |
| `search_audit_events` | Admin |
| `search_workspace` | Viewer |
| `set_home_dashboard` | Viewer |
| `set_inbox_item_done` | Viewer |
| `share_answer` | Viewer |
| `show_panel` | Viewer |
| `start_run` | Viewer |
| `trace_error_origins` | Viewer |
| `trace_slow_path` | Viewer |
| `trace_stats` | Viewer |
| `unpin_dashboard` | Viewer |
| `unpin_run` | Viewer |
| `update_annotation` | Viewer |
| `update_block` | Viewer |
| `update_dashboard` | Viewer |
| `update_observation` | Viewer |
| `update_panels` | Viewer |
| `update_saved_search` | Viewer |

The lowest role is the one the web app needs for the same work, and folder access decides the rest, as in the web app.
A person's client can create and change dashboards in their own My Dashboards, comment where they may comment, and query every datasource, whatever their role.
A key has no My Dashboards and is never on a folder's list, so below Editor it can read and query but not save directly.
A save from a Commenter becomes a [proposal](../concepts/proposals.md).

### Errors

Every refused tool call returns one shape: `code`, a stable word to branch on, and `message`, what to change.
It adds fields specific to the refusal, such as `issues`, `latest_version`, `allowed_actions` or `needed_role`.
These codes apply to many tools:

- `forbidden_role`: the call is below the tool's role, or the folder does not allow the change.
- `not_found`: a dashboard, folder or other object the caller cannot see.
- `version_not_found`: a dashboard the caller can see has no such version.
  It carries a `message` and `latest_version`, so `not_found` always means the dashboard, never the version.
- `invalid_request`: a field the tool does not take, a field of the wrong JSON type, or two names for one field with different values.
  The answer lists the fields the tool takes, or the type it needs.
- `user_session_required`: a service-account key called a tool that works only for a person.
- `too_large`: a result over its size limit; the entry for the tool says what to ask for instead.

Other codes belong to one tool and are listed in its entry.

### Tool annotations and output schemas

Each tool carries MCP annotations that say what it does.
`readOnlyHint` is true for a tool that only reads.
`destructiveHint` is true for a tool that can overwrite or delete what someone saved.
Those tools are `update_dashboard`, `update_panels`, `restore_dashboard_version`, `update_block`, `arrange_blocks`, `update_observation`, `update_annotation`, `update_saved_search`, `edit_comment`, `rename_folder`, `rename_run`, `discard_block`, `discard_proposal`, `merge_proposal`, `set_home_dashboard`, `clear_home_dashboard`, `set_inbox_item_done` and `cancel_run`.
A client that approves only non-destructive tools on its own therefore asks before any of them runs.

`list_datasources`, `list_dashboards`, `get_dashboard`, `query_range` and `query_instant` also publish an `outputSchema`, so a client knows the fields of a result before it calls.
The schema accepts the `{code, message}` body a failed call answers with, because a client that validates results checks a failed one too.
Every other tool publishes none.

### Charts in MCP Apps hosts

`show_panel` draws a chart in the conversation, in a host that supports MCP Apps (extension `io.modelcontextprotocol/ui`, version 2026-01-26), with the panel renderer of the web app.
The chart follows the host's light or dark theme, resizes with the host and has an Open in SlashObserve button that opens the same query or panel in the web app.

- The chart is one HTML resource, `ui://slashobserve/panel` of type `text/html;profile=mcp-app`, built with the web app and served by the server.
  It declares no outside domains and makes no network request; everything it draws comes in the tool result.
- The tool names the resource in `_meta.ui.resourceUri` only for a client that announced the extension with that type in `initialize` (or, for the 2026-07-28 protocol, in the capabilities of the request).
  Any other client sees `show_panel` as an ordinary tool with no `_meta.ui`, and its result is the text summary alone.
  SlashObserve is stateless, so a client that announced the extension is remembered through the `Mcp-Session-Id` the server returns on `initialize`; a client that does not send it back gets the plain tool.
- For a host with apps the result carries the frames, the panel settings and the link as `structuredContent`, which the host gives to the chart and does not add to the model's context.
  Other clients get the summary as `structuredContent`, like `query_range`.
- The built-in agent has no `show_panel`.
  A server built from source without `make web` has no chart resource and offers `show_panel` without `_meta.ui`.

### Tools only a person can use

`pin_dashboard`, `unpin_dashboard`, `list_pinned_dashboards`, `list_inbox` and `set_inbox_item_done` work only over a person's OAuth connection.
A service-account key does not see them in `tools/list`, and answers `user_session_required` if it calls one.
`get_home_dashboard`, `set_home_dashboard` and `clear_home_dashboard` also answer `user_session_required` to a key.

### Argument names and aliases

A thing's id is named `<thing>_id` and a datasource is named `datasource`.
Every tool that takes one also accepts its common alias, so a guess costs no call:

- `datasource_uid` is accepted wherever `datasource` is.
- `id` is accepted for `dashboard_id` on a tool named for a dashboard, for `observation_id` on one named for an observation and for `run_id` on one named for a run.
- `run_id` is accepted for the `id` of every tool named for a run, such as `get_run`, `get_run_build`, `cancel_run` and `retry_run`.
- `diff_dashboard_versions` also accepts `from_version` and `to_version` for `from` and `to`.
- `get_panels` accepts `panel_ids` for `ids`.

A call that sends both names with the same value works, and one that sends them with different values is refused with `invalid_request`.
The names in `tools/list` are the documented ones; a run of the built-in agent takes only those.

### Tool text and instructions

On `initialize` the server sends instructions with the order of work: read `get_spec_guide` once before the first save, find real names with the discovery tools, check each query with `lint_query` and `query_range` or `query_instant`, then save, sending changes with `update_panels`.
Each tool's description and input schema are the text the built-in agent reads.
MCP clients differ in three places: the full guide from `get_spec_guide`, `folder_id` on `create_dashboard`, and `dashboard_id` and `parent_version` on `update_dashboard` and `update_panels`.
Where a description names a tool no run has, such as `create_comment` or `start_run`, the agent's text leaves that name out.
`get_spec_guide` returns the core of the authoring guide plus one section per datasource type the server has configured, the text the built-in agent reads.
An Admin also gets the section on the built-in audit datasource.

### Discovery by datasource type

Each datasource type answers these discovery tools:

- Prometheus: `list_metrics`, `list_labels` and `list_label_values`, over the last hour unless you pass `from` and `to`.
- ClickHouse: the same on the OpenTelemetry tables, plus `get_schema` and `sample_rows`.
- Loki:
  - `list_labels` and `list_label_values` for stream labels, narrowed by a stream selector in `metric`.
  - `sample_rows` for up to 20 lines of the last 15 minutes, from every stream or from a stream selector in `table`.
  - `list_metrics` with a stream selector as `scope`.
- Tempo: `list_labels` and `list_label_values` for TraceQL attributes, and `get_schema` for the services seen in the last hour with their span names as `columns` and `rows`, plus `attributes`; when Tempo does not answer in time for some services, they are still listed with a null span name and `notes` says how many; `list_labels` and `list_label_values` read the same hour unless you pass `from` and `to`.
- Jaeger: `list_label_values` for the services, or for the operations of one service with `service=checkout` as `metric`, and `get_schema` for every service with its span names as `columns` and `rows`.
  Tags are free-form and are not listed.
- Elasticsearch and OpenSearch:
  - `list_labels` for the fields you can group or filter by, and `list_label_values` for a field's top values over the range.
  - `get_schema` for the concrete indices of the index pattern, or with `table` set to an index name every mapped field with its kind.
  - `sample_rows` for up to 20 of the newest documents of the index pattern with no `table`.
  - `list_metrics` with a `scope` answers an empty inventory there.

`list_metrics` without a `scope` on Loki, Tempo, Jaeger, Elasticsearch or OpenSearch, and `get_schema` on Prometheus or Loki, answer `invalid_request` and name the datasource types they read, instead of an empty list or the datasource's own wording.

On Loki each family of a scoped `list_metrics` is a stream label key, and its `series` counts the streams that carry it.
That inventory, and the empty one of Elasticsearch and OpenSearch, never replaces the one `uncovered` reads.

### Save results report what is left uncovered

The server keeps the last scoped inventory of each service account.
After one, the results of `create_dashboard`, `update_dashboard` and `update_panels` carry two fields:

- `uncovered`, up to 50 families of that inventory that no panel query names.
  It is `[]` once every family is covered.
- `uncovered_total`, how many there are in all.

Runtime families left out of the inventory are never in `uncovered`.
A histogram, summary or counter counts as covered when a query names any of its series, such as `<name>_bucket` or `<name>_total`.

Save results also carry `notes`, up to 20 hints that never reject a save:

- A time series or bar chart query with no `legend`.
- A stat, gauge, bar gauge or time series with no `unit`.
- A stat with no `description`.

### Comment rules

These rules apply to every comment tool.

Comments can sit on a dashboard, an annotation, an observation or a proposal.
A `target` is written `kind:uuid`, for example `observation:7f0c1d2e-5a4b-4c3d-8e9f-0a1b2c3d4e5f`.
A `proposal:<uuid>` target is read with View and written with Comment on the proposal's dashboard, and a comment on it has no `panel_id` or `version`.
A key needs the Comment ability on the target to write.
A target the key cannot see answers `not_found`, and only the author can edit a message (`not_author`).

Each comment the tools return has these fields:

- `version`, which is null off a dashboard, `edited_at` and its `mentions`.
- `actor`, the same object the HTTP API returns (`kind`, `id`, `name`, `origin`, `client`, `run_id` and `label`).
  Its `run_id` is the run that wrote an agent reply and null for a person.
- `author`, the plain name.
- A top-level `run_id`, only on an agent reply.
- `target`, when the result was listed by `target`.
- `block_id`, null for the whole observation and on every other target.
- `reply_to`, null when it quotes nothing, else the quoted message's `id`, its `author` as the web app labels it, and an `excerpt` of its first 200 characters.
- `anchor` and `at`, on the first comment of a thread that points at a spot.
  `anchor` is the spot as `create_comment` takes it, and `at` is the spot in words.
  Replies carry neither, so read the spot from the comment their `parent_id` names.

[Anchors over MCP and the CLI](ask-and-comment.md#anchors-over-mcp-and-the-cli) lists the kinds of `anchor` and what each needs.
A single comment too large for a page first drops `at`, then its mentions lose their names but keep their `kind` and `id`, and then its `body` is cut, with `body_truncated`.
Its `anchor` is always kept whole.
A comment id is the id of any message of the thread, and the first message's id is the thread's.

Mentions:

- Write `@[Name](user:<id>)` for a person.
  `list_mentionable_people` gives the `id`.
- Write `@observe` to address the built-in agent, `@[Name](grant:<id>)` for one of your own connected agents and `@[Name](service_account:<id>)` for a service account that takes runs.
- A mention is recorded and appears in the activity log as `thread.mentioned`.
- Mentioning a person who does not exist or cannot see the target answers `invalid_request` with the reason in its message.
  The HTTP API and the CLI say `invalid_mention`.
  A mentioned person gets it in their Inbox.

Asking an agent:

- A comment is for everyone and asks no agent unless the call sets `to` to `agent`.
- With `to: agent` the comment asks the one agent it mentions, else the thread's agent, else `@observe`, in the same transaction as the comment.
  That agent becomes the thread's agent.
  A comment for everyone turns the thread's agent off.
- The result carries `to`, and `run` (`id`, `status`, `assigned_kind`) or `run_refusal` (`code`, `message`) when the comment was stored without a run.
- A dashboard or an observation thread starts a read-only ask, which needs Comment on the dashboard or observation and answers in the thread without changing it.
  An annotation or a proposal thread starts none.
  A run started this way is checked like one the same person starts in the web app.
- Two agents in one comment write nothing and answer `invalid_request`, as do a grant that is not yours, revoked or expired, a service account that takes no runs, and a grant mention from a key.
  The HTTP API and the CLI name these `too_many_agents` and `invalid_mention`.
- The refusal codes are `mention_target_unsupported`, `agent_unavailable`, `agent_not_yours`, `forbidden_role` and `too_many_runs`.
  `agent_unavailable` also applies when the thread's agent was disconnected, which clears it.
  `agent_not_yours` means the thread's agent is someone else's connected agent.

When a run a comment started ends, whatever the outcome, the server posts one reply in the thread.
The reply is written by the actor the run's agent writes as:

- "Observe Agent for <person>" for the built-in agent.
- "<person> (via <client>)" for a connected agent.
- The service account's own name for a service account.

The reply is a status line the server writes, and it does not resolve the thread.
`list_comments` shows it like any other message, with `run_id` set to the run that wrote it when the built-in agent did.
When the run saved an answer you can read, the reply also carries `answer`: `run_id`, `finding_id`, `title`, the start of `markdown` (400 bytes, with `markdown_truncated` when cut), `evidence_panels`, `suggested_change` and `observation_id`.
`get_run` reads the rest.
A reply that could not be posted when the run ended is tried again every minute for up to a day, and a reply a person deleted is never posted again.

Every opened, replied, edited, resolved and reopened thread appends to the activity log, which `list_activity` reads.

### Tool reference

The entries are grouped by task.
Each entry names the arguments the tool takes and what it returns.

### Dashboard tools

#### `get_spec_guide`

Returns the authoring guide, the JSON Schema and an example for ObserveSpec, the dashboard format.
Call it once before the first save.
It takes no arguments.
Its content is described under [Tool text and instructions](#tool-text-and-instructions).

#### `list_dashboards`

Lists the dashboards the key can see, with `id`, `title`, `tags`, `folder`, `latest_version`, `updated_at`, `view_count` and `url`.

- `limit` is 1 to 50, 20 by default, and `cursor` takes the `next_cursor` of the previous page.
- `order` is `updated` (the default), `views` for the most viewed first, or `name` for A to Z by title.
  Pass a `next_cursor` back with the order that produced it.
- `exclude_audit: true` leaves out the dashboards in the [Audit folder](activity.md#audit-folder).
- `view_count` is the number of people-days on which the dashboard was opened in the last 30 days.

#### `get_dashboard`

Reads a dashboard's Spec at the latest version, or at `version`.

- The result has `saved_by`, `origin`, `saved_at` and `summary` of that version, its `url`, its `folder` and its `unverified` panels and variables.
- `dashboard_origin` says what else writes the dashboard: `starter` (a release, so it is read-only), `file` (a file in the server's [provisioning directory](../operate/provisioning.md#edits-made-in-the-app)), `applied` (`observe dashboards apply`) or `editable` (nothing).
  `origin` is a different value: where that version was saved from.
- It also has `view_count`, with `window_days`.
- A Spec over 128 KiB returns `too_large`; read its panels with `get_panels` instead.

#### `get_panels`

Reads the full JSON of up to 20 panels by `ids`, at the latest version or at `version`, together with the dashboard's variables.

- Ids it did not return are in `missing`.
- `truncated` is true when the result was cut to fit.
- It returns at least one panel, and a panel too large to return is listed in `too_large`.

#### `create_dashboard`

Saves a new dashboard from a complete Spec document and returns its `dashboard_id`, `version` and `url`.

- It takes `spec`, a `summary` and an optional `folder_id` from `list_folders`.
  Without a `folder_id`, a person's dashboard goes to their My Dashboards and a key's to General.
- It runs every panel query before it saves, and rejects a panel whose query fails or returns no data, as [What a save checks](../concepts/agent.md#what-a-save-checks) describes.
- A retry is safe.
  When the same caller already saved a dashboard with that title and an identical spec in that folder, it returns that dashboard with `replayed` true instead of making a second one.
- A `folder_id` the key cannot create in returns `forbidden_role`, and an unknown one `not_found`.
- Propose mode does not change it: a connection in propose mode still creates dashboards.

#### `update_dashboard`

Saves a new version of a dashboard from the complete document, which replaces it whole.
Use it only to rewrite most of a dashboard; for anything smaller, use `update_panels`.

- It needs `dashboard_id` and the `parent_version` you read, and sends the complete document in `spec`.
- It runs every panel query before it saves, as `create_dashboard` does.
- If a newer version has been saved since and your save changed none of the panels, variables and settings that version changed, the server merges the two.
  The result then carries `merged_from`, with the `base_version` you read and the `onto_version` it merged onto.
- If both changed the same panel, variable or setting, it returns the tool error `version_conflict` with `latest_version` and `conflicts`.
  Each conflict has a `kind`, an `id`, a `reason` and a `message`.
  The client reads the latest version and applies its change again.
- A save to a dashboard whose `dashboard_origin` is `file` works, and the result carries a `warning`: the next change to the file replaces the edit, which stays in the version history.
- It saves a [proposal](../concepts/proposals.md) instead when the caller can comment but not edit, or when its connection is in propose mode.
  The answer is a success, not a tool error.
  Its code is `proposed`, and it carries `proposal_id`, `revision`, `opened` (false when the save added a revision to the caller's open proposal) and the dashboard's `url`.
  A Viewer still gets `forbidden_role`.

#### `update_panels`

Changes an existing dashboard by sending only what changed, so a one-panel fix costs one panel and not the whole document.
It takes `dashboard_id`, `parent_version` and a `summary`, and a patch with any of these:

- `upsert`: up to 50 complete panels, each replacing the panel with its id or, with a new id, added.
  A panel that leaves out `gridPos` keeps its position, and a row keeps its children, which are upserted by their own ids.
- `delete`: up to 50 panel ids; a row only together with all of its children.
- `layout`: up to 200 moves of existing panels to a `gridPos` or into a row, with `row` set to `""` for the top level.
- `variables`: the complete variable list, which replaces the existing one.
- `dashboard`: the default `time`, the `title`, the `tags` and the `links`.

Behavior:

- A key present in `dashboard` replaces that value whole, so `tags` and `links` are not merged item by item and `[]` clears them.
  A key left out keeps the current value.
- The server merges the patch into the version named by `parent_version` and validates the whole result, so a bad tag or time is rejected with its path in the document.
- It runs only the queries it changed, and every query when `time` changes to a different range, as [What a save checks](../concepts/agent.md#what-a-save-checks) describes.
- A rejection lists `failing_panels`, so the client resends only those panels.
- An overlap is reported at the panel the patch upserted or moved, with both panels' positions in the message.
- Merging, the `warning` on a dashboard from a provisioning file, `version_conflict`, `proposed` and `proposal_required` work as for `update_dashboard`.

#### `list_dashboard_versions`

Lists a dashboard's history, 20 versions per page by default and at most 100.
It takes `dashboard_id`, `limit` and `cursor`.

#### `diff_dashboard_versions`

Compares two versions of a dashboard.
It takes `dashboard_id`, `from` (0 compares against an empty dashboard; by default the version before `to`) and `to` (by default the latest).

#### `restore_dashboard_version`

Saves an earlier version as a new one.
It takes `dashboard_id`, `version`, the `parent_version` you read as the latest, and an optional `summary` of at most 200 characters.
It applies the same `parent_version` check as a save.
A connection in propose mode cannot restore a dashboard directly and gets `proposal_required`.
It can save the change with `update_dashboard`, which becomes a proposal.

#### `move_dashboard`

Moves a dashboard to another folder.
It takes `dashboard_id` and `folder_id`.
A connection in propose mode gets `proposal_required`.

#### `duplicate_dashboard`

Copies a dashboard.
It takes `dashboard_id`, a `title` and an optional `folder_id`.
The copy goes to `folder_id`, by default the dashboard's own folder when you may create there, else your My Dashboards or General, as the web dialog does.

#### `import_grafana_dashboard`

Hands the server a Grafana dashboard.
See [Import Grafana dashboards](import-grafana.md).

- It takes `json`, the dashboard JSON text.
  It also takes an optional `folder` title and a `mapping` from Grafana datasource keys to your datasource uids, for datasources the result says could not be matched.
- Panels whose queries fail are saved as unverified and listed in the result, so the client can fix them with `update_dashboard`.
- The tool refuses JSON that converts to an HTML text panel, a placeholder panel or a panel action with `import_content_not_allowed`.
  This is the same content `update_dashboard` and `create_dashboard` refuse.
  The message names the panels and asks the agent to remove them or to have a person import the JSON in the web app, where a person can also use `observe import`.
- The result names the folder the dashboard went to in `folder`, and `folder_note` says why when it is not the one requested.
- A file whose Grafana uid an earlier import already holds, in a dashboard you can see, is not imported twice.
  The call fails with `already_imported`, and `existing` names that dashboard with its `dashboard_id`, `title`, `version`, `url` and `can_update`.
  - Pass its `dashboard_id` as the tool's `update_dashboard` argument, which is not the `update_dashboard` tool, to save the JSON as its next version.
    That needs Edit, keeps the folder, and answers `updated: true`.
  - Or set `import_as_new` to make another dashboard on purpose.
- An update with a file that has another Grafana uid than the dashboard was imported from fails with `grafana_uid_mismatch` and changes nothing.
- A retried call of the same agent request still answers with the dashboard it made.
- The built-in agent in a run that creates a dashboard imports as before and is not refused, because the person asked it to import that JSON.
  It may not use `update_dashboard` on this tool, because that run may not save a version of an existing dashboard.

#### `list_dashboard_viewers`

Lists, by name, the other people who have the dashboard open in the web app now, so a client can see who is looking before it changes the dashboard.
It needs View, and the list is empty when nobody is looking.
The built-in agent has it too.

#### `list_dashboard_health`

Lists the dashboards with [health problems](dashboard-health.md) in the folders the key can see.
The built-in agent has it too.

- The order is the one in [Dashboard health](dashboard-health.md): failing first, then no data, then duplicate title, then unopened, and within each the most recently opened first.
- A page has 20 dashboards by default and at most 50.
  `folder_id` and `kind` narrow the list, and `next_cursor` pages on.
- Each dashboard has its `problems`: `kind`, and `panel_id`, `panel_title`, `variable`, `code` and `message` where they apply.
  It also has `checked_at` and `checked_version`.
- The `job` field shows the state of the daily check.

#### `run_dashboard_health`

Starts the health run, as `POST /api/v1/health/run` does, and needs an Admin.
It answers at once and starts nothing more while a run is going.
The `job` field of `list_dashboard_health` shows how it went.
It is over MCP only, because it checks every dashboard of the workspace, far beyond what one agent run is about.

#### `pin_dashboard`, `unpin_dashboard` and `list_pinned_dashboards`

Manage the person's pins, the dashboards in their sidebar.
The HTTP API calls them stars (`/dashboards/{id}/star`, `/stars`).
They work only over a person's OAuth connection.

#### `get_home_dashboard`, `set_home_dashboard` and `clear_home_dashboard`

Manage the dashboard that opens for the person after sign-in in place of Home.
The HTTP API calls it `/me/home-dashboard`.

- `get_home_dashboard` returns `status`, one of `none`, `ok`, `gone` or `unreadable`, and, when it is `ok`, the dashboard.
- `set_home_dashboard` needs a dashboard the person can view and answers `not_found` otherwise.
  Setting the same dashboard again changes nothing.
- They work only over a person's OAuth connection.

### Datasource and query tools

#### `list_datasources`

Lists the configured datasources with their `uid`, `type`, `name` and `healthy` flag.
An unhealthy one carries `message` and `detail`.
An Admin's call also shows `audience`, `everyone` or `admins`.
Every other tool names a datasource by this `uid`.

#### `recheck_datasource`

Runs a fresh health check on one datasource instead of reading the cached one, and answers its status.
Calls within 2 seconds of the last check return that check.
The built-in agent has it too.

#### `list_metrics`

Lists the metric names of a Prometheus datasource, or the OpenTelemetry metric names of a ClickHouse datasource.
It takes `datasource`, `search`, `limit` (1 to 500, 100 by default), `offset` and `scope`.

- `scope` is a series selector such as `{job="checkout"}` on Prometheus, or a table name on ClickHouse.
  With it, the tool answers every metric family in that scope in one result.
  Each family has its name, type, unit, label keys and series count.
  The result also has `families_total` and `grouped`, the families that share a name prefix such as `http_server`.
- Label keys that every family has are listed once in `common_labels`.
- A scope too large for one result pages with `next_offset` like the other lists.
- A scope that matches no family comes back with a `note` that lists up to 100 label keys the datasource has, on Prometheus and Loki.
  When only runtime families match, the note says so instead.
  When the scope names a label key that is not among them, the note says so, so a guessed key such as `service_name` on a store that uses `job` shows up at once.
- Runtime families, `up` and the names that start with `go_`, `process_`, `promhttp_` or `scrape_`, are left out of the list and counted in `runtime_families`.
  They stay in when the scope names `__name__`, as `{__name__=~"go_.+"}` or `go_goroutines{job="checkout"}` do.
- `search` and `limit` are refused together with `scope`.
- The inventory of a scoped call feeds `uncovered` in save results; see [Save results report what is left uncovered](#save-results-report-what-is-left-uncovered).

#### `list_labels` and `list_label_values`

List label names, and the values of one label, for a datasource; [Discovery by datasource type](#discovery-by-datasource-type) says what each type answers.
`list_label_values` takes the `label`.

- Both take `datasource`, `metric`, `table`, `search`, `limit` and `offset`.
- Both also take `from` and `to`, both or neither, so Loki and Tempo answer only the names seen in that window instead of their own default one.

#### `get_schema`

Lists the tables of a ClickHouse datasource, or the columns and types of one table with `table`.
On Tempo, Jaeger, Elasticsearch and OpenSearch it answers as [Discovery by datasource type](#discovery-by-datasource-type) says.
It takes `offset` to page.

#### Paging of discovery tools

`list_metrics`, `list_labels`, `list_label_values` and `get_schema` take an `offset`.
They answer `truncated`, `truncated_by` (`limit` or `bytes`) and `next_offset`, so a client pages through a long list by passing `next_offset` back as `offset`.

#### `sample_rows`

Reads recent rows to show real values.
It takes `datasource`, `table` and `limit`, 1 to 20 and 5 by default.
`table` is required on ClickHouse.

#### `list_services`

Lists the services a log, trace or metric datasource has seen over a recent window, and finds which datasources hold a service.

- With `datasource` it reads that one source.
  Without it, it asks every Loki, Tempo, Jaeger, ClickHouse, Elasticsearch, OpenSearch and Prometheus source that everyone can query, and ClickHouse answers once for its logs table and once for its traces table.
  A source only Admins can query is left out unless you name it with `datasource`, so a plain listing does not mark an Admin's run as one that read Admin-only data.
- `signal` (`logs`, `traces` or `metrics`) picks one signal.
- A Prometheus source lists the values of the first of `service_name`, `service`, `app` and `job` that its series carry in the window, and names that label as `field`, the same rule Loki follows.
  `service_name` comes first because OpenTelemetry names the service that way, `service` and `app` follow as the usual hand-written labels, and `job` comes last because every scraped series has it but it names a scrape job, which is often but not always a service.
  A Prometheus source with none of them answers an empty `field` and no services.
  Metric services come from Prometheus datasources only, not from ClickHouse metric tables.
- `window` (such as `15m`, default `1h`, at most `6h`) sets how far back the window ends now.
  Jaeger ignores it and lists every service it retains.
- `service` keeps only the sources that hold a service whose name contains it, case-insensitive.
- Each source answers its `field`, the label, column or attribute that holds the service, so a query can filter on it.
- A source that does not answer in time says why under `unavailable` while the others still answer.
- Answers are cached for about a minute.
  A source lists at most 200 services, with `truncated` set when there were more.

#### `lint_query`

Checks a PromQL, LogQL, TraceQL, ClickHouse SQL, KQL, Lucene or Query DSL query for syntax and policy problems without running it.
It takes `datasource` and `query`.
`ok` is false when any diagnostic is an error.
It cannot tell whether the query returns data.
An error such as `datasource_unavailable` means the check could not run, not that the query is wrong.

#### `query_range`

Runs a query over a time range and returns a compact summary: up to 20 series with points, min, max and last, or the first rows of a table.

- It takes `datasource`, `query`, `from` and `to` (default `now-1h` and `now`, at most 31 days apart), `max_data_points` (10 to 1000, default 200) and `format` (`timeseries`, `table`, `heatmap` or `logs`; default `timeseries`).
- It takes `variables`, an object of variable name to a list of values for each `$variable` the query uses, and `dashboard_title` for `$__dashboard`.
- It returns `step_ms`, the step the query ran at, which is the larger of window divided by `max_data_points` and the datasource's scrape interval.
  The `min`, `max` and `last` of a series are of those sampled points.
  On a Prometheus datasource, when the step is longer than the scrape interval, the `note` says an extreme between samples is not shown and suggests `max_over_time` or `min_over_time` over the window as a `query_instant`.
- It returns `executed`, the text each query ran as after variables and macros were expanded, when it was sent to the datasource.
  That shows what a `$job` or a `$__rate_interval` became.
- A Loki log query comes back as one table of lines, newest first across every stream, with a `labels` column naming each line's stream.
  The summary shows up to ten lines; `query_instant` with `page_size` shows the whole page.
- On any ClickHouse table, `query_range` refuses a query that does not read the time range through a macro such as `$__timeFilter(Timestamp)`.
  A macro inside a comment does not count.
- On Prometheus and Loki, an empty result's `note` says why nothing matched, as [Empty results](#empty-results) explains.

#### `show_panel`

Draws a chart for a host that supports MCP Apps, as [Charts in MCP Apps hosts](#charts-in-mcp-apps-hosts) explains.
It is read-only, and it runs through the same path and under the same limits as `query_range`.

- Give `datasource` and `query`, with `type` (`timeseries`, `barchart`, `stat`, `gauge`, `bargauge`, `piechart`, `table`, `trend`, `histogram`, `heatmap`, `state-timeline`, `status-history` or `logs`; default `timeseries`), `title`, `from`, `to`, `max_data_points` and `variables` as for `query_range`.
- Or give `dashboard_id` and `panel`, with `from`, `to` and `max_data_points`; `type`, `title` and `variables` are refused with it.
  The panel runs with its dashboard's variables, time zone and time overrides, for a reader who can view the dashboard.
  A panel type the chart view does not draw, such as a map or a text panel, is refused with a link to the dashboard.
- The chart shows the range actually queried, after the panel's time override and shift.
- It returns the summary of `query_range` and `open_url`, the address of the query in Explore or of the panel in its dashboard.
- A chart with more than 2 MiB of data is not drawn: the host gets the summary and a note to ask for a shorter range or fewer series.
- A server built without the chart view, and a client that does not show MCP Apps, get the summary only.

#### `measure_aggregate`

Measures one figure pooled over a whole window on a Prometheus or Loki datasource: a total, a ratio of two totals, a quantile, or the peak, low or average of a field in log lines.
It runs one instant query at `to` over a range as long as the window, so the figure covers every sample and is never pieced together from the points of a range query.

- Pass `datasource`, `op` and `query`, and optionally `denominator`, `quantile`, `unwrap`, `by`, `from` and `to`.
- On Prometheus, `query` is a series selector such as `http_requests_total{service_name="cart"}`; an expression is refused, because the server adds `increase` and `sum` itself.
- `op` `total` is `sum(increase(query[W]))`, where `W` is the window.
- `op` `ratio` divides the total of `query` by the total of `denominator`.
  A group whose numerator has no series but whose denominator grew reads 0, not an empty result.
  Each value also carries `numerator` and `denominator`, and the result carries the two queries that gave them.
- `op` `quantile` with `quantile`, such as `0.99`, is `histogram_quantile(q, sum by (le) (increase(<name>_bucket{...}[W])))`.
  When `<name>_bucket` has no series, the selector is read as a native histogram, `histogram_quantile(q, sum(increase(<name>{...}[W])))`, and `query` names the one that answered.
- On Loki, `query` is a log query, a stream selector with an optional pipeline such as `{service_name="cart"} | json`; a metric query is refused, because the server adds the range function itself.
  `op` `total` counts the matching lines, `sum(count_over_time(query [W]))`, and `ratio` divides the line counts of `query` and `denominator`, 0 when `query` matches nothing.
  With `unwrap`, the numeric label or parsed field to measure (`duration(field)` and `bytes(field)` read values with units), `total` is the field's sum and `quantile`, `max`, `min` and `avg` pool every line in the window, such as `quantile_over_time(0.99, query | unwrap ms | __error__="" [W]) by ()`.
  Lines whose field does not parse are skipped.
- `by` names up to 5 labels to group by, such as `service_name`; leave it out for one figure over everything matched.
- The result holds `store`, the datasource type, `query`, the query that ran, and one entry per group in `values` (at most 20) with `value` at full precision, or `undefined` when nothing was counted, such as a ratio whose denominator did not grow.
  With the grounding check on, a fact of a Loki result is the logs signal of a cause, and one of Prometheus is the metrics signal.
- An empty result says so in `note`: no series matched, which does not show the value was zero, and the note first says why, as [Empty results](#empty-results) explains.
- On Prometheus, `max`, `min` and `avg` are `measure_window`'s, and another datasource type answers `invalid_request`.

#### `measure_window`

Measures one figure of a PromQL expression over a whole window on a Prometheus datasource.
The datasource computes it from every sample or step, so a peak between the points of a range query is not missed.

- Pass `datasource`, `query`, `stat` and optionally `from`, `to` and `step`.
- `stat` is `max`, `min`, `avg` or `last` for a value, and `first_time` or `last_time` with `where` for when a condition first or last held, such as `== 0` or `> 100`.
- A plain selector such as `queue_depth{service="orders"}` is measured over every stored sample.
  Any other expression is evaluated every `step`, 1 minute by default and 15 seconds for a time.
- The result holds `query`, the PromQL that ran, `sampled`, and one entry per series in `values` (at most 20): `value` at full precision, or `at` and `seconds_after_from` for a time.
- `max` and `min` of a single series also carry `at`, when the value was first reached, when a 15 second step finds it.
- An empty result says so in `note`: no series matched, which does not show the value was zero, and the note first says why, as [Empty results](#empty-results) explains.
- A total, a ratio or a quantile over the window comes from `measure_aggregate`.
- Another datasource type answers `invalid_request`.

#### Empty results

When `query_range`, `query_instant`, `measure_window` or `measure_aggregate` returns nothing on a Prometheus or Loki datasource, the server reads the datasource's label names and values for each selector in the query before it answers, and the `note` starts with the first reason it finds:

- A label key the selector uses is on no series of that metric, or no stream, in the window, with the closest keys the datasource has, such as `service_name` and `job` for `service`.
- The key exists but the value does not, with the closest values, or some of the values when none is close.
- The key or value exists in the 7 days around the window but has no samples in it, or the metric has series but none in the window.
- No metric of that name exists, with the closest names.
- On Loki, the stream selector matches lines in the window, so the pipeline after it dropped them all, with the count of those lines.
- On Prometheus, when every selector's values exist, the note says that either no series has them all together or the query's functions or label matching removed every series.

The check makes at most 6 discovery calls in 4 seconds; when one fails or the time runs out, the note says nothing more and the result is unchanged.

#### `query_instant`

Runs a query as an instant query, PromQL evaluated once at `to`, and returns the summary of the rows or series.
Use it for a table panel's query, or a query with `instant: true`.

- It takes the arguments of `query_range` except `format` and `max_data_points`, plus `page_size`, `page`, `cursor`, `sort` and `as_written`.
- It returns `executed`, as `query_range` does, and on Prometheus and Loki explains an empty result in `note`.
- On a ClickHouse trace datasource it also runs your own SQL over `otel_traces`, the query the SQL view of the trace search runs, with the same read-only, time range and row checks.
  `page_size` (50 to 500), `page` and `sort` page it.
  - A paged query that selects `Timestamp` comes back as log lines, newest first, with a `TimeNs` column.
  - Pass `as_written` with `page_size` to get the rows as the SQL wrote them instead, with its own `ORDER BY`, `LIMIT` and columns.
- On any ClickHouse table, `query_instant` without `page_size` refuses a query that does not read the time range through a macro such as `$__timeFilter(Timestamp)`, even one that selects `Timestamp`.
  Only a paged query is cut to the range by the server.

#### `draft_query`

Turns a question into a checked LogQL, ClickHouse SQL, KQL, PromQL or TraceQL query, or trace search filters, as [Ask](logs-and-traces.md#ask-in-plain-language) does.
It works for one Loki, ClickHouse, Elasticsearch, OpenSearch, Prometheus, Tempo or Jaeger datasource.
It is over MCP only, because a run's cost is counted once for the run and `draft_query` makes model calls outside it.

- `signal` is `logs`, `traces` or `metrics`.
  It defaults to the one the datasource holds, and a ClickHouse datasource needs it.
- A Prometheus datasource answers with `language` `promql`, evaluated at the end of the range.
- It spends at most two of this server's model calls per question, checks the query with the lint and a short sample, and never runs the full query.
  Run the answer with `query_instant`.
- A question it cannot turn into a query returns the tool error `cannot_translate` with the reason.
  A server with no model returns `agent_unavailable`.

### Log and trace tools

#### `log_context`

Reads the log lines just before and just after one line on a Loki, ClickHouse, Elasticsearch or OpenSearch datasource, as [Show context](logs-and-traces.md#context-around-a-line) does.
The built-in agent has the same tool.

- Pass `datasource`, the `query` and the line's `time_ns` from a `query_instant` result as `anchor`.
- `before` and `after` default to 20 lines each and take at most 100.
- The answer holds each side's lines with `time`, `time_ns`, `body` and `labels`.
  The before side is newest first and the after side is oldest first.
  It says when more lines exist within an hour on either side.
- On an Elasticsearch or OpenSearch time field of type `date`, context is exact to the millisecond.

#### `log_patterns`

Groups the lines a logs query matches into patterns with counts on a Loki, ClickHouse, Elasticsearch or OpenSearch datasource, as [Patterns](logs-and-traces.md#pattern-templates) does.
The built-in agent has the same tool.

- It reads from Loki's pattern index or Elasticsearch's `categorize_text` when it has one, otherwise from the newest 2,000 lines.
  OpenSearch has no such aggregation, so the server clusters the newest 2,000 lines there and says `source: server`.
- Pass `datasource`, `query` and optionally `from`, `to` and `limit`, 10 patterns by default and at most 30.
- The answer says `source` (`loki`, `elasticsearch` or `server`), `lines_sampled` and `lines_matched`.
- Each pattern has `count`, `sample` cut to 200 bytes, `first`, `last`, `level` and `trend`.
  `trend` holds the counts of 30 equal buckets over the range, whose width in milliseconds is `trend_step_ms`.
- With the grounding check on, each pattern's `count` and the answer's `lines_matched` carry fact ids in `facts`; with `source: server` they are sampled facts, which a finding shows as `at least` the count ([Facts](../concepts/investigate.md#facts)).

#### `log_facets`

Counts the top values of each label, field or column that a logs query matches on a Loki, ClickHouse, Elasticsearch or OpenSearch datasource.
Counts are exact where the datasource can give them, and come from the newest sampled lines where it cannot.
The built-in agent has the same tool.

- With the grounding check on, each value's `count` and the answer's `lines_sampled` carry fact ids in `facts`.
  A count of a field that does not say `exact: true` is a sampled fact, which a finding shows as `at least` the count, and a count with `unit: bytes` shows `bytes` after it ([Facts](../concepts/investigate.md#facts)).

#### `logs_for_trace`

Reads the log lines that carry a trace id, or one span id, from a Loki, ClickHouse, Elasticsearch or OpenSearch datasource over a window.
These are the same lines the trace view opens from a span.
It answers the table summary of `query_instant`.

- It takes `datasource`, `trace_id` (16 or 32 hex characters), `span_id` (16 hex characters), `service`, `from`, `to` and `limit` (1 to 200, default 50).
- `service` narrows the Loki stream selector or the Elasticsearch and OpenSearch service field; ClickHouse ignores it.

#### `get_trace`

Reads one trace by id from a Tempo, Jaeger or ClickHouse datasource, or from an Elasticsearch or OpenSearch datasource with `spans: jaeger` ([Read spans that Jaeger stored](../connect/opensearch.md#read-spans-that-jaeger-stored)): its services, root span, error spans with the message each set, and slowest spans.
A trace over 20,000 spans is cut to its earliest spans and says so.
The built-in agent has the same tool.

#### `trace_stats`

Counts spans and errors per time bucket (at most 60, set with `buckets`).
It gives p50, p95 and p99 durations per bucket and a duration histogram over a filter.
It also gives p50, p95 and p99 for the whole range in `window`, and with `group_by` (one attribute such as `resource.service.name`, `span.http.route` or `.http.method`, or `name`, `status` or `kind`, which ClickHouse maps to its columns) the span count, error count and the same quantiles per value, at most 20 values by span count.
The window and group quantiles are exact: each is the duration at position q times n of every matching span's duration, sorted, which is what ClickHouse `quantilesExact` reads.
On Tempo the server reads those durations from a trace search of at most 20,000 spans; when the search returns fewer spans than match, `quantiles` says `sample` and `quantile_spans` says how many spans the figures rest on.
Tempo's per-bucket quantiles are interpolated from its log-scale duration buckets, so they are approximate, and the response says so in `note`.

The `query` is a TraceQL filter on Tempo, a SQL condition over `otel_traces` on ClickHouse, or a query in the datasource's own language over the span documents of an Elasticsearch or OpenSearch datasource with `spans: jaeger`, such as `process.serviceName:checkout`.
On such a datasource the window and group quantiles are exact from up to 20,000 span durations, and the per-bucket quantiles are the cluster's percentile estimates, which `note` says are approximate.
The built-in agent has the same tool.

#### `trace_error_origins`

Finds where failing requests start over a window.
It counts each failing trace once, at its deepest erroring span with no error below it, and groups the traces by that span's service, operation and status message.
Each group has its trace count, its share of the examined traces, one full example trace id and `relayed_by`, the services above the origin whose spans also failed and only passed the error on.

- It takes `datasource` (Tempo, Jaeger, ClickHouse, or an Elasticsearch or OpenSearch datasource with `spans: jaeger`), `service`, `route` (the root span name), `operation`, `by_route`, `from` and `to`; Jaeger needs `service`.
- `by_route` splits the groups by route, so each share is of that route's examined traces, and `routes` lists each route's failing and examined traces.
- The server lists up to 2,000 failing traces, the newest first, and reads up to 200 of them, spread over the window and taken from every route in turn.
  `failing_traces` is how many it listed, `complete` whether that is every failing trace in the window, `examined` how many it read and `sample` whether the figures rest on fewer than all of them.

The built-in agent has the same tool.

#### `trace_slow_path`

Finds where slow requests spend their time, for each route on its own.
A trace is slow when it lasts longer than `threshold_ms`, or by default longer than twice the p50 of its route over the listed traces of the window.
For each route (the root span name) it gives the threshold, the number of slow traces, the three spans with the most self time across them (service, operation, self time, share of the slow traces' self time and how many traces have the span), every service in the slow traces with its trace count, and the slowest one as an example with its full trace id and exact duration.

- It takes `datasource` (Tempo, Jaeger, ClickHouse, or an Elasticsearch or OpenSearch datasource with `spans: jaeger`), `service`, `route`, `operation`, `threshold_ms`, `from` and `to`; Jaeger needs `service`.
- Self time is a span's duration less the time its children cover.
- The server lists up to 2,000 traces and reads up to 200 slow ones, spread over the window and taken from every route in turn; `complete` and `sample` say whether the figures cover every slow trace, and at most 10 routes are returned.
- Tempo lists trace durations in whole milliseconds, so on Tempo the threshold and p50 rest on those, while the example's duration comes from the whole trace.

The built-in agent has the same tool.

#### `compare_traces`

Explains why some traces are slow or failing.
It takes up to 50 `selected` trace ids and up to 150 `baseline` ids from a Tempo or ClickHouse datasource, or from an Elasticsearch or OpenSearch datasource with `spans: jaeger`.
It returns the attribute values whose share of traces differs most between the two sets, at most `limit` of them (default 20, at most 100).
The built-in agent has the same tool.

#### `save_search`

Saves a logs, trace or metrics search with a title, so people can open it from the logs, traces or metrics page.

- It takes `kind` (`logs`, `traces` or `metrics`), `title` and `datasource`, and optionally `query`, `filters`, `from`, `to` and `folder_id`.
  It returns the saved search with its `url`.
- A logs search needs a `query` and a Loki, ClickHouse, Elasticsearch or OpenSearch datasource.
  Its `filters` takes only `columns`.
- A traces search needs a Tempo, Jaeger or ClickHouse datasource.
  The `query` is optional: TraceQL on Tempo, `key=value` pairs on Jaeger and SQL on ClickHouse, which reopens the search in the SQL view.
  Its `filters` takes `service`, `span`, `min`, `max` and `errors`.
- A metrics search needs a PromQL `query` and a Prometheus datasource.
  Its `filters` takes only `mode`, `graph` (the default) or `table`.
- `from` and `to` default to `now-1h` and `now`, and are saved as written.
- A search is filed in `folder_id`, by default the key's My Dashboards when it has one, else General, and the folder's readers can open it.
  A `folder_id` the key cannot save in returns `forbidden_role`, and an unknown one `not_found`.
  A datasource that does not exist or does not fit the kind returns `invalid_request`.
- The query is linted as `lint_query` does it.
  One with a lint error is refused with `invalid_request` and the lint message, so a saved search never opens on a query that cannot run.
  When the lint check itself cannot run, for example because the datasource is unreachable, the search is saved anyway.
  Saving runs no query, so check the query with `lint_query` or `draft_query` first.
- A repeat of a search you already saved, with the same kind, title, datasource, query, filters, range and folder, returns that search with `replayed: true` and saves no second copy.
- `POST /api/v1/saved-searches` and `observe saved-searches save` apply the same two checks to a service-account key.
  They refuse a lint error with 422 `query_invalid` and answer a repeat with 200 and `replayed` true.
  A person's browser session saves as before.
- No MCP tool renames or deletes a saved search, so people do that in the web app or with the CLI.

#### `list_saved_searches`

Lists the saved searches in the folders the key can see, most recently changed first.
It takes an optional `kind` or `folder_id`.
Each search has its datasource, query, filters, range, folder and `url`.

#### `update_saved_search`

Changes what a saved search runs.
Inside a run it changes only a search that run saved.
Pass its `id` from `list_saved_searches` and any of `query`, `filters`, `from` and `to`.

- Each one replaces the saved value.
  `filters` replaces all the filters, so send every filter to keep, and `null` removes them all.
- An empty `query` removes the query of a trace search and is refused for a logs search.
- The result is checked as a save is, so a rule it breaks returns `invalid_request`.
- The kind, datasource, title and folder stay, and the tool returns the search with its `url`.
- It needs Edit on the search's folder, which a Viewer key lacks (`forbidden_role`).
  A search the key cannot see returns `not_found`.
- No tool renames, moves or deletes a saved search: people do that on the web or with `observe saved-searches`.

### Proposal tools

#### `list_proposals`

Lists proposals.
It takes a `dashboard_id`, a `state` (`open` by default, or `merged`, `discarded` or `all`) and a `cursor`, and pages with `next_cursor`.

#### `get_proposal`

Returns a proposal with these fields:

- Its `spec` and its `changes`.
- The `conflicts` a merge would hit while it is open and behind the dashboard.
- The `allowed_actions` of the caller.
- The field-by-field `diff`.
  The diff is left out, with the step saying so, when it would take the result over 128 KiB.
  A result over 128 KiB without it answers `too_large`.

#### `merge_proposal`

Saves a proposal as a new version.

- It takes the `proposal_id`, the `revision` read with `get_proposal` and, for each conflict, a choice such as `{ "kind": "panel", "id": "p1", "take": "theirs" }`.
  `ours` keeps the dashboard's unit and `theirs` takes the proposal's.
- It needs Edit on the dashboard, and runs the queries of the panels it changes like any save.
- It answers `proposal_changed` with the newest `revision` when the author added one.
  It answers `version_conflict` with `conflicts` when a conflict has no choice.
  It answers `invalid_transition` with the `state` and `allowed_actions` when the proposal is already closed.
- In propose mode it answers `propose_mode`.

#### `discard_proposal`

Closes a proposal.
It takes the `proposal_id` and an optional `reason` of up to 500 characters that the proposal shows.
Its author or someone who may merge it can discard it.
A caller without the access gets `forbidden_role`, and an unknown proposal or one on a dashboard the caller cannot see gets `not_found`.
In propose mode it works only on a proposal the agent itself opened.

### Comment tools

The [comment rules](#comment-rules) above apply to all of these tools.

#### `list_comments`

Lists the comments on a dashboard, observation, annotation or proposal, oldest first.
Open threads are listed by default.

- It takes exactly one of `target` and `dashboard_id`.
  `dashboard_id` means the same as `dashboard:<id>`.
- Optional arguments are `status` (`open`, the default, `resolved` or `all`), `panel_id`, `limit` (1 to 100, 50 by default) and `cursor` from `next_cursor`.
- Comments carry the fields in [Comment rules](#comment-rules).

#### `create_comment`

Opens a thread on a `target`.

- It takes `target` and `body`, and optionally `to`, `client_id`, `anchor`, `block_id`, and on a dashboard `panel_id` and `version`.
  It takes `target` and not `dashboard_id`, so a comment on a dashboard is written with `target` `dashboard:<id>`, and a `dashboard_id` is refused.
- On a dashboard it defaults to the whole dashboard on the latest version.
- On an observation, it takes `block_id`, the block the message is about from `get_observation`, and omits it for the whole observation.
  A block of another observation, or a `block_id` on any other target, answers `invalid_request`.
- An observation has one discussion, so `create_comment` on an observation that has one adds the comment to it.
  With `anchor` it opens a thread of its own on that spot instead.
- `anchor` is the spot a dashboard or observation thread points at.
  With `block_id` on an observation, it is a spot in that block.
- `client_id` is a UUID you make for the message.
  Sending the same call again returns the message already posted with `replayed` true, and starts nothing.

#### `reply_to_comment`

Replies to a thread.
It works on any thread.

- It takes `comment_id`, `body`, and optionally `to`, `client_id`, `block_id` and `reply_to_id`.
- `reply_to_id` is a message of the same thread that the reply answers, which the web app shows quoted.
- On an observation it takes `block_id` as `create_comment` does.
- `client_id` works as in `create_comment`.

#### `resolve_comment`

Resolves a thread.
It works on any thread, and takes `version` only on a dashboard, the version that fixed the comment.
A caller with only Comment resolves only a thread they started and gets `not_starter` on any other.
A caller who can edit resolves any.

#### `reopen_comment`

Reopens a resolved thread.

#### `edit_comment`

Changes the text of a comment the same key wrote.
Only the author can edit a message (`not_author`).

#### `list_mentionable_people`

Lists the people who can see a `target`, so they can be mentioned.
It takes a `target` and an optional `query`.

- Each person has an `id` to put in `@[Name](user:<id>)`.
- It returns at most 20 people, and `more` is true when others match, so narrow with `query`.
- It also takes a `limit`, at most 20.
- It takes `target` and not `dashboard_id`.

#### `apply_suggested_change`

Applies the change that an answer on a dashboard suggests.
An answer may carry `suggested_change`, one sentence on the change the person asked for or the nearest safe one.

- It takes `comment_id` and the answer's `finding_id` from `get_run`.
- It posts "Revise the dashboard: <suggested change>" in that thread as you, and starts a run that makes the change.
  The run saves a version when you can edit the dashboard and opens a proposal when you can only comment.
- The run is yours and goes to the agent that answered.
- A repeat call while it is queued or running, or after it succeeded, posts nothing and returns that run with `replayed` true.
- An id that is not an answer given in this thread answers `not_found`, and an answer without a suggested change `invalid_request`.
- The run's own refusals are `too_many_revisions`, `agent_unavailable`, `assignee_unavailable` and `provisioned` on a starter dashboard.

#### `request_comment_access`

Asks for [comment access](../teams/folders.md#ask-for-comment-access) to a dashboard or observation the caller can view but not comment on.
It takes `object_kind` (`dashboard` or `observation`), `object_id` and an optional one-line `note` of at most 200 characters.

- It changes no access, and its answer never names who received the request.
- It answers what `POST /api/v1/access-requests` answers, including `already_can_comment` and `not_found`.
  It also answers `request_open` with `requested_at`, also named in its message, while a request is open.
- A service-account key may call it, and the built-in agent has it only in a chat, because it changes the requester's own requests.

#### `list_access_requests`

Lists the open requests on the folders the caller manages, as `GET /api/v1/access-requests` does, with each request's `folder`.
When `folder.general` is true, the object is in General, which takes no folder entry.
Only a role change or moving the object to another folder first grants it.
A service-account key may call it, and the built-in agent has it too.

### Observation tools

An observation is a page of blocks about one question.
[Observations](observations.md) describes them in the web app.

#### `create_observation`

Creates an observation and its blocks together.

- It takes a `title`, and optionally a `folder_id` (by default the key's My Dashboards when it has one, else General), a range with `from` and `to`, and up to 10 `blocks`.
- When a block is refused nothing is created, and the call answers `block_refused` with the `block_index` and the reason.

#### `get_observation`

Returns the observation with `allowed_actions` and its blocks in order.

- Each block has its `kind`, `state`, `position`, `width`, `height`, `version` and a short view.
- The short view is Markdown clipped to 2,000 characters (`markdown_truncated`), a panel block's `title`, `type`, datasources, `variables` (the values it runs with) and its full `panel`, in the shape `update_block` takes, or a snapshot's `snapshot_id`.
- A snapshot block also has its frozen panel's `title`, `type` and `unit`, its `from` and `to`, and a `data` summary of at most 10 series within 1.5 KiB, each with its `name`, the `labels` that differ between the series (at most 5), `points`, `min`, `max` and `last` and the times `min_at` and `max_at`; `series_total` counts them all and `other_frames` counts the frames that are not series.
  A snapshot never carries query text, so none is shown.
  When the snapshot cannot be read, the block has `data_error` instead of the summary.
- `blocks_truncated` says the rest did not fit.
- `findings` holds the structured fields of each saved investigation answer, as in [Investigate](investigate.md#structured-fields), with the `run_id` that wrote it.
  It is left out when no answer set a field.
- A block's `allowed_actions` never lists `remove`, because no MCP tool removes a block.
- The built-in agent also has this tool, and only reads with it.

#### `list_observations`

Lists the observations in the folders the key can see, most recently changed first.
It takes an optional `folder_id`, `state` (`open` or `resolved`), a `title` that contains the given text, a `limit` (20 by default, at most 50) and a `cursor`, and pages with `next_cursor`.
The built-in agent also has this tool, and only reads with it.

#### `add_block`

Adds a block.
It takes `observation_id` and a `kind`:

- `text` takes `markdown` of at most 4,000 characters.
- `panel` takes `panel`, optionally `variables` and its own `from` and `to`.
  `variables` is an object of variable name to a list of values, for each variable the panel's queries use.
  Without `from` and `to`, the panel uses the range of the observation, else the last hour.
- `snapshot` takes `from_block_id`, the panel block to freeze.

`width` (`third`, `half`, `two_thirds` or `full`, by default `full`), `height` (120 to 960 pixels, by default the block's own) and `position` apply to every kind.
A field of another kind is refused with `invalid_request`.
A panel that uses a variable with no value in `variables` is refused with `variable_missing`, and the message names the variable.

#### `update_block`

Changes a block.

- It takes `block_id`, the `version` you read and the new `markdown`, or the new `panel` with its range.
  It optionally takes `width`, `height` (a number, or `null` for the automatic height) or `position`.
- A `height` applies to panel and snapshot blocks only, and a number for a text block is refused with `invalid_block`.
- `follow_page: true` drops a panel block's own `from` and `to`, so it shows the observation's range again.
  It goes without `panel`, `markdown`, `from` or `to`.

#### `arrange_blocks`

Sets the order and size of the page's blocks in one call.

- It takes `observation_id` and `blocks`, a list of `block_id` in the order the page should show them, each with an optional `width` and `height` (panel and snapshot blocks only).
- A block left out keeps its place after the block it followed.
  An id that is not a block of the observation refuses the call and names the id.
- It needs no `version`, and a layout change never makes a text or panel edit stale, because a block's `version` counts content only.
- The last arrangement wins, and it answers with the blocks in their new order.

#### `keep_block` and `discard_block`

Decide a block that someone who can only comment added.
Such a block arrives as `requested` and says so in `note`.
A person who can edit keeps it with `keep_block` or discards it with `discard_block`, and the one who asked for it may discard it too.

#### Block versions and states

A block's `version` is checked by `update_block` and `keep_block`.
A stale one answers `version_conflict`, so read the block with `get_observation` and apply the change to the version it names.
A transition the state does not allow, such as keeping a block that is already live, answers `invalid_transition` with the actions allowed now.
Like every agent write, a text block with HTML is refused with `html_not_allowed`.
A panel is checked like the evidence of an investigation's answer: a panel whose query fails or returns no data in its range is refused with its issues.
No tool removes a block: people do that in the web app or through the HTTP API.
Every write answers with the ids, the `state` of what it changed, the block `version` and the `url`.

#### `resolve_observation`

Closes an observation with a one-line `summary`.

#### `update_observation`

Changes the title or range of an observation.
A new title also renames the chat the observation shares its title with when you started that chat.
`chat_rename` in its result is `renamed`, or `kept` when the chat's title was left as it is.
`rename_run` on that chat's first run renames the observation and answers `observation_rename` the same way; see [Chat](chat.md#one-title-for-a-chat-and-its-observation).

#### `move_observation`

Moves an observation to another folder.

#### `reopen_observation`

Reopens a resolved observation.

#### `pin_observation`

Pins an observation to a dashboard as an annotation.

- It pins to `dashboard_id`, or by default to the dashboard its run was about, and needs edit access to that dashboard.
  Inside an agent run `dashboard_id` is required, so the run's scope sees which dashboard it changes.
- Pinning again answers with the same annotation and `created: false`.
- An observation with no dashboard to pin to answers `observation_has_no_dashboard`.

#### `make_dashboard`

Makes a dashboard in My Dashboards from an observation's live panel blocks, or from one block with `block_id`, with the variables each block carries.

- Making it again returns the dashboard you made first (`replayed: true`) unless `new` is true.
- When the observation came from your own chat and that chat is about nothing else, the chat becomes about the new dashboard.
- An observation with no live panel block answers `observation_has_no_panels`.

#### `publish_rca`

Publishes an observation as the next immutable RCA revision; see [Observations](observations.md#publish-as-rca).

- It takes `observation_id` and answers with the `revision`, the number of `blocks` and `snapshots` it holds and the `observation_url`.
- A failing panel answers `panel_failed` with the block it names.
- The other refusals are `rca_limit`, `rca_unchanged`, `observation_changed`, `snapshot_too_large` and `personal_folder`.
- It never creates a public link: a person does that, and no tool lists or revokes one either.

#### `list_rcas` and `get_rca`

List an observation's published revisions and read one, as `GET /api/v1/observations/{id}/rcas` does.
No tool withdraws a revision: people do that in the web app, through the HTTP API or with `observe o withdraw`.

### Built-in agent run tools

These tools hand work to SlashObserve's own agent and read its runs.
They act as the person who connected the client, with their role and folder access.
`start_run`, `retry_run` and `cancel_run` are over MCP only, because a run that started another would escape the cost and call limits of the first.
The built-in agent has the reads, and renames, pins, unpins and archives only runs of its own chat.

#### `start_run`

Queues a run of the built-in agent and answers with it at once.
Follow it with `get_run`.

- It takes a `prompt` (1 to 4000 characters) and optionally these arguments:
  - `kind`: `ask` (the default) or `revise`.
  - `dashboard_id`, with `base_version` for a revise.
  - `observation_id`.
  - `chat_id`, one of your chats from `list_chats` whose next turn the run is.
  - `anchor`, the spot on the dashboard or observation the prompt is about, shaped as `list_comments` shows it.
  - `block_id`, a block of `observation_id`: the one the anchor sits on, or without an anchor the whole block the prompt is about.
  - `client_id`, `new_chat`, `applies_finding_id` and `as_of`.
- Without a `dashboard_id`, an `ask` builds a new dashboard or answers in the chat.
  `kind: revise` with a `dashboard_id` changes that dashboard and saves a version.
- An `ask` with one of `dashboard_id` or `observation_id` is a [private ask](../concepts/agent.md#private-asks): it only answers, changes nothing, and is read only by you and Admins.
- `kind: investigate` is refused, because an investigation is a person's decision.
- An Admin may pass `as_of`, an RFC 3339 time or epoch milliseconds in the past, to run as of that time.
  It works for an `ask` about a dashboard or an observation and not for `revise`.
  Every relative time in the run, such as `now-1h`, resolves to it, and `get_run` and the finding give it back as `as_of`.
  See [Investigate a past incident](investigate.md#investigate-a-past-incident).
- With `kind: revise`, `applies_finding_id` in place of `prompt` makes the change an answer about that dashboard suggests.
  The answer's `suggested_change` from `get_run` is the prompt.
  While a run applying it is queued or running, or after it succeeded, the call returns your run with `replayed: true`, or answers `already_applied` for someone else's.
- Every run is a turn of a chat.
  Without `chat_id` a new chat starts, and a revise joins your latest chat on that dashboard unless `new_chat` is true.
  `new_chat` always starts a new chat and is refused with `chat_id`.
- Send a new `client_id` (a UUID) with each question.
  A resend with the same one answers with the run it started and `replayed: true`, and one with another prompt answers `client_id_reused`.
  Without `client_id` every call queues another run, so read `get_run` or `list_activity` before asking again.
- The limits answer `too_many_requests` (3 new-dashboard runs per caller), `too_many_revisions` (5 revisions per dashboard) and `agent_busy` (an instance-wide cap).
  `agent_unavailable` means the server has no model.

#### `get_run`

Reads one run of the built-in agent by the id of a `run.*` event from `list_activity`.

- The answer is the object `GET /api/v1/observe/{id}` returns: its `state`, `result_kind`, `allowed_actions`, `attempt`, the last 20 steps and its usage.
- While the run is running it also has `live_text`, the text the agent is writing now, which is never stored and is `null` once the run ends.
- A run is visible to the same people as through the API.
  An Admin can read any run; the answer then has `read_only: true` and no allowed actions.
- A question someone asked in their own chat answers `not_found` to anyone but them and Admins, unless it is an investigation or its answer was shared into a thread you can read.
- A revision from someone else's chat, other than an investigation or an answer shared into a thread you can read, has `redacted: true`, `read_only: true` and no allowed actions.
  Its `title`, `prompt`, step messages, `live_text`, `continue_note` and `error` are empty, while `failure_code` stays.
- `subject_observation_id` names the observation an ask is about, such as one asked from an observation's chat.
  It stays set after the answer is saved into another observation.
  It is null when the run is `redacted` or when you cannot open that observation.
- The run's `investigation` field says whether it is an investigation.
- The run's `finding` also carries the structured fields `causes`, `root_cause`, `onset`, `impact`, `confidence`, `ruled_out`, `next_steps` and `no_incident`, which the agent's `save_finding` sets; see [Investigate](investigate.md#structured-fields).
- `anchor` and `block_id` are the spot a run points at, shaped as `start_run` takes it.
  They are filled only for the person who asked the run, and are `null` for anyone else and for a run that points at nothing.
  The turns of `get_chat` carry the same two fields under the same rule.

When the run ended with an answer to a question, the object carries it as `finding`.
The `finding` has an `observation_id` and `url`, which are null until the answer is saved as an observation.
Only the person who asked and an Admin can read such an answer.
When a request made on Home asked to change one existing dashboard, the `finding` carries `ask_on`, the `dashboard_id` and `title` of that dashboard.
No `ask_on` means the finding names none or several.
To make that change, use `update_panels` on that dashboard yourself, or ask the built-in agent with one of these:

- `start_run` with `kind` `revise`, the finding's `ask_on.dashboard_id` as `dashboard_id` and the run's prompt.
- `observe --revise --dashboard <id>`.
- `POST /api/v1/observe` with `kind` `revise` and `dashboard_id`.

#### `get_run_build`

Reads the dashboard a run has saved so far, the object `GET /api/v1/observe/{id}/build` returns.
Only the person who started the run and an Admin can read it.

#### `cancel_run`

Stops a run the caller started.
It is safe to repeat: a run that is already canceled, succeeded or failed answers with its current state.

#### `retry_run`

Starts a succeeded, failed or canceled run again with the same prompt as a new run.

- A queued or running run, a failed or canceled run that was already retried, and a run you read `redacted` answer `invalid_transition` with `allowed_actions`.
- Investigations are a person's decision, so `retry_run` answers `investigation_person_only` for one.
- A private ask about a dashboard or an observation is not an investigation, also once its answer is saved as an observation.
  `retry_run` starts it again as a private ask about the same dashboard or observation, in the same chat.

#### `list_chats`

Lists your chats, pinned ones first and then the one most recently asked in.

- It takes `scope`, `dashboard_id`, `observation_id`, `pinned` and `q` as `GET /api/v1/chats` takes them, and a `limit` and `cursor`.
  A page has 20 chats by default and at most 50, and it pages with `next_cursor`.
- `scope: all` lists every person's chats by their last turn only, and needs an Admin.
- Each chat carries `dashboard_title`, null when you cannot see its dashboard.
  Its `observation_id` and its last turn's `subject_observation_id` are null when you cannot open that observation.

#### `get_chat`

Reads one chat with its turns oldest first, each the run as `get_run` returns it without steps.
When the turns do not all fit it answers `truncated: true` and a `next_offset` to pass as `offset`.

- A chat's owner and Admins read it and nobody else sees it.
- An Admin reads another person's chat read only, with `owner` and `read_only` saying so.
- An Admin who passes another person's `chat_id` to `start_run` gets `chat_read_only`, not `not_found`.
- Archiving a chat is left to people, with `observe chats archive` or `DELETE /api/v1/chats/{id}`.

#### `list_runs`

Lists the caller's runs, newest first with pinned ones on top.
These are the runs the web app lists as chats, with the same rules as `GET /api/v1/observe`, `PATCH /api/v1/observe/{id}`, `DELETE /api/v1/observe/{id}` and `observe requests`.

- A page has 20 runs by default and at most 50, and pages with `next_cursor`.
  The first page also carries `summary.total`, the number of runs that match, which counts a chat once per message.
- Each run has its `id`, `title`, `state`, `kind` (`build` or `investigate`), `intent` (`ask`, `apply` or `suggest`) and `investigation`.
  `investigation` is true for an investigation that writes into an observation, false for an ask.
- Each run also has `result_kind`, dashboard, observation, `anchor` and `block_id` (the spot your own question points at), `pinned_at`, `created_at` and `allowed_actions`.
  It does not carry the prompt, which `get_run` returns.
- Filters:
  - `state` (a list), `dashboard_id`, `pinned` and `q`, a word of the title, the prompt or an answer the caller may read.
  - `scope`: `all` lists everyone's runs and needs the Admin role.
  - `observation_id` keeps the runs asked about that observation and the runs that wrote into it.
  - `anchored: true` keeps the runs that point at a spot.
    It works only with the default `scope` of `mine`: with `scope: all` it answers `invalid_request`, even for an Admin.
- `anchor` and `block_id` are filled only for the person who asked the run.
  They are `null` for anyone else, such as a reader of a shared answer or an Admin reading a private chat.

#### `rename_run`, `pin_run`, `unpin_run` and `archive_run`

Work the runs the web app lists as chats.

- `rename_run` takes a `title` of 1 to 120 characters on one line, or `null` to take it back to the start of the prompt.
- `pin_run` and `unpin_run` move a run to the top of the list and back.
- `archive_run` removes the run from the list while `get_run` still reads it and everything it saved stays.
  An observation the run saved stays too, because no MCP tool deletes anything.
  A person deletes it with the chat in the web app, or with `observe requests archive <id> --delete-observation --yes`.
- Only the person or key that started a run can rename, pin, unpin or archive it.
  Anyone else gets `forbidden` on a run they can see on a dashboard and `not_found` on any other, so a chat that only its requester can read never shows it exists.
- Each is safe to repeat.

#### `save_run_answer`

Files the answer of a run as an observation.

- It takes `run_id`.
  The observation goes in the folder of the dashboard the chat was about, else in the requester's default folder (a person's My Dashboards, a key's General).
- It answers `observation_id`, `url` and `replayed`, true when the answer was already saved.
- Until then a run's `result_kind` is `answer` and its `observation_id` is null.
  It reads `observation` afterwards, in `get_run`, `list_runs`, `observe requests list --json` and `GET /api/v1/observe`.
- Only the one who asked can save it.
  An Admin gets `forbidden_role`, and anyone else and a run without an answer get `no_answer`.
- It is over MCP only: inside a run a later turn of a chat could publish an earlier private ask's answer.

#### `share_answer`

Shows the answer of a private question (`start_run` with `kind` `ask`) to the people who can read the dashboard or observation it was about.

- It takes `run_id`.
  It opens a comment thread there, at the same panel or block, with the question as your message and the agent's answer as its reply.
  On an observation it also writes the answer onto the page.
- With `thread_id` it adds both to that thread instead, which must be on the same dashboard or observation.
- It answers `thread_id`, `message_id` and `replayed`, true when the run was already shared.
  A run is shared once.
- Only the one who asked can share it, and only when you can comment there.
  Anyone else gets `not_found`, a run that is not a private question gets `not_shareable` and one without an answer gets `no_answer`.
- An answer whose evidence reads a datasource only Admins can query is refused with `restricted_answer`, unless the dashboard or observation is in a folder only Admins can read.
- The run and the rest of the chat stay readable only by you and Admins.
- Withdrawing a shared answer deletes the shared question and its answer, or the thread the share opened, so no MCP tool does it.
  The person who asked uses `observe comments withdraw` or `DELETE /threads/{id}/share`.

### Workspace tools

#### `list_folders`

Lists the folders the key can see, with `can_create` for each, and sets `truncated` when there are more than fit in the result.
The built-in agent also has it, so a request that names a folder can place the dashboard there.

- For an Admin key each folder also carries `admins_only`, true for a folder only Admins can read.
  A save that reads an Admins-only datasource, and is refused with `restricted_content`, goes in one of those.
  A key below Admin never sees the field.
- Each folder carries `audit`, true when it is the [Audit folder](activity.md#audit-folder) and false otherwise.
- No tool changes a folder's access: an Admin sets it with `observe folders permissions admins-only`.

#### `create_folder`

Creates a folder.
It takes a `title` (1 to 100 characters, unique without regard to case).
A Viewer is refused by the folder rules, an Editor or Admin may create, and the caller manages the folder they create.
It answers `folder_exists` for a taken title.

#### `rename_folder`

Renames a folder.
It takes a `folder_id` and a `title`, and needs the Manage permission on the folder.
It answers `folder_exists` for a taken title and `general_folder` for the General folder.
No tool deletes a folder or changes who may see one, because people do that in the web app or with `observe folders`.

#### `search_workspace`

Runs the [search](search.md) of the workspace.

- Pass `q`, and optionally `kinds` (`dashboard`, `annotation`, `observation`, `comment`, `saved_search`), `limit` (20 by default, at most 50) and `cursor`.
- Each result has its `kind`, `id`, `title`, a plain-text `snippet`, the `url`, the `folder` and `updated_at`.
  For a comment it also has the `target` it is on and its `thread_id`.
- It finds only what the key may read, and the built-in agent has it too.

#### `list_annotations`

Lists [annotations](annotations.md), such as deploy markers, newest first.

- It returns the global ones, plus one dashboard's own with `dashboard_id`.
  It is narrowed by `from` and `to` (RFC 3339) and `tags`.
- A page has 50 by default and at most 100.
- The built-in agent has it in every run, as the person who asked, so it can see a deploy beside a spike.

#### `create_annotation`

Adds an annotation.
It has the fields of `POST /api/v1/annotations`: `text`, `tags`, `time_from` and `time_to`, and `dashboard_id` and `panel_id` to place it on one dashboard or panel.
Without `dashboard_id` the annotation shows on every dashboard.
It needs Edit on the dashboard's folder, and an annotation with no `dashboard_id` needs the Editor role.
Inside a run it needs the `dashboard_id` of the dashboard the run is about or built, so a run never adds an annotation to every dashboard.

#### `update_annotation`

Changes the `text`, the `tags` or both of an annotation by `id`.
Only the caller who added it, or an Admin, may, and inside a run only an annotation that run added.
Its time and dashboard never change.
No tool deletes an annotation: people do that in the web app or through the HTTP API.

#### `list_activity`

Lists the [activity](activity.md) of the workspace, newest first.
It covers dashboard saves, restores, moves and deletes, annotations, findings and agent runs, each with its `actor` and the actor's `label`.

- Narrow it with `dashboard_id`, `folder_id`, `observation_id`, `actor_id`, `types`, `since` and `until` (RFC 3339).
- A page has 50 by default and at most 100, and pages with `next_cursor`.
- It lists only the events of folders the key may see, plus the [admin changes](activity.md#admin-changes) when the key is an Admin's.
- The built-in agent has it too.

#### `list_inbox`

Lists the [inbox](inbox.md) of the person who signed in to the connected agent: what asks something of them.
Each item has the `reason` and whether it is `unread`, and the result has `unread_count`.

- Pass `view` (`for_you`, `proposals`, `mentions` or `everything`, `for_you` by default), `filter`, `limit` and `cursor` from `next_cursor`.
  `filter` is `open` by default, `unread` for the open items that are unread, or `done` for the items marked done.
  `limit` is 50 by default and at most 100.
- It answers what `GET /api/v1/inbox` answers, and it does not mark anything read.
- A service-account key gets `user_session_required`.

#### `set_inbox_item_done`

Marks one inbox item done by its `seq` from `list_inbox`, or brings it back with `done: false`.
A done item leaves the default list, counts as read and stays done when the inbox is read, as the done button in the web app does.
It answers like `PUT /api/v1/inbox/items/{seq}`.
A service-account key gets `user_session_required`.

### Audit tools

Six tools read the [audit trail](../reference/audit-queries.md), and all need an Admin.
They are `search_audit_events`, `get_audit_event`, `list_audit_facets`, `get_audit_histogram`, `aggregate_audit_events` and `list_audit_fields`.

- `search_audit_events` returns 20 events a call by default and at most 100, paged with `next_cursor`.
- `get_audit_event` returns one event with the events of its request and agent run.
- The tools take `query` in KQL, `lucene:` or a Query DSL body, and `since` and `until`, the last 24 hours by default and at most 366 days.
- An answer is at most 16 KiB, so a page can hold fewer events than `limit`.
  It then says `truncated: true` with a note, and `next_cursor` continues it.
  An agent that counts events must follow the cursor until it is null.
- A call to any of them except `list_audit_fields` is recorded in the trail as `audit.search` with the query and its view.
  It is recorded once per key, view and query text in 10 minutes; see [Audit queries](../reference/audit-queries.md).
  `list_audit_fields` records nothing, and none of them changes or deletes an event.
- The export is in the CLI and the API only, because a tool answer holds at most 16 KiB.

### MCP source tools

Three tools let an agent read from the [MCP sources](../operate/mcp-sources.md) an Admin registered.
All three need the Viewer role, and a run offers them in an investigation or a question asked in a chat only, not when a dashboard is built or revised or a panel is suggested.

#### `list_mcp_sources`

Lists the MCP sources the caller may use, each with the approved tools it may call.

- A source shows its `source` (the id to pass to `call_mcp_tool`), `name`, an `about` line, a `status` when its last check failed, and its `tool_count`.
  The line is the description an Admin gave the source, or "<name>, an MCP server at <host>" when it has none.
- While the caller may use 20 approved tools or fewer in all, each tool also shows its `name`, a one-line `description`, its `arguments`, such as `query (string, required), limit (integer)`, and `read_only`.
  `read_only` is true when the source's server marks the tool as only reading, and false for a tool an Admin approved by name although the server does not mark it so; calling such a tool may change the system it reaches.
  Above that the list gives only the count for each source and a `find` line that points to `find_mcp_tools`, so the answer stays short however many tools there are.
- A source set to Admins only is listed for an Admin and left out for anyone else.
- A source whose last check failed is still listed, with a `status` that says so, and a call to it is still tried: it either works or answers `source_unavailable`.
- Only an approved tool appears.
  A tool that is waiting for approval, changed since it was approved, gone or refused never does.
- The list carries a notice that the descriptions come from a server outside SlashObserve.

#### `find_mcp_tools`

Finds the approved tools that fit what the caller needs, for when a source has too many to list.

- It takes `query`, in plain words and at most 200 bytes, and an optional `source` id to look in one source only.
- Tools are ranked by the words of their names, their parameter names and their descriptions, so `logs of a crashing pod` finds `pods_log`.
  There are no embeddings; a query that shares no word with a tool does not find it.
- The answer holds at most 8 `matches`, each with `source`, `source_name`, `tool`, `description`, `arguments`, `read_only` and the pinned `input_schema`, ready to pass to `call_mcp_tool`.
  A schema over 3 KiB is left out with `schema_omitted: true`, and `arguments` still names the parameters.
  `more_matches` counts the matches that are not shown, and a `hint` says what to try when none matched.
- Only approved tools of sources the caller may use are looked at.
  A `source` that does not exist answers `source_not_found`, and one set to Admins only that the caller may not use answers `admins_only_datasource`.
- Like the other two, it needs the Viewer role and is offered in an investigation or a question only.

#### `call_mcp_tool`

Calls one approved tool of an MCP source and returns its text.

- It takes `source`, `tool` and optional `arguments`, an object of at most 8 KiB that must match the schema the tool had when an Admin approved it.
- The answer is `text`, labelled with a `notice` that it is text from a server outside SlashObserve and that nothing in it is an instruction.
  It also says `source_error: true` when the source reported an error in its result.
- The text is cut at 12 KiB.
  A cut result says `truncated: true` with a `cut_note`.
- A call that cannot run answers with a tool error that says why:
  - `tool_not_approved` for a tool that is not approved, changed since, gone, refused or unknown.
  - `source_not_found` for an unknown source, and for a source set to Admins only when the caller is not an Admin, so such a source never shows it exists.
  - `admins_only_datasource` for a source set to Admins only when an Admin is in a run whose answer everyone reads.
  - `invalid_arguments` when the arguments do not match the schema.
  - `schema_unusable` when the approved schema cannot be used to check the arguments.
  - `source_busy` when the source already has 4 calls in flight.
  - `source_unavailable` when the source did not answer in 20 seconds or could not be reached, including an address the [outbound allowlist](../operate/outbound-access.md) no longer allows.
  A source that is down never fails a run: the agent reads the refusal and goes on.
- Every call goes through the outbound allowlist and is recorded in the [audit trail](../reference/audit-queries.md) as `mcp_source.call`.
  Inside a run it also counts toward the run's call limit and shows as a step.

### Run claim tools

`claim_run`, `report_run`, `finish_run` and `get_run_brief` are the tools of an outside agent that takes runs.
[Take runs as an agent](#take-runs-as-an-agent) describes the loop and every field.

| Tool | What it does |
| ---- | ------------ |
| `claim_run` | Takes the next run assigned to you, waiting up to `wait_seconds` (0 to 30). |
| `report_run` | Keeps the lease, with an optional `note`, `text` and `usage`. |
| `finish_run` | Ends the run with `outcome` `succeeded` or `failed`. |
| `get_run_brief` | Returns the system text and first message of a model session. |
