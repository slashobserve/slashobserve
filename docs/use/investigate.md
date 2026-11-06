---
title: Investigate
description: Ask the agent a question about your data and get a finding, a short written answer with up to four evidence panels, that you can pin to a dashboard or turn into one.
---

An investigation asks the agent a question instead of asking it for a dashboard, such as "why did checkout errors rise after 10:00?".
The agent reads your datasources the way it does for a build, then ends by saving a finding.
It needs a model; see [LLM providers](../connect/llm-providers.md).

## What a finding is

A finding is the agent's answer, written as Markdown of at most 400 words.
It carries up to four evidence panels, each a panel whose query the server ran and checked before the finding was saved.
It covers one window of time, shown under the answer with when it was found.
Every finding is saved with its own address, `/findings/<id>`, which you can share with anyone who can view its dashboard.
A finding that is not about a dashboard is visible only to the person who asked and to Admins.

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

## Read the finding

In the chat, an investigation's turn shows its steps like a build, with the status "Investigating" while it runs.
When it ends, the turn shows the finding: the answer, the evidence panels, and when and over which window it was found.
On a dashboard, your running investigations show in the activity pane as running entries, and each finding about the dashboard appears there as "Agent found" with the same card and an **Open finding** link to its page.

The finding page at `/findings/<id>` shows the title, who found it, the answer and the evidence, with links to the dashboard and to the chat that asked.
The evidence panels always show the finding's own window, not the dashboard's current range.

## Pin it to the dashboard

**Pin to dashboard** adds the finding to its dashboard as an annotation at the end of the finding's window, tagged `finding`, whose text is the finding's first line and a link to it.
Charts on the dashboard then show it as a marker; see [Annotations](annotations.md).
The card then reads "Pinned to the dashboard" with an **Open dashboard** link, and pinning again changes nothing.
Pinning needs Edit on the dashboard.

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
It saves a finding with the `create_finding` tool and reads the saved ones with `list_findings`.
