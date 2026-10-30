---
title: Roles
description: The four instance roles and how a person's role is decided.
---

Every person and every service account has one of four instance roles, from the most to the least permitted: Admin, Editor, Commenter and Viewer.

| Role      | May                                                                                                                                                                      |
| --------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Admin     | Everything an Editor may, in every folder, plus the People and Datasources pages (change roles, sign people out), service accounts and keys, and the outbound allowlist. |
| Editor    | Create, edit, import and restore dashboards, create folders, chat and send requests to the agent, comment, and keep their own saved prompts.                             |
| Commenter | View dashboards and versions, and add comments. Cannot create, import or edit dashboards or send requests to the agent, in any folder.                                   |
| Viewer    | View dashboards and versions only, in any folder.                                                                                                                        |

A request the role does not allow is refused with `403` and the code `forbidden_role`.
A service-account key can never create or revoke keys, whatever its role.

A person's role is decided in this order, and the first rule that applies wins:

1. **`SLASHOBSERVE_ADMIN_EMAILS`.**
   An email address in this list (compared without regard to case) is an Admin.
2. **The first user.**
   When `SLASHOBSERVE_ADMIN_EMAILS` is not set, the earliest person to have signed in is an Admin.
3. **`SLASHOBSERVE_OIDC_GROUP_ROLES`.**
   The highest role mapped to a group the person belongs to.
   When two mapped groups give the same role, the one listed first wins.
4. **A role an Admin chose** on the People page.
5. **Editor**, the default for everyone else.

The first three are set by configuration, so the People page cannot change them.

`SLASHOBSERVE_OIDC_GROUP_ROLES` is a comma-separated list of `group=role` entries, split on the last `=`, where the role is `admin`, `editor`, `commenter` or `viewer`.
A group name is 1 to 256 bytes without control characters and cannot contain a comma.
For example:

```text
SLASHOBSERVE_OIDC_GROUP_ROLES=platform-admins=admin,engineering=editor,support=commenter,contractors=viewer
```

Groups come from the `groups` claim of the ID token, a JSON array of strings.
The server asks for the `groups` scope only when the provider lists it in `scopes_supported` in its discovery document (Dex does; Google rejects unknown scopes), and it reads the claim whenever the token has one.
At most 200 groups are kept, each 1 to 256 bytes without control characters; others are dropped.
Without a `groups` claim a person has no groups.

A role is recomputed every time the person signs in, and for everyone when the server starts.
Changing `SLASHOBSERVE_ADMIN_EMAILS` or `SLASHOBSERVE_OIDC_GROUP_ROLES` therefore applies at the next restart, and a group change reaches a person at their next sign-in.
Existing people become Editors when you upgrade, and the rules above are applied at the first start.
