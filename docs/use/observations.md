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

An [investigation](investigate.md) opens an observation when it is queued, and works into it while it runs.
`POST /api/v1/observe` takes an optional `observation_id` for an investigation, which must be an observation you can see and may comment on, and without one the server starts a new observation.
A new observation goes in the folder of the dashboard the investigation is about when you can edit that folder, otherwise in the folder you create in by default, which is My Dashboards for a person.
The page shows a `run` block while the agent works.

When the agent saves its finding, the finding is saved as before, and its conclusion also becomes a text block, with one panel block for each panel of its evidence.
The blocks are `live` when you can edit the folder and `requested` when you can only comment, and saving twice never adds them twice.
A failure writing the observation never changes the finding, and an investigation whose observation cannot be placed still runs and saves its finding.
Runs report the observation in `observation_id`, and `observe investigate --json` carries it.
