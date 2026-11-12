---
title: Investigate
description: Ask the agent a question about your data and get back an answer in an observation, with the panels that show it.
---

An investigation asks the agent a question, such as why the checkout error rate is up, instead of asking it for a dashboard.
The agent reads your datasources with its read tools and ends the run by saving one finding, an answer written into an [observation](../use/observations.md).
The finding is the answer: the agent's final text is discarded, as it is in every run.

## Starting one

Start an investigation with `POST /api/v1/observe` and `kind` set to `investigate`.
`dashboard_id` names the dashboard the question is about, and `panel_id` one of its panels, which needs `dashboard_id`.
`from` and `to` set the window the agent looks at, as relative times or epoch milliseconds, at most 400 days apart.
Without them the agent looks at the last hour.
`observation_id` names an existing [observation](../use/observations.md) to write into: the answer is 404 `not_found` when you cannot see it and 403 `forbidden_role` when you cannot comment on it.
Without it the server starts a new observation, and the request carries the id of either one as `observation_id`, or null when no folder allowed a new observation.

Starting an investigation needs the Editor role or above, and view access on the dashboard when one is named.
A caller may have 3 investigations queued or running at once, and the instance 10.
Investigations do not wait for other requests on the same dashboard, because they never change it.
The request lists with `source` `investigate`, and `GET /api/v1/observe/{id}` returns its finding once it succeeds.
When the finding is saved, its conclusion and evidence panels are written into the observation, as described in [Observations](../use/observations.md#investigations-write-into-an-observation).

## The finding

A finding holds markdown of at most 400 words, up to 4 evidence panels and the window it was made for.
The server runs every evidence query when the finding is saved and refuses a panel whose query fails or returns no data.
Evidence panels get the ids `evidence-1` to `evidence-4` in the order the agent sent them.

The answer lives in an observation: a text block that starts with `## <title>` and one panel block for each evidence panel.
The blocks and the finding are written in one transaction, so a failure leaves neither.
A finding has the id of its observation when the save creates it, and its own id when the run already had an observation, and its `observation_id` names the observation either way.
Each answer has the address of its observation in the app, `/o/<id>`, where its evidence panels run against the stored queries, and the `url` of a finding is that address.

A finding is read through its observation, so it follows the folder the observation is in.
An investigation about a dashboard files its observation in that dashboard's folder when you can edit it, and otherwise in your My Dashboards or in General; see [Observations](../use/observations.md#investigations-write-into-an-observation).
The observation stays where it was filed when the dashboard moves, and a finding saved over the API or MCP with no dashboard is filed in General when the saver has no My Dashboards.
A finding someone cannot read answers 404, never 403.

## The finding API is deprecated

The finding routes under `/api/v1/findings`, the tools `create_finding`, `list_findings` and `move_finding`, and `observe findings` stay for one release as adapters over observations and are removed in the next release.
Every finding route answers with a `Deprecation` header, and a finding carries its `observation_id`.
Use `/api/v1/observations`, `create_observation` with `add_block`, `list_observations` and `observe o` instead.
A comment target `finding:<id>` and a search kind `finding` still work and mean the finding's observation.

## Acting on a finding

`POST /api/v1/findings/{id}/pin` pins the finding to its dashboard as an [annotation](../use/annotations.md) at the end of its window, tagged `finding`, with the finding's first line and its address as the text.
Pinning needs Edit on the dashboard, and pinning a finding twice returns the same annotation.
`POST /api/v1/findings/{id}/dashboard` saves the evidence panels as a new dashboard in the caller's My Dashboards, after running their queries again.

## Tools

The built-in agent investigates with the same read tools it uses to build dashboards, plus `list_annotations`, so it sees deploy markers beside a spike, and `save_finding`; `list_annotations` is offered only in an investigation, and `save_finding` also in a new-dashboard or revision request that only asks a question.
It does not get the dashboard save tools in an investigation.
MCP clients investigate with their own model: they read data with the read tools, save the answer into an observation with `create_observation` and `add_block`, and read earlier ones with `list_observations`; `create_finding` and `list_findings` still work and are deprecated (see [MCP](../use/mcp.md)).

## When an investigation fails

A run that ends without saving a finding fails with the failure code `no_finding`.
The agent is reminded once to save a finding before the run fails.
