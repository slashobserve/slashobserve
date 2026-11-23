---
title: Your first dashboard
description: Ask the agent for a dashboard on the demo stack, watch it work, open the result, ask for a change and set the time range.
---

This walkthrough has you ask the built-in agent for a dashboard, watch it build one from your datasources, and then change the result.
It starts where the [quick start](quick-start.md) stops: the demo runs at http://localhost:58080 and you are signed in as `admin@example.com`.

The agent needs a model.
You set `SLASHOBSERVE_LLM_PROVIDER` and `SLASHOBSERVE_LLM_API_KEY` before `make demo`.
Without one, the chat shows "No model is connected to this server." and prompts get `503 agent_unavailable`; see [LLM providers](../connect/llm-providers.md).

## Ask for a dashboard

1. Open Home at `/`.
   Its **Ask Observe Agent** tab has a box with the placeholder "Ask a question or describe a dashboard".
   Pressing `/` anywhere on the page moves focus to the box.
2. Type a prompt:

   ```text
   checkout health by service, last hour
   ```

3. Press Enter, or choose **Send**.
   Shift+Enter starts a new line, and the Up arrow recalls an earlier prompt.

A prompt is 1 to 4000 characters.

Under the box, a dot and a name mark each healthy datasource the agent may use, followed by the name of the model.
In the demo the datasources are `prom` (Prometheus), `ch` (ClickHouse), `loki` (Loki), `tempo` (Tempo) and `jaeger` (Jaeger).
Two more, `es` (Elasticsearch) and `os` (OpenSearch), appear when you started the stack with `make demo-search`.

Under the box, **Example prompts** are buttons that send a ready-made prompt at once.
There are at most four, and they follow your datasources and your role.
An Editor or Admin sees prompts that build a dashboard, such as "Service health by pod, last 6h" or a request to add a row to a dashboard you can edit, and everyone sees up to two questions, such as "Which routes are slowest right now?".

## Watch the agent work

The page switches to a conversation that shows your prompt.
Below it, "Building the dashboard" and a progress list appear.
A request that only asks a question shows "Working on your request" instead, and a revision shows "Revising" and the dashboard's title.

Each line of the list is one phase of the work, built from the steps the server recorded:

1. Discovering: the agent lists datasources, metrics, labels, tables and schemas, and samples rows.
2. Checking queries and Running queries: it lints a draft query and runs it to see that it returns data.
3. Saving: it saves the draft, which runs every panel query again.
4. Repairing: it fixes a draft that the save rejected.

A line names the tools its phase used, with a count when a tool ran more than once, such as "list_metrics 3".
A phase with more than one step has a **Show N steps** button that lists each step's message, such as "Listed 12 metrics on prom".
A phase that needed a fix gets a warning icon.
While the agent writes, its latest text shows under the list, and "Running for" counts the time since the run started.

The agent only works with names it discovered, so a wrong metric or label usually shows up here as a failed check followed by a repair.
A save is refused when a panel's query is invalid or returns no data.
The agent then repairs the draft and saves again, for a few repair rounds.
If it still cannot save, the conversation ends with "The dashboard could not be built" and a **Try again** button.

When the dashboard is saved, the progress list collapses into one line, such as "Ran 14 steps in 1m 12s for about $0.12, repaired 1 query".
Open the line to see every step.
The cost shows only when the model is priced.
Below the line, a result card shows the dashboard title, how many panels it has and which datasources they use.
Its footer reads "Saved as version 1", with when it was saved.

## Open the dashboard

1. Choose **Open dashboard** on the result card.
   The dashboard opens in the same tab, and its version button reads "Live · v1".
2. Choose **Activity** at the top right to open the activity card.
   It lists the saved version, with the agent's one-sentence summary, next to any comments and requests.

A new dashboard goes into your My Dashboards, where only you can see it.
In the Dashboards page's **Made by** column it shows "Observe Agent for" and your name.
To share it, open **Share** and publish it to a folder where you have Edit; see [Folders](../teams/folders.md).

## Ask for a change

1. Go back to the conversation.
   The box now reads "Ask about" and the dashboard's title, and a note says "The dashboard changes only when you choose Revise the dashboard on an answer."
2. Ask for the change:

   ```text
   add a panel with memory usage by pod, and show the last 12h
   ```

3. Read the answer.
   The agent answers in the chat and leaves the dashboard as it is.
   When a change fits, the answer ends with **Revise the dashboard** and a one-line description of the change.
4. Choose **Revise the dashboard**.
   The agent revises the latest version of the same dashboard instead of starting a new one, and keeps every panel you did not ask it to change.
   When the run finishes, the result card says "Saved as version 2".

If you have the dashboard open, a toast says "Updated to version 2" and the page switches to it.
Version 1 is kept.
You can compare the two versions or restore the older one; see [Comments, agent requests and versions](../use/review.md).

## Ask about one spot, or ask in a comment

You can ask without leaving the dashboard.

1. Choose **Ask** at the bottom of the screen, or press `a` with the pointer on no panel.
2. Click a spot on a panel, such as a point on a line, a row of a table or a stat's value.
   A small box opens there, and the answer arrives in it.

The question is a turn of your private chat, which the dashboard's **Chat** tab shows too, and only you and Admins read it.
**Comment**, or `c`, works the same way for a note the team reads, and a comment that mentions `@observe` asks the agent in the thread.
[Ask and comment on a dashboard](../use/ask-and-comment.md) and [Comments, agent requests and versions](../use/review.md#ask-the-agent-from-a-comment) describe both.

## Change the time range

The button at the top of the dashboard shows the current range, for example "Last 6 hours".

1. Open the button.
2. Pick a quick range such as "Last 1 hour" or "Yesterday", or type values into **From** and **To**.
3. Choose **Apply**.

On a time series panel you can also drag across the chart to zoom to that range.
The range, the refresh interval and the variable selections live in the page address, as `from`, `to`, `refresh` and `var-<name>`.
You can share a link that carries them, and nothing is saved to the dashboard.
To change the saved default range for everyone, ask the agent in a follow-up, such as "make the default range the last 24 hours".

### What From and To accept

**From** and **To** accept `now-6h`, `now/d`, `2026-10-17 09:00` and an epoch.

- A typed date and time is read in the dashboard's time zone (UTC or the zone it names).
  When the dashboard follows the browser, it is read in the browser's zone.
  The box shows times in that same zone.
- A number typed into the boxes is an epoch: 10 digits are read as seconds and 13 digits as milliseconds.
  A time before the year 2000 is refused with a message.
- A `from` or `to` written in an address or a Spec is always milliseconds when it is all digits.

### Time zone and refresh

The time zone box next to the range draws the dashboard's times in a zone you choose, such as UTC or `Asia/Tokyo`.
It applies to you and to anyone who opens your link, because the address carries it as `tz`.
A panel that sets its own zone keeps it, and choosing the dashboard's own zone restores it.

The **Auto-refresh** menu next to **Refresh now** reloads the panels on an interval.
A dashboard can set its own refresh choices and quick ranges, start on a refresh interval, or hide the time picker.
Hiding the time picker also hides the time zone box, but not **Refresh now**.

### Variables

If the dashboard has variables, a bar of selectors appears under the top row.
A query or custom variable is a menu, with an All choice when the dashboard allows it, and a textbox variable is a text box.
The agent adds variables only when the prompt asks for a breakdown people will filter, so a dashboard from the prompt above may have none.

### Chart hover

**Chart hover** in the **More** menu chooses how hovering one time chart shows on the others.
Cmd/Ctrl+O cycles through "This chart only", "Shared crosshair" and "Shared tooltip".
The choice is not saved to the dashboard.
A choice that differs from the dashboard's saved `cursorSync` goes into the address as `cursor`.

## Next steps

- [Chat and follow-ups](../use/chat.md) explains what the agent does at each step and what it cannot do.
- [Ask and comment on a dashboard](../use/ask-and-comment.md) covers asking about a spot, in a thread or in a private chat.
- [Comments, agent requests and versions](../use/review.md) covers review, comparing versions and restoring.
- [Edit a panel](../use/edit-panel.md) covers changing a query or a panel by hand.
- [MCP](../use/mcp.md) and [CLI](../use/cli.md) build and edit dashboards from your own tools.
- [Import Grafana dashboards](../use/import-grafana.md) brings in dashboards you already have.

## Reference

### Phases of a build

| Phase | What the agent does |
| --- | --- |
| Discovering | Lists datasources, dashboards, metrics, labels, tables and schemas, samples rows and reads the dashboard it revises. |
| Checking queries | Lints a draft query against its datasource, without running it. |
| Running queries | Runs a query to see that it returns data. |
| Saving | Saves the draft, which runs every panel query again. |
| Repairing | Fixes a draft that the save rejected. |

### Keys on Home

| Key | What it does |
| --- | --- |
| `/` | Moves focus to the box. |
| Enter | Sends the prompt. |
| Shift+Enter | Starts a new line. |
| Up arrow | Recalls an earlier prompt. |
