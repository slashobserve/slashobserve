---
title: Home
description: The page the web app opens on, where you ask Observe Agent, start an observation or import a Grafana dashboard, and see what needs you, what is running and what you opened last.
---

Home is the first entry in the sidebar, at `/`, and the web app opens on it after you sign in.
Its card has three tabs: **Ask Observe Agent**, **New observation** and **Import a dashboard**.
The tab you pick is kept in the address as `tab=observe` or `tab=import`, and the command palette's **Ask Observe Agent**, **New observation** and **Import a dashboard** open these tabs.
An address of `/` with any other query string opens the chat at `/ask` with that query string.

## Ask Observe Agent

The tab holds the chat composer, with the example prompts under the card; sending a prompt opens the chat as on the [chat](chat.md) page.
Under the card, **Recent chats** lists your five newest chats, each with its state, Running, Stopped or Done, and what it saved.
**All chats** opens the History panel beside the page.

## New observation

The tab asks for a **Title** of 1 to 200 characters and a **Folder**, like the panel on the observations list, and **Create** makes the observation and opens it.
Under the card, **Your open observations** lists the five open observations you started that changed last, each with how many messages its discussion holds, such as "4 messages", and nothing before the first one; see [Observations](observations.md).

## Import a dashboard

Paste a Grafana dashboard's JSON and choose **Preview**, or choose or drop a JSON file.
Home then opens Import from Grafana at `/import` with that dashboard, where you pick the folder and the datasources; see [Import Grafana dashboards](import-grafana.md).
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
