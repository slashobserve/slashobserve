---
title: Annotations
description: Mark deploys, incidents and maintenance on a moment or a range of time, see them as markers on charts, and add, edit and delete them from the dashboard, the CLI, MCP or the API.
---

An annotation is a note pinned to a moment or a range of time, such as a deploy, an incident or a maintenance window.
Charts draw it as a marker, so you can see a deploy beside the spike it caused.

Annotations are stored on the server beside your dashboards and never inside a dashboard's [ObserveSpec](../concepts/observespec.md).
Adding one saves no version.
You add, edit and delete them from the dashboard, the CLI and the HTTP API, and an MCP client can add, list and edit them.

## Add an annotation on a dashboard

You need Edit on the dashboard's folder; see [Folders](../teams/folders.md).

1. Open the dashboard and choose **Add annotation** in the top row.
   A form opens with the fields **What happened**, **From**, **To (optional)** and **Tags**.
2. Type what happened.
3. Set **From**, as `2026-11-18 14:22` in the dashboard's time zone, or as `now-15m`.
   Leave **To (optional)** empty to mark one moment, or fill it in to mark a range.
4. Optionally add tags, separated by commas or spaces, such as `incident, db`.
5. Optionally choose **Shown on** to pick the scope: **This dashboard**, or **Every dashboard** for a global annotation.
   Only an Editor or an Admin sees **Every dashboard**.
6. Choose **Add**.
   A toast says "Annotation added", and the marker appears on the charts.

A time before 1970 or after 2099 is refused.
The web form lowercases the tags you type.

### Add one from a chart

On a time series panel, hold Cmd (Ctrl on Windows and Linux) and click to add an annotation at that moment, or drag to cover a range.
The form opens with the time filled in and the panel as the scope, so the marker appears on that panel only.
Choose **This dashboard** under **Shown on** to show it on every panel.

A state timeline has no chart gesture, so use the button there.
Kiosk mode and a viewer without Edit hide **Add annotation** and the Cmd-click gesture.

## See the markers

Time series, candlestick and state timeline panels draw every annotation in their time range.
A moment is a vertical line in the info colour, and a range is a light band between two lines.
A small flag at the bottom of each line opens a card when you hover, focus or tap it.

1. Hover or tap a flag.
   The card shows the time, who added it, the text and the tags.
2. For a global annotation, the card also says `Every dashboard`.
3. Move the pointer away, scroll or resize the window to close it.

A key at the top right of the chart names the flag, as Annotation or Annotations, while the chart shows any.
Markers closer than 8 pixels share one flag, and its card lists all of them newest first and scrolls when they do not fit.
The card opens above the flag and is never cut off by the panel's edge.
Times carry the zone they are shown in, such as `2026-11-18 14:22:00 UTC`.

A link in the text becomes a link in the card: an `http` or `https` address, or a path on this server such as `/d/<id>`.
Other text, `javascript:` and `mailto:` addresses included, stays plain text.

An annotation with a `panel_id` appears only on that panel.
One without a panel appears on every panel of its dashboard, and a global one appears on every dashboard.

## Edit or delete an annotation

The card of an annotation you added has **Edit** and **Delete**.
An Admin sees them on every card.

- **Edit** changes the text and the tags.
  The time, dashboard and author stay.
- **Delete** asks first and cannot be undone.

## Show only some markers

When the markers in the time range you are looking at have tags, the top row shows an **Annotations** menu.
A dashboard that does not exist or cannot be read shows none.

1. Open the **Annotations** menu.
   It reads `Annotations: All` until you pick a tag.
2. Pick one or more tags to keep only the markers that have any of them.
3. Pick **All annotations** to show every marker again.

The menu lists the tags of the ranges you have opened in this session.
A tag whose markers all lie outside those ranges appears once you widen the range to include one of them.
The choice is kept in the address as `ann=deploy,incident`, so a shared link shows the same markers.

## Add an annotation from the CLI

`observe annotate` adds an annotation from a shell or a CI job.
It needs a [service-account key](../operate/service-account-keys.md) in `OBSERVE_TOKEN`:

```sh
observe annotate "Deploy checkout v1.4.2" --tags deploy,checkout
observe annotate "Database failover" --dashboard <dashboard id> --at now-15m
```

You see `Added annotation <id> at <time> on every dashboard`, or `on dashboard <id>` when you pass `--dashboard`.

- Without `--dashboard` the annotation is global, which needs the Editor role.
  `--dashboard` puts it on one dashboard instead, and needs Edit on that dashboard.
- `--at` places it in the past, as `now`, `now-15m` or an RFC 3339 time.
  Without it the annotation is placed at the server's current time.
- `--tags` takes comma-separated lowercase tags.

See [CLI](cli.md#annotations) for the output and the exit codes.

## Add a deploy marker from your pipeline

Without the CLI, create a [service-account key](../operate/service-account-keys.md) with the Editor role for your CI system.
Then add a global annotation from the deploy job:

```sh
curl -fsS -X POST "$OBSERVE_SERVER/api/v1/annotations" \
  -H "Authorization: Bearer $OBSERVE_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"text": "Deploy checkout v1.4.2", "tags": ["deploy", "checkout"]}'
```

The answer is `201 Created` with the annotation and a `Location` header.
Without `time_from`, the annotation is placed at the server's current time.
Send `time_from` and `time_to` as RFC 3339 times, such as `2026-11-01T10:00:00Z`, to mark a range instead of a moment.
Add `dashboard_id`, and optionally `panel_id`, to pin the note to one dashboard or panel.

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

## List annotations

`GET /api/v1/annotations` lists annotations newest first:

```sh
curl -fsS -H "Authorization: Bearer $OBSERVE_TOKEN" \
  "$OBSERVE_SERVER/api/v1/annotations?dashboard_id=<dashboard id>&from=2026-11-01T00:00:00Z&to=2026-11-02T00:00:00Z&tags=deploy,incident"
```

The answer is `{"annotations": [...], "next_cursor": ...}`.
Pass the `next_cursor` of a page as `cursor` to read the next one, until `next_cursor` is null.

From the CLI, `observe annotations list` prints the same list as the columns `ID`, `TIME`, `DASHBOARD`, `TAGS` and `TEXT`, with the text cut at 60 characters:

```sh
observe annotations list --dashboard <dashboard id> --from 2026-11-01T00:00:00Z --tag deploy
```

[Parameters of the list](#parameters-of-the-list) has every filter, and [CLI](cli.md#annotations) has the flags.

## Fix the text or tags of an annotation

Change the text or the tags without deleting the annotation, so its author and time stay:

```sh
curl -fsS -X PATCH "$OBSERVE_SERVER/api/v1/annotations/<annotation id>" \
  -H "Authorization: Bearer $OBSERVE_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"text": "Deploy checkout v1.4.3", "tags": ["deploy"]}'
```

Send `text`, `tags` or both.
A field you leave out stays as it was, and `tags` replaces the old tags.
An empty `tags` list removes them all.
Sending neither answers 422 `invalid_request`.

Only the person or key that added the annotation, or an Admin, may edit it.
Its time, dashboard and panel never change.
The change is recorded as `annotation.updated` in the [activity](activity.md) log, without the text.
A change that leaves the text and tags as they were records nothing.

`observe annotations update <id> --text <text> --tags <tags>` does the same from the CLI.
`--tags ""` removes every tag.

## Delete an annotation

```sh
curl -fsS -X DELETE -H "Authorization: Bearer $OBSERVE_TOKEN" "$OBSERVE_SERVER/api/v1/annotations/<annotation id>"
```

The answer is `204 No Content`.
Only the person or key that added the annotation, or an Admin, may delete it, and it cannot be undone.
`observe annotations delete <id>` does the same from the CLI; see [CLI](cli.md#annotations).

## Use annotations from an MCP client

An MCP client such as Claude Code lists annotations with `list_annotations`, adds one with `create_annotation` and changes the text or tags of one it added with `update_annotation`.
The agent that just deployed can then post and correct the marker itself.
No tool deletes an annotation.
See [MCP](mcp.md).

- `create_annotation` takes the fields of `POST /api/v1/annotations`.
  It needs Edit on the dashboard, or the Editor role for an annotation on every dashboard.
- `update_annotation` takes an `id` and `text`, `tags` or both, like `PATCH /api/v1/annotations/{id}`.
- `list_annotations` takes `dashboard_id`, `from`, `to`, `tags`, `limit` and `cursor`, and returns `next_cursor`.
  `limit` is 1 to 100 and defaults to 50, which is smaller than the HTTP list's 500.
  Each annotation carries `created_by_label`, the label the web app shows.

The built-in agent reads annotations with `list_annotations` in every run, as the person who asked and only on dashboards they can open.
It can see a deploy beside a spike or say when the last deploy was, and it never adds one.

## Reference

### Global and dashboard annotations

A global annotation has no dashboard and belongs on every dashboard.
Deploy markers are usually global, because a deploy can move any chart.

A dashboard annotation belongs to one dashboard, and optionally to one panel of it through `panel_id`.
Deleting the dashboard deletes its annotations.

### Who may do what

| Action | What it needs |
| --- | --- |
| Add a dashboard annotation | Edit on the dashboard's folder. |
| Add a global annotation | The Editor role or higher. |
| Read a dashboard's annotations | That you can see the dashboard. |
| Edit or delete | Being the person or key that added it, or an Admin. |

A dashboard you cannot see answers 404, as if it did not exist.
That includes reading, editing and deleting the annotations of a dashboard you cannot see.

### Tags

Tags group annotations so a dashboard can show only the kinds it cares about.
An annotation has at most 10 tags, each 1 to 40 lowercase letters, digits, `_`, `.` or `-`, starting with a letter or digit.
The web form lowercases what you type, while the API, the CLI and MCP refuse an uppercase tag with 422 `invalid_tags`.

### Fields of an annotation

| Field | Holds |
| --- | --- |
| `id` | The annotation's id. |
| `dashboard_id` | The dashboard, or null for a global annotation. |
| `panel_id` | The panel, or null. |
| `time_from` | When it happened or started. |
| `time_to` | When a range ended, or null for a moment. |
| `text` | What happened, 1 to 2000 characters. |
| `tags` | Up to 10 tags. |
| `created_by` | The id of the person or key that added it. |
| `created_by_name` | Their name, or their email or the service account's name, or null once they are deleted. |
| `actor` | Who added it, with the `label` the web app shows, such as `deploy-bot (via Claude Code)`, or null once they are deleted. |
| `created_at` | When it was added. |

### Parameters of the list

| Parameter | Matches |
| --- | --- |
| `dashboard_id` | That dashboard's annotations together with the global ones. Without it, only the global ones. |
| `from` | Annotations that end at or after this RFC 3339 time. |
| `to` | Annotations that start at or before this RFC 3339 time. |
| `tags` | Comma-separated tags, keeping annotations with any of them. |
| `limit` | 1 to 500, 100 by default. |
| `cursor` | The `next_cursor` of the previous page. |

A range that starts before `from` and ends inside the window is listed, because `from` and `to` keep the annotations that overlap it.
A moment counts as a range of one instant.
`to` before `from` answers 400.

### Limits and errors

| Limit | Value |
| --- | --- |
| Text | 1 to 2000 characters. |
| Tags | At most 10. |
| Times | From 1970-01-01 to 2099-12-31. |
| Request body | 16 KiB. |

| Status and code | When |
| --- | --- |
| 400 `invalid_request` | `dashboard_id` is not a UUID, a `from`, `to` or `cursor` does not parse, or the list's `tags` are invalid. |
| 403 | You lack Edit on the dashboard's folder, or the Editor role for a global annotation. |
| 403 `not_creator` | You are neither the person or key that added it nor an Admin. |
| 404 `not_found` | The dashboard or annotation does not exist, or you cannot see it. |
| 422 `invalid_time` | A time is not RFC 3339, lies outside 1970 to 2099, or `time_to` is before `time_from`. It is never replaced by the current time. |
| 422 `invalid_text` | The text is empty or breaks the [text rules](../reference/text-rules.md). |
| 422 `invalid_tags` | More than 10 tags, or a tag outside the pattern. |
| 422 `invalid_panel_id` | `panel_id` is not 1 to 64 characters without NUL, or has no `dashboard_id`. |
| 422 `unknown_panel` | `panel_id` is not a panel of the dashboard's latest version. |
| 422 `invalid_request` | An update gives neither `text` nor `tags`. |
