---
title: Dashboards as code
description: Export a folder's dashboards to ObserveSpec files, keep them in Git, and apply them back from your machine or a CI job.
---

A dashboard is an ObserveSpec JSON document, so a folder of dashboards fits in a Git repository as a folder of files.
`observe dashboards export` writes a folder's dashboards to files, and `observe dashboards apply` saves the files back as dashboards.
Both run with the [CLI](cli.md) and act with the folder access of the person or key running them.

## Export a folder

```sh
observe dashboards export Payments --dir dashboards
```

Export takes a folder id or a folder title, ignoring case, and writes the latest version of every dashboard in the folder you can see.
Each file holds the Spec as indented JSON with a final newline, so a change shows as a small diff.
It prints one row per file with the file, the dashboard id and the title, or a JSON array with `--json`.
Running it again overwrites the files in place, and leaves alone a file whose dashboard was deleted.

## The directory layout

```text
dashboards/
  checkout.json
  Payments/
    api-latency.json
    checkout.json
  Team %2F Ops/
    on-call.json
```

A file directly in the directory belongs to General, and a file in a subdirectory belongs to the folder named by that directory.
The directory name is the folder title with `%` written as `%25` and `/` as `%2F`, so `Team / Ops` becomes `Team %2F Ops` and any title survives a round trip.
Deeper subdirectories are not allowed, and files and directories whose names start with `.` are skipped.
The server's [provisioning directory](../operate/provisioning.md) uses the same layout.

A dashboard that was applied from a file keeps that file name on export.
Any other dashboard is named after its title in lowercase words joined by `-`, such as `api-latency.json`, or `dashboard.json` for a title with no letters or digits.
When two dashboards in the folder would get the same name, the one with the lower id keeps it and the others get `-2`, `-3` and so on.

## Apply a directory

```sh
observe dashboards apply --dir dashboards
```

Apply sends each `.json` file to the server, one at a time, and prints one row per file with its outcome: `created`, `updated`, `unchanged` or `failed`.
Each save gets the summary `Applied from <path>` in the dashboard's history.
It never deletes a dashboard: removing a file leaves its dashboard in place.

The server finds the dashboard for each file in this order:

1. The dashboard last applied from the same path.
2. Otherwise, the one dashboard in the file's folder whose title matches the Spec's `title`, ignoring case.
   Apply adopts it and remembers the path from then on, so exporting a folder and applying it adopts every dashboard instead of copying it.
3. Otherwise, a new dashboard, in a new folder if the folder does not exist yet.

When the folder holds more than one dashboard with the title, apply refuses the file with the code `ambiguous`; rename one of them or apply the file under the path it was applied from.
A file whose Spec matches the latest version saves nothing and shows `unchanged`, so applying the same directory twice adds no versions.
Every save runs the same checks as a save in the web app, so a file whose queries fail or return no data fails with the panels named.
A dashboard changed in the web app since the last apply is overwritten by the next apply of its file, with the old version kept in its history.

Apply also refuses, with the code `provisioned`, a dashboard that comes from the server's provisioning directory; change that file instead.
A file that is not JSON fails with `invalid_json` before it is sent.
When any file fails, apply still sends the others and then exits 1 with `<n> of <total> files failed`.
`--json` prints the results as a JSON array of objects with `path`, `outcome`, `dashboard_id`, `version`, `code` and `message`.

A key saves only where its role and folder access let it edit; it matches no folder entry, so a restricted folder is out of reach unless the key's account is an Admin or created it (see [Service-account keys](../operate/service-account-keys.md)).
Use an Editor key, because a Viewer or Commenter key cannot save.

## Apply from GitHub Actions

Create an Editor key, save it as a repository secret named `OBSERVE_TOKEN`, and add a workflow that applies the `dashboards` directory on every push to `main`:

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
      - uses: actions/checkout@v5
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
