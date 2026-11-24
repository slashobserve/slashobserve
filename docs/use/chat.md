---
title: Chat and follow-ups
description: Ask the agent for a dashboard or an answer in plain words, watch the build, read each turn, follow up, share answers, and find, rename, pin and delete your chats.
---

The chat on Home's **Ask Observe Agent** tab, and on the Ask page at `/ask`, turns a sentence into a saved dashboard or a written answer.
The server runs the agent against your configured datasources and checks every query it writes.
A request for a dashboard saves a new dashboard, and a question gets an answer in the chat.
The chat needs a model; see [LLM providers](../connect/llm-providers.md).
For a first run, follow [Your first dashboard](../get-started/first-dashboard.md).

The same chat works from the CLI (`observe "<prompt>"`), over MCP (`start_run`) and over HTTP (`POST /api/v1/observe`); [Use the chat from the CLI, MCP or HTTP](#use-the-chat-from-the-cli-mcp-or-http) shows each.

## Ask for a dashboard

1. On Home, open **Ask Observe Agent**, or open `/ask`.
2. Click the box that reads "Ask a question or describe a dashboard, like: checkout health by pod, last 6h" and press Enter.
   For example: `checkout p99 latency, last 6h`.
   Shift+Enter adds a line.
3. The chat opens at `/c/<request id>`.
   The status line reads "Working on your request", and the steps list what the agent is doing, such as "Listed 14 metrics on prom".
4. When the agent saves the first panel of a new dashboard, the page moves to the build page at `/b/<request id>`.
   Each accepted panel appears as soon as it is saved, and the status line reads "Building, 12 of 40 panels".
5. When the build ends, the chat shows a result card with the title, "N panels from `<datasources>`" and "Saved as version 1".
   Choose **Open dashboard** to go to it.

The dashboard goes to your My Dashboards, where only you see it until you publish it from its Share pane; see [Folders](../teams/folders.md).
Name a folder in the request and the agent builds the dashboard there when you can create dashboards in that folder.
If you cannot, or no folder has that name, the agent builds it in My Dashboards and the version summary says so.
Move it from the dashboards list; see [Roles](../teams/roles.md).

A dashboard has 2 to 8 panels unless you ask for another number.
The agent adds variables only when the request asks for a breakdown people will filter on.
It uses the time range you state ("last 6h" becomes `now-6h` to `now`), and otherwise the last hour.

If the run stops early, the chat ends with "The dashboard could not be built" and a **Try again** button; [Read a turn](#read-a-turn) lists the causes.
To bring a dashboard over without a model, use the **Import a dashboard** tab on Home, or the **Import from Grafana** link on the Ask page.
Both open [Import Grafana dashboards](import-grafana.md).
With no model connected, the link sits under the "No model is connected to this server." note, and Home's **Ask Observe Agent** tab does not show it.

## Ask a question about your data

A message that is not a request for a dashboard gets an answer instead.
The agent reads the data and saves a short written answer in the chat.
These get an answer:

- a question about your data, such as "what logs are available?";
- a request to explain a dashboard or a panel, and a why question;
- something outside the server's data;
- the meaning of an observability term, such as "what does p99 mean", which gets a definition of two or three lines tied to your data.

A request the server cannot do, such as an alert, a notification or a schedule, gets an answer that says so and what to ask for instead.
It builds nothing.

An answer stays in the chat, and no [observation](observations.md) is created, until you choose **Create observation** on it.
An `/investigate` request always opens an observation, and in a chat about an observation it writes into that observation.
When there is none to write into, because no folder can take a new one or the one it names is full, the run's first step reads "No observation to write into; the answer stays on this run", and the answer stays in the chat like any other.

An answer, the observation saved from it and a dashboard saved from it take a short title the agent writes, three to eight words.
An answer without one is titled `Answer`, or `Finding` for an investigation, and never takes words from your message.
A chat takes its title from the start of your message instead: its first line, cut at a word to 60 characters.

For why a dashboard or a window looks wrong, use [Investigate](investigate.md).

### What an answer offers

An answer's actions sit in one row, each where it applies:

- the evidence chip, which unfolds the panels behind the answer;
- **Revise the dashboard**, on an answer about a dashboard;
- **Create observation**, and once the answer is in an observation, **Pin to dashboard** and **Make a dashboard**, which shows only when the answer has evidence panels;
- **Ask again**, and the **Answer actions** menu with **Copy answer**.

**Copy answer** puts the answer's Markdown on the clipboard, and **Copy message** next to your own message copies the whole message, also when it is folded.
**Ask again** under the latest answer of your own chat sends the same question once more, in the same chat or on the same dashboard.
It is off while a run goes on.
While a run works you see its live text, but the answer is written in one piece when the agent saves it, so there is nothing to copy until then.

**Create observation** files the answer in the folder of the dashboard the chat was about, or in your My Dashboards, and the chat then links to the observation.
Saving twice files it once.
The answer keeps its card afterwards: the chip still unfolds the evidence, and the link under it reads **Open the observation**, or sits in the row when there is no evidence.
An investigation's answer shows as the observation's finding instead, with its evidence open.

## Change a dashboard from an answer

A message in a dashboard's chat is a question about that dashboard.
It gets an answer in the chat and leaves the dashboard as it is, even when it asks for a change.
The dashboard changes only when you choose **Revise the dashboard** on an answer.

1. Ask about the dashboard, for example in its **Chat** tab: "Add a latency row".
2. When a change fits, the answer offers it with **Revise the dashboard**.
3. Choose **Revise the dashboard**.
   The chat starts its next turn as a revision that applies that answer, so the server takes the answer's suggested change as its prompt.
4. The revision saves the result as the next version of the dashboard.
   The turn reads "Saved as version N".

Once the change is asked for, or was already applied from that answer, the button reads **Revision asked for**, in the chat and in any thread the answer is in.
It stays off until you reload the page.
Asking again while that run is waiting or running, or after it succeeded, starts nothing more.
While no model is connected, the button is off with "No model is connected to this server, so the agent cannot make this change."
On a [starter dashboard](activity.md#audit-folder) the answer keeps its suggested change without the button and offers **Duplicate**.
On an answer about a dashboard, the evidence chip and **Revise the dashboard** work as in a [comment thread](review.md#ask-the-agent-from-a-comment).

You need Comment on the dashboard's folder, which Commenters, Editors and Admins have by default.
With Comment but not Edit, a revise ends as a [proposal](../concepts/proposals.md) that someone with Edit merges.
It does not need to have been your dashboard.
See [Roles](../teams/roles.md).

A revise works on the latest version of that dashboard.
It keeps panel ids, positions and everything else you did not ask to change.
It does not delete panels marked unverified unless you ask.
It saves only after the same checks as a new dashboard.
If someone saves a version while it works, it restarts once from the newest one.

### Change an existing dashboard from Home

A request on Home builds a new dashboard and cannot change one that exists.
When the agent finds exactly one dashboard you meant, its answer is followed by **Ask this on `<title>`** and **Open dashboard**.
**Ask this on `<title>`** asks the same prompt about that dashboard and opens it as a new chat.
The agent answers there without changing the dashboard, and the change is made only when you choose **Revise the dashboard**, which shows when you can comment on the dashboard.
Without Comment on that dashboard you get only **Open dashboard**, with the line "This request is about the dashboard `<title>`, where you cannot ask the agent."
With Comment the line reads "This request is about the dashboard `<title>`. Ask there and the agent answers about it; a change it suggests is made only when you choose Revise the dashboard."
When the agent finds none or several, the answer lists the candidates, and you open the one you mean and ask in its activity pane.

## Share an answer

The **Answer actions** menu of your own answer has these choices:

- **Show to everyone**, when the menu offers it, opens a thread on the dashboard with your question and the answer.
- **Share to thread** adds both to an open thread you pick.
- **Copy answer** copies the answer as Markdown.

Only you see the two share choices; anyone else who reads the chat gets **Copy answer** alone.

Once you share the answer, the menu has **Shared in the thread**, which opens that thread on the dashboard, and **Withdraw**.
**Withdraw** makes the answer private again: it removes your question and the answer from the thread.
It deletes the thread only when sharing opened it, so a thread you shared into keeps its other messages.
Withdraw works only while every message in the thread is yours or the agent's reply to you.
Once someone else wrote in it, the answer stays shared.
After a withdraw you can share the answer again, which opens a new thread.
Sharing an answer that is already shared, from another tab for example, adds nothing and says "This answer is already shared in its thread".

In a chat about an observation the same menu shares into that observation's threads.
**Show to everyone** also writes the answer onto the page, **Shared in the thread** opens the observation, and a withdraw takes the answer out of the observation again, so it is no longer read or listed through it.
The blocks the share already wrote stay on the page until the observation's editors remove them like any other block.

Once the answer is in an observation, the web app no longer offers to share it to a thread from the chat, because the observation is where the team reads it.
An answer you shared before saving it keeps **Shared in the thread** and **Withdraw**.
The exception is an answer to a question about an observation that you saved into another one: its menu still offers **Show to everyone** and **Share to thread** into the discussion of the observation it was about, and **Withdraw** after it.

A saved answer to a question about a dashboard or an observation is read by the readers of the observation it was saved in, not by the readers of the dashboard or observation it was about.
Sharing it afterwards writes its text onto the page of the observation it was about and posts it in that discussion.
The answer stays in the observation it was saved in, also after a withdraw.
Retrying it asks the same private question again in the same chat.

## Ask a follow-up

Once the first dashboard exists, the box under the conversation reads "Ask about `<title>`", with the note "The dashboard changes only when you choose Revise the dashboard on an answer."
A follow-up there, and **Ask again**, are questions about the dashboard: they get an answer and save no version.
In a chat about an observation, a follow-up and **Ask again** ask about that observation the same way.

Before that, when the agent answered a question instead of building, the box reads "Ask a follow-up, or describe a dashboard to build".
The follow-up continues the same chat, below the earlier turns.
In a chat with no dashboard, while the latest turn still runs, the box is disabled and the note under it reads "Wait for the answer, or cancel the run, then ask a follow-up."
An empty follow-up says "Describe the change you want first." on a dashboard and "Ask a question or describe a dashboard first." without one.
What a follow-up is about comes from the chat, so while the chat cannot be read the page shows "Could not load this request" with **Retry** and no box.

A follow-up starts a fresh run that sees the datasources, your new message and the earlier turns of the conversation as context.
Say what you want in the message itself all the same, for example "only 5xx" or "and yesterday?".
The context helps with "and the same for latency", but it does not replace the dashboard definition.
[Follow-ups](#follow-ups) has the exact context rules and the API fields.

On a dashboard you can also ask from the side panel.
[In the dashboard's Chat tab](#in-the-dashboards-chat-tab) and [From the dashboard's activity pane](#from-the-dashboards-activity-pane) describe the two places.
[Ask mode](ask-and-comment.md#ask-mode) asks about one spot of a panel as a turn of the chat open in the Chat tab, or of a new chat when none is open.
Its answer and follow-ups show both in the box at that spot and in the Chat tab, and each follow-up there carries the same spot.

## Watch the build

A large dashboard is not written in one model call.
The agent builds it panel by panel.
Every message opens its chat page at `/c/<request id>` first, because the agent decides only while it works whether the message is a question or a dashboard.
The moment it saves the first panel of a new dashboard, the page moves to the build page at `/b/<request id>`.
A question never shows the build page.

Until the first panel is saved, the status line reads "Working on your request" and the steps list what the agent is doing.
Opening `/b/<request id>` of a request that has not saved a panel yet, from a bookmark or a reload, takes you to its chat page instead of an empty canvas.
While the agent keeps the message box disabled, the note under it says to wait for the answer or cancel the run.

While the agent works, the Working row shows what it is writing: one short sentence before a group of tool calls, saying what it is about to do and why.
The running entry in a dashboard's activity pane shows the same text.
That text is live only: the server keeps the last 2,000 characters in memory while the run works and drops them when the run ends, so only the saved dashboard or finding stays.
Between two messages of the agent's live text there is a blank line.
Outside the browser, `observe status` prints it as a `Live text` section and `get_run` over MCP returns it as `live_text`; see [CLI](cli.md#follow-ups-and-waiting) and [MCP](mcp.md).

Only you, and read only an Admin, can watch your build.
Nothing appears in the dashboards list, a folder or the version history until the agent finishes.
Anyone else, and anyone who can no longer view the dashboard you are revising, gets "Only the person who asked can watch this build."
[Who reads a chat](#who-reads-a-chat) lists the other messages a reader can see in its place.

The build page shows the chat beside the build.
On a screen wider than 1200 px the chat is a pane on the left, which you can widen or narrow by dragging its edge or with the arrow keys on the handle.
On a smaller screen the chat sits below the build.
The pane holds the conversation with the status line, **Cancel run**, and, when the agent stops, **Save what was built** and **Continue**.
Its **Open the chat page** link opens the full chat page at `/c/<request id>`.
The dashboard is saved once, as one version.
The build page then opens it with the chat in the Chat tab of its side panel.

On the chat page, a card above the steps shows the same status line with thumbnails of the most recent rows and panels built so far, and an **Open build** link.
When the agent stops, the card keeps the thumbnails and offers **Save what was built** and **Continue**.
In the pane beside a build, the card leaves out the link and the thumbnails, because the build is already on screen.
On the dashboard page, the running entry shows the same status line and, for you, an **Open chat** link that opens the request's chat in the Chat tab of the side panel.
Every turn of the conversation keeps its card and buttons for as long as the build is kept, not only the latest ones.

### Queue position

While a request waits for the agent, its status line reads "Queued, position 3".
The position counts the requests ahead of it in the order the agent picks them up.
It can change if a request is skipped, because a dashboard runs one revision at a time and a request whose dashboard is busy waits for it.
The line is the same everywhere a queued request is shown: in the conversation, in the chat pane of its build page and in the dashboard's running entry.

### Cancel a run

While a request is queued or running, **Cancel run** stops it.
Only the person who asked sees it; an Editor or Admin cannot cancel someone else's request.
From the CLI, `observe cancel <request-id>` does the same, and over MCP it is `cancel_run`.

A queued request stops at once.
A running one shows "Canceling" and stops within a few seconds, once the call the agent is making returns.
If its worker is gone, the request ends canceled at once instead.
The request ends as failed with the code `canceled` and keeps what was built, so you can save or continue it.
If the agent had already saved the finished dashboard, the request counts as succeeded and the cancel changes nothing.

The chat shows a canceled run as a calm "You stopped this run" with what to do next, not as a failure.
The chat lists, History included, say "Stopped" with "Canceled before anything was built" or how far it got, instead of a red error.

Other people's follow-ups on the same dashboard wait behind a run until it ends.
A run that goes nowhere holds them back until its owner cancels it or one of its limits stops it.

### Save what was built

If the agent stops before it finishes, the build page keeps what was built for 7 days.
Choose **Save what was built** to keep it:

- for a new dashboard it becomes version 1 in the folder the request named, or in your My Dashboards;
- for a follow-up it becomes the next version of the dashboard when nobody has saved a version since the build started.

When someone has, the build cannot be saved and the page says so; open the dashboard and ask again.
You need Edit access to the target, and the request stays marked as failed.
Once the build is saved, the build page opens the dashboard.

The chat keeps the result card with "5 of 8 panels, saved before the run finished" when the plan is known.
The status line says "Saved part of the dashboard" instead of "Stopped".
**Add the missing panels** asks the agent for the rest as a follow-up on that dashboard.
The button shows only on the newest turn of the chat, so once you have asked for the rest, or anything else, the earlier card no longer offers it.

### Continue a stopped build

**Continue** on a stopped build starts a new run that picks up from the panels already built.

1. Choose **Continue**.
   A form opens with an optional note for the agent, such as "skip the network rows" or "use the prod datasource".
2. Write the note, or leave it blank to send none.
3. Choose **Continue build**.

The new turn appears below the old one, marked "Continues an earlier run" with your note under it, and the stopped turn says "Continued in a new run".
The agent sees the note in every start of that run, including a restart after a server shutdown.
A new dashboard continues on its own page, and a follow-up continues in the same conversation.
From the CLI, `observe continue <request-id>` does the same.

The note follows the prompt rule: 1 to 4000 characters, with no control or bidirectional-text characters.
The form shows "Use at most 4000 characters." when it is too long.

Only the person who asked can continue a build, and it needs the same access as starting a run.
A build can be continued once, and not after you saved it.
**Save what was built** and **Continue** exclude each other: whichever you choose first wins, and the other says it can no longer be done.
A follow-up whose dashboard got a newer version since the build started cannot be continued.
The page says "Someone saved a newer version after this build started, so it can't be continued. Open the dashboard and ask again."
A continued run counts toward the same limits as a new request.
When the agent lowers the number of panels it planned after you continue, the finished turn says so, for example "Planned 6 panels, built 3."

## Read a turn

While a run works, its turn lists the steps as a timeline, one line per phase in a row.
The phases are Discovering, Checking queries, Running queries, Saving and Repairing.
Each line names the tools the phase used, with a count when a tool ran more than once, such as "list_metrics 3, get_schema 1".
A phase with more than one step has a **Show N steps** button that lists each step's message.
Saving always lists its messages, because they name the panels, such as "Saved panel 3 of 8: p95 latency".
A phase that needed a fix is marked with a warning icon.
Under the timeline, "Running for 1m 12s" counts the time since the run started.

When the run ends, the timeline collapses into a line such as "Ran 14 steps in 1m 12s for about $0.12, repaired 1 query".
Open it to see every step again.
The cost is the server's estimate from its price table, and it shows only when the model is priced.
Only the server's step messages and errors are shown, and the answer the agent saved, which an investigation also writes into its observation.

Every request has an id, shown as "Request" and its first eight characters at the bottom of the conversation.
Quote it when you report a problem.

The page keeps polling the server, also while its browser tab is in the background, so a finished run leaves "Running" without a reload.
It shows "Reconnecting..." after two failed polls, and "Progress stopped updating." with a **Retry** button when polling stalls.
The run itself goes on server-side either way.

### When a run fails

A failed request shows "The dashboard could not be built" (or "The dashboard could not be updated" for a follow-up).
Below it you see what the failure means, the reason the server stored, and what to do next.
The advice to save or continue a build shows only while a build is left to save or continue.
Otherwise the last column reads "Try again." or "Try again with a narrower prompt.", and for `budget_exhausted` "Ask an admin about the limit, or try again."
A run you stopped shows "You stopped this run" instead.

| Code | What it means | What to do |
| --- | --- | --- |
| `run_timeout` | The run reached the server's time limit. | Try again with a narrower prompt, or continue the build. |
| `call_timeout` | A model call took too long to answer. | Try again. |
| `rate_limited` | The model provider is limiting requests from this server. | Wait a minute, then try again. |
| `provider_error` | The model provider answered with an error. | Try again, and ask an admin to check the provider if it keeps failing. |
| `output_limit` | The model's answer was cut off at its output limit. | Try again and ask for fewer panels. |
| `no_tool_call`, `no_progress` | The agent stopped making progress. | Try again and name the metric, service or table you want. |
| `validation_exhausted`, `check_failed` | The agent could not get its queries past the checks. | Try again and name the metric, service or table you want, or ask for those panels one at a time. |
| `refused` | The model declined the request. | Rephrase the prompt. |
| `conflict` | Someone saved a newer version while the agent worked. | Open the dashboard and ask again. |
| `canceled` | The run was canceled. | Save what was built, continue it, or try again. |
| `requester_forbidden`, `requester_gone` | You no longer have access to this dashboard. | Ask an admin for Edit access. |
| `crashed`, `lease_expired` | The server stopped while it ran this request. | Try again. |
| `no_finding` | The agent ended without writing a finding. | Ask a narrower question, with the service and the time range. |
| `budget_exhausted` | The run reached the server's cost limit. | Save what was built or continue it, or ask an admin about the limit. |
| `agent_unavailable` | No outside agent took this run within 10 minutes. | Check that the agent is connected and asking for work, then try again. |
| `agent_failed` | The outside agent said it could not finish the run. | Read its message, then try again. |

For `validation_exhausted` and `check_failed`, the reason names the panels that kept failing.
A code not in the table shows the server's reason alone.
The states a run moves through, and what each allows, are in [States](../reference/states.md).

### Try a failed run again

**Try again** starts a new request with the same prompt.
When the request already has a dashboard, an **Open dashboard** button is shown next to it.
The button shows only while the run's `allowed_actions` include `retry`, because the server decides whether this run may be tried again (`POST /observe/{id}/retry`).
The new run is yours, whoever started the one you retry.

You can try your own failed or stopped run again once.
The old turn then reads "Tried again in a new run" with a link to the new one, its `retried_by`.
A second retry is refused with 409 `invalid_transition`, and a **Continue** of the old build with 409 `not_continuable`, instead of starting a duplicate.
Someone else who retries your run starts a request of their own, which leaves your turn and your retry as they were.
When the person who asked retries, the new request stays in the same chat, so the chat lists keep one row for it.

Anyone who can see a finished request and may start that kind of request can run it again, over the API with `POST /observe/{id}/retry` or with `observe retry`.
The exception is a request they read `redacted`: its words stay the requester's, so it offers them no retry and the call answers 409 `invalid_transition`.
A retry points at the same panel, spot or block as the request it retries.
Someone else's retry of a private ask, and a retry of a thread question by someone who cannot comment in that thread, points at nothing.
A retried investigation writes into the same observation when the person retrying may comment on it, and otherwise into a new one; see [Observations](observations.md#investigations-write-into-an-observation).

A run that a server shutdown interrupts goes back to the queue and starts again, and one that a crash interrupts gets one more attempt.

## Find, rename, pin and delete your chats

Home's **Recent chats** lists the five chats you asked in most recently, and **All chats** opens the Chats page at `/chats`.
The Ask page's Recent chats shows the same rows.
A chat is one row however many turns it has: its title, how many turns it has, and the dashboard it is about, if any, with the time of its last message.

On the Chats page:

1. Open `/chats`.
   A table lists all your chats with the columns Chat, Dashboard, Turns and Last message.
   The chats you pinned come first, then the rest, each part with the chat asked in most recently first.
2. Search to find chats whose title or any of whose prompts contains the text, ignoring case.
   **Dashboard** keeps the chats about one dashboard.
3. Open a row's menu to **Rename**, **Pin** or **Delete** the chat.
   The menu shows when you point at the row or focus it, and always on a touch device.

Older chats load as you scroll, and the order holds as they load.
The list ends with how many chats it shows; while older chats are still to load it says "so far".
A chat about no dashboard, or about one that was deleted, reads "No dashboard".
One about a dashboard you cannot see reads "A dashboard you cannot see".
That includes one in another person's My Dashboards in the Everyone list.

Admins also get **Mine** and **Everyone**.
Everyone lists every person's chats with who started each one, by their last message only.
It opens someone else's chat read only, with no composer and no actions, and its rows have no menu.

The actions apply to the whole chat:

- **Rename** changes the chat's title.
  It turns the row into a title box, on the Chats page and in the History list: Enter saves, Escape cancels, and an empty title goes back to the short title taken from the prompt.
  A title is at most 120 characters.
- **Pin** keeps the chat ahead of the rest, and **Unpin** puts it back.
- **Delete** archives the chat with every turn in it, and keeps the rows, marked archived.
  The dialog asks "Delete this chat?" and says how many messages go.
  After **Delete chat**, the chat leaves your chat lists for good.
  The chat page still opens from its link.

The Delete dialog says what stays, depending on what the chat left:

- "The dashboard it made stays."
- "The answer stays in the observation "`<title>`", which is not deleted."
- "The answer stays on the chat page, which still opens from its link." for an answer not saved as an observation.
- "The panel suggestion stays." or "The proposal stays."
- "Nothing else is removed."

A conversation of several messages says how many messages go and names what they saved, such as `What they saved stays: the dashboard "Checkout health" and the observation "Slowest routes".`.
It says "Nothing else is removed." when they saved nothing.

The Delete dialog also shows an unchecked **Also delete the observation "`<title>`"** for each observation the chat saved that you may delete.
That needs Manage on its folder, or Edit if you started the observation.
Checked, the dialog says the observation is deleted for everyone, with its blocks, discussion and published RCA revisions, and **Delete chat and observation** deletes it first and then deletes the chat.
The two differ: the chat only leaves your lists and still opens from its link, while the observation is gone for good and cannot be undone.
If the observation cannot be deleted, the chat is kept and the dialog says why.

Only the person who started a chat can rename, pin or delete it, or ask in it.
An Admin who can read the chat gets 403 with the code `chat_read_only` from those actions and from a request naming its `chat_id`.
Anyone who cannot read it gets 404.

The **History** button beside a chat lists older chats and Grafana imports.
Its search box finds chats whose title, prompt or answer contains the text, ignoring case.
Chats you pinned come first under "Pinned", most recently pinned first.
The rest are grouped by the dashboard they are about, with "No dashboard" for chats that never got one and "A dashboard you cannot see" for one you lost access to.
History says for each chat its state (Running, Stopped or Done) and then what it left behind:

- "Saved `<dashboard>`" or "Revised `<dashboard>`";
- "Saved part of `<dashboard>`" for a build that stopped and was saved in part;
- "Answered, no dashboard" when you asked for a dashboard and the agent only answered;
- "Answered, dashboard unchanged" for a question about a dashboard;
- "Suggested a panel" or "Opened a proposal" for a run that ended on a suggested panel or a proposal;
- "Answered in `<title>`", with the observation it was saved to, for an investigation, as long as you can open that observation's folder;
- "Saved an observation" for the same case when you cannot see its title;
- "Answered, no observation" for an investigation whose answer stayed in the chat;
- "Nothing saved".

## Use the chat from the CLI, MCP or HTTP

Send a prompt from a terminal with `observe`:

```sh
observe "checkout p99 latency, last 6h"
```

It prints a line for each panel the build adds, as `Panel 3 of 8: Checkout p99 (timeseries)`, and ends with `Saved version 1  https://observe.example.com/d/<dashboard id>/<slug>`.
When the output is not a terminal, `Saved version 1` goes to the error stream and only the address to the output.
With `--quiet`, only the address is printed.
[CLI](cli.md#send-a-prompt) has the flags, such as `--dashboard`, `--revise`, `--chat` and `--new-chat`.

Over MCP, `start_run` asks the agent and returns the run at once, and `get_run` follows it.
Over HTTP, `POST /api/v1/observe` answers 202 with the request, and `GET /api/v1/observe/{id}` shows its state:

```sh
curl -fsS -X POST "$OBSERVE_SERVER/api/v1/observe" \
  -H "Authorization: Bearer $OBSERVE_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"prompt": "checkout p99 latency, last 6h"}'
```

The CLI and MCP also manage chats and requests:

| Task | Web app | CLI | MCP |
| --- | --- | --- | --- |
| List chats | Chats page | `observe chats list` | `list_chats` |
| Read a chat | Chat page | `observe chats show <chat-id>` | `get_chat` |
| Pin or unpin | Chat menu | `observe chats pin`, `observe chats unpin` | `pin_run`, `unpin_run` |
| Rename | Chat menu | `observe requests rename` | `rename_run` |
| Delete (archive) | Chat menu | `observe chats archive`, `observe requests archive` | `archive_run` |
| Create observation | **Create observation** | `observe save-answer <request-id>` | `save_run_answer` |
| Cancel | **Cancel run** | `observe cancel` | `cancel_run` |
| Retry | **Try again** | `observe retry` | `retry_run` |

The MCP tools and `observe requests rename` act on one request.
Pass the id of the chat's first request to act on the whole chat.
`observe chats pin`, `unpin` and `archive` take the chat id.
To list single requests rather than chats, use `observe requests list` or the MCP tool `list_runs`.
See [CLI](cli.md#your-requests) and [MCP](mcp.md).

## Reference

### Start a chat in detail

Type into the box and press Enter.
Its visible hint is "Ask a question or describe a dashboard, like: checkout health by pod, last 6h", and its accessible name is "Message the agent".
Shift+Enter adds a line, and on a touch device Enter adds a line and the arrow button sends.
Inside the box a line says so ("Enter sends. Shift+Enter adds a new line. Up arrow recalls an earlier prompt. Type a slash for commands."); on a touch device it is not shown.
Pressing `/` outside a text field moves the cursor to the box.

#### Prompt rules

- A prompt is 1 to 4000 characters, at least one of them visible, and cannot contain control or bidirectional-text characters; see [Text rules](../reference/text-rules.md).
- The box shows "Ask a question or describe a dashboard first." when it is empty and "Use at most 4000 characters." when it is too long.
- From 3600 characters a counter such as "3650 of 4000" appears in the box's footer, and turns red with "too long" past 4000.
- A long message of yours, over 600 characters or 6 lines, is shown cut to 6 lines in the conversation with a **Show the whole message** button.
- The up arrow, with the cursor at the start of the box, recalls your earlier prompts, newest first, and the down arrow steps back toward what you were typing.
  The last 20 prompts are kept in this browser only, never on the server.

#### Links that fill the box

An address of `/ask?prompt=<text>` opens the box with that text already in it, up to 4000 characters, and sends nothing until you do.
`/?prompt=<text>` redirects there.
Links inside the app that ask the agent about something use it.

Adding `dashboard=<id>` (a dashboard id) to a link that has `prompt` puts that dashboard in the box as a chip, so the send asks about it.
The agent answers without changing the dashboard, and when a change fits, the answer offers it with **Revise the dashboard**.
The chip appears for a dashboard you can comment on, because asking about one needs Comment.
The box stays closed for a moment while the dashboard is looked up, so a prompt is never sent before the chip is there.
For a dashboard you cannot comment on, the box says "You cannot ask about `<title>`, because you cannot comment on it; this message asks without it."

#### Slash commands

A slash command at the start of the box changes what the send does.
Type the command and a space, and it turns into a chip above the box.

| Command | Chip | What the send does |
| --- | --- | --- |
| `/investigate` | Investigate | Asks a question and answers in an observation, see [Investigate](investigate.md). |
| `/import` | Import | Imports a Grafana dashboard from a link or its JSON. |

- While you type a command, a line under the box lists the matching commands and says "Type a space after the command to use it."
- Remove the chip with its button, or press Backspace in the empty box.
- In a chat that already has a dashboard, a send without a command asks about that dashboard.
- `/import` works only in the chat, and no command works in a dashboard's activity pane.
- `/investigate` is offered to Editors and Admins only, because starting an investigation needs the Editor role.
- In a chat about a dashboard, a second chip names that dashboard.

#### Footer, chips and examples

- The footer shows the model and, when the server sets one, its effort, for example "claude-haiku-5-5, medium effort".
  The model comes from the server's configuration and cannot be picked in the chat box.
- While one of your runs in the chat is queued or running, a square stop button, labelled "Stop the run", sits next to the send button and cancels it; see [Cancel a run](#cancel-a-run).
- The chips under the box list the healthy datasources.
  The agent can only query datasources the server has configured; see [Prometheus](../connect/prometheus.md), [ClickHouse](../connect/clickhouse.md), [Loki](../connect/loki.md), [Tempo](../connect/tempo.md) and [Jaeger](../connect/jaeger.md).
- Without a model the box is replaced by "No model is connected to this server."
  On the Ask page the note holds the **Import from Grafana** link, and Home's **Ask Observe Agent** tab does not show it.
- Under the example prompts, the Ask page at `/ask` lists your saved prompts and those shared with everyone.
  Typing `/` at the start of an otherwise one-line box opens a picker for them; see [Saved prompts](../teams/saved-prompts.md).

The example prompts under the box are built from your datasources and dashboards, at most four of them.
Editors and Admins are offered dashboard prompts, and everyone is offered up to two questions, such as "Which services had the most errors in the last hour?" or "Which routes are slowest right now?".
With Prometheus and a dashboard you can edit, the first dashboard prompt adds a latency row to one you changed, or else to the first one you can edit.
The others start a new dashboard, such as "Service health by pod, last 6h" with Prometheus or "Error logs by service, last hour" with Loki or ClickHouse.
Choosing a new-dashboard example sends it at once.
Choosing the latency-row example sends nothing: it opens the box with the text and the dashboard's name as a chip, so you can check or remove the target, edit the text and press Enter.
A dashboard you can only view is never offered.

#### Limits on requests

Each person may have 3 chat requests queued or running that are not about a dashboard or an observation, and the instance 20.
These are a new dashboard, a question, and a follow-up in a chat with no dashboard.
Revisions and questions about a dashboard do not count.
Past that the server answers `429` with `too_many_requests` or `agent_busy`; wait for a request to finish.
For `agent_busy` the box shows "The agent is busy with other requests; try again in a moment."
An investigation has its own cap of 3 per person and 10 on the instance, with the same codes.
Requests that change one dashboard run one at a time, and up to 5 can be queued or running.
A sixth is refused with `429 too_many_revisions`.

### What the agent does

The agent is a loop of model calls and tool calls run by the server.
It does not see your browser, and it can only act through its tools.
Its instructions hold the Spec guide, the schema and an example.
They tell it to use only the datasources it is given and only the metric, label, table and column names it discovered, so it does not invent names.
If the prompt itself contains a Grafana dashboard JSON, the agent imports it with the importer instead of rewriting it, which ends the run like a save.
This works for a new dashboard only, and the JSON has to fit in the 4000 characters of the prompt.

Its order of work is:

1. Pick a datasource, then discover what exists: list datasources, metrics, labels and label values on Prometheus, and tables, schemas and sample rows on ClickHouse.
   For Loki, Tempo, Jaeger, Elasticsearch and OpenSearch the discovery tools list label names and values, services and span names, tables and fields.
2. Draft the dashboard as an ObserveSpec document.
3. Lint every new or changed query against its datasource.
4. Optionally run a query to see its data.
5. Save.

The conversation shows these as phases, each with a message from the server:

| Phase | Examples of what you see |
| --- | --- |
| Discovering | "Listed 14 metrics on prom", "Read the otel_logs schema on ch" |
| Checking queries | "Checked a query on prom: 0 errors" |
| Running queries | "Ran a query on prom: ..." |
| Saving | "Saved version 1" |
| Repairing | "Save rejected: 2 issues (...)" |

#### Saves and repairs

A save is the real check.
A full save runs every panel query, and an `update_panels` save runs only the panels and variables it changed, unless it changes the time range, which runs every query.
Either rejects a panel whose query is invalid or returns no data, except for alert list panels and panels over the built-in audit datasource, which can be quiet.
The rejection comes back to the agent with the problem for each panel, and the agent fixes the draft and saves again.
The agent may repair after a rejection, but the third rejected save in a row stops the run and the request fails.
A save that finishes the dashboard ends the run.

The server writes the version summary from the finished build, not from the agent's last save:

- A new dashboard reads `Built the dashboard with 6 panels: "A", "B", "C", "D" and 2 more.`
- A revision lists what changed since the version it started from, such as `Added "p99 latency"; renamed "Orders" to "Orders (agent)"; changed the description.`
- When the build ends up identical to an earlier version, such as after "undo that", the version is a restore.
  Its summary reads `Restored version 3 (removed "p99 latency").` and the history marks it as restored from that version.
- A build that changed nothing keeps the sentence the agent wrote.

#### Limits on a run

Queries the agent runs while drafting are capped at 15 seconds, 200 series, 100,000 points and 1,000 rows, so a heavy query may fail where the saved panel would work.

A run has no cap on model calls and no time limit by default.
It stops after three rejected saves in a row, when it stops making progress, or when the person who asked cancels it.
Reading past the limit of model calls without a save, or repeating a save, are examples of no progress.
An administrator can set a time limit and a cost limit (`SLASHOBSERVE_AGENT_RUN_MAX_COST_USD`), and a run stops when it reaches either.
When a run stops early, the conversation ends with "The dashboard could not be built" and a **Try again** button; when it kept a build, you can also continue it.

When the agent answers a question after its reads were refused at its limit of model calls, the answer ends with a line saying reading stopped early and the counts may be partial.
The limit is twelve calls for a message in a chat with no dashboard or observation until it saves its first panel, for an investigation and for a question about a dashboard or an observation, unless `SLASHOBSERVE_AGENT_INVESTIGATION_CALLS` (6 to 60) sets another number for those.
It is six calls for a revision and for each panel once a chat message has started building a dashboard.

When a read timed out, its datasource did not answer or the server was too busy to ask it, and no later read of the same kind on that datasource succeeded, the answer ends with a line naming each datasource it could not check and why.
A timeout is never read as no data.

An answer reads deploy markers as you before it says there was no deploy, so a question such as when checkout last deployed is answered from them.
Times in an answer are in your browser's time zone with its label, such as 19:52 IST, because the chat sends that zone with each request.

### Follow-ups

A follow-up is a continuation of the chat.
The agent starts a fresh run that sees the datasources, your new message and, as context, the earlier turns of the conversation.
Each earlier turn is the question with the answer it got, or "Saved version N of the dashboard: ..." for a turn that saved a version.
The agent gets the last 6 earlier turns of the chat that succeeded, each prompt cut to 600 characters and each answer to 1200, and at most 8000 characters in all, dropping the oldest first.
For a chat about a dashboard these are the earlier turns of that dashboard chat.
For a chat with no dashboard they are the earlier turns of that chat.

Turns of one chat run one at a time in the order they were sent.
The earlier turns are read when a turn starts running, so a follow-up sent while the last one still runs sees its answer.

#### Chat fields in the API

- `POST /observe` takes `chat_id` to ask the next turn of one of your chats.
- `kind` `revise` with `dashboard_id` changes that dashboard; an `ask`, the default, only answers about it.
- Every request carries its `chat_id`, the id of the chat's first request.
- Without `chat_id` a new chat starts, except a `revise` on a dashboard, which joins your latest chat on that dashboard unless `new_chat` is true.
  `new_chat` matters only for a `revise` on a dashboard, because a private ask already starts its own chat.
- `client_id`, a UUID the client picks once per message, makes a resend safe.
  The same `client_id` answers with the request it started and `replayed: true`.
- A request an ask in a thread started belongs to no chat: its `chat_id` is null and `GET /chats/{id}` has nothing to open for it.
  A further ask in the same thread is another request in that thread, which sees the thread's earlier messages.
- `GET /chats` lists your chats, `GET /chats/{id}` reads one with every turn, and `DELETE /chats/{id}` archives it with its turns.
- A chat is pinned with `PATCH /observe/{id}` on its first request, which MCP `pin_run` and `unpin_run` call.

### In the dashboard's Chat tab

A dashboard's side panel has a **Chat** tab next to **Activity**, which shows your own chats about that dashboard.
It opens your latest chat about the dashboard and names it in the address as `tab=chat&chat=<chat id>`, so the chat stays open when a turn in another chat makes that one the latest.

- The select at its top lists up to 50 of your chats on this dashboard, newest first, each with the time of its last message and its number of turns.
- **All your chats** opens the Chats page filtered to this dashboard.
- **New chat** clears the conversation and shows an empty box, with `chat=new` in the address, so the browser's Back returns to the chat you left.
  The chat your first message lands in then takes its place in the address.
- The first message after **New chat** is a question about the dashboard and starts a new chat, which the tab then shows.
  The note under the box says "Only you and Admins can see this chat."
- A chat that is about another dashboard, or about none, opens read only, with a link to continue it on its chat page.

While the chat has one turn, yours and finished, above the box the tab offers up to two suggested follow-ups.
They are the questions Home suggests for your datasources, such as "Which routes are slowest right now?".
Choosing one asks it about the dashboard in the same chat, without leaving the dashboard, and the suggestions go once the chat has a second turn.
They show when you can ask the agent and are not shown on the chat page.

### From the dashboard's activity pane

You can also send a follow-up from the dashboard's activity pane.

1. Write the change in "Comment or request".
2. Either mention `@observe`, or choose the "To: everyone here" chip above the box so it reads "To: Observe Agent".
3. Send.

To ask inside a comment thread, choose **Reply** on it.
The same box shows "Replying in" and the panel or dashboard, and the send goes to that thread.
The up arrow recalls your earlier prompts there too, and the footer shows the model.
These requests carry on in the activity pane; see [Comments, agent requests and versions](review.md).
In the conversation, a turn sent from the dashboard shows "Sent from the dashboard".

While a request runs, its entry in the activity pane shows the latest three phases of its timeline, with **Show all steps** for the rest, how long it has been running, and **Cancel run** for the person who asked.
While your own request runs, the same stop button sits next to the send button.
When the request saves a version, its entry reads "Saved as version N" with a link to the change between that version and the one before, and says how long the run took.
The dashboard reloads the new version, and panels the version added fade in once.

### Who reads a chat

Most requests you send are turns of a chat that only you and Admins can read; the exceptions are the shared requests listed below.
These are private, so only you and Admins read them, and for anyone else they do not exist:

- a request with neither a dashboard nor an observation;
- a question about a dashboard or an observation asked in a chat, in the dashboard's Chat tab or with [Ask mode](ask-and-comment.md#ask-mode).

For a chat on a dashboard, you read the answer only while you can still open that dashboard.

These are shared work that anyone who can see the dashboard reads:

- a request in a dashboard's comment thread;
- an investigation on a dashboard;
- a request that revises the dashboard.

An investigation or an ask in the thread of an observation is read by anyone who can read the observation.
Readers of shared work read that one request only.
Its `chat_id` is null for them, the first request of a chat shows them its own title rather than the chat's name, and the rest of the chat stays yours.
That includes the earlier questions the agent was given.

An Admin reads another person's chat read only.
For anyone who is not an Admin, a chat that does not exist, or that someone else asked, shows one message on `/c/<request id>` and on `/b/<request id>`: "You can't open this chat. It does not exist, or only the person who asked and workspace admins can open it."

An Admin who opens someone else's chat whose answer sits in an observation in a folder the Admin cannot see, such as that person's My Dashboards, reads "The answer is saved in an observation in a folder you cannot see."
An answer that was never saved as an observation is hidden the same way when the chat is about a dashboard you cannot open.
The chat then reads "The answer is about a dashboard you cannot open."

#### Redacted requests

Unless it is an investigation, a question asked in a thread, which belongs to no chat, or an answer you shared into a thread they can read, a reader of shared work also reads it without your words.
Its `title`, `prompt`, step messages, `live_text`, `continue_note` and `error` are empty and `redacted` is true.
Its state, times, your name, its kind, its `failure_code` and the version it saved stay.
The dashboard's revision list shows its `prompt`, `continue_note` and `error` empty the same way.

In the web app such a run reads "Asked in a private chat" where its message would be, in its page and browser tab title and in the dashboard's activity.
Its steps show without their messages, and while it runs the dashboard's activity names its current phase instead of its latest step.
Its page is read only: it offers nothing to copy, retry or ask again, and has no message box.

#### Audit events for reads

Every read of a chat or a request, yours included, is recorded in the audit trail, in the web app, over MCP and with the CLI.
It is recorded as `chat.view` or `run.view` with the owner's id as `owner_id`.
A list of chats or requests is recorded as one `chat.list` or `run.list` with `scope` (`mine` or `all`, and `dashboard` for the list of a dashboard's requests) in its data, never one event per row.
Each is recorded once per reader and chat, request or list in 10 minutes, so a page that polls adds no events.
A refused read is recorded every time.

### Answers that read Admin-only data

A run reads a datasource only Admins can query, such as the [audit trail](../reference/audit-queries.md), only when everyone who reads what the run writes is an Admin.
That is an Admin's private chat, or a thread, dashboard, observation or build in a folder only Admins can read.
Any other run, an Admin's included, is not offered those datasources, and a query on one answers `admins_only_datasource` with where the data can be read.

The same holds for what is stored in a folder only Admins can read.
A run that writes where others read sees no such folder, so its dashboards, panels, annotations and observations answer `not_found`, and they are left out of its lists.
A run that may read them is marked when a tool returns one: reading a dashboard, its panels or its annotations, or reading an observation.

Reading the dashboard or observation the run is about is not counted, for an ask and an investigation as for a revision or a suggestion.
What the run learns there goes wherever that dashboard or observation goes.
Its panel data still counts when it comes from a datasource only Admins can query.
The observation an ask's answer was saved into is not what that ask is about, so a later attempt of the ask that reads it there is counted.

Lists of dashboards, folders and observations show titles only.
In an Admin's run they leave out what is in folders only Admins can read, with this exception:

- the built-in Audit folder and its starter dashboards, what the run is about, and what is in the folders it writes to.

The Audit exception applies to the dashboards and folders lists only, because the observations list has no built-in Audit exception.
The lists say how many they left out and mark nothing.
`include_restricted: true` lists those too and marks the answer.

A run that writes where others read lists none of them and refuses `include_restricted`.

#### What is marked restricted

An answer whose run read such a datasource or such content is marked `restricted`.
So is a run that starts from one, because it carries that answer as context:

- a later turn of the same chat;
- an ask in a thread that holds one;
- an apply of one;
- a continuation of a restricted run;
- a run about a dashboard or observation that holds one.

#### Where a restricted answer can go

A restricted answer stays where only Admins read it.
These are each refused with `409 restricted_answer` unless the place is in a folder only Admins can read, and the message says why:

- sharing it in a thread or onto a page;
- saving it as an observation;
- pinning it, or making it a dashboard;
- applying its suggested change;
- saving its build, and proposing its build as a change.

The same refusal reaches MCP tools and the CLI.

#### Answers about Admin-only content

An answer about a dashboard or observation that sits in a folder only Admins can read is kept the same way when the answer is placed, even when its run read nothing only Admins can query.
It may go into that dashboard's or observation's own threads and page, or into a folder only Admins can read.
It is refused with `409 restricted_answer` anywhere else, and saving it as an observation files it in that folder.
The observation an ask's answer was saved into is not what the ask is about, so its folder keeps nothing.
The observation an investigation opened for itself does keep its answer.

A run that starts from such an answer in any of those ways is marked `restricted` too, unless it is about that same dashboard or observation, for example a later turn of the chat on a dashboard in General.
What it was placed in then holds it as it would hold a restricted answer.
That content moves, is copied and has its folder opened only as [Folders](../teams/folders.md) describes, except the dashboard or observation the answer is about, which moves freely.
Once the dashboard or observation has moved to a folder others read, nothing is refused.

Deleting the dashboard or observation while it is in a folder only Admins can read, or deleting a run whose answer is kept this way, does not free what was placed from the answer.
Each dashboard and observation it was placed in keeps holding it.
A run about a dashboard that files into an observation, both in folders only Admins can read, makes each of them hold the other's answer, so neither moves out on its own.
To free them, delete that run's reply, block or pin from one of them, then move that one out first and the other after it.

#### Who can read a restricted run

A run asked in a thread that ends with a `restricted` answer, when people who are not Admins read that thread, replies there with a fixed text that says the answer is not shown.
Only whoever asked and Admins read it, in the run.

A `restricted` run, and a run whose answer is kept for what it is about, answers `404 not_found` to anyone who is neither its requester nor an Admin.
That holds wherever a run is returned, and the revision history leaves it out for them, until nothing keeps it any more.
Its finding says `restricted` for as long.

Saving a restricted answer as an observation files it in the first folder only Admins can read among these:

1. the folder of the dashboard it was about;
2. the folder of the observation its run files into;
3. the folder of the observation it was asked about.

Else it files in the built-in Audit folder (see [Audit folder](activity.md#audit-folder)).
When the Audit folder is not Admins only either, the save is refused.
Withdrawing a share works as for any other answer, and who can read a chat does not change.

### One title for a chat and its observation

A chat with no dashboard and exactly one observation made from it shares its title with that observation.
The observation is made by **Create observation** or by an investigation that opened it.
Renaming the chat renames the observation, and renaming the observation renames the chat.
This holds in the web app, the API, the CLI and MCP alike, and both lists show the new title at once.

- When you may not edit the observation, the chat is still renamed and a note says "The observation made from this chat keeps its title, because you cannot edit it."
  An observation you cannot open is left alone and never named.
- When someone other than the person who started the chat renames the observation, the chat keeps its title and the note says so.
- Titles stay apart for a chat with two or more observations, a chat on a dashboard, a later turn's own title, and an observation not made from a chat.

Over MCP, `chat_rename` in the result of `update_observation` is `renamed`, or `kept` when the chat's title was left as it is.
`rename_run` on that chat's first run renames the observation and answers `observation_rename` the same way.

### What the agent cannot do

- **Use a datasource that is not configured or not healthy**, or names it did not discover.
- **Change a dashboard you cannot edit.** It cannot reach folders you have no Edit access to.
- **Change an existing dashboard from Home.** A request on Home builds a new dashboard; [Change an existing dashboard from Home](#change-an-existing-dashboard-from-home) says where to ask instead.
- **Add panels with no data.** The save refuses them, and the agent does not create placeholders.
- **Make the same choices every time.** Results depend on the model; a clearer prompt with the metric, service or time range you want gives better dashboards.

To work on a dashboard in your own editor and model, use [MCP](mcp.md) or the [CLI](cli.md).
To change one panel by hand, see [Edit a panel](edit-panel.md).
