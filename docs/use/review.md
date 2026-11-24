---
title: Comments, agent requests and versions
description: Review a dashboard with comments, send changes to the agent, and compare or restore saved versions.
---

Every dashboard keeps its comments, the requests people sent to the agent and its saved versions in one place, the **Activity** tab of its side panel.
Use it to discuss a panel with your team, to ask the agent a question, and to compare or restore an earlier version.

Versions are immutable and linear.
Each save adds the next version number, and nothing edits or deletes an earlier one.

## Comment on a panel and ask the agent

1. Choose **Activity** at the top right of a dashboard.
   The side panel opens, and the button shows the number of open comments.
2. At the bottom of the card, pick a panel in **On**, or leave "The whole dashboard".
3. Type in the box labelled "Comment or request" and press Enter.
   You see your comment at the bottom of the timeline.
4. To ask the agent instead, mention it, for example `@observe why did the error rate rise at 09:12?`.
   The send button now reads "Send and ask Observe Agent".
5. Wait for the answer.
   It arrives as a reply in the same thread, and the line under your message says "Observe Agent is working on it" while it runs.
6. Choose **Resolve** on the thread when the question is settled.

A comment that mentions no agent goes to the team and starts nothing.
The agent changes nothing when you ask a question; it reads the dashboard and its data and answers in the thread.
[Ask the agent from a comment](#ask-the-agent-from-a-comment) has the details, including how to turn an answer into a change.

From a terminal or an agent, `observe comments` and the comment tools of [MCP](mcp.md) do the same work.

## Open the Activity tab

Choose **Activity** at the top right of a dashboard to open the side panel.
The side panel has two tabs, **Chat** and **Activity**, and the address keeps the open one as `tab=chat` or `tab=activity`.
The Chat tab is described in [Chat with the agent](chat.md#in-the-dashboards-chat-tab).

## Delete a dashboard

Deleting the dashboard deletes its versions, comments, agent requests and import record with it, for everyone, and cannot be undone.
**Delete dashboard...** is in the dashboard's **More** menu and in the row menu on the Dashboards page.
It asks for confirmation and needs Edit on the folder.
`observe dashboards delete` does the same from the [CLI](cli.md).
Anyone with the dashboard open sees a toast saying that it was deleted.

## The activity timeline

The Activity tab holds comments, agent requests, proposals, annotations and saved versions.
Entries are sorted by time and grouped under a heading for each day ("Today", "Yesterday" or a date).
Each shows its author's mark and the time of day, such as 14:32, or "Just now" for the first minute.
Hover the time for the full date.

Two menus filter the list.
**Show** switches between Everything, Open comments and Changes only, and **Panel** narrows it to one panel.
An empty list says "No activity yet." and suggests commenting on a panel from its menu or writing below.
Filters that match nothing say "No activity matches these filters."

The entries come in six kinds.

### Comment threads

A thread shows the author's name with no verb and, when the comment is about a panel, "on `<panel>`".
The name is the same label versions use.
A comment written through an MCP client reads "Ishan (via Claude Code)", and a thread the agent resolved after a request reads "Resolved by Observe Agent for Ishan".

- Choosing the panel name scrolls to the panel and highlights it.
- A comment on a panel that no longer exists reads "on a removed panel".
- Replies sit under the first message, each with its author's name and no verb.
  An agent's reply speaks as the agent, such as "Observe Agent for Ishan".
- A message that arrives while the card is open slides in, and the ones already there stay put.

### Agent requests

An agent request sent from a comment shows inside its thread, as one status line per run under the message that asked.

- "Observe Agent is working on it · View the run" while it works.
- "`<name>` was asked" for a connected agent.
- The agent's reply once it ends.

**View the run** opens the run's conversation at `/c/<id>`, and the built-in agent's reply carries the same link.

A request that no loaded thread holds shows "`<name>` asked the agent" and one of these states:

- "Sent to the agent".
- "Resolved by version N".
- "Proposal opened, waiting for an Editor".
- "Finished, nothing changed".
- "The agent could not finish: `<reason>`".

A request from someone else's private chat reads "Asked in a private chat" in place of its message.
A run by someone who cannot edit ends in a [proposal](../concepts/proposals.md), and its status links to the proposal and follows it: "Proposal merged as version N" or "Proposal discarded".

While the agent works, the entry reads "Agent is working on it" with its latest step and the step number.
It reads "Queued, position N" while it is queued, and "Canceling" after its requester cancels it.
**Details** opens the full conversation at `/c/<id>` in a new tab.

### Versions

A version shows who saved as their label and "saved", such as "Ishan saved", "Ishan (via Claude Code) saved" or "Observe Agent for Ishan saved".
Then it shows "Version N" with its summary, or "No summary".
**See changes** opens the comparison with the previous version, on the dashboard's version history page at `/d/<dashboard id>/versions`, headed "Versions of `<title>`".
A version saved by merging a proposal adds "Proposed by `<name>`, merged by `<name>`".

The card loads the 50 newest versions first.
When the dashboard has more, **Load older versions** at the top of the list adds the next 50 and moves focus to the oldest version it added.
The button shows while the filters keep versions in the list, so it is hidden under **Open comments** and when a panel is chosen.

### Imports

An import shows "`<label>` imported this dashboard from Grafana", where the label is the importer's, such as "Ishan" or "Ishan (via Claude Code)".
It shows the conversion counts.
It lists the panels that are still unverified in the latest version with the reason each failed, and an **Ask the agent to fix** button where you may send requests.
See [Import Grafana dashboards](import-grafana.md).

### Proposals

A proposal shows "`<name>` proposed a change" for each open proposal, with its title and a **Review** link to the proposal page.
A merged one shows on its version instead.

### Annotations

An annotation shows "`<name>` added an annotation", with its text and tags, and "on `<panel>`" when it belongs to a panel.
The card lists the 50 newest annotations of the dashboard.

## Comment on a panel or the dashboard

At the bottom of the card, the form "Add to the activity" has an **On** menu with "The whole dashboard" and every panel title, and a box labelled "Comment or request".
Write the comment and press Enter, or choose **Send** (the up arrow).
Shift+Enter adds a line, and Cmd+Enter (Ctrl+Enter elsewhere) also sends.
On a touch device Enter adds a line and **Send** sends.
Every comment box says how to send under it: "Enter to send, Shift+Enter for a new line.", or "Tap the arrow to send." on a touch screen.
The same keys work in an observation's discussion, in the inbox, in replies and in the edit box.

Above the box, a "To:" chip says who reads the message and a "Visible to:" chip says who sees it.
A comment that mentions no agent reads "To: everyone here", goes to the team and starts nothing.
Choose the chip to send it to the agent instead, and choose it again to go back.
A mention of an agent turns the chip to that agent's name.

A comment is 1 to 4000 characters, counted without the spaces around it.
From 3600 a counter such as "3650 of 4000" appears under the box.
Past 4000 it turns red with "too long", and sending then says "Use at most 4000 characters."

A comment is attached to the latest version when you write it.
A comment that mentions no agent never asks an agent to do anything.

### Start from a panel

To start from a panel, do any of these:

- Hover the panel and choose **Comment** in its header.
- Press `c` while the pointer is over it.
- Open its menu ("Panel menu: `<title>`") and choose **Comment**.
- Choose **Comment** in the toolbar of the selected panel.

On the dashboard each opens a small composer on the panel, and the comment is saved as a thread on the whole panel.
In the full-size view and TV mode the card opens scoped to that panel with the cursor in the box instead.

In the full-size view and TV mode, press `c` outside a field to open the pane and put the cursor in this box.
Elsewhere on the dashboard `c` turns on Comment mode, described in [Ask and comment on a dashboard](ask-and-comment.md#comment-mode).

**Ask**, in the header, on `a` or in the same menu, is available on a data panel when you can comment and a model is connected.
It opens the [ask box](ask-and-comment.md#ask-mode) on the whole panel, a private question in your chat rather than a thread.
In the full-size view and TV mode it opens the dashboard's Chat tab with the panel as the question's target.
A panel with open comments shows a badge, "N open comments on `<title>`".

### Who can comment

Commenting needs the Comment level on the dashboard's folder: the Commenter role or higher, or a Comment entry or higher on the folder.
Without it you see "You can view dashboards in `<folder>`. To comment, ask the people who manage this folder for Comment access."
**Request comment access** sends them that request, with an optional note, through their inbox; see [Ask for comment access](../teams/folders.md#ask-for-comment-access).

With Comment but not Edit you see "You can view and comment in `<folder>`. What the agent changes for you reaches the dashboard as a proposal that someone with Edit on this folder merges."
See [Roles](../teams/roles.md) and [Folders](../teams/folders.md).

## Edit a comment and mention people

Each message you may change has a menu, **Comment actions** (**Thread actions** on the first message of a thread).
It shows when you hover or focus the message, and always shows on a touch screen.
Your own comments and replies have **Edit** in it while you can still comment on the dashboard.

**Edit** opens the text in place.
**Save**, Enter or Cmd+Enter (Ctrl+Enter elsewhere) keeps the change, Shift+Enter adds a new line, and **Cancel** or Escape drops it.
An edited comment shows "edited" beside its time.
Saving the text unchanged does nothing.

An agent's reply, such as "Saved version 3 of this dashboard.", has no **Edit** for anyone, including the person who asked for it.
The server refuses an edit of one with 403 `not_author`.
Editing never reopens a resolved thread and never changes who wrote the comment.

### Mention a person or an agent

The @ button next to a comment box, or typing `@` at the start of the box or after a space, opens a list.
It holds the built-in agent (Observe Agent, `@observe`), your own connected agents and the people who can see what the comment is about.
It works in the Activity box, in thread replies, in an observation's discussion and in the edit box.

A button press opens a search field for a name or an email.
When you type `@`, the letters you type stay in the box and narrow the list.
The arrow keys move through it, Enter picks the highlighted entry in place of what you typed, and Escape closes it.
The first entry is highlighted once you have typed a letter, so on a bare `@` the list does not take Enter.
The list closes when nothing matches.

A picked person shows in the box as `@Name`, and the comment is stored and shown with the mention.
Someone who cannot see the dashboard, observation or proposal is not offered.
Mentioning one is refused with 422 `invalid_mention`, as is a person who does not exist.

The list of people comes from the server (`GET /api/v1/threads/mentionable`), at most 20 at a time.
The form at the bottom of the card offers the people who can see the dashboard as well as the agents.
An agent over MCP or the CLI lists them with `list_mentionable_people` or `observe comments people`.

A person is stored as `@[Name](user:<id>)` and a connected agent as `@[Name](grant:<id>)`.
Both show as "@Name" once the comment is saved, using the name the server has.
A comment can contain at most 20 mentions, and a repeated mention counts each time.

A mention of a person is recorded, appears in the [activity log](activity.md) and reaches that person's [Inbox](inbox.md).
A mention of an agent starts a run only when you send the comment from a composer that asks the agent, as described under [Ask the agent from a comment](#ask-the-agent-from-a-comment).

### How comments render

A comment written by a person renders Markdown: bold, italic, code, lists, quotes, tables and links.
Links open in a new tab and only `http`, `https` and `mailto` links work.
Raw HTML and images are not rendered.
Text an agent wrote, a reply from a run or a message from a connected agent or service account, is always shown as plain text.

## Delete a comment

You can delete your own comment or reply while you can still comment.
**Delete** in the message's menu asks for confirmation.
The dialog reads "Delete this comment?", or "Delete this thread?" when it removes the whole thread, and says it cannot be undone.
**Delete** is in the menu of comments in the Activity card, in an observation's discussion and in the Inbox pane.

- The first comment of a thread can be deleted only while nobody else has written in the thread, and deleting it deletes the thread.
- Someone with Manage on the folder can delete any comment, including an agent's reply and the first comment of a thread that others have answered, which deletes the thread.
- Nobody else can delete an agent's reply.

An agent's reply that someone deleted is not posted again when its run is retried.
The server refuses anything else with 403 `not_author`, and `observe comments delete` does the same from the [CLI](cli.md).
A deletion is recorded in the [activity log](activity.md) as `thread.deleted`.

## Ask the agent from a comment

Mention one agent in a comment and send it.
The send button then reads "Send and ask Observe Agent" (or the connected agent's name), and the agent gets a run for that message.
Asking needs Comment on the dashboard's folder.

Only a thread on a dashboard or an observation can ask the agent.
A thread whose agent is someone else's connected agent saves your message and starts no run.
The agent answers in the thread and changes nothing: it reads the latest version, your message and the panel you chose, and its data.
It saves no version and opens no observation.

### From the form

Write your question, mention `@observe`, and send.
Pick a panel in **On** to tell the agent which panel you mean.
The comment is attached to the latest version and the panel you chose.

### From a thread

1. Choose **Reply** on a comment.
   The box at the bottom of the card shows "Replying in" and the panel or dashboard, and **On** hides.
2. Write the instruction with the mention, and send.
   The box stays in the thread, so the next message goes there too until you remove the "Replying in" chip.

The agent gets the earlier messages of the thread as context, the last 20 and at most 8000 characters.
An earlier answer of the agent appears as its finding, its title and text, not as the short reply that announced it.
The agent you asked becomes the thread's agent, so a later reply in the thread asks it again without a mention.
A reply that mentions a person goes to the team and starts nothing, and the thread keeps its agent for the next reply.
A message someone sent to everyone, by turning the "To:" chip or through the API, MCP or the CLI, turns the thread's agent off until a message asks an agent again.

### What you see while it runs

Under the form the card says "Observe Agent is working on it" with a link to the run, "`<name>` was asked" for a connected agent, or why no run started.
One reason is that the dashboard already has 5 runs waiting.
The comment is saved in every case, and a refusal never loses it.

When the run ends, its answer arrives as a reply in the same thread.
It comes from "Observe Agent for `<your name>`" or, for a connected agent, "`<your name>` (via `<client>`)".
A run started from a mention never resolves or reopens the thread.
Only a revision started over the API with `comment_id` resolves the thread when it saves a version.
**Revise the dashboard** on an answer posts its result in the thread and leaves the thread as it was.

A discussion entry reads "asked the agent" only when the message started a run.
A message that mentions an agent but started none, such as a comment for everyone, reads "mentioned the agent".
A message can mention one agent when it asks for a run: with two, the form says "Mention one agent per message" and does not send.

### When a mention does not start a run

A mention does not start a run, and the form says why, when:

- you are viewing an older version: "You are viewing an older version. Open the latest version to send this to the agent.";
- you are editing the layout: "Finish editing the layout to send this to the agent."

When the server has no model, the box reads "Leave a comment for the team and Claude Code".
A comment that mentions `@observe` is saved with a line saying no run started.
An external agent can pick the comment up over [MCP](mcp.md).

### Read and use the answer

When the run left an answer, the reply shows it as an answer card: the answer, and a chip such as "2 evidence panels" that unfolds the evidence panels in place and folds them again.
The reply itself reads "Answered.", and the answer shows under it in the thread, so everyone who can read the thread reads what was found without opening the run.

When the answer suggests a change to the dashboard, every reader sees "Suggested change: `<change>`".
Someone who may reply in the thread also gets **Revise the dashboard**.
It posts "Revise the dashboard: `<change>`" in the thread as you and starts a run, yours and taken by the agent that answered.
That run saves a new version when you can edit the dashboard, and opens a proposal when you can only comment.

Once the change is asked for, or was already applied from that answer, the button reads **Revision asked for** and is off until you reload the page.
Asking again while that run is waiting or running, or after it succeeded, posts nothing more.
A refusal, such as five revisions already waiting on the dashboard, shows the server's reason and leaves the button as it was.

The button is off in these cases:

- A message to the agent could not be sent because you are viewing an older version or editing the layout.
  It shows the same reason as the form below.
- The server has no model and the built-in agent answered.
  It shows "No model is connected to this server, so the agent cannot make this change."

On a [starter dashboard](activity.md#audit-folder), and for anyone who can no longer comment on the dashboard, the answer keeps its suggested change without **Revise the dashboard**.
A starter adds "This starter dashboard is kept current by SlashObserve releases, so it cannot be edited. Duplicate it to get a copy you can change." with **Duplicate**, which opens the same dialog as the dashboard menu.
A revision the server still refuses as `provisioned` shows "This starter dashboard is kept current by SlashObserve releases, so it cannot be edited."

### Keep the answer as an observation

The person who asked gets **Create observation**, which saves the answer as an observation and opens it.
In the web app it sits beside the chip when the agent marked the answer worth keeping and it is not yet in an observation.
Otherwise it sits under the evidence once you unfold it.
`observe save-answer` and `save_run_answer` save it whichever way the web app shows it.

A question asked in an observation's discussion writes nothing on that observation's page, so its **Create observation** files a new observation, and the reply in the discussion stays.
An answer already in an observation links to it with **Open the observation**.
When the asker later saves that answer as an observation, everyone who can read the thread still opens it from the thread, and someone who cannot see the new observation reads the answer without it.

## Share a link

**Share** in the dashboard's top row opens a Share pane in the same card.

- "Link to this dashboard" copies the address of the dashboard.
  It carries the time range and the variable values you are looking at, and each has a checkbox to leave it out.
- "Link to one panel" picks a data or text panel by title and copies a link that opens the dashboard with that panel focused, expanding its row if the row is collapsed.
- "Who has access" says "Everyone, by role" for an open folder, or lists the people and groups of a restricted one, with the instance Admins who always have access.

A link grants nothing.
The person who opens it signs in, and sees "Dashboard not found" when the dashboard's folder is not open to them, the same page as for a link with a wrong id.
People who can manage a folder other than General also get a **Manage folder access** button.
Everyone else sees who can add people and create public links, namely the people who manage that folder; see [Folders](../teams/folders.md).

## See which threads are unread

A thread that holds something you have not read shows a small dot before its author's name.
It is unread when someone else wrote in it, or the agent answered a request you made in it, after you last opened it, answered in it or joined.
Your own messages never make a thread unread, and threads written before your account existed do not count.

Click the dot, or click or focus anywhere in the thread, and it counts as read for you alone.
Answering in a thread marks it read too.

Over the API each thread and comment carries `unread`, and `list_comments` shows it on the first comment of each thread.
`POST /api/v1/threads/{thread_id}/read` marks one read for a person, and a service-account key never has unread threads.

## See who else is viewing

While a dashboard is open, the header shows "Also viewing" with the names of the other people who have it open.
It lists up to three names and a count for the rest, and is empty when nobody else is there.
Anyone who can view the dashboard sees the names, and a person counts for 45 seconds after their open page last reported in.

Over the API, `GET /api/v1/dashboards/{id}/viewers` returns the list.
`list_dashboard_viewers` returns it over MCP, and `observe dashboards viewers <dashboard-id>` prints it.
A service-account key never counts as a viewer.

## Resolve and reopen a thread

A thread has **Resolve** and, once resolved, **Reopen**.
A resolved thread folds to its first line with the number of replies, such as "2 replies".
**Show thread** unfolds it and **Hide thread** folds it again.
The buttons are hidden while a request tied to the thread is waiting or running.

A resolved thread reads "Resolved by version N" when the agent or an MCP client resolved it with the version that fixed it, and "Resolved by `<name>`" when a person did.
With no model on the server, an open thread reads "Open, waiting for `<client>`", so the person or tool that will act on it is clear.

Resolving and replying need the same access as commenting.
A Commenter sees **Resolve** only on a thread they started, and the server refuses any other with 403 `not_starter`.
An Editor or an Admin resolves any thread, and every commenter may reopen one.

Over the API and MCP each thread carries `allowed_actions`, the actions the caller may take now (`reply`, `resolve` or `reopen`).
It is empty for someone who may not comment, and leaves out `resolve` for a Commenter on a thread they did not start; see [States](../reference/states.md).

A thread can also be on a [proposal](../concepts/proposals.md), written `proposal:<id>` as the target of the threads API.
It has no panel or version and follows the proposal's dashboard: View reads it and Comment writes to it.

## Switch between versions

The version button in the dashboard's top row reads "Live · v3" when you are on the latest version and "v2 of 3" when you have pinned an older one.
"Live" becomes "Connecting" or "Reconnecting" while the page has no connection for live updates.
A dashboard in a background tab lets go of that connection after 15 seconds and catches up with what changed when you return to it.
When someone moves the dashboard into a folder you cannot open, the page shows "Dashboard not found" as soon as that connection ends.

Choose the button to see the five newest versions, each with its number, summary, author and time, and "Latest" on the newest.
Picking one pins it in the page address as `version=2`, so a link shares exactly that version.

While a version is pinned, the dashboard is read-only: you cannot edit the layout or a panel, and you cannot send changes to the agent.
Open the latest version first.
When someone saves a newer version, a toast reads "Version N is available" with **View latest**.
On the latest version the page switches by itself and says "Updated to version N".

### Open the version history

**Compare versions** in the menu opens the history page, `/d/<id>/versions`, headed "Versions of `<title>`".
It lists every version in a table with Version, Summary, Author, Saved and Actions, 50 at a time with **Load more**.
Badges mark "Latest" and "Restored from v2" for a version created by a restore.
**View** opens the dashboard at that version.

## Compare two versions

1. Open the history page and choose **Compare** on a row.
   It compares that version with the one before it, and version 1 compares against "an empty dashboard".
2. To pick another pair, use the **From** and **To** menus in the section "Changes from v2 to v3".
   "Empty dashboard" is available under **From**.

The page asks you to pick an older version on the left when From is not earlier than To, and says "No differences" when the two are equal.
Changes are grouped by the dashboard, then each panel and variable.
A row is Added, Removed, Changed or Reordered, and a changed value shows its Before and After.
**Show all** expands a long one.

## Restore a version

1. Choose **Restore** on any row except the latest.
   It needs Edit on the dashboard's folder.
2. Read the dialog "Restore version N?", which says "This saves a new version X with the dashboard exactly as it was in version N. Nothing is deleted."
3. Confirm.
   A new latest version is added with the summary "Restore version N", the toast reads "Restored version N as version M", and you return to the dashboard.

Restore is a copy, not a rewind.
The server checks that the old Spec is valid and that its datasource references still exist, but it does not run the panel queries again.
A restored panel can therefore show no data if its datasource has changed since, and panels already marked unverified stay so.

If someone saved a newer version while the dialog was open, it says "Version M was saved meanwhile." and the button changes to "Restore on top of version P".

## Related pages

- [Chat and follow-ups](chat.md) for what the agent does with a request.
- [Edit a panel](edit-panel.md) to fix a panel by hand, which also saves a version.
- [MCP](mcp.md) for agents that read and resolve comments from outside.
- [Agents](agents.md) for handing a comment to a connected agent and choosing whether it saves or proposes.
- [Proposals](../concepts/proposals.md) for a change that waits for someone to merge it.
