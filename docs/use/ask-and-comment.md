---
title: Ask and comment on a dashboard
description: Ask the agent in a comment thread, in a private chat or by pointing at a spot on a panel, and comment on a spot, with the Select, Comment and Ask tool bar, the A and C keys, pins, what a click can name, private asks and sharing an answer.
---

A dashboard offers three ways to put a question to the agent and one way to leave a note for the team on a precise spot.
This page describes the canvas: the **Select**, **Comment** and **Ask** tool bar, the spots a click can name, the pins that mark them and the boxes that open there.
The thread and the chat have their own pages, linked below, and an [observation](observations.md#comment-and-ask-on-the-blocks) has the same modes on its blocks.

## Which one to use

| You want to | Use | Who reads it |
| --- | --- | --- |
| Ask the team, and the agent when you want it to answer in front of them | A comment thread, in the **Activity** tab of the side pane or on a spot with Comment mode. | Everyone who can read the thread. |
| Ask about the dashboard in a conversation of your own | The **Chat** tab of the side pane. | You and Admins. |
| Ask about one part of one panel | Ask mode, the **Ask** button of a panel or the `a` key. | You and Admins, until you share the answer into a thread. |

A comment thread asks the agent when its message mentions `@observe` or its "To:" chip reads "To: Observe Agent"; a reply in a thread that already has an agent asks it again, unless the reply mentions a person or goes to everyone.
[Ask the agent from a comment](review.md#ask-the-agent-from-a-comment) describes the rule in full, the answer card and **Revise the dashboard**.
A private chat starts from the **Chat** tab, or from **New chat** in it; [In the dashboard's Chat tab](chat.md#in-the-dashboards-chat-tab) describes it, and the answer to a question never changes the dashboard until someone chooses **Revise the dashboard** on it.

Commenting and asking need Comment on the dashboard's folder, so the tool bar is hidden for a Viewer, and asking needs a model on the server.
Over MCP and the CLI the same spots are [anchors](#anchors-over-mcp-and-the-cli).

## The tool bar

At the bottom of the screen a tool bar offers **Select**, **Comment** and **Ask**, and it is hidden on a phone, in TV mode, while a panel is open in the editor and when you cannot comment, as a Viewer cannot.
**Ask** shows only when a model is connected, and only a data panel can be asked about: a text panel takes comments but no questions.
A line beside the bar, or below it when the row wraps, says what a click does while a mode is on.
**Select** is the default: a click selects a panel, as [Select a panel](web.md#select-a-panel) describes.
**Comment**, or `c` with the pointer on no panel, turns on [Comment mode](#comment-mode), and **Ask**, or `a` with the pointer on no panel, turns on [Ask mode](#ask-mode).
Each mode stays on until you choose **Select**, press `Esc` or press its key again, and both are greyed while you have unsaved layout changes.

With the pointer over a panel, or a panel selected, `c` and `a` do not turn a mode on: they open the composer or the ask box on the whole panel.
The same two actions are **Comment** and **Ask** in the panel's menu, in the toolbar above a selected panel and in two icon buttons in the panel's header, named "Comment on `<title>`" and "Ask about `<title>`", which show when you hover the panel and hide while it is selected or too narrow to hold them.
In the full-size view and TV mode there is no canvas to point at, so they open the dashboard's side pane instead: **Comment** the **Activity** tab, scoped to the panel, and **Ask** the **Chat** tab with a chip naming the panel, so the question is about that panel until you remove the chip.
`Esc` closes Inspect first, then the full-size view, then TV mode and full screen, then leaves Comment or Ask mode for **Select**, and then clears the selected panel; [Keyboard shortcuts](keyboard-shortcuts.md) lists the keys.

## Comment mode

In Comment mode the pointer becomes a dark comment marker whose bottom left tip is the spot a click lands on, the line beside the tool bar reads "Click a spot on a panel to comment on it. Esc returns to Select.", and the part of the panel under the pointer is outlined: the point, series, bar, row, cell or value the click would name, or the whole panel when the panel type cannot name one.
A click opens a small composer at that spot, with chips naming the panel and the part you clicked, and the comment is saved as a thread on that part of the panel.

The chips are short: a time reads "22 Nov 08:54:11 UTC", a range on one day names the day once, as in "Range 22 Nov 08:54:11 to 09:11:32 UTC", and pointing at a chip shows the full text.
The composer and the ask box fit the window on a narrow screen, and the handle in their bottom right corner resizes them: drag it, or focus it and press the arrow keys (Shift for larger steps).
Each box opens at the size you last gave it in this browser, and a double-click on the handle returns it to its default size.

On a time series, a heatmap, a state timeline or a status history, dragging across the time axis instead comments on the range you dragged over, which is outlined while you drag.
A click on the title bar comments on the panel's title, and Tab to the title and Enter, or `c` over a panel, comments on the whole panel.

The composer sends like the Activity box, and its "To:" chip decides whether the agent reads the comment: a comment that mentions no agent goes to the team and starts nothing.
It closes when you post or press `Esc`, and Comment mode stays on for the next comment.
Once you have typed, `Esc`, **Select** or another tool first asks "Discard your draft?", and **Keep editing** keeps the text.
A click outside closes it only while it is empty: once you have typed, it stays open, and a click on another spot or on a pin leaves it as it is.
While the layout has unsaved moves the composer is hidden, and it comes back with your text once the moves are saved or discarded.
In the table view of a panel that is not a table, a click or drag in Comment mode comments on the whole panel.
The copies of a repeated panel take no canvas comments: Comment mode does nothing on a copy, **Comment** or `c` on a copy starts the comment in the activity pane, and pins show only on the first panel.

## What a click can name

Each panel type names the finest part of itself that it draws, and a pin follows that part when the data moves.

| Panel | A click names | The pin follows |
|---|---|---|
| Time series | A point on a line, a line, a moment in time, or a dragged range | The series and the time, or the range |
| Bar chart | A bar, a stacked segment or a category | The category and the series |
| Histogram | A bucket | The bucket's edges |
| Heatmap | A cell, or a dragged range of time | The cell's time step and bucket, or the range |
| Pie chart | A slice | The slice's name |
| Stat, Gauge, Bar gauge | The value under the pointer | The value's series |
| Table | A row, with the value under the pointer shown as a chip, or a column header | The row's identifying cells, or the column |
| State timeline, Status history | A state at a time, a row, or a dragged range of time | The row and the time, or the range |
| Logs | A log line | The line |
| Traces | A span | The span |
| Alert list | An alert, in the list view | The alert's name and labels |
| Canvas | An element | The element, the topmost one where drawings overlap |

A canvas element panned out of view, and a row on another page of a state timeline, count as gone.
The panel types not listed, and the alert list's count view, have nothing finer than the whole panel, so a click there comments on the spot you clicked and the whole panel is outlined.

The stored spot is the thread's anchor, for example a point, a range, a row or a value, with the place of the click on the panel.
A thread or a question with an anchor says it in words, such as "the row payments, authorize_card", wherever it shows, and the agent is told the same sentence.

## Comment pins

A thread on a part of a panel shows as a pin with its author's mark at the spot it names.
A pin follows its data: when the series, bar or row moves, the pin moves with it.
A resolved thread's pin is grey.
A pin on a moment, a point or a range outside the shown time range is not drawn until the range shows it again, and a range partly in view keeps its pin on the part that shows.
A pin on a series hidden in the legend is not drawn either, and while the panel shows an error, is loading or is in its table view only pins on its title are drawn.
When the series, row or cell is no longer on the panel while its time is in view, for example after a filter changed, the pin stays where it was placed, is faded, and its thread says that its data is no longer on the panel.
A table row, log line, span or alert that is on the panel but scrolled out of view, or a table row on another page, points at the bottom edge of the panel, and opening its pin scrolls or pages the panel to it.

Choose a pin, or Tab to it and press Enter, to read the thread with its replies and reply in place.
Pins that would overlap stack into one marker with their count; choose it for a list of those threads, and choose one to read it, with a button back to the list.
Pins show only the threads you can read, to people who can comment, and they are not drawn in print, in a panel image, in TV mode, in the full-size view or in the panel editor.
A pin on a panel that shows no data is drawn faded.

## Ask mode

In Ask mode the pointer carries a teal marker with the slash in place of the dark one, and the line beside the tool bar ("Click a spot on a panel to ask the agent about it. Esc returns to Select.") and the outline work as in Comment mode.
A click opens a small box at that spot with chips naming the panel, the part you clicked, the value under the pointer and the time window, and sends your question to the agent with all of them.
Dragging across the time axis of a panel that takes a range, as in Comment mode, asks about the range you dragged over.
In the table view of a panel that is not a table, a click or drag in Ask mode asks about the whole panel.

The question is a turn of the dashboard's current chat, or of a new chat that becomes the current chat when there is none.
The current chat is the one you last opened in the Chat tab or asked in on the canvas.
Asking on the canvas never opens a closed side pane; when the pane shows Activity or Chat it switches to that chat, and the Chat tab shows it whenever you open it later.
The answer arrives in the same box: "Waiting for a free agent" while the question is queued, "Looking into it" while it runs, and "Revising the dashboard" for a revision.
Then comes the answer card with the **Answer actions** menu, an evidence chip when the answer has evidence panels, and **Revise the dashboard** when the agent suggested a change, which then reads **Revision asked for**.
A follow-up box under it continues the same chat about the same spot.
The same turns show in the Chat tab, and only **Revise the dashboard** changes the dashboard; the line under it says that a revision is visible to everyone who can see the dashboard.
When no model is connected the box says so and cannot send, and while you view an older version it asks you to open the latest one first.

The box closes on `Esc`, and once the question is sent also on its **Close** button, and the focus goes back to where it was; with a typed question it first asks "Discard your draft?".
A click outside closes it only while it is empty: once you have typed a question, it stays open with your text, and a click on another spot or on a pin leaves it as it is.
While the layout has unsaved moves the box is hidden, and it comes back with your text once the moves are saved or discarded.
The copies of a repeated panel take no canvas questions: Ask mode does nothing on a copy, and **Ask** or `a` on a copy opens the dashboard's Chat tab on that panel.

## Private asks and sharing

The ask box says "Only you and Admins can see this.", because a question asked on the canvas is a turn of your own chat.
Nobody else reads the question or the answer, and the answer card's menu has no **Show to everyone**.
**Share to thread** adds your question and the answer to an open thread on the dashboard, where everyone who can read the thread sees it, until you choose **Withdraw**.
[Follow-ups](chat.md#follow-ups) describes who reads a chat and what an Admin sees.

Each spot you asked about shows a teal pin that only you see, named "Your question on" and the spot; choose it, or Tab to it and press Enter, to open the box on that chat again.
Your pins that would overlap stack into one teal marker with their count, "N of your questions here", which lists them; choose one to open its box.
Pins are drawn for your newest 100 questions on the dashboard that name a spot, and a Viewer, who cannot ask, loads none.
An Admin reading your chat does not see your pins, and an answer you share into a thread also shows that thread's pin for everyone.

## Anchors over MCP and the CLI

An agent or a script points at a spot with an anchor: a JSON object with a `kind`, the fields that kind needs and `pos`, the place on the panel as `x` and `y` between 0 and 1.
The kinds are `time`, `range`, `series`, `point`, `row`, `column`, `cell`, `bar`, `segment`, `heat_cell`, `slice`, `value`, `log_line`, `span`, `element`, `text`, `title`, `variable` and `position`, and an anchor may also carry `shown`, the words the person saw, and `window`, the time window they saw.
An anchor over 1536 bytes once encoded is refused.

| Kind | Needs |
| --- | --- |
| `time` | `from` |
| `range` | `from` and `to`, with `to` not before `from` |
| `series` | `series`, a map of label to value |
| `point` | `series` and `from` |
| `row` | `row`, a map of label to value, and optionally `quote` |
| `value` | nothing, and optionally `series` |
| `position`, `title` | nothing |

`start_run` and `create_comment` over MCP take it as `anchor`, and `list_comments`, `list_runs` and `get_run` give it back; `--at` takes it in the CLI, on [`observe` with a prompt](cli.md#send-a-prompt), on `observe investigate` and on [`observe comments add`](cli.md#comments); [MCP](mcp.md) lists the tools.
The CLI also takes short forms such as `row:service=payments,span=authorize_card` and `range:23:50Z..23:58Z` in place of the JSON.

## Related pages

- [Comments, agent requests and versions](review.md) for threads, the answer card, resolving and versions.
- [Chat and follow-ups](chat.md) for the chat and the Chat tab.
- [Observations](observations.md#comment-and-ask-on-the-blocks) for the same modes on blocks.
- [Keyboard shortcuts](keyboard-shortcuts.md) for `a`, `c` and `Esc`.
- [Roles](../teams/roles.md) for who may comment and ask.
