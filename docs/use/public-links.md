---
title: Public links and TV mode
description: Share one dashboard read-only with people who have no account through a revocable link that can expire, and show a dashboard on a wall screen in TV mode.
---

A public link opens one dashboard read-only for anyone who has the link, with no sign-in.
Use it for a status screen, a vendor, or a wall TV that has no account.
TV mode shows a dashboard full screen on a wall display, for people who do have an account.

## Share a dashboard with a public link

You need Manage on the dashboard's folder; [Who can create a link](#who-can-create-a-link) explains.

1. Open the dashboard and choose **Share**.
2. Under **Public links**, below "Who has access", pick how variables behave.
   **Frozen: the values selected now** stores the variable values in the address you are looking at.
   A variable the address does not set uses the dashboard's default.
   **Free: viewers choose** lets viewers pick values.
3. Pick **Expires**: **No expiry**, **1 day**, **7 days**, **30 days** or **On a date**.
   With a date, the link stops working at the end of that day, local time.
4. Optionally enter a **Label**, up to 80 characters, so the list says what the link is for.
5. Choose **Create public link**.
   The link's address shows once, with **Copy** and "Copy it now. It is not shown again."
6. Send the address to the people who need it.

The link always shows the dashboard's latest version, so an edit reaches every screen that has it open at its next refresh.
A public link shows no snapshot of a past moment.
It cannot be framed in another page, because every response carries `X-Frame-Options: DENY` and `frame-ancestors 'none'`; open it in its own window or screen.

The Share pane lists each link with:

- its prefix and label;
- its variables mode, and the values a frozen link holds;
- its expiry, or "No expiry";
- "Expired", or "Not usable: uses the viewer's identity", when the link no longer opens;
- who created it and when, and when it was last viewed.

## Revoke a link

1. Open the dashboard's **Share** pane.
2. Choose **Revoke** on the link, then confirm.

Revoking deletes the link, so its address stops working at once.
A lost link cannot be shown again, because the server keeps only a hash of the token.
Create a new link and revoke the old one.

## Choose how variables behave

Each link is **Frozen** or **Free**.

- A frozen link always uses the selections stored with it, whatever the address says.
- A free link lets the viewer choose values of query, custom, interval and datasource variables.
  The viewer can choose only among the options the server lists for that variable, so nobody can type in a label value the dashboard does not offer.
- Textbox, adhoc, group by and constant variables keep the stored selections or the dashboard's defaults.
- When you create a free link, the server checks its stored selections against those options.
  A value that is not an option is refused, instead of making every panel fail for the viewer.

The viewer can always change the time range and the refresh interval.
The query proxy's limits bound every query whatever the range.

## Who can create a link

Creating, listing and revoking public links needs Manage on the dashboard's folder, the level that sets the folder's permissions; see [Folders](../teams/folders.md).
A link lets people outside the folder's list see the dashboard, so it needs that level.
Service-account keys with Manage can call the API too.

The agent and MCP clients cannot create or list public links.
Publishing a dashboard outside the team is a person's decision.
A published RCA revision has its own links; see [Share a revision](observations.md#share-a-revision).

A dashboard has at most 20 unexpired links, and an expired link stops counting.

### Dashboards that cannot have a link

- A dashboard in My Dashboards is private until you publish it, so move it to a shared folder first.
- A dashboard whose queries read the viewer's identity through `${__user.*}` has nobody to read on a public link.
  Creating a link for it is refused.
  An existing link stops working, and shows as not usable, once a new version starts using `${__user.*}`.
- A dashboard in a folder only Admins can read stays inside the team, so no link can be created for it.
  The same holds for an RCA revision of an observation there.

Details of Admins-only folders:

- A dashboard with an unexpired link, or an observation whose RCA has one, cannot move into such a folder until the link is revoked.
- Its folder cannot be made Admins only until the link is revoked.
  Both answer `409 admins_only_folder`.
- This way a link never reaches content that reads an [Admins-only datasource](../connect/datasources-in-the-app.md#who-can-query-it).
- A link whose dashboard or observation is in such a folder anyway answers like a link that does not exist.
  The public view and its panel data refuse such a dashboard as well.

## What a public view shows

A public view at `/p/<token>` shows:

- the dashboard's title and its panels;
- the time range and refresh pickers;
- the variables a free link lets the viewer change, while the other variables' values read as one line of text.

It never shows query text, annotations, comments, version history or the agent, and it has no link into the app.
Its dashboard, panel data and variable requests all go to `/api/v1/public/`, so the viewer needs no account.

An address that is unknown, revoked or expired reads "This link has expired or was turned off."
A page that is already open shows the same message as soon as a panel refresh finds the link revoked or expired, without a reload.

Some panels need the signed-in app.
The dashboard list, news and geomap panels show "Not available on a public link" instead.
Remote images in text and canvas panels load through the signed-in assets route, so they do not show on a public view.
Panel data goes through the same query proxy as the app, which runs only the dashboard's saved queries, and each link may have 8 requests in flight.

## Show it on a wall

TV mode shows a dashboard on a wall screen with nothing but its panels.
The top row, the variables bar, the links and the activity card are hidden.

1. Open the dashboard.
2. Choose **TV mode** in the dashboard's **More** menu, or press `d` then `k`.
   Either one also puts the browser in full screen.
3. Press `Esc` to leave both.
   A hint saying so shows for three seconds when TV mode starts and whenever the mouse moves.

When the browser leaves full screen on its own, TV mode ends with it.
A browser goes full screen only after a click or a key press, so TV mode opened from a link starts without it.
It also starts without when the browser refuses full screen.
The hint then also offers `f`, which puts the browser in full screen.

TV mode lives in the address, so a TV can open a bookmark straight into it.
[TV mode addresses](#tv-mode-addresses) lists the query keys.

### Cycle through dashboards

Choose **TV mode for this folder** in the **More** menu to open each dashboard of the same folder in turn, ordered by title.
It is not offered for a personal folder.
A folder cycle keeps the time range and the auto-refresh, and opens each dashboard with its default variable values.
It reads up to 100 dashboards of the folder.

### Sign-in for a TV

TV mode is part of the signed-in app, so the screen signs in as someone who can view the dashboard.
A public link's view at `/p/<token>` has no TV mode and ignores these keys.
A screen without an account opens a public link and shows the whole public view.

## Reference

### TV mode addresses

| Query key | Meaning |
| --- | --- |
| `kiosk=1` | TV mode; `kiosk` alone or `kiosk=true` work too. |
| `cycle=rows` | Bring each expanded row to the top of the screen in turn. |
| `cycle=folder` | Open each dashboard of the same folder in turn, ordered by title. |
| `interval=<seconds>` | Seconds between steps of a cycle. |

Details of `interval`:

- The range is 10 to 3600, and the default is 60.
- A value outside the range is moved to the nearest limit and shown so in the address.
  `interval=5` becomes `interval=10`.

### Limits of a link

| Limit | Value |
| --- | --- |
| Unexpired links per dashboard | 20 |
| Label length | 80 characters |
| Requests in flight per link | 8 |
| Dashboards in a folder cycle | 100 |

### API

Every route except the public ones needs a session or a service-account key with Manage on the dashboard's folder.

| Route | Does |
| --- | --- |
| `GET /api/v1/dashboards/{id}/public-links` | Lists the links. |
| `POST /api/v1/dashboards/{id}/public-links` | Creates a link, with an optional `label`, and returns its token and `url` once. |
| `DELETE /api/v1/dashboards/{id}/public-links/{link_id}` | Revokes a link, and answers 204. |
| `GET /api/v1/public/{token}` | Returns the dashboard for the link; the token is the only credential. |
| `GET /api/v1/public/{token}/panels/{panel_id}/data` | Runs one panel's saved queries for the link. |
| `GET /api/v1/public/{token}/variables/{name}/values` | Lists one variable's values for the link. |

Details of the list: each link has its prefix, label, variables mode, expiry, creator, last view and whether it is usable.
A link that is not usable carries `unusable_reason`: `expired` when its expiry has passed, or `user_scoped` when the dashboard's latest version reads the viewer's identity.
`unusable_reason` is null while the link is usable.
It also has a `state`, `active` or `expired`; see [States](../reference/states.md).

Details of the public view: the answer holds the latest version's Spec with every query's text, the dashboard's tags and its links left out, and the datasources it uses (uid and type).

Create a free link that expires at the end of November 2026:

```sh
curl -s -X POST "$OBSERVE_SERVER/api/v1/dashboards/$DASHBOARD_ID/public-links" \
  -H "Authorization: Bearer $OBSERVE_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"variables": "free", "selections": {"env": ["prod"]}, "expires_at": "2026-12-01T00:00:00Z"}'
```

The answer is 201 with the link, its `token` and its `url`, and the `url` is the address to share.

### API errors

| Status and code | When |
| --- | --- |
| 400 `invalid_request` | `expires_at` is not an RFC 3339 time, or the mode or label is invalid. |
| 403 `forbidden_role` | You do not have Manage. |
| 404 `not_found` | The dashboard or link does not exist or you cannot see it, or a public route got a token that is unknown, revoked or expired. |
| 409 `personal_folder` | The dashboard is in My Dashboards. |
| 409 `admins_only_folder` | The dashboard is in a folder only Admins can read. |
| 422 `user_scoped` | The dashboard reads the viewer's identity. |
| 422 `invalid_expiry` | The expiry is not allowed. |
| 422 `unknown_variable` | A selection names a variable the dashboard lacks. |
| 422 `selection_not_allowed` | A free link's selection is not one of the variable's options. |
| 422 `too_many_links` | The dashboard already has 20 unexpired links. |
| 502 `variable_options_unavailable` | The options could not be read to check the selections. |

Details of `admins_only_folder`:

- The message says to move the dashboard to a folder that is not Admins only.
- When the dashboard reads an Admins-only datasource and so cannot leave, the message says public links are not available for it and names the datasource.
  The answer then also has a `datasource` field.

A free link also answers 400 `selection_not_allowed` on a public route when a request carries a value that is not one of the variable's options.
