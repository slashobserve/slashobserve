---
title: Investigate
description: Ask the agent a question about your data and get back an answer in an observation, with the panels that show it, and the request, limits and API behind it.
---

An investigation asks the agent a question, such as why the checkout error rate is up, instead of asking it for a dashboard.
The agent reads your datasources with its read tools and ends the run by saving one finding, an answer written into an [observation](../use/observations.md).
The finding is the answer: the agent's final text is discarded, as it is in every run.

For example, you ask "why did checkout errors rise after 10:00?" about the Checkout dashboard.
The agent queries the error and latency series, then saves a short conclusion with up to four panels as evidence.
The conclusion and the panels appear in an observation at `/o/<id>`, filed in the dashboard's folder.

[Investigate](../use/investigate.md) covers asking from the web app.
This page covers what an investigation is, what it may do, and how to start one from the CLI, the HTTP API and MCP.

## Start an investigation

1. Sign in once with a service-account key with the Editor role, as [CLI](../use/cli.md#sign-in) shows.
2. Ask the question:

   ```sh
   observe investigate why did checkout errors rise after 10:00
   ```

   The command follows the run while the agent reads your datasources, then prints the finding with its title, its answer and its address.
3. Narrow the question with a flag.
   `--dashboard <id>` makes the question about a dashboard, and `--panel <panel-id>` narrows it to one of its panels.
   `--from now-6h --to now` sets the window the agent looks at.

   ```sh
   observe investigate --dashboard <id> --panel <panel-id> --from now-6h --to now what changed
   ```

4. To write into an observation you already have, pass `--observation <id>`.
   Without it, the server starts a new observation.

From the HTTP API, send `POST /api/v1/observe` with `kind` set to `investigate`:

```sh
curl -fsS -X POST -H "Authorization: Bearer $OBSERVE_TOKEN" \
  -H "Content-Type: application/json" \
  "$OBSERVE_SERVER/api/v1/observe" \
  -d '{"kind": "investigate", "prompt": "why did checkout errors rise after 10:00", "dashboard_id": "<id>", "from": "now-6h", "to": "now"}'
```

The answer is `202` with the request, and its `Location` header is `/api/v1/observe/{id}`.
`GET /api/v1/observe/{id}` returns the finding once the run succeeds, and the request lists with `kind` `investigate`.
The request carries the id of the observation it writes into as `observation_id`, or null when no folder allowed a new observation.

An MCP client investigates with its own model.
It reads data with the read tools, saves the answer into an observation with `create_observation` and `add_block`, and reads earlier ones with `list_observations` and `get_observation`.
It does not call `save_finding`.

## What the agent does

The agent investigates with the same read tools it uses to build dashboards, and these as well:

- `list_annotations`, so it sees deploy markers beside a spike.
- `list_observations` and `get_observation`, so it can read earlier observations in the folders the requester can open, as evidence.
- `save_finding`, which ends the run.

It does not get the dashboard save tools in an investigation, because an investigation never changes a dashboard.
Investigations therefore do not wait for other requests on the same dashboard.

Each task type also gets its own short prompt section, a playbook with the steps and the answer shape for that kind of question, and the rules below reach only the types they belong to; a question no rule sorts gets them all.
How the agent reads and answers:

- The verdict follows the comparison the question asked for, and a short spike inside the window is a separate observation.
- In its own reads, a current rate uses a short window ending now, such as `[5m]`, and a rate an hour ago or yesterday is the same expression with PromQL's `offset 1h`; a wide window is for questions that ask for an average over that span, never for a current rate or a point-in-time comparison, and saved evidence panels keep `$__rate_interval`.
- When the ratios the question asks for are within about 10% of 1, the agent says that factor does not explain the change.
- When the question is whether one factor or another explains a change, it also decides from the ratio of each over the same windows and states the ratios; a ratio against the change explains nothing, and a mean over another span or a short spike does not overturn the windowed ratios.
- When the question names a set, a total of counts or quantities across its members is a sum, a per-member mean is given only when labelled as one, and a ratio or percentile across the set is computed over the set, never summed.
- When a term has two plausible readings, the agent headlines one figure for the reading it used and names the other reading in a separate one-line note.
- When the question names several services, routes or operations, it reads each one's slow tail or error set separately.
- When it compares or ranks the members of a set, it gives every member, including the ones that look fine, the same metric, window and unit.
- To say where requests first failed, the agent calls `trace_error_origins`, which counts each failing request once at its deepest erroring span; the leaders by count and share are the origin, named by service and operation in the headline and the body alike, and the services that relayed the error are secondary.
- Slowness is judged from the slow requests, not a percentile alone: the agent calls `trace_slow_path`, with the question's threshold when it gives one, and reports for each route the slow count, the span with the most self time, every service in the slow traces with its count and an example trace with its exact duration.
- A quantile over a window comes from `trace_stats` with `group_by`, which reads it exactly or says it rests on a sample.
- When it found an onset, the annotations show no deploy and a change is a plausible cause, it searches the involved services' logs at every level for deploy, rollout, version or restart lines in a short window before the onset.
- It lists every cause the evidence supports, most confident first, and adds none to be safe: a second service's errors are a cause of their own only with their own onset or trigger, and otherwise a knock-on of the cause they follow or background checked against the rate before the onset.
- It names a cause's trigger by its key exactly as the system spells it, such as a flag key, a setting or a version, with its value.
- It does not say something does not exist, is not happening, is not recurring or was not deployed unless a query checked it, and it says what was checked.
- It reads durations and rates at the series' native step in a short range around the event.
- It takes a baseline such as typical or normal from the question's whole window without the event's own span, read at an aggregate or coarser step.

### Task types

When an investigation or a question about a dashboard or an observation starts, the server sorts it into one task type by fixed rules over the question's words, whether a dashboard or panel is its subject and which kinds of datasource are configured.
No model call is made for this.

| Task type | Questions it takes |
| --- | --- |
| `measure` | A value, peak, total, rate, share, quantile or a comparison of them. |
| `logs` | Reading, counting or characterising log lines, when a log store is configured. |
| `traces` | Single requests, slow or failing calls and one trace, when a trace store is configured. |
| `incident` | Why something happened, what changed, health and root cause. |
| `inventory` | What exists: services, metrics, labels, dashboards, datasources. |
| `explain` | Explaining or summarizing a dashboard, panel, observation, query or term, and a change asked for on a dashboard. |

A question that asks for a write, such as pinning, publishing or adding a block, one that names both logs and traces, and one no rule decides gets the full investigation, exactly as without task types.
The run's steps show one line with the task type, such as `Routed the question to the measure task type`, and never the question.
The type is decided once, when the run's session starts, and kept with it.
A new-dashboard request, a revision and a suggestion are not sorted.

A sorted run offers only its type's tools, the built-in agent and a connected agent with a run token alike.
Every type gets `list_services`, `list_labels`, `list_label_values`, `save_finding`, `note_hypotheses` when the setting turns it on, the three MCP source tools and `list_more_tools`, plus `get_panels` when a dashboard is the subject and `get_observation` when an observation is:

| Task type | Its own tools |
| --- | --- |
| `measure` | `list_metrics`, `measure_window`, `measure_aggregate`, `query_range`, `query_instant` |
| `logs` | `query_range`, `measure_aggregate`, `log_patterns`, `log_facets`, `log_context`, `sample_rows` |
| `traces` | `trace_stats`, `trace_error_origins`, `trace_slow_path`, `get_trace`, `compare_traces`, `logs_for_trace` |
| `incident` | `list_metrics`, `list_annotations`, `measure_window`, `query_range`, `trace_error_origins`, `trace_slow_path`, `log_patterns`, `get_trace` |
| `inventory` | `list_datasources`, `list_metrics`, `list_dashboards`, `list_observations`, `search_workspace` |
| `explain` | `get_dashboard`, `get_panels`, `get_observation`, `list_dashboards`, `search_workspace`, `get_spec_guide`, `query_range` |

A call to any other tool is answered `unknown_tool` with a line that names the run's tools.
When those are not enough, the agent calls `list_more_tools`: the run then offers every investigation tool until it ends, the steps record it, and the built-in agent lists its tools again before its next model call.
A type never adds a tool its run would not offer without one.
The datasource list in the agent's context marks an Elasticsearch or OpenSearch datasource that holds Jaeger spans with `spans: jaeger`, so an incident run, which has no `list_datasources`, still knows where the trace tools read.

### The preflight brief

Before the first model call of an investigation, the server sweeps the datasources the requester's run may read and writes what it found into the agent's first message, in a `<triage>` block.
It is on by default; `SLASHOBSERVE_AGENT_INVESTIGATION_BRIEF` set to `off` turns it off.
A plain question, a chat question and a dashboard build or revision do not sweep.

The sweep calls no model.
It reads through the same tools, access checks and limits as the agent, never reads the audit datasource or any datasource only Admins can query, and stops after 40 reads or 20 seconds, whichever comes first.
It reads:

- The service map: on each store, the label or field that names a service and the services seen.
- Request rate, error rate and p95 latency per service over the last 1 hour, 6 hours and 24 hours, each against the same length just before it, from span-metric or HTTP server metrics on Prometheus, otherwise span counts on Tempo or ClickHouse, otherwise error-level log lines on Loki; each line says which.
- The onset of each service whose errors or latency moved: the first step above twice the baseline's median error ratio and at least 1 point more, or 1.5 times its median p95 latency and at least 5 ms more, that stays there for 3 steps, read at 120 steps per window.
- Failing requests counted at their deepest erroring span with `trace_error_origins`, with one example trace id per group.
- Log patterns on Loki that are new or grew in the window, at every level, for up to 3 services that moved.
- Change events: annotations in the window and the day before, deploy, rollout, version and restart lines in Loki, and feature flag evaluations in Loki log records and in Tempo span events (`feature_flag.evaluation`, with `feature_flag.key` and its variant), with the first time each variant was seen.

The server groups what moved into numbered candidates, `c1`, `c2` and so on, in onset order, each with the change events of the 30 minutes before its onset.
A service whose errors start later than another's and that relays that service's errors in the traces is listed as its knock-on, not as a candidate of its own, and errors present before the window at a similar level are listed as background.
The block also says what was skipped and why, such as a store that failed or the read budget running out.
It is at most 6 KiB; the lines cut first are the skipped steps, then the background, log patterns, per-service rates, service lists, evidence lines, signal lines and last the latest candidates.

The block is a starting point, not evidence: the agent is told to test each candidate with its own reads, and every figure in the finding still comes from a read of the run.
The sweep runs once per run: the block and its duration are kept with the run session, so a shortened conversation, a restart or another server reuses it, and the run's steps show one line with the number of reads and the time it took.
The block is sent to the LLM provider like any tool result and never written to the server's logs.

### The hypothesis ledger

With `SLASHOBSERVE_AGENT_INVESTIGATION_LOOP` set to `true`, the default, the built-in agent also gets `note_hypotheses`, and an investigation that opens an observation is told in its prompt to use it.
A plain question gets the tool but no prompt section and no `<hypotheses>` block.
It writes down 2 to 4 candidate causes after its first look and sends the whole list again with each batch of reads.
Each hypothesis has a short id, one sentence, a status of `open`, `supported` or `ruled_out`, up to 3 evidence lines and, for an open one, the next check.
A hypothesis is ruled out or supported only with an evidence line that says what was read and what it showed.
A supported or ruled-out hypothesis may also carry a closing that says how it was settled:

| Closing | Status | Means |
| --- | --- | --- |
| `cause` | `supported` | It is one of the finding's causes. |
| `knock_on` | `ruled_out` | It follows from another hypothesis, named in `knock_on_of`. |
| `background` | `ruled_out` | It was at a similar level before the window, and the evidence line says so. |
| `transient` | `ruled_out` | It was gone on a later read: it needs two evidence lines, the read that showed it and the later one. |
| `unrelated` | `ruled_out` | It has nothing to do with the question, with the evidence line that shows it. |

A closing with the wrong status, a closing with no evidence line, a `transient` with one line and a `knock_on` that names no other hypothesis are refused with the reason.

Details of the ledger:

- It holds at most 6 hypotheses and 4 KiB of text; a longer one is refused with the reason, and the agent shortens it.
- The tool changes nothing in the workspace and is not a read, so the call limit never refuses it.
- A turn with only a ledger call and no read counts as a stalled turn, like a turn in which every call was refused, and two in a row end the run as `no_progress` with the step `Stopped: 2 turns in a row only updated the hypotheses`.
- A turn spent only on the ledger still uses a model call, and that call counts toward the model-call limit.
- The run's steps show one line for each change, such as `3 hypotheses: 1 supported, 1 ruled out, 1 open`, under the label `ledger`.
- When the conversation is shortened, the server puts the ledger in front of the agent again in a `<hypotheses>` block.
- The finished run's `usage.hypotheses` holds the last ledger with each closing, in the run API and in `get_run`, for the requester and Admins only, because the ledger quotes data the run read.
- Without the setting, the tool, the prompt section and the block are absent.

### Stages and the call budget

Stages belong to the built-in agent only.
A connected agent that gets the ledger tool in an investigate run sees no stage line, no stage step and no early save message, and keeps the fixed limit.

With the same setting on, the server also works out which stage the investigation is in and lets the call limit follow progress.
The agent never declares a stage; the server derives it from the ledger and the call count after each turn.

| Stage | When |
| --- | --- |
| `orient` | No ledger yet. |
| `hypothesize` | A ledger exists and no hypothesis has an evidence line. |
| `test` | At least one evidence line exists and the ledger is not ready to conclude. |
| `conclude` | One hypothesis is supported and none is open, or the call before the limit has come. |

Details of the stages:

- The agent gets one line after each turn, such as `Stage: test. Model call 7 of 12.`, and the run's steps show one line each time the stage changes, under the label `stage`.
- The limit starts at `SLASHOBSERVE_AGENT_INVESTIGATION_CALLS`, or the task type's own limit when that is unset.
  It grows by 3 calls each time a hypothesis moves to `supported` or `ruled_out`, as long as the run made at least one successful read since the limit last grew, and stops at `SLASHOBSERVE_AGENT_INVESTIGATION_CALLS_MAX`, 40 by default.
  The move counts when the hypothesis carried evidence already or gains an evidence line that no hypothesis of the earlier ledger had, so renaming a hypothesis earns nothing.
  A ledger written without reading anything earns nothing, and unset, the ceiling equals the starting limit, so the limit never grows.
- When the stage becomes `conclude` because a hypothesis is supported and none is open, the agent is told at once to save the finding instead of waiting for the limit.
- After a ledger exists, two turns in a row with reads and no ledger change get one reminder to update the ledger or save; it is a message, never a refusal, and a later change followed by two more quiet turns earns another.
  Send the ledger in the same call as reads: a turn with only ledger calls still counts as stalled.
- Repeated reads, stalled turns, rejected findings, the cost limit and the run timeout are unchanged and end a run in any stage.
- The finished run's `usage.call_limit` is the limit when it ended and `usage.stage_at_save` is the stage when it saved its finding.
- Without the setting, none of this exists and the limit is the fixed one.

### The grounding check

With `SLASHOBSERVE_AGENT_FINDING_GROUNDING` set to `report` or `enforce`, the default, the server checks each finding against what the run read before it is saved.
It looks at the cited facts and `answers` (see [Facts](#facts)), the figures and trace ids in the Markdown and the impact, the root cause component and the onset.

- A cited fact is a figure from a read by construction, so it always passes the figure rules.
- A figure typed as digits passes when it is a fact's value within 1% and within the precision it was written with, such as `523 ms` for a fact of 0.5234123 s.
  When it is within the looser tolerance below of a fact but more than 1% away, such as `0.5 s` for that fact, it does not pass, and the send-back names the fact to cite.
- Otherwise a figure passes when the run read it, rounded, within 2% or within the precision it was written with, such as `0.52 s` for a read value of 0.5234, and never more than 10% away.
- A figure also passes when it is the difference, sum, ratio or percentage change of two numbers from the same read, such as `+82%` from 287 ms to 523 ms; a percentage is compared as a fraction, so `82%` is never the plain difference of 300 and 382.
- Times in a result are not numbers for this check.
- `ms` and `s` are converted into each other, and `K` and `M` multiply a number.
- Times, dates, whole numbers of 10 or less with no unit, code spans and links are never checked; a small number with a unit, such as `3 s` or `5%`, is checked.
- Nor are figures that are not measurements: a number that appears in the question; a version after `version`, `release` or `v`; a port after `port` or a host and colon; an HTTP status from 100 to 599 after `HTTP`, `status`, `code` or `returned`, or in a list after one, such as `503s`; a duration in seconds after `last`, `past`, `every`, `each`, `within`, `step` or `interval`, or before `step`, `window`, `interval` or `resolution`; a quantile after or before `quantile` or `percentile`; a whole number after `top`, `first` or `bottom`; a year from 1990 to 2100 after `in`, `since`, `during` or `of`; list numbers; and percentile names such as `p99`.
- A run of 8 to 32 hexadecimal characters with a letter and a digit is a trace or span id, never a figure: it passes when it is a whole id a result of the run showed, it does not pass when it is the start of such an id cut short, and a 16 or 32 character id the run never read does not pass; link targets and fenced code are not checked.
- The root cause component passes when it is a name the run discovered or saw as a label value or a trace's service, or a name in the text of a `call_mcp_tool` result (see [Names from MCP sources](#names-from-mcp-sources)), as a whole or as whole words of it, so `checkout service` passes on `checkout` and `payment-error-handler` does not.
- The onset passes when it falls inside a time range the run queried, or next to a time a result showed, such as a log line's, give or take a minute.

### Names from MCP sources

An approved MCP source tool's result, read through `call_mcp_tool`, gives the check names only, such as a Service, a NetworkPolicy or a feature flag key that a cluster or a code server returned.
Its numbers never pass a figure, its times never pass an onset, its trace ids never pass a trace id, and it never gives a fact, because the server cannot vouch for what an outside tool computed.
Recording a name is all the server does with the text: it stays outside text that the run treats as data, never as instructions.
A failed call gives nothing, and the answers of `list_mcp_sources` and `find_mcp_tools` describe tools, so they give nothing either.

The server takes a name from the result's text, read as plain text even when it holds JSON, by these rules:

- A name is a run of ASCII letters, digits, `_`, `.` and `-` that starts with a letter after the start of the text or a character other than a letter or digit, and ends with a letter or digit, so `4bf92f35` and `T10` inside a timestamp are not names.
- It is from 3 to 64 bytes long; a longer one is left out whole, never cut.
- Each result gives at most its first 500 distinct names.
- The run keeps the 2000 names it saw most recently, the oldest dropped first, and a name seen again counts as new; these are kept apart from the names of datasource reads and of discovery, which they never push out.

### Facts

With `report` or `enforce`, the values the measurement and trace reads return are numbered facts.
Each object in such a result that holds one carries a `facts` map from the value's key to its id, such as `"facts":{"value":"f3"}`; an id is unique within the run, and the same value read again keeps its id.
The reads that give facts are `measure_window` and `measure_aggregate` (each value, numerator and denominator), `trace_stats` (the counts and the whole-window and per-group quantiles), `trace_error_origins` (the counts, shares and example trace ids), `trace_slow_path` (each route's threshold, median, slow count and examined count, the self times, shares and counts, and the example trace id and duration), `get_trace` (the trace id, span count, duration and the slowest spans), `log_facets` (the count of each value, and the number of lines sampled), `log_patterns` (the count of each pattern, and the lines matched), and the `min`, `max` and `last` of each `query_range` and `query_instant` series.
`log_context` and `logs_for_trace` return lines, and their only counts are how many lines they showed, so they give no facts.
The run keeps each fact with its value at full precision, its kind (number, duration, count, ratio or trace id), its unit when the tool knows it, its labels, its window and the query that produced it.
The ledger holds the newest 400 facts, so the oldest are dropped first, and it is stored with the run's session, never logged and never returned by the API.
Every read also records the trace and span ids its result showed.

A fact of `log_facets` or `log_patterns`, and of `measure_aggregate` on Loki, is the logs signal of a cause (see [Causes and confidence](#causes-and-confidence)), a fact of a trace read or a trace id is the traces signal, and any other fact is the metrics signal.
A count the tool took from a sample of lines is a sampled fact: a `log_facets` count of a field that does not say `exact: true`, and the counts of `log_patterns` when it says `source: server`.
A Loki index label's `log_facets` count is in bytes, as the tool says with `unit: bytes`, and renders with `bytes` after it.

The finding cites a fact with a placeholder in place of the figure, `{{f3}}`, in its Markdown, title, impact, root cause summary, ruled-out items and next steps.
The server writes the value into the text before the finding is stored, so the saved and shown finding holds the number and nobody downstream sees a placeholder.
A placeholder takes options after colons, in any order:

| Option | What it does |
| --- | --- |
| none | The value exactly as the read returned it, with its unit: `{{f3}}` is `1312.75 ms`. |
| `ms`, `s`, `us`, `min` | A duration in another unit; a value with no unit is taken as seconds, as Prometheus writes durations: `{{f3:s}}` is `1.31275 s`. |
| `%` | A ratio as a percent: `{{f4:%}}` of 0.4167 is `41.67%`. |
| `1` to `9` | That many significant figures, refused when the shown value would be more than 1% from the fact: `{{f3:s:3}}` is `1.31 s`. |
| `bare` | No unit after the number. |

A trace id fact takes no option and renders as the whole id.
A sampled fact always renders after `at least`, because the lines a sample counted are the fewest that matched: `{{f6}}` of 37 sampled error lines is `at least 37`, with every option, `bare` included.
Significant figures that would round a sampled count up, such as `{{f6:3}}` of 1249, are refused, because `at least 1250` would claim more than was counted.
A placeholder naming a fact the run does not hold, or an option that does not fit the fact or would move the value by more than 1%, is a problem with the finding: with `enforce` it is sent back once naming the id, and when the finding is saved anyway, an unknown fact is written as `[figure not read]`, a broken option is ignored, and the item is stored with the finding.

### One value per quantity

`save_finding` takes `answers`: for each quantity the question asks for, a few words naming it and the one fact that answers it, such as `{"quantity":"peak p99 of checkout","fact":"f12"}`.
The list is checked and then dropped; the finding stores the rendered text, not the list.
These are problems with the finding, handled like any other item of the check:

- An entry names a fact the run does not hold.
- Two entries name the same quantity (compared without case or punctuation) with facts whose values are more than 1% apart, or with two different trace ids.
- An entry's fact is not given in the Markdown or the impact, cited or written within 1% of its value.
- The Markdown or the impact cites two facts of one quantity with values more than 1% apart: the same tool, statistic and field for the same labels, window and asked query, such as one peak read at two steps, or two durations of one trace.

With `enforce`, a `measure`, `logs` or `traces` question (see [Task types](#task-types)) whose finding states a figure, cited or typed, but has no `answers` entry is also sent back once to list them; the item is only in the send-back, never stored, and the second save is accepted.
The check runs on a finding saved in any run that can save one, so a question asked on Home or in a revision's chat is checked like an investigation, and a suggestion has none.

With `report` or `enforce`, the investigate system text drops its two rules on giving one value per quantity at the precision the tool returned and on settling two different figures with one more read, because the server now enforces them, and ends instead with a short Facts section that says to cite facts, how the options work and to list `answers`.
A question asked on Home or in a revision gets the same instructions as one note in its context, and with `off` every prompt is the one it was.
`get_run_brief` gives a connected agent the same text.

With `report`, the items that did not pass are stored with the finding and nothing else changes.
With `enforce`, the first save with such items is sent back once with a line per item that says how to fix it: "read it, correct it, or remove it" for a figure or name no read gave, the fact to cite for a rounded figure, the whole id for a cut one, and the settling read for two values of one quantity.
The agent may read past its call limit to do that, and the send-back is not counted as a rejected save.
The second save is accepted, and the items that still did not pass are stored with it.
The check never fails a run, and with `off` nothing is recorded.

The stored items are returned as `unverified` on the finding and on each entry of the observation's `findings`, and the observation shows one line under its header, such as `2 figures were not found in what the agent read: 9.3 s, 2,950`.
`observe investigate` prints the same items with the summary lines before the answer.

### Causes and confidence

An investigation's finding lists every cause the evidence supports, up to 6, most confident first, in `causes`; see [Structured fields](../use/investigate.md#structured-fields) for each cause's fields.
A cause that only follows another, such as a second service failing on its calls to the first, is listed as a knock-on of that cause, never as a root cause of its own.

With `SLASHOBSERVE_AGENT_FINDING_GROUNDING` set to `report` or `enforce`, the server scores each cause from 0 to 1 when the finding is saved.
The agent may give its own `estimate`, but the stored `confidence` is the server's, from what the run read:

| Signal | Adds |
| --- | --- |
| A cited fact of the cause's evidence the run holds, or a trace id it read, on one of metrics, logs and traces | 0.25 |
| Each further one of those three signals with such evidence | 0.15 |
| The component is a name the run saw, in a datasource read, in discovery or in an MCP source's result | 0.1 |
| The onset agrees with a cited fact's time, within 5 minutes of it or inside its window | 0.15 |
| Otherwise, the onset falls inside a time range the run read | 0.05 |
| The trigger's key is a name the run saw, the same way | 0.2 |
| A `ruled_out` line cites a fact the run holds | 0.05 |

The sum is capped at 1, and a cause with no cited fact the run holds and no trace id it read scores at most 0.3, so it is always low.
A score of 0.7 or more is `high`, of 0.4 or more is `medium`, and below that is `low`.
A cause scored `medium` or `high` is stated (`stated: true`); a `low` one is listed as unconfirmed, so a report can leave it out or name it as unexplored.
So a fact on a metric, a component the run saw and an onset that agrees with the fact make a medium cause, adding a trigger the run read makes it high, and a symptom alone, a fact and a component, stays low.
The causes are kept in the agent's order within a band, and the bands are sorted high first.
`save_finding` answers with each cause's score and band, and says so when a cause is unconfirmed or when the agent's estimate is two bands away from the score.

With `off`, causes are not scored: `confidence` and `band` are null and every cause is stated.
`root_cause` is the first stated cause that is not a knock-on, and null when there is none.

## Where the answer goes

The answer lives in an observation: a text block that starts with `## <title>`, and one panel block for each evidence panel.
The blocks and the finding are written in one transaction, so a failure leaves neither.
Each answer has the address of its observation in the app, `/o/<id>`, where its evidence panels run against the stored queries.
The `url` of a finding is that address.

The observation follows the access of the folder it is in, so someone who cannot read it gets `404`, never `403`.
These rules decide the folder:

- An investigation about a dashboard files its observation in that dashboard's folder when you can edit it.
- Otherwise it files it in your My Dashboards, or in General when you have none.
- The observation stays where it was filed when the dashboard moves.

[Observations](../use/observations.md#investigations-write-into-an-observation) describes the blocks and the rules in full.

## Pin a finding or turn it into a dashboard

Two deprecated routes act on a finding.
`POST /api/v1/findings/{id}/pin` pins the finding to its dashboard as an [annotation](../use/annotations.md).
`POST /api/v1/findings/{id}/dashboard` saves the evidence panels as a new dashboard in the caller's My Dashboards, after running their queries again.

Details of pinning:

- The annotation sits at the end of the finding's window, tagged `finding`.
- Its text is the finding's first line and its `/findings/<id>` path.
- Pinning needs Edit on the dashboard.
- Pinning a finding twice returns the same annotation, with `200` the second time and `201` the first.

## When an investigation fails

A run that ends without saving a finding fails with the failure code `no_finding`.
The agent is reminded once to save a finding before the run fails.
Retry it with `POST /api/v1/observe/{id}/retry` or `observe retry`.

## Reference

### Request fields

| Field | Meaning |
| --- | --- |
| `kind` | `investigate`. |
| `prompt` | The question. |
| `dashboard_id` | The dashboard the question is about. |
| `panel_id` | One panel of that dashboard. Needs `dashboard_id`. |
| `observation_id` | An existing observation to write into. |
| `from`, `to` | The window the agent looks at. Both or neither. |
| `as_of` | Admin only. Runs the investigation as of a past time. |

Details of `from` and `to`:

- Each is a relative time such as `now-1h`, epoch milliseconds or an RFC 3339 time.
- `from` must be before `to`, and the two may be at most 400 days apart.
- Without them the agent starts from the last hour and widens it when the question needs more.
- With `as_of`, `to` must not be later than `as_of`.

Details of `observation_id`:

- The answer is `404` `not_found` when you cannot see the observation.
- The answer is `403` `forbidden_role` when you cannot comment on it.
- Without it, the server starts a new observation.

Details of `as_of`:

- It is an RFC 3339 time or epoch milliseconds in the past, and a relative time is refused.
- Every relative time in the run, such as `now-1h`, resolves to it.
- `observe investigate --as-of <time>` sets it; see [Investigate a past incident](../use/investigate.md#investigate-a-past-incident).

### Who may start one, and limits

- Starting an investigation needs the Editor role or above, and view access on the dashboard when one is named.
- A caller may have 3 investigations queued or running at once.
  The fourth answers `429` `too_many_requests`.
- The instance may have 10.
  Beyond that the answer is `429` `agent_busy`.
- The answer is `503` `agent_unavailable` when no model is configured.
- A question asked privately with `kind` `ask` and a `dashboard_id` or an `observation_id`, or `observe investigate --private`, is a different request: it needs Comment access, counts toward none of these limits, and keeps its answer on the request until its asker files it as an observation.
  Use `POST /api/v1/observe/{id}/observation`, `observe save-answer` or `save_run_answer`.
- `panel_id` that is not a panel of the dashboard's latest version answers `422` `unknown_panel`.

### The finding

| Part | Limit |
| --- | --- |
| Markdown | At most 400 words and 4,000 characters, as plain Markdown with no HTML. |
| Evidence | Up to 4 data panels, with ids `evidence-1` to `evidence-4` in the order the agent sent them. |
| Window | The window the finding was made for. |

Details of the evidence:

- The server runs every evidence query when the finding is saved.
- It refuses a panel whose query fails or returns no data, and the agent repairs or drops the panel.
- When a datasource did not answer in time, the agent drops those panels and says in the text that it did not read their data.

Details of the markdown:

- The server appends a note when reads were refused after the run reached its limit of model calls (the task type's limit by default, set for every investigation by `SLASHOBSERVE_AGENT_INVESTIGATION_CALLS`), or when a datasource did not answer.
- That note counts toward the 400 words and the 4,000 characters.

A finding has the id of its observation when the save creates it, and its own id when the run already had an observation.
Its `observation_id` names the observation either way.

### The finding API is deprecated

The finding tools `create_finding`, `list_findings` and `move_finding`, the command `observe findings`, the route `POST /api/v1/findings/{id}/move`, the comment target `finding:<id>` and the search kind `finding` are removed.
The routes that read a finding, pin it and make a dashboard from it remain under `/api/v1/findings` as adapters over observations that the web app still reads.
Every finding route answers with a `Deprecation` header, and a finding carries its `observation_id`.
Use `/api/v1/observations`, `create_observation` with `add_block`, `list_observations` and `observe o` instead.

### `observe investigate` flags

| Flag | Meaning |
| --- | --- |
| `--dashboard`, `--panel` | The dashboard, and one of its panels, the question is about. |
| `--observation` | The observation to write into, by id. |
| `--from`, `--to` | The window, both or neither. |
| `--at`, `--block` | The spot or block of an observation the question is about. |
| `--as-of` | Admin only. The time to investigate as of. |
| `--private` | Answer privately about `--dashboard` or `--observation`. |
| `--no-wait` | Print the request id and exit. |
| `--timeout` | Stop waiting after this long. The run keeps going. |
| `--open` | Open the finding in the browser once it is saved. |

### Tools of the built-in agent

`save_finding` is also offered in a new-dashboard or revision request that only asks a question.
There the answer stays on the request, where its asker and Admins can read it, until the asker files it as an observation.
