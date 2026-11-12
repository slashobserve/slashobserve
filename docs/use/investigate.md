---
title: Investigate
description: Ask the agent a question about your data and get an answer in an observation, a short written conclusion with up to four evidence panels, that you can pin to a dashboard or turn into one.
---

An investigation asks the agent a question instead of asking it for a dashboard, such as "why did checkout errors rise after 10:00?".
The agent reads your datasources the way it does for a build, then ends by saving its answer, a finding, into an [observation](observations.md).
It also reads the [annotations](annotations.md) of the dashboard and the global ones, so it can see a deploy or a change beside a spike.
It needs a model; see [LLM providers](../connect/llm-providers.md).

A plain question sent without the command, such as "how many pods are up?", is answered in an observation too, without building a dashboard.

## What a finding is

A finding is the agent's answer, written as Markdown of at most 400 words.
It carries up to four evidence panels, each a panel whose query the server ran and checked before the finding was saved.
It covers one window of time, shown under the answer with when it was found.
The answer is saved into an observation, as a text block with the conclusion and one panel block for each evidence panel, so its address is the observation's, `/o/<id>`, which you can share with anyone who can read the observation.
The observation is in a folder and follows that folder's access; see [Observations](observations.md#who-can-do-what).
An investigation about a dashboard files its observation in that dashboard's folder when you can edit it, and otherwise in your My Dashboards, or in General when you have none.
It stays there when the dashboard moves.
The finding routes, tools and commands are deprecated and are removed in the next release; use the observation's own.

## Start an investigation

In any chat box, type `/investigate` and a space, then your question.
The Investigate chip shows above the box, and the send asks the question instead of building a dashboard.
In a chat about a dashboard, the investigation is about that dashboard and its answer appears in the same conversation.
Without a dashboard, the send opens a new chat page for it.

On a dashboard, choose **Investigate** in a panel's menu.
It opens the activity pane in investigate mode, with the panel as the target and two chips above the box: Investigate and the dashboard's current time range.
The box reads "Ask a question about this panel", and the button reads **Investigate**.
Remove the time range chip to send the question without a window, or remove the Investigate chip to go back to a normal follow-up.
In the activity pane you can also type `/investigate` and a space to ask about the dashboard or the panel the box is on, and `/revise` and a space to go back.
After the send, the box goes back to a normal follow-up.

The panel menu item shows only on data panels, when you can edit the dashboard and a model is connected.
While you have unsaved layout changes it reads "Investigate (finish the layout first)" and cannot be chosen.

From a terminal, `observe investigate <question...>` asks the same question and prints the finding; `--dashboard`, `--panel`, `--from` and `--to` set what it is about.
See [CLI](cli.md).

Every investigation also opens an observation, which shows the run while it works and receives the finding's conclusion and evidence when it is saved.
See [Investigations write into an observation](observations.md#investigations-write-into-an-observation) for where it goes and how to write into an existing one with `observation_id`.

## Read the finding

In the chat, an investigation's turn shows its steps like a build, with the status "Investigating" while it runs.
When it ends, the turn shows the answer: the conclusion, the evidence panels, and when and over which window it was found.
On a dashboard, your running investigations show in the activity pane as running entries, and each finding about the dashboard appears there as "Agent found" with the same card and an **Open finding** link to its page.

The page is the observation at `/o/<id>`, with the conclusion and the evidence panels as blocks, the discussion and who wrote what; see [Observations](observations.md#open-an-observation).
The evidence panels always show the finding's own window, not the dashboard's current range.

## Comment on a finding

The comments on an answer are the observation's discussion, one thread for the whole page, and you write in it as described in [Observations](observations.md#who-can-do-what).
Mention people and the agent the way you do on a dashboard; see [Edit a comment and mention people](review.md#edit-a-comment-and-mention-people).
A mention is recorded in the [activity log](activity.md) and starts no agent run.
Commenting follows who can see the observation: you need Comment on its folder, and anyone else who can read it sees the discussion but no box and no buttons.
Threads that people opened on a finding before the discussion existed were merged into it when the server upgraded.
The comment API still takes `finding:<id>` as a target for one release and resolves it to the discussion of the finding's observation.

## Pin it to the dashboard

**Pin to dashboard** adds the finding to its dashboard as an annotation at the end of the finding's window, tagged `finding`, whose text is the finding's first line and a link to it.
Charts on the dashboard then show it as a marker; see [Annotations](annotations.md).
The card then reads "Pinned to the dashboard" with an **Open dashboard** link, and pinning again changes nothing.
Pinning needs Edit on the dashboard.

## Share a finding

Move the observation to another folder to share it: **More actions**, then **Move to folder**, as described in [Observations](observations.md#rename-resolve-reopen-and-move).
Everyone who can view that folder can then read the answer, and My Dashboards takes no moves.
`finding.moved` and `move_finding`, `bin/observe findings move` and `POST /api/v1/findings/{id}/move` are deprecated and are removed in the next release.

## Make a dashboard from it

**Make a dashboard** saves the evidence panels as a new dashboard in your My Dashboards and opens it.
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

## From an MCP client

An agent in your own editor can do the same over [MCP](mcp.md).
It investigates with its own model and the discovery and query tools, reads deploy markers with `list_annotations`, saves the answer into an observation with `create_observation` and `add_block`, and reads earlier ones with `list_observations`.
`create_finding`, `list_findings` and `move_finding` still work and are deprecated.
