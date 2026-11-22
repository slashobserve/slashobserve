---
title: Chat and follow-ups
description: How a prompt becomes a dashboard, how to watch the build beside the chat, how to read each turn, and how to find, rename, pin and delete your chats.
---

The chat on Home's **Ask Observe Agent** tab, and on the Ask page at `/ask`, turns a sentence into a saved dashboard.
The server runs the agent against your configured datasources, checks every query it writes, and saves the result as a version of a new dashboard.
It needs a model; see [LLM providers](../connect/llm-providers.md).
For a first run, follow [Your first dashboard](../get-started/first-dashboard.md).

A question about your data such as "what logs are available?", a request to explain a dashboard or a panel, a why question, or something outside the server's data gets an answer instead of a dashboard: the agent reads the data and saves a short written answer in the chat.
A request for the meaning of an observability term, such as "what does p99 mean", gets a definition of two or three lines tied to your data.
A request the server cannot do, such as an alert, a notification or a schedule, gets an answer that says so and what to ask for instead, and builds nothing.
A request for a dashboard builds one, and a message in a dashboard's chat is a question about that dashboard: it gets an answer in the chat and leaves the dashboard as it is, even when it asks for a change.
The dashboard changes only when you choose **Revise the dashboard** on an answer.
The answer stays in the chat, and no observation is created, until you choose **Create observation** on it; an `/investigate` request always opens an observation, and in a chat about an observation it writes into that observation.
When it has none to write into, because no folder can take a new one or the one it names is full, the run's first step reads "No observation to write into; the answer stays on this run", and the answer stays in the chat like any other.

On an answer about a dashboard in a chat, the evidence chip and **Revise the dashboard** work as in a [comment thread](review.md#ask-the-agent-from-a-comment), and **Revise the dashboard** starts the next turn of the chat as a revision that applies that answer, so the server takes the answer's suggested change as its prompt.
Once the change is asked for, or was already applied from that answer, the button reads **Revision asked for**, in the chat and in any thread the answer is in, and is off until you reload the page; asking again while that run is waiting or running, or after it succeeded, starts nothing more.
While no model is connected to the server, the button is off with "No model is connected to this server, so the agent cannot make this change."
On a [starter dashboard](activity.md#audit-folder) the answer keeps its suggested change without the button and offers **Duplicate**, as in a [comment thread](review.md#ask-the-agent-from-a-comment).

The answer's **Answer actions** menu has **Show to everyone**, which opens a thread on the dashboard with your question and the answer, **Share to thread**, which adds both to an open thread you pick, and **Copy answer**, which copies the answer as Markdown.
Only you see the two share choices; anyone else who reads the chat gets **Copy answer** alone.
Once you share the answer, the menu has **Shared in the thread**, which opens that thread on the dashboard, and **Withdraw**, which makes the answer private again: it removes your question and the answer from the thread, and deletes the thread only when sharing opened it, so a thread you shared into keeps its other messages.
Withdraw works only while every message in the thread is yours or the agent's reply to you; once someone else wrote in it, the answer stays shared.
After a withdraw you can share the answer again, which opens a new thread.
Sharing an answer that is already shared, from another tab for example, adds nothing and says "This answer is already shared in its thread".
In a chat about an observation the same menu shares into that observation's threads, and **Show to everyone** also writes the answer onto the page; **Shared in the thread** opens the observation, and a withdraw takes the answer out of the observation again, so it is no longer read or listed through it, while the blocks the share already wrote stay on the page until the observation's editors remove them like any other block.

For why a dashboard or a window looks wrong, use [Investigate](investigate.md).

The answer, the observation saved from it and a dashboard saved from it take a short title the agent writes for the answer, three to eight words; an answer without one is titled `Answer`, or `Finding` for an investigation, and never takes words from your message.
A chat is titled the same way, from your message.

## Start a chat

Type into the box labelled "Message the agent" and press Enter.
Shift+Enter adds a line, and on a touch device Enter adds a line and the arrow button sends.
Inside the box a line says so ("Enter sends. Shift+Enter adds a new line. Up arrow recalls an earlier prompt. Type a slash for commands."); on a touch device it is not shown.
Pressing `/` outside a text field moves the cursor to the box.
A prompt is 1 to 4000 characters, at least one of them visible, and cannot contain control or bidirectional-text characters; see [Text rules](../reference/text-rules.md).
The box shows "Ask a question or describe a dashboard first." when it is empty and "Use at most 4000 characters." when it is too long.
From 3600 characters a counter such as "3650 of 4000" appears in the box's footer, and turns red with "too long" past 4000.
A long message of yours, over 600 characters or 6 lines, is shown cut to 6 lines in the conversation with a **Show the whole message** button.

An address of `/ask?prompt=<text>` opens the box with that text already in it, up to 4000 characters, and sends nothing until you do; `/?prompt=<text>` redirects there.
Adding `dashboard=<id>` puts that dashboard in the box as a chip so the send asks about it: the agent answers without changing the dashboard, and when a change fits, the answer offers it with **Revise the dashboard**.
The chip appears for a dashboard you can comment on, because asking about one needs Comment, and the box stays closed for a moment while the dashboard is looked up, so a prompt is never sent before the chip is there.
For a dashboard you cannot comment on, the box says "You cannot ask about `<title>`, because you cannot comment on it; this message asks without it."
Links inside the app that ask the agent about something use it.

The up arrow, with the cursor at the start of the box, recalls your earlier prompts, newest first, and the down arrow steps back toward what you were typing.
The last 20 prompts are kept in this browser only, never on the server.

A slash command at the start of the box changes what the send does.
Type the command and a space, and it turns into a chip above the box:

| Command        | Chip        | What the send does                                                                |
| -------------- | ----------- | --------------------------------------------------------------------------------- |
| `/investigate` | Investigate | Asks a question and answers in an observation, see [Investigate](investigate.md). |
| `/import`      | Import      | Imports a Grafana dashboard from a link or its JSON.                              |

While you type a command, a line under the box lists the matching commands and says "Type a space after the command to use it."
Remove the chip with its button, or press Backspace in the empty box.
In a chat that already has a dashboard, a send without a command changes that dashboard.
`/import` works only in the chat, and no command works in a dashboard's activity pane.
`/investigate` is offered to Editors and Admins only, because starting an investigation needs the Editor role.
In a chat about a dashboard, a second chip names that dashboard.

The footer shows the model and, when the server sets one, its effort, for example "claude-haiku-5-5, medium effort".
While one of your runs in the chat is queued or running, a square stop button, labelled "Stop the run", sits next to the send button and cancels it, see [Cancel a run](#cancel-a-run).

Everyone can start a dashboard: it goes to your My Dashboards, where only you see it until you publish it.
Name a folder in the request and the agent builds the dashboard there when you can create dashboards in that folder.
If you cannot, or no folder has that name, the agent builds it in My Dashboards and the version summary says so; move it from the dashboards list.
See [Roles](../teams/roles.md).

The chips under the box list the healthy datasources; the agent can only query datasources the server has configured, see [Prometheus](../connect/prometheus.md), [ClickHouse](../connect/clickhouse.md), [Loki](../connect/loki.md) and [Tempo](../connect/tempo.md).
Without a model the box is replaced by "No model is connected to this server."
**Import from Grafana** next to the send button opens [Import Grafana dashboards](import-grafana.md), which needs no model.

The example prompts under the box are built from your datasources and dashboards, at most four of them.
Editors and Admins are offered dashboard prompts, and everyone is offered up to two questions, such as "Which services had the most errors in the last hour?" or "Which routes are slowest right now?".
With Prometheus and a dashboard you can edit, the first dashboard prompt adds a latency row to one you changed, or else to the first one you can edit; the others start a new dashboard, such as "Service health by pod, last 6h" with Prometheus or "Error logs by service, last hour" with Loki or ClickHouse.
Choosing a new-dashboard example sends it at once.
Choosing the latency-row example sends nothing: it opens the box with the text and the dashboard's name as a chip, so you can check or remove the target, edit the text and press Enter.
A dashboard you can only view is never offered.

Under the example prompts, the Ask page at `/ask` lists your saved prompts and those shared with everyone, and typing `/` as the whole first line of the box opens a picker for them; see [Saved prompts](../teams/saved-prompts.md).

Each person may have 3 new-dashboard requests queued or running, and the instance 20.
Past that the server answers `429` with `too_many_requests` or `agent_busy` ("The agent is busy with other requests; try again in a moment."); wait for a request to finish.
Follow-ups do not count toward the 3.

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

| Phase            | Examples of what you see                                       |
| ---------------- | -------------------------------------------------------------- |
| Discovering      | "Listed 14 metrics on prom", "Read the otel_logs schema on ch" |
| Checking queries | "Checked a query on prom: 0 errors"                            |
| Running queries  | "Ran a query on prom: ..."                                     |
| Saving           | "Saved version 1"                                              |
| Repairing        | "Save rejected: 2 issues (...)"                                |

A save is the real check.
A full save runs every panel query, and an `update_panels` save runs only the panels and variables it changed.
Either rejects a panel whose query is invalid or returns no data, except for alert list panels.
The rejection comes back to the agent with the problem for each panel, and the agent fixes the draft and saves again.
The instructions allow three repair attempts; if the agent still cannot save, the request fails.
A save that finishes the dashboard ends the run.

The server writes the version summary from the finished build, not from the agent's last save: a new dashboard reads `Built the dashboard with 6 panels: "A", "B", "C", "D" and 2 more.`, and a revision lists what changed since the version it started from, such as `Added "p99 latency"; renamed "Orders" to "Orders (agent)"; changed the description.`
When the build ends up identical to an earlier version, such as after "undo that", the version is a restore: its summary reads `Restored version 3 (removed "p99 latency").` and the history marks it as restored from that version.
A build that changed nothing keeps the sentence the agent wrote.

Queries the agent runs while drafting are capped at 15 seconds, 200 series, 100,000 points and 1,000 rows, so a heavy query may fail where the saved panel would work.

When the agent answers a question after its reads were refused at its limit of model calls (six in a chat, twelve in an investigation), the answer ends with a line saying reading stopped early and the counts may be partial.
An answer reads deploy markers as you before it says there was no deploy, so a question such as when checkout last deployed is answered from them.
Times in an answer are in your browser's time zone with its label, such as 19:52 IST, because the chat sends that zone with each request.
When a read timed out, its datasource did not answer or the server was too busy to ask it, and no later read of the same kind on that datasource succeeded, the answer ends with a line naming each datasource it could not check and why, so a timeout is never read as no data.

A run has no cap on model calls and no time limit by default; it stops after three rejected saves in a row, when it stops making progress (for example by reading past six model calls without a save, or by repeating a save), or when the person who asked cancels it; see [Cancel a run](#cancel-a-run).
An administrator can set a time limit.
When a run stops early, the conversation ends with "The dashboard could not be built" and a **Try again** button; when it kept a build, you can also continue it, see [Continue a stopped build](#continue-a-stopped-build).

**Try again** asks the server to retry the run (`POST /observe/{id}/retry`), and the server decides whether this run may be tried again: the button shows only while the run's `allowed_actions` include `retry`.
The new run is yours, whoever started the one you retry.
You can try your own failed or stopped run again once: the old turn then reads "Tried again in a new run" with a link to the new one, its `retried_by`, and a second retry, or a **Continue** of the old build, is refused with 409 `invalid_transition` instead of starting a duplicate.
Someone else who retries your run starts a request of their own, which leaves your turn and your retry as they were.
A run that a server shutdown interrupts goes back to the queue and starts again, and one that a crash interrupts gets one more attempt.

Only the server's step messages and errors are shown, and the answer the agent saved, which an investigation also writes into its observation.

A dashboard has 2 to 8 panels unless you ask for another number, and the agent adds variables only when the request asks for a breakdown people will filter on.
It uses the time range you state ("last 6h" becomes `now-6h` to `now`), and otherwise the last hour.

## Watch the build

A large dashboard is not written in one model call.
The agent builds it panel by panel.
Every message opens its chat page at `/c/<request id>` first, because the agent decides only while it works whether the message is a question or a dashboard.
The moment it saves the first panel of a new dashboard, the page moves to the build page at `/b/<request id>`, where each accepted panel appears as soon as it is saved; a question never shows it.
Until then the status line reads "Working on your request" and the steps list what the agent is doing.

Opening `/b/<request id>` of a request that has not saved a panel yet, from a bookmark or a reload, takes you to its chat page instead of an empty canvas.
While the agent keeps the message box disabled, the note under it says to wait for the answer or cancel the run.
While the agent works, the Working row shows what it is writing, one short sentence before a group of tool calls saying what it is about to do and why.
The running entry in a dashboard's activity pane shows the same text.
That text is live only: the server keeps the last 2,000 characters in memory while the run works, and it is not kept once the run ends, so only the saved dashboard or finding stays.
Between two messages of the agent's live text there is a blank line.
Outside the browser, `observe status` prints it as a `Live text` section and `get_run` over MCP returns it as `live_text`; see [CLI](cli.md#follow-ups-and-waiting) and [MCP](mcp.md).

Only you can watch your build: anyone else, and anyone who can no longer view the dashboard you are revising, gets "Only the person who asked can watch this build."
A chat that does not exist, or that someone else asked, shows one message on `/c/<request id>` and on `/b/<request id>`: "You can't open this chat. It does not exist, or only the person who asked and workspace admins can open it."
An Admin who opens someone else's chat whose answer sits in an observation in a folder the Admin cannot see, such as that person's My Dashboards, reads "The answer is saved in an observation in a folder you cannot see."
An answer that was never saved as an observation is hidden the same way when the chat is about a dashboard you cannot open, and the chat reads "The answer is about a dashboard you cannot open."
Nothing appears in the dashboards list, a folder or the version history until the agent finishes.

The build page shows the chat beside the build.
On a screen wider than 1200 px the chat is a pane on the left, which you can widen or narrow by dragging its edge or with the arrow keys on the handle.
On a smaller screen the chat sits below the build.
The pane holds the conversation with the status line, such as "Building, 12 of 40 panels", **Cancel run**, and, when the agent stops, **Save what was built** and **Continue**.
Its heading links to the full chat page at `/c/<request id>`.
The dashboard is saved once, as one version, and the build page then opens it with the chat in the Chat tab of its side panel.

If the agent stops before it finishes, the build page keeps what was built for 7 days.
Choose **Save what was built** to keep it: for a new dashboard it becomes version 1 in the folder the request named, or in your My Dashboards, and for a follow-up it becomes the next version of the dashboard when nobody has saved a version since the build started.
When someone has, the build cannot be saved and the page says so; open the dashboard and ask again.
You need Edit access to the target to save it, and the request stays marked as failed.
Once the build is saved, the build page opens the dashboard.
The chat keeps the result card with "5 of 8 panels, saved before the run finished" when the plan is known, the status line says "Saved part of the dashboard" instead of "Stopped", and **Add the missing panels** asks the agent for the rest as a follow-up on that dashboard.
The button shows only on the newest turn of the chat, so once you have asked for the rest, or anything else, the earlier card no longer offers it.

On the chat page, a card above the steps shows the same status line with thumbnails of the most recent rows and panels built so far, and a **Watch live** link; when the agent stops, the card keeps the thumbnails and offers **Save what was built** and **Continue**.
In the pane beside a build, the card leaves out the link and the thumbnails, because the build is already on screen.
On the dashboard page, the running entry shows the same status line and, for you, an **Open chat** link that opens the request's chat in the Chat tab of the side panel.
Every turn of the conversation keeps its card and buttons for as long as the build is kept, not only the latest ones.

### Queue position

While a request waits for the agent, its status line reads "Queued, position 3".
The position counts the requests ahead of it in the order the agent picks them up.
It can change if a request is skipped, because a dashboard runs one revision at a time and a request whose dashboard is busy waits for it.
Everywhere a queued request is shown, in the conversation, in the chat pane of its build page and in the dashboard's running entry, the line is the same.

### Cancel a run

While a request is queued or running, **Cancel run** stops it.
Only the person who asked sees it; an Editor or Admin cannot cancel someone else's request.
A queued request stops at once.
A running one shows "Canceling" and stops within a few seconds, once the call the agent is making returns.
If its worker is gone, the request ends canceled at once instead.
The request ends as failed with the code `canceled` and keeps what was built, so you can save or continue it.
The chat shows it as a calm "You stopped this run" with what to do next, not as a failure, and the chat lists, History included, say "Stopped" with "Canceled before anything was built" or how far it got, instead of a red error.
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
When the agent lowers the number of panels it planned after you continue, the finished turn says so, for example "Planned 6 panels, built 3."

## Read a turn

While a run works, its turn lists the steps as a timeline, one line per phase in a row: Discovering, Checking queries, Running queries, Saving or Repairing.
Each line names the tools the phase used, with a count when a tool ran more than once, such as "list_metrics 3, get_schema 1".
A phase with more than one step has a **Show N steps** button that lists each step's message.
Saving always lists its messages, because they name the panels, such as "Saved panel 3 of 8: p95 latency".
A phase that needed a fix is marked with a warning icon.
Under the timeline, "Running for 1m 12s" counts the time since the run started.

When the run ends the timeline collapses into a line such as "Ran 14 steps in 1m 12s for about $0.12, repaired 1 query".
The cost is the server's estimate from its price table, and it shows only when the model is priced.
Open it to see every step again.
The result card shows the title, "N panels from `<datasources>`" and the footer "Saved as version 1".
Choose **Open dashboard** to go to it.
The dashboard lands in the folder you named, or in your My Dashboards; publish it from its Share pane to share it, see [Folders](../teams/folders.md).

A failed request shows "The dashboard could not be built" (or "The dashboard could not be updated" for a follow-up), then what the failure means, the reason the server stored, and what to do next.
The advice to save or continue a build shows only while a build is left to save or continue; otherwise the table's last column reads "Try again." or "Try again with a narrower prompt.".
A run you stopped shows "You stopped this run" instead.

| Code                                     | What it means                                              | What to do                                                             |
| ---------------------------------------- | ---------------------------------------------------------- | ---------------------------------------------------------------------- |
| `run_timeout`                            | The run reached the server's time limit.                   | Try again with a narrower prompt, or continue the build.               |
| `call_timeout`                           | A model call took too long to answer.                      | Try again.                                                             |
| `rate_limited`                           | The model provider is limiting requests from this server.  | Wait a minute, then try again.                                         |
| `provider_error`                         | The model provider answered with an error.                 | Try again, and ask an admin to check the provider if it keeps failing. |
| `output_limit`                           | The model's answer was cut off at its output limit.        | Try again and ask for fewer panels.                                    |
| `no_tool_call`, `no_progress`            | The agent stopped making progress.                         | Try again and name the metric, service or table you want.              |
| `validation_exhausted`, `check_failed`   | The agent could not get its queries past the checks; the reason names the panels that kept failing. | Try again and name the metric, service or table you want, or ask for those panels one at a time. |
| `refused`                                | The model declined the request.                            | Rephrase the prompt.                                                   |
| `conflict`                               | Someone saved a newer version while the agent worked.      | Open the dashboard and ask again.                                      |
| `canceled`                               | The run was canceled.                                      | Save what was built, continue it, or try again.                        |
| `requester_forbidden`, `requester_gone`  | You no longer have access to this dashboard.               | Ask an admin for Edit access.                                          |
| `crashed`, `lease_expired`               | The server stopped while it ran this request.              | Try again.                                                             |
| `no_finding`                             | The agent ended without writing a finding.                 | Ask a narrower question, with the service and the time range.          |
| `budget_exhausted`                       | The run reached the server's cost limit.                   | Save what was built or continue it, or ask an admin about the limit.   |
| `agent_unavailable`                      | No outside agent took this run within 10 minutes.          | Check that the agent is connected and asking for work, then try again. |
| `agent_failed`                           | The outside agent said it could not finish the run.        | Read its message, then try again.                                      |

A code not in the table shows the server's reason alone.
**Try again** starts a new request with the same prompt, and when the request already has a dashboard, an **Open dashboard** button is shown next to it.
Anyone who can see a finished request and may start that kind of request can run it again, over the API with `POST /observe/{id}/retry` or with `observe retry`, and the new request is theirs, unless they read it `redacted`: its words stay the requester's, so it offers them no retry and the call answers 409 `invalid_transition`.
When the person who asked retries, the new request stays in the same chat, so the chat lists keep one row for it.
A retry points at the same panel, spot or block as the request it retries; someone else's retry of a private ask, and a retry of a thread question by someone who cannot comment in that thread, points at nothing.
A retried investigation writes into the same observation when the person retrying may comment on it, and otherwise into a new one (see [Observations](observations.md#investigations-write-into-an-observation)).
The states a run moves through, and what each allows, are in [States](../reference/states.md).
Every request has an id, shown as "Request" and its first eight characters at the bottom of the conversation, which you can quote when reporting a problem.

**Copy answer** under an answer puts its Markdown on the clipboard, and **Copy message** next to your own message copies the whole message, also when it is folded.
**Ask again** under the latest answer of your own chat sends the same question once more, in the same chat or in the same dashboard, so you can get a fresh answer without retyping it.
An answer's actions sit in one row, each where it applies: the evidence chip, **Revise the dashboard**, **Create observation**, and, once the answer is in an observation, **Pin to dashboard** and **Make a dashboard**, then **Ask again** and the **Answer actions** menu with **Copy answer**.
It is off while a run goes on.
While a run works you see its live text, but the answer itself is written in one piece when the agent saves it, so there is nothing to copy until then.
The model comes from the server's configuration and cannot be picked in the chat box.

An answer keeps that card after you save it as an observation: the chip still unfolds the evidence, and the link under it reads **Open the observation**, or sits in the row when there is no evidence.
Once the answer is in an observation it can no longer be shared to a thread from the chat, because the observation is where the team reads it; an answer you shared before saving it keeps **Shared in the thread** and **Withdraw**.
The exception is an answer to a question about an observation that you saved into another one: its menu still offers **Show to everyone** and **Share to thread** into the discussion of the observation it was about, and **Withdraw** after it.
An investigation's answer shows as the observation's finding instead, with its evidence open.

The page keeps polling the server, also while its browser tab is in the background, so a finished run leaves "Running" without a reload.
It shows "Reconnecting..." after two failed polls, and "Progress stopped updating." with a **Retry** button when polling stalls.
The run itself goes on server-side either way.

## Follow-ups

Once the first dashboard exists, the box under the conversation reads "Ask about `<title>`", with the note "The dashboard changes only when you choose Revise the dashboard on an answer."
A follow-up there, and **Ask again**, are questions about the dashboard: they get an answer and save no version.
In a chat about an observation, a follow-up and **Ask again** ask about that observation the same way.
What a follow-up is about comes from the chat, so while the chat cannot be read the page shows "Could not load this request" with **Retry** and no box.
Before that, when the agent answered a question instead of building, the box reads "Ask a follow-up, or describe a dashboard to build" and the follow-up continues the same chat: it stays on the same page, below the earlier turns.
While the latest turn still runs the box is disabled with "Wait for the answer, or cancel the run, then ask a follow-up."
An empty follow-up says "Describe the change you want first."

A follow-up is a continuation of the chat.
The agent starts a fresh run that sees the datasources, your new message and, as context, the earlier turns of the conversation: each earlier question with the answer it got, or "Saved version N of the dashboard: ..." for a turn that saved a version.
It gets the last 6 turns, each prompt cut to 600 characters and each answer to 1200, and at most 8000 characters in all, dropping the oldest first.
For a chat about a dashboard these are the earlier requests you sent on that dashboard; for a chat with no dashboard they are the earlier turns of that chat.
Say what you want in the message itself all the same, for example "only 5xx" or "and yesterday?"; the context helps with "and the same for latency", it does not replace the current Spec.

Every request you send is a turn of a chat that you and Admins can read, and nobody else sees the chat.
A request with neither a dashboard nor an observation is private the same way; a request on a dashboard, an investigation included, is shared work that anyone who can see the dashboard reads, and a request on an observation, an ask in its thread included, anyone who can read the observation.
They read that one request only: its `chat_id` is null for them, the first request of a chat shows them its own title rather than the chat's name, and the rest of the chat, the earlier questions the agent was given included, stays yours.
Unless it is an investigation or an answer you shared into a thread they can read, they also read it without your words: its `title`, `prompt`, step messages, `live_text`, `continue_note` and `error` are empty and `redacted` is true, while its state, times, your name, its kind, its `failure_code` and the version it saved stay, and the dashboard's revision list shows its `prompt`, `continue_note` and `error` empty the same way.
In the web app such a run reads "Asked in a private chat" where its message would be, in its page and browser tab title and in the dashboard's activity, its steps show without their messages, and while it runs the dashboard's activity names its current phase instead of its latest step.
Its page is read only: it offers nothing to copy, retry or ask again, and has no message box.
An Admin reads another person's chat read only.

Every read of a chat or a request, yours included, in the web app, over MCP and with the CLI, is recorded in the audit trail as `chat.view` or `run.view` with the owner's id as `owner_id`.
A list of chats or requests is recorded as one `chat.list` or `run.list` with `scope` (`mine` or `all`) in its data, never one event per row.
Each is recorded once per reader and chat, request or list in 10 minutes, so a page that polls adds no events, and a refused read is recorded every time.

Turns of one chat run one at a time in the order they were sent, and the earlier turns are read when a turn starts running, so a follow-up sent while the last one still runs sees its answer.
Over the API, `POST /observe` takes `chat_id` to ask the next turn of one of your chats and `kind` `revise` with `dashboard_id` to change that dashboard (an `ask`, the default, only answers about it), and every request carries its `chat_id`, the id of the chat's first request.
A request an ask in a thread started belongs to no chat: its `chat_id` is null, `GET /chats/{id}` has nothing to open for it, and a further ask in the same thread is another request in that thread, which sees the thread's earlier messages.
Without `chat_id` a new chat starts, except a `revise` on a dashboard, which joins your latest chat on that dashboard unless `new_chat` is true.
`new_chat` matters only for a `revise` on a dashboard, because a private ask already starts its own chat.
`client_id`, a UUID the client picks once per message, makes a resend safe: the same `client_id` answers with the request it started and `replayed: true`.
`GET /chats` lists your chats, `GET /chats/{id}` reads one with every turn, and `DELETE /chats/{id}` archives it with its turns; the CLI has `observe chats` (`list`, `show`, `pin`, `unpin` and `archive`), and MCP `list_chats` and `get_chat`; a chat is pinned with `PATCH /observe/{id}` on its first request, which MCP `pin_run` and `unpin_run` call.

A revise, which **Revise the dashboard** on an answer sends, works on the latest version of that dashboard and saves the result as the next version.
It keeps panel ids, positions and everything else you did not ask to change, and it does not delete panels marked unverified unless you ask.
It saves again only after the same checks as a new dashboard.
If someone saves a version while it works, it restarts once from the newest one.

Requests for one dashboard run one at a time, in the order they were sent, and up to 5 can be queued or running.
A sixth is refused with `too_many_revisions`.

A follow-up and a revise need Comment on the dashboard's folder, which Commenters, Editors and Admins have by default; with Comment but not Edit, a revise ends as a [proposal](../concepts/proposals.md) that someone with Edit merges.
See [Roles](../teams/roles.md).
It does not need to have been your dashboard.

### Answers that read Admin-only data

A run reads a datasource only Admins can query, such as the [audit trail](../reference/audit-queries.md), only when everyone who reads what the run writes is an Admin.
That is an Admin's private chat, or a thread, dashboard, observation or build in a folder only Admins can read; any other run, an Admin's included, is not offered those datasources, and a query on one answers `admins_only_datasource` with where the data can be read.
The same holds for what is stored in a folder only Admins can read: a run that writes where others read sees no such folder, so its dashboards, panels, annotations and observations answer `not_found`, and they are left out of its lists.
A run that may read them is marked when a tool returns one: reading a dashboard, its panels or its annotations, or reading an observation.
Reading the dashboard or observation the run is about is not counted, for an ask and an investigation as for a revision or a suggestion, because what the run learns there goes wherever that dashboard or observation goes; its panel data still counts when it comes from a datasource only Admins can query.
The observation an ask's answer was saved into is not what that ask is about, so a later attempt of the ask that reads it there is counted.
Lists of dashboards, folders and observations show titles only: in an Admin's run they leave out what is in folders only Admins can read, except the built-in Audit folder and its starter dashboards, what the run is about and what is in the folders it writes to, say how many they left out and mark nothing, and `include_restricted: true` lists those too and marks the answer; a run that writes where others read lists none of them and refuses `include_restricted`.
An answer whose run read such a datasource or such content is marked `restricted`, and so is a run that starts from one, because it carries that answer as context: a later turn of the same chat, an ask in a thread that holds one, an apply of one, a continuation of a restricted run, and a run about a dashboard or observation that holds one.

A restricted answer stays where only Admins read it.
Sharing it in a thread or onto a page, saving it as an observation, pinning it, making it a dashboard, applying its suggested change, saving its build and proposing its build as a change are each refused with `409 restricted_answer` unless the place is in a folder only Admins can read, and the message says why.
The same refusal reaches MCP tools and the CLI.

An answer about a dashboard or observation that sits in a folder only Admins can read, when the answer is placed, is kept the same way even when its run read nothing only Admins can query: it may go into that dashboard's or observation's own threads and page, or into a folder only Admins can read, and is refused with `409 restricted_answer` anywhere else; saving it as an observation files it in that folder.
The observation an ask's answer was saved into is not what the ask is about, so its folder keeps nothing; the observation an investigation opened for itself does keep its answer.
A run that starts from such an answer in any of those ways is marked `restricted` too, unless it is about that same dashboard or observation, for example a later turn of the chat on a dashboard in General.
What it was placed in then holds it as it would hold a restricted answer, so that content moves, is copied and has its folder opened only as [Folders](../teams/folders.md) describes, except the dashboard or observation the answer is about, which moves freely.
Once the dashboard or observation has moved to a folder others read, nothing is refused.
Deleting it while it is in a folder only Admins can read, or deleting a run whose answer is kept this way, does not free what was placed from the answer: each dashboard and observation it was placed in keeps holding it.
A run about a dashboard that files into an observation, both in folders only Admins can read, makes each of them hold the other's answer, so neither moves out on its own; delete that run's reply, block or pin from one of them, then move that one out first and the other after it.

A run asked in a thread that ends with a `restricted` answer, when people who are not Admins read that thread, replies there with a fixed text that says the answer is not shown, and only whoever asked and Admins read it, in the run.
A `restricted` run, and a run whose answer is kept for what it is about, answers `404 not_found` to anyone who is neither its requester nor an Admin, wherever a run is returned, and the revision history leaves it out for them, until nothing keeps it any more; its finding says `restricted` for as long.
Saving a restricted answer as an observation files it in the first folder only Admins can read among those of the dashboard it was about, the observation its run files into and the observation it was asked about, else in the built-in Audit folder (see [Audit folder](activity.md#audit-folder)); when neither is Admins only, the save is refused.
Withdrawing a share and the privacy of chats are unchanged.

### In the dashboard's Chat tab

A dashboard's side panel has a **Chat** tab next to **Activity**, which shows your own chats about that dashboard.
It opens your latest chat about the dashboard and names it in the address as `tab=chat&chat=<chat id>`, so the chat stays open when a turn in another chat makes that one the latest.
The select at its top lists your chats on this dashboard, newest first, each with the time of its last message and its number of turns, and **All your chats** opens the Chats page filtered to this dashboard.
**New chat** clears the conversation and shows an empty box, with `chat=new` in the address, so the browser's Back returns to the chat you left; the chat your first message lands in then takes its place in the address.
The first message after **New chat** is a question about the dashboard and starts a new chat, which the tab then shows; the note under the box says "Only you and Admins can see this chat."
A chat that is about another dashboard, or about none, opens read only, with a link to continue it on its chat page.

While the chat has one turn, yours and finished, above the box the tab offers up to two suggested follow-ups, the questions Home suggests for your datasources, such as "Which routes are slowest right now?".
Choosing one asks it about the dashboard in the same chat, without leaving the dashboard, and the suggestions go once the chat has a second turn.
They show when you can ask the agent and are not shown on the chat page.

### From the dashboard's activity pane

You can also send a follow-up from the dashboard.
In its activity pane, write the change in "Comment or request" and either mention `@observe` or choose the "To: everyone here" chip above the box so it reads "To: Observe Agent", then send.
To ask inside a comment thread, choose **Reply** on it: the same box shows "Replying in" and the panel or dashboard, and the send goes to that thread.
The up arrow recalls your earlier prompts there too, and the footer shows the model.
These requests carry on in the activity pane; see [Comments, agent requests and versions](review.md).
In the conversation, a turn sent from the dashboard shows "Sent from the dashboard".

While a request runs, its entry in the activity pane shows the latest three phases of its timeline, with **Show all steps** for the rest, how long it has been running, and **Cancel run** for the person who asked.
While your own request runs, the same stop button sits next to the send button.
When the request saves a version, its entry reads "Saved as version N" with a link to the change between that version and the one before, and says how long the run took.
The dashboard reloads the new version, and panels the version added fade in once.

## Your chats

Home's **Recent chats** lists the five chats you asked in most recently, and **All chats** opens the Chats page at `/chats`.
A chat is one row however many turns it has: its title, how many turns it has, and the dashboard it is about, if any, with the time of its last message.
The Ask page's Recent chats shows the same rows.
The Chats page lists all your chats in a table with the columns Chat, Dashboard, Turns and Last message: the chats you pinned first, then the rest, each part with the chat asked in most recently first, and the order holds as older chats load.
A chat about no dashboard, or about one that was deleted, reads "No dashboard", and one about a dashboard you cannot see, such as one in another person's My Dashboards in the Everyone list, "A dashboard you cannot see".
Search finds chats whose title or any of whose prompts contains the text, ignoring case, **Dashboard** keeps the chats about one dashboard, and older chats load as you scroll.
Admins also get **Mine** and **Everyone**: Everyone lists every person's chats with who started each one, by their last message only, and opens someone else's chat read only, with no composer and no actions.
The list ends with how many chats it shows; while older chats are still to load it says "so far".

History says for each chat its state (Running, Stopped or Done) and then what it left behind.
A row says what the chat left behind: "Saved <dashboard>", "Revised <dashboard>", "Answered, no dashboard" when you asked for a dashboard and the agent only answered, "Answered, dashboard unchanged" for a question about a dashboard, or "Nothing saved".
A run that ended on a suggested panel or a proposal reads "Suggested a panel" or "Opened a proposal".
An investigation reads "Answered in <title>" with the observation it was saved to, as long as you can open that observation's folder.

An answer to a question stays in the chat, under the short title described above, and only you and an Admin can read it, and for a chat on a dashboard only while you can still open that dashboard.
**Create observation** files it in the folder of the dashboard the chat was about, or in your My Dashboards, and the chat then links to the observation; saving twice files it once.

A saved answer to a question about a dashboard or an observation is read by the readers of the observation it was saved in, not by the readers of the dashboard or observation it was about.
Retrying it asks the same private question again in the same chat.
Sharing it afterwards writes its text onto the page of the observation it was about and posts it in that discussion, and the answer stays in the observation it was saved in, also after a withdraw.

On the Chats page, each of your chats has a menu with Rename, Pin and Delete, shown when you point at the row or focus it and always on a touch device.
The actions apply to the whole chat: Rename changes the chat's title, Pin keeps it ahead of the rest, and Delete archives the chat with every turn in it, with a dialog that says how many messages go.
Rows of other people's chats under Everyone have none.
The **History** button beside a chat still lists older chats and Grafana imports.
From a terminal, `observe requests list`, `pin`, `unpin`, `rename` and `archive` work the same chats, `observe save-answer <request-id>` is **Create observation**, and over MCP `list_runs`, `pin_run`, `unpin_run`, `rename_run`, `archive_run` and `save_run_answer` do the same; see [CLI](cli.md#your-requests) and [MCP](mcp.md).

In History, the search box finds chats whose title, prompt or answer contains the text, ignoring case.
Chats you pinned come first under "Pinned", most recently pinned first, and the rest are grouped by the dashboard they are about, with "No dashboard" for chats that never got one and "A dashboard you cannot see" for one you lost access to.
Each chat has a menu with these actions:

- **Rename** turns the row into a title box: Enter saves, Escape cancels, and an empty title goes back to the short title taken from the prompt.
- **Pin** keeps the chat at the top, and **Unpin** puts it back.
- **Delete** asks "Delete this chat?" and, after **Delete chat**, removes it from your chat lists for good. What the dialog says stays depends on what the chat left: "The dashboard it made stays.", "The answer stays in the observation "<title>", which is not deleted.", "The answer stays on the chat page, which still opens from its link." for an answer not saved as an observation, "The panel suggestion stays.", "The proposal stays." or "Nothing else is removed." The chat page still opens from its link.
  A conversation of several messages says how many messages go and names what they saved, such as `What they saved stays: the dashboard "Checkout health" and the observation "Slowest routes".`, or says "Nothing else is removed." when they saved nothing.

Only the person who started a chat can rename, pin or delete it, or ask in it.
Delete archives the chat: the rows are kept and marked archived, and `observe chats archive` and `DELETE /chats/{id}` do the same.
An Admin who can read the chat gets 403 with the code `chat_read_only` from them and from a request naming its `chat_id`, and anyone who cannot read it gets 404.

The Delete dialog shows an unchecked **Also delete the observation "<title>"** for each observation the chat saved that you may delete, which needs Manage on its folder, or Edit if you started the observation.
Checked, the dialog says the observation is deleted for everyone, with its blocks, discussion and published RCA revisions, and **Delete chat and observation** deletes it first and then deletes the chat.
The two differ: the chat only leaves your lists and still opens from its link, while the observation is gone for good and cannot be undone.
If the observation cannot be deleted, the chat is kept and the dialog says why.

### One title for a chat and its observation

A chat with no dashboard and exactly one observation made from it, by **Create observation** or by an investigation that opened it, shares its title with that observation.
Renaming the chat renames the observation, and renaming the observation renames the chat, in the web app, the API, the CLI and MCP alike, and both lists show the new title at once.
When you may not edit the observation, the chat is still renamed and a note says "The observation made from this chat keeps its title, because you cannot edit it."; an observation you cannot open is left alone and never named.
When someone other than the person who started the chat renames the observation, the chat keeps its title and the note says so.
A chat with two or more observations, a chat on a dashboard, a later turn's own title and an observation not made from a chat keep their titles apart.

## What the agent cannot do

- **Use a datasource that is not configured or not healthy**, or names it did not discover.
- **Change a dashboard you cannot edit.** It cannot reach folders you have no Edit access to.
- **Change an existing dashboard from Home.** A request on Home builds a new dashboard and cannot change one that exists.
  When the agent finds exactly one dashboard you meant, its answer is followed by **Ask this on <title>**, which asks the same prompt about that dashboard and opens it as a new chat, and **Open dashboard**.
  The agent answers there without changing the dashboard, and a change it suggests is made only when you choose **Revise the dashboard** on the answer; the button shows when you can comment on the dashboard.
  Without Edit on that dashboard you get only **Open dashboard**, with a line saying someone who can edit it can ask there.
  When it finds none or several, the answer lists the candidates, and you open the one you mean and ask in its activity pane.
- **Add panels with no data.** The save refuses them, and the agent does not create placeholders.
- **Make the same choices every time.** Results depend on the model; a clearer prompt with the metric, service or time range you want gives better dashboards.

To ask a question about your data rather than build a dashboard, use [Investigate](investigate.md).
To work on a dashboard in your own editor and model, use [MCP](mcp.md) or the [CLI](cli.md).
To change one panel by hand, see [Edit a panel](edit-panel.md).
