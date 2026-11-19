---
title: Proposals
description: How a change from someone who cannot edit, or from a connection in propose mode, waits as a proposal until a person who can edit merges or discards it.
---

A proposal is a change to a dashboard that is saved for review and not into the dashboard.
It holds a whole [ObserveSpec](observespec.md) document and the version it builds on, and it changes nothing until a person who can edit merges it.
Merging saves it as the next [version](versions.md) of the dashboard.

## Who gets a proposal

A save becomes a proposal when the caller has Comment on the dashboard but not Edit, or when the caller's connection is in propose mode, and the request says `propose_if_needed: true`.
The save is checked as any other save is, so a document that does not parse or whose queries fail is refused with `422` and opens no proposal.
The answer is `202` with the `proposal_id`, the `revision` and whether the save `opened` the proposal.

Without `propose_if_needed`, the same save is refused with `403`.
A Viewer can never propose.
A connected agent in propose mode can never write a version directly, even when its person is an Editor.
It also cannot remove panels or variables: such a save answers `proposal_required` and names them, and nothing is proposed, so the agent asks its person to remove them.

## One open proposal for each author

A person or key has at most one open proposal on a dashboard, and a run of the built-in agent has at most one as well.
A later save by the same author does not open another proposal: it adds a revision to the open one.
The new revision keeps what the earlier revisions changed when it can be combined with the new save, taking the new save where both changed the same panel, variable or setting.
When they cannot be combined, the new revision holds the new save alone.

Each revision is kept with the version it builds on.
`revision` on a proposal is the newest one, and a merge names the revision the caller read.
The proposal's `title` is the newest revision's summary, so the title and the summary under it agree.

## Behind and conflicts

A proposal is `behind` when the dashboard has a newer version than the one the proposal builds on, and `latest_version` names it.
Behind is no problem by itself: a merge puts the proposal's changes on top of the latest version.

A merge is refused while the proposal and the newer versions changed the same panel, variable or setting.
`GET /api/v1/proposals/{id}` lists these under `conflicts` while the proposal is open and behind, and `changes` lists what the proposal adds, changes, removes or moves compared with the version it builds on.
`diff` lists the same comparison field by field, in the shape of `GET /api/v1/dashboards/{id}/diff`, with the old and new value of each field.

## Merge

`POST /api/v1/proposals/{id}/merge` needs Edit on the dashboard and a connection that is not in propose mode.
The body names the `revision` the caller read, and `409` with the code `proposal_changed` says the author added a newer one.
For each conflict the body also carries a choice: `ours` keeps the dashboard's newer version of the unit and `theirs` takes the proposal's.

The merge runs the same checks as any save, and the new version records the proposal it came from as `merged_proposal_id`.
When the result equals the latest version, nothing is saved, and the proposal closes as merged with no `merged_version`; this happens, for example, when every conflict is resolved with `ours`, which drops the proposed unit.

## Discard

`POST /api/v1/proposals/{id}/discard` closes the proposal without saving it.
Its author, and anyone who may merge it, can discard it.
The body may carry a `reason`, trimmed and at most 500 characters (`400` `invalid_request` beyond that); the proposal then shows it as `close_reason`.
The reason is not copied into the activity log, which never carries free text: read it from the proposal.
A proposal that is already merged or discarded answers `409` with the code `invalid_transition` and the actions that are allowed.

## States

A proposal is `open`, `merged` or `discarded`.
Its `allowed_actions` list what the caller may do now; `update` is a new save by the author, which has no route of its own.
[States](../reference/states.md) lists the moves and who may make them.

Each step is an entry in [Activity](../use/activity.md): `proposal.opened`, `proposal.updated`, `proposal.merged` and `proposal.discarded`.
Deleting the dashboard deletes its proposals.

## Review one in the web app

A dashboard with open proposals shows "1 proposal" or "N proposals" in its top row, and the button lists each with its title, its author, when it was opened and "Behind" when the dashboard has moved on.
The count follows the dashboard's live events: when someone opens, updates, merges or discards a proposal, every open copy of the dashboard updates it without a reload (the `proposal.updated` event of the dashboard event stream).
A proposal also opens from the [inbox](../use/inbox.md) and from `/proposals/<id>`.
The page shows the title, who proposed it and when, a line such as "Open, based on version 3. The dashboard is now at version 5.", the summary, the list of changes by panel, "Variable <name>" and "Dashboard settings", and below it the field-by-field difference from the version the proposal builds on, with each query or option before and after, as the [version history](../concepts/versions.md) shows two versions.
A link opens the dashboard at the version the proposal builds on, or at the merged version once it is merged, and the proposal's discussion, a comment thread on the proposal, sits beside the page on a wide window and below it on a narrow one.

When a change conflicts, its row says in words which panel, variable or setting changed on both sides, shows the panel's title on the dashboard and in the proposal, and asks you to choose "Keep the dashboard's" or "Take this proposal's", and **Merge** stays disabled with "Choose a side for each conflict first" until you have chosen for every row.
A change that fails the checks once merged onto the latest version says "This change cannot be merged as it is" and has no choice; discard the proposal or ask its author to fix it.
Someone who cannot edit sees "Only someone who can edit this dashboard can merge a proposal." in place of **Merge**.
**Merge** shows to a person who can edit, and the toast reads "Merged as version N" with **Open dashboard**.
When you kept the dashboard's side of a conflict, the toast adds which panels, variables or settings the proposed change was dropped for, and a merge that saved nothing reads "Merged without a new version".
The agent's merge tool says the same in its step.
**Discard** asks "Discard this proposal?" with an optional **Reason (optional)** field, and shows to the author and to anyone who may merge.
A discarded proposal's page shows the reason under its state line.
If the author added a revision while you were reading, a merge says "This proposal has a newer revision. Review it and merge again.", and the page reloads the proposal.

## Routes

- `GET /api/v1/dashboards/{id}/proposals` lists the proposals of a dashboard, the open ones unless `state` is `merged`, `discarded` or `all`.
  `run_id` narrows it to the proposal that one agent request opened, however many the dashboard has; a run's status on the dashboard uses it, and shows plain "Proposal" until it knows the state.
- `GET /api/v1/proposals/{id}` returns one proposal with its document, changes and conflicts.
- `POST /api/v1/proposals/{id}/merge` merges it.
- `POST /api/v1/proposals/{id}/discard` discards it.

## MCP and the CLI

The MCP tools `update_dashboard` and `update_panels` save a proposal in the cases above and answer `proposed` with the `proposal_id`, and a save on an older version merges onto the latest one when it changed none of the panels, variables and settings that the newer versions changed.
`list_proposals`, `get_proposal`, `merge_proposal` and `discard_proposal` are the routes above as tools; see [MCP](../use/mcp.md).
`observe proposals list`, `show`, `merge` and `discard` do the same from the command line; see [the CLI](../use/cli.md#proposals).
