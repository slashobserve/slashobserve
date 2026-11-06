---
title: Addresses in the web app
description: The short addresses of dashboards, folders, chats, builds, logs and traces, their title slugs, the redirects from the old forms, and how a dashboard fits a phone.
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

## On a phone

Below 768 pixels wide a dashboard shows its panels in one column.
Each panel type keeps a minimum height there, so a chart or a table stays readable even when the dashboard made it short.
Up to 760 pixels wide, the time range, refresh, annotation filter and the dashboard's actions move behind one **Controls** button, which opens them in a sheet at the bottom of the screen.
The activity card shows below the panels.
