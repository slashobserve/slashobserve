---
title: Inbox
description: See what asks something of you, a proposal to merge, a block to keep, a mention, a reply, a failed run or a saved health repair, with the reason it is there and an unread count that follows you across browsers.
---

The inbox is the activity you can see, filtered to what asks something of you now.
It is a read over the [activity log](activity.md), not a second store of events, so it cannot show something that did not happen.
`GET /api/v1/inbox` lists it, newest first, and `POST /api/v1/inbox/read` marks it read.
Both need the Viewer role or higher and a signed-in person: a service-account key gets 403 `user_session_required`.
A connected agent signed in as a person reads the same list with the MCP tool `list_inbox`, which marks nothing read.

## In the web app

**Inbox** is in the sidebar under Home, at `/inbox`.
A badge on the entry shows the unread count, in the full sidebar and on the icon rail.
The page lists the items in two groups, "Needs you" for the items with a reason, each marked with its reason as "Proposal", "Suggested block", "Mention", "Reply" or "Run failed", and "Workspace activity" for the rest, which only the Everything view shows.
The tabs **For you**, **Everything**, **Proposals** and **Mentions** are the [views](#views) below, and **Load more** fetches the next page.
The list refreshes when new activity arrives.

**Mark all read** marks the newest item on the page and everything older as read, and shows only while something is unread.
Arrow keys move through the rows.
On a window 1100 pixels wide or more, a selected row opens in a pane beside the list, and the address keeps the view and the item (`view` and `item`).
A proposal opens there with its changes, **Merge** and **Discard**, and **Open proposal** goes to its page; any other item has an **Open** button.
On a narrower window the list stands alone and each row is a link.

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

An item leaves the inbox when its reason stops being true, for example when the proposal is merged or the failed run is continued.

## Opening an item

A proposal item reads "proposed" followed by the proposal's title, and opens the proposal.
A mention or a reply opens the thread it is in: on a dashboard that is the dashboard with the activity pane open on the thread (`thread=<thread id>` in the address), on an observation its discussion, and on a proposal the proposal.
A failed run opens its chat.
A repaired item reads "repaired", the number of panels and the dashboard's title, and opens the versions compare of the version the repair saved.
An item recorded before the title and panel count were stored reads "Your repair saved version N".

## Views

`view` chooses what the list holds, and `for_you` is the default.

| View | Holds |
| --- | --- |
| `for_you` | Every item with a reason. |
| `proposals` | The `proposal` items. |
| `mentions` | The `mention` and `reply` items. |
| `everything` | Every visible event from others, and every item with a reason, with `reason` null on the events that have none. |

`limit` is 1 to 100 and defaults to 50, and `cursor` takes the `next_cursor` of the previous page.
An item is the event as `GET /api/v1/activity` returns it, plus `reason` and `unread`, and like the log it carries no comment, message or finding text.

## What you have read

The server keeps one read position per person, so it follows you from one browser to the next.
`POST /api/v1/inbox/read` takes `{"seq": N}`, the `seq` of the newest item you saw: that event and everything older is read.
The position only moves forward, so marking an older `seq` changes nothing, and an unknown `seq` is refused with 422 `unknown_seq`.
Marking one item without the ones before it is not offered.
Until you read once, every item in the last 30 days is unread.

Every list answers `unread_count`, the number of unread `for_you` items whatever the view is, and 100 means 100 or more.
