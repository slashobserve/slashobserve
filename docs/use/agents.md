---
title: Agents
description: See who can do work for you, the Observe Agent, your connected agents and the service accounts that take runs, with how each did in the last seven days, through the HTTP API.
---

`GET /api/v1/agents` puts everyone who can do work for you in one answer: the Observe Agent, your connected agents and the service accounts that take runs.
It needs the Viewer role or higher and a signed-in person; a service-account key gets 403 `user_session_required`.
It is read-only: a connected agent's mode and its disconnect stay with `PATCH` and `DELETE` on `/api/v1/me/grants/{id}`.

## What it lists

| Part | Shows |
| --- | --- |
| `observe_agent` | Whether the built-in agent is on, the configured provider and model names (never an address or a key), and its runs of the last seven days. |
| `connected` | Your own live connections, newest first: the client, `direct` or `propose` mode, whether it is present, when you connected it and what it did in the last seven days. |
| `service_accounts` | Every service account that [takes runs](../operate/service-account-keys.md), with its name and whether it is present. |

An agent is present while it holds a run or asked for work in the last 40 seconds; [states](../reference/states.md) describes the three presences behind it.
The Observe Agent's seven days cover every run in the workspace for an Admin and only your own runs for anyone else, and `scope` says which.
A service account's seven days are shown to an Admin and are null for anyone else, who still gets its name to mention it.

## Seven days of work

Each `last_7_days` counts the runs created in the last seven days that were assigned to that agent.
`failed` leaves out runs that were canceled.
`cost_usd` adds the recorded cost of the runs and is null when a finished run has none, because an unknown price is never counted as zero.
`median_queue_wait_seconds` is the middle wait between a request and its start over the runs that started, and null when none did.
A connected agent also has `writes_7d`, the changes you made through MCP under its client name in the last seven days; two connections of one client name share that count.

## Mentioning an agent

A connected agent's `grant_id` and a service account's `id` are the ids a comment names to hand work over: `@[Name](grant:<grant_id>)` and `@[Name](service_account:<id>)`.
[Connect your own agent](mcp.md#connect-your-own-agent) and the CLI's `observe comments` describe what a mention starts.
