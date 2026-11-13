---
title: Addresses and viewing tools in the web app
description: The short addresses of the inbox, dashboards, folders, chats, builds, observations, proposals, agents, logs and traces, their title slugs and old forms, selecting a panel to act on it and arranging panels, the tools for viewing a dashboard at full size, in Inspect and on a phone, with a pointer to TV mode, the sidebar, and the light and dark theme.
---

Every page that belongs to one dashboard, folder or agent request has a short address you can paste into an incident, a runbook or a chat:

| Address | Opens |
| --- | --- |
| `/inbox` | Your inbox: what happened on your observations, dashboards and requests. `view` and `item` in the query string reopen a view and an open item. `/` opens it. |
| `/ask` | The chat: ask the Observe Agent for a dashboard. |
| `/observations` | The list of [observations](observations.md). |
| `/o/<observation id>` | One observation, its blocks and its discussion; `rca=1` in the query string opens the Publish as RCA panel. |
| `/proposals/<proposal id>` | One [proposal](../concepts/proposals.md) with its changes, Merge and Discard. |
| `/agents` | The Observe Agent, your connected agents and the service accounts that take runs; `/agents/<ref>` opens one. |
| `/d/<dashboard id>/<slug>` | A dashboard; `/d/<dashboard id>/versions` opens its history, and `thread=<thread id>` opens the activity pane on that comment thread. |
| `/f/<folder id>` | A folder's settings. |
| `/c/<request id>` | The conversation of an agent request. |
| `/b/<request id>` | The live view of an agent request's build. |
| `/logs` | The logs page; `ds`, `q`, `ask`, `from` and `to` in the query string reopen a search or a question. |
| `/traces` | The trace search; `ds`, `service`, `span`, `min`, `max`, `errors`, `q`, `ask`, `from` and `to` in the query string reopen a search or a question. |
| `/t/<trace id>?ds=<datasource uid>` | One trace in a Tempo or ClickHouse datasource. |
| `/p/<token>` | A dashboard opened through a public link, with no sign-in. |
| `/connect/<request id>` | Approve or deny an MCP client that asked to act as you. |

[Logs and traces](logs-and-traces.md) describes the logs page, the trace search and the trace view.

The lists keep their names: `/dashboards` lists dashboards, `/` redirects to `/inbox`, and the chat that used to open at `/` is at `/ask`, with its query string kept.

## Public links

A dashboard's Share pane can create a public link, whose `/p/<token>` address opens the dashboard read-only for anyone who has it, without signing in.
The page reads only the time range, the refresh interval and, on a free link, the variables the viewer may choose from its address.
[Public links](public-links.md) describes who can create one, what the view shows, and how to revoke one.

## Slugs

The slug is the dashboard title in lowercase, with every run of other characters turned into one dash, trimmed, and cut to 60 characters.
Only the letters `a` to `z` and the digits `0` to `9` are kept, so `Checkout health (prod)` becomes `checkout-health-prod` and a title with no such characters has no slug.
The slug is only for people: `/d/<dashboard id>` opens the same dashboard, and so does any address with an old slug.
When a dashboard opens, the address bar switches to the slug of its current title without adding a history entry, so a copied address always carries the title.
A title whose slug would be `versions` gets no slug, because `/d/<dashboard id>/versions` is the history page.

The server writes the same form wherever it returns an address: the `url` fields of the API, the MCP tool results and the `observe` CLI.
The address of a request read with `GET /api/v1/observe/{id}` has no slug, and the page adds it when it opens.

## Old addresses

The old forms keep working and redirect with `301 Moved Permanently`, keeping the query string, so bookmarks, curl and links in old runbooks reach the new address:

| Old address | Redirects to |
| --- | --- |
| `/dashboards/<id>` and `/dashboards/<id>/versions` | `/d/<id>` and `/d/<id>/versions` |
| `/folders/<id>` | `/f/<id>` |
| `/observe/<id>` | `/c/<id>` |
| `/builds/<id>` | `/b/<id>` |

The web app also sends `/health` to `/dashboards?kind=any` and `/import/repair` to `/dashboards?kind=imported`, keeping `kind` and `folder`; these are not server redirects.

The server redirects only when the id is a UUID, and the web app does the same for links it follows itself.
`/dashboards/by-title?title=<title>` is not an id address and keeps working as described in [ObserveSpec](../concepts/observespec.md).
A Grafana dashboard's own address, `/d/<grafana uid>` or `/d/<grafana uid>/<slug>`, redirects with `302 Found` to `/d/<id>` of the dashboard imported from that uid, keeping the slug and the query string.
It resolves only for a signed-in caller who can see the dashboard: a signed-out visitor signs in first and comes back to it, and a uid nobody imported, or whose dashboard is in a folder you cannot see, opens the app's not-found page.

## Dashboards on the Ask page and pins

The chat page at `/ask` lists your dashboards below the chat composer and its example prompts, or above the model note when no model is connected.
It shows Recently viewed: the last five dashboards you opened in this browser.
When you have none, it lists the first eight dashboards you can see, folder by folder, so a fresh demo shows its three provisioned dashboards.
The search box below them finds dashboards by title or description, and `tag:` words narrow it to tags: `tag:prod latency` finds dashboards tagged `prod` that mention `latency`.
Arrow keys move through the results and Enter opens one.

Pin a dashboard with the pin button in its header, labelled **Pin to sidebar**; it then reads **Unpin from sidebar**, and pressing it again unpins the dashboard.
A pinned dashboard appears in the sidebar under **Dashboards**, indented, newest pin first, and the one you have open is highlighted.
The sidebar lists ten pins and then a **More** link to the dashboards list, and the phone menu lists them too; on the icon rail, **Dashboards** opens a menu with a **Dashboards** link and your pins.
Pins belong to you and are stored on the server, so they follow you to every browser, and `GET /api/v1/stars` lists them; the API keeps its original `stars` name.
Only people pin: a service-account key gets `403 user_session_required`.
A pinned dashboard in a folder you can no longer see leaves the sidebar, and deleting a dashboard removes its pins.
Recently viewed dashboards stay in this browser's local storage, at most 20, and never reach the server.

## TV mode

TV mode shows a dashboard on a wall screen with nothing but its panels, and can cycle through its rows or its folder's dashboards.
[Show it on a wall](public-links.md#show-it-on-a-wall) describes how to start it, the address keys that open a TV straight into it, and how it fits with public links.

## Select a panel

A dashboard is a canvas: select a panel, then act on it.
Click the panel's title bar, or Tab to its title and press Enter, and the panel gets a ring and a small toolbar above its top right corner.
`Esc` clears the selection, and with the title focused `↑`, `↓`, `Home` and `End` move it through the panels in reading order.
The toolbar shows only what you may do:

- **Comment**, when you can comment, starts a comment on the panel.
- **Ask @observe**, when you can edit the dashboard and a model is connected, opens the activity pane in investigate mode with the panel as the target.
- **Edit query** opens the panel editor on its query when you can edit the dashboard, and reads **Suggest an edit** when you can only comment.
- **Add to observation** is described in [Add to an observation](#add-to-an-observation).
- **More actions** opens the panel's menu, which every panel also has in its own title bar without selecting it.

The arrow keys move between the toolbar's buttons, and `Home` and `End` go to the first and last.
While you have unsaved layout changes, Comment, Ask, Edit query and Add to observation are greyed.

At the bottom of the screen a tool bar offers **Select**, **Comment** and **Ask**, and it is hidden on a phone, in TV mode and when you can neither comment nor ask, as a Viewer cannot.
With **Comment** pressed, a click on a panel starts a comment on it and goes back to **Select**, and `Esc` goes back to **Select** too.
**Ask** opens the activity pane in investigate mode for the whole dashboard.

### Arrange panels

There is no edit mode.
When you can edit the dashboard, are on its latest version and the dashboard area is at least 768 pixels wide, the selected panel shows a grip in its title bar and a handle in its corner: drag the title bar to move the panel and the corner to resize it.
With the grip focused, the arrow keys move the panel and `Shift` with an arrow resizes it.
While a panel is selected for arranging, the grid shows the saved layout, so a repeated panel shows once and a row cannot be collapsed until you clear the selection.

After the first move a bar at the bottom of the screen says "Your layout changes are not saved".
**Save version N** asks for a summary and saves the layout as the next version, and **Discard** drops the moves.
When someone else saves a version, your moves stay on top of it; a panel you did not touch takes the other version's size and place.
Leaving the dashboard, or opening an older version, with unsaved moves asks first.
While you have unsaved moves, the actions that would change the dashboard in another way, such as Edit details, Duplicate, Delete, Inspect and View, say "finish the layout first".

## Health on a dashboard

When the last health check found a problem in the dashboard, its header has a button that counts the problems, such as **2 problems**.
A dashboard that was checked and has no problem shows a quiet **Healthy** button instead, whose popover says "No problems found" and when and on which version it was checked.
Choose the problem button to open a popover with every problem, when and on which version the dashboard was checked, and **Repair**.
The popover says "before the latest version" when the dashboard has been saved since the check.
**Not opened in 90 days** is never listed here, because you just opened the dashboard.

A panel that failed the check, or returned no data, shows a badge in its title bar with the kind of the problem.
Choose the badge to read the message of every problem in the panel.
The badges show only while you look at the version that was checked.
Another version, such as an older one or the latest one after a save, shows the header button but no badges.
A panel that is also **Unverified** shows only its Unverified badge.

An Editor sees **Repair** in the popover of the header button and in the popover of a badge.
It asks the agent to fix every failing panel and variable on the dashboard in one revision, not only the panel you chose.
The header says **Repair all** when more than one problem can be repaired.
Both places show the same states: **Repair queued**, **Repairing**, **Repaired** with **See changes**, and **Repair failed** with the reason and **Repair** again.
While you have unsaved layout changes, **Repair** is greyed and says "finish the layout first".
Without a configured agent it is greyed too.

The problems come from the last health check; see [Dashboard health](dashboard-health.md).

## One panel at full size

**View** in a panel's menu, or `v` over a panel, shows that panel alone in the whole dashboard area.
The address carries `viewPanel=<panel id>`, so the view can be shared.
**Back to dashboard**, `Esc` or `v` again returns to the grid.

## Inspect

**Inspect** in a panel's menu, or `i` over a panel, opens a pane in the place of the activity card.
The address carries `inspect=<panel id>` and, for the second and third tab, `inspectTab=query` or `inspectTab=stats`.
`Esc` or the close button closes it.
`viewPanel` and `inspect` are dropped while you have unsaved layout changes or the panel editor is open.

The pane has three tabs:

- **Data** shows the panel's results after its transforms as a table of up to 500 rows, with **Download CSV** and **Save as PNG**.
- **Query** shows the time range and step the server ran, and for each query its datasource, step, saved text and the variables it uses with their selected values.
- **Stats** shows how long the request took, whether the server answered from its cache, when it ran, and the number of frames, series, points and rows with the first and last time; **Run again** runs the panel's queries again.

**Download CSV** and **Save as PNG** are also in each panel's menu once its data has loaded.

The CSV has every row, quoted as RFC 4180 describes, with times in RFC 3339 UTC.
When every series of the panel has its own time column with no repeated times, the CSV joins them into one row per time, with one column per series.
Otherwise each row starts with a `series` column naming the series it came from.

The PNG is the panel's body at twice the screen's pixel density, on the page's background colour, named after the panel title.

## Open in Logs or Traces

**Open in Logs** in a logs panel's menu, and **Open in Traces** in a traces panel's menu, open the explorer with the panel's query, its datasource and the dashboard's time range.
The item is greyed, with the reason after it, when the panel has several queries, has no datasource, uses variables in its query, or is a ClickHouse SQL trace search.

## Add to an observation

**Add to observation** in a data panel's menu copies the panel into an [observation](observations.md), the open one you choose or a new one.
The item is greyed, with "(uses dashboard variables)" after it, when a query or the datasource of the panel uses a variable, and it is greyed in the toolbar and missing from the menu while you have unsaved layout changes.
[Observations](observations.md#add-to-an-observation-from-a-panel-or-from-logs) describes the side panel and the range the block takes.

## Sidebar

The sidebar starts with **Home**, then Inbox, Observations, Dashboards, Logs, Traces and Agents, and a Workspace group with Datasources and People for Admins and Settings for everyone.
**Collapse sidebar**, beside your account at the bottom, folds it to a rail of icons, and **Expand sidebar**, above your avatar, opens it again; this browser remembers the choice.
Under 1200 pixels wide, and on dashboard pages, the sidebar is always a rail, and **Expand sidebar** opens the full sidebar over the page.
Hovering or focusing an icon on the rail shows its name.
On the rail, Inbox shows its unread count, Observations shows a green dot while any observation is open, and Dashboards opens a menu that Escape or a click outside closes.

## Search

Press `⌘K` or `Ctrl+K`, or choose **Search or jump to** in the top bar, and type at least two characters.
The command palette lists the pages and your dashboards that match, and after a short pause it adds what the server finds in the workspace: dashboards, observations, comments, annotations and saved searches you can read, under their own headings with the matched words marked.
Press Enter, or click, to open a result here, and hold `⌘` or `Ctrl` to open it in a new tab.
A comment opens the page of what it is on, and an annotation that is on no dashboard is left out, because it has no page.
[Search](search.md) describes what each kind matches and how to write a query.

## Keyboard shortcuts

Dashboards answer `v`, `e`, `i`, `t z`, `t ←`, `t →`, `d r`, `d k` and `?`.
[Keyboard shortcuts](keyboard-shortcuts.md) lists them all.

## On a phone

Below 768 pixels wide a dashboard shows its panels in one column.
Each panel type keeps a minimum height there, so a chart or a table stays readable even when the dashboard made it short.
Up to 760 pixels wide, the time range, refresh, annotation filter and the dashboard's actions move behind one **Controls** button, which opens them in a sheet at the bottom of the screen.
The activity card and the Inspect pane show below the panels, and opening Inspect scrolls to it.

## Connected agents

Open Settings and choose **Connected agents** in its section list to see the MCP clients you approved, each with the name it registered under, the host your browser returned to when you approved it, when it connected and when it last made a request.
**Disconnect** asks first and then ends the connection at once; the client has to be connected again from the agent to work.
The API behind it also returns each connection's presence (working, listening or offline) and when it last asked for work; this section does not show them yet.
The section also shows the command that adds this server to Claude Code.
The **Agents** page in the sidebar lists the same connections next to the Observe Agent and the service accounts, and each can switch between direct and propose mode there.
An Admin sees a second card, **Everyone's connected agents**, with the person each belongs to, and can disconnect any of them.
[Connect your own agent](mcp.md#connect-your-own-agent) describes how a client is approved.

## Theme

The app follows your system's light or dark setting until you choose one.
Open Settings, choose **Appearance** in its section list, and pick **Light**, **Dark** or **Match the system** under **Theme**; the choice applies at once.
The account menu offers the same three choices under **Theme**, and the toolbar's **Switch theme** button and the command palette's **Switch theme** command flip between light and dark.
The choice stays in this browser's local storage and never reaches the server, so it does not follow you to another browser.
