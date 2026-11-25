---
title: Inbox
description: See what asks something of you, a proposal to merge, a block to keep, a mention, a reply, a failed run, a saved health repair or a request for comment access, with the reason it is there and an unread count that follows you across browsers.
---

The inbox lists what asks something of you now: a proposal to merge, a block to keep, a mention, a reply, a failed run, a saved health repair or a request for comment access.
Each item shows the reason it is there, and an unread count follows you from one browser to the next.

The inbox is a read over the [activity log](activity.md), not a second store of events, so it cannot show something that did not happen.
It needs the Viewer role or higher and a signed-in person.
A service-account key gets 403 `user_session_required`, and the `observe` command line signs in with a key, so it has no inbox commands.

## Work through your inbox

1. Choose **Inbox** in the sidebar, under Home, which opens `/inbox`.
   A badge on the entry shows the unread count, in the full sidebar and on the icon rail.
2. Read the **Needs you** group.
   Each item there is marked with its reason: "Proposal", "Suggested block", "Mention", "Reply", "Run failed", "Repaired", "Proposal closed" or "Access request".
3. Select an item to open it.
   Opening it marks it read.
4. Act on it: merge a proposal, reply to a mention, or open a failed run's chat to ask again.
5. Mark the item done, with **Mark done**.
   A "Marked done" toast offers **Undo**.

On a window 1100 pixels wide or more, a selected row opens in a pane beside the list.
The address keeps the view and the item (`view` and `item`).
On a narrower window the list stands alone, each row is a link, and following it marks the item read.

Each row names who did what and the real title of the dashboard, observation or proposal.
It shows the start of the comment for a mention or a reply, and has a dot while it is unread.
A reply the agent wrote reads "Observe Agent answered" followed by the question it answered, in place of "replied on" and the title.
A connected agent keeps its own name, such as "Ada (via Claude Code)".

The list refreshes when new activity arrives.
**Load more** fetches the next page, and arrow keys move through the rows.

## Choose what the list shows

The tabs **For you**, **Everything**, **Proposals** and **Mentions** are the [views](#views).
**For you** is the default.
**Everything** also lists "Workspace activity", the events that have no reason.

Under the tabs, **Open**, **Unread** and **Done** choose which items the list shows; see [Done and the unread filter](#done-and-the-unread-filter).
**Group by dashboard or observation** puts the items about the same dashboard, observation or proposal together under its title, newest group first.
The view, the filter, the grouping and the selected item are kept in the address.

## Mark items read or done

Two buttons at the right of a row, shown when you hover or focus the row, mark that one item read or unread, and done or, on the Done filter, back in the inbox.

**Mark all read** marks the newest item on the page and everything older as read.
It shows only while something is unread.

Opening an item marks it read, once.
Marking it unread again while it is open keeps it unread.

The pane of an item has an **Open** button, **Mark as read** or **Mark as unread**, and **Mark done** or, on the Done filter, **Move back to the inbox**.

## Reply or merge from the pane

- A proposal opens with its changes, **Merge** and **Discard**.
  **Open proposal** goes to its page.
- A mention or a reply opens as the whole thread, with a **Reply** box that works as it does on the page.
  `@` asks an agent, and you can edit or delete your own comments.
- Any other item shows its text.

## What opens when you open an item

- A proposal item reads "proposed" followed by the proposal's title, and opens the proposal.
- A mention or a reply opens the thread it is in.
  On a dashboard that is the dashboard with the activity pane open on the thread (`thread=<thread id>` in the address).
  On an observation and on a proposal it is the page with its discussion open and the thread marked (`thread=<thread id>` in the address).
- A closed proposal reads "merged your proposal" or "discarded your proposal" with its title, shows the reason given for a discard, and opens the proposal.
  The same notice is posted as a comment in the proposal's discussion.
- A failed run reads "Your run failed on" followed by the dashboard or observation it was about and the question you asked, shows the error, and opens its chat.
  A run that answered in a thread is one item, not also a reply with the failure text.
- A repaired item reads "repaired", the number of panels and the dashboard's title, and opens the versions compare of the version the repair saved.
  An item recorded before the title and panel count were stored reads "Your repair saved version N".
- An access request reads "asked for comment access to" followed by the title, and shows the note.
  It opens the dashboard's Share pane, or for an observation its folder's settings.
  For something in General, which takes no folder entry, it says to raise the person's role or move the item to a folder first.
  It then opens People, or API keys for a service account.
  Marking it done declines it for every recipient.

## Read the inbox from an agent or the API

A connected agent signed in as a person reads the same list with the MCP tool `list_inbox`, which marks nothing read.
It marks an item done with `set_inbox_item_done`, which takes a `seq` and `done`.

Over HTTP:

- `GET /api/v1/inbox` lists the items, newest first.
- `POST /api/v1/inbox/read` marks the inbox read up to one item.
- `PUT /api/v1/inbox/items/{seq}` marks one item read, unread or done.

The HTTP routes answer only to a signed-in person's session, so a service-account key gets 403 `user_session_required`.

## Reference

### What asks something of you

Each item has a `reason`.
An item that matches several rules has the first in this order.
Items are from the last 30 days, in folders you can see, and never your own action unless the rule says so.

| Reason | The item is |
| --- | --- |
| `proposal` | A [proposal](../concepts/proposals.md) that someone else opened on a dashboard in a folder you can edit, still open. |
| `requested_block` | A block someone requested on an observation you created and can edit, still waiting. |
| `mention` | A comment by someone else that mentions you by name. |
| `reply` | A reply in a thread you wrote in, or a reply an agent run wrote for your run. |
| `run_failed` | A run you requested that failed and that nobody has continued. |
| `repaired` | A health repair you started that saved a version. |
| `proposal_closed` | A proposal you opened that someone else merged or discarded. |
| `access_request` | An open request for [comment access](../teams/folders.md#ask-for-comment-access). |

Details of the rules:

- A `proposal` item is one whose proposal is still open.
- A `requested_block` item is one whose block is waiting to be kept or discarded.
- A `reply` by someone else counts when you wrote earlier in the thread, or when an agent run wrote it for a run you requested; the failure reply of a run you requested that failed is left out while the `run_failed` item stands for it, but an answer the run posted before it failed stays.
- A `repaired` item comes from a health repair you started from a dashboard or the dashboards list.
- An `access_request` item is for a dashboard or observation in a folder you manage, or as an instance Admin when nobody manages it.
- An item leaves the inbox when its reason stops being true, for example when the proposal is merged or the failed run is continued.
- A comment that mentions you and also replies in a thread you wrote in is one `mention` item, not two.

### Views

`view` chooses what the list holds, and `for_you` is the default.

| View | Holds |
| --- | --- |
| `for_you` | Every item with a reason. |
| `proposals` | The `proposal` and `proposal_closed` items. |
| `mentions` | The `mention` and `reply` items. |
| `everything` | Every visible event from others, and every item with a reason. |

In `everything`, `reason` is null on the events that have none.
An invalid `view` answers 400 `invalid_view`.

### Paging and the item shape

`limit` is 1 to 100 and defaults to 50, and an invalid one answers 400 `invalid_limit`.
`cursor` takes the `next_cursor` of the previous page, and an invalid one answers 400 `invalid_cursor`.

An item is the event as `GET /api/v1/activity` returns it, plus these fields.

| Field | Holds |
| --- | --- |
| `reason` | The reason, or null in the `everything` view for an event without one. |
| `unread` | Whether the item is unread. |
| `target_title` | The title of the dashboard, observation or proposal the item is about. |
| `excerpt` | A short plain-text extract, as described below. |
| `question` | The question an agent reply answered, or the prompt of a failed run, as described below. |

Details of `target_title`: on a `run_failed` item it is the dashboard or observation the run was about; it is null when the item has no target, the target was deleted, or it is in a place you can no longer read.

Details of `excerpt`:

- It is the start of the comment for a `mention` or `reply`, with each mention shown as `@Name`.
- It is the error of a failed run (null when the run stored none), the reason a proposal was discarded, or the note of an access request.
- It is plain text of at most 200 characters.
- It is null for other items, and for a comment in a thread you can no longer read.

Details of `question`:

- It is the first line of the thread message that an agent run answered, cut to a title of at most 60 characters, on the `thread.replied` item that run wrote.
- On a `run_failed` item it is the first line of the question the run was asked, cut the same way: the thread message for a run asked in a thread, null once that message is deleted or you can no longer read the thread, and the prompt for any other run.
- On a `run_failed` item for a health repair it is "a panel repair", not the generated prompt.
- It is null on every other item, once that message is deleted or withdrawn, and when you can no longer read the thread.

The rest of an item carries no comment, message or finding text, as in the log.

### Done and the unread filter

`filter` chooses which of the items of the view the list holds, and `open` is the default.
An invalid one answers 400 `invalid_filter`.

| Filter | Holds |
| --- | --- |
| `open` | Every item you have not marked done. |
| `unread` | The open items that are unread. |
| `done` | The items you marked done. |

`PUT /api/v1/inbox/items/{seq}` takes one of `{"done": true}`, `{"done": false}`, `{"unread": true}` and `{"unread": false}`, and answers 204.
Send exactly one of `unread` and `done` in a body.

- `{"done": true}` marks an item done.
  It leaves the `open` and `unread` lists, counts as read, never counts in `unread_count`, and stays done when you read the inbox.
- `{"done": false}` brings it back to the open list as read.
- Marking a done item unread brings it back as unread.

The MCP tool `list_inbox` takes the same `filter`.
An item that is done stays done until it leaves the 30 days of the inbox.

### What you have read

The server keeps one read position per person, so it follows you from one browser to the next.

- `POST /api/v1/inbox/read` takes `{"seq": N}`, the `seq` of the newest item you saw.
  That event and everything older is read, and the answer is 204.
- The position only moves forward, so marking an older `seq` changes nothing.
  An unknown `seq` is refused with 422 `unknown_seq`.
- `PUT /api/v1/inbox/items/{seq}` with `{"unread": true}` or `{"unread": false}` marks that one item, whatever its place relative to the position.
- Marking a later `seq` read with `POST /api/v1/inbox/read` sets every item up to it to read again, including those you marked unread.
- Until you read once, every item in the last 30 days is unread.

Every list answers `unread_count`, the number of unread `for_you` items whatever the view is.
The count stops at 100, so 100 means 100 or more.
