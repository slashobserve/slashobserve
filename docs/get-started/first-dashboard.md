---
title: Your first dashboard
description: Ask the agent for a dashboard on the demo stack, watch it work, and change the result.
---

This walkthrough starts where the [quick start](quick-start.md) stops: the demo stack is running at http://localhost:8080, you set `SLASHOBSERVE_LLM_PROVIDER` and `SLASHOBSERVE_LLM_API_KEY` before `make demo`, and you are signed in as `admin@example.com`.
Without a model the chat shows "No model is connected to this server." and prompts get `503 agent_unavailable`; see [LLM providers](../connect/llm-providers.md).

## Ask for a dashboard

Open **Observe** in the sidebar.
The page asks "What do you want to see?" and has a box labelled "Message the agent".
Type a prompt and press Enter, or choose **Send**.
Shift+Enter starts a new line, and pressing `/` anywhere on the page moves focus to the box.

```text
checkout health by pod, last 6h
```

Under the box, a chip lists each healthy datasource the agent may use.
In the demo these are `prom` (Prometheus) and `ch` (ClickHouse).
The buttons under "Example prompts" send a ready-made prompt at once, such as "Service health by pod, last 6h" or "Error logs by service from ClickHouse".

Starting a dashboard from the chat needs the Editor role.
A Commenter or Viewer sees "Starting dashboards needs the Editor role. Ask an Admin if you need it."
See [Roles](../teams/roles.md).

A prompt is 1 to 4000 characters.

## Watch the agent work

The page switches to a conversation that shows your prompt and, below it, "Building the dashboard" with a progress list.
Each line is one step the server recorded, grouped by phase:

| Phase            | What the agent is doing                                                               |
| ---------------- | ------------------------------------------------------------------------------------- |
| Discovering      | Listing datasources, metrics, labels, tables and schemas, and reading the Spec guide. |
| Checking queries | Linting a draft query against its datasource, without running it.                     |
| Running queries  | Running a query to see that it returns data.                                          |
| Saving           | Saving the draft, which runs every panel query again.                                 |
| Repairing        | Fixing a draft that the save rejected.                                                |

Repeated steps of one phase fold into a single line such as "3 times, latest: Listed 12 metrics on prom".
A step that failed gets a warning icon and is read out as "needed a fix".
The agent only works with names it discovered, so a wrong metric or label usually shows up here as a failed check followed by a repair.

A save is refused when a panel's query is invalid or returns no data.
The agent then repairs the draft and saves again, a few repair rounds.
If it still cannot save, the conversation ends with "The dashboard could not be built" and a **Try again** button.

When the dashboard is saved, the progress list collapses into a line such as "Ran 14 steps, repaired 1 query", which you can open to see every step.
Below it a result card shows the dashboard title, how many panels it has and which datasources they use, and the footer "Saved as version 1".

## Open the dashboard

Choose **Open dashboard** on the result card.
The dashboard opens in the same tab with the version button reading "Live · v1".

A new dashboard goes into the folder you last created a dashboard in, or General if you have not created one.
The Dashboards page marks it as made by the agent, "Agent, for `<your name>`".
See [Folders](../teams/folders.md).

Choose **Activity** at the top right to open the activity card.
It lists the saved version, with the agent's one-sentence summary, next to any comments and requests.

## Ask for a change

Go back to the conversation, or use the activity card, and ask for another change.
In the conversation the box now reads "Ask for another change to `<title>`", with the note "Each change is saved as a new version of the dashboard."

```text
add a panel with memory usage by pod, and show the last 12h
```

The agent revises the latest version of the same dashboard instead of starting a new one.
It keeps every panel you did not ask it to change.
When the run finishes the result card says "Saved as version 2".

If you have the dashboard open, a toast says "Updated to version 2" and the page switches to it.
Version 1 is kept.
You can compare the two versions or restore the older one; see [Comments, agent requests and versions](../use/review.md).

## Change the time range and variables

The button at the top of the dashboard shows the current range, for example "Last 6 hours".
Open it to pick a quick range such as "Last 1 hour" or "Yesterday", or type values into **From** and **To**.
Both accept `now-6h`, `now/d` and `2026-10-17 09:00`, and **Apply** sets them.
On a time series panel you can also drag across the chart to zoom to that range.

The **Auto-refresh** menu next to **Refresh now** reloads the panels on an interval.
A dashboard can set its own refresh choices and quick ranges, start on a refresh interval, or hide the time picker except for **Refresh now**.

If the dashboard has variables, a bar of selectors appears under the top row: a menu for a query or custom variable, with an All choice when the dashboard allows it, a text box for a textbox variable, and so on.
The agent adds variables only when the prompt asks for a breakdown people will filter, so a dashboard from the prompt above may have none.

The range, refresh interval and variable selections live in the page address (`from`, `to`, `refresh` and `var-<name>`), so you can share a link that carries them, and nothing is saved to the dashboard.
To change the saved default range for everyone, ask the agent in a follow-up, such as "make the default range the last 24 hours".

## Next steps

- [Chat and follow-ups](../use/chat.md) explains what the agent does at each step and what it cannot do.
- [Comments, agent requests and versions](../use/review.md) covers review, comparing versions and restoring.
- [Edit a panel](../use/edit-panel.md) covers changing a query or a panel by hand.
- [MCP](../use/mcp.md) and [CLI](../use/cli.md) build and edit dashboards from your own tools.
- [Import Grafana dashboards](../use/import-grafana.md) brings in dashboards you already have.
