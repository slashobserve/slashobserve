---
title: Investigate
description: Ask the agent a question about your data and get back a finding with the panels that show the answer.
---

An investigation asks the agent a question, such as why the checkout error rate is up, instead of asking it for a dashboard.
The agent reads your datasources with its read tools and ends the run by saving one finding.
The finding is the answer: the agent's final text is discarded, as it is in every run.

## Starting one

Start an investigation with `POST /api/v1/observe` and `kind` set to `investigate`.
`dashboard_id` names the dashboard the question is about, and `panel_id` one of its panels, which needs `dashboard_id`.
`from` and `to` set the window the agent looks at, as relative times or epoch milliseconds, at most 400 days apart.
Without them the agent looks at the last hour.

Starting an investigation needs the Editor role or above, and view access on the dashboard when one is named.
A caller may have 3 investigations queued or running at once, and the instance 10.
Investigations do not wait for other requests on the same dashboard, because they never change it.
The request lists with `source` `investigate`, and `GET /api/v1/observe/{id}` returns its finding once it succeeds.

## The finding

A finding holds markdown of at most 400 words, up to 4 evidence panels and the window it was made for.
The server runs every evidence query when the finding is saved and refuses a panel whose query fails or returns no data.
Evidence panels get the ids `evidence-1` to `evidence-4` in the order the agent sent them.
Each finding has an address in the app, `/findings/<id>`, where its evidence panels run against the stored queries.

Anyone who can view the finding's dashboard can read it.
A finding without a dashboard is readable by the person who made it and by Admins.
A finding someone cannot read answers 404, never 403.

## Acting on a finding

`POST /api/v1/findings/{id}/pin` pins the finding to its dashboard as an [annotation](../use/annotations.md) at the end of its window, tagged `finding`, with the finding's first line and its address as the text.
Pinning needs Edit on the dashboard, and pinning a finding twice returns the same annotation.
`POST /api/v1/findings/{id}/dashboard` saves the evidence panels as a new dashboard in the caller's My Dashboards, after running their queries again.

## Tools

The built-in agent investigates with the same read tools it uses to build dashboards, plus `list_annotations`, so it sees deploy markers beside a spike, and `save_finding`; both are offered only in an investigation.
It does not get the dashboard save tools in an investigation.
MCP clients investigate with their own model: they read data with the read tools, save the answer with `create_finding` and read earlier findings with `list_findings` (see [MCP](../use/mcp.md)).

## When an investigation fails

A run that ends without saving a finding fails with the failure code `no_finding`.
The agent is reminded once to save a finding before the run fails.
