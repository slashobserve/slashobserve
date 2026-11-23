---
title: Proposals
description: How a change from someone who cannot edit, or from a connection in propose mode, waits as a proposal until a person who can edit merges or discards it.
---

A proposal is a change to a dashboard that is saved for review instead of into the dashboard.
It holds a whole [ObserveSpec](observespec.md) document and the version it builds on, and it changes nothing until a person who can edit merges it.
Merging saves it as the next [version](versions.md) of the dashboard.

For example, Sam has Comment access to the Checkout dashboard but cannot edit it.
Sam asks the built-in agent to add a latency panel.
The dashboard stays as it is, and a proposal titled "Add p95 latency panel" appears in its top row as "1 proposal".
An Editor opens it, reads the changes, and chooses **Merge**, which saves version 6 of the dashboard.

## Review a proposal

1. Open the dashboard.
   When it has open proposals, the top row shows "1 proposal" or "N proposals", with "N+" when more than one page of them is open.
2. Choose that button, then choose a proposal.
   The list shows each title, its author, when it was opened and "Behind" when the dashboard has moved on.
   A proposal also opens from the [inbox](../use/inbox.md) and from `/proposals/<id>`.
3. Read the page.
   It shows the title, who proposed it and when, a line such as "Open, based on version 3. The dashboard is now at version 5.", the summary, and the list of changes by panel, "Variable <name>" and "Dashboard settings".
   Below that, a field-by-field difference from the version the proposal builds on shows each query or option before and after, as the [version history](versions.md) shows two versions.
4. Choose **Merge**, or choose **Discard** and optionally give a **Reason (optional)**.
   A toast reads "Merged as version N" with **Open dashboard**.

A link on the page opens the dashboard at the version the proposal builds on, or at the merged version once it is merged.
The proposal's discussion, a comment thread on the proposal, sits beside the page on a wide window and below it on a narrow one.
The count in the top row follows the dashboard's live events, so every open copy of the dashboard updates it without a reload.

You cannot merge when you lack Edit on the dashboard.
In that case the page says "Only someone who can edit this dashboard can merge a proposal." in place of **Merge**.
The author still sees **Discard**, with the note "You can discard this one because you opened it."

## Resolve a conflict

A proposal can be behind: the dashboard has a newer version than the one the proposal builds on, and `latest_version` names it.
Behind is no problem by itself, because a merge puts the proposal's changes on top of the latest version.

A conflict exists when the proposal and a newer version changed the same panel, variable or setting.
The merge is refused until you choose a side for each conflict.

1. Open the proposal.
   Each conflicting row says in words which panel, variable or setting changed on both sides, and shows the panel's title on the dashboard and in the proposal.
2. Choose "Keep the dashboard's" or "Take this proposal's" for every row.
   **Merge** stays disabled with "Choose a side for each conflict first" until you have chosen for every row.
3. Choose **Merge**.
   When you kept the dashboard's side of a conflict, the toast adds which panels, variables or settings the proposed change was dropped for.

A change that fails the checks once merged onto the latest version says "This change cannot be merged as it is" and has no choice.
Discard the proposal or ask its author to fix it.

If the author added a revision while you were reading, the merge says "This proposal has a newer revision. Review it and merge again." and the page reloads the proposal.
A merge that saved nothing reads "Merged without a new version".
That happens when the result equals the latest version, for example when you resolve every conflict with the dashboard's side.

## Make a proposal

You do not create a proposal with its own route.
You save, and the save becomes a proposal when you cannot save directly.

1. Send the save with `propose_if_needed: true`:

   ```sh
   curl -fsS -X POST -H "Authorization: Bearer $OBSERVE_TOKEN" \
     -H "Content-Type: application/json" \
     "$OBSERVE_SERVER/api/v1/dashboards/$DASHBOARD_ID/versions" \
     -d '{"parent_version": 5, "summary": "Add p95 latency panel", "propose_if_needed": true, "spec": {...}}'
   ```

2. When you have Comment but not Edit, or your connection is in propose mode, the answer is `202`.
   The body has `proposal_id`, `revision` and `opened`, and the `Location` header names the proposal.
3. Read the proposal with `GET /api/v1/proposals/{id}`.

The save is checked as any other save is.
A document that does not parse or whose queries fail is refused with `422` and opens no proposal.

`POST /api/v1/dashboards/{id}/panels` takes `propose_if_needed` too.
The MCP tools `update_dashboard` and `update_panels` always propose when they must, and answer `proposed` with the `proposal_id`.
The built-in agent does the same.

## Change a proposal you opened

Save again.
A person or key has at most one open proposal on a dashboard, and a run of the built-in agent has at most one as well.
A later save by the same author adds a revision to the open proposal and does not open another.

The new revision keeps what the earlier revisions changed when it can be combined with the new save.
Where both changed the same panel, variable or setting, it takes the new save.
When they cannot be combined, the new revision holds the new save alone.

Each revision is kept with the version it builds on.
`revision` on a proposal is the newest one, and a merge names the revision the caller read.
The proposal's `title` is the newest revision's summary, so the title and the summary under it agree.

## Merge or discard from the API, MCP and the CLI

`POST /api/v1/proposals/{id}/merge` needs Edit on the dashboard and a connection that is not in propose mode.

1. Read the proposal and note its `revision` and its `conflicts`.
2. Merge, with one choice per conflict:

   ```sh
   curl -fsS -X POST -H "Authorization: Bearer $OBSERVE_TOKEN" \
     -H "Content-Type: application/json" \
     "$OBSERVE_SERVER/api/v1/proposals/$PROPOSAL_ID/merge" \
     -d '{"revision": 2, "choices": [{"kind": "panel", "id": "p1", "take": "theirs"}]}'
   ```

3. The answer is `200` with the proposal.
   `merged_version` holds the new version, and the `Location` header points at it.

The `take` of a choice is `ours` to keep the dashboard's newer version of the unit, or `theirs` to take the proposal's.
`POST /api/v1/proposals/{id}/discard` closes a proposal without saving it, with an optional body `{"reason": "..."}`.

From the command line, `observe proposals list`, `show`, `merge` and `discard` do the same; see [the CLI](../use/cli.md#proposals).
The MCP tools `list_proposals`, `get_proposal`, `merge_proposal` and `discard_proposal` are the routes above as tools; see [MCP](../use/mcp.md).

## Reference

### Who gets a proposal

| Caller | Save with `propose_if_needed: true` | Save without it |
| --- | --- | --- |
| Editor, connection not in propose mode | A new version. | A new version. |
| Commenter | A proposal, `202`. | `403`. |
| Connection in propose mode | A proposal, `202`, even for an Editor. | `403`. |
| Viewer | `403`. | `403`. |

Details of a connection in propose mode:

- It can never write a version directly, even when its person is an Editor.
- It cannot merge a proposal, and answers `403` with the code `propose_mode`.
- It cannot remove panels or variables.
  Such a save answers `409` with the code `proposal_required` and a `removed` list that names them, and nothing is proposed.
  The agent asks its person to remove them.

### Merge and conflicts

`GET /api/v1/proposals/{id}` returns the proposal with these extra fields.

| Field | Holds |
| --- | --- |
| `spec` | The proposal's whole document. |
| `changes` | What the proposal adds, changes, removes or moves compared with the version it builds on. |
| `diff` | The same comparison field by field, in the shape of `GET /api/v1/dashboards/{id}/diff`, with the old and new value of each field. |
| `conflicts` | The units that a merge cannot combine, listed while the proposal is open and behind. |

Details of a merge:

- The body must name the `revision` the caller read, a positive 32-bit integer.
  `409` with the code `proposal_changed` and the newest `revision` says the author added a newer one.
- `choices` takes at most 400 items, each with a `kind` of `panel`, `variable` or `setting`, the unit's `id` and a `take` of `ours` or `theirs`.
- It runs the same checks as any save, and the new version records the proposal it came from as `merged_proposal_id`.
- When the result equals the latest version, nothing is saved, and the proposal closes as merged with no `merged_version`.

### Discard

- The author and anyone who may merge can discard.
- The `reason` is trimmed and at most 500 characters, and it must not contain the NUL character.
  A longer reason answers `400` with the code `invalid_request`.
- The proposal then shows the reason as `close_reason`.
- The reason is not copied into the activity log, which never carries free text.
  Read it from the proposal.
- A proposal that is already merged or discarded answers `409` with the code `invalid_transition` and the actions that are allowed.

### What a close tells people

- When someone other than the author merges or discards a proposal, the author gets a `proposal_closed` item in the [Inbox](../use/inbox.md), with the reason as its excerpt for a discard.
- Merging or discarding posts a comment in the proposal's discussion, written as the person or key that closed it.
  A merge posts "Merged as version N." or "Merged." when no version was saved.
  A discard posts "Discarded." and adds "Reason: ..." when there is one.
- A failed post never undoes the close.

### States

A proposal is `open`, `merged` or `discarded`.
Its `allowed_actions` list what the caller may do now.
`update` is a new save by the author, which has no route of its own.
[States](../reference/states.md) lists the moves and who may make them.

Each step is an entry in [Activity](../use/activity.md): `proposal.opened`, `proposal.updated`, `proposal.merged` and `proposal.discarded`.
Deleting the dashboard deletes its proposals.

### Routes

| Route | Does |
| --- | --- |
| `GET /api/v1/dashboards/{id}/proposals` | Lists the proposals of a dashboard. |
| `GET /api/v1/proposals/{id}` | Returns one proposal with its document, changes and conflicts. |
| `POST /api/v1/proposals/{id}/merge` | Merges it. |
| `POST /api/v1/proposals/{id}/discard` | Discards it. |

Details of the list:

- It shows the open proposals unless `state` is `merged`, `discarded` or `all`.
  Any other `state` answers `400`.
- `run_id` narrows it to the proposal that one agent request opened, however many the dashboard has.
  A run's status on the dashboard uses it, and shows plain "Proposal" until it knows the state.
- `limit` is 1 to 100 and defaults to 50, and `cursor` continues from `next_cursor`.

### MCP tools

- `update_dashboard` and `update_panels` save a proposal in the cases above and answer `proposed` with the `proposal_id`.
- A save on an older version merges onto the latest one when it changed none of the panels, variables and settings that the newer versions changed.
- `list_proposals`, `get_proposal`, `merge_proposal` and `discard_proposal` are the routes above as tools.
