---
title: Versions
description: How every save becomes a version, how to see who made it and what changed, restore an old one, and how comments attach to panels.
---

Every save of a dashboard is a new version, numbered from 1 in a single line.
A version holds the whole [ObserveSpec](observespec.md) document as it was saved, so no version depends on another, and a version is never edited after it is written.

These all save a version:

- The first save, which is version 1, whether it comes from the agent, the web app, the API or a Grafana import.
- A change in the web app: saving a layout, or saving a panel from the panel editor.
  Both ask for a short summary.
- The agent revising a dashboard, and `update_dashboard` from an MCP client.
- `POST /api/v1/dashboards/{id}/versions` over the API.
- A restore of an older version.

Moving a dashboard to another folder saves no version.

## What a version records

Each version keeps a summary of 1 to 500 characters, the time, and who or what made it:

- The author, which is a person or a service account.
  When the agent saves a version, the author is the person or key that asked for it.
- The origin, one of `web`, `cli`, `mcp` or `api`, and for `mcp` the client's own name, such as `claude-code`.
  The client's name is what the client reports about itself and is not verified.
  The origin is a label and never decides what someone may do.
- The agent request that saved it, if there was one.
- The version it was restored from, if it is a restore.

The version menu in the dashboard header lists the five latest versions with their summaries.
It shows "Agent" for a version the built-in agent saved, the MCP client's name for one saved over MCP, and the author's name for the rest.
The History page also lists the author of each version, which for an agent save is the person who asked.
The Dashboards page shows who made version 1, and the header names the client of a dashboard made over MCP.

A save names the version it builds on.
If someone saved a newer version in the meantime, the save is refused with `409` and the code `version_conflict`, and the response gives `latest_version`.

## Compare versions

Choose **Compare versions** in the version menu to open the History page of the dashboard.
It lists the versions, newest first, with **View**, **Compare** and **Restore**, and shows the changes between two versions.
**Compare** on a row diffs it with the version before it, and the **From** and **To** menus pick any two.
Version 1 can be compared with an empty dashboard.

The changes are grouped under the dashboard, each panel and each variable, and each is marked Added, Removed, Changed or Reordered.
Panels are matched by id across versions, so a panel whose title or position changed shows as a change to that panel and not as a removal and an addition.

Over the API, `GET /api/v1/dashboards/{id}/diff?from=&to=` returns the same changes as JSON pointers into the document.
`from` defaults to the version before `to`, `to` defaults to the latest, and `from=0` diffs against an empty document.

**View** opens the dashboard at that version, and the header then reads "v3 of 5" and not "Live".

## Restore a version

**Restore** on any version except the latest asks for confirmation and then saves a copy of that version as a new latest version, marked "Restored from v3".
Nothing is deleted, so the versions in between stay in the history, and you can restore again to go back.
Deleting the dashboard is the only way versions go away: it deletes every version with it.
The summary defaults to "Restore version 3".

A restore checks the document and that its datasources are still configured, but it does not run the queries again.
Restoring a known-good version therefore does not fail because a datasource is briefly down.
The restored version keeps the list of panels and variables that were unverified when it was first saved.

Restoring needs Edit on the dashboard's folder, so a Commenter or Viewer never sees a **Restore** button (see [Roles](../teams/roles.md) and [Folders and access](../teams/folders.md)).
Over the API it is `POST /api/v1/dashboards/{id}/versions/{version}/restore` with the `parent_version` it builds on, and it can answer `409` like any save.
MCP has no restore tool.

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

MCP clients and the CLI can list, reply to and resolve comments (see [MCP](../use/mcp.md) and [CLI](../use/cli.md)).
