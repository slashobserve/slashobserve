---
title: Keyboard shortcuts
description: The keys that view, edit and inspect a panel, move the time range, refresh a dashboard and start TV mode, where to see them in the app, and the keys of the logs page and the trace search.
---

A dashboard answers a few keys, the same ones Grafana uses.
Press `?` on a dashboard to see them all in the app.

| Keys | What they do |
| --- | --- |
| `v` | View the panel at full size, or go back to the dashboard. |
| `e` | Edit the panel, when you may edit the dashboard. |
| `i` | Open Inspect for the panel, or close it. |
| `Esc` | Close Inspect, then the full-size view, then TV mode and full screen. |
| `t` then `z` | Zoom the time range out to twice its length. |
| `t` then `←` | Move the time range back by half its length. |
| `t` then `→` | Move the time range forward by half its length. |
| `d` then `r` | Refresh every panel. |
| `d` then `k` | Enter TV mode in full screen. |
| `f` | Go full screen in TV mode opened from a link. |
| `?` | Show the keyboard shortcuts. |
| `⌘O` or `Ctrl+O` | Change how chart hover is shared between panels. |
| `⌘K` or `Ctrl+K` | Open the command palette. |

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
`e` also does nothing while you edit the layout.

## Logs page keys

The logs page answers two keys of its own, described in [Logs and traces](logs-and-traces.md).

| Keys | What they do |
| --- | --- |
| `⌘I` or `Ctrl+I` | Switch between Ask and Query, from anywhere on the page. |
| `⌘Enter` or `Ctrl+Enter` | Run the query, from the query editor. |

## Trace search keys

The trace search answers keys of its own too, described in [Logs and traces](logs-and-traces.md).

| Keys | What they do |
| --- | --- |
| `⌘I` or `Ctrl+I` | Switch between Ask and Query, from anywhere on the page. |
| `Enter` | Ask the question, or run the search from Service, Span name or a duration field. |
| `Shift+Enter` | Add a line to the question. |
