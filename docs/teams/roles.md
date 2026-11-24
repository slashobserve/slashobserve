---
title: Roles
description: The four instance roles and how a person's role is decided.
---

Every person and every service account has one of four instance roles, from the most to the least permitted: Admin, Editor, Commenter and Viewer.
The role says what someone may do across the whole server.
A folder can give a person more on that folder; see [Folders and access](folders.md).

## Find out what a role may do

| Role | May |
| --- | --- |
| Admin | Everything an Editor may, plus the Admin pages and settings. |
| Editor | Build and change dashboards, create folders, comment and ask the agent. |
| Commenter | View dashboards, comment, and send requests to the agent. |
| Viewer | View dashboards and versions, and ask for comment access. |

Details of each role follow.
A Commenter or Viewer also has a private My Dashboards folder, where they can build; see [Folders, keys and actions](#folders-keys-and-actions).

### Admin

An Admin may do everything an Editor may, in every folder except other people's My Dashboards.
An Admin also has these:

- The People and Datasources pages: change roles, sign people out, disconnect their agents, invite, deactivate, reactivate and remove people, and manage teams (see [People](people.md)).
- Service accounts and keys, the outbound allowlist, Grafana connections and webhooks.
- The Audit pages, which hold the audit log and the Audit folder (see [Activity](../use/activity.md#audit-log)).
- Read-only access to every chat with Observe Agent.

### Editor

An Editor may do these:

- Create, edit, import, duplicate, restore and delete dashboards.
- Run canvas actions.
- Create folders.
- Chat with the agent and send it requests.
- Comment.
- Keep their own saved prompts.
- Read the outbound allowlist.

### Commenter

A Commenter views dashboards and versions, adds comments, and sends requests to the agent.
The agent's changes arrive as proposals.
In their own My Dashboards only, a Commenter also creates, imports and edits dashboards and chats with the agent to build them.

### Viewer

A Viewer views dashboards and versions only, and can ask for comment access (see [Ask for comment access](folders.md#ask-for-comment-access)).
In their own My Dashboards only, a Viewer also creates, imports and edits dashboards and chats with the agent to build them.

## Folders, keys and actions

A restricted folder's list can give a person more than their role on that folder, and never less.
Edit lets them change, create and start dashboards there, and Admin lets them manage the folder.
Every person also has a private My Dashboards folder where they always have Edit, so a Viewer or Commenter can ask the agent for dashboards there.
Those runs spend model tokens and let the agent query every configured datasource for them.

The exception is a datasource only Admins may read.
That is a datasource built into SlashObserve, or one whose audience is set to `admins` in the [datasources file](../reference/datasources-file.md) or the app.
Every other role, key and public link gets the same answer for it as for a datasource that does not exist, and it is never listed, queried or health-checked for them.
An MCP client a person connects has the same reach: it may do over MCP what the person may do in the web app.

A key is never on a folder list, so a key has what its role gives.
Creating folders, managing saved prompts and the Admin pages follow the role only.
A service-account key can never create or revoke keys, whatever its role.
A canvas action sends a request to another site on a person's behalf (see [Actions](../operate/outbound-access.md#actions)).
It needs Edit on the dashboard and a person, so a service-account key cannot run one, whatever its role.
Starting an investigation needs the Editor role, so `/investigate` is offered only to Editors and Admins.

## How a role is decided

SlashObserve decides a person's role in this order, and the first rule that applies wins:

1. **`SLASHOBSERVE_ADMIN_EMAILS`.**
   An email address in this list (compared without regard to case) is an Admin, when the person signed in with an address the provider reports as verified.
2. **The first user.**
   When `SLASHOBSERVE_ADMIN_EMAILS` is not set, the earliest person to have signed in is an Admin.
   This stays with that person: if they are removed, nobody else becomes the first user.
3. **`SLASHOBSERVE_OIDC_GROUP_ROLES`.**
   The highest role mapped to a group the person belongs to.
   When two mapped groups give the same role, the one listed first wins.
4. **A role an Admin chose** on the People page.
5. **Editor**, the default for everyone else.

The first three are fixed by the server, so the People page cannot change them.

### Admin emails and verification

An entry you add to `SLASHOBSERVE_ADMIN_EMAILS` takes effect at that person's next verified sign-in, not at the next restart.
The restart only keeps Admin for people who already got it from this list.

A provider that never sends `email_verified`, such as Microsoft Entra ID, makes no one an Admin through this list.
The server logs `admin email signed in unverified` with the person's id at each such sign-in.
Map an admin group with `SLASHOBSERVE_OIDC_GROUP_ROLES` instead.

Releases before the verified check granted Admin from this list to an unverified address too, and the restart keeps that Admin.
After upgrading, check the People page for Admins whose role comes from the list, and remove any address you do not trust from it.

## Map OIDC groups to roles

1. Choose a group for each role you want to set, as your identity provider names it.
2. Set `SLASHOBSERVE_OIDC_GROUP_ROLES` to a comma-separated list of `group=role` entries:

   ```text
   SLASHOBSERVE_OIDC_GROUP_ROLES=platform-admins=admin,engineering=editor,support=commenter,contractors=viewer
   ```

3. Restart the server.
   A person gets the mapped role at their next sign-in.

The list holds up to 100 entries, and each splits on its last `=`.
The role is `admin`, `editor`, `commenter` or `viewer`.
A group name is 1 to 256 bytes without control characters, and cannot contain a comma.

Groups come from the `groups` claim of the ID token, a JSON array of strings.
The server asks for the `groups` scope only when the provider lists it in `scopes_supported` in its discovery document (Dex does; Google rejects unknown scopes), and it reads the claim whenever the token has one.
At most 200 groups are kept, each 1 to 256 bytes without control characters; others are dropped.
Without a `groups` claim a person has no groups.

## Change someone's role

An Admin chooses a role for the person on the People page; see [People](people.md).
That applies only to someone whose role is not set by `SLASHOBSERVE_ADMIN_EMAILS`, the first-user rule or a group.

A change applies to the person's next request, not at their next sign-in.
An open web app learns of it when its tab becomes visible again, or when an action it tried is refused with `forbidden_role`, and then reloads what the person may see and do.
A client that uses a key sees it on the next call, and `GET /api/v1/auth/session` returns the current role.

## Know when a role changes

SlashObserve recomputes a role every time the person signs in, and for everyone when the server starts.

- Changing `SLASHOBSERVE_OIDC_GROUP_ROLES` applies at the next restart.
  A group change reaches a person at their next sign-in.
- Removing an entry from `SLASHOBSERVE_ADMIN_EMAILS` applies at the next restart.
  Adding one applies at that person's next verified sign-in.
- Existing people become Editors when you upgrade, and the rules above are applied at the first start.

## Reference

### Refusals

A request the role does not allow is refused with `403` and the code `forbidden_role`.
On dashboards, folders, runs, pins, annotations and the public links of a dashboard or of an observation's RCA, the body says what would let it through:

- `needed_access` is `view`, `comment`, `edit` or `manage`, with the `folder_title` it applies to.
- `needed_role` names the role, for a route that needs a role.

The web app shows it as "it needs Edit on Payments".
A refusal anywhere else carries only the code and the message.

### Role order

The roles rank Viewer, Commenter, Editor, Admin, from lowest to highest.
A route that needs a role accepts that role and every higher one.
