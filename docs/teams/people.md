---
title: People
description: How people join, how an Admin invites, deactivates, reactivates and removes them, how teams share folders, and what happens to what they made.
---

The People page lists everyone who has signed in, and the people invited who have not signed in yet.
Only Admins see it.
Use it to invite people with a role, to stop someone from signing in, and to group people into teams that share folders.

A person appears the first time they sign in through the identity provider, when the sign-in rules allow their email (`SLASHOBSERVE_AUTH_ALLOWED_EMAIL_DOMAINS`, see [OIDC](../connect/oidc.md)).
Their role follows [Roles](roles.md).

## Invite people

An invite gives a person a role before their first sign-in.
No email is sent.
You send the sign-in link yourself.

1. Choose **People** in the sidebar, then **Invite people**.
2. Enter the email, pick the **Role** the person should have, and confirm.
   The dialog shows the sign-in link, which is the `/login` page of your server.
3. Choose **Copy** and send the link.
   The invite appears under **Pending invites**, where you can change its role, choose **Copy sign-in link** again or choose **Withdraw**.

From the CLI, `observe people invite <email> --role <role>` does the same, `observe people invites` lists pending invites, and `observe people uninvite <invite id>` withdraws one.
See [`observe people`](../use/cli.md#people).

When someone first signs in with that email and the identity provider says the email is verified, they get the invited role.
It shows as set by the Admin who invited them, and the pending invite is gone.

### Limits of an invite

An invite lets no one in that the sign-in rules refuse.

- An email outside `SLASHOBSERVE_AUTH_ALLOWED_EMAIL_DOMAINS` cannot be invited.
  The API answers 422 `email_domain_not_allowed`.
- With `SLASHOBSERVE_AUTH_ALLOWED_EMAIL_DOMAINS=*`, a sign-in whose email is not verified gets the usual role and leaves the invite pending.
- Admin emails and identity-provider group roles still decide the role over an invite, as [Roles](roles.md) describes.

An email that has already signed in cannot be invited.
The API answers 409 `already_member`; change that person's role in the table instead.
Only a first sign-in uses an invite.
Someone who signed in before keeps their role, even if the identity provider later gives them the invited email.
An invite left unused, such as one used by an unverified sign-in, stays pending until you withdraw it.

## Deactivate a person

Deactivating stops a person from signing in and ends what they have open.

1. Open the person's menu on the People page and choose **Deactivate**.
2. Confirm.

In the same moment the server does these:

- It deletes every session of the person, so their open tabs return to the sign-in page.
- It disconnects every agent they connected over MCP.
- It refuses their next sign-in with "This account was deactivated by an Admin. Ask an Admin to reactivate it."

Everything they made stays where it is: dashboards, versions, observations, comments, proposals and folder grants.
Agent runs they requested that are still queued fail when an agent picks them up, and webhooks they own stop.
Service-account keys they created keep working, because a key acts as its service account and not as the person.
Revoke those keys on the API keys page if they should stop.

You cannot deactivate yourself.
The last active Admin cannot be deactivated, and a deactivated Admin does not count as an Admin, so the last active Admin's role cannot be lowered either.
Deactivating someone who is already deactivated changes nothing.

Deactivation applies to the identity provider account the person signed in with.
Remove the person at the identity provider too, so they cannot come back with a new account.

## Reactivate a person

Choose **Reactivate** in the menu of a deactivated person.
They can sign in again at once, with the role the rules give them.
The agents they had connected stay disconnected until they connect them again.

## Remove a person

Removing deletes the person for good.
Only a deactivated person can be removed.

1. Open the deactivated person's menu and choose **Remove**.
2. Read the dialog.
   It says how many dashboards, observations and saved searches are in their My Dashboards, because those are deleted with them.
3. Confirm.

Their saved prompts, stars, views, inbox state and folder grants are deleted too.
What they made in shared folders stays, under the name it was saved with: dashboards and their versions, observations, comments, proposals and activity.
Service accounts and API keys they created stay and keep working.

Removal cannot be undone.
If the same identity-provider account signs in later, it joins as a new person with an empty My Dashboards.
Removing the first user hands the first-user Admin rule to nobody, so keep another Admin or set `SLASHOBSERVE_ADMIN_EMAILS` first.

## Share folders with a team

A team is a named group of people that an Admin keeps in SlashObserve, for when your identity provider sends no groups or not the ones you need.

1. On the People page, under **Teams**, choose **New team** and enter a name.
2. Choose **Add person** on the team's row and pick each member.
   The X on a member removes them.
3. Open a folder's settings and add the team's name as a group entry; see [Folders and access](folders.md).
   Every member gets that level.

From the CLI, `observe teams` has `list`, `create <name>`, `delete <team id>`, `add <team id> <person id>` and `remove <team id> <person id>`.
See [`observe teams`](../use/cli.md#teams).

## Teams

A team's name works like a group from the identity provider in a folder's access list.
A change to a team applies at each member's next request, with no new sign-in.
A team never changes anyone's role, because roles come only from the identity provider's groups and the rules in [Roles](roles.md).

A team name is 1 to 100 characters without control characters.
Names are unique without regard to case, but a folder entry matches the exact name.
A team with the same name as an identity-provider group adds its members to that group's entries.
Deleting a team ends what its name gave its members.
Folder entries for the name stay and match no one until a team or an identity-provider group of that name exists.
Removing a person also removes them from every team.

## Find who changed a role or an invite

Role changes, invites, deactivations, reactivations, removals and sign-outs everywhere are recorded in the Audit log.
Choose **Audit** in the sidebar.
Only Admins read it; see [Activity](../use/activity.md#admin-changes).

## Reference

### Who may call what

`GET /api/v1/users`, the `/api/v1/invites` routes and the `/api/v1/teams` routes take an Admin signed in or an Admin service-account key.
An agent with an Admin key can therefore list and invite people through [`observe people`](../use/cli.md#people) and manage teams through [`observe teams`](../use/cli.md#teams).
Inviting a pending email again answers 200 and changes its role.

No MCP tool manages people or teams.
Who may sign in, with which role and with which access, is an Admin's decision, as a webhook is.

### Routes that need a signed-in Admin

These routes need an Admin signed in to the web app:

| Route | Use |
| --- | --- |
| `PATCH /api/v1/users/{id}` | Change a person's role. |
| `DELETE /api/v1/users/{id}/sessions` | Sign a person out everywhere. |
| `POST /api/v1/users/{id}/deactivate` | Deactivate. |
| `POST /api/v1/users/{id}/reactivate` | Reactivate. |
| `DELETE /api/v1/users/{id}` | Remove a deactivated person. |

A service-account key cannot call them, because keys never end another person's sessions or connected agents.
The `observe` CLI and MCP have no command for them.

### Errors

| Status and code | When |
| --- | --- |
| 409 `not_deactivated` | Removing an active person. A deactivated person's `personal_items` gives the counts the removal deletes: `dashboards`, `observations` and `saved_searches`. |
| 409 `cannot_deactivate_self` | Deactivating yourself. |
| 409 `last_admin` | Deactivating the last active Admin. |
| 409 `already_member` | Inviting an email that has signed in. |
| 422 `email_domain_not_allowed` | Inviting an email the sign-in rules refuse. |
| 404 `not_found` | An unknown person or invite. |
