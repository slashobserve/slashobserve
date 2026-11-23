---
title: Annotations
description: Mark deploys, incidents and maintenance on a moment or a range of time, see them as markers on charts, and add, edit and delete them from the dashboard, the CLI or the API.
---

An annotation is a note pinned to a moment or a range of time, such as a deploy, an incident or a maintenance window.
Annotations are stored on the server beside your dashboards and never inside a dashboard's [ObserveSpec](../concepts/observespec.md), so adding one saves no version.
Charts draw them as markers, you add them from the dashboard, the CLI, the HTTP API or an MCP client, you list them through the CLI, the HTTP API or an MCP client, you edit their text and tags from the dashboard, the CLI, the HTTP API or an MCP client, and you delete them from the dashboard, the HTTP API or the CLI.

## Global and dashboard annotations

A global annotation has no dashboard and belongs on every dashboard.
Deploy markers are usually global, because a deploy can move any chart.

A dashboard annotation belongs to one dashboard, and optionally to one panel of it through `panel_id`.
Deleting the dashboard deletes its annotations.

Who may do what:

- Adding a dashboard annotation needs Edit on the dashboard's folder (see [Folders](../teams/folders.md)).
- Adding a global annotation needs the Editor role or higher.
- Reading the annotations of a dashboard needs only that you can see the dashboard; a dashboard you cannot see answers 404, as if it did not exist.
- Editing or deleting an annotation is for the person or key that added it, and for Admins.

## Markers on charts

Time series, candlestick and state timeline panels draw every annotation in their time range.
A moment is a vertical line in the info colour, and a range is a light band between two lines.
A small flag at the bottom of each line opens a card when you hover, focus or tap it.
A key at the top right of the chart names the flag, as Annotation or Annotations, while the chart shows any.
The card shows the text, the tags, who added it and when, and says `Every dashboard` for a global one.
Times carry the zone they are shown in, such as `2026-11-18 14:22:00 UTC`.
The card opens above the flag and is never cut off by the panel's edge; it closes when you scroll or resize.
Markers closer than a few pixels share one flag, and its card lists all of them newest first and scrolls when they do not fit.

An annotation with a `panel_id` appears only on that panel, and one without appears on every panel of its dashboard.
Global annotations appear on every dashboard.

A link in the text becomes a link in the card: an `http` or `https` address, or a path on this server such as `/d/<id>`.
Other text, `javascript:` and `mailto:` addresses included, stays plain text.

## Add, edit and delete on the dashboard

Anyone with Edit on the dashboard's folder sees an `Add annotation` button in the top row.
It opens a form with what happened, a start time, an optional end time and tags, and puts the annotation on this dashboard.
Times are typed as `2026-11-18 14:22` in the dashboard's time zone, or as `now-15m`; a time before 1970 or after 2099 is refused.
Editors and Admins can also choose `Every dashboard` for a global annotation.

On a time series panel, hold Cmd (Ctrl on Windows and Linux) and click to add an annotation at that moment, or drag to cover a range.
The form opens with the time filled in and the panel as the scope, so the marker appears on that panel only; choose `This dashboard` to show it on all of them.
A state timeline has no chart gesture, so use the button there.

The card of an annotation you added, or any annotation if you are an Admin, has `Edit` and `Delete`.
Edit changes the text and tags; the time, dashboard and author stay.
Delete asks first and cannot be undone.
The `Add annotation` button and the Cmd-click gesture are hidden in kiosk mode and for viewers.

## Filter markers by tag

When the markers in the time range you are looking at have tags, the top row shows an `Annotations` menu on a dashboard that exists; a missing or unreadable dashboard shows none.
The menu lists the tags of the ranges you have opened in this session, so a tag whose markers all lie outside those ranges appears once you widen the range to include one of them.
Pick one or more tags to keep only the markers that have any of them, or `All annotations` to show every marker again.
The choice is kept in the address as `ann=deploy,incident`, so a shared link shows the same markers.

## Add one from the CLI

`observe annotate` adds an annotation from a shell or a CI job, with a [service-account key](../operate/service-account-keys.md) in `OBSERVE_TOKEN`:

```sh
observe annotate "Deploy checkout v1.4.2" --tags deploy,checkout
observe annotate "Database failover" --dashboard <dashboard id> --at now-15m
```

Without `--dashboard` the annotation is global, which needs the Editor role, and `--dashboard` puts it on one dashboard instead.
`--at` places it in the past, as `now-15m` or as an RFC 3339 time, and without it the annotation is placed at the server's current time.
See [CLI](cli.md) for the output and the exit codes.

## Add a deploy marker

Without the CLI, create a [service-account key](../operate/service-account-keys.md) with the Editor role for your CI system, then add a global annotation from the deploy job:

```sh
curl -fsS -X POST "$OBSERVE_SERVER/api/v1/annotations" \
  -H "Authorization: Bearer $OBSERVE_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"text": "Deploy checkout v1.4.2", "tags": ["deploy", "checkout"]}'
```

Without `time_from`, the annotation is placed at the server's current time.
Send `time_from` and `time_to` as RFC 3339 times, such as `2026-11-01T10:00:00Z`, to mark a range instead of a moment.
A time before 1970-01-01 or after 2099-12-31 is refused with 422 `invalid_time`, never replaced by the current time.
Add `dashboard_id`, and optionally `panel_id`, to pin the note to one dashboard or panel; a `panel_id` that is not a panel of the dashboard's latest version is refused with 422 `unknown_panel`.

The text is 1 to 2000 characters.

In GitHub Actions, store the key as a repository secret named `OBSERVE_TOKEN` and add a step after the deploy:

```yaml
- name: Post a deploy marker
  env:
    OBSERVE_SERVER: https://observe.example.com
    OBSERVE_TOKEN: ${{ secrets.OBSERVE_TOKEN }}
  run: |
    curl -fsS -X POST "$OBSERVE_SERVER/api/v1/annotations" \
      -H "Authorization: Bearer $OBSERVE_TOKEN" \
      -H "Content-Type: application/json" \
      -d "{\"text\": \"Deploy ${GITHUB_REPOSITORY} ${GITHUB_SHA::7}\", \"tags\": [\"deploy\"]}"
```

A pipeline that needs a key before anyone signs in can get one from the [service-accounts file](../operate/service-account-keys.md#declare-service-accounts-in-a-file).

## From MCP clients

An MCP client such as Claude Code lists annotations with `list_annotations`, adds one with `create_annotation` and changes the text or tags of one it added with `update_annotation`, so the agent that just deployed can post and correct the marker itself.
`create_annotation` takes the fields of `POST /api/v1/annotations` and needs Edit on the dashboard, or the Editor role for an annotation on every dashboard.
`update_annotation` takes an `id` and `text`, `tags` or both, like `PATCH /api/v1/annotations/{id}`.
The built-in agent reads annotations with `list_annotations` in every run, as the person who asked and only on dashboards they can open, so it can see a deploy beside a spike or say when the last deploy was, and it never adds one.
No tool deletes an annotation.
See [MCP](mcp.md).

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
  It also carries `actor`, with the `label` the web app shows, such as `deploy-bot (via Claude Code)`, or null once they are deleted; `list_annotations` over MCP returns that label as `created_by_label`.

## Edit an annotation

Fix a typo or change the tags without deleting the annotation, so its author and time stay:

```sh
curl -fsS -X PATCH "$OBSERVE_SERVER/api/v1/annotations/<annotation id>" \
  -H "Authorization: Bearer $OBSERVE_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"text": "Deploy checkout v1.4.3", "tags": ["deploy"]}'
```

Send `text`, `tags` or both; a field you leave out stays as it was, and `tags` replaces the old tags.
Only the person or key that added the annotation, or an Admin, may edit it, and its time, dashboard and panel never change.
The change is recorded as `annotation.updated` in the [activity](activity.md) log, without the text.
`observe annotations update <id> --text <text> --tags <tags>` does the same from the CLI.

## Delete an annotation

```sh
curl -fsS -X DELETE -H "Authorization: Bearer $OBSERVE_TOKEN" "$OBSERVE_SERVER/api/v1/annotations/<annotation id>"
```

Only the person or key that added the annotation, or an Admin, may delete it, and it cannot be undone.
`observe annotations delete <id>` does the same from the CLI, and `observe annotations list` lists them; see [CLI](cli.md#annotations).
