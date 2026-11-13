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
Under the card, **Your open observations** lists the five open observations you started that changed last; see [Observations](observations.md).

## Import a dashboard

Paste a Grafana dashboard's JSON and choose **Preview**, or choose or drop a JSON file.
Home then opens Import from Grafana at `/import` with that dashboard, where you pick the folder and the datasources; see [Import Grafana dashboards](import-grafana.md).
A dashboard over the 8 MiB import limit stays on Home with "This file is larger than 8 MiB, the import limit."
The JSON stays in the page's memory only, so reloading `/import` starts over.
Under the card, **Recent imports** lists your five newest imports.

## Needs you, Working now and Pick up where you left off

**Needs you** shows the three newest items of your [inbox](inbox.md) that have a reason, and **Open Inbox** opens it.
**Working now** lists the agent requests you started that are queued, running or canceling, newest first, and refreshes every five seconds while one runs.
**Pick up where you left off** lists the five dashboards and observations you opened last in this browser; the list lives in this browser's local storage and never reaches the server.
