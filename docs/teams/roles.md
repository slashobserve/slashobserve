---
title: Roles
description: The four instance roles and how a person's role is decided.
---

Every person and every service account has one of four instance roles, from the most to the least permitted: Admin, Editor, Commenter and Viewer.

| Role      | May                                                                                                                                                                                                          |
| --------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Admin     | Everything an Editor may, in every folder except other people's My Dashboards, plus the People and Datasources pages (change roles, sign people out, disconnect their agents, invite, deactivate, reactivate and remove people and manage teams, see [People](people.md)), service accounts and keys, the outbound allowlist, Grafana connections, webhooks, the Audit pages, which hold the audit log and the Audit folder (see [Activity](../use/activity.md#audit-log)), and every chat with Observe Agent, read only. |
| Editor    | Create, edit, import, duplicate, restore and delete dashboards, run canvas actions, create folders, chat and send requests to the agent, comment, keep their own saved prompts, and read the outbound allowlist.                          |
| Commenter | View dashboards and versions, add comments, and send requests to the agent, whose changes arrive as proposals. The role alone creates, imports and edits dashboards, and chats with the agent to build them, only in their own My Dashboards.             |
| Viewer    | View dashboards and versions only, and ask for comment access (see [Folders and access](folders.md#ask-for-comment-access)). The role alone creates, imports and edits dashboards, and chats with the agent to build them, only in their own My Dashboards.                                                              |


## Folders, keys and actions

A restricted folder's list can give a person more than their role on that folder, and never less: Edit lets them change, create and start dashboards there, and Admin lets them manage the folder (see [Folders and access](folders.md)).
Every person also has a private My Dashboards folder where they always have Edit, so a Viewer or Commenter can ask the agent for dashboards there.
Those runs spend model tokens and let the agent query every configured datasource for them.
The exception is a datasource only Admins may read, one built into SlashObserve or one whose audience is set to `admins` in the [datasources file](../reference/datasources-file.md) or the app: every other role, key and public link gets the same answer for it as for a datasource that does not exist, and it is never listed, queried or health-checked for them.
An MCP client a person connects has the same reach: it may do over MCP what the person may do in the web app.

A key is never on a list, so a key has what its role gives.
Creating folders, managing saved prompts and the Admin pages follow the role only.
A request the role does not allow is refused with `403` and the code `forbidden_role`.
On dashboards, folders, runs, pins, annotations and the public links of a dashboard or of an observation's RCA the body says what would let it through: `needed_access` (`view`, `comment`, `edit` or `manage`) with the `folder_title` it applies to, or `needed_role` for a route that needs a role, and the web app shows it as "it needs Edit on Payments".
A refusal anywhere else carries only the code and the message.
Investigating a question needs the Editor role, so a Commenter or Viewer sees Ask disabled with "needs the Editor role" instead of a refusal after the fact.
A service-account key can never create or revoke keys, whatever its role.
A canvas action sends a request to another site on a person's behalf (see [Actions](../operate/outbound-access.md#actions)), so it needs Edit on the dashboard and a person: a service-account key cannot run one, whatever its role.

## How a role is decided

A person's role is decided in this order, and the first rule that applies wins:

1. **`SLASHOBSERVE_ADMIN_EMAILS`.**
   An email address in this list (compared without regard to case) is an Admin, when the person signed in with an address the provider reports as verified.
   An entry you add takes effect at that person's next verified sign-in, not at the next restart; the restart only keeps Admin for people who already got it from this list.
   A provider that never sends `email_verified`, such as Microsoft Entra ID, makes no one an Admin through this list; the server logs `admin email signed in unverified` with the person's id at each such sign-in, so map an admin group with `SLASHOBSERVE_OIDC_GROUP_ROLES` instead.
   Releases before the verified check granted Admin from this list to an unverified address too, and the restart keeps that Admin, so after upgrading check the People page for Admins whose role comes from the list and remove any address you do not trust from it.
2. **The first user.**
   When `SLASHOBSERVE_ADMIN_EMAILS` is not set, the earliest person to have signed in is an Admin.
   This stays with that person: if they are removed, nobody else becomes the first user.
3. **`SLASHOBSERVE_OIDC_GROUP_ROLES`.**
   The highest role mapped to a group the person belongs to.
   When two mapped groups give the same role, the one listed first wins.
4. **A role an Admin chose** on the People page.
5. **Editor**, the default for everyone else.

The first three are set by configuration, so the People page cannot change them.

## Mapping OIDC groups to roles

`SLASHOBSERVE_OIDC_GROUP_ROLES` is a comma-separated list of up to 100 `group=role` entries, split on the last `=`, where the role is `admin`, `editor`, `commenter` or `viewer`.
A group name is 1 to 256 bytes without control characters and cannot contain a comma.
For example:

```text
SLASHOBSERVE_OIDC_GROUP_ROLES=platform-admins=admin,engineering=editor,support=commenter,contractors=viewer
```

Groups come from the `groups` claim of the ID token, a JSON array of strings.
The server asks for the `groups` scope only when the provider lists it in `scopes_supported` in its discovery document (Dex does; Google rejects unknown scopes), and it reads the claim whenever the token has one.
At most 200 groups are kept, each 1 to 256 bytes without control characters; others are dropped.
Without a `groups` claim a person has no groups.

## When a role changes

A role is recomputed every time the person signs in, and for everyone when the server starts.
Changing `SLASHOBSERVE_OIDC_GROUP_ROLES` therefore applies at the next restart, and a group change reaches a person at their next sign-in.
Removing an entry from `SLASHOBSERVE_ADMIN_EMAILS` applies at the next restart; adding one applies at that person's next verified sign-in.
Existing people become Editors when you upgrade, and the rules above are applied at the first start.

A change an Admin makes on the People page applies to the person's next request, not at their next sign-in.
An open web app learns of it when its tab becomes visible again, or when an action it tried is refused with `forbidden_role`, and then reloads what the person may see and do.
A client that uses a key sees it on the next call, and `GET /api/v1/auth/session` returns the current role.
