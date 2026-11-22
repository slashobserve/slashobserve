---
title: Investigate
description: Ask the agent a question about your data and get an answer in an observation, a short written conclusion with up to four evidence panels, that you can pin to a dashboard or turn into one.
---

An investigation asks the agent a question instead of asking it for a dashboard, such as "why did checkout errors rise after 10:00?".
The agent reads your datasources the way it does for a build, then ends by saving its answer, a finding, into an [observation](observations.md).
It also reads the [annotations](annotations.md) of the dashboard and the global ones, so it can see a deploy or a change beside a spike, and cites the ones that fit with their times.
It can also read earlier [observations](observations.md) in the folders you can open, as you, and when one bears on the question it cites it by title and `/o/<id>` link and says its numbers are from an earlier time.
The web app sends the time zone you read times in, the dashboard's zone or the one in its time zone box on a dashboard and your browser's elsewhere, and the agent writes every time in the finding in that zone with its label, such as 19:52 IST; a finding that labels its times UTC instead is sent back to the agent to convert.
Over the CLI and MCP no zone is known, so times are written in UTC with the word UTC; an API caller can name one with `time_zone`.
A finding starts with one plain sentence, names the window it used, and says what it did not check; when two pods or services tie it leads with the one with the most errors.
It needs a model; see [LLM providers](../connect/llm-providers.md).

A plain question sent without the command, such as "how many pods are up?", is answered in the chat without building a dashboard, and no observation is made until you choose **Create observation** on the answer, or run `observe save-answer <request-id>`; see [Chat and follow-ups](chat.md).

## What a finding is

A finding is the agent's answer, written as Markdown of at most 400 words.
It carries up to four evidence panels, each a panel whose query the server ran and checked before the finding was saved.
It covers one window of time, shown under the answer with when it was found.

An answer about a dashboard may also carry `suggested_change`, one plain sentence describing a change to the dashboard that would help; the agent never makes the change itself, and nothing applies it until someone starts a revision with it or, for an answer given in a dashboard thread, applies it from that thread (`POST /threads/{id}/apply`, `apply_suggested_change` or `observe comments apply`), which posts "Revise the dashboard: <suggested change>" there and starts the revision as them.
An answer you can read, such as one in your chat on the dashboard, is also applied as a revision of its dashboard that takes the suggested change as its prompt, with `kind` `revise` and `applies_finding_id` on `POST /api/v1/observe`, `start_run` or `observe --revise --dashboard <id> --apply <finding-id>`; a second apply of the same answer while the first is queued or running, or after it succeeded, returns that run with `replayed` true when it is yours and answers `already_applied` when it is someone else's.
`observe_worthy` is true when the agent judged that the answer shows a jump, a drop or an error spike worth keeping as an observation; it saves nothing by itself.
Both are fields of the run's `finding` in `GET /observe/{id}`, `get_run`, `get_chat` and `observe status --json`.

The answer is saved into an observation, as a text block with the conclusion and one panel block for each evidence panel, so its address is the observation's, `/o/<id>`, which you can share with anyone who can read the observation.
The observation is in a folder and follows that folder's access; see [Observations](observations.md#who-can-do-what).
An investigation about a dashboard files its observation in that dashboard's folder when you can edit it, and otherwise in your My Dashboards, or in General when you have none.
It stays there when the dashboard moves.

The `/api/v1/findings` routes that remain (list, read, pin, make a dashboard and panel data) are deprecated adapters over observations; use the observation's own routes.
The finding tools and commands are gone.

## Start an investigation

In any chat box, type `/investigate` and a space, then your question.
The Investigate chip shows above the box, and the send asks the question instead of building a dashboard.
In a chat about a dashboard, the investigation is about that dashboard and its answer appears in the same conversation.
Without a dashboard, the send opens a new chat page for it.

On a dashboard, **Ask** in a panel's menu opens the [ask box](ask-and-comment.md#ask-mode) on the whole panel, which asks the agent privately in your chat about that dashboard; to ask in a thread the team reads, choose the "To:" chip in a comment, see [Ask the agent from a comment](review.md#ask-the-agent-from-a-comment).
The activity pane has no Investigate chip and takes no slash command: a message there that asks the agent is a request in a comment thread, see [Comments, agent requests and versions](review.md).
To start an investigation about a dashboard, type `/investigate` in a chat about that dashboard.

The panel menu item shows only on data panels, when you can edit the dashboard and a model is connected.
While you have unsaved layout changes it reads "Ask (finish the layout first)" and cannot be chosen.

From a terminal, `observe investigate <question...>` asks the same question and prints the finding; `--dashboard`, `--panel`, `--from` and `--to` set what it is about.
See [CLI](cli.md).

Every investigation also opens an observation, which shows the run while it works and receives the finding's conclusion and evidence when it is saved.
See [Investigations write into an observation](observations.md#investigations-write-into-an-observation) for where it goes and how to write into an existing one with `observation_id`.

## Read the finding

In the chat, an investigation's turn shows its steps like a build, with the status "Investigating" while it runs.
When it ends, the turn shows the answer: the conclusion, the evidence panels, and when and over which window it was found.
On a dashboard, your running investigations show in the activity pane as running entries, and each finding about the dashboard appears there as "Agent found" with the same card and an **Open observation** link to the observation it lives in.

The page is the observation at `/o/<id>`, with the conclusion and the evidence panels as blocks, the discussion and who wrote what; see [Observations](observations.md#open-an-observation).
The evidence panels always show the finding's own window, not the dashboard's current range.

## Comment on a finding

The comments on an answer are the observation's discussion, one thread for the whole page, and you write in it as described in [Observations](observations.md#who-can-do-what).
Mention people and the agent the way you do on a dashboard; see [Edit a comment and mention people](review.md#edit-a-comment-and-mention-people).
A mention is recorded in the [activity log](activity.md), and a comment that asks an agent (`to: agent` over the API, MCP or the CLI) starts an investigation for it; see [Ask an agent from a comment](cli.md#ask-an-agent-from-a-comment).
Commenting follows who can see the observation: you need Comment on its folder, and anyone else who can read it sees the discussion but no box and no buttons.
Threads that people opened on a finding before the discussion existed were merged into it when the server upgraded.
The comment API takes only the observation as a target: a `finding:<id>` target is refused as an invalid target.

## Pin it to the dashboard

**Pin to dashboard** adds the finding to its dashboard as an annotation at the end of the finding's window, tagged `finding`, whose text is the finding's first line and a link to it.
Charts on the dashboard then show it as a marker; see [Annotations](annotations.md).
The card then reads "Pinned to the dashboard" with an **Open dashboard** link, and pinning again changes nothing.
Pinning needs Edit on the dashboard, so the button is not shown to a Commenter or Viewer, who see "Pinned to the dashboard" once someone else has pinned it.
Over the API, `POST /api/v1/findings/{id}/pin` without Edit answers `403` `forbidden_role` with `needed_access` `edit`.

## Share a finding

Move the observation to another folder to share it: **More actions**, then **Move to folder**, as described in [Observations](observations.md#rename-resolve-reopen-and-move).
Everyone who can view that folder can then read the answer, and My Dashboards takes no moves.
The finding move tool, command and route are gone; only earlier `finding.moved` events remain in the activity log.

## Make a dashboard from it

**Make a dashboard** saves the evidence panels as a new dashboard in your My Dashboards and opens it.
From a chat, the dashboard opens with that chat in the Chat tab of its side panel, where you can keep asking.
The server attaches the chat to the new dashboard when it makes it, whichever way you made it: the button, the API route or the `make_dashboard` tool.
The chat stays private to you, and Admins read it without writing; nobody else sees it on the dashboard.
A chat that is already about another dashboard or observation stays where it is, and the new dashboard opens without it.
A build that is still running in the same chat keeps the chat on whichever dashboard attached it first, so make the dashboard after the build finishes if you want the chat on the built one.
The server runs every evidence query again before it saves.
Anyone who can read the finding can do this, and a finding without evidence panels has no such button.

## When there is no finding

When the agent ends without saving a finding, the turn reads "The question could not be answered", then "The agent ended without writing a finding." and "Ask a narrower question, with the service and the time range."
**Try again** asks the same question again.
A run that finished but saved nothing reads "The run finished without a finding."
The other failures read as they do for a build; see [Read a turn](chat.md#read-a-turn).

## Who can investigate

Starting an investigation needs the Editor role or above, and view access on the dashboard it is about.
Viewers can read the observations in the folders they can view.
**Ask** in a panel's menu, the panel toolbar and the panel's header, and [Ask mode](ask-and-comment.md#ask-mode) in the bottom tool bar, ask a question as a private turn of your chat rather than starting an investigation, so they need only Comment on the dashboard and a connected model; a Viewer does not see them.
Over the API, `POST /api/v1/observe` with `kind` `investigate` from a Viewer or Commenter answers `403` `forbidden_role` with `needed_role` `editor`.

## From an MCP client

An agent in your own editor can do the same over [MCP](mcp.md).
It investigates with its own model and the discovery and query tools, reads deploy markers with `list_annotations`, saves the answer into an observation with `create_observation` and `add_block`, and reads earlier ones with `list_observations` and `get_observation`.
