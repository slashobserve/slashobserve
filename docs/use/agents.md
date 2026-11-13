---
title: Agents
description: See and direct everyone who can do work for you, the Observe Agent, your connected agents and the service accounts that take runs, choose whether a connected agent saves directly or proposes, and hand it a run by mentioning it.
---

Three kinds of agent can do work in a workspace.
The Observe Agent is the built-in one and needs a model on the server.
A connected agent is an MCP client that you approved to act as you, such as Claude Code.
A service account that takes runs is a bot with its own key.
The **Agents** page in the sidebar, at `/agents`, and `GET /api/v1/agents` put the three in one place.
Both need the Viewer role or higher and a signed-in person; a service-account key gets 403 `user_session_required`.

## The Agents page

The page has three parts.

- **Observe Agent** shows whether it is On or Off, the provider and model it uses or "No model configured", and four counts for the last seven days: Runs, Failed, Cost and Median wait.
  Under them, "Last 7 days, everyone" shows for an Admin and "Last 7 days, yours" for anyone else.
- **Connected agents** lists your own live connections with the agent name, its **Mode**, its status ("Present", "Never seen" or "Last seen" with a time) and its writes in the last 7 days.
  **Manage in Settings** opens the Connected agents section, where you disconnect one.
  With none, the page says "No connected agents yet."
- **Service accounts that take runs** lists each account with its status, and an Admin also sees its runs and failures for the last 7 days.
  An Admin sees an **API keys** link here, which opens the page that creates keys (`/keys`).

**Connect an agent** opens a side panel with the command `claude mcp add --transport http slashobserve <mcp url>`, which adds this server to Claude Code, and waits.
The panel says "Connected" with the client's name when the approval you give in the browser window it opens is done; other clients are set up as described in [Connect your own agent](mcp.md#connect-your-own-agent).
Choosing an agent's name opens `/agents/<ref>`, which shows the same counts for that one agent: `observe` for the Observe Agent, `grant-<id>` for a connected agent and `sa-<id>` for a service account.

## Direct and propose mode

A connected agent has a mode, and the **Mode** menu on its row and on its page changes it.
Direct, the default, lets its saves change dashboards as the person who connected it may.
Propose makes every dashboard save of the agent a [proposal](../concepts/proposals.md) that someone who can edit merges, even when the person is an Editor.
The mode applies from the agent's next request, and only a signed-in person can change it, with `PATCH /api/v1/me/grants/{id}` and `{"mode": "propose"}` or `"direct"`.
No agent or key can change its own mode.
A run assigned to a connected agent keeps that connection's mode, so its saves become a proposal while the mode is propose.
A service account has no mode.

## What it lists

| Part | Shows |
| --- | --- |
| `observe_agent` | Whether the built-in agent is on, the configured provider and model names (never an address or a key), its `runtime` (always `sidecar`), and its runs of the last seven days. |
| `connected` | Your own live connections, newest first: the client, `direct` or `propose` mode, whether it is present, when you connected it and what it did in the last seven days. |
| `service_accounts` | Every service account that [takes runs](../operate/service-account-keys.md), with its name and whether it is present. |

An agent is present while it holds a run or asked for work in the last 40 seconds; [states](../reference/states.md) describes the three presences behind it.
The Observe Agent's seven days cover every run in the workspace for an Admin and only your own runs for anyone else, and `scope` says which.
A service account's seven days are shown to an Admin and are null for anyone else, who still gets its name to mention it.

## Seven days of work

Each `last_7_days` counts the runs created in the last seven days that were assigned to that agent.
`failed` leaves out runs that were canceled.
`cost_usd` adds the recorded cost of the runs and is null when a finished run has none, because an unknown price is never counted as zero; the page then shows "Not priced".
`median_queue_wait_seconds` is the middle wait between a request and its start over the runs that started, and null when none did.
A connected agent also has `writes_7d`, the changes you made through MCP under its client name in the last seven days; two connections of one client name share that count.

## Give an agent a run

A comment that mentions an agent hands it work.
A connected agent's `grant_id` and a service account's `id` are the ids a comment names: `@[Name](grant:<grant_id>)` and `@[Name](service_account:<id>)`, and `@observe` names the Observe Agent.
The `@` picker of every web composer lists the Observe Agent while a model is connected, then your connected agents, then the service accounts that take runs, all from this one answer.
A message that mentions exactly one agent starts a run for it when you send it from a composer that asks the agent, or with `start_runs` over the API, MCP or the CLI.
[Ask the agent from a comment](review.md#ask-the-agent-from-a-comment) describes the composer, and the run answers in the same thread when it ends.

You can hand a run only to your own connected agents and to service accounts that take runs, and a service-account key can never mention a connected agent.
A connected agent works only for the person who connected it.
An agent claims its runs with `claim_run`; see [Take runs as an agent](mcp.md#take-runs-as-an-agent) for the whole loop.
A run that no outside agent took within 10 minutes fails with the code `agent_unavailable`, and its reply in the thread reads "No agent picked this up within 10 minutes."
Retrying or continuing such a run goes to the same agent, and answers `assignee_unavailable` when that agent was disconnected or no longer takes runs.
