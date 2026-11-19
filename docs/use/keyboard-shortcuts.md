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
| `c` | Open the activity pane and put the cursor in its comment box. |
| `Esc` | Close Inspect, then the full-size view, then TV mode and full screen, then go back from the Comment tool to Select, then clear the selected panel. |
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

Escape also closes the Share, Activity, Chat and Dashboard JSON panes of a dashboard.
The focus returns to the button that opened the pane, and so does the pane's Close button.
A pane that was already open when the page loaded returns the focus to its own button in the header: Activity for the Activity and Chat panes, Share for Share, and More for Dashboard JSON.

## Moving around the app

The first Tab stop on every page after sign-in is **Skip to the page**, which appears at the top left; press Enter to move the focus to the page's heading and skip the sidebar.
When a page opens because you pressed Enter or followed a link, and the focus was on something that went away with the old page, the focus moves to the new page's heading.
Focus stays where it is when you open a page from the sidebar, and when the new page puts the focus in a field itself.

## Which panel a key acts on

`v`, `e` and `i` act on the panel under the pointer.
With no panel under the pointer, they act on the panel that holds the keyboard focus.

## Two-key shortcuts

For a two-key shortcut, press the second key within one second of the first.
Any other key in between cancels it.

## Time range keys

`t z`, `t ←` and `t →` turn the time range into fixed times, so the address of the page keeps the range you moved to.
A range that would end in the future ends now instead and keeps its length.
They do nothing on a dashboard that hides the time picker.

## When keys are ignored

The keys do nothing while you type in a field, while a menu or dialog is open, or while you hold `⌘`, `Ctrl` or `Alt`.
`e` also does nothing while you have unsaved layout changes.

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
| `j` and `k` | Move to the next and previous line, while a line has the focus. |
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
