---
title: Service-account keys
description: Create and revoke the keys that the CLI, MCP clients and scripts use.
---

The CLI, MCP clients and scripts authenticate with a service-account key, not a browser session.

1. Sign in to the web app as an Admin and open `<public url>/keys`, which the **API keys** link on the Agents page also opens (the old `/settings/keys` address redirects).
2. Choose **Create key**, name the key, pick a service account or **New service account** (lowercase letters, digits and dashes, up to 63 characters, for example `ci-bot`), a role, and an expiry: in 90 days (the default), in 30 days, in 1 year, or never.
3. Choose **Create key** in the panel and copy the key.
   It is shown once, in that panel, with an `export OBSERVE_TOKEN=…` line to paste into your shell.
   The server stores only a hash of the key, so a lost key cannot be recovered; create a new one and revoke the old one.

Keys start with `sok_`.
Add more keys to an account, and revoke a key, from the same page.
A revoked or expired key is refused immediately.
Each key listed over the API has a `state`: `active`, `expiring` within 7 days of its expiry, `expired` or `revoked`; see [States](../reference/states.md).
The page also shows ready-made client setup snippets for your server's address; they refer to `OBSERVE_TOKEN` and never contain the key.

Only a signed-in Admin can create service accounts and keys, and only an Admin sees the API keys page; a service-account key cannot create more keys, whatever its role.
A key acts with its service account's role, which starts as Editor for a new account.
Choosing another role for an existing account in the Create key panel changes the role of every key of that account.

An account takes runs only when an Admin turns on **Takes agent runs** in the Create key panel, or sets `takes_runs` with `PATCH /api/v1/service-accounts/{id}`; the account then lists as `listening` or `working` while it asks for work.
The switch applies to every key of the account, like the role, and is saved when you create the key.
Folder access applies to a key as it does to a person, except that a key matches no folder entry: a restricted folder is invisible to it unless its service account is an Admin or created the folder (see [Folders and access](../teams/folders.md)).
A key has no My Dashboards folder.
A service account that takes runs works under the same guards as the built-in agent, enforced by the server on every call of the run's session; see [MCP](../use/mcp.md#the-run-session-enforces-the-guards).
Beyond that, see [the security model](security-model.md) for what any key can do.

## Declare service accounts in a file

A tool that must start with a key and no person in the loop, such as a deploy pipeline or the demo's load generator, gets its key from a file the server reads at boot.
Point `SLASHOBSERVE_SERVICE_ACCOUNTS_FILE` at a YAML file that lists the accounts:

```yaml
serviceAccounts:
  - name: deploy-bot
    role: editor
    description: Posts deploy markers from CI
    keyFile: /run/secrets/deploy-bot.key
```

- `name` follows the same rule as in the web app, and each name appears once.
- `role` is `viewer`, `editor` or `admin`.
- `description` is optional, up to 256 characters.
- `takesRuns` is optional and false by default; true lets the account claim the runs assigned to it over MCP with `claim_run`.
- `keyFile` is required and names a file that holds one key: `sok_` followed by 43 base64url characters, with surrounding whitespace ignored.
  Each account needs its own key.
  Generate one with `printf 'sok_%s\n' "$(openssl rand -base64 32 | tr '+/' '-_' | tr -d '=\n')"` and keep it out of version control.

At each start the server creates every listed account, marked as provisioned, or updates its role, description and `takesRuns` to match the file.
It keeps the key from `keyFile` as the account's one active key named `From file`, stored only as a SHA-256 hash, and never writes the key to a log or an error.
The server refuses to start when the file is invalid, or when an account with a listed name already exists but was created in the web app or the API, because it never takes over such an account.

A provisioned account is managed through the file only.
Changing its role in the web app, or its role or `takes_runs` with `PATCH /api/v1/service-accounts/{id}`, answers 409 `provisioned`, and so does revoking one of its keys.
The web app locks its **Takes agent runs** switch.
To rotate the file key, write a new key to its file and restart the server: the old file key is revoked and the new one replaces it.
A key that was revoked before cannot come back; write a fresh one.
Removing an account from the file leaves it and its key in place.
