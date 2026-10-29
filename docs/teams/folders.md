---
title: Folders and access
description: How folders hold dashboards and who can see or change them.
---

Every dashboard lives in one folder.
A new install has one folder, General, and every dashboard that existed before folders is in it.
General is open to everyone by role, and it cannot be renamed, deleted or restricted.

Editors and Admins create folders and move dashboards between them on the Dashboards page.
A move needs Edit on both folders and saves no version.
A folder can be deleted only when it is empty.
A title is 1 to 100 characters and unique without regard to case.

A folder is open or restricted, and the folder's settings page (**Folder settings** on the Dashboards page) switches it:

- An open folder has no list.
  Everyone sees it and has the access their role gives: Admins manage it, Editors edit, Commenters comment and Viewers view.
- A restricted folder has a list of people and groups, at most 50.
  Only the people on the list, the person who created the folder and Admins see it.
  Everyone else gets no trace of its dashboards: they are missing from lists, search, tags and datasource counts, their panel data, events and comments are refused, and the agent cannot reach them.
  Removing the last entry opens the folder to everyone again.

Each entry gives a level, and a person gets the higher of the level their role gives and the levels of the entries that match them, so an entry never lowers access:

| Level | May                                                                              |
| ----- | -------------------------------------------------------------------------------- |
| View  | See the folder and its dashboards.                                               |
| Edit  | Also change dashboards, comment, send requests to the agent and move dashboards. |
| Admin | Also rename the folder, delete it and change its list.                           |

Some rules that follow from the roles:

- A Commenter or Viewer can be given up to Edit, never Admin.
  They then edit and comment in that folder but cannot manage it.
- Creating a dashboard, in the web, through the API or by importing, needs the Editor role as well as Edit on the folder.
- The person who created a folder is its Admin and keeps seeing it when it is restricted, even if the list does not name them.
- Admins have the Admin level on every folder.

An entry names a person by email address or a group by its name.
A person has to have signed in once before an entry can name them, and an unknown address is refused with `422 unknown_user`.
Groups come from the `groups` claim of the ID token, as in [Roles](roles.md), and a person's groups are read at each sign-in, so a change to a group reaches folder access at the person's next sign-in.

A service-account key matches no entry, because entries name people and groups.
A key therefore sees a restricted folder only when its service account is an Admin or created the folder.
To let automation work in a restricted folder, use a key of an Admin service account.

The agent works as the person or key that asked.
The request needs Edit on the dashboard, and the server checks it when the request is queued and again when a worker starts it.
A request whose person lost access or was deleted in between fails with a message saying so.

Over the API, a dashboard in a folder the caller cannot see answers `403 forbidden_role` and a folder answers `404 not_found`; MCP tools answer `not_found` for both.
