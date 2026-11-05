---
title: Annotations
description: Mark deploys, incidents and maintenance on a moment or a range of time, see them as markers on charts, and add them from the CLI or the API.
---

An annotation is a note pinned to a moment or a range of time, such as a deploy, an incident or a maintenance window.
Annotations are stored on the server beside your dashboards and never inside a dashboard's [ObserveSpec](../concepts/observespec.md), so adding one saves no version.
Charts draw them as markers, and you add, list and delete them through the HTTP API.

## Global and dashboard annotations

A global annotation has no dashboard and belongs on every dashboard.
Deploy markers are usually global, because a deploy can move any chart.

A dashboard annotation belongs to one dashboard, and optionally to one panel of it through `panel_id`.
Deleting the dashboard deletes its annotations.

Who may do what:

- Adding a dashboard annotation needs Edit on the dashboard's folder (see [Folders](../teams/folders.md)).
- Adding a global annotation needs the Editor role or higher.
- Reading the annotations of a dashboard needs only that you can see the dashboard; a dashboard you cannot see answers 404, as if it did not exist.
- Deleting an annotation is for the person or key that added it, and for Admins.

## Markers on charts

Time series, candlestick and state timeline panels draw every annotation in their time range.
A moment is a vertical line in the info colour, and a range is a light band between two lines.
A small flag at the bottom of each line opens a card when you hover, focus or tap it.
The card shows the text, the tags, who added it and when, and says `Every dashboard` for a global one.
Markers closer than a few pixels share one flag, and its card lists them newest first.

An annotation with a `panel_id` appears only on that panel, and one without appears on every panel of its dashboard.
Global annotations appear on every dashboard.

A link in the text becomes a link in the card: an `http` or `https` address, or a path on this server such as `/d/<id>`.
Other text, `javascript:` and `mailto:` addresses included, stays plain text.

## Filter markers by tag

When the dashboard's annotations have tags, the top row shows an `Annotations` menu.
Pick one or more tags to keep only the markers that have any of them, or `All annotations` to show every marker again.
The choice is kept in the address as `ann=deploy,incident`, so a shared link shows the same markers.

## Add a deploy marker

Create a [service-account key](../operate/service-account-keys.md) with the Editor role for your CI system, then add a global annotation from the deploy job:

```sh
curl -fsS -X POST "$OBSERVE_SERVER/api/v1/annotations" \
  -H "Authorization: Bearer $OBSERVE_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"text": "Deploy checkout v1.4.2", "tags": ["deploy", "checkout"]}'
```

Without `time_from`, the annotation is placed at the server's current time.
Send `time_from` and `time_to` as RFC 3339 times, such as `2026-11-01T10:00:00Z`, to mark a range instead of a moment.
Add `dashboard_id`, and optionally `panel_id`, to pin the note to one dashboard or panel.

The text is 1 to 2000 characters.

## Tags

Tags group annotations so a dashboard can show only the kinds it cares about.
An annotation has at most 10 tags, each 1 to 40 lowercase letters, digits, `_`, `.` or `-`, starting with a letter or digit.

## List annotations

`GET /api/v1/annotations` lists annotations newest first:

```sh
curl -fsS -H "Authorization: Bearer $OBSERVE_TOKEN" \
  "$OBSERVE_SERVER/api/v1/annotations?dashboard_id=<dashboard id>&from=2026-11-01T00:00:00Z&to=2026-11-02T00:00:00Z&tags=deploy,incident"
```

- `dashboard_id` returns that dashboard's annotations together with the global ones; without it, only the global ones are listed.
- `from` and `to` keep the annotations that overlap the window, so a range that starts before `from` and ends inside it is listed.
- `tags` is comma-separated and keeps the annotations with any of the tags.
- `limit` is 1 to 500 and defaults to 100; pass the `next_cursor` of a page as `cursor` to read the next one, until `next_cursor` is null.
- Each annotation carries `created_by_name`, the name of the person or service account that added it, or null once they are deleted.

## Delete an annotation

```sh
curl -fsS -X DELETE -H "Authorization: Bearer $OBSERVE_TOKEN" "$OBSERVE_SERVER/api/v1/annotations/<annotation id>"
```
