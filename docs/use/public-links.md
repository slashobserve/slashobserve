---
title: Public links
description: Share one dashboard read-only with people who have no account, through a revocable link that can expire.
---

A public link opens one dashboard read-only for anyone who has the link, with no sign-in.
Use it for a status screen, a vendor, or a wall TV that has no account.
The link always shows the dashboard's latest version, so an edit reaches every screen that has it open at its next refresh.

## Who can create one

Creating, listing and revoking public links needs Manage on the dashboard's folder, the level that sets the folder's permissions (see [Folders](../teams/folders.md)), because a link lets people outside the folder's list see the dashboard.
Service-account keys with that level can call the API too.
The agent and MCP clients cannot create or list public links: publishing a dashboard outside the team is a person's decision.

Some dashboards cannot have a public link:

- A dashboard in My Dashboards is private until you publish it, so move it to a shared folder first.
- A dashboard whose queries read the viewer's identity through `${__user.*}` has nobody to read on a public link.
  Creating a link for it is refused, and an existing link stops working, and shows as not usable, once a new version starts using `${__user.*}`.

A dashboard has at most 20 links.

## Variables, expiry and revoke

Each link chooses how its variables behave:

- **Frozen** links always use the selections stored with the link, whatever the address says.
- **Free** links let the viewer choose values of query, custom, interval and datasource variables, but only among the options the server lists for that variable, so a viewer cannot type in a label value the dashboard does not offer.
  Textbox, adhoc, group by and constant variables keep the stored selections or the dashboard's defaults.

The viewer can always change the time range and the refresh interval; the query proxy's limits bound every query whatever the range.

A link can expire after a chosen time, and it stops working when it does.
Revoking a link deletes it, so its address stops working at once.
The token in the address is shown once, when the link is created, and the server keeps only a hash of it, so a lost link cannot be shown again: create a new one and revoke the old one.

## What a public view shows

A public view at `/p/<token>` shows the dashboard's title, its panels, the time range and refresh pickers, and the variables a free link lets the viewer change; the other variables' values read as one line of text.
It never shows query text, annotations, comments, version history or the agent, and it has no link into the app.
Its dashboard, panel data and variable requests all go to `/api/v1/public/`, so the viewer needs no account.
The dashboard list, news and geomap panels need the signed-in app, so they show "Not available on a public link" instead.
Remote images in text and canvas panels load through the signed-in assets route, so they do not show on a public view.
Panel data goes through the same query proxy as the app, which runs only the dashboard's saved queries, and each link may have 8 requests in flight.
An address that is unknown, revoked or expired reads "This link does not work. It may have expired or been revoked."

## API

Every route except the public ones needs a session or a service-account key with Manage on the dashboard's folder:

| Route | Does |
| --- | --- |
| `GET /api/v1/dashboards/{id}/public-links` | Lists the links, with their prefix, variables mode, expiry, creator, last view, and whether they are expired or usable. |
| `POST /api/v1/dashboards/{id}/public-links` | Creates a link and returns its token and `url` once. |
| `DELETE /api/v1/dashboards/{id}/public-links/{link_id}` | Revokes a link. |
| `GET /api/v1/public/{token}` | Returns the latest version's Spec with every query's text left out, and the datasources it uses; the token is the only credential. |
| `GET /api/v1/public/{token}/panels/{panel_id}/data` | Runs one panel's saved queries for the link. |
| `GET /api/v1/public/{token}/variables/{name}/values` | Lists one variable's values for the link. |

Create a free link that expires at the end of November 2026:

```sh
curl -s -X POST "$OBSERVE_SERVER/api/v1/dashboards/$DASHBOARD_ID/public-links" \
  -H "Authorization: Bearer $OBSERVE_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"variables": "free", "selections": {"env": ["prod"]}, "expires_at": "2026-12-01T00:00:00Z"}'
```

The answer's `url` is the address to share.
Answers: 403 `forbidden_role` without Manage, 409 `personal_folder` for a dashboard in My Dashboards, 422 `user_scoped`, `invalid_expiry`, `unknown_variable` or `too_many_links`.
A public route answers 404 `not_found` for a token that is unknown, revoked or expired, and a free link answers 400 `selection_not_allowed` for a value that is not one of the variable's options.
