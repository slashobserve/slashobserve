---
title: How the agent builds a dashboard
description: The tool loop the built-in agent runs, what a save checks, and whose permissions it uses.
---

The built-in agent turns a prompt into a saved dashboard version by calling tools against your datasources.
It writes no dashboard text of its own.
The only thing a run produces is a complete [ObserveSpec](observespec.md) document saved through a tool.
The agent is on only when an LLM provider is configured (see [LLM providers](../connect/llm-providers.md)).

A run starts from a prompt in the chat home, from `observe <prompt>`, from a follow-up on an existing dashboard, or from a comment that asks for a revision.
The conversation page shows the steps the tools report and the result, never the model's own text.

## The tool loop

The model starts with the prompt, the configured datasources and whether each is healthy, the authoring guide and an example.
The authoring guide is a core plus one section per datasource type, Prometheus, ClickHouse, Loki and Tempo, and the system text carries only the sections of the configured types.
The JSON Schema is not in that text: the model reads it on demand with `get_spec_guide`, which inside a run returns only the schema.
For a revision it also gets the current version of the dashboard.
When that version is over 16 KiB as compact JSON, it gets an outline instead: the title, default time range, refresh, tags, links and variables in full, one line for each panel with its id, type, title, row and position, and the full JSON of the panel it was asked about and of any panel whose id the prompt names.
The model reads the JSON of other panels with `get_panels`.
It then works in this order:

1. **Discover.**
   `list_metrics`, `list_labels` and `list_label_values` find real metric names, label names and label values on Prometheus and on the OpenTelemetry tables of ClickHouse.
   `get_schema` and `sample_rows` list the tables and columns of a ClickHouse datasource and read a few rows.
   On Loki, `list_labels` and `list_label_values` find stream labels and their values, narrowed by a stream selector in `metric`, `sample_rows` reads up to 20 lines of the last 15 minutes from every stream or from a stream selector in `table`, and `list_metrics` with a stream selector as `scope` lists each stream label key with the number of streams that carry it.
   On Tempo, `list_labels` and `list_label_values` find TraceQL attributes and their values, and `get_schema` lists every service with its span names, plus the attributes.
   The model calls independent discovery tools together in one turn, and uses only names it found in this run or that are already in the dashboard.
   For a new dashboard, the model first calls `list_metrics` with a `scope` that covers the request, such as `{job="checkout"}` or a ClickHouse table, and gets every metric family in it in one result, with its type, unit, label keys and series count.
   Go, process, `promhttp_` and scrape runtime families and `up` are only counted, unless the scope names `__name__`.
   It plans one panel per family, or a table for a group of related families, and sets `planned_panels` from that plan.
   A family the request covers ends in a panel, or in the `summary` of the last save with the reason it was left out.
2. **Draft** the panels.
3. **Lint.**
   `lint_query` parses each new or changed query and reports syntax and policy problems without running it.
4. **Run.**
   `query_range` and `query_instant` run a query and return a compact summary, so the model can check a query returns data.
   These ad hoc queries run with tighter limits than a saved dashboard (see [Datasources file](../reference/datasources-file.md)).
5. **Save.**
   `create_dashboard` saves a new dashboard with a full document.
   `update_panels` saves a new version of an existing dashboard from a patch that holds only what changed: the panels to add or replace, the ids to delete, moves, the variables, and a `dashboard` field for the default `time`, the `title`, the `tags` and the `links`.
   The server merges the patch into the version it builds on and validates the whole result, so changing one panel of a large dashboard costs a small save.
   A key present in `dashboard` replaces that value whole, and a key left out keeps the current one.
   `get_panels` reads the full JSON of up to 20 panels by id, with the variables, before the model changes them.
   `update_dashboard` saves a new version from a full document and is for rewriting a dashboard whole.
   When the prompt contains Grafana dashboard JSON, the model calls `import_grafana_dashboard` instead of rewriting it, and the agent may import only JSON that the prompt contains.
6. **Repair**, if the save is rejected.

Every accepted save, into a build or by an MCP client, also returns advice that never rejects a save:

- `uncovered` names up to 50 families of the caller's last scoped inventory that no panel query names, and `uncovered_total` counts them all; a histogram, summary or counter is covered when a query names any of its series.
- `notes` lists up to 20 hints: a time series or bar chart query with no `legend`, a stat, gauge, bar gauge or time series with no `unit`, and a stat with no `description`.
  A save into a build lists them only for the panels it added or changed, and the finished version lists them for the whole dashboard.

The model is told to finish only when `uncovered` is empty or the `summary` of its last save says why each family in it was left out.

## The build

A run does not save versions as it goes.
Every accepted `create_dashboard`, `update_dashboard` or `update_panels` goes into a build, an unsaved copy of the dashboard that only the person who asked can watch (see [Chat](../use/chat.md#watch-the-build)).
Each save into the build is checked like a version save, so every panel's queries are verified as the panel arrives.
The build becomes one version when the agent finishes, either with `finish` set on its last save or with an answer that has no tool call.
Nothing shows in the dashboard list, a folder or the history before that, and an intermediate panel never becomes a version.

The agent builds every dashboard panel by panel.
A new dashboard has no id until the build commits, so the model saves and reads its build without a `dashboard_id`.
The first save is `create_dashboard` with the title, time range, variables and one panel, with `row` naming the row it belongs to, plus `planned_panels`, the number of panels the model expects the finished dashboard to have.
Every other panel is its own `update_panels` call, with the panel alone in `upsert` and `row` naming the row it belongs to; a new row is created with its first panel.
A save that adds or changes more than one panel is rejected.
The server places each new panel at the first free spot of its row, scanning from the row's top line left to right, and moves what is below only when the row grows.
In a build the server owns every position: it reads only `w` and `h` from a panel the agent sends, keeps a panel the agent sends again where it is, and places a panel again only when its size or row changes, at the spot a `layout` entry asks for when that spot is free, else at the first free spot of its row.
The agent may leave a new panel's size out, or send only its width: the server then uses the panel type's default size, gives the panel the width of a same-type panel whose line it continues and the height of the line it joins, and a full-width time series 7 units of height.
A size the agent sends is kept.
When a new dashboard's build becomes a version, whether the agent finished it or a person saved what was built, the server widens the panels of every line that does not fill 24 columns, keeping their order, row and height, and puts the legend of a full-width time series with several series on the right unless the agent set one.
A revision's lines are left as they are.
Rows keep the place the server gave them.
`update_dashboard` in a build keeps the build's position for every panel it already holds and places a new panel the same way.
Each accepted save tells the model the rows of the build, the positions in the row it saved into and any gaps, within 1.5 KiB, so it never needs a read to know what exists.
It also names the free end of that row's last line and its height, so the model can size the next panel to fill it.
A revision changes one panel per `update_panels` call, so a change to many existing panels takes one call each; deletes, moves, variables and dashboard fields may go with the panel.
Each save checks only the panel it adds or changes, so a build of 100 panels does not rerun the queries of the panels already built.
`planned_panels`, `finish` and `row` exist only for the built-in agent; MCP clients keep one version per accepted save.
The planned count is an estimate: the web app shows it beside the number of panels built, and a finished build commits even when fewer panels were built than planned.
A save that changes the planned count says so in its step, and the finish save's step names the panel it added.

The first save must come early.
From the sixth model call since the session started, its history was shortened, or the last accepted save, reads are refused and only a save can run; the call before gets a note to save next.
A suggest run, which proposes one panel for the panel editor, gets 12 calls instead of 6, with the note at the 11th.
Reads in the same call as a save always run, and so do the reads of the call right after a rejected save, so the model can look into why a save was rejected.
Two turns in a row in which every call was refused end the run with `no_progress`.

If the model answers without a tool call while fewer panels than planned are built, its answer is dropped and its conversation is shortened the same way as after a cut-off, with a note that it is told once; its next answer without a tool call saves what is built.
If the model answers without a tool call in a new-dashboard or revision request that has saved nothing, it is told once that its text is not shown and to call `save_finding` or save a panel; its next answer without a tool call ends the run with `no_tool_call`.
If a model call is cut off at the output limit or runs past the per-call time limit, the agent shortens its conversation to the last few calls, the build and the names it already discovered, and goes on; a second cut-off with no accepted save in between ends the run with `output_limit` or `call_timeout`.
After an accepted save, once the conversation passes `SLASHOBSERVE_AGENT_SESSION_RENEW_KIB` (256 KiB by default), it is shortened the same way, so a long build stays inside the model's context window.
Shortening keeps the last 4 turns of the conversation, up to 32 KiB, and cuts their older tool results to 2 KiB.
It puts the build, the reason and a list of up to 4 KiB of the metric, label and table names the agent found in front of them, so the agent does not discover them again.
The names are sent to the model and never written to the server's logs.
Older read results are also left out of what is sent once they add up to 64 KiB, while the last 4 turns stay in full.
Only the model's view is shortened: the run keeps everything it read.
A shortened conversation is not a fresh session, and neither has a count of its own that ends the run.

A run that fails writes no version and keeps its build for 7 days.
The person who asked can keep it with **Save what was built**, which saves the build as a new dashboard, or as the next version when nobody saved the dashboard since the build started.
The request stays failed and keeps its failure code.
A run the server restarts continues from its build.

## Limits

A run has no cap on model calls and runs one save per turn.
It ends when it finishes, is canceled, reaches `SLASHOBSERVE_AGENT_RUN_TIMEOUT` when that is set, loses its claim on the request, or stops making progress.
Three rejected saves in a row end it.
The guards steer before they stop, and every refusal and every stop shows as a step that names its rule, such as "Refused query_range: ..." or "Stopped: ...".
A run that keeps asking a read it already made is first answered, then refused: a successful read is answered once more from its first result with a note, a failed read or a query result runs again, and a third identical call is refused.
A save identical to an earlier one is refused.
A tool result over its size limit goes back to the model as a `result_too_large` error that names the parameters that narrow that tool's answer, such as `search` and `limit` for `list_metrics` or `get_panels` instead of `get_dashboard`.
A save result is never an error for its size, because the save already happened: its panel list is cut to fit and marked `truncated`.
A tool that fails, such as a datasource that does not answer, goes back to the model as a `tool_failed` error without the error's own text, and the run goes on.
A tool call ends the run only on a broken internal invariant: a cancellation, a lost claim on the build, or a tool that panicked (`tool_panicked`).
On an evals stack, `SLASHOBSERVE_AGENT_LOG_TOOL_INPUTS` logs each tool input by hash, size, panel ids and the issue paths of a rejected save, never by its text; see [Configuration](../reference/configuration.md).
A save that changes no panel, or changes a panel for the fifth time, gets a note, and the fourth such save in a row ends the run with `no_progress`.
Only the person who asked can cancel a queued or running request.
A queued request is canceled at once, and a running one stops within a few seconds, after the model or tool call in flight returns.
A canceled run keeps its build like any other stopped run, and a version saved before the cancel took effect still counts.
The person who asked can also continue a stopped build: it starts a new run from the panels already built, with an optional note for the agent.
A stopped build is saved or continued, never both.
A queued request shows its place in the queue, such as "Queued, position 3".
One request per dashboard runs at a time, so other revisions of a dashboard wait behind a run that does not end, and nobody else can cancel it.
A run has no time limit by default; `SLASHOBSERVE_AGENT_RUN_TIMEOUT` sets one of at least `1m`.
Model calls stream.
An attempt that receives nothing for `SLASHOBSERVE_LLM_IDLE_TIMEOUT` (180 s by default, `0` for none) is retried with backoff, like a rate limit or a server error, up to 4 attempts within `SLASHOBSERVE_LLM_REQUEST_TIMEOUT`.
A single model call, retries included, is stopped after 10 minutes by default; `SLASHOBSERVE_LLM_REQUEST_TIMEOUT` sets that limit and `0` removes it.
The first call of a new build runs one reasoning step higher than `SLASHOBSERVE_LLM_EFFORT`, and later calls cap their output at half of `SLASHOBSERVE_LLM_MAX_TOKENS` (at least 8192), resending a cut call once at the full cap.
While a run is in flight the server renews its claim on the request every 5 seconds, so a long run is not picked up by a second worker.
A run that ends without an accepted save writes no version.
A run shows its most recent 1000 steps per attempt.

A failed run records one failure code beside its message, in `failure_code` on the request.
The codes you will see most are `run_timeout` (the whole run ran out of time), `call_timeout` (one model call did not answer in time), `rate_limited` (the provider answered 429 after its retries), `provider_error` (any other provider or network failure), `output_limit` (the model ran out of output tokens), `no_tool_call` (the model finished without saving), `refused`, `validation_exhausted` (three rejected saves in a row), and `no_progress` (the run stopped making progress).
A run the server lost track of, such as after a restart, or whose save found its build taken over by a newer attempt, is `lease_expired`.
Runs from v0.1.0 may show `turn_limit`, a model-call cap later versions removed.
The request's `usage` also reports `failed_calls`, `retries` (extra HTTP attempts inside model calls), `tool_calls`, `model_ms`, `tool_ms`, `max_save_output_tokens`, `build_saves` (saves into the build), `compactions` (times the session history was shortened, including after an early stop with panels still planned), `renewals` (compactions after the history passed the renewal size), `nudges` (steering messages before reads are refused), `save_now_refusals` (reads refused for lack of a save), `stalled_turns` (turns in which every call was refused) and `one_panel_refusals` (saves refused for changing more than one panel).
A run that fails on its first model call has `usage` with `model_calls` 0 and `failed_calls` 1.

## What a save checks

Before it stores a version, the server runs the queries itself, whatever the model checked earlier.
Besides validating the document, it runs queries over the dashboard's default time range and with each variable at its default, hidden queries included.
Which queries run depends on the tool:

- `create_dashboard` and `update_dashboard` run every query of every panel and every query variable.
- `update_panels` runs only the queries that changed against the version it builds on: the panels whose query text, datasource, type, `timeFrom`, `timeShift`, `maxDataPoints` or `interval` changed, the panels that use a variable whose query changed, and the variables that changed.
  A new panel counts as changed, but an edit of only a panel's title or position, or a change to `tags` or `links`, runs nothing.
  A panel the patch did not touch is not run again, so a query that has started failing since the last save does not block a patch that leaves it alone.
- `update_panels` runs every query when it changes the default `time` to a different `from` or `to`, because each panel's check depends on that range, and a panel that returns nothing over the new range rejects the save.
  It runs every query too when it changes the title of a dashboard whose queries use `$__dashboard`.
  The model is told to keep such a panel and widen the range, not delete the panel to make the save pass.

The save is rejected when:

- a query fails, or a datasource uid is not configured.
- none of the queries of a panel returns data, except on an `alertlist` panel, where no firing alerts is normal.
- the document breaks a rule of the format, such as overlapping panels or an undeclared variable.

The rejection lists each problem with the path of the panel or variable and a code.
Every rejection in a run lists `failing_panels`, the ids of the panels the problems belong to, with a line that says how to repair them.
The step of a rejected save names the first problem's path, and a refused input names the reason.
The model fixes only the panels it names and saves again.
Three rejected saves in a row end the run with an error that names the last issue codes, and an accepted save into the build resets the count.

When the dashboard changed while the agent was revising it, the server moves the build onto the latest version, at the commit or as soon as a new session would start on the older build, as long as the build changed no dashboard setting and none of the panels the newer versions changed.
The agent's panels are checked again on the latest version, and a commit that hit the change is retried at once.
Otherwise the build is discarded and the run restarts once from the latest version.
If it changes again, the run fails.

A revision starts from the current document and keeps panel ids, positions and everything the prompt does not ask to change, so [comments](versions.md#comments) stay attached to their panels.
A comment on a panel asks to change that panel unless it says otherwise.
The save tools reject a document that creates or edits a placeholder panel, writes HTML tags in a text panel, or adds or changes the actions of a canvas element, a panel's fields or an override (see [Maps, canvases and actions](observespec.md#maps-canvases-and-actions)), and the model is told not to delete an unverified panel or its queries to make a save pass (see [Import Grafana dashboards](../use/import-grafana.md)).

## Whose permissions it uses

The agent acts as the person or key that asked.
The server checks the request when it is queued and again when a worker starts it.
A new dashboard goes to the requester's My Dashboards, where every person has Edit, or to General for a key; a follow-up or revision needs Edit on the dashboard's folder (see [Folders and access](../teams/folders.md)).

The saved version records the requester as its author and the request that asked for it, and the version menu on the dashboard shows "Agent" as the one who saved it.
The tool-level roles do not apply inside a run, because the requester was already checked.

The server has 24 tools: MCP clients get 23, and the built-in agent gets 18, the 17 it shares with MCP clients plus `save_finding`, which every run except a suggestion offers.
The 17 shared tools are the 11 above, plus `update_panels`, `get_panels`, `get_spec_guide`, `list_datasources`, `list_dashboards` and `get_dashboard`.
The comment tools, `list_folders` and `create_finding` are for MCP clients only, so the agent never writes or resolves a comment reply.
The agent reads earlier findings with `list_findings`, as MCP clients do.
Its tool descriptions and input schemas are the ones MCP clients read, except where a run behaves differently.
The run names the dashboard and its version, so the agent's `update_dashboard` and `update_panels` take no `dashboard_id` or `parent_version`, and `get_panels` without `dashboard_id` reads the dashboard being worked on, its build included.
Its saves take `finish`, `planned_panels` and `row`, its `update_panels` takes one panel, and its `create_dashboard` has no `folder_id`.
Inside a run `get_spec_guide` returns only the JSON Schema, because the guide and the example are already in the agent's system text; over MCP it returns the guide with the sections of the configured types, the schema and the example.
When a revision request comes from a comment, the server resolves that thread itself once a version is saved.

The agent sends what its tools return to the LLM provider, including names, label values, sampled rows and query results (see [LLM providers](../connect/llm-providers.md)).

## MCP clients use their own model

An MCP client such as Claude Code runs the loop itself, with its own model, against the same tools at `<public url>/mcp`.
The server needs no LLM provider for that.
Each call is checked against the role of the key's service account and the folders it can see, and a save goes through the same validation and query check as above.
The client can also read and answer comments and pick a folder, which the built-in agent cannot.
A version saved this way records `mcp` as its origin and the client's name.
See [MCP](../use/mcp.md) for the tools and the setup.
