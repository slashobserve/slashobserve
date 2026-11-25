---
title: How the agent builds a dashboard
description: The tool loop the built-in agent runs, what a save checks, whose permissions it uses, and the limits, guards and failure codes of a run.
---

The built-in agent turns a prompt into a saved dashboard version by calling tools against your datasources.
It writes no dashboard text of its own.
The only dashboard a run produces is a complete [ObserveSpec](observespec.md) document saved through a tool, and a run that only answers a question ends in a saved finding.
The agent is on only when an LLM provider is configured; see [LLM providers](../connect/llm-providers.md).

For example, you type "show checkout error rate and p95 latency by route" in the chat home.
The agent lists the real metrics and labels in your Prometheus, drafts one panel, runs its query to see that it returns data, and saves it.
It repeats that for each panel, and when it finishes, the panels become version 1 of a new dashboard in your My Dashboards.

A run starts from a prompt in the chat home, from `observe <prompt>`, from a follow-up on an existing dashboard, or from a comment that asks for a revision.
The conversation page shows the steps the tools report and the result.
While a run works, it also shows the short sentences the model writes before its tool calls, live.
They are never stored, so once the run ends only the result stays.
An agent that reads the run over MCP or the CLI sees the same text as `live_text` while the run is running.

## Watch, stop and keep a run

The person who asked can watch the run, cancel it, continue it and keep what it built.
[Chat](../use/chat.md#watch-the-build) shows these controls in the web app.

1. Watch.
   The conversation page lists each step.
   A queued request shows its place in the queue, such as "Queued, position 3".
2. Cancel.
   Only the person who asked can cancel a queued or running request.
   A queued request is canceled at once, and a running one stops within a few seconds, after the model or tool call in flight returns.
3. Continue.
   The person who asked can continue a stopped build.
   It starts a new run from the panels already built, with an optional note for the agent.
4. Keep.
   A run that fails writes no version and keeps its build for 7 days.
   **Save what was built** saves the build as a new dashboard, or as the next version when nobody saved the dashboard since the build started.
   The request stays failed and keeps its failure code.

A canceled run keeps its build like any other stopped run, and a version saved before the cancel took effect still counts.
A stopped build is saved or continued, never both.
A run the server restarts continues from its build.

Each server runs `SLASHOBSERVE_AGENT_WORKERS` requests at once, 8 by default, and the rest wait in the queue.
A free worker takes the waiting request whose requester has the fewest runs in progress, then the oldest.
One request that applies a change to a dashboard runs at a time, so other revisions of that dashboard wait behind a run that does not end, and nobody else can cancel it.
Investigations and questions do not wait on the dashboard.

## The tool loop

The model starts with these:

- The prompt.
- The configured datasources and whether each is healthy.
- The authoring guide and an example.
- For a revision, the current version of the dashboard.

It then works in this order: discover, draft, lint, run, save, and repair if the save is rejected.

Details of the authoring guide:

- It is a core plus one section per datasource type: Prometheus, ClickHouse, Loki, Tempo, Jaeger, and one shared by Elasticsearch and OpenSearch.
- The system text carries only the sections of the configured types.
- An Admin's runs also get a section on the built-in audit datasource, which no one else can read.
- The JSON Schema is not in that text.
  The model reads it on demand with `get_spec_guide`, which inside a run returns only the schema.

Details of a revision's current version:

- When that version is over 16 KiB as compact JSON, the model gets an outline instead.
- The outline has the title, default time range, refresh, tags, links and variables in full.
- It has one line for each panel with its id, type, title, row and position.
- It has the full JSON of the panel the request was asked about and of any panel whose id the prompt names.
- The model reads the JSON of other panels with `get_panels`.

Details of an anchored question:

- When the question points at a spot on a panel or an observation block, the brief names it in one sentence, such as `The request points at "the row payments, authorize_card, card declined (440), last hour".`
- The words after "points at" are the anchor's description, as `list_comments` shows it in `at`, with the values the person saw there when the web sent them.
- The whole is quoted, cut at 500 characters, and has `&`, `<` and `>` written as `&amp;`, `&lt;` and `&gt;` so it cannot open or close a tag.
- It is treated as data, never as an instruction, and a block is named by its id.
- A run with no anchor gets no such sentence.
- The server refuses an anchor with control, bidirectional override or hidden characters in any of its text, as it does for a thread message.

### Discover

The model calls independent discovery tools together in one turn, and uses only names it found in this run or that are already in the dashboard.
What each tool finds depends on the datasource:

| Datasource | Tools and what they find |
| --- | --- |
| Prometheus and the OpenTelemetry tables of ClickHouse | `list_metrics`, `list_labels` and `list_label_values` find real metric names, label names and label values. |
| ClickHouse | `get_schema` and `sample_rows` list the tables and columns and read a few rows. |
| Loki | `list_labels` and `list_label_values` find stream labels and values. `sample_rows` reads lines. `list_metrics` lists stream label keys. |
| Jaeger | `list_label_values` finds services and the operations of one service. `get_schema` lists every service with its span names. Tags are free-form. |
| Tempo | `list_labels` and `list_label_values` find TraceQL attributes and values. `get_schema` lists every service with its span names, plus the attributes. |
| Elasticsearch and OpenSearch | `list_labels` and `list_label_values` find fields and a field's top values. `get_schema` lists indices or the fields of one. `sample_rows` reads documents. |

Details of the Loki tools:

- `list_labels` and `list_label_values` are narrowed by a stream selector in `metric`.
- `sample_rows` reads up to 20 lines of the last 15 minutes from every stream or from a stream selector in `table`.
- `list_metrics` with a stream selector as `scope` lists each stream label key with the number of streams that carry it.

Details of the Elasticsearch and OpenSearch tools:

- `get_schema` lists the indices of the index pattern or, with a `table`, the fields of one index.
- `sample_rows` reads up to 20 of the newest documents.

These tools work across datasources:

- `list_services` lists the services a log or trace datasource has seen in a recent window, one hour by default and at most six hours, or finds which datasources hold one, and names the field a query filters on.
- On logs, `log_facets` counts the top values of each field a query matches, `log_patterns` groups the matching lines into patterns and `log_context` reads the lines around one.
  `logs_for_trace` reads the lines that carry a trace id.
- On traces, `get_trace` reads one trace, `trace_stats` counts spans, errors and durations over a filter and `compare_traces` ranks what differs between two sets of traces.
- `list_annotations` shows deploy markers beside a spike, `list_folders` names the folders a request may place a dashboard in, and `list_datasources` says whether each source is healthy.

For a new dashboard, the model first calls `list_metrics` with a `scope` that covers the request, such as `{job="checkout"}` or a ClickHouse table.
It gets every metric family in the scope in one result, with its type, unit, label keys and series count.
Go, process, `promhttp_` and scrape runtime families and `up` are only counted, unless the scope names `__name__`.
It plans one panel per family, or a table for a group of related families, and sets `planned_panels` from that plan.
A family the request covers ends in a panel, or in the `summary` of the last save with the reason it was left out.

### Draft, lint and run

The model drafts the panels.
`lint_query` parses each new or changed query and reports syntax and policy problems without running it.
`query_range` and `query_instant` run a query and return a compact summary, so the model can check that a query returns data.
These ad hoc queries run with tighter limits than a saved dashboard; see [Datasources file](../reference/datasources-file.md).

### Save

| Tool | Saves |
| --- | --- |
| `create_dashboard` | A new dashboard with a full document. |
| `update_panels` | A new version of an existing dashboard, from a patch that holds only what changed. |
| `update_dashboard` | A new version from a full document, for rewriting a dashboard whole. |
| `import_grafana_dashboard` | A Grafana dashboard that the prompt contains. |
| `get_panels` | Nothing. It reads the full JSON of up to 20 panels by id, with the variables, before the model changes them. |

Details of the `update_panels` patch:

- It holds the panels to add or replace, the ids to delete, moves, the variables, and a `dashboard` field for the default `time`, the `title`, the `tags` and the `links`.
- The server merges the patch into the version it builds on and validates the whole result, so changing one panel of a large dashboard costs a small save.
- A key present in `dashboard` replaces that value whole, and a key left out keeps the current one.

When the prompt contains Grafana dashboard JSON, the model calls `import_grafana_dashboard` instead of rewriting it.
The agent may import only JSON that the prompt contains.

### Repair

The model repairs the panels the server names when it rejects a save, as [What a save checks](#what-a-save-checks) describes.

### Advice returned with a save

Every accepted save, into a build or by an MCP client, also returns advice that never rejects a save:

- `uncovered` names up to 50 families of the caller's last scoped inventory that no panel query names, and `uncovered_total` counts them all.
  A histogram, summary or counter is covered when a query names any of its series.
- `notes` lists up to 20 hints.
  They are a time series or bar chart query with no `legend`, a stat, gauge, bar gauge or time series with no `unit`, and a stat with no `description`.
  On a build save or an `update_panels` save only, they also name a query, custom, textbox, interval or constant variable that an earlier version used and this save left without a user.
- A save into a build lists the panel hints only for the panels it added or changed, and the finished version lists them for the whole dashboard.

The model is told to finish only when `uncovered` is empty or the `summary` of its last save says why each family in it was left out.

## The build

A run does not save versions as it goes.
Every accepted `create_dashboard`, `update_dashboard` or `update_panels` goes into a build, an unsaved copy of the dashboard that only the person who asked can watch.
Each save into the build is checked like a version save, so every panel's queries are verified as the panel arrives.

The build becomes one version when the agent finishes, either with `finish` set on its last save or with an answer that has no tool call.
When the requester can no longer edit the dashboard, it becomes a [proposal](proposals.md) instead; see [When the requester cannot edit](#when-the-requester-cannot-edit).
Nothing shows in the dashboard list, a folder or the history before that, and an intermediate panel never becomes a version.

### One panel per save

The agent builds every dashboard panel by panel.
A new dashboard has no id until the build commits, so the model saves and reads its build without a `dashboard_id`.

1. The first save is `create_dashboard` with the title, time range, variables and one panel.
   `row` names the row the panel belongs to, and `planned_panels` (1 to 200) is the number of panels the model expects the finished dashboard to have.
2. Every other panel is its own `update_panels` call, with the panel alone in `upsert` and `row` naming its row.
   A new row is created with its first panel.
3. A save that adds or changes more than one panel is rejected.

A revision changes one panel per `update_panels` call, so a change to many existing panels takes one call each.
Deletes, moves, variables and dashboard fields may go with the panel.
Each save checks only the panel it adds or changes, so a build of 100 panels does not rerun the queries of the panels already built.

`planned_panels`, `finish` and `row` exist only for the built-in agent, and MCP clients keep one version per accepted save.
The planned count is an estimate.
The web app shows it beside the number of panels built, and a finished build commits even when fewer panels were built than planned.
A save that changes the planned count says so in its step, and the finish save's step names the panel it added.

### Panel placement

The server places each new panel at the first free spot of its row.
It scans from the row's top line left to right, and moves what is below only when the row grows.
In a build the server owns every position:

- It reads only `w` and `h` from a panel the agent sends.
- It keeps a panel the agent sends again where it is.
- It places a panel again only when its size or row changes, at the spot a `layout` entry asks for when that spot is free, else at the first free spot of its row.

The agent may leave a new panel's size out, or send only its width.
The server then uses the panel type's default size.
It gives the panel the width of a same-type panel whose line it continues and the height of the line it joins, and a full-width time series 7 units of height.
A size the agent sends is kept.

When a new dashboard's build becomes a version, whether the agent finished it or a person saved what was built, the server tidies the layout:

- It widens the panels of every line that does not fill 24 columns, keeping their order, row and height.
- It puts the legend of a full-width time series with several series on the right unless the agent set one.

A revision's lines are left as they are, and rows keep the place the server gave them.

`update_dashboard` in a build keeps the build's position for every panel it already holds and places a new panel the same way.
Each accepted save tells the model the rows of the build, the positions in the row it saved into and any gaps, within 1.5 KiB, so it never needs a read to know what exists.
It also names the free end of that row's last line and its height, so the model can size the next panel to fill it.

### Saving early

The first save must come early.
From the sixth model call since the session started, its history was shortened, or the last accepted save, reads are refused and only a save can run.
This limit of six is for a revision and for a chat message that has started building a dashboard.
The call before gets a note to save next.

- A new message in a chat with no dashboard or observation, which may be a question, gets the investigation limit instead of 6 until its first panel is saved (12 calls by default, with the note one call before).
  Its note and its refusal name the first panel for a dashboard request and `save_finding` otherwise.
  After its first accepted save, including an import, six calls apply to each panel.
- A suggest run, which proposes one panel for the panel editor, gets 12 calls instead of 6, with the note at the 11th.
- An investigation, which saves a finding, gets 12 calls by default, with the note one call before.
  `SLASHOBSERVE_AGENT_INVESTIGATION_CALLS` sets it from 6 to 60 for investigations that need longer, on the server and for connected agents alike.
- Reads in the same call as a save always run, and so do the reads of the call right after a rejected save, so the model can look into why a save was rejected.
- Two turns in a row in which every call was refused end the run with `no_progress`.

### Answers without a tool call

- If the model answers without a tool call while fewer panels than planned are built, its answer is dropped and its conversation is shortened the same way as after a cut-off, with a note that it is told once.
  Its next answer without a tool call saves what is built.
- If the model answers without a tool call in a new-dashboard or revision request that has saved nothing, it is told once that its text is not kept and to call `save_finding`, which keeps its answer in the chat, or save a panel.
  Its next answer without a tool call ends the run with `no_tool_call`.

### Cut-offs and shortened conversations

If a model call is cut off at the output limit or runs past the per-call time limit, the agent shortens its conversation and goes on.
It keeps the last few calls, the build and the names it already discovered.
A second cut-off with no accepted save in between ends the run with `output_limit` or `call_timeout`.

After an accepted save, once the conversation passes `SLASHOBSERVE_AGENT_SESSION_RENEW_KIB` (256 KiB by default), it is shortened the same way, so a long build stays inside the model's context window.

Details of shortening:

- It keeps the last 4 turns of the conversation, up to 32 KiB, and cuts their older tool results to 2 KiB.
- It puts the build, the reason and a list of up to 4 KiB of the metric, label and table names the agent found in front of them, so the agent does not discover them again.
- The names are sent to the model and never written to the server's logs.
- Older read results are also left out of what is sent once they add up to 64 KiB, while the last 4 turns stay in full.
- Only the model's view is shortened: the run keeps everything it read.
- A shortened conversation is not a fresh session, and neither has a count of its own that ends the run.

## Limits

A run has no cap on model calls and runs one save per turn.
It ends when it finishes, is canceled, reaches `SLASHOBSERVE_AGENT_RUN_TIMEOUT` or `SLASHOBSERVE_AGENT_RUN_MAX_COST_USD` when that is set, loses its claim on the request, or stops making progress.
Three rejected saves in a row end it.

### Guards

The guards steer before they stop.
Every refusal and every stop shows as a step that names its rule, such as "Refused query_range: ..." or "Stopped: ...".

Details of repeated calls:

- A run that keeps asking a read it already made is first answered, then refused.
- A successful read is answered once more from its first result with a note.
- A failed read or a query result runs again, and a third identical call is refused.
- A save identical to an earlier one is refused.

Details of a finding:

- When a read was refused because the run reached its window of model calls, the finding the run then saves ends with a line saying reading stopped early and how many reads were refused, so its counts are not taken for complete.
- A finding that says a metric is missing is sent back once when no `list_metrics` call of the run set `scope`.
- A finding that says there was no deploy is sent back once when the run did not read `list_annotations`.

Details of the time zone:

- A request may name the person's time zone, as `time_zone` on `POST /api/v1/observe`, which the web app always sends.
- The brief tells the model to write times in it with its label.
- `save_finding` answers `wrong_time_zone` for a finding that labels times UTC without ever using that label.
- A retry or continue keeps the zone, and without one times stay UTC.

Details of reads that could not be checked:

- A datasource read that ends in `query_timeout`, `datasource_unavailable` or `too_many_requests` is marked as could not check.
- Its error carries a `note` telling the model the read says nothing about whether the data exists, and its step reads "could not check".
- The run remembers the datasource until the same tool reads it successfully, so the finding ends with a line naming each datasource that could not be checked and why.
- An empty `list_metrics` search carries a `note` that a service, job or pod is usually a label value, to be listed with `scope` before the model says a service has no metrics.

Details of what stops a run:

- A run that is stopped after three rejected saves names the panels that kept failing in its reason.
- An investigation or a question answered with a finding says "the finding was rejected in 3 saves in a row" instead.
  It names the evidence panels that failed (`evidence-1` is the first panel sent) and the last issue codes.
  It adds that a datasource did not answer in time when every issue was a timeout, an unavailable datasource or a busy server.
- A finding rejected for such evidence tells the model to send it again without those panels and to say that their data was not read.
- A save that changes no panel, changes a panel for the fifth time, or deletes a panel and adds another, gets a note, and the fourth such save in a row ends the run with `no_progress`.

The server enforces these guards at its MCP tool boundary on every run session, so an agent you connect that takes runs meets the same guards; see [MCP](../use/mcp.md#the-run-session-enforces-the-guards).

### Tool results and failures

- A tool result over its size limit goes back to the model as a `result_too_large` error that names the parameters that narrow that tool's answer, such as `search` and `limit` for `list_metrics` or `get_panels` instead of `get_dashboard`.
- A save result is never an error for its size, because the save already happened.
  Its panel list is cut to fit and marked `truncated`.
- A tool that fails, such as a datasource that does not answer, goes back to the model as a `tool_failed` error without the error's own text, and the run goes on.
- A tool call ends the run only on a broken internal invariant: a cancellation, a lost claim on the build, or a tool that panicked (`tool_panicked`).

### Timeouts and model calls

A run has no time limit by default.
`SLASHOBSERVE_AGENT_RUN_TIMEOUT` sets one of at least `1m`.

Model calls stream.
An attempt that receives nothing for `SLASHOBSERVE_LLM_IDLE_TIMEOUT` (180 s by default, `0` for none) is retried with backoff, like a rate limit or a server error, up to 4 attempts within `SLASHOBSERVE_LLM_REQUEST_TIMEOUT`.
A single model call, retries included, is stopped after 10 minutes by default.
`SLASHOBSERVE_LLM_REQUEST_TIMEOUT` sets that limit and `0` removes it.

There is no cap on model calls, but `SLASHOBSERVE_AGENT_RUN_MAX_COST_USD` can bound what one attempt spends.
When an attempt's estimated cost has reached it, the agent starts no further model call and the request fails with `budget_exhausted`, keeping its build.
The call that crossed the limit has already run its tool calls, so a finishing save is kept.
See [Run cost](../connect/llm-providers.md#run-cost).

More details of a run:

- The first call of a session, before any save to the build, runs one reasoning step higher than `SLASHOBSERVE_LLM_EFFORT`: `low` becomes `medium`, `medium` becomes `high`, and any other value stays.
- Every other call caps its output at half of `SLASHOBSERVE_LLM_MAX_TOKENS`, but at least 8192 and never above it.
  A call cut off at that lower cap is sent once more at the full value.
- While a run is in flight, the server renews its claim on the request every 5 seconds, so a long run is not picked up by a second worker.
- A run that ends without an accepted save writes no version.
- A run shows its most recent 1000 steps per attempt.

## What a save checks

Before it stores a version, the server runs the queries itself, whatever the model checked earlier.
Besides validating the document, it runs queries over the dashboard's default time range and with each variable at its default, hidden queries included.
A default range longer than 400 days is checked over its last 400 days, the longest range the query routes accept, so the save is not refused for it and no check query reads more.

### Which queries run

Which queries run depends on the tool:

- `create_dashboard` and `update_dashboard` run every query of every panel and every query variable.
- `update_panels` runs only the queries that changed against the version it builds on.
  These are the panels whose query text, datasource, type, `timeFrom`, `timeShift`, `maxDataPoints` or `interval` changed, the panels that use a variable whose query changed, and the variables that changed.
  A new panel counts as changed, but an edit of only a panel's title or position, or a change to `tags` or `links`, runs nothing.
  A panel the patch did not touch is not run again, so a query that has started failing since the last save does not block a patch that leaves it alone.
- `update_panels` runs every query when it changes the default `time` to a different `from` or `to`, because each panel's check depends on that range.
  A panel that returns nothing over the new range rejects the save.
  It runs every query too when it changes the title of a dashboard whose queries use `$__dashboard`.
  The model is told to keep such a panel and widen the range, not delete the panel to make the save pass.

### Rejections

The save is rejected when:

- a query fails, or a datasource uid is not configured.
- none of the queries of a panel returns data, except on an `alertlist` panel, where no firing alerts is normal, and on a panel that reads only the audit trail.
  A signed-in person can keep such a panel from the [panel editor](../use/edit-panel.md#save-as-a-new-version), marked "Unverified"; the agent, MCP clients and the CLI cannot.
- the document breaks a rule of the format, such as overlapping panels or an undeclared variable.

A panel or variable the save did not change is not rejected when it only times out, for instance a heavy panel over a longer time range.
It is saved as "Unverified" with the timeout as its reason, and so are the panels that read a variable that timed out.
A panel the save did not change is also saved as "Unverified" when it returns no data after the save lengthened the default time range, because the longer range did not empty it.
A panel the save changed still has to pass.

The rejection lists each problem with the path of the panel or variable and a code.
Every rejection in a run lists `failing_panels`, the ids of the panels the problems belong to, with a line that says how to repair them.
The step of a rejected save names the first problem's path, and a refused input names the reason.
The model fixes only the panels it names and saves again.
Three rejected saves in a row end the run with an error that names the last issue codes, and an accepted save into the build resets the count.

### Changes made meanwhile

When the dashboard changed while the agent was revising it, the server moves the build onto the latest version.
It does so at the commit, or as soon as a new session would start on the older build.
That works as long as the build changed no dashboard setting and none of the panels the newer versions changed.
The agent's panels are checked again on the latest version, and a commit that hit the change is retried at once.

Otherwise the build is discarded and the run restarts once from the latest version.
If it changes again, the run fails with `conflict`.

### Revisions

A revision starts from the current document and keeps panel ids, positions and everything the prompt does not ask to change, so [comments](versions.md#comments) stay attached to their panels.
A comment on a panel asks to change that panel unless it says otherwise.

The save tools reject a document that does any of these:

- It creates or edits a placeholder panel.
- It writes HTML tags in a text panel.
- It adds or changes the actions of a canvas element, a panel's fields or an override; see [Maps, canvases and actions](observespec.md#maps-canvases-and-actions).

The model is told not to delete an unverified panel or its queries to make a save pass; see [Import Grafana dashboards](../use/import-grafana.md).

## Whose permissions it uses

The agent acts as the person or key that asked.
The server checks the request when it is queued and again when a worker starts it.
The saved version records the requester as its author and the request that asked for it, and the version menu on the dashboard shows "Agent" as the one who saved it.
The tool-level roles do not apply inside a run, because the requester was already checked.

Each run carries an `intent`:

| Intent | Does |
| --- | --- |
| `ask` | Reads and answers. |
| `apply` | Changes a dashboard. |
| `suggest` | Proposes one panel. |

A new-dashboard request is an `ask` until it saves its dashboard, and then an `apply`.
Run listings show the `intent` and the run's `kind`, `build` or `investigate`; a private ask lists as `investigate`.
Each run also carries `investigation`, true when it was started as an investigation that opens or writes into an observation, and false for an ask from the Chat tab, the canvas, a thread or Ask this on an observation.
It is set when the run is queued and never changes: saving, sharing, showing on a page or withdrawing an answer leaves it as it was.

### Where a new dashboard goes

A new dashboard goes to the requester's My Dashboards, where every person has Edit, or to General for a key.
A request can name a folder the requester can create in: the agent reads the folders with `list_folders` and passes one as `folder_id`.
A folder it cannot create in is refused and never replaced by another.

### Changing a dashboard from Home

A request on Home that asks to change a dashboard that already exists is answered with a finding, because a Home run only builds new dashboards.

1. The agent finds the dashboard with `list_dashboards` by the id or exact title in the request.
2. When exactly one matches, it passes that dashboard's id as `ask_on_dashboard_id` on `save_finding`, and the server stores it only if the requester can view the dashboard.
3. The finding then carries `ask_on`, the dashboard's `dashboard_id` and `title`, so the web app can start a revision there with the same prompt.
4. When none or several match, the finding lists the candidates and carries no `ask_on`.

A run never changes its own mode, and only a run on Home takes `ask_on_dashboard_id`.

### Who may ask

- A follow-up, a revision or a comment sent to the agent needs Comment on the dashboard's folder.
  A person who can comment but not edit may ask, and their change becomes a [proposal](proposals.md) that someone who can edit merges; see [Folders and access](../teams/folders.md).
- A [suggestion](../use/edit-panel.md) for one panel still needs Edit.
- An investigation that writes into an observation needs the Editor role.
  A question asked in a thread on an observation needs only Comment on it and is answered in the thread, writing nothing on the observation's page, whoever asks it.
- These runs count toward the same limit as any other, at most 5 revisions queued or running on a dashboard.

### Private asks

A private ask is a question about one dashboard or one observation, asked in a chat of yours.
Start it with `observe investigate --private`, `start_run` or `POST /observe` with `kind` `ask` (the default) and a `dashboard_id` or `observation_id`, or `observe --dashboard <id>`.
It needs Comment on what it is about, and it does not count toward the investigation limits.

It writes nothing anyone else reads:

- No observation is opened or written, and no dashboard version is saved.
- Its activity events are seen only by you and Admins.
- Its run and its answer are read only by you and, read only, by Admins.

#### Share the answer

Once it has an answer you can share it with `observe share-answer <request-id>`, `share_answer`, or `POST /observe/{id}/share`.

1. The server opens a comment thread on the dashboard or observation, at the same panel, block or spot.
   Your question is your message and the agent's answer is its reply.
2. On an observation it also writes the answer onto the page.
3. With a thread id, the two are added to that thread instead, which must be on the same dashboard or observation.
4. Everyone who can read the thread then reads the answer and its evidence.
   They read the run itself, read only, with `GET /observe/{id}`, `get_run` or `observe status`, while the chat stays yours.

Details of a shared answer:

- Their reads are recorded in the audit trail like yours.
- Someone who can reply in the thread may apply the answer's suggested change there.
- The run names that thread as `shared_thread_id`, which only you see.
- A run is shared once, and sharing needs Comment there.
- An answer whose evidence reads a datasource only Admins can query, or whose run read one, can only be shared on content in a folder only Admins can read, and is otherwise refused with `restricted_answer`.

#### Withdraw the answer

While nobody else has written in that thread, you can withdraw the answer back to only you.
Use `observe comments withdraw <thread-id>` or `DELETE /threads/{id}/share`.
It deletes the shared question and its answer, or the whole thread when the share opened it, and makes the run a private ask again, which you can share later.
On an observation the run and its answer leave the observation's lists.
The answer's text that was written onto the page stays there as page content until an editor removes it.
No MCP tool withdraws, because none deletes.

### When the requester cannot edit

The server checks the requester once more when the build commits.
When they can still comment on the dashboard but can no longer edit it, or the run acts in propose mode, the build is saved as a [proposal](proposals.md) and not as a version.

- The proposal belongs to the run, so a second attempt of the same run adds a revision to it and does not open another.
- The run succeeds with `result_kind: proposal` and no `result_version`.
- The comment that asked for it stays open, because nothing was saved.
- Someone who can edit the dashboard merges or discards the proposal, and the run's last step says "Proposed for an Editor to merge".
- **Save what was built** still needs Edit, so it is not offered to a requester who can only comment.

### Tools in a run

The server has 110 tools: MCP clients get 109, every one except `save_finding`, which only a run offers.
The built-in agent gets 30 in a new-dashboard or revision request and 29 in a suggestion, which leaves out `save_finding`.
An investigation is offered only the read tools and `save_finding`.

The other 80 tools are for MCP clients only, so the agent never writes or resolves a comment reply, never edits an observation's blocks, never merges a proposal and never claims a run.
They are:

- the comment, observation, proposal, RCA, inbox, saved search, pin, home dashboard and claim tools;
- the dashboard version, move and duplicate tools;
- `create_folder`, `rename_folder`, `create_annotation`, `update_annotation`, `list_activity`, `list_dashboard_health`, `list_dashboard_viewers`, `list_mentionable_people`, `request_comment_access`, `list_access_requests` and `recheck_datasource`;
- `get_run`, `get_run_build`, `start_run`, `cancel_run`, `retry_run`, `list_runs`, `rename_run`, `pin_run`, `unpin_run`, `archive_run`, `list_chats` and `get_chat`;
- `run_dashboard_health`, the six audit trail tools, `save_run_answer`, `share_answer`, `apply_suggested_change`, `search_workspace` and `draft_query`.

[MCP](../use/mcp.md) lists each with its role.
`list_observations` and `get_observation` are offered to the built-in agent in every run, so it can read the observations that earlier answers were saved into, as the person who asked and only the ones they may open.
An investigation cites what it used by the observation's title and `/o/<id>`.

Its tool descriptions and input schemas are the ones MCP clients read, except where a run behaves differently:

- The run names the dashboard and its version, so the agent's `update_dashboard` and `update_panels` take no `dashboard_id` or `parent_version`.
  `get_panels` without `dashboard_id` reads the dashboard being worked on, its build included.
- Its saves take `finish`, `planned_panels` and `row`, and its `update_panels` takes one panel.
- Its `create_dashboard` takes `folder_id` only on the first save of a build, which the server keeps with the build until it becomes a version.
- Inside a run `get_spec_guide` returns only the JSON Schema, because the guide and the example are already in the agent's system text.
  Over MCP it returns the guide with the sections of the configured types, the schema and the example.

When a revision request comes from a comment, the server resolves that thread itself once a version is saved.
When a message that mentions an agent started the run, the server instead posts one status reply in that thread when the run ends, whatever the outcome, and leaves the thread as it is.
A reply that could not be posted, because the dashboard was locked or the write failed, is tried again every minute for up to a day.
The agent also reads the thread's earlier messages with the request.

### Data sent to the provider

The agent sends what its tools return to the LLM provider, including names, label values, sampled rows and query results.
See [LLM providers](../connect/llm-providers.md).

## The agent process

The server starts `observe-agent` as a child process, which claims the built-in agent's runs and works them through the same MCP tools as a connected agent does.
The server enforces the guards and the limits and stores the saved result.
The claim answer to the built-in agent carries the model settings, which no other claimer receives, so the sidecar needs no provider configuration of its own.

The sidecar reaches the server only over the loopback address, with a token that is new for every start of the process.
The server refuses that token on any request that carries a forwarding header (`Forwarded`, `X-Forwarded-For`, `X-Forwarded-Host`, `X-Forwarded-Proto` or `X-Real-Ip`).
A reverse proxy on the same host that adds none of them, such as nginx with a bare `proxy_pass`, still reaches the server from loopback.
So configure it to send `X-Forwarded-For` or keep `/mcp` off it.

When an LLM provider is configured, the server looks for `observe-agent` on `PATH`.
It does not start when the binary is missing or when `SLASHOBSERVE_HTTP_ADDR` names a host other than a loopback address or all addresses.
Without a provider it makes neither check.

## MCP clients use their own model

An MCP client such as Claude Code runs the loop itself, with its own model, against the same tools at `<public url>/mcp`.
The server needs no LLM provider for that.
Each call is checked against the role of the key's service account and the folders it can see, and a save goes through the same validation and query check as above.
The client can also read and answer comments, which the built-in agent cannot.
A version saved this way records `mcp` as its origin and the client's name.
See [MCP](../use/mcp.md) for the tools and the setup.

## Reference

### Failure codes

A failed run records one failure code beside its message, in `failure_code` on the request.
Every request records `trigger`, what started it: `person` when the requester asked, and `mention` when a mention of the Observe Agent in a comment did.

| Code | Meaning |
| --- | --- |
| `run_timeout` | The whole run ran out of time. |
| `call_timeout` | One model call did not answer in time. |
| `rate_limited` | The provider answered 429 after its retries. |
| `provider_error` | Any other provider or network failure. |
| `output_limit` | The model ran out of output tokens. |
| `no_tool_call` | The model finished without saving. |
| `refused` | The model refused. |
| `validation_exhausted` | Three rejected saves in a row. |
| `restricted_answer` | Three saves in a row were refused because the run read data only Admins can read. They do not count toward `validation_exhausted`. |
| `no_progress` | The run stopped making progress. |
| `budget_exhausted` | The run reached the server's cost limit. |
| `conflict` | The dashboard changed twice while the agent was revising it. |
| `lease_expired` | The server lost track of the run, such as after a restart, or its save found its build taken over by a newer attempt. |

Runs from v0.1.0 may show `turn_limit`, a model-call cap later versions removed.

### Usage fields

The request's `usage` reports these:

| Field | Meaning |
| --- | --- |
| `failed_calls` | Model calls that failed. |
| `retries` | Extra HTTP attempts inside model calls. |
| `tool_calls` | Tool calls made. |
| `model_ms`, `tool_ms` | Time spent in model calls and in tools. |
| `max_save_output_tokens` | The most output tokens of one save. |
| `build_saves` | Saves into the build. |
| `compactions` | Times the session history was shortened, including after an early stop with panels still planned. |
| `renewals` | Compactions after the history passed the renewal size. |
| `nudges` | Steering messages before reads are refused. |
| `save_now_refusals` | Reads refused for lack of a save. |
| `stalled_turns` | Turns in which every call was refused. |
| `one_panel_refusals` | Saves refused for changing more than one panel. |
| `cost_usd` | The estimated cost in US dollars of the attempt that finished the request, from the server's price table. It is `null` when the model has no price. |

A run that fails on its first model call has `usage` with `model_calls` 0 and `failed_calls` 1.
See [Run cost](../connect/llm-providers.md#run-cost) for the price table.

### Settings that shape a run

| Variable | Default | Effect |
| --- | --- | --- |
| `SLASHOBSERVE_AGENT_WORKERS` | 8 | Requests one server runs at once. |
| `SLASHOBSERVE_AGENT_RUN_TIMEOUT` | none | Time limit of a run, at least `1m`. |
| `SLASHOBSERVE_AGENT_RUN_MAX_COST_USD` | none | Cost limit of one attempt. |
| `SLASHOBSERVE_AGENT_SESSION_RENEW_KIB` | 256 | Conversation size after which it is shortened, from 32 to 4096. |
| `SLASHOBSERVE_AGENT_INVESTIGATION_CALLS` | 12 | Model calls an investigation gets before only a save runs, from 6 to 60. |
| `SLASHOBSERVE_LLM_IDLE_TIMEOUT` | 180 s | Time without output after which a call is retried, `0` for none. |
| `SLASHOBSERVE_LLM_REQUEST_TIMEOUT` | 10 minutes | Time limit of one model call, retries included, `0` for none. |
| `SLASHOBSERVE_LLM_EFFORT` | `medium` on Anthropic, nothing sent on OpenAI | Reasoning effort. |
| `SLASHOBSERVE_LLM_MAX_TOKENS` | 16000 | Output limit of a model call. |

[Configuration](../reference/configuration.md) describes every variable.
