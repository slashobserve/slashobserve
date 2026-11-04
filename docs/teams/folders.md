---
title: Folders and access
description: How folders hold dashboards and who can see or change them.
---

Every dashboard lives in one folder.
A new install has one folder, General, and every dashboard that existed before folders is in it.
General is open to everyone by role, and it cannot be renamed, deleted or restricted.

Editors and Admins create folders.
Anyone with Edit on a folder moves its dashboards into another folder where they have Edit, on the Dashboards page, and deletes a dashboard from its row menu there or from the dashboard's **More** menu, after a confirmation.
A move needs Edit on both folders and saves no version.
A folder can be deleted only when it is empty.
A title is 1 to 100 characters and unique without regard to case, and no shared folder can be called My Dashboards.

A folder is open or restricted, and the folder's settings page (**Folder settings** on the Dashboards page) switches it:

- An open folder has no list.
  Everyone sees it and has the access their role gives: Admins manage it, Editors edit, Commenters comment and Viewers view.
- A restricted folder has a list of people and groups, at most 50.
  Only the people on the list, the person who created the folder and Admins see it.
  Everyone else gets no trace of its dashboards: they are missing from lists, search, tags and datasource counts, their panel data, events and comments are refused, and the agent cannot reach them.
  Removing the last entry opens the folder to everyone again.

## My Dashboards

Every person has one personal folder, My Dashboards, that only they see.
Admins cannot see it either.
The person has Edit in it whatever their role: they create, edit, import and duplicate dashboards into it, ask the agent for dashboards in it, move dashboards out of it into a folder where they have Edit, and delete its dashboards.
A Viewer or Commenter can do all of this there, and nowhere else without a grant.
An agent run in My Dashboards spends model tokens and queries every configured datasource for that person.

My Dashboards has no list and cannot be renamed, shared or deleted, and it lists first on the Dashboards page.
In the sidebar, **My Dashboards** opens the Dashboards page filtered to it, and its row there reads "Only you".
Its settings page cannot rename, share or delete it, and **Who has access** shows only you.
A service-account key has no My Dashboards folder.
When a person's account is deleted, their My Dashboards becomes visible to Admins only, who can move its dashboards out and delete the folder once it is empty.

### Moving dashboards in and out

You can move a dashboard into your My Dashboards only if you created it and you have Edit on the folder it is in.
Dashboards the agent made for you count as yours.
To keep someone else's dashboard, duplicate it into My Dashboards instead, because the copy is yours.
In a dashboard's **Move to folder** menu, My Dashboards is greyed out with "Only dashboards you created" for a dashboard someone else created.
Moving a dashboard out needs Edit on the target folder, like any move.
Moving your dashboard in hides it from everyone else, and their queued agent requests on it then fail with the "no longer has access" message.
A move the rule refuses answers `403 not_dashboard_creator`.

Where a new dashboard goes when no folder is named: the folder you last created a dashboard in, else General, else your My Dashboards, else the first folder where you have Edit.

## Levels

Each entry gives a level.
A person gets the higher of the level their role gives and the highest entry that matches them, on the folder and on every dashboard in it.
An entry can give a Viewer or Commenter more and never gives anyone less:

| Level   | May                                                                                                                                                                                                |
| ------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| View    | See the folder and its dashboards.                                                                                                                                                                 |
| Comment | Also comment and reply.                                                                                                                                                                            |
| Edit    | Also change and restore dashboards, edit their details, run canvas actions, send requests to the agent, create, duplicate or import dashboards into the folder, and move or delete its dashboards. |
| Admin   | Also rename the folder, delete it and change its list.                                                                                                                                             |

Restricted folders on the Dashboards page show `Your access: <level>` in their header.
The Dashboards page shows 50 dashboards per page; a folder that runs onto the next page repeats its header there, marked "Continued from the previous page".
Folder settings tells a person who cannot manage the folder `Your access to this folder is <level>. Only folder Admins can change it.`

Some rules that follow:

- A Viewer or Commenter given Edit edits in that folder, including creating dashboards and starting them with the agent, and given Admin manages it.
- An Editor given View or Comment still edits, and an Admin entry makes them the folder's Admin.
- Admins have the Admin level on every folder except other people's My Dashboards.
- The person who created a folder manages it while they are an Editor, and keeps seeing it when it is restricted, even if the list does not name them.
- Creating, duplicating and importing a dashboard need Edit on the target folder.
  Starting a dashboard with the agent needs Edit on some folder, which every person has in My Dashboards.
- Creating a folder needs the Editor role.

An entry names a person by email address or a group by its name.
A person has to have signed in once before an entry can name them, and an unknown address is refused with `422 unknown_user`.
Groups come from the `groups` claim of the ID token, as in [Roles](roles.md), and a person's groups are read at each sign-in, so a change to a group reaches folder access at the person's next sign-in.

A service-account key matches no entry, because entries name people and groups.
A key therefore sees a restricted folder only when its service account is an Admin or created the folder.
To let automation work in a restricted folder, use a key of an Admin service account.

The agent works as the person or key that asked.
The request needs Edit on the dashboard, and the server checks it when the request is queued and again when a worker starts it.
A new dashboard from the chat needs Edit on some folder and goes to a folder where the requester has Edit, by the default order under My Dashboards above.
A request whose person lost access or was deleted in between fails with a message saying so.

Over the API, a dashboard in a folder the caller cannot see answers `403 forbidden_role` and a folder answers `404 not_found`; MCP tools answer `not_found` for both.

An entry used to give a Viewer or Commenter nothing beyond their role.
Now an existing entry at Edit or Admin lifts them, so review the lists of restricted folders after you upgrade.

A shared folder already titled My Dashboards keeps working but should be renamed, because an import that names My Dashboards goes to the importer's own folder.
