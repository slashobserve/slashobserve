---
title: Keyboard shortcuts
description: The keys that view, edit and inspect a panel, move the time range, refresh a dashboard and start TV mode, where to see them in the app, and the keys of the logs page and the trace search.
---

A dashboard answers a few keys, the same ones Grafana uses.
Press `?` on a dashboard to see them all in the app, or choose **Keyboard shortcuts** in the command palette on any page.

| Keys | What they do |
| --- | --- |
| `v` | View the panel at full size, or go back to the dashboard. |
| `e` | Edit the panel, when you may edit the dashboard. |
| `i` | Open Inspect for the panel, or close it. |
| `a` | Ask the agent about the whole panel in a box on the panel, when you may comment on the dashboard and a model is connected; with no panel it turns Ask mode on, and in Ask mode it turns it off. |
| `c` | Comment on the whole panel in a composer on the panel, when you may comment on the dashboard; with no panel it turns Comment mode on, and in Comment mode it turns it off. |
| `Esc` | Close Inspect, then the full-size view, then TV mode and full screen, then leave Comment or Ask mode for Select, then clear the selected panel. |
| `t` then `z` | Zoom the time range out to twice its length. |
| `t` then `←` | Move the time range back by half its length. |
| `t` then `→` | Move the time range forward by half its length. |
| `d` then `r` | Refresh every panel. |
| `d` then `k` | Enter TV mode in full screen. |
| `f` | Go full screen in TV mode opened from a link. |
| `?` | Show the keyboard shortcuts. |
| `⌘O` or `Ctrl+O` | Change how chart hover is shared between panels. |
| `⌘K` or `Ctrl+K` | Open the command palette, which searches the workspace as you type. |
| `⌘\` or `Ctrl+\` | Collapse or expand the sidebar; under 1200 pixels wide it opens the full sidebar over the page. |

Opening the Share, Activity, Chat or Dashboard JSON pane of a dashboard moves the focus to the pane's heading, on a phone as well, so a screen reader reads it and the keys work inside it.
A pane that was already open when the page loaded does not take the focus.
Escape closes the pane when the focus is inside it or on the button that opened it.
The focus returns to the button that opened the pane, which on a phone is the Controls button, and so does the pane's Close button.
A pane that was already open when the page loaded returns the focus to its own button in the header: Activity for the Activity and Chat panes, Share for Share, and More for Dashboard JSON.

## Moving around the app

The first Tab stop on every page after sign-in is **Skip to the page**, which appears at the top left; press Enter to move the focus to the page's heading and skip the sidebar.
When a page opens because you pressed Enter or followed a link, and the focus was on something that went away with the old page, the focus moves to the new page's heading.
Focus stays where it is when you open a page from the sidebar, and when the new page puts the focus in a field itself.

## Which panel a key acts on

`v`, `e`, `i`, `a` and `c` act on the panel under the pointer.
With no panel under the pointer, they act on the panel that holds the keyboard focus.

Hovering a panel also shows **Ask** and **Comment** buttons in its header, which do what `a` and `c` do.
They fade in under a mouse or trackpad, and on a touch screen and while the keyboard focus is in the panel they are always shown.
A panel narrower than 240 pixels leaves them and its datasource name out of the header so the title keeps the room; the keys and the panel menu still work there.
**Ask** needs the same things as `a`, and the panel menu offers both with the same rule.

## Observation page keys

An observation page answers `c`, `a` and `Esc` for the canvas, as described in [Comment and ask on the blocks](observations.md#comment-and-ask-on-the-blocks).
`c` and `a` open the composer or the ask box on the whole block under the pointer, or on the one that holds the keyboard focus; with no block there, they turn Comment or Ask mode on, and in that mode they turn it off.
`a` skips a panel block whose panel shows no data.
`Esc` hides the **Comment** and **Ask** buttons under selected text first, then closes an open composer or ask box, and then leaves Comment or Ask mode for Select; when you have typed, it asks "Discard your draft?" before your text goes.
The keys do nothing while you move or resize blocks, while you type in a field, and for a person who cannot comment.

## Two-key shortcuts

For a two-key shortcut, press the second key within one second of the first.
Any other key in between cancels it.

## Time range keys

`t z`, `t ←` and `t →` turn the time range into fixed times, so the address of the page keeps the range you moved to.
A range that would end in the future ends now instead and keeps its length.
They do nothing on a dashboard that hides the time picker.

## When keys are ignored

The keys do nothing while you type in a field, while a menu or dialog is open, or while you hold `⌘`, `Ctrl` or `Alt`.
`v`, `e`, `i`, `a` and `c` also act on no panel while you have unsaved layout changes; `c` then only opens the activity pane.
In the full-size view and in TV mode, `c` comments on the panel in the activity pane instead, and with no panel it opens the activity pane.
On a copy of a repeated panel, `c` also comments in the activity pane.

## Logs page keys

The logs page answers keys of its own, described in [Logs and traces](logs-and-traces.md).
`?` opens the same list in a dialog named Keyboard shortcuts.
`/` and `?` do nothing while you type in a field or while a dialog is open.

| Keys | What they do |
| --- | --- |
| `/` | Focus the query. |
| `⌘Enter` or `Ctrl+Enter` | Run the query, from the query editor. |
| `⌘I` or `Ctrl+I` | Switch between Ask and Query, from anywhere on the page. |
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
| `⌘I` or `Ctrl+I` | Switch between Ask and PromQL, from anywhere on the page. |

## Trace search keys

The trace search answers keys of its own too, described in [Logs and traces](logs-and-traces.md).

| Keys | What they do |
| --- | --- |
| `⌘I` or `Ctrl+I` | Switch between Ask and Query, from anywhere on the page. |
| `⌘Enter` or `Ctrl+Enter` | Run the search, from the Service, Span name and duration fields or from the SQL editor. |
| `Enter` | Ask the question, or run the search from Service, Span name or a duration field. |
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
