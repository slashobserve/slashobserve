---
title: How the agent builds a dashboard
description: The tool loop the built-in agent runs, what a save checks, and whose permissions it uses.
---

The built-in agent turns a prompt into a saved dashboard version by calling tools against your datasources.
It writes no dashboard text of its own.
The only thing a run produces is a complete [ObserveSpec](observespec.md) document saved through a tool.
The agent is on only when an LLM provider is configured (see [LLM providers](../connect/llm-providers.md)).

A run starts from a prompt in the chat home, from `observe <prompt>`, from a follow-up on an existing dashboard, or from a comment that asks for a revision.
The conversation page shows the steps the tools report and the result.
While a run works, it also shows the short sentences the model writes before its tool calls, live; they are never stored, so once the run ends only the result stays.
An agent that reads the run over MCP or the CLI sees the same text as `live_text` while the run is running.

## The tool loop

The model starts with the prompt, the configured datasources and whether each is healthy, the authoring guide and an example.
The authoring guide is a core plus one section per datasource type, Prometheus, ClickHouse, Loki, Tempo, Jaeger, and one shared by Elasticsearch and OpenSearch, and the system text carries only the sections of the configured types.
An Admin's runs also get a section on the built-in audit datasource, which no one else can read.
The JSON Schema is not in that text: the model reads it on demand with `get_spec_guide`, which inside a run returns only the schema.
For a revision it also gets the current version of the dashboard.
When the question points at a spot on a panel or an observation block, the brief names it in one sentence, such as `The request points at "the row payments, authorize_card, card declined (440), last hour".`
The words after "points at" are the anchor's description, as `list_comments` shows it in `at`, with the values the person saw there when the web sent them; the whole is quoted, cut at 500 characters, has `&`, `<` and `>` written as `&amp;`, `&lt;` and `&gt;` so it cannot open or close a tag, and is treated as data, never as an instruction, and a block is named by its id.

A run with no anchor gets no such sentence.
The server refuses an anchor with control, bidirectional override or hidden characters in any of its text, as it does for a thread message.

When that version is over 16 KiB as compact JSON, it gets an outline instead: the title, default time range, refresh, tags, links and variables in full, one line for each panel with its id, type, title, row and position, and the full JSON of the panel it was asked about and of any panel whose id the prompt names.
The model reads the JSON of other panels with `get_panels`.

It then works in this order: discover, draft, lint, run, save, and repair if the save is rejected.

### Discover

- `list_metrics`, `list_labels` and `list_label_values` find real metric names, label names and label values on Prometheus and on the OpenTelemetry tables of ClickHouse.
- `list_services` lists the services a log or trace datasource has seen in a recent window, one hour by default and at most six, or finds which datasources hold one, and names the field a query filters on.
- `get_schema` and `sample_rows` list the tables and columns of a ClickHouse datasource and read a few rows.
- On Loki, `list_labels` and `list_label_values` find stream labels and their values, narrowed by a stream selector in `metric`, `sample_rows` reads up to 20 lines of the last 15 minutes from every stream or from a stream selector in `table`, and `list_metrics` with a stream selector as `scope` lists each stream label key with the number of streams that carry it.
- On Jaeger, `list_label_values` finds services, and the operations of one service, and `get_schema` lists every service with its span names; tags are free-form.
- On Tempo, `list_labels` and `list_label_values` find TraceQL attributes and their values, and `get_schema` lists every service with its span names, plus the attributes.
- On Elasticsearch and OpenSearch, `list_labels` and `list_label_values` find the fields you can group or filter by and a field's top values, `get_schema` lists the indices of the index pattern or, with a `table`, the fields of one index, and `sample_rows` reads up to 20 of the newest documents.
- On logs, `log_facets` counts the top values of each field a query matches, `log_patterns` groups the matching lines into patterns and `log_context` reads the lines around one.
  `logs_for_trace` reads the lines that carry a trace id.
- On traces, `get_trace` reads one trace, `trace_stats` counts spans, errors and durations over a filter and `compare_traces` ranks what differs between two sets of traces.
- `list_annotations` shows deploy markers beside a spike, `list_folders` names the folders a request may place a dashboard in, and `list_datasources` says whether each source is healthy.

The model calls independent discovery tools together in one turn, and uses only names it found in this run or that are already in the dashboard.

For a new dashboard, the model first calls `list_metrics` with a `scope` that covers the request, such as `{job="checkout"}` or a ClickHouse table, and gets every metric family in it in one result, with its type, unit, label keys and series count.
Go, process, `promhttp_` and scrape runtime families and `up` are only counted, unless the scope names `__name__`.
It plans one panel per family, or a table for a group of related families, and sets `planned_panels` from that plan.
A family the request covers ends in a panel, or in the `summary` of the last save with the reason it was left out.

### Draft, lint and run

The model drafts the panels.
`lint_query` parses each new or changed query and reports syntax and policy problems without running it.
`query_range` and `query_instant` run a query and return a compact summary, so the model can check a query returns data.
These ad hoc queries run with tighter limits than a saved dashboard (see [Datasources file](../reference/datasources-file.md)).

### Save

`create_dashboard` saves a new dashboard with a full document.
`update_panels` saves a new version of an existing dashboard from a patch that holds only what changed: the panels to add or replace, the ids to delete, moves, the variables, and a `dashboard` field for the default `time`, the `title`, the `tags` and the `links`.
The server merges the patch into the version it builds on and validates the whole result, so changing one panel of a large dashboard costs a small save.
A key present in `dashboard` replaces that value whole, and a key left out keeps the current one.

`get_panels` reads the full JSON of up to 20 panels by id, with the variables, before the model changes them.
`update_dashboard` saves a new version from a full document and is for rewriting a dashboard whole.
When the prompt contains Grafana dashboard JSON, the model calls `import_grafana_dashboard` instead of rewriting it, and the agent may import only JSON that the prompt contains.

### Repair

The model repairs the panels the server names when it rejects a save, as [What a save checks](#what-a-save-checks) describes.

### Advice returned with a save

Every accepted save, into a build or by an MCP client, also returns advice that never rejects a save:

- `uncovered` names up to 50 families of the caller's last scoped inventory that no panel query names, and `uncovered_total` counts them all; a histogram, summary or counter is covered when a query names any of its series.
- `notes` lists up to 20 hints: a time series or bar chart query with no `legend`, a stat, gauge, bar gauge or time series with no `unit`, a stat with no `description`, and, on a build save or an `update_panels` save only, a query, custom, textbox, interval or constant variable that an earlier version used and this save left without a user.
  A save into a build lists the panel hints only for the panels it added or changed, and the finished version lists them for the whole dashboard.

The model is told to finish only when `uncovered` is empty or the `summary` of its last save says why each family in it was left out.

## The build

A run does not save versions as it goes.
Every accepted `create_dashboard`, `update_dashboard` or `update_panels` goes into a build, an unsaved copy of the dashboard that only the person who asked can watch (see [Chat](../use/chat.md#watch-the-build)).
Each save into the build is checked like a version save, so every panel's queries are verified as the panel arrives.
The build becomes one version when the agent finishes, either with `finish` set on its last save or with an answer that has no tool call.
When the requester can no longer edit the dashboard, it becomes a [proposal](proposals.md) instead (see [When the requester cannot edit](#when-the-requester-cannot-edit)).
Nothing shows in the dashboard list, a folder or the history before that, and an intermediate panel never becomes a version.

### One panel per save

The agent builds every dashboard panel by panel.
A new dashboard has no id until the build commits, so the model saves and reads its build without a `dashboard_id`.
The first save is `create_dashboard` with the title, time range, variables and one panel, with `row` naming the row it belongs to, plus `planned_panels`, the number of panels the model expects the finished dashboard to have.
Every other panel is its own `update_panels` call, with the panel alone in `upsert` and `row` naming the row it belongs to; a new row is created with its first panel.
A save that adds or changes more than one panel is rejected.

A revision changes one panel per `update_panels` call, so a change to many existing panels takes one call each; deletes, moves, variables and dashboard fields may go with the panel.
Each save checks only the panel it adds or changes, so a build of 100 panels does not rerun the queries of the panels already built.

`planned_panels`, `finish` and `row` exist only for the built-in agent; MCP clients keep one version per accepted save.
The planned count is an estimate: the web app shows it beside the number of panels built, and a finished build commits even when fewer panels were built than planned.
A save that changes the planned count says so in its step, and the finish save's step names the panel it added.

### Panel placement

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

### Saving early

The first save must come early.
From the sixth model call since the session started, its history was shortened, or the last accepted save, reads are refused and only a save can run; the call before gets a note to save next.
A suggest run, which proposes one panel for the panel editor, gets 12 calls instead of 6, with the note at the 11th.
An investigation, which saves a finding, gets 12 calls by default, with the note one call before; `SLASHOBSERVE_AGENT_INVESTIGATION_CALLS` sets it from 6 to 60 for investigations that need longer, on the server and for connected agents alike.
Reads in the same call as a save always run, and so do the reads of the call right after a rejected save, so the model can look into why a save was rejected.
Two turns in a row in which every call was refused end the run with `no_progress`.

### Answers without a tool call

If the model answers without a tool call while fewer panels than planned are built, its answer is dropped and its conversation is shortened the same way as after a cut-off, with a note that it is told once; its next answer without a tool call saves what is built.
If the model answers without a tool call in a new-dashboard or revision request that has saved nothing, it is told once that its text is not kept and to call `save_finding`, which keeps its answer in the chat, or save a panel; its next answer without a tool call ends the run with `no_tool_call`.

### Cut-offs and shortened conversations

If a model call is cut off at the output limit or runs past the per-call time limit, the agent shortens its conversation to the last few calls, the build and the names it already discovered, and goes on; a second cut-off with no accepted save in between ends the run with `output_limit` or `call_timeout`.
After an accepted save, once the conversation passes `SLASHOBSERVE_AGENT_SESSION_RENEW_KIB` (256 KiB by default), it is shortened the same way, so a long build stays inside the model's context window.
Shortening keeps the last 4 turns of the conversation, up to 32 KiB, and cuts their older tool results to 2 KiB.

It puts the build, the reason and a list of up to 4 KiB of the metric, label and table names the agent found in front of them, so the agent does not discover them again.
The names are sent to the model and never written to the server's logs.
Older read results are also left out of what is sent once they add up to 64 KiB, while the last 4 turns stay in full.
Only the model's view is shortened: the run keeps everything it read.
A shortened conversation is not a fresh session, and neither has a count of its own that ends the run.

### Failed runs

A run that fails writes no version and keeps its build for 7 days.
The person who asked can keep it with **Save what was built**, which saves the build as a new dashboard, or as the next version when nobody saved the dashboard since the build started.
The request stays failed and keeps its failure code.
A run the server restarts continues from its build.

## Limits

A run has no cap on model calls and runs one save per turn.
It ends when it finishes, is canceled, reaches `SLASHOBSERVE_AGENT_RUN_TIMEOUT` or `SLASHOBSERVE_AGENT_RUN_MAX_COST_USD` when that is set, loses its claim on the request, or stops making progress.
Three rejected saves in a row end it.

### Guards

The guards steer before they stop, and every refusal and every stop shows as a step that names its rule, such as "Refused query_range: ..." or "Stopped: ...".
A run that keeps asking a read it already made is first answered, then refused: a successful read is answered once more from its first result with a note, a failed read or a query result runs again, and a third identical call is refused.
A save identical to an earlier one is refused.
When a read was refused because the run reached its window of model calls, the finding the run then saves ends with a line saying reading stopped early and how many reads were refused, so its counts are not taken for complete.

A finding that says a metric is missing is sent back once when no `list_metrics` call of the run set `scope`, and one that says there was no deploy is sent back once when the run did not read `list_annotations`.
A request may name the person's time zone (`time_zone` on `POST /api/v1/observe`, which the web app always sends): the brief tells the model to write times in it with its label, and `save_finding` answers `wrong_time_zone` for a finding that labels times UTC without ever using that label; a retry or continue keeps the zone, and without one times stay UTC.
A datasource read that ends in `query_timeout`, `datasource_unavailable` or `too_many_requests` is marked as could not check: its error carries a `note` telling the model the read says nothing about whether the data exists, its step reads "could not check", and the run remembers the datasource until the same tool reads it successfully, so the finding ends with a line naming each datasource that could not be checked and why.
An empty `list_metrics` search carries a `note` that a service, job or pod is usually a label value, to be listed with `scope` before the model says a service has no metrics.

A run that is stopped after three rejected saves names the panels that kept failing in its reason.
An investigation or a question answered with a finding says "the finding was rejected in 3 saves in a row" instead, names the evidence panels that failed (`evidence-1` is the first panel sent) and the last issue codes, and adds that a datasource did not answer in time when every issue was a timeout, an unavailable datasource or a busy server.
A finding rejected for such evidence tells the model to send it again without those panels and to say that their data was not read.
A save that changes no panel, or changes a panel for the fifth time, gets a note, and the fourth such save in a row ends the run with `no_progress`.
The server enforces these guards at its MCP tool boundary on every run session, so an agent you connect that takes runs meets the same guards (see [MCP](../use/mcp.md#the-run-session-enforces-the-guards)).

### Tool results and failures

A tool result over its size limit goes back to the model as a `result_too_large` error that names the parameters that narrow that tool's answer, such as `search` and `limit` for `list_metrics` or `get_panels` instead of `get_dashboard`.
A save result is never an error for its size, because the save already happened: its panel list is cut to fit and marked `truncated`.
A tool that fails, such as a datasource that does not answer, goes back to the model as a `tool_failed` error without the error's own text, and the run goes on.
A tool call ends the run only on a broken internal invariant: a cancellation, a lost claim on the build, or a tool that panicked (`tool_panicked`).

### Cancel, continue and queueing

Only the person who asked can cancel a queued or running request.
A queued request is canceled at once, and a running one stops within a few seconds, after the model or tool call in flight returns.
A canceled run keeps its build like any other stopped run, and a version saved before the cancel took effect still counts.

The person who asked can also continue a stopped build: it starts a new run from the panels already built, with an optional note for the agent.
A stopped build is saved or continued, never both.

A queued request shows its place in the queue, such as "Queued, position 3".
Each server runs `SLASHOBSERVE_AGENT_WORKERS` requests at once (8 by default); the rest wait in the queue in the order they were made.
One request per dashboard runs at a time, so other revisions of a dashboard wait behind a run that does not end, and nobody else can cancel it.

### Timeouts and model calls

A run has no time limit by default; `SLASHOBSERVE_AGENT_RUN_TIMEOUT` sets one of at least `1m`.
Model calls stream.
An attempt that receives nothing for `SLASHOBSERVE_LLM_IDLE_TIMEOUT` (180 s by default, `0` for none) is retried with backoff, like a rate limit or a server error, up to 4 attempts within `SLASHOBSERVE_LLM_REQUEST_TIMEOUT`.
A single model call, retries included, is stopped after 10 minutes by default; `SLASHOBSERVE_LLM_REQUEST_TIMEOUT` sets that limit and `0` removes it.

There is no cap on model calls, but `SLASHOBSERVE_AGENT_RUN_MAX_COST_USD` can bound what one attempt spends.
When an attempt's estimated cost (see [Run cost](../connect/llm-providers.md#run-cost)) has reached it, the agent starts no further model call and the request fails with `budget_exhausted`, keeping its build; the call that crossed the limit has already run its tool calls, so a finishing save is kept.

The first call of a new build runs one reasoning step higher than `SLASHOBSERVE_LLM_EFFORT`, and later calls cap their output at half of `SLASHOBSERVE_LLM_MAX_TOKENS` (at least 8192), resending a cut call once at the full cap.
While a run is in flight the server renews its claim on the request every 5 seconds, so a long run is not picked up by a second worker.
A run that ends without an accepted save writes no version.
A run shows its most recent 1000 steps per attempt.

### Failure codes and usage

A failed run records one failure code beside its message, in `failure_code` on the request.
Every request records `trigger`, what started it; `person` when the requester asked, and `mention` when a mention of the Observe Agent in a comment did.

The codes you will see most are:

- `run_timeout` (the whole run ran out of time)
- `call_timeout` (one model call did not answer in time)
- `rate_limited` (the provider answered 429 after its retries)
- `provider_error` (any other provider or network failure)
- `output_limit` (the model ran out of output tokens)
- `no_tool_call` (the model finished without saving)
- `refused`
- `validation_exhausted` (three rejected saves in a row)
- `no_progress` (the run stopped making progress)
- `budget_exhausted` (the run reached the server's cost limit)
- `conflict` (the dashboard changed twice while the agent was revising it)

A run the server lost track of, such as after a restart, or whose save found its build taken over by a newer attempt, is `lease_expired`.
Runs from v0.1.0 may show `turn_limit`, a model-call cap later versions removed.

The request's `usage` also reports:

- `failed_calls`
- `retries` (extra HTTP attempts inside model calls)
- `tool_calls`
- `model_ms`
- `tool_ms`
- `max_save_output_tokens`
- `build_saves` (saves into the build)
- `compactions` (times the session history was shortened, including after an early stop with panels still planned)
- `renewals` (compactions after the history passed the renewal size)
- `nudges` (steering messages before reads are refused)
- `save_now_refusals` (reads refused for lack of a save)
- `stalled_turns` (turns in which every call was refused)
- `one_panel_refusals` (saves refused for changing more than one panel)
- `cost_usd` (the estimated cost in US dollars of the attempt that finished the request, from the server's price table; `null` when the model has no price, see [Run cost](../connect/llm-providers.md#run-cost))

A run that fails on its first model call has `usage` with `model_calls` 0 and `failed_calls` 1.

## What a save checks

### Which queries run

Before it stores a version, the server runs the queries itself, whatever the model checked earlier.
Besides validating the document, it runs queries over the dashboard's default time range and with each variable at its default, hidden queries included.
A default range longer than 400 days is checked over its last 400 days, the longest range the query routes accept, so the save is not refused for it and no check query reads more.
Which queries run depends on the tool:

- `create_dashboard` and `update_dashboard` run every query of every panel and every query variable.
- `update_panels` runs only the queries that changed against the version it builds on: the panels whose query text, datasource, type, `timeFrom`, `timeShift`, `maxDataPoints` or `interval` changed, the panels that use a variable whose query changed, and the variables that changed.
  A new panel counts as changed, but an edit of only a panel's title or position, or a change to `tags` or `links`, runs nothing.
  A panel the patch did not touch is not run again, so a query that has started failing since the last save does not block a patch that leaves it alone.
- `update_panels` runs every query when it changes the default `time` to a different `from` or `to`, because each panel's check depends on that range, and a panel that returns nothing over the new range rejects the save.
  It runs every query too when it changes the title of a dashboard whose queries use `$__dashboard`.
  The model is told to keep such a panel and widen the range, not delete the panel to make the save pass.

### Rejections

The save is rejected when:

- a query fails, or a datasource uid is not configured.
- none of the queries of a panel returns data, except on an `alertlist` panel, where no firing alerts is normal.
  A signed-in person can keep such a panel from the [panel editor](../use/edit-panel.md#save-as-a-new-version), marked "Unverified"; the agent, MCP clients and the CLI cannot.
- the document breaks a rule of the format, such as overlapping panels or an undeclared variable.

The rejection lists each problem with the path of the panel or variable and a code.
Every rejection in a run lists `failing_panels`, the ids of the panels the problems belong to, with a line that says how to repair them.
The step of a rejected save names the first problem's path, and a refused input names the reason.
The model fixes only the panels it names and saves again.
Three rejected saves in a row end the run with an error that names the last issue codes, and an accepted save into the build resets the count.

### Changes made meanwhile

When the dashboard changed while the agent was revising it, the server moves the build onto the latest version, at the commit or as soon as a new session would start on the older build, as long as the build changed no dashboard setting and none of the panels the newer versions changed.
The agent's panels are checked again on the latest version, and a commit that hit the change is retried at once.
Otherwise the build is discarded and the run restarts once from the latest version.
If it changes again, the run fails.

### Revisions

A revision starts from the current document and keeps panel ids, positions and everything the prompt does not ask to change, so [comments](versions.md#comments) stay attached to their panels.
A comment on a panel asks to change that panel unless it says otherwise.
The save tools reject a document that creates or edits a placeholder panel, writes HTML tags in a text panel, or adds or changes the actions of a canvas element, a panel's fields or an override (see [Maps, canvases and actions](observespec.md#maps-canvases-and-actions)), and the model is told not to delete an unverified panel or its queries to make a save pass (see [Import Grafana dashboards](../use/import-grafana.md)).

## Whose permissions it uses

Each run carries an `intent`: `ask` reads and answers, `apply` changes a dashboard, and `suggest` proposes one panel.
A new-dashboard request is an `ask` until it saves its dashboard, and then an `apply`.
Run listings show the `intent` and the run's `kind`, `build` or `investigate`; a private ask lists as `investigate`.
Each run also carries `investigation`, true when it was started as an investigation that opens or writes into an observation and false for an ask from the Chat tab, the canvas, a thread or Ask this on an observation.
It is set when the run is queued and never changes: saving, sharing, showing on a page or withdrawing an answer leaves it as it was.

The agent acts as the person or key that asked.
The server checks the request when it is queued and again when a worker starts it.
A new dashboard goes to the requester's My Dashboards, where every person has Edit, or to General for a key, unless the request names a folder the requester can create in: the agent reads the folders with `list_folders` and passes one as `folder_id`, and a folder it cannot create in is refused and never replaced by another.

A request on Home that asks to change a dashboard that already exists is answered with a finding, because a Home run only builds new dashboards.
The agent finds the dashboard with `list_dashboards` by the id or exact title in the request; when exactly one matches, it passes that dashboard's id as `ask_on_dashboard_id` on `save_finding`, and the server stores it only if the requester can view the dashboard.
The finding then carries `ask_on`, the dashboard's `dashboard_id` and `title`, so the web app can start a revision there with the same prompt; when none or several match, the finding lists the candidates and carries no `ask_on`.
A run never changes its own mode, and only a run on Home takes `ask_on_dashboard_id`.

A follow-up, a revision or a comment sent to the agent needs Comment on the dashboard's folder, so a person who can comment but not edit may ask, and their change becomes a [proposal](proposals.md) that someone who can edit merges (see [Folders and access](../teams/folders.md)).
A [suggestion](../use/edit-panel.md) for one panel still needs Edit.
An investigation that writes into an observation needs the Editor role, while a question asked in a thread on an observation needs only Comment on it and is answered in the thread, writing nothing on the observation's page, whoever asks it.
These runs count toward the same limit as any other, at most 5 revisions queued or running on a dashboard.

The saved version records the requester as its author and the request that asked for it, and the version menu on the dashboard shows "Agent" as the one who saved it.
The tool-level roles do not apply inside a run, because the requester was already checked.

### Private asks

A private ask is a question about one dashboard or one observation, asked in a chat of yours: `observe investigate --private`, `start_run` or `POST /observe` with `kind` `ask` (the default) and a `dashboard_id` or `observation_id`, or `observe --dashboard <id>`.
It needs Comment on what it is about, and it does not count toward the investigation limits.
It writes nothing anyone else reads: no observation is opened or written, no dashboard version is saved, and its activity events are seen only by you and Admins.
Its run and its answer are read only by you and, read only, by Admins.

Once it has an answer you can share it: `observe share-answer <request-id>`, `share_answer`, or `POST /observe/{id}/share`.
That opens a comment thread on the dashboard or observation, at the same panel, block or spot, with your question as your message and the agent's answer as its reply, and on an observation it also writes the answer onto the page.
With a thread id the two are added to that thread instead, which must be on the same dashboard or observation.
Everyone who can read the thread then reads the answer and its evidence, and the run itself, read only, with `GET /observe/{id}`, `get_run` or `observe status`, while the chat stays yours.
Their reads are recorded in the audit trail like yours, and one who can reply in the thread may apply the answer's suggested change there.
The run then names that thread as `shared_thread_id`, which only you see.
A run is shared once, sharing needs Comment there, and an answer whose evidence reads a datasource only Admins can query can only be shared on content in a folder only Admins can read.

While nobody else has written in that thread, you can withdraw the answer back to only you: `observe comments withdraw <thread-id>` or `DELETE /threads/{id}/share` deletes the shared question and its answer, or the whole thread when the share opened it, and makes the run a private ask again, which you can share later.
On an observation the run and its answer leave the observation's lists, but the answer's text that was written onto the page stays there as page content until an editor removes it.
No MCP tool withdraws, because none deletes.

### When the requester cannot edit

The server checks the requester once more when the build commits.
When they can still comment on the dashboard but can no longer edit it, or the run acts in propose mode, the build is saved as a [proposal](proposals.md) and not as a version.
The proposal belongs to the run, so a second attempt of the same run adds a revision to it and does not open another.
The run succeeds with `result_kind: proposal` and no `result_version`, and the comment that asked for it stays open, because nothing was saved.
Someone who can edit the dashboard merges or discards the proposal; the run's last step says "Proposed for an Editor to merge".
**Save what was built** still needs Edit, so it is not offered to a requester who can only comment.

### Tools in a run

The server has 110 tools: MCP clients get 109, every one except `save_finding`, which only a run offers.
The built-in agent gets 30 in a new-dashboard or revision request and 29 in a suggestion, which leaves out `save_finding`.
An investigation offers it only the read tools, `list_annotations` and `save_finding`.
The other 80 tools are for MCP clients only, so the agent never writes or resolves a comment reply, never edits an observation's blocks, never merges a proposal and never claims a run.
They are the comment, observation, proposal, RCA, inbox, saved search, pin, home dashboard and claim tools, the dashboard version, move and duplicate tools, `create_folder`, `rename_folder`, `create_annotation`, `update_annotation`, `list_activity`, `list_dashboard_health`, `list_dashboard_viewers`, `list_mentionable_people`, `request_comment_access`, `list_access_requests`, `recheck_datasource`, `get_run`, `get_run_build`, `start_run`, `cancel_run`, `retry_run`, `list_runs`, `rename_run`, `pin_run`, `unpin_run`, `archive_run`, `list_chats`, `get_chat`, `run_dashboard_health`, the six audit trail tools, `save_run_answer`, `share_answer`, `apply_suggested_change`, `search_workspace` and `draft_query`; [MCP](../use/mcp.md) lists each with its role.
`list_observations` and `get_observation` are offered to the built-in agent in every run, so it can read the observations that earlier answers were saved into, as the person who asked and only the ones they may open; an investigation cites what it used by the observation's title and `/o/<id>`.

Its tool descriptions and input schemas are the ones MCP clients read, except where a run behaves differently.
The run names the dashboard and its version, so the agent's `update_dashboard` and `update_panels` take no `dashboard_id` or `parent_version`, and `get_panels` without `dashboard_id` reads the dashboard being worked on, its build included.
Its saves take `finish`, `planned_panels` and `row`, its `update_panels` takes one panel, and its `create_dashboard` takes `folder_id` only on the first save of a build, which the server keeps with the build until it becomes a version.
Inside a run `get_spec_guide` returns only the JSON Schema, because the guide and the example are already in the agent's system text; over MCP it returns the guide with the sections of the configured types, the schema and the example.
When a revision request comes from a comment, the server resolves that thread itself once a version is saved.
When a message that mentions an agent started the run, the server instead posts one status reply in that thread when the run ends, whatever the outcome, and leaves the thread as it is; a reply that could not be posted, because the dashboard was locked or the write failed, is tried again every minute for up to a day; the agent also reads the thread's earlier messages with the request.

### Data sent to the provider

The agent sends what its tools return to the LLM provider, including names, label values, sampled rows and query results (see [LLM providers](../connect/llm-providers.md)).

## The agent process

The server starts `observe-agent` as a child process, which claims the built-in agent's runs and works them through the same MCP tools as a connected agent does.
The claim answer to the built-in agent carries the model settings, which no other claimer receives, so the sidecar needs no provider configuration of its own.
The sidecar reaches the server only over the loopback address, with a token that is new for every start of the process.
The server refuses that token on any request that carries a forwarding header (`Forwarded`, `X-Forwarded-For`, `X-Forwarded-Host`, `X-Forwarded-Proto` or `X-Real-Ip`).
A reverse proxy on the same host that adds none of them, such as nginx with a bare `proxy_pass`, still reaches the server from loopback, so configure it to send `X-Forwarded-For` or keep `/mcp` off it.
The server enforces the guards and the limits and stores the saved result.
It looks for `observe-agent` on `PATH`, and it does not start when the binary is missing or when `SLASHOBSERVE_HTTP_ADDR` names a host other than a loopback address or all addresses.

## MCP clients use their own model

An MCP client such as Claude Code runs the loop itself, with its own model, against the same tools at `<public url>/mcp`.
The server needs no LLM provider for that.
Each call is checked against the role of the key's service account and the folders it can see, and a save goes through the same validation and query check as above.
The client can also read and answer comments, which the built-in agent cannot.
A version saved this way records `mcp` as its origin and the client's name.
See [MCP](../use/mcp.md) for the tools and the setup.
