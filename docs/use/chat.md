---
title: Chat and follow-ups
description: How a prompt becomes a dashboard, what the agent does at each step, and how follow-ups revise the same dashboard.
---

The chat on the **Observe** page turns a sentence into a saved dashboard.
The server runs the agent against your configured datasources, checks every query it writes, and saves the result as a version of a new dashboard.
It needs a model; see [LLM providers](../connect/llm-providers.md).
For a first run, follow [Your first dashboard](../get-started/first-dashboard.md).

## Start a chat

Type into the box labelled "Message the agent" and press Enter (Shift+Enter adds a line), or choose one of the example prompts.
A prompt is 1 to 4000 characters and cannot contain control or bidirectional-text characters.
The box shows "Describe the dashboard you want first." when it is empty and "Use at most 4000 characters." when it is too long.

Starting a dashboard needs the Editor role, because the agent creates a dashboard for you.
A Commenter or Viewer sees "Starting dashboards needs the Editor role. Ask an Admin if you need it."
See [Roles](../teams/roles.md).

The chips under the box list the healthy datasources; the agent can only query datasources the server has configured, see [Prometheus](../connect/prometheus.md) and [ClickHouse](../connect/clickhouse.md).

Each person may have 3 new-dashboard requests queued or running, and the instance 20.
Past that the server answers `429` with `too_many_requests` or `agent_busy` ("The agent is busy with other requests; try again in a moment."); wait for a request to finish.
Follow-ups do not count toward the 3.

Your recent chats appear on the Observe page under "Recent chats", and the **History** button lists older chats and Grafana imports.

## What the agent does

The agent is a loop of model calls and tool calls run by the server.
It does not see your browser, and it can only act through its tools.
Its instructions tell it to use only the datasources it is given and only the metric, label, table and column names it discovered, so it does not invent names.

Its order of work is:

1. Pick a datasource, then discover what exists: list datasources, metrics, labels and label values on Prometheus, and tables, schemas and sample rows on ClickHouse.
2. Draft the dashboard as an ObserveSpec document, reading the Spec guide first.
3. Lint every new or changed query against its datasource.
4. Optionally run a query to see its data.
5. Save.

The conversation shows these as phases, each with a message from the server:

| Phase            | Examples of what you see                                                               |
| ---------------- | -------------------------------------------------------------------------------------- |
| Discovering      | "Listed 14 metrics on prom", "Read the otel_logs schema on ch", "Read the Spec guide"  |
| Checking queries | "Checked a query on prom: 0 errors"                                                    |
| Running queries  | "Ran a query on prom: ..."                                                             |
| Saving           | "Saved version 1"                                                                      |
| Repairing        | "Save rejected: 2 issues (...)"                                                        |

A save is the real check.
It runs every panel query and rejects a panel whose query is invalid or returns no data, except for alert list panels.
The rejection comes back to the agent with the problem for each panel, and the agent fixes the draft and saves again.
The instructions allow three repair attempts; if the agent still cannot save, the request fails.
A successful save ends the run, and the version summary is a sentence the agent wrote, "Create dashboard" if it wrote none.

Queries the agent runs while drafting are capped at 15 seconds, 200 series, 100,000 points and 1,000 rows, so a heavy query may fail where the saved panel would work.
A run gets 3 minutes and at most 20 model calls.
When it runs out of either, the conversation ends with "The dashboard could not be built" and a **Try again** button.
A run interrupted by a restart is picked up again once.

Only the server's step messages and errors are shown.
The model's own text is never displayed, so there is no commentary to read and no answer to a question.

A dashboard has 2 to 8 panels unless you ask for another number, and the agent adds variables only when the request asks for a breakdown people will filter on.
It uses the time range you state ("last 6h" becomes `now-6h` to `now`), and otherwise the last hour.

## Read the result

When the run ends the progress list collapses into a line such as "Ran 14 steps, repaired 1 query".
Open it to see every step.
The result card shows the title, "N panels from <datasources>" and the footer "Saved as version 1".
Choose **Open dashboard** to go to it.

The dashboard lands in the folder you last created a dashboard in, or General when you have not created one; see [Folders](../teams/folders.md).
A failed request shows "The dashboard could not be built" with the reason the server stored, and **Try again** starts a new request with the same prompt.
Every request has an id, shown as "Request" and its first eight characters at the bottom of the conversation, which you can quote when reporting a problem.

The page keeps polling the server.
It shows "Reconnecting..." after two failed polls, and "Progress stopped updating." with a **Retry** button when polling stalls.
The run itself goes on server-side either way.

## Follow-ups

Once the first dashboard exists, the box under the conversation reads "Ask for another change to <title>", with the note "Each change is saved as a new version of the dashboard."
Until then it is disabled with "Wait for the dashboard, then ask for changes."

A follow-up is not a continuation of the chat.
The agent starts a fresh run that sees the datasources, your new message and the current Spec of the dashboard, not the earlier messages.
Say what to change in the message itself, for example "only show 5xx" or "add a panel with memory by pod".

The agent revises the latest version of that dashboard and saves the result as the next version.
It keeps panel ids, positions and everything else you did not ask to change, and it does not delete panels marked unverified unless you ask.
It saves again only after the same checks as a new dashboard.
If someone saves a version while it works, it restarts once from the newest one.

Requests for one dashboard run one at a time, in the order they were sent, and up to 5 can be queued or running.
A sixth is refused with `too_many_revisions`.

You can also send a follow-up from the dashboard.
In its activity card, write the change in "Write a comment, or tell the agent what to change" and choose **Send to agent**, or reply to a comment thread and choose **Send to agent** there.
These requests carry on in the activity card; see [Comments, agent requests and versions](review.md).
In the conversation, a turn sent from the dashboard shows "Sent from the dashboard".

A follow-up needs Edit on the dashboard's folder, which Editors and Admins have by default; see [Roles](../teams/roles.md).
It does not need to have been your dashboard.

## What the agent cannot do

- **Answer questions.** It only produces dashboards, and its text is not shown.
- **Use a datasource that is not configured or not healthy**, or names it did not discover.
- **Work on a dashboard over 128 KiB of Spec.** It fails with "the dashboard is too large for the agent to revise".
  A dashboard holds at most 200 panels.
- **Change a dashboard you cannot edit.** It cannot reach folders you have no Edit access to.
- **Add panels with no data.** The save refuses them, and the agent does not create placeholders.
- **Make the same choices every time.** Results depend on the model; a clearer prompt with the metric, service or time range you want gives better dashboards.
- **Make ad hoc filters apply to ClickHouse on their own.** Prometheus applies them to every query on its datasource, but a ClickHouse query uses an ad hoc filter only where its WHERE clause contains `AND $name`.

To work on a dashboard in your own editor and model, use [MCP](mcp.md) or the [CLI](cli.md).
To change one panel by hand, see [Edit a panel](edit-panel.md).
