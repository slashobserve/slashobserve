---
title: Comments, agent requests and versions
description: Review a dashboard with comments, send changes to the agent, and compare or restore saved versions.
---

Every dashboard keeps its comments, the requests people sent to the agent and its saved versions in one place, the **Activity** card.
Choose **Activity** at the top right of a dashboard to open it; the button shows the number of open comments.

Versions are immutable and linear.
Each save adds the next version number, and nothing edits or deletes an earlier one.
Deleting the dashboard deletes its versions, comments, agent requests and import record with it, for everyone, and cannot be undone.

## The activity timeline

The card is headed "Activity" with the description "Comments, agent requests and saved versions."
Without a model on the server it reads "Comments and saved versions."
Entries are sorted by time and come in four kinds:

- **Comment threads** show "`<name>` commented" and, when the comment is about a panel, "on `<panel>`".
  Choosing the panel name scrolls to the panel and highlights it.
  A comment on a panel that no longer exists reads "on a removed panel".
- **Agent requests** show "`<name>` asked the agent" and one of "Sent to the agent", "Resolved by version N", "Finished, nothing changed" or "The agent could not finish: `<reason>`".
  While the agent works, the entry reads "Agent is working on it" with its latest step, or "Waiting to start".
  **Details** opens the full conversation at `/observe/<id>` in a new tab.
- **Versions** show who saved: "Agent saved", "`<name>` saved" or "`<client>` saved over MCP", then "Version N" with its summary, or "No summary".
  **See changes** opens the comparison with the previous version.
  The card loads the 50 newest versions first.
  When the dashboard has more, **Load older versions** at the top of the list adds the next 50 and moves focus to the oldest version it added.
  The button shows while the filters keep versions in the list, so it is hidden under **Open comments** and when a panel is chosen.
- **Imports** show "`<name>` imported this dashboard from Grafana" with the conversion counts, and list the panels that are still unverified in the latest version with the reason each failed, and an **Ask the agent to fix** button where you may send requests.
  See [Import Grafana dashboards](import-grafana.md).

Two menus filter the list.
**Show** switches between Everything, Open comments and Changes only, and **Panel** narrows it to one panel.
An empty list says "Nothing here yet. Comments, agent requests and versions show up here."
Filters that match nothing say "No activity matches these filters."

## Comment on a panel or the dashboard

At the bottom of the card, the form "Add to the activity" has an **On** menu with "The whole dashboard" and every panel title, and a box labelled "Comment or request".
Write the comment and choose **Add comment**, or press Cmd+Enter (Ctrl+Enter elsewhere).
A comment is 1 to 4000 characters; a counter appears from 3500.

To start from a panel, open its menu ("Panel menu: `<title>`") and choose **Comment**.
The card opens scoped to that panel with the cursor in the box.
A panel with open comments shows a badge, "N open comments on `<title>`".

A comment is attached to the latest version when you write it.
**Add comment** never asks the agent to do anything.

Commenting needs the Commenter role or higher, or Edit on the dashboard's folder.
A Viewer without that access sees "You can view this dashboard as a Viewer. Ask an Admin if you need to comment."
A Commenter sees "You can view and comment as a Commenter. An Editor can send your comment to the agent or change the panels."
See [Roles](../teams/roles.md) and [Folders](../teams/folders.md).

## Send a comment to the agent

There are two ways to ask the agent for a change from the activity card, and both need Edit on the dashboard's folder.

- **From the form.** Write what to change in the box and choose **Send to agent**.
  The request appears as an agent request entry, not a comment thread.
  Pick a panel in **On** to tell the agent which panel you mean.
- **From a thread.** Choose **Reply** on a comment, write the instruction, and choose **Send to agent** in the reply form.
  The server saves your reply first, then sends the request tied to that comment.
  If the request fails after the reply is saved, the card says "Your reply was saved." and why the request failed.
  When the agent saves a version for it, the server resolves the thread itself and the thread shows "Resolved by version N".

The agent revises the latest version and saves the next one, as described in [Chat and follow-ups](chat.md).
It sees the Spec, your message and the panel you chose.

**Send to agent** is not offered when:

- the server has no model; the box then reads "Leave a comment for the team and Claude Code", and an external agent can pick the comment up over [MCP](mcp.md);
- you are viewing an older version: "You are viewing an older version. Open the latest version to send changes to the agent.";
- you are editing the layout: "Finish editing the layout to send changes to the agent.";
- 5 requests are already queued or running for this dashboard: "5 requests are already waiting for the agent. Send this when one finishes."

## Resolve and reopen

A thread has **Resolve** and, once resolved, **Reopen**.
A resolved thread reads "Resolved by version N" when the agent or an MCP client resolved it with the version that fixed it, and "Resolved by `<name>`" when a person did.
The buttons are hidden while a request tied to the thread is waiting or running.

Replies sit under the comment they answer.
With no model on the server, an open thread reads "Open, waiting for `<client>`", so the person or tool that will act on it is clear.

Resolving and replying need the same access as commenting.

## Versions

The version button in the dashboard's top row reads "Live · v3" when you are on the latest version and "v2 of 3" when you have pinned an older one.
Choose it to see the five newest versions, each with its number, summary, author and time, and "Latest" on the newest.
Picking one pins it in the page address as `version=2`, so a link shares exactly that version.

While a version is pinned, the dashboard is read-only: you cannot edit the layout or a panel, and you cannot send changes to the agent.
Open the latest version first.
When someone saves a newer version, a toast reads "Version N is available" with **View latest**; on the latest version the page switches by itself and says "Updated to version N".

**Compare versions** in the menu opens the history page, `/dashboards/<id>/versions`, headed "`<title>`: history".
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
