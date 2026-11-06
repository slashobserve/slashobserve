---
title: Dashboard health
description: Find the dashboards that broke after a move from Grafana or rotted since, from panels with no data to dashboards nobody opens, and ask the agent to repair them.
---

Every dashboard is checked about once a day for panels that stopped working, and the health list shows each dashboard with a problem.
It is the list to work through after [switching from Grafana](switch-from-grafana.md), and the one to come back to as datasources and metrics change.

## The Health page

Choose **Health** in the sidebar, or open `<public url>/health`.
The page lists each dashboard with a problem, one row per dashboard, with its folder, every problem found, when it was last checked and on which version, and when anyone last opened it.
Failures come first in each row, then panels with no data, then the dashboard-wide problems.
The menus at the top narrow the list to one kind of problem or one folder, and the address keeps the choice, as `?kind=no_data&folder=<folder id>`, so a filtered list can be shared.

The line under the header shows the state of the job: whether a run is going, and when the last one finished, failed or was interrupted.
An Editor sees **Repair** on each dashboard they can edit, and the button is turned off with the reason when the row has nothing the agent can repair or no agent is configured.
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

`GET /api/v1/health` lists the dashboards with a problem in folders the caller can see, most recently updated first.
It takes `folder_id`, `kind` (one of the kinds above), `limit` (up to 100) and `cursor`, and answers the state of the job beside the list:

```json
{
  "dashboards": [
    {
      "id": "0a1b2c3d-0000-4000-8000-000000000001",
      "title": "Checkout",
      "url": "https://observe.example.com/d/0a1b2c3d-0000-4000-8000-000000000001/checkout",
      "folder": { "id": "0a1b2c3d-0000-4000-8000-0000000000f1", "title": "Shop", "general": false, "personal": false },
      "can_edit": true,
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
      ]
    }
  ],
  "next_cursor": null,
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

An `observe health` CLI command and a health tool for MCP clients are planned on top of this API.
