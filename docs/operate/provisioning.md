---
title: Provisioning
description: Keep dashboards as ObserveSpec files in a directory, for example a git checkout, and let the server turn each file into a dashboard at startup and whenever the file changes.
---

Provisioning turns a directory of ObserveSpec JSON files into dashboards.
Use it to keep dashboards in a git repository, review changes to them like code, and have the server pick them up without anyone clicking Save.

## Provision a directory

1. Write one [ObserveSpec](../concepts/observespec.md) JSON file for each dashboard, and put the files in a directory.
   A file directly in the directory goes to General, and a file in a subdirectory goes to a folder:

   ```text
   /provision
     overview.json            -> General
     shop/checkout.json       -> folder "shop"
     shop/payments/api.json   -> folder "shop"
   ```

2. Set `SLASHOBSERVE_PROVISION_DIR` to the directory and start the server.
   When the setting is unset, provisioning is off.
3. As an Admin, read the status:

   ```sh
   curl -fsS -H "Authorization: Bearer $OBSERVE_TOKEN" http://localhost:8080/api/v1/provision
   ```

   Each file that became a dashboard is listed under `dashboards`, and each file that did not is under `errors`; [the API](#api) shows the body.
4. Change a file, or `git pull` into the directory.
   The server checks the directory every 30 seconds, comparing each file's path, size and modification time, so the dashboard changes within half a minute.
   To read the directory at once, run `POST /api/v1/provision/run`.

The server reads the directory once at startup and then keeps checking it.
With several servers, one provisions at a time under a Postgres lock, and the others then find each file already saved.
Behind a transaction-mode pooler the lock needs `SLASHOBSERVE_DATABASE_DIRECT_URL`; see [Behind a connection pooler](production.md#behind-a-connection-pooler).

## Choose the folder of a dashboard

Each file whose name ends in `.json` is one dashboard.
A subdirectory names a folder, which the server creates when no folder has that title.
Only the first subdirectory counts: `team/payments/api.json` goes to the `team` folder, because folders are flat.

A directory name cannot hold `/`, so a folder title is written with escapes:

- `%` is written as `%25`.
- `/` is written as `%2F`.
- `\` is written as `%5C`.
- A leading `.` is written as `%2E`.

Nothing else changes, so the directory `Team %2F Payments` is the folder `Team / Payments`.

A subdirectory named General or My Dashboards, or one whose name is not a valid folder title, puts its files in General.
Files and directories whose names start with `.` are skipped, so `.git` is never read, and files with other extensions are ignored.

A file may be a symbolic link, relative as in a Kubernetes ConfigMap mount or absolute, only when it points to a file inside the provisioning directory.
A link that leaves the directory is not read and shows as a failed file.
A file that is not a regular file is skipped.

## Change a dashboard

Edit its file.
For every file, the server hashes its content and compares it with the content it last saved for that path:

- A new path becomes a new dashboard at version 1.
- An unchanged file saves nothing, so a restart or a repeated run adds no version.
- A changed file adds exactly one version on top of the dashboard's latest version, with the summary `Provisioned from <path>`.

An edit someone made in the app since then is replaced by the file's content, and stays in the version history.
Versions are authored by `Provisioning` and their origin is `api`.
Dashboards provisioned from files are ordinary dashboards: people can edit, move and comment on them, and the next change to the file adds a version again.

### Edits made in the app

A dashboard read tells an editor where the dashboard comes from, so the edit does not come as a surprise later:

- `GET /api/v1/dashboards/{id}` returns `origin`: `file` for a dashboard bound to a provisioning file, `applied` for one that `observe dashboards apply` wrote, `starter` for a [starter dashboard](#starter-dashboards) and `editable` for every other dashboard.
  `get_dashboard` returns the same value as `dashboard_origin`.
- The web app shows a note on a `file` dashboard, where an edit starts: in the panel editor and above the prompt.
- A save to a `file` dashboard works, and its answer carries a `warning` that the next change to the file replaces the edit.
  `update_dashboard` and `update_panels` return the same `warning`.

A save checks the file the way every save is checked: the Spec must be valid and every panel and variable query must run.
Unlike a Grafana import, a provisioned file is never saved with failing queries.

A file is known by its path relative to the directory.
Renaming or moving a file is therefore a new file: it becomes a new dashboard, and the dashboard of the old path stays.

## Remove a dashboard

Delete it in the app.
Provisioning never deletes a dashboard.
A file removed from the directory keeps its dashboard, and the server logs `a provisioned file is gone; its dashboard stays` once.

Deleting a provisioned dashboard while its file is still in the directory brings it back as a new dashboard the next time the file is read.
That happens at startup, after a file in the directory changes, or on a manual run.

## Fix a file that fails

A file that cannot be read, does not parse or whose queries fail does not stop the others.
The server logs `provisioning a file failed` at WARN with the file's `path` and an `error_code`, once until the message changes, and keeps the message for the API:

- A file that never became a dashboard is listed under `errors`.
- A file whose dashboard exists but whose new content failed keeps its dashboard at the last saved version, and the message is the entry's `last_error`.

While a file is failing, the server tries it again every 30 seconds, so a dashboard whose datasource was not up at startup appears once its queries answer.
A fixed file clears its error on the next run, and the error of a file removed from the directory is cleared too.

[Error codes](#error-codes) lists what each message means.

## Choose between provisioning and apply

Provisioning reads a directory on the server's own disk, saves as the `Provisioning` account and needs no key.
[`observe dashboards apply`](../use/dashboards-as-code.md) sends files from anywhere, such as a CI job, saves as the key's account with the summary `Applied from <path>`, and follows the same directory layout.

The two keep separate records, so one dashboard belongs to at most one of them.
Apply refuses a dashboard that came from the provisioning directory with 409 `provisioned`, because the next change to the server's file would replace what apply saved.
Change that file instead.

| | Provisioning | Apply |
| --- | --- | --- |
| Where the files are | `SLASHOBSERVE_PROVISION_DIR` on the server | Anywhere the CLI runs |
| Who saves | The `Provisioning` account | The key or person running it, with their folder access |
| When | At startup and within 30 seconds of a change | When the command runs |
| A renamed file | Becomes a new dashboard | Adopts the dashboard with the same title in the folder |
| A removed file | Its dashboard stays | Its dashboard stays |

## Provision on the demo and on Kubernetes

The [demo](demo.md) mounts `deploy/compose/dashboards/Demo` read-only at `/provision/Demo` and sets `SLASHOBSERVE_PROVISION_DIR=/provision`.
Put a Spec file there and it appears in the Demo folder within 30 seconds.
The directory holds `astronomy-overview.json`, the Astronomy Shop overview.
The demo does not load the internals dashboard, because its Prometheus does not scrape the server; [Observe SlashObserve](observe-slashobserve.md) describes `SlashObserve/internals.json`.

The Helm chart mounts one ConfigMap per folder into its provisioning directory; see [Run on Kubernetes](kubernetes.md#provision-dashboards).

## Reference

### API

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

### Error codes

| `error_code` | Message in the status | Meaning |
| --- | --- | --- |
| `file_unreadable` | The file cannot be read. | The file cannot be opened. |
| `invalid_spec` | The spec is invalid, followed by each path and problem. | The file does not parse as a valid Spec, or a query fails. |
| `version_conflict` | The dashboard got a newer version while the file was saved; the next run saves it again. | Someone saved the dashboard while the file was being saved. |
| `forbidden` | The provisioning account cannot save into this folder or dashboard. | The folder or dashboard is out of the account's reach. |
| `restricted_content` | This dashboard reads the datasource, named by its uid in quotes, which only Admins can query; save it in a folder only Admins can read. | The file's folder is not one only Admins can read; see [Who can query a datasource](../reference/datasources-file.md#who-can-query-a-datasource). |
| `internal` | The save failed; the server log has the cause. | Any other failure, a link that leaves the directory included. |

A message is cut at 2048 bytes.

### Starter dashboards

The starter dashboards in the Audit folder are the exception to the rules above.
The server provisions them itself, so they are read-only and a change is refused with 409 `provisioned`.
[Audit trail](audit.md) says what they show.

- At every start, the server adds one version to each starter whose shipped content differs from what is stored, with the summary `Updated to this release`.
  It leaves an unchanged one alone, and several servers starting together add the version once.
- Each such change is recorded as `provision.apply`, and the threads, pins and view counts of the starter stay.
- A starter that someone edited before this behavior existed is kept as a separate dashboard named `<title> (your copy)`, and the starter takes the shipped content.
  The copy and the new version are saved together, so a failed update leaves no copy and the next start tries again.
- A starter that is deleted comes back at the next start, while the Audit folder exists.
  An Admin who wants a changed one duplicates it, and the copy is ordinary.
- A starter does not list a panel that returned no data as unverified, because the audit starters can be empty on a fresh install.
  Its copy lists it like any dashboard.
