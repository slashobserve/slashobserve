---
title: Dashboard health
description: Find the dashboards that broke after a move from Grafana or rotted since, from panels with no data to dashboards nobody opens, and ask the agent to repair them.
---

Every dashboard is checked about once a day for panels that stopped working, and the health list shows each dashboard with a problem.
It is the list to work through after [switching from Grafana](switch-from-grafana.md), and the one to come back to as datasources and metrics change.

## The Health page

Choose **Health** in the sidebar, or open `<public url>/health`.
A summary line under the header counts the dashboards that are failing, have no data, have not been opened in 90 days or share a title, over every dashboard in the chosen folder whatever the problem filter, and says when the last check finished or that one is going now.
When no dashboard has a problem, the page says that every dashboard passed its last check, and when.

The list shows one row per dashboard, ranked with failures first, then dashboards with no data, then duplicate titles and dashboards nobody opens; inside each rank, the most recently opened dashboard comes first.
Each row shows the dashboard's folder, its problems, when it was last checked and on which version, and when anyone last opened it.
Problems of one kind with the same message share one line with the number of panels, such as "No data · 13 panels", the message once, and **Show panels** to list them.
On a phone each row is a short list entry with the problems, the times and the action.

The **Problem** and **Folder** menus narrow the list to one kind of problem or one folder, and the address keeps the choice, as `?kind=no_data&folder=<folder id>`, so a filtered list can be shared.
For Editors, the **Problem** menu also has **Imported, not verified**, at `?kind=imported`, which holds the repair queue of imported dashboards with **Repair all**; see [Repair](switch-from-grafana.md#repair).

Each row ends with the next thing to do.
**Repair** shows on a dashboard you can edit that has a problem the agent can repair, and it is turned off with the reason when no agent is configured.
While the repair waits or runs, the row says **Repair queued** or **Repairing**.
After a repair that saved a version, the row says **Repaired** with **See changes**, which opens the versions compare of that version, until the next check reads it.
After a repair that failed, the row says **Repair failed** with the reason, and **Repair** again.
Any other row offers **Open**.
An Admin also sees **Run now**, which is turned off while a run is going; the page follows the run until it finishes.

## What counts as a problem

| Kind | Meaning |
| --- | --- |
| `no_data` | A panel query runs but returns nothing over the last 7 days. |
| `query_failed` | A panel query fails, for example a syntax error, an unknown metric or a datasource that does not answer. |
| `variable_failed` | A variable resolves to no value, so the panels that use it cannot run. |
| `duplicate_title` | Another dashboard in the same folder has the same title, compared without case. |
| `unopened` | Nobody opened the dashboard in the last 90 days, counting from when it was created if nobody ever did. |

The first three come from the health check.
`duplicate_title` and `unopened` are worked out every time the list is read, so they change as soon as a dashboard is renamed, moved or opened.

An open is one person loading the dashboard page; the server keeps each person's latest open.
Opens through an API key, the CLI or MCP do not count.

## The health check

A server job named `dashboard_health` runs every hour.
Each run spends at most 8 minutes and checks at most 2 dashboards at once, taking the dashboards never checked first and then the oldest checks, and skips any dashboard checked in the last 23 hours.
Every dashboard is therefore checked about once a day without one long run.

A check runs every panel and variable query of the dashboard's latest version the way a save does, over the last 7 days instead of the dashboard's own time range, so a dashboard that only shows the last 15 minutes is not reported because the last 15 minutes were quiet.
It runs with no viewer, so a dashboard whose queries read `${__user.login}`, `${__user.email}` or `${__user.name}` is not checked and is marked `depends_on_viewer`.
A check that cannot run at all is recorded as a `query_failed` problem with the code `check_failed`, and the server log has the cause.

## Repair

An Editor can ask the agent to repair a dashboard, with **Repair** on the Health page or the API, when its last check found a `no_data`, `query_failed` or `variable_failed` problem.
The repair is an ordinary revision with a prompt that names each failing panel and variable, so it shows in the dashboard's history and can be reviewed and reverted like any other.
It needs a configured LLM provider and edit access to the dashboard, and a dashboard with a revision already queued or running is skipped.
Each person can have 20 health repairs queued or running at once.

## Run now

An Admin can start a run at once with **Run now**, for example right after a large import.
It checks the dashboards that are due, as the hourly run does, and when a run is already going nothing more starts.

## API

`GET /api/v1/health` lists the dashboards with a problem in folders the caller can see.
It takes `folder_id`, `kind` (one of the kinds above), `limit` (up to 100) and `cursor`, and answers the state of the job beside the list.

The list is ranked, so the dashboards most worth fixing come first:

| Rank | Dashboards |
| --- | --- |
| 0 | a `query_failed` or `variable_failed` problem |
| 1 | a `no_data` problem |
| 2 | a `duplicate_title` problem |
| 3 | only `unopened` |

Inside a rank, the most recently opened dashboard comes first, never-opened dashboards come last, and ties go by title.
A `kind` filter keeps each dashboard's rank, so the same order holds inside one kind.
`next_cursor` pages through that order.
A cursor from an older server, which ordered by last update, answers 400 `invalid_request`, and the client starts again from the first page.

The first page, sent without `cursor`, carries `summary`, and later pages have `summary` set to null.
Each count in it is of dashboards, not problems, over every dashboard in the folders the caller can see, narrowed by `folder_id` and never by `kind` or the page:

| Count | Dashboards |
| --- | --- |
| `dashboards` | with any problem |
| `failing` | with a `query_failed` or `variable_failed` problem |
| `query_failed`, `variable_failed`, `no_data` | with that problem |
| `duplicate_titles` | with a `duplicate_title` problem |
| `unopened` | with an `unopened` problem |

Each dashboard carries a `state`: `unchecked`, `outdated`, `skipped`, `failing` or `healthy`, computed on the server from its last check.
`no_data`, `duplicate_title` and `unopened` stay problems in the list but do not make a dashboard `failing`; see [States](../reference/states.md).

Each dashboard carries `repair`, its latest health repair whatever its age, or null when it never had one.
Its `state` is the status of the repair's run: `queued`, `running`, `succeeded` or `failed`.
Its `outcome` is what the repair came to: `queued`, `running`, `repaired`, `still_failing` or `failed`, where `still_failing` means the repaired version was checked and still fails.
`version` is the version a succeeded repair saved, `error` is the sentence a failed repair ended with, and both are null in the other states.
`updated_at` is when the repair finished, else when it started, else when it was queued.
`request_id` is the revision request, which `GET /api/v1/dashboards/{id}/revisions/{revision_id}` reads.

```json
{
  "dashboards": [
    {
      "id": "0a1b2c3d-0000-4000-8000-000000000001",
      "title": "Checkout",
      "url": "https://observe.example.com/d/0a1b2c3d-0000-4000-8000-000000000001/checkout",
      "folder": { "id": "0a1b2c3d-0000-4000-8000-0000000000f1", "title": "Shop", "general": false, "personal": false },
      "can_edit": true,
      "state": "healthy",
      "checked_version": 4,
      "checked_at": "2026-11-06T03:00:12Z",
      "last_viewed_at": "2026-11-05T17:41:00Z",
      "skipped": null,
      "problems": [
        {
          "kind": "no_data",
          "panel_id": "p_rps",
          "panel_title": "Requests per second",
          "variable": null,
          "code": "query_empty",
          "message": "query returned no data for the dashboard's default time range and variable values"
        }
      ],
      "repair": {
        "state": "succeeded",
        "outcome": "repaired",
        "request_id": "0a1b2c3d-0000-4000-8000-0000000000a1",
        "version": 5,
        "error": null,
        "updated_at": "2026-11-06T03:12:40Z"
      }
    }
  ],
  "next_cursor": null,
  "summary": {
    "dashboards": 1,
    "failing": 0,
    "query_failed": 0,
    "variable_failed": 0,
    "no_data": 1,
    "duplicate_titles": 0,
    "unopened": 0
  },
  "job": {
    "last_started_at": "2026-11-06T03:00:00Z",
    "last_finished_at": "2026-11-06T03:04:31Z",
    "last_outcome": "succeeded",
    "running": false
  }
}
```

`POST /api/v1/health/repairs` takes `{"dashboard_ids": [...]}` with up to 50 dashboards, needs the Editor role, and answers 202 with the repairs it queued and the dashboards it skipped, with the reasons `active`, `nothing_to_repair`, `not_found`, `forbidden_role` and `queue_full`.
It answers 503 `agent_unavailable` when no LLM provider is configured.

`POST /api/v1/health/run` needs the Admin role and answers 202 while the run starts in the background.

## From the CLI

`observe health` lists the same problems in a terminal, in the same ranked order, `observe health repair <dashboard-id>...` queues repairs and `observe health run` starts a check; see [CLI](cli.md).
Its `STATE` column shows each dashboard's state, and its `REPAIR` column shows the outcome of the latest health repair: `repair queued`, `repair running`, `repaired in version N`, `repair saved version N, still failing`, `repair failed: <error>`, `repair failed` when the failure has no sentence, or `-` when it never had one.

## From MCP clients

MCP clients call `list_dashboard_health` with a Viewer key or higher: it takes `folder_id`, `kind`, `limit` (up to 50) and `cursor`, and answers the same dashboards and problems in the same ranked order, with the state of the job.
The first page also carries the same `summary` counts, and each dashboard carries its `state`; one with a health repair carries the repair's `state` and `outcome`, with `version` when it succeeded and `error` when it failed.
A client fixes a failing panel with its own model, reading the panel with `get_panels` and saving the fix with `update_panels`, so the repair needs no LLM provider on the server.
See [MCP](mcp.md).
