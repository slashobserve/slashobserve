---
title: Investigate
description: Ask the agent a question about your data and get a finding, a short written answer with up to four evidence panels, that you can pin to a dashboard or turn into one.
---

An investigation asks the agent a question instead of asking it for a dashboard, such as "why did checkout errors rise after 10:00?".
The agent reads your datasources the way it does for a build, then ends by saving a finding.
It also reads the [annotations](annotations.md) of the dashboard and the global ones, so it can see a deploy or a change beside a spike.
It needs a model; see [LLM providers](../connect/llm-providers.md).

A plain question sent without the command, such as "how many pods are up?", is answered with a finding too, without building a dashboard.

## What a finding is

A finding is the agent's answer, written as Markdown of at most 400 words.
It carries up to four evidence panels, each a panel whose query the server ran and checked before the finding was saved.
It covers one window of time, shown under the answer with when it was found.
Every finding is saved with its own address, `/findings/<id>`, which you can share with anyone who can read the finding.
A finding on a dashboard follows that dashboard, so anyone who can view the dashboard can read it.
A finding that is not about a dashboard is filed in a folder, by default your My Dashboards, so only you can read it until it is moved to a shared folder.
A finding that is in no folder, such as one a service account saved before findings had folders, is readable by that account and by Admins.

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
While you are editing the layout it reads "Investigate (finish the layout first)" and cannot be chosen.

From a terminal, `observe investigate <question...>` asks the same question and prints the finding; `--dashboard`, `--panel`, `--from` and `--to` set what it is about.
See [CLI](cli.md).

Every investigation also opens an observation, which shows the run while it works and receives the finding's conclusion and evidence when it is saved.
See [Investigations write into an observation](observations.md#investigations-write-into-an-observation) for where it goes and how to write into an existing one with `observation_id`.

## Read the finding

In the chat, an investigation's turn shows its steps like a build, with the status "Investigating" while it runs.
When it ends, the turn shows the finding: the answer, the evidence panels, and when and over which window it was found.
On a dashboard, your running investigations show in the activity pane as running entries, and each finding about the dashboard appears there as "Agent found" with the same card and an **Open finding** link to its page.

The finding page at `/findings/<id>` shows the title, who found it, the answer and the evidence, with links to the dashboard and to the chat that asked.
Its header reads "Found by Observe Agent for Ishan" for a finding the agent saved and "Saved by Ishan (via Claude Code)" for one saved over MCP, with the same label every surface shows.
The evidence panels always show the finding's own window, not the dashboard's current range.

## Comment on a finding

Under the finding on its page, **Comments** lists the finding's threads, each with its messages and who wrote them, and says "No comments yet." when there are none.
Write in the box "Comment on this finding" and choose **Comment**, or press Cmd+Enter (Ctrl+Enter elsewhere), to start a thread.
Each thread has **Reply**, **Resolve** and, once resolved, **Reopen**, and your own messages have **Edit**.
Mention people and the agent the way you do on a dashboard; see [Edit a comment and mention people](review.md#edit-a-comment-and-mention-people).
A mention is recorded in the [activity log](activity.md) and starts no agent run.

Commenting follows who can see the finding.
A finding on a dashboard follows that dashboard's folder, and a finding without a dashboard follows the folder it is filed in, so you need Comment on that folder; a finding in no folder can be seen and commented on by the account that saved it, and by Admins.
Anyone else who can read the finding sees its threads but no box and no buttons.

## Pin it to the dashboard

**Pin to dashboard** adds the finding to its dashboard as an annotation at the end of the finding's window, tagged `finding`, whose text is the finding's first line and a link to it.
Charts on the dashboard then show it as a marker; see [Annotations](annotations.md).
The card then reads "Pinned to the dashboard" with an **Open dashboard** link, and pinning again changes nothing.
Pinning needs Edit on the dashboard.

## Share a finding

A finding without a dashboard is filed in a folder, and its page names that folder under the title.
A finding you saved starts in your My Dashboards, where only you can read it.
**Publish to folder** on such a finding, or **Move to folder** on one filed in a shared folder, moves it to another folder, and then everyone who can view that folder can read it.
A finding on a dashboard follows the dashboard and has no folder of its own to move.

Moving needs Edit on the finding and on the folder you move it to, and My Dashboards takes no moves.
The move is recorded in the [activity log](activity.md) as `finding.moved`, and the finding's earlier activity moves to the new folder with it.
From the command line, `bin/observe findings move <finding-id> --folder <folder-id>` does the same, and over [MCP](mcp.md) `move_finding` does.

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
Viewers can read the findings on dashboards they can view.

## From an MCP client

An agent in your own editor can do the same over [MCP](mcp.md).
It investigates with its own model and the discovery and query tools, reads deploy markers with `list_annotations`, saves a finding with the `create_finding` tool, reads the saved ones with `list_findings` and moves one to another folder with `move_finding`.
