---
title: Observations
description: Work one question on a page of text, panel and snapshot blocks that people and agents build together, resolve it with a summary, and publish it as an immutable RCA that a person can share read-only.
---

An observation is a page about one question, such as why checkout errors rose at 09:12.
It has a title, a time range, a folder and an ordered list of blocks.
People and agents add the blocks as they learn, and the page ends with a one-line summary when the question is answered.
Every change is a recorded [activity](activity.md) event, so the log shows who wrote what.

## Blocks

A block is one of these kinds:

| Kind | What it holds |
| --- | --- |
| `text` | Markdown of at most 8,000 characters, and no HTML when an agent or MCP client writes it. |
| `panel` | A data panel in ObserveSpec form with its own query. It runs live, over the observation's range or its own `from` and `to`. |
| `snapshot` | The data of a panel block frozen at one moment, so the page keeps showing what you saw. |
| `run` | The agent run that is investigating, added by the server. |

Each block is `third`, `half`, `two_thirds` or `full` width, only a panel or snapshot block has an optional `height` of 120 to 960 pixels, and a height for a text block is refused with `invalid_block`, and blocks keep the order of their `position`.
A person who can only comment sees positions that count only the blocks they can see, and a suggestion they place at one of those positions lands there once an editor keeps it.
A page holds at most 100 blocks, of which at most 24 are panel and snapshot blocks together.
A panel must be a valid data panel, and one that an agent, an MCP client or a service-account key (the [CLI](cli.md) and scripts) adds must also run and return data in its range, the check that the evidence of a [finding](investigate.md) gets.
Frozen data is limited to 2 MiB, and a larger result is refused with `snapshot_too_large`.
An agent or MCP client never writes HTML into a text block, as everywhere else.

## Who can do what

An observation lives in one folder and follows that folder's access, as a dashboard does (see [Folders](../teams/folders.md)).

- A person who can view the folder can read the observation.
- A person who can comment adds blocks that are `requested`, and takes part in the observation's one discussion, a single comment thread for the whole page.
- A person who can edit adds `live` blocks, changes and removes them, moves the observation, resolves and reopens it, and publishes it.
- A person who can manage the folder creates public links to its RCA, and deletes any observation in it.
- A person who started the observation and can still edit it deletes it.

A requested block is not part of the page's settled content until someone who can edit keeps it.
Until then only its author, editors and admins see it; viewers and other commenters read the page without it, in the app, over the API, in MCP and in the CLI.
An editor keeps a requested block or discards it, and the one who asked for it may discard it too.
Each block has a `version`, and a change or a keep that names an older one is refused with `version_conflict`, so two people never overwrite each other unseen.
An observation in My Dashboards is private to its owner.
Over the API and MCP, an observation and each of its blocks carry `allowed_actions`, the actions the caller may take now; [States](../reference/states.md#observation) lists them.
A block someone requested also shows in the inbox of the person who created the observation, as a "Suggested block" that waits to be kept or discarded; see [Inbox](inbox.md).

## Work on one in the web app

### Open an observation

**Observations** in the sidebar opens the list at `/observations`, one row for each observation with its title, folder, state, who opened it and when it last changed, most recently changed first.
The **Open**, **Resolved** and **All** tabs above the list choose which observations show, Open by default.
The search box keeps the observations whose title contains the text, **Opened by me** keeps the ones you started, and the sort menu orders by last change, creation time or title, in either direction.
Below the list, **Per page** chooses 20, 50 or 100 rows, and the page numbers with **Previous** and **Next** move through the result; the header says which rows show, as "21-40 of 59".

The tab, search, filter, sort, page and page size are in the address, so a link reopens the same view.
The checkbox on a row selects it, and the one in the header selects every row on the page.
With rows selected, **Resolve** closes them all with one summary and **Delete** removes them; each counts only the rows you may resolve or delete.

The **...** menu on a row offers **Resolve**, **Reopen** and **Delete** for that row when you may do them.
Under **Observations** in the sidebar, up to five open observations are listed, indented, each with a green dot and in bold while it has unread inbox items, and **All observations** follows when more are open.
On the icon rail, a dot on **Observations** shows that some are open.

An observation opens at `/o/<id>`.
The page updates when someone else changes its blocks, its state or the discussion.
An address that is not an observation id, or an observation you cannot see, shows "Observation not found" with a link back to the list, and no Retry, because retrying cannot help.
**Share** in the top row copies the address of the page, which only people who can see the observation can open; a read-only link for everyone else is a [public link to an RCA revision](#share-a-revision-in-the-web-app).

### Start one

**New observation** on the list opens a panel with a **Title** of 1 to 200 characters and a **Folder**, and Home's **New observation** tab, which the command palette's **New observation** opens, asks for the same two fields plus an optional first note, which becomes the first text block, and links in it to a logs search, a trace or a dashboard panel of this app become panel blocks; see [Home](home.md#new-observation).
The folder choices are the folders you can edit.
With none chosen, the observation goes in My Dashboards when you may create there, and otherwise in General, and the folder box names that default before you pick one.
**Create** opens the new page.

### Add blocks

A person who can edit sees **Add a block**, and a person who can only comment sees **Suggest a block**.
The row offers four ways:

- **Text** opens a box for Markdown.
  **Add** or **Suggest** saves it, and `⌘Enter` or `Ctrl+Enter` does the same.
  `Esc` closes the box and keeps what you typed for when you open it again, and **Cancel** closes it and throws the text away.
  A word with no break in it, such as a hash or a long address, wraps inside the block instead of widening the page.
- **Panel** offers **From a dashboard** and **New query**.
  **From a dashboard** opens a side panel where you search for a dashboard, then choose one of its data panels.
  The block is a copy of the panel with its own query, and it stays live on the page and follows the page's range, or the last hour when the observation has none.
  **New query** opens the panel editor on an empty panel with the first datasource, and is disabled when there is no datasource; **Add**, or **Suggest** for a commenter, saves it as a panel block that follows the page's range (see [Edit a panel block](#edit-a-panel-block)).
- **Logs** opens the logs page with this observation preselected, so a search can be added from there (see [Add to an observation from a panel or from Logs](#add-to-an-observation-from-a-panel-or-from-logs)).
- **Snapshot** lists the live panel blocks of the page and is disabled when there is none.
  Choosing one freezes that panel's data into a snapshot block placed right after it.

An editor's block goes on the page as `live`.
A commenter's block is `requested`: it shows with a dashed border and the caption "`<name>` suggests this block", or "Observe Agent suggests this block" when an agent run wrote it.
**Keep** makes it live and **Discard** removes it.
Both show to an editor, and only **Discard** shows to the person who asked for the block.
When the block above a suggestion is a live panel block, the row also says that keeping it adds this block next to the one above and replaces nothing.
Resolving does not close the page: a resolved observation still takes blocks from editors and suggestions from commenters, and the row says "This observation is resolved" so nobody is surprised; an editor who does not want more reopens it or discards the suggestion.

A panel block's caption names its datasource and its window: "Last hour, live", the page's range, or "Pinned to" the block's own; a snapshot's caption says "Frozen" with the time range of its data.
A panel block is drawn as a dashboard panel, and its panel menu has **View as table**, **Inspect**, **Download CSV** and **Save as PNG**, plus **Open in Logs** or **Open in Traces** on a logs or traces panel.
**Inspect** opens the panel's data and query in the right pane in place of the discussion, and closing it brings the discussion back.
A block that carries variables shows a picker above the panel for each one with stored options, starting at the values it was added with; a choice changes only what you see and is not saved.
A textbox, ad hoc filter or group by variable shows its stored value and cannot be changed on the block.
A panel block can be frozen from its own menu too, with **Freeze as snapshot**.
A person who can only comment suggests that snapshot, as with any other block.

### Edit a panel block

A panel block's panel menu has **Edit panel** for an editor and **Suggest an edit** for a commenter.
It opens the same panel editor as a dashboard panel, in the right pane in place of the discussion: the query, the display and the JSON view, with a preview of the draft over the block's window and with the block's variables.
The preview runs with exactly the access the block's own data has, so it needs comment access to the observation and the datasource's access, and an Admins-only datasource runs only in an Admins-only folder.
Nothing changes until you save, and `Esc`, **Close panel editor**, leaving the page or opening another block, **New query** or **From a dashboard** asks first when the draft has changes; **Keep editing** keeps the editor and its draft.

**Save** changes the block in place and keeps its width, height and window.
A commenter's **Suggest edit** leaves the block as it is and adds the edited copy right after it as a requested block, which an editor keeps or discards like any other suggestion; keeping it adds a second block, so an editor who wants only the edit then deletes the original.
When someone changed the block since you opened it, the editor says so and keeps your draft.
When someone deletes the block while your draft has changes, the editor says so and keeps the draft: **Add as a new block**, or **Suggest as a new block** for a commenter, puts it back where the block was, and **Discard** throws it away.
A snapshot and a published RCA revision are frozen, so neither offers the editor.

Over [MCP](mcp.md) `update_block` changes a panel block's `body`, and on the [CLI](cli.md#observations) `observe o edit-block <block-id> --panel-file <path>` does the same.

### Change blocks

Hover a block, or focus it, to show its **Block actions** menu.
It offers **Move up** and **Move down**, which swap the block with its neighbour, a **Make ... width** item for each other width, **Reset height** on a panel or snapshot block with its own height, **Freeze as snapshot** on a live panel block, and **Delete block** on a live block.
Blocks sit side by side in rows of three thirds when the page is wide enough: three thirds, a third and two thirds, or two halves; a block that does not fit the rest of a row starts the next one.

### Arrange by hand

A person with edit access arranges the page without a menu.
Drag a block by the grip at its top left corner to drop it before or after another block, and drag the corner at its bottom right to snap its width to a third, a half, two thirds or the full page and, on a panel or snapshot block, to set its height in steps of 20 pixels.
Focus the grip and press the arrow keys to move the block, **Shift** with the left or right arrow to change its width, and **Shift** with the up or down arrow to change a panel's height by 40 pixels; each change is announced.
Below 640 pixels of block list width the page shows one column, so the width does not apply; the right dock, when open, can bring the list below that.

Arranging never conflicts with a text edit that is open on the same block, because a block's version counts its content and not its place or size.
When two people arrange at once the last arrangement wins.
A person who can only comment cannot arrange.

**Delete block** on a block an agent wrote, which is every run block and any block added by the built-in agent or over MCP, asks "Delete this block?" first, because the block cannot be brought back.
**Edit** on a text block opens its Markdown, and **Save** or `⌘Enter` keeps the change.
If someone changed the block first, the editor says so and **Use their version** replaces your text with theirs.
Text that is empty is refused; delete the block instead.
Editing, moving, resizing and deleting need edit access to the folder.
An agent changes one block with `update_block` and its `position`, `width` or `height`, and arranges the whole page with `arrange_blocks`, which needs no version.

### Time range

The page's time range sits in the top row, and every panel block follows it unless the block is pinned.
With no range stored it reads **Last hour, live**.
A person who can edit chooses another one; a quick range such as **Last 6 hours** is stored as the absolute window it means at that moment, so everyone reads the same data, and **Last hour, live** clears it.
Anyone else sees the range without the picker.

A panel block's panel menu offers **Pin to this window**, which keeps the window the block shows now, and **Pin to a window**, which opens a time range picker for that block.
Dragging across a time series chart pins the block to the dragged range.
A pinned block keeps its window when the page's range changes, and **Follow the page** releases it.
A block added from a dashboard panel's menu arrives pinned to the window it was taken from, and one chosen with **Panel**, **From a dashboard** follows the page.
Pinning needs edit access to the folder.

Over [MCP](mcp.md) `update_observation` sets the page's range, and `update_block` sets a block's own `from` and `to` or releases them with `follow_page: true`.
On the [CLI](cli.md#observations) `observe o edit --from <time> --to <time>` or `--clear-range` sets the page's range, and `observe o edit-block --follow-page` releases a block.

### Rename, resolve, reopen and move

A person who can edit changes the title by choosing it, typing, and pressing `Enter`, or `Esc` to leave it as it was.
An observation that is the one observation made from a chat with no dashboard shares its title with that chat, so renaming one renames the other; see [Chat](chat.md#one-title-for-a-chat-and-its-observation).
**Resolve** asks for a one-line summary of at most 500 characters, which then shows under the title and in the list.
**Reopen** turns a resolved observation back into an open one.
The summary stays stored, so the next **Resolve** starts from it, and the page and a published RCA show it only while the observation is resolved: `observe o show` prints it as `Last summary`, and over the API and MCP it is still in `summary` beside `state: open`.
**More actions**, then **Move to folder**, opens a panel that searches the shared folders you can edit; My Dashboards is not a choice.
Resolving, reopening and moving need edit access, and moving needs it on the target folder too.
The state line also shows who started the observation and when, and links to its folder.

### Pin it and make a dashboard

**More actions** has **Pin to dashboard** when the observation came from a run about a dashboard and you have edit access to that dashboard.
It adds an annotation at the end of the observation's range with the title and a link back.
Over the API the observation's `can_pin` says whether you may, and a pin to a dashboard you cannot open answers 404 "dashboard not found".
Once it is pinned the item reads **Pinned to dashboard**, and pinning again over the API or MCP returns the same annotation.

**Make a dashboard**, in **More actions** and in the actions of a live panel block, saves the live panel blocks, or that one block, as a new dashboard in My Dashboards and opens it.
The dashboard keeps the variables each block carries and uses the observation's range, or the first panel's when it has none, and every query is run again first.
Making it again from the same observation or block opens the one you made first, unless you can no longer see that dashboard, and then a new one is made.
Over the API `new: true` makes another.
When the observation came from your own chat and that chat is about nothing else, the chat becomes about the new dashboard and opens in its Chat tab.
Suggested blocks, text blocks and snapshots are left out.

### Discuss beside the blocks

The discussion is the observation's one comment thread, in a pane on the right of the page at full height on a wide window and below the blocks on a narrow one.
Drag the pane's left edge to make it wider or narrower; it shares its width with a dashboard's activity pane.
The **Discussion** button in the top row, which shows the number of messages, hides and shows the pane, and **Close panel** in the pane hides it.
The pane lists the messages together with the changes to the page, newest at the bottom, grouped under a heading for each day, with each entry showing its author's mark and the time of day.
The changes are who started the observation (with the title it was created with and its folder), who added or suggested each block (a panel block also names its datasource and says it is live, and a snapshot names the panel it froze), who changed or removed a block, who renamed the observation, from which title to which, and who moved, resolved, reopened, published or withdrew a revision of it.
The pane reads up to 500 of the newest such events.
A removed block is drawn without a name, because the block is gone.
A block's name scrolls the page to that block.

Two lists under the heading filter what the pane shows.
**Show** narrows it to messages only or changes only, and **Block** narrows it to one block: that block's messages, the changes to it, and the replies that quote those messages.
When nothing matches, the pane says so and **Clear filters** shows everything again.

A person who can comment writes messages in the box at its foot.
A person who can only view the observation sees **Request comment access** there instead, which asks the people who manage the folder; see [Ask for comment access](../teams/folders.md#ask-for-comment-access).
The control above the box says where the message goes: **On the whole page**, or **On** one block, which is the block chosen in **Block** until you pick another.
A message written on a block shows "On" and the block's name with **Show**, which scrolls the page to the block; a message written on a block that has since been deleted shows nothing extra.
**Reply** on a message puts "Replying to" and its author above the box, with a button that removes it; the reply then shows the quoted message above its text and reads "replied to" and the quoted author.
Each message also has **Add to page** for an editor, or **Suggest for page** for a commenter, which puts the message's text on the page as a text block.
The message stays in the discussion as well.

A message can be about one block of the page or about the whole observation, and a reply can quote the message it answers.
Over the API, MCP and the CLI a message carries `block_id` and `reply_to`, both null when the message is about the whole page and quotes nothing; `reply_to` holds the quoted message's `id`, its `actor` and an `excerpt` of its first 200 characters.
The MCP comment tools return `reply_to` with the author as `author`, the label the web app shows.
Deleting a block keeps the messages written about it, which are then about the whole observation, and deleting a quoted message leaves the reply with no quote.
A block that belongs to another observation answers 422 `invalid_block`, and a quoted message that is not in the discussion answers 422 `invalid_reply_to`.
Editing a message changes neither.

Typing `@` opens the picker of people and agents, as in [Edit a comment and mention people](review.md#edit-a-comment-and-mention-people), and a person can delete a message as described in [Delete a comment](review.md#delete-a-comment).
Deleting the first message deletes the whole discussion only while nobody else has written in it.

A message that mentions one agent, `@observe`, one of your connected agents or a service account that takes runs, asks that agent a read-only question about this observation, and it needs Comment on the observation.
The run writes nothing on the page, for an Editor as for a Commenter: when it ends the server posts one status line in the discussion, such as "Answered: <title>." and the first paragraph of the answer, and everyone who can read the observation can open the run and its answer.
The line is not the model's text, and the paragraph is shown as plain text, and it never resolves the discussion.
[Give an agent a run](agents.md#give-an-agent-a-run) describes who you can mention and what happens to the run.

### Add to an observation from a panel or from Logs

**Add to observation** in a dashboard panel's menu copies that panel into an observation.
The item shows for every data panel while you have no unsaved layout changes.
A panel that uses dashboard variables, a `$name` or `${name}` in a query or in the panel's datasource, is copied with the variables it uses and the values selected at that moment, so the block shows what you saw.
The built-in names, which start with two underscores, such as `$__rate_interval`, `$__interval` and `${__user.name}`, are not variables, because a block runs them as it runs any query.
A `$` before a digit or on its own does not count, and a variable in the panel's title does not either.
A multi-value selection and **All** expand in the query as they do on the dashboard, because the block keeps the variable's definition.
The block says "with `service=checkout`" under its title, and changing the dashboard's selection later does not change the block.

The side panel asks **Where**: **An open observation**, chosen from the open ones you can see, or **A new observation**, which asks for a title that starts as the panel's title and a folder as above.
With no open observation, only the new one is offered.
**Add** puts the panel on the page and shows "Added to `<title>`", or "Suggested on `<title>`" for a person who can only comment, with **Open** to go there.
If the new observation was created but the panel could not be added, the message says so, and **Add** tries again on the same observation.
The block takes the time range in the dashboard's address as absolute times, and without one it follows the observation's range, or the last hour.
A panel's `repeat` setting is dropped.

**Add to observation** on the [logs page](logs-and-traces.md#add-to-an-observation) works the same way for the search you ran, as a logs panel named Logs over the range you are viewing.
**Add to observation** on the [metrics page](metrics.md#add-to-a-dashboard-or-an-observation) does the same with a time series panel named Metrics, or a table panel in the table mode.

**Add to observation** on the [trace search](logs-and-traces.md#trace-search) and in the [trace view](logs-and-traces.md#trace-view) does the same with a traces panel: the search you ran, or the trace you are looking at, over the time range the page shows.

## Work on one from MCP and the CLI

[MCP](mcp.md) clients use `create_observation`, `get_observation`, `list_observations`, `add_block`, `update_block`, `update_observation`, `move_observation`, `resolve_observation`, `reopen_observation`, `keep_block`, `discard_block`, `publish_rca`, `list_rcas`, `get_rca`, `pin_observation` and `make_dashboard`.
The [CLI](cli.md#observations) has the same work as `observe o new`, `show`, `list`, `add`, `edit`, `move`, `resolve`, `reopen`, `keep`, `discard`, `edit-block`, `publish`, `rcas`, `pin` and `dashboard`, and `observe o delete`, `remove-block` and `withdraw`, which no MCP tool matches because no MCP tool deletes anything.
Both reach the HTTP API under `/api/v1/observations`, and a person works on the page in the [web app](#work-on-one-in-the-web-app).

`GET /api/v1/observations` filters with `folder_id`, `state`, `q` (a title that contains the text, ignoring case, at most 200 characters) and `created_by=me`, and sorts with `sort` (`updated`, the default, `created` or `title`) and `order` (`asc` or `desc`; `title` starts ascending, the others descending).
`limit` is 1 to 100 and defaults to 20.
It pages in one of two ways: `cursor` and `next_cursor` for the default order, or `offset` (at most 100000), which also answers `total`, the number of observations that match.
A cursor cannot be combined with `offset` or with another sort.

Every observation the HTTP API and the MCP observation tools return carries `message_count`, how many messages its discussion holds, replies included, and 0 before anyone has written one.
To read what changed on one observation, filter the [activity](activity.md) with `observation_id`.
Searching the workspace finds observations too (see [Search](search.md)).

## Delete an observation

Deleting removes the observation for everyone, with its blocks, snapshots, discussion and RCA revisions, and the public links of those revisions stop working.
It cannot be undone.
A pin of the observation on a dashboard is taken off it too, when you started that pin or are an Admin; otherwise the marker stays and its link shows "Observation not found".
While an investigation on the observation is queued or running, the delete is refused with `run_in_progress`; delete it once the run ends or has been stopped.
The person who started the observation and can edit it may delete it, and so may anyone who manages its folder.
In the web app it is **More actions**, then **Delete observation**, on the page, or **Delete** on the list, and it asks you to confirm first.
Over HTTP it is `DELETE /api/v1/observations/{id}`, which answers 204, and in the CLI `observe o delete <observation-id> --yes`.
The [activity](activity.md) log keeps an `observation.deleted` event with the title.

## Resolve with a summary

Resolving closes an observation with a summary of one line, at most 500 characters, that says what was found.
A resolved observation stays readable and can be reopened by a person, and it can still be published.

## Publish as RCA

Publishing turns the page into a root cause analysis that never changes.
Each publish adds the next numbered revision, so an RCA is a history of what the team believed as the investigation went on.

A revision freezes:

- The title, summary, state and time range of the observation when it was published.
- The `live` text blocks, as Markdown, in page order.
- Every `snapshot` block with its frozen panel and data, and a new snapshot of every `live` panel block, taken in the same step.
- Each block's width, and the height of a panel or snapshot block that was set by hand.

A revision published before heights were kept has none, so its panels show at the default height for their width.

A published page lays its blocks out in rows of thirds like the observation page does, from 640 pixels of width up and in one column below that, so a block that sits alone at half width takes half the row.
Revisions published before this packing existed show such a block at half width too, where they used to give it the whole row.

A revision leaves out requested blocks, run blocks, and the query text of every panel: a published copy shows what the data was and never how to query your datasources.
If a panel cannot be frozen, nothing is published and the answer is `panel_failed` with the block it names, so fix or remove that panel and publish again.
If the page changed while it was being frozen, the answer is `observation_changed`, and publishing again takes the new page.
An observation keeps at most 50 revisions, and one more is refused with `rca_limit`.
A page with no live panel block whose title, summary, state, range and blocks equal its latest revision is refused with `rca_unchanged`, so pressing Publish twice, or an agent retrying, never makes a copy.
A page with a live panel block always publishes, because each publish takes new data.
Publishing needs edit access to the observation's folder, and an observation in My Dashboards must be moved to a shared folder first, as a dashboard must before it is shared.
Such an observation does not list `publish` in its `allowed_actions`, and the web page offers **Publish as RCA** with a **Move to folder** button instead of a publish button.

An agent can publish with `publish_rca` and a person with `observe o publish <observation-id>`, which prints `Published revision N of <title>`.
Neither shares the result with anyone outside the folder.

### Publish in the web app

**Publish as RCA** in the top row of the page opens a panel, and its address carries `?rca=1` so it can be linked.
**Publish revision N** freezes the page as the next revision and says "Published revision N" when it is done.
**Revisions** lists each revision with who published it and when.
A person who cannot edit sees **RCA revisions** instead, and only when a revision exists, with the note "You can view revisions. Editors publish them."
The panel refuses a publish with the reasons above: a panel that could not be frozen, a page that changed meanwhile, the limit of 50 revisions, a page that has not changed since the latest revision, a panel with too much data, and an observation in My Dashboards.
When a panel could not be frozen, the message names the panel and why, and **Show block** closes the panel and scrolls to that block so you can fix or remove it.

Choosing a revision shows **Open revision N**, its public links and, for a person who can edit, **Withdraw**.
**Open revision N** shows the stored revision inside the app at `/o/<observation-id>/rca/<revision>`, for anyone who can see the observation, whether or not it has a public link.
The page is the same frozen text and panels a public link shows, with the name of who published it, and a link back to the observation.

### Withdraw a revision

A slip in a published revision cannot be edited, because a revision never changes, but it can be withdrawn.
**Withdraw** asks for confirmation and then deletes the revision and its frozen panels for everyone.
It needs Edit on the observation's folder and the same states as publishing, and the activity log keeps `observation.rca_withdrawn` with the title and the revision number.
A revision that still has a live public link answers `rca_shared` (409), because a public link is a person's decision: a person with Manage on the folder revokes its links first and then withdraws it.
Links that have already expired go with the revision.
Withdrawing the newest revision frees its number, so the next publish takes it again.
A person withdraws with `observe o withdraw <observation-id> <revision> --yes` or `DELETE /api/v1/observations/{id}/rcas/{revision}`.
No MCP tool deletes anything, so an agent cannot withdraw a revision.

## Share a revision

A public link shows one revision of an RCA read-only to anyone who has it, with no sign-in.
Making one is a person's decision, as it is for a dashboard (see [Public links](public-links.md)): no agent or MCP tool creates, lists or revokes an RCA link, and `observe o` has no verb for it.
A person with Manage on the folder creates a link with `POST /api/v1/observations/{id}/rcas/{revision}/public-links`, and revokes it with `DELETE /api/v1/observations/{id}/public-links/{link_id}`.

The link's address is `<public url>/rca/<token>`, and the token is shown once, when the link is created.
A link can expire at a time you choose, and it stops working then; revoking it stops it at once.
An observation has at most 20 links across all its revisions.

A link serves the stored revision only, never the live observation, a live panel or a query, and it shows who published it by name and by no id.
An observation in My Dashboards cannot have a link, and one in a folder only Admins can read is refused with `409 admins_only_folder`, which says to move it to a folder that is not Only Admins, or names the Admins-only datasource it reads (also in a `datasource` field) when it cannot leave.
A revision with a snapshot of data that depends on who is viewing, from `${__user.*}` in a query, cannot have a link, because nobody could read it on a public address.

### Share a revision in the web app

Choose a revision in the **Publish as RCA** panel to see **Public links, revision N**.
**Expires** offers No expiry, 1 day, 7 days, 30 days or On a date, and **Create link** makes the link.
**On a date** stops the link at the end of that day in your browser's time zone, and it must be today or later.
The full address shows once in a box with **Copy**, with the note "Copy it now. It is not shown again."
The list below it shows each link's first characters, whether it is active or expired and when it expires, who made it, and when it was last viewed.
**Copy link** stays on a link made since you chose the revision.
**Revoke** asks "Revoke this public link?" and then stops the link at once.
Creating, listing and revoking links need Manage on the folder, and the panel shows the server's reason when you lack it, when the observation is in My Dashboards, when the revision holds data that depends on the viewer, or when the observation already has 20 links.


### The public page

`<public url>/rca/<token>` opens with no sign-in and shows the SlashObserve name, then:

- The title, and a line "Revision N, published by `<name>`, `<date and time>` `<time zone>`", in the time zone of the reader's browser.
- "Resolved", or "Open at publish time", and the summary when there is one.
- The blocks in page order: text as Markdown, and each panel as its frozen data under the panel's title.
  A panel's caption is "Frozen" with the observation's time range and its time zone when it had one, and otherwise "Frozen when published".
- Blocks side by side in rows of thirds when the page is wide enough.

It shows no query, discussion, comment, activity, live panel or other revision, and it runs no query against a datasource.
When a newer revision has been published since, the page says "Revision N is newer, so this one may be out of date", with N only and no link, because the reader cannot open another revision; `GET /api/v1/public/{token}/rca` carries the number as `superseded_by`, null on the newest.
The same notice shows in the app, with **Open revision N** beside it.
A person who finds a link to a superseded revision can revoke it, or withdraw the revision once its links are revoked.
A panel of a type that cannot show on a public link, or that the page cannot draw yet, shows "This panel type is not shown yet."
An expired, revoked or unknown token shows "This link has expired or was turned off."

## Investigations write into an observation

An [investigation](investigate.md) opens an observation when it is queued, and its run shows there as a `run` block while the agent works.
`POST /api/v1/observe` with `kind` `investigate`, and `observe investigate`, both do this.

`POST /api/v1/observe` takes an optional `observation_id` for an investigation, the observation to write into, and `observe investigate` takes it as `--observation`.
It must be an observation you can see, or the answer is 404 `not_found`, and one you may comment on, or the answer is 403 `forbidden_role`.
An investigation that names an observation with 100 blocks already gets no run block and no `observation_id`, because the page has no room.

Without `observation_id`, the server makes a new observation:

- Its title is the first non-empty line of the prompt without `@observe`, cut at a word to about 60 characters with "...", or "Investigation" when no line is left.
- Its range is the window of the investigation, and it has none when the investigation was given no `from` and `to`.
- It goes in the folder of the dashboard the investigation is about when you can edit that folder.
  Otherwise it goes in your My Dashboards when you may create there, and otherwise in General when you may create there.
- When no such folder exists, the investigation still runs, writes no observation, and its `observation_id` is null.

When the agent saves its finding, the finding and its conclusion are written in one transaction, and a failure leaves neither.
The conclusion becomes a text block that starts with `## <title>` and goes on with the finding's Markdown, cut to the 8,000 characters the server accepts in a text block, followed by one panel block for each panel of its evidence.
The panel blocks show the finding's own window.
The run block then cites these blocks, and a run adds them once, so saving the finding again adds nothing.
The page names each cited block by the finding's heading or the panel's title.
An evidence panel that is not valid, or that does not fit in the page's limits, is left out, and the text block is still written.
A finding saved by a run that has an observation takes its own id, and one saved with no run observation takes the id of the observation it creates; its `observation_id` names the observation either way.

Where the answer goes depends on what the question was asked on.
A question asked on one block puts the run block, the text block and the evidence panels directly under that block, in that order, and the blocks below move down by that many places.
A question on the whole page, and one asked on a block that has been deleted since, puts them at the end of the page.
The new blocks are full width, and the order, widths and heights you arranged for the other blocks stay as they are.
The same rule holds when the agent writes its answer over MCP, when you save an answer from a chat onto the page, and when a commenter asks, whose blocks arrive as `requested` blocks in the same place.

On the page a run block that finished normally shows the outcome and **View the run** on one line, and **Run details** unfolds the steps and the blocks it cites.
A run that failed or was stopped stays unfolded, so its error and the retry control show without a click.
A text block written by a run that is longer than 600 characters shows its heading and first paragraph, and **The rest of the answer** shows or hides the remainder.
Folding is only how the page is drawn, so the stored block, a published revision and a snapshot always hold the whole text.

A block is `live` when you can edit the observation's folder at the moment it is written, and `requested` when you can only comment.
When the run's observation is gone, hidden from you, or in a folder you cannot comment in, saving the finding fails and writes nothing, instead of opening a second observation.
A server error while the observation is opened queues nothing and answers the request with that error.

`observation_id` on a run is the observation it writes into, and `GET /api/v1/observe/{id}` and `observe investigate --json` carry it.
`observe investigate --observation <id>` names the observation to write into, and without it a terminal starts a new one.

Starting an investigation again, with `POST /api/v1/observe/{id}/retry` or `observe retry`, writes into the observation of the run when you may comment on it.
Otherwise, or when that run had none, the rules for a new observation apply to you.
