---
title: Folders and access
description: How folders hold dashboards, observations and saved searches, and who can see or change them.
---

A folder groups dashboards and decides who can see or change them.
Use folders to keep a team's work together, and restrict a folder when only some people should open it.

Every dashboard lives in one folder.
A folder also holds the [observations](../use/observations.md) in it, the answers of investigations included, and the logs, trace and metrics searches saved in it.
The folder's access rule covers all three: whoever can view the folder can read its dashboards, observations and saved searches, and the levels below decide who may change them.
A finding is an answer written into an observation, so it follows the folder its observation is in.

A new install has one folder, General, and every dashboard that existed before folders is in it.
General is open to everyone by role, and it cannot be renamed, deleted or restricted.

## Restrict a folder to some people

A folder is open until you list someone.
Once it has a list, only the people on it, the person who created the folder and Admins can see it.

1. Open the **Dashboards** page and choose **Folder settings** on the folder.
   You need the Admin level on the folder; see [Levels](#levels).
2. Choose **Only the people and groups below**.
   The page says it restricts nothing until someone is listed.
3. Type an email address or a group name, choose a level (**View**, **Comment**, **Edit** or **Admin**) and choose **Add**.
   An instance Admin gets suggestions from the people who have signed in as they type an address.
   Everyone else types the address, and a group is always typed as its name.
4. Repeat for each person or group.
   The folder is restricted from the first entry.

Everyone else now gets no trace of the folder's dashboards: they are missing from lists, search, tags and datasource counts, their panel data, events and comments are refused, and the agent cannot reach them.
Removing the last entry opens the folder to everyone again.

From the CLI, `observe folders permissions grant <folder-id> --email <address> --level edit` adds an entry, and `observe folders permissions show <folder-id>` lists them.
See [the command line](../use/cli.md#folders).
An entry can also name a [team](people.md#teams) as a group.

## Create, move and delete things

Editors and Admins create folders: on the Dashboards page, choose **New folder**.
A title is 1 to 100 characters and unique without regard to case.
No shared folder can be called My Dashboards or General.

- Anyone with Edit on a folder moves its dashboards into another folder where they have Edit, on the Dashboards page.
  A move needs Edit on both folders and saves no version.
- Anyone with Edit deletes a dashboard from its row menu on the Dashboards page or from the dashboard's **More** menu, after a confirmation.
- Anyone with Edit on a saved search renames, moves or deletes it from its row menu on the folder's page.
- Anyone with Edit moves an observation, with the finding in it, from the observation's **More actions** menu.
- A folder can be deleted only when it holds no dashboards, observations or saved searches.

A folder's page lists its dashboards, and below them its **Observations**, with their state and when they last changed, and its **Saved searches**, which appears only when the folder has some.
An empty folder you cannot edit says "Your access" with your level and who to ask.
It offers no chat or import button, because those start dashboards in your My Dashboards, not in the folder.

## Make a folder Admins only

An Admins-only folder has no list, and only Admins see it, the person who created it included.
Only an Admin chooses **Admins only** in the folder settings.
General and My Dashboards cannot be made Admins only.
Its row on the Dashboards page reads "Admins only".
Listing someone or opening the folder ends it.

From the CLI, an Admin key runs `observe folders admins-only <folder-id> --yes`.
Over REST, `PUT /api/v1/folders/{id}/permissions` with `{"entries": [], "admins_only": true}` does the same.
The folder is the only place for content that reads Admin-only data; see [Admins-only folders in detail](#admins-only-folders-in-detail).

## My Dashboards

Every person has one personal folder, My Dashboards, that only they see.
Admins cannot see it either.
A dashboard there is private: nobody else can open it, find it in a list or search, or reach it through the agent.

Every new dashboard you make starts there unless you choose a folder.
That covers dashboards from the chat, including a stopped build kept with **Save what was built**, and imports whose export names no folder.
A service-account key has no My Dashboards, so its new dashboards go to General unless it names a folder.

### Publish a dashboard from My Dashboards

1. Open the dashboard's **Share** pane.
2. Choose a folder where you have Edit.
3. Choose **Publish**, read who will be able to open the dashboard, and confirm.

Or choose **Publish to folder** in the dashboard's row menu on the Dashboards page.
Publishing moves the dashboard with its versions, comments and link, and saves no version.

Nothing moves into My Dashboards; to keep a private copy of a dashboard, duplicate it into My Dashboards.
A move into My Dashboards answers `409 personal_folder`.

### What you can do in My Dashboards

You have Edit in My Dashboards whatever your role.
A Viewer or Commenter can ask the agent for dashboards there, keep what a stopped build made, and edit and delete them.
They publish only to a folder where an entry gives them Edit.
An agent run in My Dashboards spends model tokens and queries every configured datasource for that person.

My Dashboards has no list and cannot be renamed, shared or deleted.
It is the first folder on the Dashboards page, and its row there reads "Only you".
When a person's account is deleted, their My Dashboards and every dashboard, observation and saved search in it are deleted with it, like their saved prompts.

## Ask for comment access

A person who can view a dashboard or an observation but not comment on it can ask the people who manage its folder for Comment access.
Asking changes nobody's access.

1. Open the dashboard's activity pane or the observation's discussion.
   Under the note that says you cannot comment, choose **Request comment access**.
2. Write an optional note of one line, at most 200 characters.
3. Choose **Send request**.

The request goes to the people who manage the object's folder: the person who created the folder while they are an Editor or Admin, and the people an entry gives Admin.
When nobody manages the folder that way, as for General, it goes to the instance Admins.
The person who asked is never told who received it, beyond what the Share pane already shows them.

Each of them gets an `access_request` item in their [inbox](../use/inbox.md) that names who asked, the dashboard or observation and the note.
Opening it goes to the dashboard's Share pane, or for an observation to its folder's settings, where an entry with Comment or higher gives the access.
General takes no entry, because it cannot be restricted.
A request for something in General says so in the inbox item and opens People, where the Admin raises the person's role, or API keys for a service account.
The other way is to move the item to a folder and grant it there.

From the CLI, `observe access request dashboard:<id> --note "<note>"` asks and `observe access requests` lists what waits for you.
MCP clients have `request_comment_access` and `list_access_requests`; see [the command line](../use/cli.md) and [MCP](../use/mcp.md).

### How a request ends

A person has at most one open request on each dashboard or observation.
A request closes in one of these ways:

- The person who asked can now comment there.
  A folder entry gives them Comment or higher, or their role, or a service account's role, was raised to Commenter or higher.
  The activity log records `access_request.granted`, with who changed the access and who had asked, beside the `access_request.opened` event of the request.
- Someone who received it marks the inbox item done, which is how a request is declined.
  It then leaves every recipient's open inbox.
- It is 30 days old.

A change to a person's groups reaches folder access only at their next sign-in, so it does not close a request.
Mark the item done once they can comment.
After a request closes, the person can ask again.

A service-account key may ask too.
Because keys match no entry, its request is granted by raising the service account's role.

## Reference

### Open, restricted and Admins-only folders

The folder's settings page switches a folder between three kinds:

| Kind | Who sees it |
| --- | --- |
| Open (**Everyone, by role**) | Everyone, with the access their role gives. It has no list. |
| Restricted (**Only the people and groups below**) | The people and groups on its list, the person who created it and Admins. The list holds at most 50 entries. |
| **Admins only** | Admins only, the person who created it included. It has no list. |

In an open folder, Admins manage, Editors edit, Commenters comment and Viewers view.

### Levels

Each entry gives a level.
A person gets the higher of the level their role gives and the highest entry that matches them, on the folder and on every dashboard in it.
An entry can give a Viewer or Commenter more and never gives anyone less.

| Level | May |
| --- | --- |
| View | See the folder and its dashboards. |
| Comment | Also comment and reply, and send a request to the agent, whose change reaches the dashboard as a proposal. |
| Edit | Also change dashboards and create them in the folder. |
| Admin | Also rename the folder, delete it and change its list. |

Details of Edit:

- Change and restore dashboards, and edit their details.
- Run canvas actions.
- Ask the agent to suggest a panel change.
- Create, duplicate or import dashboards into the folder.
- Move or delete the folder's dashboards.

Some rules that follow:

- A Viewer or Commenter given Edit edits in that folder, including creating dashboards and starting them with the agent, and given Admin manages it.
- An Editor given View or Comment still edits, and an Admin entry makes them the folder's Admin.
- Admins have the Admin level on every folder except other people's My Dashboards.
- The person who created a folder manages it while they are an Editor, and keeps seeing it when it is restricted, even if the list does not name them.
- Creating, duplicating and importing a dashboard need Edit on the target folder.
  A new dashboard from the agent goes to your My Dashboards, where you always have Edit.
- Creating a folder needs the Editor role.

An entry used to give a Viewer or Commenter nothing beyond their role.
Now an existing entry at Edit or Admin lifts them, so review the lists of restricted folders after you upgrade.

### Entries

An entry names a person by email address or a group by its name.
A person has to have signed in once before an entry can name them, and an unknown address is refused with `422 unknown_user`.
Groups come from the `groups` claim of the ID token, as in [Roles](roles.md), and a person's groups are read at each sign-in.
A change to a group therefore reaches folder access at the person's next sign-in.

A service-account key matches no entry, because entries name people and groups.
A key therefore sees a restricted folder only when its service account is an Admin or created the folder.
To let automation work in a restricted folder, use a key of an Admin service account.

### What the Dashboards page shows

Restricted folders on the Dashboards page show `Your access: <level>` in their header.
The page shows 50 dashboards per page.
A folder that runs onto the next page repeats its header there, marked "Continued from the previous page".

A folder header counts the folder's dashboards while no search or filter is set.
With a search or filter, it counts the folder's matching dashboards only when all matches fit on the one page, and shows no count otherwise.
Folder settings tells a person who cannot manage the folder `Your access to this folder is <level>. Only folder Admins can change it.`

### Folder routes and commands

A key that manages the folder, such as one of an Admin service account, can change the list with `observe folders permissions show`, `grant`, `revoke` and `open`.
An Admin key makes a folder Admins only with `admins-only`.
See [the command line](../use/cli.md#folders).

Over REST, `GET /api/v1/folders/{id}/permissions` reads the list, and `PUT` replaces all of it in one call.
`{"entries": [], "admins_only": true}` makes the folder Admins only.

Each folder in `GET /api/v1/folders` and `GET /api/v1/folders/{id}` carries a `kind` and the caller's `allowed_actions`.
`kind` is the first of these that fits; [States](../reference/states.md#folder) lists them too.

| `kind` | The folder |
| --- | --- |
| `general` | General. |
| `personal` | Your My Dashboards. |
| `audit` | [The Audit folder](#the-audit-folder), whoever reads it. |
| `admins_only` | Only Admins read it. |
| `restricted` | It lists people or groups. |
| `open` | Everyone sees it by role. |

`allowed_actions` names what you may do with the folder now, so a client does not work it out from the level and the kind.

| Action | Means | Offered when |
| --- | --- | --- |
| `create_in` | Create a dashboard, an observation or a saved search in it. | You have Edit. |
| `move_into` | Move one of those into it. | You have Edit, and the folder is not My Dashboards. |
| `rename` | Rename the folder. | You manage it, and it is not General. |
| `set_permissions` | Change its list or make it Admins only. | You manage it, and it is not General. |
| `delete` | Delete it once it is empty. | You manage it, and it is not General. |

A move or a save into a folder that offers the action is still refused when the folder may not hold that content; see [Admins-only folders in detail](#admins-only-folders-in-detail).

| Error | When |
| --- | --- |
| `409 general_folder` | Renaming, deleting or restricting General. |
| `409 folder_not_empty` | Deleting a folder that holds dashboards, observations or saved searches. |
| `409 personal_folder` | Moving a dashboard into My Dashboards. |
| `422 unknown_user` | An entry names an address nobody has signed in with. |

A `general_folder`, `personal_folder` or `admins_only_folder` refusal carries `allowed_actions` too: what you may do with General, with your My Dashboards or with an Admins-only folder.

### The Audit folder

The [Audit folder](../use/activity.md#audit-folder) is an ordinary Admins-only folder that the server made.
Each folder in `GET /api/v1/folders` carries a read-only `audit`, true for that one whatever it is now called and false for every other folder.
The same flag is `audit` in the MCP `list_folders` result, and `observe folders list` marks that folder `(Audit folder)`.
Its `kind` is `audit`, and `admins_only` still says whether only Admins read it.

The Dashboards page leaves that folder out, and so can any other client.
These leave the Audit folder's dashboards out of the rows, the total and the paging alike, and change nothing else:

- `GET /api/v1/dashboards?exclude_audit=true`
- MCP `list_dashboards` with `exclude_audit: true`
- `observe dashboards list --exclude-audit`

These do the same for the tag list and its counts:

- `GET /api/v1/dashboards/tags?exclude_audit=true`
- `observe dashboards tags --exclude-audit`

Without the option, every dashboard the caller can see is listed, as before.

### How the agent uses folders

The agent works as the person or key that asked.
The request needs Comment on the dashboard's folder, and the server checks it when the request is queued and again when a worker starts it.
A new dashboard from the chat goes to the requester's My Dashboards, or to General for a key.
A request whose person lost access or was deleted in between fails with a message saying so.

### What a caller without access sees

Over the API and MCP, a dashboard or a folder the caller cannot see answers `404 not_found`.
That is the same answer as an id that does not exist, so a guessed id never confirms that a hidden dashboard exists.
A dashboard the caller can see but not change answers `403 forbidden_role`.

### Old folders titled My Dashboards

A shared folder already titled My Dashboards keeps working but should be renamed.
An import that names My Dashboards goes to the importer's default folder.

### Admins-only folders in detail

An Admins-only folder is the only kind that can hold a dashboard, observation or saved search that reads an [Admins-only datasource](../connect/datasources-in-the-app.md#who-can-query-it).
While it holds one, listing someone or opening the folder is refused with `409 restricted_content`, which names the datasource and the content to move first.

It is also the only place an answer that read Admin-only data can go.
An Admin's run in it may read the [audit trail](../reference/audit-queries.md).
A `restricted` answer from a private chat is shared, saved, pinned or applied only into it (see [Answers that read Admin-only data](../use/chat.md#answers-that-read-admin-only-data)).
Elsewhere that is refused with `409 restricted_answer`.

These move only to another Admins-only folder, and the folder that holds them is not listed or opened while they are in it.
Both are refused with `409 restricted_answer`, which names the content to move first.

- A dashboard or observation that holds such an answer.
  That covers a version a run saved, the answer a dashboard was made from, a proposal a run opened, a pinned finding, a block and a thread reply.
- A dashboard or observation that a `restricted` run was about.
- An answer about another dashboard or observation that is in an Admins-only folder now.

When the refusal comes from a move, the message names the answer by its question and run id to an Admin and to the person who asked it, and only says "an answer" to anyone else.
It names the answer whenever the cause is a run the item holds, such as one that saved a version, wrote a block, opened a proposal or replied in a thread, and says "an answer" when the cause is a copy of one.
A copy of a dashboard or observation that holds such an answer goes only into an Admins-only folder, and is refused elsewhere with `409 restricted_answer`.
A copy comes from duplicating the dashboard, making a dashboard from the observation or pinning the observation onto a dashboard.
The copy then holds the answer too, so it stays in Admins-only folders as well.

While a run about a dashboard or observation in an Admins-only folder has not ended, the same two are refused with `409 restricted_answer` and a message to wait for the run to end.
What the run writes may still read Admin-only data.

An Admins-only folder has no [public links](../use/public-links.md).
Choosing **Admins only** for a folder that holds a dashboard or an RCA with an unexpired public link, or moving such a dashboard or observation into it, is refused with `409 admins_only_folder`.
Revoke the links first.

### Access request routes

`POST /api/v1/access-requests` takes `{"object": {"kind": "dashboard", "id": "<id>"}, "note": "<note>"}`, with `observation` as the other kind, and answers the request.
`GET /api/v1/access-requests` lists the open requests on the folders you manage.
Both need the Viewer role or higher.

Errors:

- `409 request_open` means you already have an open request.
  The body carries `requested_at`, the time of the open request, and a message that names it.
  The web app says how long ago it was made, and the CLI and MCP print the time.
- `409 already_can_comment` means you can already comment.
- `404 not_found` means the dashboard or observation is not visible to you, the same answer as an id that does not exist.
