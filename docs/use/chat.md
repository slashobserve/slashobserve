---
title: Chat and follow-ups
description: How a prompt becomes a dashboard, what the agent does at each step, and how follow-ups revise the same dashboard.
---

The chat on the **Observe** page turns a sentence into a saved dashboard.
The server runs the agent against your configured datasources, checks every query it writes, and saves the result as a version of a new dashboard.
It needs a model; see [LLM providers](../connect/llm-providers.md).
For a first run, follow [Your first dashboard](../get-started/first-dashboard.md).

## Start a chat

Type into the box labelled "Message the agent" and press Enter (Shift+Enter adds a line; on a touch device Enter adds a line and the arrow button sends), or choose one of the example prompts, which sends it at once.
Pressing `/` outside a text field moves the cursor to the box.
A prompt is 1 to 4000 characters and cannot contain control or bidirectional-text characters.
The box shows "Describe the dashboard you want first." when it is empty and "Use at most 4000 characters." when it is too long.

Everyone can start a dashboard: it goes to your My Dashboards, where only you see it until you publish it.
See [Roles](../teams/roles.md).

The chips under the box list the healthy datasources and the name of the model; the agent can only query datasources the server has configured, see [Prometheus](../connect/prometheus.md) and [ClickHouse](../connect/clickhouse.md).
Without a model the box is replaced by "No model is connected to this server."
**Import from Grafana** next to the send button opens [Import Grafana dashboards](import-grafana.md), which needs no model.

Under the example prompts, the chat home lists your saved prompts and those shared with everyone, and typing `/` as the whole first line of the box opens a picker for them; see [Saved prompts](../teams/saved-prompts.md).

Each person may have 3 new-dashboard requests queued or running, and the instance 20.
Past that the server answers `429` with `too_many_requests` or `agent_busy` ("The agent is busy with other requests; try again in a moment."); wait for a request to finish.
Follow-ups do not count toward the 3.

Your recent chats appear on the Observe page under "Recent chats", and the **History** button lists older chats and Grafana imports.

## What the agent does

The agent is a loop of model calls and tool calls run by the server.
It does not see your browser, and it can only act through its tools.
Its instructions hold the Spec guide, the schema and an example, and tell it to use only the datasources it is given and only the metric, label, table and column names it discovered, so it does not invent names.
If the prompt itself contains a Grafana dashboard JSON, the agent imports it with the importer instead of rewriting it, which ends the run like a save; this works for a new dashboard only, and the JSON has to fit in the 4000 characters of the prompt.

Its order of work is:

1. Pick a datasource, then discover what exists: list datasources, metrics, labels and label values on Prometheus, and tables, schemas and sample rows on ClickHouse.
2. Draft the dashboard as an ObserveSpec document.
3. Lint every new or changed query against its datasource.
4. Optionally run a query to see its data.
5. Save.

The conversation shows these as phases, each with a message from the server:

| Phase            | Examples of what you see                                                               |
| ---------------- | -------------------------------------------------------------------------------------- |
| Discovering      | "Listed 14 metrics on prom", "Read the otel_logs schema on ch"                         |
| Checking queries | "Checked a query on prom: 0 errors"                                                    |
| Running queries  | "Ran a query on prom: ..."                                                             |
| Saving           | "Saved version 1"                                                                      |
| Repairing        | "Save rejected: 2 issues (...)"                                                        |

A save is the real check.
A full save runs every panel query, and an `update_panels` save runs only the panels and variables it changed.
Either rejects a panel whose query is invalid or returns no data, except for alert list panels.
The rejection comes back to the agent with the problem for each panel, and the agent fixes the draft and saves again.
The instructions allow three repair attempts; if the agent still cannot save, the request fails.
A save that finishes the dashboard ends the run, and the version summary is a sentence the agent wrote, "Create dashboard" if it wrote none.

Queries the agent runs while drafting are capped at 15 seconds, 200 series, 100,000 points and 1,000 rows, so a heavy query may fail where the saved panel would work.
A run has no cap on model calls and no time limit by default; it stops after three rejected saves in a row, when it stops making progress (for example by reading past six model calls without a save, or by repeating a save), or when the person who asked cancels it; see [Cancel a run](#cancel-a-run).
An administrator can set a time limit.
When a run stops early, the conversation ends with "The dashboard could not be built" and a **Try again** button; when it kept a build, you can also continue it, see [Continue a stopped build](#continue-a-stopped-build).
A run that a server shutdown interrupts goes back to the queue and starts again, and one that a crash interrupts gets one more attempt.

Only the server's step messages and errors are shown.
The model's own text is never displayed, so there is no commentary to read and no answer to a question.

A dashboard has 2 to 8 panels unless you ask for another number, and the agent adds variables only when the request asks for a breakdown people will filter on.
It uses the time range you state ("last 6h" becomes `now-6h` to `now`), and otherwise the last hour.

## Watch the build

A large dashboard is not written in one model call.
The agent builds it panel by panel, and each accepted panel appears in a live view of the dashboard at `/builds/<request id>`, with a status line such as "Building, 12 of 40 panels".
Only you can watch your build: anyone else, and anyone who can no longer view the dashboard you are revising, gets "Only the person who asked can watch this build."
Nothing appears in the dashboards list, a folder or the version history until the agent finishes.
The dashboard is then saved once, as one version, and the live view opens it.

If the agent stops before it finishes, the live view keeps what was built for 7 days.
Choose **Save what was built** to keep it: for a new dashboard it becomes version 1 in your My Dashboards, and for a follow-up it becomes the next version of the dashboard when nobody has saved a version since the build started.
When someone has, the build cannot be saved and the page says so; open the dashboard and ask again.
You need Edit access to the target to save it, and the request stays marked as failed.

In the conversation, a card above the progress list shows the same status line with thumbnails of the most recent rows and panels built so far, and a **Watch live** link; when the agent stops, the card keeps the thumbnails and offers **Save what was built** and **Continue**.
On the dashboard page, the running entry shows the same status line and, for you, a **Watch live** link.
Every turn of the conversation keeps its card and buttons for as long as the build is kept, not only the latest ones.

### Queue position

While a request waits for the agent, its status line reads "Queued, position 3".
The position counts the requests ahead of it in the order the agent picks them up.
It can change if a request is skipped, because a dashboard runs one revision at a time and a request whose dashboard is busy waits for it.
Everywhere a queued request is shown, in the conversation, on its build page and in the dashboard's running entry, the line is the same.

### Cancel a run

While a request is queued or running, **Cancel run** stops it.
Only the person who asked sees it; an Editor or Admin cannot cancel someone else's request.
A queued request stops at once.
A running one shows "Canceling" and stops within a few seconds, once the call the agent is making returns.
The request ends as failed with "the run was canceled" and keeps what was built, so you can save or continue it.
If the agent had already saved the finished dashboard, the request counts as succeeded and the cancel changes nothing.

Other people's follow-ups on the same dashboard wait behind a run until it ends, so a run that goes nowhere holds them back until its owner cancels it or one of its limits stops it.

### Continue a stopped build

**Continue** on a stopped build starts a new run that picks up from the panels already built.
Choosing it opens a form with an optional note for the agent, such as "skip the network rows" or "use the prod datasource".
The note follows the prompt rule: 1 to 4000 characters, with no control or bidirectional-text characters, and the form shows "Use at most 4000 characters." when it is too long.
A blank note sends none.
Choose **Continue build** to start the run.

The new turn appears below the old one, marked "Continues an earlier run" with your note under it, and the stopped turn says "Continued in a new run".
The agent sees the note in every start of that run, including a restart after a server shutdown.
A new dashboard continues on its own page, and a follow-up continues in the same conversation.

Only the person who asked can continue a build, and it needs the same access as starting a run.
A build can be continued once, and not after you saved it; **Save what was built** and **Continue** exclude each other, so whichever you choose first wins and the other says it can no longer be done.
A follow-up whose dashboard got a newer version since the build started cannot be continued: the page says "Someone saved a newer version after this build started, so it can't be continued. Open the dashboard and ask again."
A continued run counts toward the same limits as a new request.

## Read the result

When the run ends the progress list collapses into a line such as "Ran 14 steps, repaired 1 query".
Open it to see every step.
The result card shows the title, "N panels from `<datasources>`" and the footer "Saved as version 1".
Choose **Open dashboard** to go to it.

The dashboard lands in your My Dashboards; publish it from its Share pane to share it, see [Folders](../teams/folders.md).
A failed request shows "The dashboard could not be built" (or "The dashboard could not be updated" for a follow-up) with the reason the server stored, and **Try again** starts a new request with the same prompt.
When the request already has a dashboard, an **Open dashboard** button is shown next to it.
Every request has an id, shown as "Request" and its first eight characters at the bottom of the conversation, which you can quote when reporting a problem.

The page keeps polling the server.
It shows "Reconnecting..." after two failed polls, and "Progress stopped updating." with a **Retry** button when polling stalls.
The run itself goes on server-side either way.

## Follow-ups

Once the first dashboard exists, the box under the conversation reads "Ask for another change to `<title>`", with the note "Each change is saved as a new version of the dashboard."
Until then it is disabled with "Wait for the dashboard, then ask for changes."
An empty follow-up says "Describe the change you want first."

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
- **Change a dashboard you cannot edit.** It cannot reach folders you have no Edit access to.
- **Add panels with no data.** The save refuses them, and the agent does not create placeholders.
- **Make the same choices every time.** Results depend on the model; a clearer prompt with the metric, service or time range you want gives better dashboards.

To work on a dashboard in your own editor and model, use [MCP](mcp.md) or the [CLI](cli.md).
To change one panel by hand, see [Edit a panel](edit-panel.md).
