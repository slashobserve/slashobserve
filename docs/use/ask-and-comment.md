---
title: Ask and comment on a dashboard
description: Ask the agent in a comment thread, in a private chat or by pointing at a spot on a panel, and comment on a spot, with the Select, Comment and Ask tool bar, the A and C keys, pins, what a click can name, private asks and sharing an answer.
---

A dashboard offers three ways to put a question to the agent and one way to leave a note for the team on a precise spot.
This page covers the canvas: the **Select**, **Comment** and **Ask** tool bar, the spots a click can name, the pins that mark them and the boxes that open there.
The thread and the chat have their own pages, linked below, and an [observation](observations.md#comment-and-ask-on-the-blocks) has the same modes on its blocks.

Commenting and asking need Comment on the dashboard's folder, so the tool bar is hidden for a Viewer, and asking needs a model on the server.

## Which one to use

| You want to | Use | Who reads it |
| --- | --- | --- |
| Ask the team, and the agent when you want it to answer in front of them | A comment thread, in the **Activity** tab of the side pane or on a spot with Comment mode. | Everyone who can read the thread. |
| Ask about the dashboard in a conversation of your own | The **Chat** tab of the side pane. | You and Admins. |
| Ask about one part of one panel | Ask mode, the **Ask** button of a panel or the `a` key. | You and Admins, until you share the answer into a thread. |

A comment thread asks the agent when its message mentions `@observe` or its "To:" chip reads "To: Observe Agent".
A reply in a thread that already has an agent asks it again, unless the reply mentions a person or goes to everyone.
[Ask the agent from a comment](review.md#ask-the-agent-from-a-comment) describes the rule in full, the answer card and **Revise the dashboard**.

A private chat starts from the **Chat** tab, or from **New chat** in it; see [In the dashboard's Chat tab](chat.md#in-the-dashboards-chat-tab).
The answer to a question never changes the dashboard until someone chooses **Revise the dashboard** on it.

## Comment on a spot

1. Choose **Comment** in the tool bar at the bottom of the screen, or press `c` with the pointer on no panel.
   The line beside the bar reads "Click a spot on a panel to comment on it. Esc returns to Select."
2. Move the pointer over a panel.
   The part under it is outlined: the point, series, bar, row, cell or value the click would name, or the whole panel when the panel type cannot name one.
3. Click.
   A small composer opens at that spot, with chips naming the panel and the part you clicked.
4. Write the comment and post it.
   It is saved as a thread on that part of the panel, and a pin marks the spot.

The composer sends like the Activity box, and its "To:" chip decides whether the agent reads the comment.
A comment that mentions no agent goes to the team and starts nothing.
Comment mode stays on for the next comment until you choose **Select**, press `Esc` or press `c` again.

Other ways to comment:

- Drag across the time axis of a time series, heatmap, state timeline or status history to comment on the range you dragged over.
- Click a panel's title bar to comment on its title.
- With the pointer over a panel or a panel selected, press `c`, choose **Comment** in the panel's menu or use its header button, to comment on the whole panel.
  Tabbing to the title and pressing Enter does the same.
- In the full-size view and TV mode there is no canvas to point at, so **Comment** opens the **Activity** tab of the side pane, scoped to the panel.

### Comment mode

Details of Comment mode:

- The pointer becomes a dark comment marker whose bottom left tip is the spot a click lands on.
- The chips are short: a time reads "22 Nov 08:54:11 UTC", a range on one day names the day once, as in "Range 22 Nov 08:54:11 to 09:11:32 UTC", and pointing at a chip shows the full text.
- The composer closes when you post or press `Esc`.
- Once you have typed, `Esc`, **Select** or another tool first asks "Discard your draft?", and **Keep editing** keeps the text.
- A click outside closes the composer only while it is empty.
  Once you have typed, it stays open, and a click on another spot or on a pin leaves it as it is.
- While the layout has unsaved moves the composer is hidden, and it comes back with your text once the moves are saved or discarded.
- In the table view of a panel that is not a table, a click or drag in Comment mode comments on the whole panel.
- The copies of a repeated panel take no canvas comments.
  Comment mode does nothing on a copy, **Comment** or `c` on a copy starts the comment in the activity pane, and pins show only on the first panel.

## Ask about a spot

1. Choose **Ask** in the tool bar, or press `a` with the pointer on no panel.
2. Click a spot on a data panel.
   A small box opens there with chips naming the panel, the part you clicked, the value under the pointer and the time window.
3. Type the question and send it.
   The agent gets your question with all of those chips.
4. Read the answer in the same box.
   It shows "Waiting for a free agent" while the question is queued, "Looking into it" while it runs and "Revising the dashboard" for a revision.
5. Ask a follow-up in the box under the answer, which continues the same chat about the same spot.

The answer card has the **Answer actions** menu, an evidence chip when the answer has evidence panels, and **Revise the dashboard** when the agent suggested a change, which then reads **Revision asked for**.
Only **Revise the dashboard** changes the dashboard, and the line under it says that a revision is visible to everyone who can see the dashboard.

Other ways to ask:

- Drag across the time axis of a panel that takes a range to ask about the range you dragged over.
- With the pointer over a panel or a panel selected, press `a`, choose **Ask** in the panel's menu or use its header button, to ask about the whole panel.
- In the full-size view and TV mode, **Ask** opens the **Chat** tab with a chip naming the panel, so the question is about that panel until you remove the chip.

### Ask mode

Details of Ask mode:

- The pointer carries a teal marker with the slash in place of the dark one, and the line beside the tool bar reads "Click a spot on a panel to ask the agent about it. Esc returns to Select."
- The question is a turn of the dashboard's current chat, or of a new chat that becomes the current chat when there is none.
  The current chat is the one you last opened in the Chat tab or asked in on the canvas.
- Asking on the canvas never opens a closed side pane.
  When the pane shows Activity or Chat it switches to that chat, and the Chat tab shows it whenever you open it later.
- The same turns show in the Chat tab.
- When no model is connected the box says so and cannot send, and while you view an older version it asks you to open the latest one first.
- The box closes on `Esc`, and once the question is sent also on its **Close** button, and the focus goes back to where it was.
  With a typed question it first asks "Discard your draft?".
- A click outside closes it only while it is empty.
  Once you have typed a question, it stays open with your text, and a click on another spot or on a pin leaves it as it is.
- While the layout has unsaved moves the box is hidden, and it comes back with your text once the moves are saved or discarded.
- In the table view of a panel that is not a table, a click or drag in Ask mode asks about the whole panel.
- The copies of a repeated panel take no canvas questions: Ask mode does nothing on a copy, and **Ask** or `a` on a copy opens the dashboard's Chat tab on that panel.

## Use the tool bar

At the bottom of the screen a tool bar offers **Select**, **Comment** and **Ask**.
A line beside the bar, or below it when the row wraps, says what a click does while a mode is on.

- **Select** is the default: a click selects a panel, as [Select a panel](web.md#select-a-panel) describes.
- **Comment** and **Ask** turn on [Comment mode](#comment-mode) and [Ask mode](#ask-mode).
- Each mode stays on until you choose **Select**, press `Esc` or press its key again.
- Both are greyed while you have unsaved layout changes.
- **Ask** shows only when a model is connected, and only a data panel can be asked about: a text panel takes comments but no questions.
- The bar is hidden on a phone, in TV mode, while a panel is open in the editor and when you cannot comment.

`Esc` closes Inspect first, then the full-size view, then TV mode and full screen, then leaves Comment or Ask mode for **Select**, and then clears the selected panel.
[Keyboard shortcuts](keyboard-shortcuts.md) lists the keys.

The two icon buttons in a panel's header are named "Comment on `<title>`" and "Ask about `<title>`".
They show when you hover the panel, and hide while it is selected or too narrow to hold them.
The panel's menu and the toolbar above a selected panel have the same two actions.

Each box resizes on a narrow screen and by hand:

- The composer and the ask box fit the window on a narrow screen.
- Drag the handle in their bottom right corner to resize, or focus it and press the arrow keys (Shift for larger steps).
- Each box opens at the size you last gave it in this browser, and a double-click on the handle returns it to its default size.

## Comment pins

A thread on a part of a panel shows as a pin with its author's mark at the spot it names.
A pin follows its data: when the series, bar or row moves, the pin moves with it.
A resolved thread's pin is grey.

To read a thread, choose its pin, or Tab to it and press Enter.
The thread opens with its replies, and you reply in place.
Pins that would overlap stack into one marker with their count.
Choose it for a list of those threads, and choose one to read it, with a button back to the list.

Details of when a pin is drawn:

- Pins show only the threads you can read, to people who can comment.
- Pins are not drawn in print, in a panel image, in TV mode, in the full-size view or in the panel editor.
- A pin on a moment, a point or a range outside the shown time range is not drawn until the range shows it again.
  A range partly in view keeps its pin on the part that shows.
- A pin on a series hidden in the legend is not drawn.
  While the panel shows an error, is loading or is in its table view, only pins on its title are drawn.
- When the series, row or cell is no longer on the panel while its time is in view, for example after a filter changed, the pin stays where it was placed and is faded.
  Its thread says that its data is no longer on the panel.
- A table row, log line, span or alert that is on the panel but scrolled out of view, or a table row on another page, points at the bottom edge of the panel.
  Opening its pin scrolls or pages the panel to it.
- A pin on a panel that shows no data is drawn faded.

## Keep a question private or share it

The ask box says "Only you and Admins can see this.", because a question asked on the canvas is a turn of your own chat.
Nobody else reads the question or the answer, and the answer card's menu has no **Show to everyone**.

To share an answer, open the **Answer actions** menu and choose **Share to thread**.
It adds your question and the answer to an open thread on the dashboard, where everyone who can read the thread sees it, until you choose **Withdraw**.
[Follow-ups](chat.md#follow-ups) describes who reads a chat and what an Admin sees.

Each spot you asked about shows a teal pin that only you see, named "Your question on" and the spot.
Choose it, or Tab to it and press Enter, to open the box on that chat again.

Details of your question pins:

- Your pins that would overlap stack into one teal marker with their count, "N of your questions here", which lists them; choose one to open its box.
- Pins are drawn for your newest 100 questions on the dashboard that name a spot.
- A Viewer, who cannot ask, loads none.
- An Admin reading your chat does not see your pins.
- An answer you share into a thread also shows that thread's pin for everyone.

## Reference

### What a click can name

Each panel type names the finest part of itself that it draws, and a pin follows that part when the data moves.

| Panel | A click names | The pin follows |
| --- | --- | --- |
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

### Anchors over MCP and the CLI

An agent or a script points at a spot with an anchor.
It is a JSON object with a `kind`, the fields that kind needs and `pos`, the place on the panel as `x` and `y` between 0 and 1.
An anchor may also carry `shown`, the words the person saw, and `window`, the time window they saw.
An anchor over 1536 bytes once encoded is refused.

The kinds are `time`, `range`, `series`, `point`, `row`, `column`, `cell`, `bar`, `segment`, `heat_cell`, `slice`, `value`, `log_line`, `span`, `element`, `text`, `title`, `variable` and `position`.

| Kind | Needs |
| --- | --- |
| `time` | `from` |
| `range` | `from` and `to`, with `to` not before `from` |
| `series` | `series`, a map of label to value |
| `point` | `series` and `from` |
| `row` | `row`, a map of label to value, and optionally `quote` |
| `value` | nothing, and optionally `series` |
| `position`, `title` | nothing |

`start_run` and `create_comment` over MCP take it as `anchor`, and `list_comments`, `list_runs` and `get_run` give it back; [MCP](mcp.md) lists the tools.
`--at` takes it in the CLI, on [`observe` with a prompt](cli.md#send-a-prompt), on `observe investigate` and on [`observe comments add`](cli.md#comments).
The CLI also takes short forms such as `row:service=payments,span=authorize_card` and `range:23:50Z..23:58Z` in place of the JSON.

## Related pages

- [Comments, agent requests and versions](review.md) for threads, the answer card, resolving and versions.
- [Chat and follow-ups](chat.md) for the chat and the Chat tab.
- [Observations](observations.md#comment-and-ask-on-the-blocks) for the same modes on blocks.
- [Keyboard shortcuts](keyboard-shortcuts.md) for `a`, `c` and `Esc`.
- [Roles](../teams/roles.md) for who may comment and ask.
