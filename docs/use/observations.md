---
title: Observations
description: Work one question on a page of text, panel and snapshot blocks that people and agents build together, resolve it with a summary, and publish it as an immutable RCA that a person can share read-only.
---

An observation is a page about one question, such as why checkout errors rose at 09:12.
You and your agents add text, live panels and frozen snapshots to it as you learn, and close it with a one-line summary when the question is answered.
You can then publish it as a root cause analysis (RCA) that never changes, and share that read-only.

The page has a title, a time range, a folder and an ordered list of blocks.
Every change is a recorded [activity](activity.md) event, so the log shows who wrote what.

## Work a question on a page

1. Choose **Observations** in the sidebar, then **New observation**.
2. Type a **Title** and pick a shared **Folder** that you can edit, then choose **Create**.
   An observation in My Dashboards cannot be published, so pick a shared folder if you want an RCA.
   The page opens at `/o/<id>`.
3. Choose **Add a block**, then **Text**, write what you know in Markdown, and choose **Add**.
4. Choose **Add a block**, then **Panel**, then **From a dashboard**, and pick a data panel.
   The block is a live copy of the panel on the page's time range.
5. Write in the **Discussion** pane on the right.
   Mention `@observe` to ask the agent a question about the page.
6. Choose **Resolve** and type a one-line summary.
7. Choose **Publish as RCA**, then **Publish revision 1**, to freeze the page.

The same work from a terminal:

```sh
observe o new "Checkout errors rose at 09:12" --folder <folder-id>
observe o add <observation-id> --text "Errors started right after the 09:10 deploy."
observe o resolve <observation-id> --summary "A bad deploy; rolled back at 09:40."
observe o publish <observation-id>
```

`observe o new` prints `Created <title> <url>`, and the `<observation-id>` in the next commands is the last segment of that address.
`observe o publish` prints `Published revision 1 of <title>`.
[MCP](mcp.md) tools and the HTTP API do the same; see [Work on one from MCP and the CLI](#work-on-one-from-mcp-and-the-cli).
Who may do each step depends on the folder; see [Who can do what](#who-can-do-what).

## Open an observation

**Observations** in the sidebar opens the list at `/observations`.
It has one row for each observation with its title, folder, state, who opened it and when it last changed, most recently changed first.

- The **Open**, **Resolved** and **All** tabs choose which observations show, Open by default.
- The search box keeps the observations whose title contains the text.
- **Opened by me** keeps the ones you started.
- The sort menu orders by last change, creation time or title, in either direction.
- **Per page** chooses 20, 50 or 100 rows, and the page numbers with **Previous** and **Next** move through the result.
  The header says which rows show, as "21-40 of 59".

The tab, search, filter, sort, page and page size are in the address, so a link reopens the same view.
The checkbox on a row selects it, and the one in the header selects every row on the page.
With rows selected, **Resolve** closes them all with one summary and **Delete** removes them.
Each counts only the rows you may resolve or delete.

The **...** menu on a row offers **Resolve**, **Reopen** and **Delete** for that row when you may do them.
Under **Observations** in the sidebar, up to five open observations are listed, indented.
Each has a green dot and bold text while it has unread inbox items, and **All observations** follows when more are open.
On the icon rail, a dot on **Observations** shows that some are open.

An observation opens at `/o/<id>`, and the page updates when someone else changes its blocks, its state or the discussion.
An address that is not an observation id, or an observation you cannot see, shows "Observation not found" with a link back to the list and no Retry, because retrying cannot help.
**Share** in the top row copies the address of the page, which only people who can see the observation can open.
For a read-only link for everyone else, see [Share a revision](#share-a-revision).

## Start an observation

**New observation** on the list opens a panel with a **Title** of 1 to 200 characters and a **Folder**.
The folder choices are the folders you can edit.
With none chosen, the observation goes in My Dashboards when you may create there, and otherwise in General.
The folder box names that default before you pick one.
**Create** opens the new page.

Home has a **New observation** tab with the same two fields plus an optional first note.
The command palette's **New observation** opens it.
The note becomes the first text block, and links in it to a logs search, a trace or a dashboard panel of this app become panel blocks; see [Home](home.md#new-observation).

An observation that an [investigation](investigate.md) opens is described under [Investigations write into an observation](#investigations-write-into-an-observation).

## Add blocks

A person who can edit sees **Add a block**, and a person who can only comment sees **Suggest a block**.
The row offers six ways:

- **Text** opens a box for Markdown.
  **Add** or **Suggest** saves it, and `⌘Enter` or `Ctrl+Enter` does the same.
  `Esc` closes the box and keeps what you typed for when you open it again, and **Cancel** closes it and throws the text away.
  A word with no break in it, such as a hash or a long address, wraps inside the block instead of widening the page.
- **Panel** offers **From a dashboard** and **New query**.
  **From a dashboard** opens a side panel where you search for a dashboard, then choose one of its data panels.
  The block is a copy of the panel with its own query.
  It stays live on the page and follows the page's range, or the last hour when the observation has none.
  **New query** opens the panel editor on an empty panel with the first datasource, and is disabled when there is no datasource.
  **Add**, or **Suggest** for a commenter, saves it as a panel block that follows the page's range; see [Edit a panel block](#edit-a-panel-block).
- **Logs** opens the logs page with this observation preselected, so you can add a search from there; see [Add to an observation from a panel or from Logs](#add-to-an-observation-from-a-panel-or-from-logs).
- **Traces** opens the trace search with this observation preselected, so you can add a search from there; see [Other places that add](#other-places-that-add).
- **Metrics** opens the metrics page with this observation preselected, so you can add a query from there; see [Other places that add](#other-places-that-add).
- **Snapshot** lists the live panel blocks of the page and is disabled when there is none.
  Choosing one freezes that panel's data into a snapshot block placed right after it.

A panel block can also be frozen from its own menu with **Freeze as snapshot**.
A person who can only comment suggests that snapshot, as with any other block.

### Suggest a block

An editor's block goes on the page as `live`.
A commenter's block is `requested`.
It shows with a dashed border and the caption "`<name>` suggests this block", or "Observe Agent suggests this block" when an agent run wrote it.

**Keep** makes it live and **Discard** removes it.
Both show to an editor, and only **Discard** shows to the person who asked for the block.
When the block above a suggestion is a live panel block, the row also says that keeping it adds this block next to the one above and replaces nothing.

Resolving does not close the page.
A resolved observation still takes blocks from editors and suggestions from commenters, and the row says "This observation is resolved" so nobody is surprised.
An editor who does not want more reopens it or discards the suggestion.

### What a panel block shows

A panel block's caption names its datasource and its window: "Last hour, live", the page's range, or "Pinned to" the block's own.
A snapshot's caption says "Frozen" with the time range of its data.

A panel block is drawn as a dashboard panel.
Its panel menu has **View as table**, **Inspect**, **Download CSV** and **Save as PNG**, plus **Open in Logs** or **Open in Traces** on a logs or traces panel.
**Inspect** opens the panel's data and query in the right pane in place of the discussion, and closing it brings the discussion back.

A block that carries variables shows a picker above the panel for each one with stored options, starting at the values it was added with.
A choice changes only what you see and is not saved.
A textbox, ad hoc filter or group by variable shows its stored value and cannot be changed on the block.

## Edit a panel block

A panel block's panel menu has **Edit panel** for an editor and **Suggest an edit** for a commenter.
It opens the same panel editor as a dashboard panel, in the right pane in place of the discussion.
You get the query, the display and the JSON view, with a preview of the draft over the block's window and with the block's variables.
The preview runs with exactly the access the block's own data has.
It needs comment access to the observation and the datasource's access, and an Admins-only datasource runs only in an Admins-only folder.

Nothing changes until you save.
The editor asks first when the draft has changes and you press `Esc`, choose **Close panel editor**, leave the page, open another block, or choose **New query** or **From a dashboard**.
**Keep editing** keeps the editor and its draft.

**Save** changes the block in place and keeps its width, height and window.
A commenter's **Suggest edit** leaves the block as it is and adds the edited copy right after it as a requested block.
An editor keeps or discards it like any other suggestion.
Keeping it adds a second block, so an editor who wants only the edit then deletes the original.

When someone changed the block since you opened it, the editor says so and keeps your draft.
When someone deletes the block while your draft has changes, the editor says so and keeps the draft.
**Add as a new block**, or **Suggest as a new block** for a commenter, puts it back where the block was, and **Discard** throws it away.
A snapshot and a published RCA revision are frozen, so neither offers the editor.

Over [MCP](mcp.md), `update_block` changes a panel block's `body`.
On the [CLI](cli.md#observations), `observe o edit-block <block-id> --version <n> --panel-file <path>` does the same.
The `--version` is required: it is the block's version from `observe o show`.

## Move, resize and delete blocks

Hover a block, or focus it, to show its **Block actions** menu.
It offers:

- **Move up** and **Move down**, which swap the block with its neighbour.
- A **Make ... width** item for each other width.
- **Reset height** on a panel or snapshot block with its own height.
- **Freeze as snapshot** on a live panel block.
- **Delete block** on a live block.

Blocks sit side by side in rows of three thirds when the page is wide enough: three thirds, a third and two thirds, or two halves.
A block that does not fit the rest of a row starts the next one.

**Delete block** on a block an agent wrote asks "Delete this block?" first, because the block cannot be brought back.
That covers every run block and any block added by the built-in agent or over MCP.

**Edit** on a text block opens its Markdown, and **Save** or `⌘Enter` keeps the change.
If someone changed the block first, the editor says so and **Use their version** replaces your text with theirs.
Text that is empty is refused; delete the block instead.

Editing, moving, resizing and deleting need edit access to the folder.
An agent changes one block with `update_block` and its `position`, `width` or `height`, and arranges the whole page with `arrange_blocks`, which needs no version.
On the CLI, `observe o arrange <observation-id> <block-id>[:<width>[:<height>]]...` does the same.

### Arrange by hand

A person with edit access arranges the page without a menu.

- Drag a block by the grip at its top left corner to drop it before or after another block.
- Drag the corner at its bottom right to snap its width to a third, a half, two thirds or the full page.
  On a panel or snapshot block, the same drag sets its height in steps of 20 pixels.
- Focus the grip and press the arrow keys to move the block.
- Press **Shift** with the left or right arrow to change its width.
- Press **Shift** with the up or down arrow to change a panel's height by 40 pixels.
  Each change is announced.

Below 640 pixels of block list width the page shows one column, so the width does not apply.
The right dock, when open, can bring the list below that.

Arranging never conflicts with a text edit that is open on the same block, because a block's version counts its content and not its place or size.
When two people arrange at once, the last arrangement wins.
A person who can only comment cannot arrange.

## Set the time range

The page's time range sits in the top row, and every panel block follows it unless the block is pinned.
With no range stored it reads **Last hour, live**.

A person who can edit chooses another range.
A quick range such as **Last 6 hours** is stored as the absolute window it means at that moment, so everyone reads the same data.
**Last hour, live** clears it.
Anyone else sees the range without the picker.

### Pin a block to its own window

A panel block's panel menu offers **Pin to this window**, which keeps the window the block shows now, and **Pin to a window**, which opens a time range picker for that block.
Dragging across a time series chart pins the block to the dragged range.
A pinned block keeps its window when the page's range changes, and **Follow the page** releases it.
Pinning needs edit access to the folder.

A block added from a dashboard panel's menu arrives pinned to the window it was taken from.
One chosen with **Panel**, then **From a dashboard**, follows the page.

Over MCP, `update_observation` sets the page's range, and `update_block` sets a block's own `from` and `to` or releases them with `follow_page: true`.
On the CLI, `observe o edit --from <time> --to <time>` or `--clear-range` sets the page's range, and `observe o edit-block <block-id> --version <n> --follow-page` releases a block.

## Rename, resolve, reopen and move

A person who can edit changes the title by choosing it, typing, and pressing `Enter`, or `Esc` to leave it as it was.
An observation that is the one observation made from a chat with no dashboard shares its title with that chat, so renaming one renames the other; see [Chat](chat.md#one-title-for-a-chat-and-its-observation).

**Resolve** closes the question with a one-line summary of at most 500 characters that says what was found.
The summary shows under the title and in the list.
A resolved observation stays readable, a person can reopen it, and it can still be published.

**Reopen** turns a resolved observation back into an open one.
The summary stays stored, so the next **Resolve** starts from it.
The page and a published RCA show it only while the observation is resolved.
`observe o show` prints it as `Last summary`, and over the API and MCP it is still in `summary` beside `state: open`.

**More actions**, then **Move to folder**, opens a panel that searches the shared folders you can edit.
My Dashboards is not a choice.
Resolving, reopening and moving need edit access, and moving needs it on the target folder too.
The state line also shows who started the observation and when, and links to its folder.

## Pin it to a dashboard or make a dashboard from it

**More actions** has **Pin to dashboard** when the observation came from a run about a dashboard and you have edit access to that dashboard.
It adds an annotation at the end of the observation's range with the title and a link back.
Once it is pinned the item reads **Pinned to dashboard**, and pinning again over the API or MCP returns the same annotation.
Over the API the observation's `can_pin` says whether you may, and a pin to a dashboard you cannot open answers 404 "dashboard not found".

**Make a dashboard**, in **More actions** and in the actions of a live panel block, saves the live panel blocks, or that one block, as a new dashboard in My Dashboards and opens it.
Suggested blocks, text blocks and snapshots are left out.
The dashboard keeps the variables each block carries and uses the observation's range, or the first panel's when it has none.
Every query is run again first.

Making it again from the same observation or block opens the one you made first, unless you can no longer see that dashboard, and then a new one is made.
Over the API, `new: true` makes another.
When the observation came from your own chat and that chat is about nothing else, the chat becomes about the new dashboard and opens in its Chat tab.

## Discuss beside the blocks

The discussion is the observation's one comment thread.
It sits in a pane on the right of the page at full height on a wide window, and below the blocks on a narrow one.
Drag the pane's left edge to make it wider or narrower; it shares its width with a dashboard's activity pane.
The **Discussion** button in the top row, which shows the number of messages, hides and shows the pane, and **Close panel** in the pane hides it.

The pane lists the messages together with the changes to the page, newest at the bottom.
They are grouped under a heading for each day, and each entry shows its author's mark and the time of day.
The pane reads up to 500 of the newest such events.
The changes it lists are:

- Who started the observation, with the title it was created with and its folder.
- Who added or suggested each block.
  A panel block also names its datasource and says it is live, and a snapshot names the panel it froze.
- Who changed or removed a block.
- Who renamed the observation, from which title to which.
- Who moved, resolved, reopened, published or withdrew a revision of it.

A removed block is drawn without a name, because the block is gone.
A block's name scrolls the page to that block.

### Filter the pane

Two lists under the heading filter what the pane shows.
**Show** narrows it to messages only or changes only.
**Block** narrows it to one block: that block's messages, the changes to it, and the replies that quote those messages.
When nothing matches, the pane says so and **Clear filters** shows everything again.

### Write a message

A person who can comment writes messages in the box at its foot.
A person who can only view the observation sees **Request comment access** there instead, which asks the people who manage the folder; see [Ask for comment access](../teams/folders.md#ask-for-comment-access).

The control above the box says where the message goes: **On the whole page**, or **On** one block, which is the block chosen in **Block** until you pick another.
A message written on a block shows "On" and the block's name with **Show**, which scrolls the page to the block.
A message written on a block that has since been deleted shows nothing extra.

**Reply** on a message puts "Replying to" and its author above the box, with a button that removes it.
The reply then shows the quoted message above its text and reads "replied to" and the quoted author.

Each message also has **Add to page** for an editor, or **Suggest for page** for a commenter, which puts the message's text on the page as a text block.
On the built-in agent's answer it puts the finding there, its title and text as a text block followed by its evidence panels, and a reply of the built-in agent that carries no answer has no such action.
The message stays in the discussion as well.

Typing `@` opens the picker of people and agents, as in [Edit a comment and mention people](review.md#edit-a-comment-and-mention-people).
A person can delete a message as described in [Delete a comment](review.md#delete-a-comment).
Deleting the first message deletes the whole discussion only while nobody else has written in it.

### Ask an agent from the discussion

A message that mentions one agent asks that agent a read-only question about this observation.
The agent can be `@observe`, one of your connected agents or a service account that takes runs.
The message needs Comment on the observation.
On a message about the whole page, the agent is told which observation it was asked on, so "this" means that page, and it reads the page first.

A request to summarize, explain or rewrite the page, such as "summarize this for my manager", is answered from the page and the discussion with no new queries.

The run writes nothing on the page, for an Editor as for a Commenter.
When it ends, the server posts one status reply in the discussion, whose text is "Answered." when the run saved an answer.
The web page shows the answer card in its place, and everyone who can read the observation reads that answer in the discussion.
The reply is not the model's text, and it never resolves the discussion.
The web app sends your browser's time zone with each message, so the agent writes the times in its answer in that zone with its label, such as 19:52 IST.
Over the API a message can name a zone with `time_zone`, and a zone the server does not know is ignored and never stops the message; the CLI and MCP name none, so times are written in UTC.
[Give an agent a run](agents.md#give-an-agent-a-run) describes who you can mention and what happens to the run.

### Messages over the API

A message can be about one block of the page or about the whole observation, and a reply can quote the message it answers.
Over the API, MCP and the CLI, a message carries `block_id` and `reply_to`.
Both are null when the message is about the whole page and quotes nothing.
`reply_to` holds the quoted message's `id`, its `actor` and an `excerpt` of its first 200 characters.
The MCP comment tools return `reply_to` with the author as `author`, the label the web app shows.

When a message that asks an agent names a block, the agent is pointed at that block, even when the thread was started on another one.
Deleting a block keeps the messages written about it, which are then about the whole observation.
Deleting a quoted message leaves the reply with no quote.
A block that belongs to another observation answers 422 `invalid_block`, and a quoted message that is not in the discussion answers 422 `invalid_reply_to`.
Editing a message changes neither.

## Comment and ask on the blocks

For a person who can comment on the observation, the page has the same tool bar at the bottom of the screen as a dashboard, with **Select**, **Comment** and **Ask**.
It is hidden on a phone, and **Ask** shows only when a model is connected.

On a live panel block, Comment mode and Ask mode work as on a dashboard panel.
A click names the spot under the pointer, a drag across a time series names the range, and pins mark the threads and your own questions.
See [Comment pins](ask-and-comment.md#comment-pins) and [Ask mode](ask-and-comment.md#ask-mode).
On a snapshot or a run block, a click in either mode comments on or asks about the whole block.

`c` or `a` with the pointer over a block, or with a block focused, opens the composer or the ask box on the whole block.
Tab reaches text, snapshot and run blocks.
A panel block's menu and the **Comment** and **Ask** buttons in its header do the same.
A comment on the whole block goes into the discussion as a message on that block.
A panel block that shows no data, such as a text panel, takes comments but no questions, as on a dashboard.

### Comment on text

Selecting text in a text block shows **Comment** and **Ask** under the selection, and `Esc` hides them.
In Comment or Ask mode, selecting text opens the composer or the ask box at once.

The thread or the question points at the text you selected, up to its first 200 characters.
Its pin sits on that text wherever it moves in the block, and when the same words appear more than once, the pin takes the place nearest where you selected them.
When the block no longer holds that text, the pin is faded in the block's top right corner and its thread says the text is no longer in the block.

### Questions on the page

A question asked on the page is private.
Its box says "Only you and Admins can see this.", and the agent reads data and answers without writing anything on the page.
Only those private questions get your pins; an investigation that writes into the observation is answered on the page instead.
The questions you ask during one visit to the page are turns of one chat, which the [chat list](chat.md) keeps.

While you move or resize blocks, the pins stay but nothing takes a click or a selection, and an open composer is hidden.
It comes back with your text when the change is saved, as it does after you edit the block it is on.
If the block goes away while you are writing, your text moves to a box in the corner of the page with **Copy text** and **Discard**, and the rest of the page takes comments again.
Once you have typed, `Esc`, **Select** or another tool first asks "Discard your draft?".

A requested block takes no comments or questions until it is kept.
Over the API, MCP and the CLI, a question names a whole block with `block_id` and no anchor, as in [`observe investigate`](cli.md) with `--block` and `--observation`.

## Add to an observation from a panel or from Logs

**Add to observation** in a dashboard panel's menu copies that panel into an observation.
The item shows for every data panel while you have no unsaved layout changes.

1. Open the panel's menu and choose **Add to observation**.
2. Under **Where**, choose **An open observation** from the open ones you can see, or **A new observation**.
   A new one asks for a title that starts as the panel's title and a folder, as in [Start an observation](#start-an-observation).
   With no open observation, only the new one is offered.
3. Choose **Add**.
   You see "Added to `<title>`", or "Suggested on `<title>`" for a person who can only comment, with **Open** to go there.

If the new observation was created but the panel could not be added, the message says so, and **Add** tries again on the same observation.
The block takes the time range in the dashboard's address as absolute times, and without one it follows the observation's range, or the last hour.
A panel's `repeat` setting is dropped.

### Variables in the copied panel

A panel that uses dashboard variables, a `$name` or `${name}` in a query or in the panel's datasource, is copied with the variables it uses.
It also keeps the values selected at that moment, so the block shows what you saw.
A multi-value selection and **All** expand in the query as they do on the dashboard, because the block keeps the variable's definition.
The block says "with `service=checkout`" under its title, and changing the dashboard's selection later does not change the block.

These do not count as variables:

- The built-in names, which start with two underscores, such as `$__rate_interval`, `$__interval` and `${__user.name}`, because a block runs them as it runs any query.
- A `$` before a digit or on its own.
- A variable in the panel's title.

### Other places that add

- **Add to observation** on the [logs page](logs-and-traces.md#add-to-an-observation) adds the search you ran as a logs panel named Logs over the range you are viewing.
- **Add to observation** on the [metrics page](metrics.md#add-to-a-dashboard-or-an-observation) adds a time series panel named Metrics, or a table panel in the table mode.
- **Add to observation** on the [trace search](logs-and-traces.md#trace-search) and in the [trace view](logs-and-traces.md#trace-view) adds a traces panel: the search you ran, or the trace you are looking at, over the time range the page shows.

## Delete an observation

Deleting removes the observation for everyone, with its blocks, snapshots, discussion and RCA revisions, and the public links of those revisions stop working.
It cannot be undone.
The person who started the observation and can edit it may delete it, and so may anyone who manages its folder.

- In the web app, choose **More actions**, then **Delete observation**, on the page, or **Delete** on the list.
  It asks you to confirm first.
- Over HTTP, call `DELETE /api/v1/observations/{id}`, which answers 204.
- In the CLI, run `observe o delete <observation-id> --yes`.

A pin of the observation on a dashboard is taken off too, when you started that pin or are an Admin.
Otherwise the marker stays and its link shows "Observation not found".
While an investigation on the observation is queued or running, the delete is refused with `run_in_progress`.
Delete it once the run ends or has been stopped.
The [activity](activity.md) log keeps an `observation.deleted` event with the title.

## Publish as RCA

Publishing turns the page into a root cause analysis that never changes.
Each publish adds the next numbered revision, so an RCA is a history of what the team believed as the investigation went on.
Publishing shares the result with nobody outside the folder; [Share a revision](#share-a-revision) does that.

An agent can publish with `publish_rca` and a person with `observe o publish <observation-id>`, which prints `Published revision N of <title>`.
Publishing needs edit access to the observation's folder.

### Publish in the web app

1. Choose **Publish as RCA** in the top row of the page.
   A panel opens, and its address carries `?rca=1` so you can link to it.
2. Choose **Publish revision N**.
   The page freezes as the next revision and says "Published revision N".

**Revisions** in the panel lists each revision with who published it and when.
A person who cannot edit sees **RCA revisions** instead, and only when a revision exists, with the note "You can view revisions. Editors publish them."

Choosing a revision shows **Open revision N**, its public links and, for a person who can edit, **Withdraw**.
**Open revision N** shows the stored revision inside the app at `/o/<observation-id>/rca/<revision>`, for anyone who can see the observation, whether or not it has a public link.
The page is the same frozen text and panels a public link shows, with the name of who published it, and a link back to the observation.

An observation in My Dashboards must be moved to a shared folder first, as a dashboard must before it is shared.
Such an observation does not list `publish` in its `allowed_actions`, and the web page offers **Publish as RCA** with a **Move to folder** button instead of a publish button.

### What a revision freezes

- The title, summary, state and time range of the observation when it was published.
- The `live` text blocks, as Markdown, in page order.
- Every `snapshot` block with its frozen panel and data, and a new snapshot of every `live` panel block, taken in the same step.
- Each block's width, and the height of a panel or snapshot block that was set by hand.
  A revision published before heights were kept has none, so its panels show at the default height for their width.

A revision leaves out requested blocks, run blocks, and the query text of every panel.
A published copy shows what the data was and never how to query your datasources.

A published page lays its blocks out in rows of thirds like the observation page does, from 640 pixels of width up and in one column below that.
A block that sits alone at half width takes half the row.
Revisions published before this packing existed show such a block at half width too, where they used to give it the whole row.

### When a publish is refused

The panel refuses a publish with one of these reasons, and the API answers with the code:

| Reason | Code |
| --- | --- |
| A panel could not be frozen | `panel_failed`, 422 |
| The page changed while it was being frozen | `observation_changed`, 409 |
| The observation has 50 revisions | `rca_limit`, 409 |
| The page has not changed since the latest revision | `rca_unchanged`, 409 |
| A panel has more than 2 MiB of data | `snapshot_too_large`, 422 |
| The observation is in My Dashboards | `personal_folder`, 409 |

Details of these refusals:

- **Panel could not be frozen.**
  Nothing is published, and the answer names the block.
  The web message names the panel and why, and **Show block** closes the panel and scrolls to that block so you can fix or remove it.
  Publish again afterwards.
- **Page changed meanwhile.**
  Publishing again takes the new page.
- **No change since the latest revision.**
  A page with no live panel block whose title, summary, state, range and blocks equal its latest revision is refused.
  Pressing Publish twice, or an agent retrying, never makes a copy.
  A page with a live panel block always publishes, because each publish takes new data.

### Withdraw a revision

A revision never changes, so a slip in one cannot be edited, but the revision can be withdrawn.
**Withdraw** asks for confirmation and then deletes the revision and its frozen panels for everyone.
It needs Edit on the observation's folder and the same states as publishing.
The activity log keeps `observation.rca_withdrawn` with the title and the revision number.

A revision that still has a live public link answers `rca_shared` (409).
A public link is a person's decision, so a person with Manage on the folder revokes its links first and then withdraws the revision.
Links that have already expired go with the revision.
Withdrawing the newest revision frees its number, so the next publish takes it again.

A person withdraws with `observe o withdraw <observation-id> <revision> --yes` or `DELETE /api/v1/observations/{id}/rcas/{revision}`.
No MCP tool deletes anything, so an agent cannot withdraw a revision.

## Share a revision

A public link shows one revision of an RCA read-only to anyone who has it, with no sign-in.
Making one is a person's decision, as it is for a dashboard (see [Public links](public-links.md)).
No agent or MCP tool creates, lists or revokes an RCA link, and `observe o` has no verb for it.

### Share a revision in the web app

1. Choose **Publish as RCA**, then choose a revision.
   You see **Public links, revision N**.
2. Pick **Expires**: No expiry, 1 day, 7 days, 30 days or On a date.
   **On a date** stops the link at the end of that day in your browser's time zone, and it must be today or later.
3. Choose **Create link**.
4. Choose **Copy**.
   The full address shows once in a box, with the note "Copy it now. It is not shown again."

The list below it shows each link's first characters, whether it is active or expired and when it expires, who made it, and when it was last viewed.
**Copy link** stays on a link made since you chose the revision.
**Revoke** asks "Revoke this public link?" and then stops the link at once.

Creating, listing and revoking links need Manage on the folder.
The panel shows the server's reason when you lack it, when the observation is in My Dashboards, when the revision holds data that depends on the viewer, or when the observation already has 20 links.

### Share a revision over HTTP

A person with Manage on the folder creates a link with `POST /api/v1/observations/{id}/rcas/{revision}/public-links`, and revokes it with `DELETE /api/v1/observations/{id}/public-links/{link_id}`.
The link's address is `<public url>/rca/<token>`, and the token is shown once, when the link is created.
A link can expire at a time you choose, and it stops working then; revoking it stops it at once.
An observation has at most 20 links across all its revisions, and one more answers 409 `too_many_links`.

A link serves the stored revision only, never the live observation, a live panel or a query.
It shows who published it by name and by no id.

An observation in My Dashboards cannot have a link, and the request answers 403 `forbidden_role`.
One in a folder only Admins can read is refused with `409 admins_only_folder`.
That answer says to move it to a folder that is not Only Admins, or it names the Admins-only datasource the observation reads, also in a `datasource` field, when it cannot leave.
A revision with a snapshot of data that depends on who is viewing, from `${__user.*}` in a query, cannot have a link, because nobody could read it on a public address.
The request answers 422 `user_scoped`.

### The public page

`<public url>/rca/<token>` opens with no sign-in and shows the SlashObserve name, then:

- The title, and a line "Revision N, published by `<name>`, `<date and time>` `<time zone>`", in the time zone of the reader's browser.
- "Resolved", or "Open at publish time", and the summary when there is one.
- The blocks in page order: text as Markdown, and each panel as its frozen data under the panel's title.
  A panel's caption is "Frozen" with the observation's time range and its time zone when it had one, and otherwise "Frozen when published".
- Blocks side by side in rows of thirds when the page is wide enough.

It shows no query, discussion, comment, activity, live panel or other revision, and it runs no query against a datasource.
A panel of a type that cannot show on a public link, or that the page cannot draw yet, shows "This panel type is not shown yet."
An expired, revoked or unknown token shows "This link has expired or was turned off."

When a newer revision has been published since, the page says "Revision N is newer, so this one may be out of date", with N only and no link, because the reader cannot open another revision.
`GET /api/v1/public/{token}/rca` carries the number as `superseded_by`, null on the newest.
The same notice shows in the app, with **Open revision N** beside it.
A person who finds a link to a superseded revision can revoke it, or withdraw the revision once its links are revoked.

## Investigations write into an observation

An [investigation](investigate.md) opens an observation when it is queued, and its run shows there as a `run` block while the agent works.
`POST /api/v1/observe` with `kind` `investigate`, and `observe investigate`, both do this.

`POST /api/v1/observe` takes an optional `observation_id` for an investigation, the observation to write into, and `observe investigate` takes it as `--observation`.
It must be an observation you can see, or the answer is 404 `not_found`.
It must also be one you may comment on, or the answer is 403 `forbidden_role`.
An investigation that names an observation with 100 blocks already gets no run block and no `observation_id`, because the page has no room.

### The observation a run creates

Without `observation_id`, the server makes a new observation:

- Its title is the first non-empty line of the prompt without `@observe`, cut at a word to about 60 characters with "...", or "Investigation" when no line is left or the first line has fewer than 3 characters.
- Its range is the window of the investigation, and it has none when the investigation was given no `from` and `to`.
- It goes in the folder of the dashboard the investigation is about when you can edit that folder.
  Otherwise it goes in your My Dashboards when you may create there, and otherwise in General when you may create there.
- When no such folder exists, the investigation still runs, writes no observation, and its `observation_id` is null.

### What a saved finding writes

When the agent saves its finding, the finding and its conclusion are written in one transaction, and a failure leaves neither.
The conclusion becomes a text block that starts with `## <title>` and goes on with the finding's Markdown, cut to the 8,000 characters the server accepts in a text block.
One panel block follows for each panel of its evidence, showing the finding's own window.

The run block then cites these blocks, and a run adds them once, so saving the finding again adds nothing.
The page names each cited block by the finding's heading or the panel's title.
An evidence panel that is not valid, or that does not fit in the page's limits, is left out, and the text block is still written.
A finding saved by a run that has an observation takes its own id.
One saved with no run observation takes the id of the observation it creates.
Its `observation_id` names the observation either way.

A block is `live` when you can edit the observation's folder at the moment it is written, and `requested` when you can only comment.
When the run's observation is gone, hidden from you, or in a folder you cannot comment in, saving the finding fails and writes nothing, instead of opening a second observation.
A server error while the observation is opened queues nothing and answers the request with that error.

### Where the answer goes

Where the answer goes depends on what the question was asked on.

- A question asked on one block puts the run block, the text block and the evidence panels directly under that block, in that order.
  The blocks below move down by that many places.
- A question on the whole page puts them at the end of the page.
  So does one asked on a block that has been deleted since.

The new blocks are full width, and the order, widths and heights you arranged for the other blocks stay as they are.
The same rule holds when the agent writes its answer over MCP, when you save an answer from a chat onto the page, and when a commenter asks, whose blocks arrive as `requested` blocks in the same place.

### How a run block looks

A run block that finished normally shows the outcome and **View the run** on one line, and **Run details** unfolds the steps and the blocks it cites.
A run that failed or was stopped stays unfolded, so its error and the retry control show without a click.
A text block written by a run that is longer than 600 characters shows its heading and first paragraph, and **The rest of the answer** shows or hides the remainder.
Folding is only how the page is drawn, so the stored block, a published revision and a snapshot always hold the whole text.

### Runs over the API and the CLI

`observation_id` on a run is the observation it writes into, and `GET /api/v1/observe/{id}` and `observe investigate --json` carry it.
`observe investigate --observation <id>` names the observation to write into, and without it a terminal starts a new one.
Starting an investigation again, with `POST /api/v1/observe/{id}/retry` or `observe retry`, writes into the observation of the run when you may comment on it.
Otherwise, or when that run had none, the rules for a new observation apply to you.

## Work on one from MCP and the CLI

[MCP](mcp.md) clients use these tools:

- `create_observation`, `get_observation`, `list_observations`, `update_observation`, `move_observation`
- `resolve_observation`, `reopen_observation`
- `add_block`, `update_block`, `arrange_blocks`, `keep_block`, `discard_block`
- `publish_rca`, `list_rcas`, `get_rca`
- `pin_observation`, `make_dashboard`

The [CLI](cli.md#observations) has the same work as `observe o new`, `show`, `list`, `add`, `edit`, `move`, `resolve`, `reopen`, `keep`, `discard`, `edit-block`, `arrange`, `publish`, `rcas`, `pin` and `dashboard`.
It also has `observe o delete`, `remove-block` and `withdraw`, which no MCP tool matches because no MCP tool deletes anything.
Both reach the HTTP API under `/api/v1/observations`, and a person works on the page in the [web app](#work-a-question-on-a-page).

Every observation the HTTP API and the MCP observation tools return carries `message_count`, how many messages its discussion holds, replies included, and 0 before anyone has written one.
To read what changed on one observation, filter the [activity](activity.md) with `observation_id`.
Searching the workspace finds observations too; see [Search](search.md).

### List observations over HTTP

`GET /api/v1/observations` takes these query parameters:

| Parameter | Meaning |
| --- | --- |
| `folder_id` | Observations in one folder. |
| `state` | `open` or `resolved`. |
| `q` | A title that contains the text, ignoring case, at most 200 characters. |
| `created_by` | `me` keeps the ones you started. |
| `sort` | `updated` (the default), `created` or `title`. |
| `order` | `asc` or `desc`. |
| `limit` | 1 to 100, 20 by default. |
| `cursor` | The `next_cursor` of the last page. |
| `offset` | At most 100000, and the answer also has `total`. |

`title` sorts ascending by default, and the other sorts descending.
The list pages in one of two ways: `cursor` and `next_cursor` for the default order, or `offset`, which also answers `total`, the number of observations that match.
A cursor cannot be combined with `offset` or with another sort.

## Reference

### Blocks

A block is one of these kinds:

| Kind | What it holds |
| --- | --- |
| `text` | Markdown of at most 8,000 characters. |
| `panel` | A data panel in ObserveSpec form with its own query. It runs live, over the observation's range or its own `from` and `to`. |
| `snapshot` | The data of a panel block frozen at one moment, so the page keeps showing what you saw. |
| `run` | The agent run that is investigating, added by the server. |

Blocks keep the order of their `position`.
A person who can only comment sees positions that count only the blocks they can see, and a suggestion they place at one of those positions lands there once an editor keeps it.
Each block has a `version`, and a change or a keep that names an older one is refused with `version_conflict`.
Two people never overwrite each other unseen.

A block's `width` is `third`, `half`, `two_thirds` or `full`.
A block added without one is `full`.

### Limits and codes

| What | Limit | Refused with |
| --- | --- | --- |
| Title | 1 to 200 characters | `invalid_title` |
| Summary | 500 characters | `invalid_summary` |
| Time range | `from` before `to`, given together, at most 400 days | `invalid_range` |
| Text block | 8,000 characters, and no HTML from an agent or MCP client | `invalid_block` |
| Block height | 120 to 960 pixels, on a panel or snapshot block only | `invalid_block` |
| Variables on a panel block | 20 | `invalid_block` |
| Blocks on a page | 100 | `too_many_blocks` |
| Panel and snapshot blocks together | 24 | `too_many_panels` |
| Frozen data | 2 MiB | `snapshot_too_large` |
| Panel size | 63 KiB | `invalid_panel` |
| RCA revisions | 50 | `rca_limit` |
| RCA public links | 20 across all revisions | `too_many_links`, 409 |

These codes also appear:

| Code | Meaning |
| --- | --- |
| `version_conflict` | A change or keep names an older block version. The 409 answer carries the current block as `current`. |
| `kind_not_addable` | A `run` block cannot be added through the API. |
| `body_not_editable` | Only the body of a text or panel block can be changed. |
| `variable_missing` | A panel uses a variable and no value was given in `variable_values`. |
| `observation_has_no_dashboard` | A pin was asked for an observation that came from no dashboard. |
| `observation_has_no_panels` | **Make a dashboard** found no live panel block. |
| `personal_folder` | The observation cannot move into My Dashboards, or cannot publish from it. |
| `run_in_progress` | An investigation is queued or running on the observation. |
| `rca_shared` | A revision still has a live public link. |

A height for a text block is refused with `invalid_block`.
A panel must be a valid data panel.
One that an agent, an MCP client or a service-account key (the [CLI](cli.md) and scripts) adds must also run and return data in its range, the check that the evidence of a [finding](investigate.md) gets.
An agent or MCP client never writes HTML into a text block, as everywhere else.

### Who can do what

An observation lives in one folder and follows that folder's access, as a dashboard does; see [Folders](../teams/folders.md).

- A person who can view the folder can read the observation.
- A person who can comment adds blocks that are `requested`, and takes part in the observation's one discussion, a single comment thread for the whole page.
- A person who can edit adds `live` blocks, changes and removes them, moves the observation, resolves and reopens it, and publishes it.
- A person who can manage the folder creates public links to its RCA, and deletes any observation in it.
- A person who started the observation and can still edit it deletes it.

A requested block is not part of the page's settled content until someone who can edit keeps it.
Until then only its author, editors and admins see it.
Viewers and other commenters read the page without it, in the app, over the API, in MCP and in the CLI.
An editor keeps a requested block or discards it, and the one who asked for it may discard it too.

An observation in My Dashboards is private to its owner.
Over the API and MCP, an observation and each of its blocks carry `allowed_actions`, the actions the caller may take now; [States](../reference/states.md#observation) lists them.
A block someone requested also shows in the inbox of the person who created the observation, as a "Suggested block" that waits to be kept or discarded; see [Inbox](inbox.md).
