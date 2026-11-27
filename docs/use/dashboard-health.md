---
title: Dashboard health
description: Find the dashboards that broke after a move from Grafana or rotted since, from panels with no data to dashboards nobody opens, and ask the agent to repair them.
---

Every dashboard is checked about once a day for panels that stopped working, and the health list shows each dashboard with a problem.
Work through that list after [switching from Grafana](switch-from-grafana.md), and come back to it as datasources and metrics change.

## Find dashboards with problems

1. Open **Dashboards** in the sidebar, or go to `<public url>/health`.
   That address opens the dashboards list filtered to dashboards with any problem, and keeps `kind` and `folder`.
2. Read the **Health** column.
   Each dashboard with a problem shows its first problem, "+N more problems" for the others, and the state of its repair.
3. Choose the problem to open a popover with every problem, when and on which version the dashboard was last checked, and **Repair** (**Repair all** when several panels or variables fail).
4. Narrow the list with the **Problem** menu, which holds one kind of problem or any problem.
   Its items are "Query fails" (`query_failed`), "Variable resolves to nothing" (`variable_failed`), "No data" (`no_data`), "Duplicate title" (`duplicate_title`) and "Not opened in 90 days" (`unopened`).

Health shows in two places: the [dashboards list](#in-the-dashboards-list) and [a dashboard](web.md#health-on-a-dashboard).

### In the dashboards list

A dashboard that was checked and has no problem shows a **Queries OK** badge.
Its popover says "No problems found" and when and on which version it was checked.
The badge says only that the panel and variable queries ran.
It does not read the numbers, so a stat past its threshold can sit beside it.

The address keeps the **Problem** choice, as `?kind=<kind or any>&folder=<folder id>`.
The Health column still shows every problem of a dashboard under a filter, because the filter chooses dashboards and not problems.
For Editors, the **Problem** menu also has **Imported, not verified**, at `?kind=imported`, which holds the repair queue of imported dashboards; see [Repair](switch-from-grafana.md#repair).

A summary line above the list counts the dashboards that are failing, have no data, have not been opened in 90 days or have a duplicate title, such as **2 dashboards failing**.
It says when the last check finished or that one is going now.
It shows to everyone when there is something to report, and an Admin always sees it.

## Repair a dashboard

An Editor can ask the agent to repair a dashboard whose last check found a `no_data`, `query_failed` or `variable_failed` problem.

1. Open the problem popover in the dashboards list, or open the dashboard.
2. Choose **Repair** (**Repair all** when several panels or variables fail).
3. Watch the repair state in the Health column.
   It shows Repair queued, Repairing, Repaired, Repair proposed or Repair failed.
   When the check of the repaired version still finds no data, a failing query or a failing variable, **Repair** is offered again.
   Repair proposed means the repair was saved as a proposal instead of a version, for example because you could no longer edit the dashboard when it finished; it waits for an Editor to merge it, and no **Repair** is offered.

The repair is an ordinary revision with a prompt that names each failing panel and variable.
It shows in the dashboard's history and can be reviewed and reverted like any other.
It needs a configured LLM provider and edit access to the dashboard.

A dashboard saved since its last check has nothing to repair until the next check, because the problems on record belong to an older version.
The repair then answers `nothing_to_repair`.
A dashboard with a revision already queued or running is skipped.
Each person can have 20 health repairs queued or running at once.

## Run a check now

An Admin can start a run at once with **Run now** above the dashboards list, for example right after a large import.
It checks the dashboards that are due, as the hourly run does.
The button is turned off while a run is going, and when a run is already going nothing more starts.

## From the CLI

`observe health` lists the same problems in a terminal, in the same ranked order.
`observe health repair <dashboard-id>...` queues repairs, and `observe health run` starts a check.
See [CLI](cli.md).

Its `STATE` column shows each dashboard's state.
Its `REPAIR` column shows the outcome of the latest health repair:

- `repair queued`
- `repair running`
- `repaired in version N`
- `repair saved version N, still failing`
- `repair proposed, waiting for an editor to merge it`
- `repair failed: <error>`
- `repair failed`, when the failure has no sentence
- `-`, when the dashboard never had a repair

A listed dashboard with no problem, such as an `outdated` one kept in the list for its repair, shows one row with `-` as its kind and target.
A line after the table says that outdated dashboards wait for a recheck and that their problems belong to an earlier version.

## From MCP clients

MCP clients call `list_dashboard_health` with a Viewer key or higher.
It answers the same dashboards and problems in the same ranked order, with the state of the job.

- It takes `folder_id`, `kind`, `limit` (up to 50) and `cursor`.
- It takes `dashboard_id` for one dashboard, answered even when it has no problem, with empty `problems`.
- Each dashboard carries `can_edit`, which is false on a starter, and its `state`.
- An outdated dashboard carries a `note` saying that its problems belong to `checked_version` and a recheck is pending.
- The first page also carries the same `summary` counts.
- A dashboard with a health repair carries the repair's `state` and `outcome`, with `version` when it succeeded and `error` when it failed.

An Admin's client can start a run with `run_dashboard_health`, which does what **Run now** does.
A client fixes a failing panel with its own model, reading the panel with `get_panels` and saving the fix with `update_panels`, so the repair needs no LLM provider on the server.
See [MCP](mcp.md).

## Reference

### What counts as a problem

| Kind | Meaning |
| --- | --- |
| `no_data` | A panel query runs but returns nothing over the last 7 days. |
| `query_failed` | A panel query fails. |
| `variable_failed` | A variable resolves to no value, so the panels that use it cannot run. |
| `duplicate_title` | Another dashboard in the same folder has the same title, compared without case. |
| `unopened` | Nobody opened the dashboard in the last 90 days. |

Details of the kinds:

- `no_data` leaves out alert list panels and panels over the built-in audit datasource, because a quiet audit trail is not a fault.
- `query_failed` covers a syntax error, an unknown metric or a datasource that does not answer.
- `unopened` counts from when the dashboard was created if nobody ever opened it.
- The first three come from the health check.
  `duplicate_title` and `unopened` are worked out every time the list is read, so they change as soon as a dashboard is renamed, moved or opened.

An open is one person loading the dashboard page, and the server keeps each person's latest open.
Opens through an API key, the CLI or MCP do not count.

### The health check

A server job named `dashboard_health` runs every hour.

| Rule | Value |
| --- | --- |
| Time a run may spend | 8 minutes |
| Dashboards checked at once | 2 |
| A healthy dashboard is checked | About once a day; a run skips one checked in the last 23 hours |
| Range of each query | The last 7 days |

Details of what a run checks:

- It takes the dashboards never checked first, then the oldest checks.
- No run checks a dashboard more than once.
- A dashboard whose last check found a `query_failed` or `variable_failed` problem is checked again on the next hourly run.
  A brief outage of a datasource stops showing within an hour of the datasource coming back.
- A dashboard saved since its last check is also checked on the next hourly run, so a repair shows its result within an hour.

A check runs every panel and variable query of the dashboard's latest version the way a save does.
It uses the last 7 days instead of the dashboard's own time range, so a dashboard that only shows the last 15 minutes is not reported because the last 15 minutes were quiet.

A check runs with no viewer.
A dashboard whose queries read `${__user.login}`, `${__user.email}` or `${__user.name}` is not checked and is marked `depends_on_viewer`.
A check that cannot run at all is recorded as a `query_failed` problem with the code `check_failed`, and the server log has the cause.

### API

`GET /api/v1/health` lists the dashboards with a problem in folders the caller can see.
It answers the state of the job beside the list.

| Parameter | Meaning |
| --- | --- |
| `folder_id` | Narrow to one folder. |
| `dashboard_id` | Repeats up to 50 times; lists only those dashboards. |
| `kind` | One of the kinds above. |
| `limit` | Up to 100. |
| `cursor` | The `next_cursor` of the previous page. |

Details of `dashboard_id`:

- It lists only the dashboards the caller can see, whether or not they have a problem.
- A checked dashboard with no problem comes back with empty `problems`.
- One that was never checked has `state` `unchecked` with null `checked_at` and `checked_version`.
- With `kind` as well, the list keeps only the dashboards with a problem of that kind.
- `summary` is null on every page when `dashboard_id` is given.
- The web reads a dashboard's Queries OK badge this way.

`GET /api/v1/dashboards?problem=<kind or any>` narrows the dashboards list to the dashboards with that problem by the same rules, keeping its order, paging and `total_count`.

#### Order of the list

The list is ranked, so the dashboards most worth fixing come first.

| Rank | Dashboards |
| --- | --- |
| 0 | a `query_failed` or `variable_failed` problem |
| 1 | a `no_data` problem |
| 2 | a `duplicate_title` problem |
| 3 | only `unopened` |

- Inside a rank, the most recently opened dashboard comes first, never-opened dashboards come last, and ties go by title.
- A `kind` filter keeps each dashboard's rank, so the same order holds inside one kind.
- `next_cursor` pages through that order.
- A cursor from an older server, which ordered by last update, answers 400 `invalid_request`, and the client starts again from the first page.

#### Summary counts

The first page, sent without `cursor`, carries `summary`, and later pages have `summary` set to null.
Each count is of dashboards, not problems, over every dashboard in the folders the caller can see.
`folder_id` narrows it, and `kind` and the page never do.

| Count | Dashboards |
| --- | --- |
| `dashboards` | with any problem, or waiting for a recheck of one |
| `failing` | with a `query_failed` or `variable_failed` problem |
| `query_failed`, `variable_failed`, `no_data` | with that problem |
| `duplicate_titles` | with a `duplicate_title` problem |
| `unopened` | with an `unopened` problem |

#### State of a dashboard

Each dashboard carries a `state`: `unchecked`, `outdated`, `skipped`, `failing` or `healthy`, computed on the server from its last check.
`no_data`, `duplicate_title` and `unopened` stay problems in the list but do not make a dashboard `failing`; see [States](../reference/states.md).

An `outdated` dashboard still lists the problems of the older check, but they belong to `checked_version`.

- The counts above (except `dashboards`) and the `kind` filter leave out its `query_failed`, `variable_failed` and `no_data` problems until the next check.
- The web shows it as **Recheck pending** instead of listing them.
- Without a `kind`, such a dashboard, and one with a health repair on record, stays in the list after the dashboards with current problems.
  A repaired dashboard does not vanish before its next check.
- Its `duplicate_title` and `unopened` problems are current and stay.

#### Repair of a dashboard

Each dashboard carries `repair`, its latest health repair whatever its age.
It is null when the dashboard never had one, and always null for a starter dashboard, which no repair can change.

| Field | Holds |
| --- | --- |
| `state` | The status of the repair's run: `queued`, `running`, `succeeded` or `failed`. |
| `outcome` | What the repair came to: `queued`, `running`, `repaired`, `still_failing`, `proposed` or `failed`. |
| `version` | The version a succeeded repair saved; null otherwise. |
| `error` | The sentence a failed repair ended with; null otherwise. |
| `updated_at` | When the repair finished, else when it started, else when it was queued. |
| `request_id` | The revision request, which `GET /api/v1/dashboards/{id}/revisions/{revision_id}` reads. |

`repaired` means the repaired version was not checked yet, or the check on it, or on a later version, found no `no_data`, `query_failed` or `variable_failed` problem.
`still_failing` means that check still found one of them.
`proposed` means the repair became a proposal instead of a version, and `version` is null.

#### Example answer

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

#### Repairs and runs

`POST /api/v1/health/repairs` takes `{"dashboard_ids": [...]}` with up to 50 dashboards and needs the Editor role.
It answers 202 with the repairs it queued and the dashboards it skipped.

- The skip reasons are `active`, `nothing_to_repair`, `not_found`, `forbidden_role`, `provisioned` and `queue_full`.
- A starter dashboard is skipped as `provisioned`, because the server keeps it current with each release.
  Duplicate it to repair a copy.
- It answers 503 `agent_unavailable` when no LLM provider is configured.

`POST /api/v1/health/run` needs the Admin role and answers 202 while the run starts in the background.
