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

## One open proposal for each author

A person or key has at most one open proposal on a dashboard, and a run of the built-in agent has at most one as well.
A later save by the same author does not open another proposal: it adds a revision to the open one.
The new revision keeps what the earlier revisions changed when it can be combined with the new save, taking the new save where both changed the same panel, variable or setting.
When they cannot be combined, the new revision holds the new save alone.

Each revision is kept with the version it builds on.
`revision` on a proposal is the newest one, and a merge names the revision the caller read.

## Behind and conflicts

A proposal is `behind` when the dashboard has a newer version than the one the proposal builds on, and `latest_version` names it.
Behind is no problem by itself: a merge puts the proposal's changes on top of the latest version.

A merge is refused while the proposal and the newer versions changed the same panel, variable or setting.
`GET /api/v1/proposals/{id}` lists these under `conflicts` while the proposal is open and behind, and `changes` lists what the proposal adds, changes, removes or moves compared with the version it builds on.

## Merge

`POST /api/v1/proposals/{id}/merge` needs Edit on the dashboard and a connection that is not in propose mode.
The body names the `revision` the caller read, and `409` with the code `proposal_changed` says the author added a newer one.
For each conflict the body also carries a choice: `ours` keeps the dashboard's newer version of the unit and `theirs` takes the proposal's.

The merge runs the same checks as any save, and the new version records the proposal it came from as `merged_proposal_id`.
When the result equals the latest version, nothing is saved, and the proposal closes as merged with no `merged_version`.

## Discard

`POST /api/v1/proposals/{id}/discard` closes the proposal without saving it.
Its author, and anyone who may merge it, can discard it.
A proposal that is already merged or discarded answers `409` with the code `invalid_transition` and the actions that are allowed.

## States

A proposal is `open`, `merged` or `discarded`.
Its `allowed_actions` list what the caller may do now; `update` is a new save by the author, which has no route of its own.
[States](../reference/states.md) lists the moves and who may make them.

Each step is an entry in [Activity](../use/activity.md): `proposal.opened`, `proposal.updated`, `proposal.merged` and `proposal.discarded`.
Deleting the dashboard deletes its proposals.

## Routes

- `GET /api/v1/dashboards/{id}/proposals` lists the proposals of a dashboard, the open ones unless `state` is `merged`, `discarded` or `all`.
- `GET /api/v1/proposals/{id}` returns one proposal with its document, changes and conflicts.
- `POST /api/v1/proposals/{id}/merge` merges it.
- `POST /api/v1/proposals/{id}/discard` discards it.

MCP and the CLI have no proposal tools or commands yet.
