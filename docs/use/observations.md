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

## Work on one from MCP and the CLI

[MCP](mcp.md) clients use `create_observation`, `get_observation`, `list_observations`, `add_block`, `update_block`, `resolve_observation`, `keep_block`, `discard_block` and `publish_rca`.
The [CLI](cli.md#observations) has the same work as `observe o new`, `show`, `list`, `add`, `resolve`, `keep`, `discard` and `publish`.
Both reach the HTTP API under `/api/v1/observations`, and a person works on the page in the web app, where the sidebar lists the observations that are open.
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

## Investigations write into an observation

An [investigation](investigate.md) opens an observation when it is queued, and its run shows there as a `run` block while the agent works.
`POST /api/v1/observe` with `kind` `investigate`, and `observe investigate`, both do this.

`POST /api/v1/observe` takes an optional `observation_id` for an investigation, the observation to write into.
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
`observe investigate` has no flag to name an observation, so a terminal always starts a new one.

Starting an investigation again, with `POST /api/v1/observe/{id}/retry` or `observe retry`, writes into the observation of the run when you may comment on it.
Otherwise, or when that run had none, the rules for a new observation apply to you.
