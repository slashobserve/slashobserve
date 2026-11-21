---
title: People
description: How people join, how an Admin invites, deactivates, reactivates and removes them, how teams share folders, and what happens to what they made.
---

The People page lists everyone who has signed in, and the people invited who have not signed in yet.
Only Admins see it.

## How a person joins

A person appears the first time they sign in through the identity provider, when the sign-in rules allow their email (`SLASHOBSERVE_AUTH_ALLOWED_EMAIL_DOMAINS`, see [OIDC](../connect/oidc.md)).
Their role follows [Roles](roles.md).

## Invite people

Choose Invite people, enter an email and pick the role they should have.
No email is sent: the dialog shows the sign-in link, and Copy puts it on the clipboard for you to send.
The invite appears under Pending invites, where you can change its role, copy the link again or withdraw it.

When someone first signs in with that email, and the identity provider says the email is verified, they get the invited role and it shows as set by the Admin who invited them.
The pending invite is then gone.
An invite lets no one in that the sign-in rules refuse:

- an email outside `SLASHOBSERVE_AUTH_ALLOWED_EMAIL_DOMAINS` cannot be invited;
- with `SLASHOBSERVE_AUTH_ALLOWED_EMAIL_DOMAINS=*`, a sign-in whose email is not verified gets the usual role and leaves the invite pending;
- admin emails and identity-provider group roles still decide the role over an invite, as [Roles](roles.md) describes.

An email that has already signed in cannot be invited; change that person's role in the table instead.
Only a first sign-in uses an invite: someone who signed in before keeps their role even if the identity provider later gives them the invited email, and an invite left unused by a first sign-in, such as an unverified one, stays pending until you withdraw it.

## Deactivate a person

Open the person's menu on the People page and choose Deactivate.
In the same moment the server:

- deletes every session of the person, so their open tabs return to the sign-in page;
- disconnects every agent they connected over MCP;
- refuses their next sign-in with "This account was deactivated by an Admin. Ask an Admin to reactivate it."

Everything they made stays where it is: dashboards, versions, observations, comments, proposals and folder grants.
Agent runs they requested that are still queued fail when an agent picks them up, and webhooks they own stop.
Service-account keys they created keep working, because a key acts as its service account and not as the person; revoke them on the API keys page if they should stop.

You cannot deactivate yourself, and the last active Admin cannot be deactivated.
A deactivated Admin does not count as an Admin, so the last active Admin's role cannot be lowered either.
Deactivating someone who is already deactivated changes nothing.

Deactivation applies to the identity provider account the person signed in with.
Remove the person at the identity provider too, so they cannot come back with a new account.

## Reactivate a person

Choose Reactivate on a deactivated person.
They can sign in again at once, with the role the rules give them.
The agents they had connected stay disconnected until they connect them again.

## Remove a person

Only a deactivated person can be removed: choose Remove in their menu.
The dialog says how many dashboards, observations and saved searches are in their My Dashboards, because those are deleted with them.
Their saved prompts, stars, views, inbox state and folder grants are deleted too.

What they made in shared folders stays, under the name it was saved with: dashboards and their versions, observations, comments, proposals and activity.
Service accounts and API keys they created stay and keep working.
Removal cannot be undone; if the same identity-provider account signs in later, it joins as a new person with an empty My Dashboards.
Removing the first user hands the first-user Admin rule to nobody, so keep another Admin or set `SLASHOBSERVE_ADMIN_EMAILS` first.

## Teams

A team is a named group of people that an Admin keeps in SlashObserve, for when your identity provider sends no groups or not the ones you need.
Under **Teams** on the People page, choose **New team**, then **Add person** on its row; the X on a member removes them.

A team's name works like a group from the identity provider in a folder's access list: add the name as a group entry (see [Folders and access](folders.md)), and every member gets that level.
A change applies at each member's next request, with no new sign-in.
A team never changes anyone's role, because roles come only from the identity provider's groups and the rules in [Roles](roles.md).
A name is 1 to 100 characters without control characters.
Names are unique without regard to case, but a folder entry matches the exact name; a team with the same name as an identity-provider group adds its members to that group's entries.
Deleting a team ends what its name gave its members; folder entries for the name stay and match no one until a team or an identity-provider group of that name exists.
Removing a person also removes them from every team.

## Audit log

Role changes, invites, deactivations, reactivations, removals and sign-outs everywhere are recorded in the Audit log, under **Audit** in the sidebar, which only Admins read; see [Activity](../use/activity.md#admin-changes).

## Over the API

`GET /api/v1/users`, the `/api/v1/invites` routes and the `/api/v1/teams` routes take an Admin signed in or an Admin service-account key, so an agent with an Admin key can list and invite people through [`observe people`](../use/cli.md#people) and manage teams through [`observe teams`](../use/cli.md#teams).
Inviting a pending email again answers 200 and changes its role.
No MCP tool manages people or teams: who may sign in, with which role and with which access, is an Admin's decision, as a webhook is.

`POST /api/v1/users/{id}/deactivate`, `POST /api/v1/users/{id}/reactivate` and `DELETE /api/v1/users/{id}` need an Admin signed in to the web app.
Removing an active person answers 409 with code `not_deactivated`; a deactivated person's `personal_items` gives the counts the removal deletes.
A service-account key cannot call them, because keys never end another person's sessions or connected agents, so the `observe` CLI and MCP have no command for them.
