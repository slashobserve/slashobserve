---
title: Addresses and viewing tools in the web app
description: The short addresses of dashboards, folders, chats, builds, logs and traces, their title slugs and old forms, and the tools for viewing a dashboard on a wall TV, at full size, in Inspect and on a phone.
---

Every page that belongs to one dashboard, folder or agent request has a short address you can paste into an incident, a runbook or a chat:

| Address | Opens |
| --- | --- |
| `/d/<dashboard id>/<slug>` | A dashboard; `/d/<dashboard id>/versions` opens its history. |
| `/f/<folder id>` | A folder's settings. |
| `/c/<request id>` | The conversation of an agent request. |
| `/b/<request id>` | The live view of an agent request's build. |
| `/logs` | The logs page; `ds`, `q`, `from` and `to` in the query string reopen a search. |
| `/traces` | The trace search. |
| `/t/<trace id>?ds=<datasource uid>` | One trace in a Tempo or ClickHouse datasource. |

[Logs and traces](logs-and-traces.md) describes the logs page, the trace search and the trace view.

The lists keep their names: `/dashboards` lists dashboards and `/` is the **Observe** chat home.

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

The server redirects only when the id is a UUID, and the web app does the same for links it follows itself.
`/dashboards/by-title?title=<title>` is not an id address and keeps working as described in [ObserveSpec](../concepts/observespec.md).
Opening a Grafana dashboard's own `/d/<uid>` address will come with the Grafana migration page.

## TV mode

TV mode shows a dashboard on a wall screen with nothing but its panels: the top row, the variables bar, the links and the activity card are hidden.
Start it with **TV mode** in the dashboard's **More** menu, or with `d` then `k`.
Press `Esc` to leave it; a hint saying so shows for three seconds when TV mode starts and whenever the mouse moves.

TV mode lives in the address, so a TV can open a bookmark straight into it:

| Query key | Meaning |
| --- | --- |
| `kiosk=1` | TV mode; `kiosk` alone or `kiosk=true` work too. |
| `cycle=rows` | Bring each expanded row to the top of the screen in turn. |
| `cycle=folder` | Open each dashboard of the same folder in turn, ordered by title. |
| `interval=<seconds>` | Seconds between steps of a cycle, from 10 to 3600; the default is 60. |

**TV mode for this folder** in the **More** menu starts `cycle=folder`, and is not offered for a personal folder.
A folder cycle keeps the time range and the auto-refresh, and opens each dashboard with its default variable values.
It reads up to 100 dashboards of the folder.

## One panel at full size

**View** in a panel's menu, or `v` over a panel, shows that panel alone in the whole dashboard area.
The address carries `viewPanel=<panel id>`, so the view can be shared.
**Back to dashboard**, `Esc` or `v` again returns to the grid.

## Inspect

**Inspect** in a panel's menu, or `i` over a panel, opens a pane in the place of the activity card.
The address carries `inspect=<panel id>` and, for the second and third tab, `inspectTab=query` or `inspectTab=stats`.
`Esc` or the close button closes it.
`viewPanel` and `inspect` are dropped while you edit the dashboard.

The pane has three tabs:

- **Data** shows the panel's results after its transforms as a table of up to 500 rows, with **Download CSV** and **Save as PNG**.
- **Query** shows the time range and step the server ran, and for each query its datasource, step, saved text and the variables it uses with their selected values.
- **Stats** shows how long the request took, whether the server answered from its cache, when it ran, and the number of frames, series, points and rows with the first and last time; **Run again** runs the panel's queries again.

**Download CSV** and **Save as PNG** are also in each panel's menu once its data has loaded.

The CSV has every row, quoted as RFC 4180 describes, with times in RFC 3339 UTC.
When every series of the panel has its own time column with no repeated times, the CSV joins them into one row per time, with one column per series.
Otherwise each row starts with a `series` column naming the series it came from.

The PNG is the panel's body at twice the screen's pixel density, on the page's background colour, named after the panel title.

## Keyboard shortcuts

Dashboards answer `v`, `e`, `i`, `t z`, `t ←`, `t →`, `d r`, `d k` and `?`.
[Keyboard shortcuts](keyboard-shortcuts.md) lists them all.

## On a phone

Below 768 pixels wide a dashboard shows its panels in one column.
Each panel type keeps a minimum height there, so a chart or a table stays readable even when the dashboard made it short.
Up to 760 pixels wide, the time range, refresh, annotation filter and the dashboard's actions move behind one **Controls** button, which opens them in a sheet at the bottom of the screen.
The activity card and the Inspect pane show below the panels, and opening Inspect scrolls to it.
