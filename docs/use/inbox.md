---
title: Inbox
description: See what asks something of you, a proposal to merge, a block to keep, a mention, a reply, a failed run, a saved health repair or a request for comment access, with the reason it is there and an unread count that follows you across browsers.
---

The inbox is the activity you can see, filtered to what asks something of you now.
It is a read over the [activity log](activity.md), not a second store of events, so it cannot show something that did not happen.
`GET /api/v1/inbox` lists it, newest first, `POST /api/v1/inbox/read` marks it read up to one item, and `PUT /api/v1/inbox/items/{seq}` marks one item read, unread or done.
Both need the Viewer role or higher and a signed-in person: a service-account key gets 403 `user_session_required`.
A connected agent signed in as a person reads the same list with the MCP tool `list_inbox`, which marks nothing read, and marks an item done with `set_inbox_item_done`.
The `observe` command line signs in with a key, so it has no inbox commands.

## In the web app

**Inbox** is in the sidebar under Home, at `/inbox`.
A badge on the entry shows the unread count, in the full sidebar and on the icon rail.
The page lists the items in two groups, "Needs you" for the items with a reason, each marked with its reason as "Proposal", "Suggested block", "Mention", "Reply", "Run failed" or "Access request", and "Workspace activity" for the rest, which only the Everything view shows.
The tabs **For you**, **Everything**, **Proposals** and **Mentions** are the [views](#views) below, and **Load more** fetches the next page.
Under the tabs, **Open**, **Unread** and **Done** choose which items the list shows, as the [filter](#done-and-the-unread-filter) below says, and **Group by dashboard or observation** puts the items about the same dashboard, observation or proposal together under its title, newest group first.
The list refreshes when new activity arrives.

Each row names who did what and the real title of the dashboard, observation or proposal, shows the start of the comment for a mention or a reply, and has a dot while it is unread.
Two buttons at the right of the row, shown when you hover or focus the row, mark that one item read or unread, and done or, on the Done filter, back in the inbox.

**Mark all read** marks the newest item on the page and everything older as read, and shows only while something is unread.
Arrow keys move through the rows.
On a window 1100 pixels wide or more, a selected row opens in a pane beside the list, and the address keeps the view and the item (`view` and `item`).
A proposal opens there with its changes, **Merge** and **Discard**, and **Open proposal** goes to its page.
A mention or a reply opens there as the whole thread, with a **Reply** box that works as it does on the page: `@` asks an agent, and you can edit or delete your own comments.
Any other item shows its text, and every item has an **Open** button, **Mark as read** or **Mark as unread**, and **Mark done** or, on the Done filter, **Move back to the inbox**.
Opening an item marks it read, once: marking it unread again while it is open keeps it unread.
On a narrower window the list stands alone and each row is a link, and following it marks the item read.

## What asks something of you

Each item has a `reason`, and an item that matches several rules has the first in this order.
Items are from the last 30 days, in folders you can see, and never your own action unless the rule says so.

| Reason | The item is |
| --- | --- |
| `proposal` | A [proposal](../concepts/proposals.md) that someone else opened on a dashboard in a folder you can edit, and that is still open. |
| `requested_block` | A block that someone requested on an observation you created and can still edit, and that is still waiting to be kept or discarded. |
| `mention` | A comment by someone else that mentions you by name. |
| `reply` | A reply by someone else in a thread you wrote in, or a reply that an agent run wrote for a run you requested. |
| `run_failed` | A run you requested that failed and that nobody has continued. |
| `repaired` | A health repair you started from a dashboard or the dashboards list that saved a version. |
| `proposal_closed` | A proposal you opened that someone else merged or discarded. |
| `access_request` | A request for [comment access](../teams/folders.md#ask-for-comment-access) to a dashboard or observation in a folder you manage, or as an instance Admin when nobody manages it, that is still open. |

An item leaves the inbox when its reason stops being true, for example when the proposal is merged or the failed run is continued.
A comment that mentions you and also replies in a thread you wrote in is one `mention` item, not two.

## Opening an item

A proposal item reads "proposed" followed by the proposal's title, and opens the proposal.
A mention or a reply opens the thread it is in: on a dashboard that is the dashboard with the activity pane open on the thread (`thread=<thread id>` in the address), on an observation its discussion, and on a proposal the proposal.
A closed proposal reads "merged your proposal" or "discarded your proposal" with its title, shows the reason given for a discard, and opens the proposal; the same notice is posted as a comment in the proposal's discussion.
A failed run opens its chat.
A repaired item reads "repaired", the number of panels and the dashboard's title, and opens the versions compare of the version the repair saved.
An item recorded before the title and panel count were stored reads "Your repair saved version N".
An access request reads "asked for comment access to" followed by the title, shows the note, and opens the dashboard's Share pane, or for an observation its folder's settings; for something in General, which takes no folder entry, it says to raise the person's role or move the item to a folder first and opens People (API keys for a service account); marking it done declines it for every recipient.

## Views

`view` chooses what the list holds, and `for_you` is the default.

| View | Holds |
| --- | --- |
| `for_you` | Every item with a reason. |
| `proposals` | The `proposal` and `proposal_closed` items. |
| `mentions` | The `mention` and `reply` items. |
| `everything` | Every visible event from others, and every item with a reason, with `reason` null on the events that have none. |

`limit` is 1 to 100 and defaults to 50, and `cursor` takes the `next_cursor` of the previous page.
An item is the event as `GET /api/v1/activity` returns it, plus `reason`, `unread`, `target_title` and `excerpt`.
`target_title` is the title of the dashboard, observation or proposal the item is about, and is null when it has none or has been deleted.
`excerpt` is the start of the comment for a `mention` or `reply`, with each mention shown as `@Name`, the error of a failed run, the reason a proposal was discarded, or the note of an access request, as plain text of at most 200 characters, and is null for other items.
The rest of an item carries no comment, message or finding text, as in the log.

## Done and the unread filter

`filter` chooses which of the items of the view the list holds, and `open` is the default.

| Filter | Holds |
| --- | --- |
| `open` | Every item you have not marked done. |
| `unread` | The open items that are unread. |
| `done` | The items you marked done. |

`PUT /api/v1/inbox/items/{seq}` takes `{"done": true}` to mark an item done: it leaves the `open` and `unread` lists, counts as read, never counts in `unread_count`, and stays done when you read the inbox.
`{"done": false}` brings it back to the open list as read, and marking a done item unread brings it back as unread.
Send exactly one of `unread` and `done` in a body.
The MCP tool `list_inbox` takes the same `filter`, and `set_inbox_item_done` takes a `seq` and `done`.
An item that is done stays done until it leaves the 30 days of the inbox.

## What you have read

The server keeps one read position per person, so it follows you from one browser to the next.
`POST /api/v1/inbox/read` takes `{"seq": N}`, the `seq` of the newest item you saw: that event and everything older is read.
The position only moves forward, so marking an older `seq` changes nothing, and an unknown `seq` is refused with 422 `unknown_seq`.
`PUT /api/v1/inbox/items/{seq}` takes `{"unread": true}` or `{"unread": false}` and marks that one item, whatever its place relative to the position.
Marking a later `seq` read with `POST /api/v1/inbox/read` sets every item up to it to read again, including those you marked unread.
Until you read once, every item in the last 30 days is unread.

Every list answers `unread_count`, the number of unread `for_you` items whatever the view is, and 100 means 100 or more.
