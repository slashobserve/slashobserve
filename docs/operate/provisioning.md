---
title: Provisioning
description: Keep dashboards as ObserveSpec files in a directory, for example a git checkout, and let the server turn each file into a dashboard at startup and whenever the file changes.
---

Provisioning turns a directory of ObserveSpec JSON files into dashboards.
Set `SLASHOBSERVE_PROVISION_DIR` to the directory; when it is unset, provisioning is off.
The server reads the directory once at startup and then checks it every 30 seconds, comparing each file's path, size and modification time, so a `git pull` into the directory reaches the dashboards within half a minute.

## Directory layout

Each file whose name ends in `.json` is one dashboard, written as an [ObserveSpec](../concepts/observespec.md) document.
A file directly in the directory goes to General.
A file in a subdirectory goes to the folder named like its first subdirectory, which is created when no folder has that title; `team/payments/api.json` goes to the `team` folder too, because folders are flat.
A directory name cannot hold `/`, so a folder title is written with `%` as `%25` and `/` as `%2F`, and nothing else changes: the directory `Team %2F Payments` is the folder `Team / Payments`.
A subdirectory named General or My Dashboards, or one whose name is not a valid folder title, puts its files in General.
Files and directories whose names start with `.` are skipped, so `.git` is never read, and files with other extensions are ignored.

```text
/provision
  overview.json            -> General
  shop/checkout.json       -> folder "shop"
  shop/payments/api.json   -> folder "shop"
```

A file is known by its path relative to the directory.
Renaming or moving a file is therefore a new file: it becomes a new dashboard, and the dashboard of the old path stays.

## What a run does

For every file, the server hashes its content and compares it with the content it last saved for that path:

- A new path becomes a new dashboard at version 1.
- An unchanged file saves nothing, so a restart or a repeated run adds no version.
- A changed file adds exactly one version on top of the dashboard's latest version, with the summary `Provisioned from <path>`.
  An edit someone made in the app since then is replaced by the file's content, and stays in the version history.

Versions are authored by `Provisioning` and their origin is `api`.
Provisioned dashboards are ordinary dashboards: people can edit, move and comment on them, and the next change to the file adds a version again.

A save checks the file the way every save is checked: the Spec must be valid and every panel and variable query must run.
Unlike a Grafana import, a provisioned file is never saved with failing queries.

A file removed from the directory keeps its dashboard, and the server logs `a provisioned file is gone; its dashboard stays` once.
Provisioning never deletes a dashboard; delete it in the app when it is no longer wanted.
Deleting a provisioned dashboard while its file is still in the directory brings it back as a new dashboard the next time the file is read: at startup, after a file in the directory changes, or on a manual run.

## Errors

A file that cannot be read, does not parse or whose queries fail does not stop the others.
The server logs `provisioning a file failed` at WARN with the file's `path` and an `error_code`, once until the message changes, and keeps the message for the API:

- A file that never became a dashboard is listed under `errors`.
- A file whose dashboard exists but whose new content failed keeps its dashboard at the last saved version, and the message is the entry's `last_error`.

While a file is failing, the server tries it again every 30 seconds, so a dashboard whose datasource was not up at startup appears once its queries answer.
A fixed file clears its error on the next run, and the error of a file removed from the directory is cleared too.

## API

Both endpoints are for Admins only.

`GET /api/v1/provision` answers the status:

```json
{
  "dir": "/provision",
  "dashboards": [
    {
      "path": "shop/checkout.json",
      "dashboard_id": "0a1b2c3d-0000-4000-8000-000000000001",
      "folder": "shop",
      "updated_at": "2026-11-06T09:30:00Z",
      "last_error": null
    }
  ],
  "errors": [
    {
      "path": "broken.json",
      "message": "the file cannot be read",
      "seen_at": "2026-11-06T09:30:00Z"
    }
  ]
}
```

`dir` is null when provisioning is off; the lists stay readable.

`POST /api/v1/provision/run` reads the directory now and answers the same body.
It answers 409 `provisioning_off` when `SLASHOBSERVE_PROVISION_DIR` is unset, and 500 `provision_dir_unreadable` when the directory cannot be read.

## Provisioning and apply

Provisioning reads a directory on the server's own disk, saves as the `Provisioning` account and needs no key.
[`observe dashboards apply`](../use/dashboards-as-code.md) sends files from anywhere, such as a CI job, saves as the key's account with the summary `Applied from <path>`, and follows the same directory layout.

The two keep separate records, so one dashboard belongs to at most one of them.
Apply refuses a dashboard that came from the provisioning directory with 409 `provisioned`, because the next change to the server's file would replace what apply saved; change that file instead.

| | Provisioning | Apply |
| --- | --- | --- |
| Where the files are | `SLASHOBSERVE_PROVISION_DIR` on the server | Anywhere the CLI runs |
| Who saves | The `Provisioning` account | The key or person running it, with their folder access |
| When | At startup and within 30 seconds of a change | When the command runs |
| A renamed file | Becomes a new dashboard | Adopts the dashboard with the same title in the folder |
| A removed file | Its dashboard stays | Its dashboard stays |

## Demo stack

The compose demo mounts `deploy/compose/dashboards` read-only at `/provision` and sets `SLASHOBSERVE_PROVISION_DIR=/provision`.
Put a Spec file there and it appears in the demo within 30 seconds.
