---
title: Your first dashboard
description: Ask the agent for a dashboard on the demo stack, watch it work, and change the result.
---

This walkthrough starts where the [quick start](quick-start.md) stops: the demo is running at http://localhost:58080, you set `SLASHOBSERVE_LLM_PROVIDER` and `SLASHOBSERVE_LLM_API_KEY` before `make demo`, and you are signed in as `admin@example.com`.
Without a model the chat shows "No model is connected to this server." and prompts get `503 agent_unavailable`; see [LLM providers](../connect/llm-providers.md).

## Ask for a dashboard

Open Home at `/`; its **Ask Observe Agent** tab has a box labelled "Message the agent".
Type a prompt and press Enter, or choose **Send**.
Shift+Enter starts a new line, and pressing `/` anywhere on the page moves focus to the box.

```text
checkout health by service, last 6h
```

Under the box, a chip lists each healthy datasource the agent may use, followed by the name of the model.
In the demo the datasources are `prom` (Prometheus), `ch` (ClickHouse), `loki` (Loki) and `tempo` (Tempo) and `jaeger` (Jaeger), and `es` (Elasticsearch) and `os` (OpenSearch) when you started the stack with `make demo-search`.
The buttons under "Example prompts" send a ready-made prompt at once, such as "Service health by pod, last 6h" or "Error logs by service, last hour", and which ones you see follows your datasources.

A prompt is 1 to 4000 characters.

## Watch the agent work

The page switches to a conversation that shows your prompt and, below it, "Building the dashboard" with a progress list.
Each line is one phase of the work, made of the steps the server recorded:

| Phase            | What the agent is doing                                                                                                    |
| ---------------- | -------------------------------------------------------------------------------------------------------------------------- |
| Discovering      | Listing datasources, dashboards, metrics, labels, tables and schemas, sampling rows, and reading the dashboard it revises. |
| Checking queries | Linting a draft query against its datasource, without running it.                                                          |
| Running queries  | Running a query to see that it returns data.                                                                               |
| Saving           | Saving the draft, which runs every panel query again.                                                                      |
| Repairing        | Fixing a draft that the save rejected.                                                                                     |

A line names the tools its phase used, with a count when a tool ran more than once, such as "list_metrics 3".
A phase with more than one step has a **Show N steps** button that lists each step's message, such as "Listed 12 metrics on prom".
A phase that needed a fix gets a warning icon.
While the agent writes, its latest text shows under the list, and "Running for" counts the time since the run started.
The agent only works with names it discovered, so a wrong metric or label usually shows up here as a failed check followed by a repair.

A save is refused when a panel's query is invalid or returns no data.
The agent then repairs the draft and saves again, a few repair rounds.
If it still cannot save, the conversation ends with "The dashboard could not be built" and a **Try again** button.

When the dashboard is saved, the progress list collapses into a line such as "Ran 14 steps in 1m 12s for about $0.12, repaired 1 query", which you can open to see every step.
The cost shows only when the model is priced.
Below it a result card shows the dashboard title, how many panels it has and which datasources they use, and the footer "Saved as version 1" with when it was saved.

## Open the dashboard

Choose **Open dashboard** on the result card.
The dashboard opens in the same tab with the version button reading "Live · v1".

A new dashboard goes into your My Dashboards, where only you can see it.
To share it, open **Share** and publish it to a folder where you have Edit.
In the Dashboards page's **Made by** column it shows "Observe Agent for" and your name.
See [Folders](../teams/folders.md).

Choose **Activity** at the top right to open the activity card.
It lists the saved version, with the agent's one-sentence summary, next to any comments and requests.

## Ask for a change

Go back to the conversation and ask for another change.
In the conversation the box now reads "Ask about `<title>`", with the note "The dashboard changes only when you choose Revise the dashboard on an answer."

```text
add a panel with memory usage by pod, and show the last 12h
```

The agent answers in the chat and leaves the dashboard as it is; when a change fits, the answer ends with **Revise the dashboard** and a one-line description of the change.
Choose it, and the agent revises the latest version of the same dashboard instead of starting a new one.
It keeps every panel you did not ask it to change.
When the run finishes the result card says "Saved as version 2".

If you have the dashboard open, a toast says "Updated to version 2" and the page switches to it.
Version 1 is kept.
You can compare the two versions or restore the older one; see [Comments, agent requests and versions](../use/review.md).

## Ask about one spot, or ask in a comment

You can ask without leaving the dashboard.
Choose **Ask** at the bottom of the screen, or press `a` with the pointer on no panel, and click a spot on a panel, such as a point on a line, a row of a table or a stat's value: a small box opens there, and the answer arrives in it.
The question is a turn of your private chat, which the dashboard's **Chat** tab shows too, and only you and Admins read it.
**Comment**, or `c`, works the same way for a note the team reads, and a comment that mentions `@observe` asks the agent in the thread.
The pages [Ask and comment on a dashboard](../use/ask-and-comment.md) and [Comments, agent requests and versions](../use/review.md#ask-the-agent-from-a-comment) describe both.

## Change the time range and variables

The button at the top of the dashboard shows the current range, for example "Last 6 hours".
Open it to pick a quick range such as "Last 1 hour" or "Yesterday", or type values into **From** and **To**.
Both accept `now-6h`, `now/d`, `2026-10-17 09:00` and an epoch, and **Apply** sets them.
A typed date and time is read in the dashboard's time zone (UTC or the zone it names), or in the browser's zone when the dashboard follows the browser, and the box shows times in that same zone.
A number typed into the boxes is an epoch: 10 digits are read as seconds and 13 digits as milliseconds, and a time before the year 2000 is refused with a message.
A `from` or `to` written in an address or a Spec is always milliseconds when it is all digits.
On a time series panel you can also drag across the chart to zoom to that range.

The time zone box next to it draws the dashboard's times in a zone you choose, such as UTC or `Asia/Tokyo`, for you and anyone who opens your link (`tz` in the address); a panel that sets its own zone keeps it, and the dashboard's own zone is restored by choosing it again.
The **Auto-refresh** menu next to **Refresh now** reloads the panels on an interval.
A dashboard can set its own refresh choices and quick ranges, start on a refresh interval, or hide the time picker, and with it the time zone box, except for **Refresh now**.

If the dashboard has variables, a bar of selectors appears under the top row: a menu for a query or custom variable, with an All choice when the dashboard allows it, a text box for a textbox variable, and so on.
The agent adds variables only when the prompt asks for a breakdown people will filter, so a dashboard from the prompt above may have none.

The range, refresh interval and variable selections live in the page address (`from`, `to`, `refresh` and `var-<name>`), so you can share a link that carries them, and nothing is saved to the dashboard.
The same holds for **Chart hover** in the **More** menu, which Cmd/Ctrl+O cycles through: "This chart only", "Shared crosshair" and "Shared tooltip" choose how hovering one time chart shows on the others.
A choice that differs from the dashboard's saved `cursorSync` goes into the address as `cursor`.
To change the saved default range for everyone, ask the agent in a follow-up, such as "make the default range the last 24 hours".

## Next steps

- [Chat and follow-ups](../use/chat.md) explains what the agent does at each step and what it cannot do.
- [Ask and comment on a dashboard](../use/ask-and-comment.md) covers asking about a spot, in a thread or in a private chat.
- [Comments, agent requests and versions](../use/review.md) covers review, comparing versions and restoring.
- [Edit a panel](../use/edit-panel.md) covers changing a query or a panel by hand.
- [MCP](../use/mcp.md) and [CLI](../use/cli.md) build and edit dashboards from your own tools.
- [Import Grafana dashboards](../use/import-grafana.md) brings in dashboards you already have.
