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

### The hypothesis ledger

With `SLASHOBSERVE_AGENT_INVESTIGATION_LOOP` set to `true`, the built-in agent also gets `note_hypotheses`.
It writes down 2 to 4 candidate causes after its first look and sends the whole list again with each batch of reads.
Each hypothesis has a short id, one sentence, a status of `open`, `supported` or `ruled_out`, up to 3 evidence lines and, for an open one, the next check.
A hypothesis is ruled out or supported only with an evidence line that says what was read and what it showed.

Details of the ledger:

- It holds at most 6 hypotheses and 4 KiB of text; a longer one is refused with the reason, and the agent shortens it.
- The tool changes nothing in the workspace and is not a read, so it never counts toward the model-call limit.
- A turn with only a ledger call and no read counts as a stalled turn, like a turn in which every call was refused.
- The run's steps show one line for each change, such as `3 hypotheses: 1 supported, 1 ruled out, 1 open`, under the label `ledger`.
- When the conversation is shortened, the server puts the ledger in front of the agent again in a `<hypotheses>` block.
- The finished run's `usage.hypotheses` holds the last ledger, in the run API and in `get_run`.
- Without the setting, the tool, the prompt section and the block are absent.

### Stages and the call budget

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
- The limit starts at `SLASHOBSERVE_AGENT_INVESTIGATION_CALLS`.
  It grows by 3 calls each time a hypothesis moves to `supported` or `ruled_out` with a new evidence line, as long as the run made at least one successful read since the limit last grew, and stops at `SLASHOBSERVE_AGENT_INVESTIGATION_CALLS_MAX`.
  A ledger written without reading anything earns nothing, and unset, the ceiling equals the starting limit, so the limit never grows.
- When the stage becomes `conclude` because a hypothesis is supported and none is open, the agent is told at once to save the finding instead of waiting for the limit.
- After a ledger exists, two turns in a row with reads and no ledger change get one reminder to update the ledger or save; it is a message, never a refusal, and a later change followed by two more quiet turns earns another.
  Send the ledger in the same call as reads: a turn with only ledger calls still counts as stalled.
- Repeated reads, stalled turns, rejected findings, the cost limit and the run timeout are unchanged and end a run in any stage.
- The finished run's `usage.call_limit` is the limit when it ended and `usage.stage_at_save` is the stage when it saved its finding.
- Without the setting, none of this exists and the limit is the fixed one.

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
- Without them the agent looks at the last hour.
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

- The server appends a note when reads were refused after the run reached its limit of model calls (12 by default, set by `SLASHOBSERVE_AGENT_INVESTIGATION_CALLS`), or when a datasource did not answer.
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
