---
title: Comments, agent requests and versions
description: Review a dashboard with comments, send changes to the agent, and compare or restore saved versions.
---

Every dashboard keeps its comments, the requests people sent to the agent and its saved versions in one place, the **Activity** card.
Choose **Activity** at the top right of a dashboard to open it; the button shows the number of open comments.

Versions are immutable and linear.
Each save adds the next version number, and nothing edits or deletes an earlier one.
Deleting the dashboard deletes its versions, comments, agent requests and import record with it, for everyone, and cannot be undone.
**Delete dashboard...** is in the dashboard's **More** menu and in the row menu on the Dashboards page, asks for confirmation, and needs Edit on the folder; `observe dashboards delete` does the same from the [CLI](cli.md).
Anyone with the dashboard open sees a toast saying that it was deleted.

## The activity timeline

The card is headed "Activity" with the description "Comments, agent requests and saved versions."
Without a model on the server it reads "Comments and saved versions."
Entries are sorted by time and come in four kinds:

- **Comment threads** show "`<name>` commented" and, when the comment is about a panel, "on `<panel>`".
  The name is the same label versions use, so a comment written through an MCP client reads "Ishan (via Claude Code)", and a thread the agent resolved after a request reads "Resolved by Observe Agent for Ishan".
  Choosing the panel name scrolls to the panel and highlights it.
  A comment on a panel that no longer exists reads "on a removed panel".
- **Agent requests** show "`<name>` asked the agent" and one of "Sent to the agent", "Resolved by version N", "Finished, nothing changed" or "The agent could not finish: `<reason>`".
  While the agent works, the entry reads "Agent is working on it" with its latest step and the step number, "Queued, position N" while it is queued, or "Canceling" after its requester cancels it.
  **Details** opens the full conversation at `/c/<id>` in a new tab.
- **Versions** show who saved as their label and "saved", such as "Ishan saved", "Ishan (via Claude Code) saved" or "Observe Agent for Ishan saved", then "Version N" with its summary, or "No summary".
  **See changes** opens the comparison with the previous version.
  The card loads the 50 newest versions first.
  When the dashboard has more, **Load older versions** at the top of the list adds the next 50 and moves focus to the oldest version it added.
  The button shows while the filters keep versions in the list, so it is hidden under **Open comments** and when a panel is chosen.
- **Imports** show "`<label>` imported this dashboard from Grafana", where the label is the importer's, such as "Ishan" or "Ishan (via Claude Code)", with the conversion counts, and list the panels that are still unverified in the latest version with the reason each failed, and an **Ask the agent to fix** button where you may send requests.
  See [Import Grafana dashboards](import-grafana.md).

Two menus filter the list.
**Show** switches between Everything, Open comments and Changes only, and **Panel** narrows it to one panel.
An empty list says "Nothing here yet. Comments, agent requests and versions show up here."
Filters that match nothing say "No activity matches these filters."

## Comment on a panel or the dashboard

At the bottom of the card, the form "Add to the activity" has an **On** menu with "The whole dashboard" and every panel title, and a box labelled "Comment or request".
Write the comment and choose **Send** (the up arrow), or press Cmd+Enter (Ctrl+Enter elsewhere).
A comment is 1 to 4000 characters; a counter appears from 3500.

To start from a panel, open its menu ("Panel menu: `<title>`") and choose **Comment**.
The card opens scoped to that panel with the cursor in the box.
A panel with open comments shows a badge, "N open comments on `<title>`".

A comment is attached to the latest version when you write it.
A comment that mentions no agent never asks an agent to do anything.

Commenting needs the Comment level on the dashboard's folder: the Commenter role or higher, or a Comment entry or higher on the folder.
Without it you see "You can view dashboards in `<folder>`. Ask an Admin if you need to comment."
With Comment but not Edit you see "You can view and comment in `<folder>`. What the agent changes for you reaches the dashboard as a proposal that someone with Edit on this folder merges."
See [Roles](../teams/roles.md) and [Folders](../teams/folders.md).

## Edit a comment and mention people

Your own comments and replies have an **Edit** button while you can still comment on the dashboard.
It opens the text in place; **Save** or Cmd+Enter (Ctrl+Enter elsewhere) keeps the change, and **Cancel** or Escape drops it.
An edited comment shows "edited" beside its time.
Saving the text unchanged does nothing.
Editing never reopens a resolved thread and never changes who wrote the comment.

The **Mention** box, or typing `@` after a space, lists the people who already wrote in the thread, the built-in agent (Observe Agent, `@observe`) and your own connected agents.
Choosing one inserts it at the cursor.
The form at the bottom of the card has the same box, but a new thread has no people yet, so it offers only the agents.
A person is stored as `@[Name](user:<id>)` and a connected agent as `@[Name](grant:<id>)`; both show as "@Name" once the comment is saved, using the name the server has.
A comment can mention at most 20 people and agents.
A mention is recorded and appears in the [activity log](activity.md); it does not notify anyone.
A mention of an agent starts a run only when you send the comment from a composer that asks the agent, as described next.

## Ask the agent from a comment

Mention one agent in a comment and send it: the send button then reads "Send and ask Observe Agent" (or the connected agent's name), and the agent gets a run for that message.
Asking needs Comment on the dashboard's folder.
With Comment but not Edit, what the agent builds becomes a [proposal](../concepts/proposals.md) for someone who can edit to merge, and **Save what was built** is not offered.

- **From the form.** Write what to change, mention `@observe`, and send.
  Pick a panel in **On** to tell the agent which panel you mean.
  The comment is attached to the latest version and the panel you chose.
- **From a thread.** Choose **Reply** on a comment, write the instruction with the mention, and send.
  The agent gets the earlier messages of the thread as context.

Under the form the card says "Observe Agent is working on it" with a link to the run, "`<name>` was asked" for a connected agent, or why no run started, for example that the dashboard already has 5 runs waiting.
The comment is saved in every case, and a refusal never loses it.
When the run ends, its answer arrives as a reply in the same thread, from "Observe Agent for `<your name>`" or, for a connected agent, "`<your name>` (via `<client>`)".
A run started from a mention never resolves or reopens the thread.

A message can mention one agent when it asks for a run: with two, the form says "Mention one agent per message" and does not send.

The agent revises the latest version and saves the next one, as described in [Chat and follow-ups](chat.md).
It sees the Spec, your message and the panel you chose.

A mention does not start a run, and the form says why, when:

- you are viewing an older version: "You are viewing an older version. Open the latest version to send changes to the agent.";
- you are editing the layout: "Finish editing the layout to send changes to the agent.";
- 5 requests are already queued or running for this dashboard: "5 requests are already waiting for the agent. Send this when one finishes."

When the server has no model, the box reads "Leave a comment for the team and Claude Code", and a comment that mentions `@observe` is saved with a line saying no run started.
An external agent can pick the comment up over [MCP](mcp.md).

## Share a link

**Share** in the dashboard's top row opens a Share pane in the same card.
"Link to this dashboard" copies the address of the dashboard.
It carries the time range and the variable values you are looking at, and each has a checkbox to leave it out.
"Link to one panel" picks a data or text panel by title and copies a link that opens the dashboard with that panel focused, expanding its row if the row is collapsed.
A link grants nothing: the person who opens it signs in, and sees "You don't have access to this dashboard" when the dashboard's folder is not open to them.
"Who has access" says "Everyone, by role" for an open folder, or lists the people and groups of a restricted one, with the instance Admins who always have access.
People who can manage a folder other than General also get a **Manage folder access** button; see [Folders](../teams/folders.md).

## Resolve and reopen

A thread has **Resolve** and, once resolved, **Reopen**.
A resolved thread reads "Resolved by version N" when the agent or an MCP client resolved it with the version that fixed it, and "Resolved by `<name>`" when a person did.
The buttons are hidden while a request tied to the thread is waiting or running.
Over the API and MCP each thread carries `allowed_actions`, the actions the caller may take now (`reply`, `resolve` or `reopen`), which is empty for someone who may not comment; see [States](../reference/states.md).
A thread can also be on a [proposal](../concepts/proposals.md), written `proposal:<id>` as the target of the threads API.
It has no panel or version and follows the proposal's dashboard: View reads it and Comment writes to it.

Replies sit under the comment they answer.
With no model on the server, an open thread reads "Open, waiting for `<client>`", so the person or tool that will act on it is clear.

Resolving and replying need the same access as commenting.

## Versions

The version button in the dashboard's top row reads "Live · v3" when you are on the latest version and "v2 of 3" when you have pinned an older one.
"Live" becomes "Connecting" or "Reconnecting" while the page has no connection for live updates.
Choose it to see the five newest versions, each with its number, summary, author and time, and "Latest" on the newest.
Picking one pins it in the page address as `version=2`, so a link shares exactly that version.

While a version is pinned, the dashboard is read-only: you cannot edit the layout or a panel, and you cannot send changes to the agent.
Open the latest version first.
When someone saves a newer version, a toast reads "Version N is available" with **View latest**; on the latest version the page switches by itself and says "Updated to version N".

**Compare versions** in the menu opens the history page, `/d/<id>/versions`, headed "`<title>`: history".
It lists every version in a table with Version, Summary, Author, Saved and Actions, 50 at a time with **Load more**.
Badges mark "Latest" and "Restored from v2" for a version created by a restore.
**View** opens the dashboard at that version.

## Compare two versions

Choose **Compare** on a row to compare that version with the one before it.
Version 1 compares against "an empty dashboard".
The section "Changes from v2 to v3" has **From** and **To** menus to pick any pair, with "Empty dashboard" available under **From**.
It asks you to pick an older version on the left when From is not earlier than To, and says "No differences" when the two are equal.

Changes are grouped by the dashboard, then each panel and variable.
A row is Added, Removed, Changed or Reordered, and a changed value shows its Before and After; **Show all** expands a long one.

## Restore a version

Choose **Restore** on any row except the latest.
It needs Edit on the dashboard's folder.
The dialog "Restore version N?" explains: "This saves a new version X with the dashboard exactly as it was in version N. Nothing is deleted."
Confirming adds a new latest version whose summary is "Restore version N", shows the toast "Restored version N as version M" and returns you to the dashboard.

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
