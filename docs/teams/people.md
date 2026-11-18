---
title: People
description: How people join, how an Admin deactivates and reactivates them, and what happens to what they made.
---

The People page lists everyone who has signed in.
Only Admins see it.

## How a person joins

A person appears the first time they sign in through the identity provider, when the sign-in rules allow their email (`SLASHOBSERVE_ALLOWED_EMAIL_DOMAINS`, see [OIDC](../connect/oidc.md)).
Their role follows [Roles](roles.md).

## Deactivate a person

Open the person's menu on the People page and choose Deactivate.
In the same moment the server:

- deletes every session of the person, so their open tabs return to the sign-in page;
- disconnects every agent they connected over MCP;
- refuses their next sign-in with "This account was deactivated by an Admin."

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

## Over the API

`POST /api/v1/users/{id}/deactivate` and `POST /api/v1/users/{id}/reactivate` need an Admin signed in to the web app.
A service-account key cannot call them, because keys never end another person's sessions or connected agents, so the `observe` CLI and MCP have no command for them.
