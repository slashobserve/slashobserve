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
| `text` | Markdown of at most 4,000 characters from an agent or MCP client, with no HTML. |
| `panel` | A data panel in ObserveSpec form with its own query. It runs live, over the observation's range or its own `from` and `to`. |
| `snapshot` | The data of a panel block frozen at one moment, so the page keeps showing what you saw. |
| `run` | The agent run that is investigating, added by the server. |

Each block is `full` width or `half` width, and blocks keep the order of their `position`.
A page holds at most 100 blocks, of which at most 24 are panel and snapshot blocks together.
A panel must be a valid data panel, and one that an agent or MCP client adds must also run and return data in its range, the check that the evidence of a [finding](investigate.md) gets.
Frozen data is limited to 2 MiB, and a larger result is refused with `snapshot_too_large`.
An agent or MCP client never writes HTML into a text block, as everywhere else.

## Who can do what

An observation lives in one folder and follows that folder's access, as a dashboard does (see [Folders](../teams/folders.md)).

- A person who can view the folder can read the observation.
- A person who can comment adds blocks that are `requested`, and takes part in the observation's one discussion, a single comment thread for the whole page.
- A person who can edit adds `live` blocks, changes and removes them, moves the observation, resolves and reopens it, and publishes it.
- A person who can manage the folder creates public links to its RCA.

A requested block is visible but is not part of the page's settled content until someone who can edit keeps it.
An editor keeps a requested block or discards it, and the one who asked for it may discard it too.
Each block has a `version`, and a change or a keep that names an older one is refused with `version_conflict`, so two people never overwrite each other unseen.
An observation in My Dashboards is private to its owner.

## Work on one in the web app

### Open an observation

**Observations** in the sidebar opens the list at `/observations`, with each observation's title, state, who opened it and when it last changed, most recently changed first.
The **State** filter shows Open observations by default, or Resolved or All, and **Load more** fetches the next page.
Under the navigation, **Open observations** lists up to eight observations that are open, and shows a count with a plus sign when there are more.
It is hidden while the sidebar is collapsed and when none is open.

An observation opens at `/o/<id>`.
The page updates when someone else changes its blocks, its state or the discussion.
**Share** in the top row copies the address of the page, which only people who can see the observation can open; a read-only link for everyone else is a [public link to an RCA revision](#share-a-revision-in-the-web-app).

### Start one

**New observation** on the list, in the **New** menu and in the command palette opens a panel with a **Title** of 1 to 200 characters and a **Folder**.
The folder choices are the folders you can edit.
With none chosen, the observation goes in My Dashboards when you may create there, and otherwise in General.
**Create** opens the new page.

### Add blocks

A person who can edit sees **Add a block**, and a person who can only comment sees **Suggest a block**.
The row offers four ways:

- **Text** opens a box for Markdown.
  **Add** or **Suggest** saves it, `⌘Enter` or `Ctrl+Enter` does the same, and `Esc` or **Cancel** closes the box.
- **Panel** opens a side panel where you search for a dashboard, then choose one of its data panels.
  The block is a copy of the panel with its own query, and it stays live on the page over the observation's range, or over the last hour when the observation has none.
- **Logs** opens the logs page with this observation preselected, so a search can be added from there (see [Add to an observation from a panel or from Logs](#add-to-an-observation-from-a-panel-or-from-logs)).
- **Snapshot** lists the live panel blocks of the page and is disabled when there is none.
  Choosing one freezes that panel's data into a snapshot block placed right after it.

An editor's block goes on the page as `live`.
A commenter's block is `requested`: it shows with a dashed border and the caption "`<name>` suggests this block", or "Observe Agent suggests this block" when an agent run wrote it.
**Keep** makes it live and **Discard** removes it.
Both show to an editor, and only **Discard** shows to the person who asked for the block.

A panel block's caption names its datasource and says "live", and a snapshot's caption says "Frozen" with the time range of its data.
A panel block can be frozen from its own menu too, with **Freeze as snapshot**.
A person who can only comment suggests that snapshot, as with any other block.

### Change blocks

Hover a block, or focus it, to show its **Block actions** menu.
It offers **Make half width** or **Make full width**, **Freeze as snapshot** on a live panel block, and **Delete block** on a live block.
Two half width blocks in a row sit side by side when the page is wide enough, and a half width block with no half width neighbour takes the left half of its row.
**Edit** on a text block opens its Markdown, and **Save** or `⌘Enter` keeps the change.
If someone changed the block first, the editor says so and **Use their version** replaces your text with theirs.
Text that is empty is refused; delete the block instead.
Editing, resizing and deleting need edit access to the folder.

### Rename, resolve, reopen and move

A person who can edit changes the title by choosing it, typing, and pressing `Enter`, or `Esc` to leave it as it was.
**Resolve** asks for a one-line summary of at most 500 characters, which then shows under the title and in the list.
**Reopen** turns a resolved observation back into an open one.
**More actions**, then **Move to folder**, opens a panel that searches the shared folders you can edit; My Dashboards is not a choice.
Resolving, reopening and moving need edit access, and moving needs it on the target folder too.
The state line also shows who started the observation and when, and links to its folder.

### Discuss beside the blocks

The discussion is the observation's one comment thread, shown to the right of the blocks on a wide window and below them on a narrow one.
**Hide discussion** closes it and **Show discussion** in the top row brings it back.
A person who can comment writes messages in the box at its foot.
Each message has **Add to page** for an editor, or **Suggest for page** for a commenter, which puts the message's text on the page as a text block.
The message stays in the discussion as well.

### Add to an observation from a panel or from Logs

**Add to observation** in a dashboard panel's menu copies that panel into an observation.
The item shows for every data panel while you have no unsaved layout changes, and it is greyed with "(uses dashboard variables)" after it when the panel cannot be copied.
A panel cannot be copied when a query, or the panel's datasource, holds `$name` or `${name`, where the name starts with a letter or an underscore.
That includes the built-in names such as `$__interval`.
A `$` before a digit or on its own does not count, and a variable in the panel's title does not either.
A block runs with no dashboard variables, so a panel that needs one is not offered.

The side panel asks **Where**: **An open observation**, chosen from the open ones you can see, or **A new observation**, which asks for a title that starts as the panel's title and a folder as above.
With no open observation, only the new one is offered.
**Add** puts the panel on the page and shows "Added to `<title>`", or "Suggested on `<title>`" for a person who can only comment, with **Open** to go there.
If the new observation was created but the panel could not be added, the message says so, and **Add** tries again on the same observation.
The block takes the time range in the dashboard's address as absolute times, and without one it follows the observation's range, or the last hour.
A panel's `repeat` setting is dropped.

**Add to observation** on the [logs page](logs-and-traces.md#add-to-an-observation) works the same way for the search you ran, as a logs panel named Logs over the range you are viewing.

## Work on one from MCP and the CLI

[MCP](mcp.md) clients use `create_observation`, `get_observation`, `list_observations`, `add_block`, `update_block`, `resolve_observation`, `keep_block`, `discard_block` and `publish_rca`.
The [CLI](cli.md#observations) has the same work as `observe o new`, `show`, `list`, `add`, `resolve`, `keep`, `discard` and `publish`.
Both reach the HTTP API under `/api/v1/observations`, and a person works on the page in the [web app](#work-on-one-in-the-web-app).
Searching the workspace finds observations too (see [Search](search.md)).

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
- Each block's width.

A revision leaves out requested blocks, run blocks, and the query text of every panel: a published copy shows what the data was and never how to query your datasources.
If a panel cannot be frozen, nothing is published and the answer is `panel_failed` with the block it names, so fix or remove that panel and publish again.
If the page changed while it was being frozen, the answer is `observation_changed`, and publishing again takes the new page.
An observation keeps at most 50 revisions, and one more is refused with `rca_limit`.
Publishing needs edit access to the observation's folder, and an observation in My Dashboards must be moved to a shared folder first, as a dashboard must before it is shared.

An agent can publish with `publish_rca` and a person with `observe o publish <observation-id>`, which prints `Published revision N of <title>`.
Neither shares the result with anyone outside the folder.

### Publish in the web app

**Publish as RCA** in the top row of the page opens a panel, and its address carries `?rca=1` so it can be linked.
**Publish revision N** freezes the page as the next revision and says "Published revision N" when it is done.
**Revisions** lists each revision with who published it and when.
A person who cannot edit sees **RCA revisions** instead, and only when a revision exists, with the note "You can view revisions. Editors publish them."
The panel refuses a publish with the reasons above: a panel that could not be frozen, a page that changed meanwhile, the limit of 50 revisions, a panel with too much data, and an observation in My Dashboards.
Choosing a revision shows its public links.
The app does not render a revision itself; a revision is read at the address of one of its links.

## Share a revision

A public link shows one revision of an RCA read-only to anyone who has it, with no sign-in.
Making one is a person's decision, as it is for a dashboard (see [Public links](public-links.md)): no agent or MCP tool creates, lists or revokes an RCA link, and `observe o` has no verb for it.
A person with Manage on the folder creates a link with `POST /api/v1/observations/{id}/rcas/{revision}/public-links`, and revokes it with `DELETE /api/v1/observations/{id}/public-links/{link_id}`.

The link's address is `<public url>/rca/<token>`, and the token is shown once, when the link is created.
A link can expire at a time you choose, and it stops working then; revoking it stops it at once.
An observation has at most 20 links across all its revisions.

A link serves the stored revision only, never the live observation, a live panel or a query, and it shows who published it by name and by no id.
An observation in My Dashboards cannot have a link.
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

- The title, and a line "Revision N, published by `<name>`, `<date and time>`".
- "Resolved", or "Open at publish time", and the summary when there is one.
- The blocks in page order: text as Markdown, and each panel as its frozen data under the panel's title.
  A panel's caption is "Frozen" with the observation's time range when it had one, and otherwise "Frozen when published".
- Half width blocks side by side when the page is wide enough.

It shows no query, discussion, comment, activity, live panel or other revision, and it runs no query against a datasource.
A panel of a type that cannot show on a public link, or that the page cannot draw yet, shows "This panel type is not shown yet."
An expired, revoked or unknown token shows "This link has expired or was turned off."

## Investigations write into an observation

An [investigation](investigate.md) opens an observation when it is queued, and its run shows there as a `run` block while the agent works.
`POST /api/v1/observe` with `kind` `investigate`, and `observe investigate`, both do this.

`POST /api/v1/observe` takes an optional `observation_id` for an investigation, the observation to write into, and `observe investigate` takes it as `--observation`.
It must be an observation you can see, or the answer is 404 `not_found`, and one you may comment on, or the answer is 403 `forbidden_role`.
An investigation that names an observation with 100 blocks already gets no run block and no `observation_id`, because the page has no room.

Without `observation_id`, the server makes a new observation:

- Its title is the first line of the prompt, cut to 200 characters, or "Investigation" when that line is empty.
- Its range is the window of the investigation, and it has none when the investigation was given no `from` and `to`.
- It goes in the folder of the dashboard the investigation is about when you can edit that folder.
  Otherwise it goes in your My Dashboards when you may create there, and otherwise in General when you may create there.
- When no such folder exists, the investigation still runs, writes no observation, and its `observation_id` is null.

When the agent saves its finding, the finding is saved as before.
Its conclusion also becomes a text block that starts with `## <title>` and goes on with the finding's Markdown, cut to the 8,000 characters the server accepts in a text block, followed by one panel block for each panel of its evidence.
The panel blocks show the finding's own window.
The run block then cites these blocks, and a run adds them once, so saving the finding again adds nothing.
An evidence panel that is not valid, or that does not fit in the page's limits, is left out, and the text block is still written.

A block is `live` when you can edit the observation's folder at the moment it is written, and `requested` when you can only comment.
A failure writing these blocks never changes the finding or the investigation's result: the failure is logged and the finding stands.
A server error while the observation is opened queues nothing and answers the request with that error.

`observation_id` on a run is the observation it writes into, and `GET /api/v1/observe/{id}` and `observe investigate --json` carry it.
`observe investigate --observation <id>` names the observation to write into, and without it a terminal starts a new one.

Starting an investigation again, with `POST /api/v1/observe/{id}/retry` or `observe retry`, writes into the observation of the run when you may comment on it.
Otherwise, or when that run had none, the rules for a new observation apply to you.
