---
title: Agents
description: See and direct everyone who can do work for you, the Observe Agent, your connected agents and the service accounts that take runs, choose whether a connected agent saves directly or proposes, and hand it a run by mentioning it.
---

Three kinds of agent can do work in a workspace.

- The Observe Agent is the built-in one and needs a model on the server.
- A connected agent is an MCP client that you approved to act as you, such as Claude Code.
- A service account that takes runs is a bot with its own key.

The **Agents** page puts the three in one place.
Use it to see who is working, what it cost and what failed, to connect a new agent, and to choose how a connected agent saves.

## See your agents

1. Choose **Agents** in the sidebar, which opens `/agents`.
2. Read the three parts of the page.

**Observe Agent** shows whether it is On or Off, and the provider and model it uses or "No model configured".
It also shows four counts for the last seven days: Runs, Failed, Cost and Median wait.
Under them, "Last 7 days, everyone" shows for an Admin and "Last 7 days, yours" for anyone else.

**Connected agents** lists your own live connections.
Each row has the agent name, its **Mode**, its status and its writes in the last 7 days.
The status is "Working", "Listening", "Offline, last seen" with a time, or "Never seen".
With no connection, the page says "No connected agents yet."
**Manage in Settings** opens the Connected agents section, where you disconnect one.

**Service accounts that take runs** lists each account with its status.
An Admin also sees its runs and failures for the last 7 days, and an **API keys** link that opens the page that creates keys (`/keys`).

The page and `GET /api/v1/agents` need the Viewer role or higher and a signed-in person.
A service-account key gets 403 `user_session_required`.

## Connect an agent

1. On the **Agents** page, choose **Connect an agent**.
   A side panel opens with this command and the text "Waiting for Claude Code to connect":

   ```sh
   claude mcp add --transport http slashobserve <mcp url>
   ```

2. Run the command in Claude Code.
3. Approve the connection in the browser window that opens.
4. The panel says "Connected:" with the client's name.
   Choose **Done**.

To connect another client, see [Connect your own agent](mcp.md#connect-your-own-agent).

## Open one agent's page

Choose an agent's name to open `/agents/<ref>`, which shows the same counts for that one agent.
The `ref` is `observe` for the Observe Agent, `grant-<id>` for a connected agent and `sa-<id>` for a service account.

The page ends with a **Recent work** list of the runs behind its seven-day counts, newest first.
Each row shows:

- the run's title;
- its state: Queued, Running, Stopping, Done, Failed or Canceled;
- when it was asked, and what it cost when a cost was recorded;
- the dashboard or observation it touched.

**Load more** shows the next page, and an agent with no runs says "No runs yet."
[Who sees which runs](#who-sees-which-runs) has the rules.

## Direct and propose mode

A connected agent has a mode.
Use the **Mode** menu on its row or on its page to change it.

- **Direct**, the default, lets the agent's saves change dashboards as the person who connected it may.
- **Propose** makes every dashboard save of the agent a [proposal](../concepts/proposals.md) that someone who can edit merges, even when the person is an Editor.

The mode applies from the agent's next request.
A run assigned to a connected agent keeps that connection's mode, so its saves become a proposal while the mode is propose.
A service account has no mode.

Only a signed-in person can change the mode, and no agent or key can change its own.
Over HTTP, send `PATCH /api/v1/me/grants/{id}` with `{"mode": "propose"}` or `{"mode": "direct"}`.

## Give an agent a run

A comment that mentions an agent hands it work.

1. Open a comment composer in the web app and type `@`.
   The picker lists the Observe Agent first, then your connected agents and the service accounts that take runs, then people.
2. Keep typing to filter the list.
   The list closes at the first space or when nobody matches, so `@observe add a stat panel` typed straight through is a comment that asks the Observe Agent.
3. Send the comment from a composer that asks the agent.
   [Ask the agent from a comment](review.md#ask-the-agent-from-a-comment) describes the composer.

A message that mentions exactly one agent starts a run for it.
That agent becomes the thread's agent, which later plain replies in the web app ask too.
The run answers in the same thread when it ends.
Over the API, MCP or the CLI, send the message with `to: agent`.

The mentions in the comment text are:

| Agent | Mention |
| --- | --- |
| The Observe Agent | `@observe` |
| A connected agent | `@[Name](grant:<grant_id>)` |
| A service account | `@[Name](service_account:<id>)` |

You can hand a run only to your own connected agents and to service accounts that take runs.
A service-account key can never mention a connected agent.
A connected agent works only for the person who connected it.

An agent claims its runs with `claim_run`; see [Take runs as an agent](mcp.md#take-runs-as-an-agent) for the whole loop.

### When no agent takes the run

A run that no outside agent took within 10 minutes fails with the code `agent_unavailable`.
Its reply in the thread reads "No agent picked this up within 10 minutes."
Retrying or continuing such a run goes to the same agent.
It answers `assignee_unavailable` when that agent was disconnected or no longer takes runs.

## Who sees which runs

- An Admin sees every run of the Observe Agent or of a service account, and the rows then say who asked.
- Anyone else sees only the runs they asked for.
- A connected agent's runs are listed only to the person who connected it, even after the connection was revoked or expired.
  Anyone else, an Admin included, gets 404 `not_found` for its id.
- A dashboard or observation in a folder you cannot see is left out of its row.
- A run opens only for the person who asked for it, so other people's runs show as plain text.
- Suggestion runs are not listed because they have no page to open, although the seven-day counts still include them.
- Archived runs are listed, because archiving only hides a run from its requester's own list.

## Reference

### What the agents list holds

`GET /api/v1/agents` has three parts.

| Part | Shows |
| --- | --- |
| `observe_agent` | Whether the built-in agent is on, the provider and model names, its `runtime`, and its runs of the last seven days. |
| `connected` | Your own live connections, newest first. |
| `service_accounts` | Every service account that [takes runs](../operate/service-account-keys.md). |

Details of `observe_agent`:

- The provider and model are names only, never an address or a key.
- `runtime` is always `sidecar`.
- `scope` is `workspace` for an Admin, whose seven days cover every run in the workspace, and `you` for anyone else, whose seven days cover only their own runs.

Details of `connected`:

- Each entry has the client, its `mode` (`direct` or `propose`), whether it is `present`, its `presence`, when you connected it and what it did in the last seven days.
- `writes_7d` counts the changes you made through MCP under its client name in the last seven days.
  Two connections of one client name share that count.

Details of `service_accounts`:

- Each entry has the name, whether it is `present` and its `presence`.
- Its seven days are shown to an Admin and are null for anyone else, who still gets its name to mention it.

An agent is `present` while it holds a run or asked for work in the last 40 seconds.
`presence` says which: `working`, `listening` or `offline`; [States](../reference/states.md) describes the three presences.

### Seven days of work

Each `last_7_days` counts the runs created in the last seven days that were assigned to that agent.

| Field | Meaning |
| --- | --- |
| `runs` | The runs created in the last seven days. |
| `failed` | The failed runs; canceled runs are left out. |
| `cost_usd` | The recorded cost of the runs; null when a finished run has none, because an unknown price is never counted as zero, and the page then shows "Not priced". |
| `median_queue_wait_seconds` | The middle wait between a request and its start over the runs that started; null when none did. |

### Recent work over HTTP

`GET /api/v1/agents/runs?kind=<builtin|grant|service_account>&id=<id>` returns the same list as **Recent work**.

- It returns 20 runs a page by default and at most 50 with `limit`.
- The `next_cursor` of one page is the `cursor` of the next.
- The `id` is the grant id or the service account id, and it is refused for `builtin`.
- Like the page, it needs a signed-in person, and a service-account key gets 403 `user_session_required`.

### Working now over HTTP

`GET /api/v1/agents/working` lists the runs that are queued, running or canceling and that you started.
It also lists the runs any agent is doing on dashboards and observations you created, newest first.

- It returns 20 runs by default and at most 50 with `limit`, and has no cursor.
- It leaves out suggestion runs and archived runs, and says `truncated: true` when more active runs exist.
- A run someone else started names the agent and the dashboard or observation, never the request: its `title` is null and `mine` is false.
- A dashboard or observation in a folder you cannot see does not bring someone else's run into your list, and it comes back as null on a run you started.
- Unless you are an Admin, someone else's run is left out when it is `restricted` or its answer is kept with Admins for what it is about; see [Chat](chat.md).
- An Admin gets no wider list here, because the Agents page already shows the whole workspace.
- It needs a signed-in person, and a service-account key gets 403 `user_session_required`.
