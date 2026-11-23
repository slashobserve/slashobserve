---
title: Dashboards as code
description: Export a folder's dashboards to ObserveSpec files, keep them in Git, and apply them back from your machine or a CI job.
---

A dashboard is an [ObserveSpec](../concepts/observespec.md) JSON document, so a folder of dashboards fits in a Git repository as a folder of files.
`observe dashboards export` writes a folder's dashboards to files, and `observe dashboards apply` saves the files back as dashboards.
Both run with the [CLI](cli.md) and act with the folder access of the person or key running them.

## Export a folder to files

1. Sign in to the CLI, as [CLI](cli.md#sign-in) shows.
2. Export the folder by its id or its title, ignoring case:

   ```sh
   observe dashboards export Payments --dir dashboards
   ```

   You see one row per file, with the file, the dashboard id and the title:

   ```text
   FILE                       DASHBOARD                             TITLE
   Payments/api-latency.json  3f2a...                               API latency
   Payments/checkout.json     9c41...                               Checkout
   ```

3. Commit the `dashboards` directory to Git.

Export writes the latest version of every dashboard in the folder that you can see.
Each file holds the Spec as indented JSON with a final newline, so a change shows as a small diff.
Running it again overwrites the files in place, and leaves alone a file whose dashboard was deleted.
`--json` prints the rows as a JSON array.
If more than one folder has the title, pass the folder id.

Export always writes into a folder directory, including for General.
[The directory layout](#the-directory-layout) shows it.

## Apply a directory

1. Run apply with the directory:

   ```sh
   observe dashboards apply --dir dashboards
   ```

2. Read the table.
   Each file has one row with its outcome: `created`, `updated`, `unchanged` or `failed`.

   ```text
   FILE                       OUTCOME    DASHBOARD  VERSION  MESSAGE
   Payments/checkout.json     updated    9c41...    4
   Payments/api-latency.json  unchanged  3f2a...    2
   ```

Apply sends each `.json` file to the server, one at a time.
Each save gets the summary `Applied from <path>` in the dashboard's history.
A file whose Spec matches the latest version saves nothing and shows `unchanged`, so applying the same directory twice adds no versions.
Apply never deletes a dashboard: removing a file leaves its dashboard in place.

Exporting a folder and applying it adopts every dashboard instead of copying it.
[How apply finds the dashboard](#how-apply-finds-the-dashboard) has the rules.

A dashboard changed in the web app since the last apply is overwritten by the next apply of its file, with the old version kept in its history.
Every save runs the same checks as a save in the web app, so a file whose queries fail or return no data fails with the panels named.

When any file fails, apply still sends the others and then exits 1 with `<n> of <total> files failed`.
[Why a file fails](#why-a-file-fails) lists the codes.

## Write a file by hand

`observe dashboards sample` prints a complete example Spec.
`observe dashboards schema` prints the JSON Schema every file must satisfy, so an editor or a CI step can validate a file before it is applied.

```sh
observe dashboards sample > dashboards/checkout.json
```

Both are part of the CLI, so they match the Spec your version of `observe` writes and need no server.

## Apply from GitHub Actions

1. Create an Editor key.
   A Viewer or Commenter key cannot save.
2. Save it as a repository secret named `OBSERVE_TOKEN`.
3. Add a workflow that applies the `dashboards` directory on every push to `main`:

   ```yaml
   name: Apply dashboards
   on:
     push:
       branches: [main]
       paths: ["dashboards/**"]
   jobs:
     apply:
       runs-on: ubuntu-latest
       steps:
         - uses: actions/checkout@v7
         - name: Install observe
           run: curl -fsSL https://raw.githubusercontent.com/slashobserve/slashobserve/main/install.sh | OBSERVE_INSTALL_DIR="$HOME/.local/bin" sh
         - name: Apply
           env:
             OBSERVE_SERVER: https://observe.example.com
             OBSERVE_TOKEN: ${{ secrets.OBSERVE_TOKEN }}
           run: observe dashboards apply --dir dashboards
   ```

Set `OBSERVE_VERSION` in the install step to pin a release.
The job fails when any file fails, and its log shows the table with each failed file's message.

A key saves only where its role and folder access let it edit.
It matches no folder entry, so a restricted folder is out of reach unless the key's account is an Admin or created it; see [Service-account keys](../operate/service-account-keys.md).

## Reference

### The directory layout

```text
dashboards/
  checkout.json
  Payments/
    api-latency.json
    checkout.json
  Team %2F Ops/
    on-call.json
```

- A file directly in the directory belongs to General.
- A file in a subdirectory belongs to the folder named by that directory.
- The directory name is the folder title with `%` written as `%25`, `/` as `%2F`, `\` as `%5C` and a leading `.` as `%2E`.
  `Team / Ops` becomes `Team %2F Ops`, so any title survives a round trip.
- Export refuses to write a file outside `--dir`, so a folder titled `..` cannot reach a parent directory.
- Files and directories whose names start with `.` are skipped.
- A path with more than one directory level, such as `Ops/nested/deep.json`, is sent and refused by the server as an invalid path, so its row shows `failed`.
- The server's [provisioning directory](../operate/provisioning.md) uses the same layout.

### File names on export

A dashboard that was applied from a file keeps that file name on export.
Any other dashboard is named after its title in lowercase words joined by `-`, such as `api-latency.json`, or `dashboard.json` for a title with no letters or digits.
When two dashboards in the folder would get the same name, the one with the lower id keeps it and the others get `-2`, `-3` and so on.

### How apply finds the dashboard

The server finds the dashboard for each file in this order:

1. The dashboard last applied from the same path.
2. Otherwise, the one dashboard in the file's folder whose title matches the Spec's `title`, ignoring case.
   Apply adopts it and remembers the path from then on.
3. Otherwise, a new dashboard, in a new folder if the folder does not exist yet.

### Why a file fails

| Code | Meaning |
| --- | --- |
| `invalid_json` | The file is not JSON; the CLI refuses it before sending. |
| `file_unreadable` | The CLI cannot read the file. |
| `invalid_spec` | The Spec fails its checks, with the failing panels named. |
| `ambiguous` | The folder holds more than one dashboard with the title. |
| `provisioned` | The dashboard comes from the server's provisioning directory. |
| `version_conflict` | The dashboard got a newer version during the apply; apply again. |
| `folder_exists` | The folder exists but you cannot see it. |
| `restricted_content` | The Spec reads a datasource only Admins can query, and the folder is open to others. |
| `forbidden_role` | You cannot edit the folder or the dashboard. |
| `not_found` | The dashboard or its folder no longer exists. |
| `invalid_request` | The path is outside the layout. |

Details of the codes:

- For `ambiguous`, rename one of the dashboards, or apply the file under the path it was applied from.
- For `provisioned`, change the provisioning file instead.
- `folder_exists` is the same answer as creating a folder with that name, so apply never says what the folder holds.
  It also never files the dashboard anywhere else.
- A folder you can see but not edit answers `forbidden_role`.

### Exit codes and output

| Situation | Result |
| --- | --- |
| Every file succeeded | Exit 0. |
| Any file failed | The other files are still sent, then exit 1 with `<n> of <total> files failed`. |
| `--dir` missing, or the directory cannot be read | Exit 2 before anything is sent. |
| The directory has no `.json` file | A warning and exit 0. |

`--json` prints the results as a JSON array of objects with `path`, `outcome`, `dashboard_id`, `version`, `code` and `message`.

### API

`POST /api/v1/provision/apply` saves one file and needs the Editor role.
The CLI calls it once per file.

```sh
curl -X POST "$SERVER/api/v1/provision/apply" -H "Authorization: Bearer $OBSERVE_TOKEN" \
  -H 'Content-Type: application/json' \
  -d "{\"path\":\"Payments/checkout.json\",\"spec\":$(cat dashboards/Payments/checkout.json)}"
```

`path` is the file's path relative to the directory, with `/` between parts.
It is a file name or one folder directory and a file name, ends in `.json`, is at most 512 bytes and has no empty or dot-leading part.
`spec` is the file's ObserveSpec.

The answer is `200` with the `path`, the `outcome`, the `dashboard_id` and the `version`.
A refused file answers with an error code:

| Status | Codes |
| --- | --- |
| `409` | `ambiguous`, `provisioned`, `version_conflict`, `folder_exists`, `restricted_content` |
| `422` | `invalid_spec`, with the failing panels |
| `404` | `not_found` |
| `403` | `forbidden_role` |
| `400` | `invalid_request`, for a path outside the layout or a missing `spec` |

`GET /api/v1/provision/applied?folder_id=<folder id>` lists the paths applied to the dashboards now in a folder.
Export uses it to keep their file names, and it needs the Viewer role.
