---
title: Folders and access
description: How folders hold dashboards, observations and saved searches, and who can see or change them.
---

Every dashboard lives in one folder.
A folder also holds the [observations](../use/observations.md) in it, the answers of investigations included, and the logs, trace and metrics searches saved in it, and the folder's access rule covers all three: whoever can view the folder can read its dashboards, observations and saved searches, and the levels below decide who may change them.
A finding is an answer written into an observation, so it follows the folder its observation is in.
A folder's page lists its dashboards, and below them its **Observations**, with their state and when they last changed, and its **Saved searches**, which appears only when the folder has some.
Anyone with Edit on a saved search renames, moves or deletes it from its row menu there, and moves an observation, with the finding in it, from the observation's **More actions** menu.
An empty folder you cannot edit says "Your access" with your level and who to ask, and offers no chat or import button, because those start dashboards in your My Dashboards, not in the folder.
A new install has one folder, General, and every dashboard that existed before folders is in it.
General is open to everyone by role, and it cannot be renamed, deleted or restricted.

Editors and Admins create folders.
Anyone with Edit on a folder moves its dashboards into another folder where they have Edit, on the Dashboards page, and deletes a dashboard from its row menu there or from the dashboard's **More** menu, after a confirmation.
A move needs Edit on both folders and saves no version.
A folder can be deleted only when it holds no dashboards, observations or saved searches.
A title is 1 to 100 characters and unique without regard to case, and no shared folder can be called My Dashboards.

A folder is open, restricted or Admins only, and the folder's settings page (**Folder settings** on the Dashboards page) switches it:

- An open folder has no list.
  Everyone sees it and has the access their role gives: Admins manage it, Editors edit, Commenters comment and Viewers view.
- A restricted folder has a list of people and groups, at most 50.
  Only the people on the list, the person who created the folder and Admins see it.
  Everyone else gets no trace of its dashboards: they are missing from lists, search, tags and datasource counts, their panel data, events and comments are refused, and the agent cannot reach them.
  Removing the last entry opens the folder to everyone again.
  Choosing **Only the people and groups below** restricts nothing until someone is listed, and the page says so.
  An instance Admin gets suggestions from the people who have signed in as they type an address; everyone else types the address, and a group is always typed as its name.
- An Admins-only folder has no list, and only Admins see it, the person who created it included.
  Only an Admin chooses **Only Admins**, and General and My Dashboards cannot be made Admins only.
  Its row on the Dashboards page reads "Only Admins".
  Listing someone or opening the folder ends it.
  It is the only kind of folder that can hold a dashboard, observation or saved search that reads an [Admins-only datasource](../connect/datasources-in-the-app.md#who-can-query-it); while it holds one, listing someone or opening it is refused with `409 restricted_content`, which names the datasource and the content to move first.
  It is the only place an answer that read Admin-only data can go: an Admin's run in it may read the [audit trail](../reference/audit-queries.md), and a `restricted` answer from a private chat is shared, saved, pinned or applied only into it (see [Answers that read Admin-only data](../use/chat.md#answers-that-read-admin-only-data)); elsewhere that is refused with `409 restricted_answer`.
  A dashboard or observation that holds such an answer, or an answer about another dashboard or observation that is in an Admins-only folder now, as a version a run saved, the answer a dashboard was made from, a proposal a run opened, a pinned finding, a block or a thread reply, or that a `restricted` run was about, moves only to another Admins-only folder, and the folder that holds it is not listed or opened while it does; both are refused with `409 restricted_answer`, which names the content to move first.
  A copy of a dashboard or observation that holds such an answer, made by duplicating the dashboard, making a dashboard from the observation or pinning the observation onto a dashboard, goes only into an Admins-only folder and is refused elsewhere with `409 restricted_answer`; the copy then holds the answer too, so it stays in Admins-only folders as well.
  While a run about a dashboard or observation in an Admins-only folder has not ended, the same two are refused with `409 restricted_answer` and a message to wait for the run to end, because what the run writes may still read Admin-only data.
  It has no [public links](../use/public-links.md): choosing **Only Admins** for a folder that holds a dashboard or an RCA with an unexpired public link, or moving such a dashboard or observation into it, is refused with `409 admins_only_folder` until the links are revoked.

## My Dashboards

Every person has one personal folder, My Dashboards, that only they see; Admins cannot see it either.
A dashboard there is private: nobody else can open it, find it in a list or search, or reach it through the agent.
Every new dashboard you make starts there unless you choose a folder: dashboards from the chat, including a stopped build kept with **Save what was built**, and imports whose export names no folder.
To share one, publish it: open its **Share** pane, choose a folder where you have Edit and choose **Publish**, which names who will be able to open the dashboard and asks you to confirm, or use **Publish to folder** in its row menu on the Dashboards page.
Publishing moves the dashboard with its versions, comments and link, and saves no version.
Nothing moves into My Dashboards; to keep a private copy of a dashboard, duplicate it into My Dashboards.
A move into My Dashboards answers `409 personal_folder`.

You have Edit in My Dashboards whatever your role: a Viewer or Commenter can ask the agent for dashboards there, keep what a stopped build made, edit and delete them, and publishes only to a folder where an entry gives them Edit.
An agent run in My Dashboards spends model tokens and queries every configured datasource for that person.

My Dashboards has no list and cannot be renamed, shared or deleted.
It is the first folder on the Dashboards page, and its row there reads "Only you".
A service-account key has no My Dashboards, so its new dashboards go to General unless it names a folder.
When a person's account is deleted, their My Dashboards and every dashboard, observation and saved search in it are deleted with it, like their saved prompts.

Where a new dashboard goes when no folder is named: your My Dashboards; for a key, General.

## Levels

Each entry gives a level.
A person gets the higher of the level their role gives and the highest entry that matches them, on the folder and on every dashboard in it.
An entry can give a Viewer or Commenter more and never gives anyone less:

| Level   | May                                                                                                                                                                                                |
| ------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| View    | See the folder and its dashboards.                                                                                                                                                                 |
| Comment | Also comment and reply, and send a request to the agent, whose change reaches the dashboard as a proposal.                                                                                         |
| Edit    | Also change and restore dashboards, edit their details, run canvas actions, ask the agent to suggest a panel change, create, duplicate or import dashboards into the folder, and move or delete its dashboards. |
| Admin   | Also rename the folder, delete it and change its list.                                                                                                                                             |

Restricted folders on the Dashboards page show `Your access: <level>` in their header.
The Dashboards page shows 50 dashboards per page; a folder that runs onto the next page repeats its header there, marked "Continued from the previous page".
A folder header counts the folder's dashboards while no search or filter is set.
With a search or filter, it counts the folder's matching dashboards only when all matches fit on the one page, and shows no count otherwise.
Folder settings tells a person who cannot manage the folder `Your access to this folder is <level>. Only folder Admins can change it.`

Some rules that follow:

- A Viewer or Commenter given Edit edits in that folder, including creating dashboards and starting them with the agent, and given Admin manages it.
- An Editor given View or Comment still edits, and an Admin entry makes them the folder's Admin.
- Admins have the Admin level on every folder except other people's My Dashboards.
- The person who created a folder manages it while they are an Editor, and keeps seeing it when it is restricted, even if the list does not name them.
- Creating, duplicating and importing a dashboard need Edit on the target folder.
  A new dashboard from the agent goes to your My Dashboards, where you always have Edit.
- Creating a folder needs the Editor role.

An entry names a person by email address or a group by its name.
A person has to have signed in once before an entry can name them, and an unknown address is refused with `422 unknown_user`.
Groups come from the `groups` claim of the ID token, as in [Roles](roles.md), and a person's groups are read at each sign-in, so a change to a group reaches folder access at the person's next sign-in.

A service-account key matches no entry, because entries name people and groups.
A key therefore sees a restricted folder only when its service account is an Admin or created the folder.
To let automation work in a restricted folder, use a key of an Admin service account.
A key that manages the folder, such as one of an Admin service account, can change the list with `observe folders permissions show`, `grant`, `revoke` and `open`, and an Admin key makes a folder Admins only with `admins-only`; see [the command line](../use/cli.md#folders).
The [Audit folder](../use/activity.md#audit-folder) is an ordinary Admins-only folder that the server made, and each folder in `GET /api/v1/folders` carries a read-only `audit`, true for that one whatever it is now called and false for every other folder.
The same flag is `audit` in the MCP `list_folders` result, and `observe folders list` marks that folder `(Audit folder)`.
The Dashboards page leaves that folder out, and so can any other client: `GET /api/v1/dashboards?exclude_audit=true`, MCP `list_dashboards` with `exclude_audit: true` and `observe dashboards list --exclude-audit` leave the Audit folder's dashboards out of the rows, the total and the paging alike, and change nothing else, and `GET /api/v1/dashboards/tags?exclude_audit=true` and `observe dashboards tags --exclude-audit` do the same for the tag list and its counts; without it every dashboard the caller can see is listed, as before.
Over REST, `GET /api/v1/folders/{id}/permissions` reads the list and `PUT` replaces all of it in one call; `{"entries": [], "admins_only": true}` makes the folder Admins only.

The agent works as the person or key that asked.
The request needs Comment on the dashboard's folder, and the server checks it when the request is queued and again when a worker starts it.
A new dashboard from the chat goes to the requester's My Dashboards, or to General for a key.
A request whose person lost access or was deleted in between fails with a message saying so.

Over the API and MCP, a dashboard or a folder the caller cannot see answers `404 not_found`, the same answer as an id that does not exist, so a guessed id never confirms that a hidden dashboard exists.
A dashboard the caller can see but not change answers `403 forbidden_role`.

An entry used to give a Viewer or Commenter nothing beyond their role.
Now an existing entry at Edit or Admin lifts them, so review the lists of restricted folders after you upgrade.

A shared folder already titled My Dashboards keeps working but should be renamed, because an import that names My Dashboards goes to the importer's default folder.

## Ask for comment access

A person who can view a dashboard or an observation but not comment on it sees **Request comment access** under the note that says so, in the dashboard's activity pane or the observation's discussion.
It opens a box for an optional note of one line, at most 200 characters, and **Send request** sends it.
Sending it changes nobody's access.

The request goes to the people who manage the object's folder: the person who created the folder while they are an Editor or Admin, and the people an entry gives Admin.
When nobody manages the folder that way, as for General, it goes to the instance Admins.
The person who asked is never told who received it, beyond what the Share pane already shows them.
Each of them gets an `access_request` item in their [inbox](../use/inbox.md) that names who asked, the dashboard or observation and the note.
Opening it goes to the dashboard's Share pane, or for an observation to its folder's settings, where an entry with Comment or higher gives the access.
General takes no entry, because it cannot be restricted, so a request for something in General says so in the inbox item and opens People, where the Admin raises the person's role, or API keys for a service account; the other way is to move the item to a folder and grant it there.

A person has at most one open request on each dashboard or observation.
Asking again while it is open answers `409 request_open` with `requested_at`, the time of the open request, and a message that names it; the web app says how long ago it was made, and the CLI and MCP print the time.
A person who can already comment gets `409 already_can_comment`.
A dashboard or observation the caller cannot see answers `404 not_found`, the same answer as an id that does not exist.

A request closes in one of these ways:

- The person who asked can now comment there, because a folder entry gives them Comment or higher, or because their role, or a service account's role, was raised to Commenter or higher.
  The activity log records `access_request.granted`, with who changed the access and who had asked, beside the `access_request.opened` event of the request.
- Someone who received it marks the inbox item done, which is how a request is declined; it then leaves every recipient's open inbox.
- It is 30 days old.

A change to a person's groups reaches folder access only at their next sign-in, so it does not close a request; mark the item done once they can comment.
After a request closes, the person can ask again.

A service-account key may ask too.
Because keys match no entry, its request is granted by raising the service account's role.

`POST /api/v1/access-requests` takes `{"object": {"kind": "dashboard", "id": "<id>"}, "note": "<note>"}`, with `observation` as the other kind, and answers the request.
`GET /api/v1/access-requests` lists the open requests on the folders you manage.
The `observe` command line has `observe access request dashboard:<id> --note "<note>"` and `observe access requests`, and MCP clients have `request_comment_access` and `list_access_requests`; see [the command line](../use/cli.md) and [MCP](../use/mcp.md).
Both need the Viewer role or higher.
