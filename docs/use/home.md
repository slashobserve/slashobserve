---
title: Home
description: The page the web app opens on, where you ask Observe Agent, start an observation or import a Grafana dashboard, and see what needs you, what is running and what you opened last.
---

Home is the first entry in the sidebar, at `/`, and the web app opens on it after you sign in.
The top bar shows today's date, such as "Friday, 13 November", on wide screens.
Its card has three tabs: **Ask Observe Agent**, **New observation** and **Import a dashboard**.
The tab you pick is kept in the address as `tab=observe` or `tab=import`, and the command palette's **Ask Observe Agent**, **New observation** and **Import a dashboard** open these tabs.
An address of `/` with any other query string opens the chat at `/ask` with that query string.

## Ask Observe Agent

The tab holds the chat composer, with the example prompts under the card; sending a prompt opens the chat as on the [chat](chat.md) page.
The prompts follow your datasources: up to three ask for a dashboard, and up to two ask a question about the system, such as "Which services had the most errors in the last hour?", which the agent answers into an observation.
The prompt that adds a row to a dashboard names the first dashboard you can edit, so it never offers a dashboard you may only read.
Under the card, **Recent chats** lists your five newest chats, each with its state, Running, Stopped or Done, and what it saved.
**All chats** opens the History panel beside the page.

## New observation

The tab asks for a **Title** of 1 to 200 characters, an optional **First note** of up to 4,000 characters, and a **Folder**, and **Start observation** makes the observation and opens it.
A non-empty note becomes the observation's first text block.
A link in the note to a logs search (`/logs/<datasource>?q=...`), a trace (`/t/<trace id>?ds=<datasource>`) or a dashboard panel (`/d/<dashboard>?panel=<panel id>`) on this app's own address also becomes a panel block, over the range the link carries, at most 10 links and each once; any other link stays text.
A link that cannot be added, such as a panel that no longer exists or a datasource you cannot read, is skipped, and one message says how many were; the observation still opens.

Where [Observe Agent](chat.md) is set up, **Ask Observe Agent to start** is off by default; turned on, it starts an investigation that writes into the new observation from the note, or from the title when the note is empty, and you see what it finds as blocks you can keep or discard.
If that run cannot start, the observation still exists and a message says the agent did not start.

Under the card, **Your open observations** lists the five open observations you started that changed last, each with how many messages its discussion holds, such as "4 messages", and nothing before the first one; see [Observations](observations.md).

## Import a dashboard

The **Grafana dashboard** field takes the dashboard's JSON, or for an Admin a dashboard link such as `https://grafana.example.com/d/abc123/checkout?orgId=1`; choose **Preview**, or choose or drop a JSON file.
A link needs a [connection](switch-from-grafana.md#connect) to that Grafana in Settings; Home fetches the dashboard through it with `POST /api/v1/grafana/link`, and a failure shows under the field: no connection for the link, not a dashboard link, a dashboard the connection cannot read, or "Grafana did not answer. Try again, or paste the JSON."
Only Admins can import from a link, so everyone else sees "Paste the dashboard's JSON" in the field, and a pasted link answers "Only Admins can import from a link, through a Grafana connection. Paste the dashboard's JSON instead." without calling the server.

The **Folder** field is "Default folder" until you pick one of the folders you can edit.
Home then opens Import from Grafana at `/import` with that dashboard and, when the folder is one you can create dashboards in, with it chosen in the folder card; otherwise the card keeps Automatic; the page is where you pick the datasources, see [Import Grafana dashboards](import-grafana.md).
A dashboard over the 8 MiB import limit stays on Home with "This file is larger than 8 MiB, the import limit."
The JSON stays in the page's memory only, so reloading `/import` starts over.

Under the card, **Recent imports** lists your five newest imports, each with the mark of who imported it and its panel count, such as "18 panels, 2 placeholders" when some panels came in as placeholders.

## Needs you, Working now and Pick up where you left off

**Needs you** shows the three newest items of your [inbox](inbox.md) that have a reason, and **Open Inbox** opens it.
Each row ends with its action: **Review** on a proposal or a suggested block, **Reply** on a mention or a reply, **Open** on a repair you started, and **Try again** on a failed run, which starts the run again and opens the new chat; the action sits beside the row, and the row itself opens the item.
**Working now** lists the agent runs that are queued, running or canceling and that you started or that act on a dashboard or observation you created, whoever started them, and refreshes every five seconds while one is listed.
Each row shows the agent's mark, "<agent> is working on <title>", how long it has run or "Queued", who asked, and the observation or dashboard it works in; a run you started opens its chat, and another person's or agent's run opens the dashboard or observation.
A run on a dashboard or observation you cannot see is listed without its name.
**Pick up where you left off** lists the five dashboards and observations you opened last in this browser; the list lives in this browser's local storage and never reaches the server.
A dashboard row adds its problem count from [dashboard health](dashboard-health.md), such as "1 problem", and an observation row says who changed it last and when, such as "you added a block yesterday".

## Start here

The last section of the page, **Start here**, links the places a newcomer can read without asking the agent anything: **Dashboards**, **Logs**, **Traces**, **Observations** and the **Docs**.
It shows for everyone, so a Viewer who has nothing to work on yet still sees where the team keeps its dashboards and notes.
