---
title: Service-account keys
description: Create and revoke the keys that the CLI, MCP clients and scripts use.
---

The CLI, MCP clients and scripts authenticate with a service-account key, not a browser session.

1. Sign in to the web app as an Admin and choose **API keys** in the sidebar, or open `<public url>/keys` (the old `/settings/keys` address redirects).
2. Choose **Create key**, name the key, pick a service account or **New service account** (lowercase letters, digits and dashes, up to 63 characters, for example `ci-bot`), a role, and an expiry: in 90 days (the default), in 30 days, in 1 year, or never.
3. Choose **Create key** in the panel and copy the key.
   It is shown once, in that panel, with an `export OBSERVE_TOKEN=…` line to paste into your shell.
   The server stores only a hash of the key, so a lost key cannot be recovered; create a new one and revoke the old one.

Keys start with `sok_`.
Add more keys to an account, and revoke a key, from the same page.
A revoked or expired key is refused immediately.
The page also shows ready-made client setup snippets for your server's address; they refer to `OBSERVE_TOKEN` and never contain the key.

Only a signed-in Admin can create service accounts and keys, and only an Admin sees the API keys page; a service-account key cannot create more keys, whatever its role.
A key acts with its service account's role, which starts as Editor for a new account.
Choosing another role for an existing account in the Create key panel changes the role of every key of that account.
Folder access applies to a key as it does to a person, except that a key matches no folder entry: a restricted folder is invisible to it unless its service account is an Admin or created the folder (see [Folders and access](../teams/folders.md)).
Beyond that, see [the security model](security-model.md) for what any key can do.
