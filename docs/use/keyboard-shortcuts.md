---
title: Keyboard shortcuts
description: The keys that view, edit and inspect a panel, move the time range, refresh a dashboard, start TV mode and open the Inbox, where to see them in the app, and the keys of the logs page and the trace search.
---

A dashboard answers a few keys, the same ones Grafana uses.
Use them to open a panel, move the time range or refresh without reaching for the mouse.

## See the shortcuts in the app

1. Open a dashboard.
2. Press `?`.
   A dialog named **Keyboard shortcuts** lists the keys.

On any other page, open the command palette with `⌘K` or `Ctrl+K` and choose **Keyboard shortcuts**.

## Act on a panel

`v`, `e`, `i`, `a` and `c` act on the panel under the pointer.
With no panel under the pointer, they act on the panel that holds the keyboard focus.

1. Point at a panel.
2. Press `v` to view it at full size.
3. Press `v` again to go back to the dashboard.

Hovering a panel also shows **Ask** and **Comment** buttons in its header, which do what `a` and `c` do.
They fade in under a mouse or trackpad.
On a touch screen, and while the keyboard focus is in the panel, they are always shown.
A panel narrower than 240 pixels leaves them and its datasource name out of the header so the title keeps the room.
The keys and the panel menu still work there.
**Ask** needs the same things as `a`, and the panel menu offers both with the same rule.

## Move the time range

Press the first key, then the second within one second.
Any other key in between cancels the pair.

`t` then `z` zooms the time range out, and `t` then `←` or `→` moves it back or forward; the [dashboard keys table](#dashboard-keys) says by how much.

These keys turn the time range into fixed times, so the address of the page keeps the range you moved to.
A range that would end in the future ends now instead and keeps its length.
They do nothing on a dashboard that hides the time picker.

## Know when keys are ignored

The keys do nothing while you type in a field, while a menu or dialog is open, or while you hold `⌘`, `Ctrl` or `Alt`.
`v`, `e`, `i`, `a` and `c` also act on no panel while you have unsaved layout changes.
`c` then only opens the activity pane.

In the full-size view and in TV mode, `c` comments on the panel in the activity pane instead, and with no panel it opens the activity pane.
On a copy of a repeated panel, `c` also comments in the activity pane.

## Move around the app

The first Tab stop on every page after sign-in is **Skip to the page**, which appears at the top left.
Press Enter on it to move the focus to the page's heading and skip the sidebar.

When a page opens because you pressed Enter or followed a link, and the focus was on something that went away with the old page, the focus moves to the new page's heading.
Focus stays where it is when you open a page from the sidebar, and when the new page puts the focus in a field itself.

### Panes of a dashboard

Opening the Share, Activity, Chat or Dashboard JSON pane moves the focus to the pane's heading, on a phone as well.
A screen reader then reads the heading, and the keys work inside the pane.
A pane that was already open when the page loaded does not take the focus.

Escape closes the pane when the focus is inside it or on the button that opened it.
The focus returns to the button that opened the pane, which on a phone is the Controls button, and so does the pane's Close button.
A pane that was already open when the page loaded returns the focus to its own button in the header: Activity for the Activity and Chat panes, Share for Share, and More for Dashboard JSON.

## Open the Inbox

On every page after sign-in, press `g` then `i` within one second to open the [Inbox](inbox.md) panel, and again to close it.
On a dashboard, `i` after `g` opens the Inbox rather than Inspect.

## Observation page keys

An observation page answers `c`, `a` and `Esc` for the canvas, as [Comment and ask on the blocks](observations.md#comment-and-ask-on-the-blocks) describes.

- `c` and `a` open the composer or the ask box on the whole block under the pointer, or on the one that holds the keyboard focus.
  With no block there, they turn Comment or Ask mode on, and in that mode they turn it off.
- `a` skips a panel block whose panel shows no data.
- `Esc` first hides the **Comment** and **Ask** buttons under selected text.
  It then closes an open composer or ask box, and then leaves Comment or Ask mode for Select.
  When you have typed, it asks "Discard your draft?" before your text goes.
- The keys do nothing while you move or resize blocks, while you type in a field, and for a person who cannot comment.

## Logs page keys

The logs page answers keys of its own, described in [Logs and traces](logs-and-traces.md).
`?` opens the same list in a dialog named Keyboard shortcuts.
`/` and `?` do nothing while you type in a field or while a dialog is open.

| Keys | What they do |
| --- | --- |
| `/` | Focus the query. |
| `⌘Enter` or `Ctrl+Enter` | Run the query, from the query editor. |
| `⌘I` or `Ctrl+I` | Switch between Write a query and Query, from anywhere on the page. |
| `Ctrl+Space` | Complete a field, label or keyword, in the query editor. |
| `j` | Move to the first line when nothing has the focus, then to the next line. |
| `k` | Move to the previous line, while a line has the focus. |
| `Enter` | Open the line that has the focus. |
| `Esc` | Close the open line and return to it. |
| `?` | Show this list. |

## Metrics page keys

The metrics page answers three keys, described in [Metrics](metrics.md).

| Keys | What they do |
| --- | --- |
| `/` | Focus the query. |
| `⌘Enter` or `Ctrl+Enter` | Run the query, from the query editor. |
| `⌘I` or `Ctrl+I` | Switch between Write a query and PromQL, from anywhere on the page. |

## Trace search keys

The trace search answers keys of its own too, described in [Logs and traces](logs-and-traces.md).

| Keys | What they do |
| --- | --- |
| `⌘I` or `Ctrl+I` | Switch between Write a query and Query, from anywhere on the page. |
| `⌘Enter` or `Ctrl+Enter` | Run the search, from the Service, Span name and duration fields or from the SQL editor. |
| `Enter` | Write the query from the question, or run the search from Service, Span name or a duration field. |
| `Shift+Enter` | Add a line to the question. |

## Trace view keys

The waterfall of a trace answers keys of its own while a span row has the focus, described in [Logs and traces](logs-and-traces.md).

| Keys | What they do |
| --- | --- |
| `j` or `↓` | Move to the next span. |
| `k` or `↑` | Move to the previous span. |
| `Enter` | Open the span's details, or close them when they are open. |
| `Escape` | Close the open span's details. |
| `←` | Collapse the span's children. |
| `→` | Expand the span's children. |
| `/` | Jump to the **Find in trace** field. |

## Reference

### Dashboard keys

| Keys | What they do |
| --- | --- |
| `v` | View the panel at full size, or go back to the dashboard. |
| `e` | Edit the panel, when you may edit the dashboard. |
| `i` | Open Inspect for the panel, or close it. |
| `a` | Ask the agent about the whole panel. |
| `c` | Comment on the whole panel. |
| `Esc` | Close the topmost open thing, as listed below. |
| `t` then `z` | Zoom the time range out to twice its length. |
| `t` then `←` | Move the time range back by half its length. |
| `t` then `→` | Move the time range forward by half its length. |
| `d` then `r` | Refresh every panel. |
| `d` then `k` | Enter TV mode in full screen. |
| `f` | Go full screen in TV mode opened from a link. |
| `g` then `i` | Open or close the Inbox. |
| `?` | Show the keyboard shortcuts. |
| `⌘O` or `Ctrl+O` | Change how chart hover is shared between panels. |
| `⌘K` or `Ctrl+K` | Open the command palette, which searches the workspace as you type. |
| `⌘\` or `Ctrl+\` | Collapse or expand the sidebar; under 1200 pixels wide it opens the full sidebar over the page. |

Details of `a`:

- It opens a box on the panel when the panel is a data panel, you may comment on the dashboard and a model is connected.
- In the full-size view, in TV mode and on a copy of a repeated panel, it opens the Chat tab on that panel instead.
- With no panel it turns Ask mode on, and in Ask mode it turns it off.
- See [Ask and comment on a dashboard](ask-and-comment.md).

Details of `c`:

- It opens a composer on the panel when you may comment on the dashboard.
- With no panel it turns Comment mode on, and in Comment mode it turns it off.

Details of `Esc`, in the order it acts:

1. It closes an open comment or ask box, which leaves the mode on.
2. It closes Inspect.
3. It closes the full-size view.
4. It leaves TV mode and full screen.
5. It leaves Comment or Ask mode for Select.
6. It clears the selected panel.
