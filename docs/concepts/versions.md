---
title: Versions
description: How every save becomes a version, how to see who made it and what changed, restore an old one, and how comments attach to panels.
---

Every save of a dashboard is a new version, numbered from 1 in a single line.
You look at versions to see who changed a dashboard and what changed, and to go back to an earlier state.
A version holds the whole [ObserveSpec](observespec.md) document as it was saved, so no version depends on another, and nothing edits a version after it is written.

For example, Sam renames the dashboard and saves, and it goes from version 4 to version 5 with the summary `Rename to "Checkout latency"`.
Version 4 is still there.
If the rename was a mistake, you restore version 4, and the dashboard becomes version 6 with the contents of version 4.

## Compare versions

1. Open a dashboard and choose **Compare versions** in the version menu.
   The History page lists the versions, newest first, each with **View**, **Compare** and **Restore**.
2. Choose **Compare** on a row to diff it with the version before it.
   To compare any two, pick them in the **From** and **To** menus.
   Version 1 can be compared with an empty dashboard.
3. Read the changes.
   They are grouped under the dashboard, each panel and each variable, and each is marked Added, Removed, Changed or Reordered.
4. Choose **View** to open the dashboard at that version.
   The header then reads "v3 of 5" and not "Live".

Panels are matched by id across versions.
A panel whose title or position changed shows as a change to that panel, not as a removal and an addition.

Over the API, `GET /api/v1/dashboards/{id}/diff?from=3&to=5` returns the same changes as JSON pointers into the document.
`from` defaults to the version before `to`, `to` defaults to the latest, and `from=0` diffs against an empty document.
In the CLI, use `observe dashboards diff <dashboard-id> --from 3 --to 5` and `observe dashboards versions <dashboard-id>`.
Over MCP, use `list_dashboard_versions` and `diff_dashboard_versions`.

## Restore a version

1. Open the History page.
2. Choose **Restore** on any version except the latest, and confirm.
3. The dashboard gets a new latest version that is a copy of the one you chose, marked "Restored from v3", with the summary "Restore version 3".

Nothing is deleted, so the versions in between stay in the history, and you can restore again to go back.
Deleting the dashboard is the only way versions go away: **Delete dashboard** in the **More** menu or a row menu, or `DELETE /api/v1/dashboards/{id}`, deletes every version with it, and it cannot be undone.

A restore checks the document and that its datasources are still configured, but it does not run the queries again.
Restoring a known-good version therefore does not fail because a datasource is briefly down.
The restored version keeps the list of panels and variables that were unverified when it was first saved.

Restoring needs Edit on the dashboard's folder, so a Commenter or Viewer never sees a **Restore** button.
A provisioned starter dashboard refuses a restore with `409` and the code `provisioned`, and a connection in propose mode gets `proposal_required` from `restore_dashboard_version`.
See [Roles](../teams/roles.md) and [Folders and access](../teams/folders.md).
Over the API it is `POST /api/v1/dashboards/{id}/versions/{version}/restore` with the `parent_version` it builds on, and it can answer `409` like any save.
Over MCP it is `restore_dashboard_version`, and in the CLI `observe dashboards restore <dashboard-id> --version 4`.

## Duplicate a dashboard

1. Choose **Duplicate...** in a dashboard's **More** menu, or in its row menu on the Dashboards page.
2. Choose a title, which starts as "<title> copy", and a folder you can create dashboards in.
   The folder starts as the dashboard's own folder when you can create there.
3. Confirm.
   The latest version is saved as version 1 of a new dashboard, and the new dashboard opens.

Duplicating needs View on the dashboard and Edit in the folder you choose.
The copy starts its own history: earlier versions, comments, agent requests and the import record stay with the original, and its version 1 reads "Duplicated from "<title>" version 7".
Like a restore, it checks that the datasources are still configured but does not run the queries again, and it keeps the list of unverified panels and variables.
Over the API it is `POST /api/v1/dashboards/{id}/duplicate` with a `title` and an optional `folder_id`.
Over MCP it is `duplicate_dashboard`, and in the CLI `observe dashboards duplicate <dashboard-id> --title "Checkout copy"`.

## Save over a newer version

A save names the version it builds on.
If someone saved a newer version in the meantime, `POST /api/v1/dashboards/{id}/versions` refuses the save with `409` and the code `version_conflict`, and the response gives `latest_version`.

To merge instead, add `"merge": "auto"` to that request.
`POST /api/v1/dashboards/{id}/panels`, `observe dashboards panels`, `update_dashboard` and `update_panels` have no such option and always try the merge.

The server merges a stale save when it changed different panels, variables or settings than the newer versions did.
It saves the merged document on top of the latest version after its queries run.
The response carries `merged_from` with the version the save was based on and the version it was merged onto.

When the newer versions changed the same panel, variable or setting, the save is refused with `409` and the response lists them under `conflicts`.
Read the latest version, redo your change on it, and save again.

## Comments

A comment is anchored to a panel id and to the version it was written on, or to the whole dashboard.
It does not move when later versions change the panel.
Replies join the thread of the comment they answer and take the same version and panel.

Commenters, Editors and Admins can comment; Viewers cannot.
A thread is open until someone resolves it.
When it is resolved with a version, the thread shows "Resolved by version N", and a revision that the agent made from the thread records its own version there.

Because the anchor is the panel id, keep the id when you change a panel.
A panel with a new id is a different panel to its comments.
If the latest version no longer has the panel, the comment stays but its `panel_present` is `false`.
The agent and MCP clients keep ids when they revise a dashboard.

MCP clients and the CLI can list, reply to and resolve comments; see [MCP](../use/mcp.md) and [CLI](../use/cli.md).

## Reference

### What saves a version

| Action | Version summary |
| --- | --- |
| The first save, which is version 1 | The summary given, from the agent, the web app, the API or a Grafana import. |
| Saving a layout, or a panel from the panel editor | The summary you give. |
| **Duplicate** or **Remove** in a panel's menu | `Duplicate "<title>"` or `Remove "<title>"`. |
| **Edit details** in the **More** menu | `Rename to "<title>"` when only the title changed, otherwise "Edit dashboard details". |
| **Edit Spec JSON** in the **More** menu | The note you give, or "Edit the dashboard JSON". |
| **Add panel** in the **More** menu | `Add "<title>"` unless you add a note. |
| The agent creating or revising a dashboard | Written by the server from what the build changed. |
| `update_dashboard` over MCP | The summary the client sent, or "Update dashboard". |
| `update_panels` over MCP | The summary the client sent, which is required. |
| `observe dashboards apply` | `Applied from <path>`. |
| A file provisioned from disk | `Provisioned from <path>`. |
| `POST /api/v1/dashboards/{id}/versions` | The `summary` in the request. |
| `POST /api/v1/dashboards/{id}/panels` | The `summary` in the request. |
| A restore of an older version | "Restore version N", or the summary given. |

Editing the details asks for no summary.
Saving a layout or a panel asks for a short one.

Details of the agent's versions:

- One version per request, saved when the agent finishes, so the rows it builds along the way never appear in the history.
- Saving what a stopped run built is one version too.
- A build that comes back to an earlier version is recorded as a restore of it.
- The summary is written from what the whole build changed; see [Chat](../use/chat.md).

Details of MCP and API saves:

- A save a connected agent makes this way carries the summary the agent wrote.
  That text is agent-written, and nothing marks it as checked, so read it as the agent's claim.
- `POST .../panels` takes only the panels to add, replace, remove or move.
  It runs only their queries, like `update_panels` and `observe dashboards panels`.

Moving a dashboard to another folder saves no version.
A [proposal](proposals.md) saves none either until a person who can edit merges it.

### What a version records

Each version keeps a summary of 1 to 500 characters, the time, and who or what made it.

- The author, which is a person or a service account.
  When the agent saves a version, the author is the person or key that asked for it.
- The origin, one of `web`, `cli`, `mcp` or `api`, and for `mcp` the client's own name, such as `claude-code`.
  The client's name is what the client reports about itself and is not verified.
  The origin is a label and never decides what someone may do.
- The agent request that saved it, if there was one.
  It is what tells a version saved by an agent from one a person saved, together with the author label below.
- The version it was restored from, if it is a restore.

### The author label

Every author is shown with one label, the same in the web app, the CLI and MCP.

| Saved by | Label |
| --- | --- |
| A person or a service account | Their name, such as `Ishan`. |
| An MCP client outside a run | The name and the client, such as `Ishan (via Claude Code)`, or `Ishan (via MCP)` when the client sent no name. |
| An agent run | `Observe Agent for` and whoever asked, such as `Observe Agent for Ishan`. |

The version menu, the History page and the activity feed show that label.
Versions and dashboards in the API carry it as `actor.label`.
The Dashboards page's **Made by** column shows the label of version 1, then the channel (Web, CLI or API) for a direct save, and its tooltip adds the email or "service account".
The **Made by** filter lists each person with their email, and the dashboard header names the client of a dashboard made over MCP.

### Routes, commands and tools

| Task | HTTP | CLI | MCP |
| --- | --- | --- | --- |
| List versions | `GET /api/v1/dashboards/{id}/versions` | `observe dashboards versions` | `list_dashboard_versions` |
| Read one | `GET /api/v1/dashboards/{id}/versions/{version}` | `observe dashboards show` | `get_dashboard` |
| Compare | `GET /api/v1/dashboards/{id}/diff` | `observe dashboards diff` | `diff_dashboard_versions` |
| Save | `POST /api/v1/dashboards/{id}/versions` | | `update_dashboard` |
| Save some panels | `POST /api/v1/dashboards/{id}/panels` | `observe dashboards panels` | `update_panels` |
| Restore | `POST /api/v1/dashboards/{id}/versions/{version}/restore` | `observe dashboards restore` | `restore_dashboard_version` |
| Duplicate | `POST /api/v1/dashboards/{id}/duplicate` | `observe dashboards duplicate` | `duplicate_dashboard` |

Details of the diff:

- `to` must be at least 1 and `from` at least 0.
- `from` must be lower than `to`, or the answer is `400` `invalid_request`.
- Each change has an `op`, a `path`, and the `from` and `to` values where they apply.

Details of a restore:

- The summary is at most 500 characters over the API and at most 200 over MCP.
- The restore answers `201` with the new version, and `Location` points at it.
