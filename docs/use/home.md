---
title: Home
description: The page the web app opens on, where you ask Observe Agent, start an observation or import a Grafana dashboard, and see what needs you, what is running and what you opened last.
---

Home is the first page you see after you sign in, at `/`.
Use it to start work: ask Observe Agent a question, open an observation to keep notes, or bring in a Grafana dashboard.
Below that, it lists what needs you, what agents are running and what you opened last.

## Ask Observe Agent

1. Choose **Home** in the sidebar.
   The **Ask Observe Agent** tab is open.
2. Type a question or a request for a dashboard, or choose one of the example prompts under the card.
   An example is a button, such as "Which services had the most errors in the last hour?".
3. Send it.
   The chat opens with the answer.
   You can save an answer as an observation; see [Chat and follow-ups](chat.md).

Under the card, **Recent chats** lists your five latest chats.
Each row shows how many turns the chat has and the dashboard it is about.
**All chats** opens the Chats page at `/chats`.

The example prompts follow your datasources, and at most four show.
Up to two ask a question about the system.
Editors and Admins also get prompts that ask for a dashboard.
The prompt that adds a row to a dashboard names one you can edit.

## Start an observation

An observation is a shared note where a team keeps what it finds while it works a question; see [Observations](observations.md).

1. Choose the **New observation** tab.
2. Enter a **Title**.
3. Optionally enter a **First note** and pick a **Folder**.
4. Choose **Start observation**.
   The observation opens, and the note is its first text block.

Links in the note to a logs search, a trace or a dashboard panel on this app's own address become panel blocks.
[New observation](#new-observation) lists the limits and the rules for links.

### Have the agent start the investigation

Where [Observe Agent](chat.md) is set up, the tab shows **Ask Observe Agent to start**, off by default.
Turn it on before you choose **Start observation**.
The agent investigates from the note, or from the title when the note is empty.
You see what it finds as blocks you can keep or discard.
If the run cannot start, the observation still exists and a message says the agent did not start.

Under the card, **Your open observations** lists the five open observations you started that changed last.
Each row shows how many messages its discussion holds, such as "4 messages", and nothing before the first one.

## Import a dashboard

1. Choose the **Import a dashboard** tab.
2. In the **Grafana dashboard** field, paste the dashboard's JSON, or choose or drop a JSON file.
   An Admin can paste a dashboard link instead, such as `https://grafana.example.com/d/abc123/checkout?orgId=1`.
3. Optionally pick a **Folder**.
4. Choose **Preview**.
   Home opens Import from Grafana at `/import` with the dashboard, where you pick the datasources; see [Import Grafana dashboards](import-grafana.md).

Under the card, **Recent imports** lists your five newest imports.
Each row shows who imported it and its panel count, such as "18 panels, 2 placeholders" when some panels came in as placeholders.

### Import from a link

A link needs a [connection](switch-from-grafana.md#connect) to that Grafana in Settings.
Home fetches the dashboard through it with `POST /api/v1/grafana/link`.
A failure shows under the field.
The reasons are no connection for the link, not a dashboard link, a dashboard the connection cannot read, or "Grafana did not answer. Try again, or paste the JSON."

Only Admins can import from a link.
Everyone else sees "Paste the dashboard's JSON" in the field.
A pasted link answers "Only Admins can import from a link, through a Grafana connection. Paste the dashboard's JSON instead." without calling the server.

### Choose the folder

The **Folder** field is "Default folder" until you pick one of the folders you can edit.
When you can create dashboards in that folder, the import page opens with it chosen in the folder card.
Otherwise the card keeps Automatic.

A dashboard over the 8 MiB import limit stays on Home with "This file is larger than 8 MiB, the import limit."
The JSON stays in the page's memory only, so reloading `/import` starts over.

## See what needs you

Three sections sit below the card, whichever tab is open.

**Needs you** shows the three newest items of your [inbox](inbox.md) that have a reason.
**Open Inbox** opens the inbox.
Each row has an action button, and the row itself opens the item.

| Item | Button |
| --- | --- |
| A proposal or a suggested block | **Review** |
| A mention or a reply | **Reply** |
| A repair you started, or a proposal of yours that was closed | **Open** |
| A request for comment access | **Share** |
| A request for access to something in General | **Change role** |
| A failed run | **Try again** |

**Try again** starts the run again and opens the new chat.
A reply the agent wrote reads "Observe Agent answered" followed by the question, as in the inbox.

**Working now** lists agent runs that are queued, running or canceling.
It shows a run when you started it, or when it acts on a dashboard or observation you created, whoever started it.
**All agents** opens the [agents](agents.md) page.
The list refreshes every five seconds while a run is listed.

**Pick up where you left off** lists the five dashboards and observations you opened last in this browser.
A dashboard row adds its problem count from [dashboard health](dashboard-health.md), such as "1 problem".
An observation row says who changed it last and when, such as "you added a block yesterday".

**Start here** links the places a newcomer can read without asking the agent anything: **Dashboards**, **Logs**, **Traces**, **Observations** and **Docs**.
It shows for everyone, so a Viewer who has nothing to work on yet still sees where the team keeps its dashboards and notes.

## Open a dashboard when you sign in

You can choose a dashboard to open on sign-in, as [Home dashboard](web.md#home-dashboard) describes.
That dashboard then opens in place of Home the first time you open `/` without a query string in a browser tab.
If that dashboard was deleted or you can no longer view it, Home opens instead and says so.
**Clear it** drops the choice.

## Reference

### Tabs and addresses

| Tab | Address | Command palette entry |
| --- | --- | --- |
| **Ask Observe Agent** | `/` | **Ask Observe Agent** |
| **New observation** | `/?tab=observe` | **New observation** |
| **Import a dashboard** | `/?tab=import` | **Import a dashboard** |

An address of `/` with any other query string opens the chat at `/ask` with that query string.
The top bar shows today's date, such as "Friday, 13 November", on wide screens.

### New observation

The tab takes these fields.

| Field | Limit |
| --- | --- |
| **Title** | 1 to 200 characters. |
| **First note** | Optional, up to 4,000 characters. |
| **Folder** | The folder the observation lives in. |
| **Ask Observe Agent to start** | Off by default; shown only where the agent is set up. |

Details of links in the note:

- A link to a logs search is `/logs/<datasource>?q=...`.
- A link to a trace is `/t/<trace id>?ds=<datasource>`.
- A link to a dashboard panel is `/d/<dashboard>?panel=<panel id>`.
- Only links on this app's own address become blocks, over the range the link carries.
  Any other link stays text.
- At most 10 links become blocks, and each link once.
- A trace link on a ClickHouse datasource becomes a block that reads the trace by its id, as one Tempo trace does.
- A link to a panel that uses variables keeps their definitions, with the values in the link's `var-` parameters, or the dashboard's defaults without them.

A link that cannot be added is skipped, and one message says how many were and why.
The observation still opens.
The reasons include a panel that no longer exists, a panel that uses a dashboard variable the dashboard no longer defines (`variable_missing`), and what the server answered, such as a datasource you cannot read.

### Lists on Home

| List | Shows | Limit |
| --- | --- | --- |
| **Recent chats** | Chats you asked in, newest first. | 5 |
| **Your open observations** | Open observations you started. | 5, from the 20 newest open ones |
| **Recent imports** | Your imports. | 5 |
| **Needs you** | Inbox items that have a reason. | 3 |
| **Working now** | Queued, running and canceling runs. | 20 |
| **Pick up where you left off** | Dashboards and observations you opened. | 5 |

Details of **Working now**:

- Each row shows the agent's mark and "<agent> is working on <title>".
- It shows how long the run has run, or "Queued", then who asked, then the observation or dashboard it works in.
- A run you started opens its chat.
  Another person's or agent's run opens the dashboard or observation.
- A run on a dashboard or observation you cannot see is listed without its name.

Details of **Pick up where you left off**:

- The list lives in this browser's local storage and never reaches the server.
- It keeps the last 20 dashboards and the last 20 observations you opened, and shows the five newest of them together.
