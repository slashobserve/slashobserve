---
title: Addresses in the web app
description: The short addresses of dashboards, folders, chats, builds, logs and traces, their title slugs, and the redirects from the old forms.
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

## Home page and stars

The home page lists your dashboards below the chat composer and its example prompts, or above the model note when no model is connected.
Starred dashboards come first, newest star first, then Recently viewed: the last five dashboards you opened in this browser that are not starred.
When you have neither, it lists the first eight dashboards you can see, folder by folder, so a fresh demo shows its three provisioned dashboards.
The search box below them finds dashboards by title or description, and `tag:` words narrow it to tags: `tag:prod latency` finds dashboards tagged `prod` that mention `latency`.
Arrow keys move through the results and Enter opens one.

Star a dashboard with the **Star** button in its header; it then reads **Starred**, and pressing it again removes the star.
Stars belong to you and are stored on the server, so they follow you to every browser, and `GET /api/v1/stars` lists them.
Only people star: a service-account key gets `403 user_session_required`.
A starred dashboard in a folder you can no longer see leaves the list, and deleting a dashboard removes its stars.
Recently viewed dashboards stay in this browser's local storage, at most 20, and never reach the server.
