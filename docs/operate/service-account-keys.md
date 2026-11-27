---
title: Service-account keys
description: Create and revoke the keys that the CLI, MCP clients and scripts use, declare them in a file, and run a server with no browser sign-in.
---

The CLI, MCP clients and scripts sign in with a service-account key, not a browser session.
A service account is the identity behind one or more keys, and it has a role.
Only a signed-in Admin can create service accounts and keys.

## Create a key

1. Sign in to the web app as an Admin and open `<public url>/keys`.
   The **API keys** link on the Agents page opens the same page, and the old `/settings/keys` address redirects to it.
2. Choose **Create key**.
3. In the panel, name the key and pick the account:
   - **New service account** is the default, so an existing account is never changed by accident.
     Name it with lowercase letters, digits and dashes, up to 63 characters, for example `ci-bot`.
   - Or pick an existing service account.
4. Pick a role, which is the role of the account and so of every key it has.
   A new account starts as Editor.
5. Pick an expiry: **In 90 days** (the default), **In 30 days**, **In 1 year** or **Never**.
6. Choose **Create key** in the panel and copy the key.
   The panel shows the key once, with an `export OBSERVE_TOKEN=...` line to paste into your shell.
   If the new service account was created but the key was not, the panel says so and keeps the account picked: choose **Issue the key again**, which never creates a second account.

The server stores only a hash of the key, so a lost key cannot be recovered.
Create a new one and revoke the old one.
Keys start with `sok_`.
The page also shows ready-made client setup snippets for your server's address; they refer to `OBSERVE_TOKEN` and never contain the key.

To use the key, see [CLI](../use/cli.md) and [MCP](../use/mcp.md).

## Add or revoke a key

Add more keys to an account from the same **Create key** panel by picking the account.
To revoke a key, choose **Revoke** in its row on the keys page.
The server refuses a revoked or expired key immediately.

Choosing another role for an existing account in the **Create key** panel changes the role of every key of that account.
The table shows each key's role, which is its account's role.

## Delete a service account

The **Service accounts** list under the keys shows every account, including one with no keys left, with its role and number of working keys.
Choose **Delete** on an account to remove it and all its keys.
Every key stops working at once, and what the account made stays under its name.

You cannot delete an account from the service-accounts file here.
`DELETE /api/v1/service-accounts/{id}` answers 409 `provisioned` for it; remove it from the file instead.
Like creating keys, deleting an account needs an Admin signed in to the web app; a key cannot delete an account.

## Let an account take agent runs

An account takes runs only when an Admin turns on **Takes agent runs** in the **Create key** panel, or sets `takes_runs` with `PATCH /api/v1/service-accounts/{id}`.
The account then lists as `listening` or `working` while it asks for work.
The switch applies to every key of the account, like the role, and the panel saves it when you create the key.

A service account that takes runs works under the same guards as the built-in agent, which the server enforces on every call of the run's session.
See [MCP](../use/mcp.md#the-run-session-enforces-the-guards).

## Declare service accounts in a file

A tool that must start with a key and no person in the loop, such as a deploy pipeline or the demo's load generator, gets its key from a file the server reads at boot.

1. Generate a key and keep it out of version control:

   ```sh
   printf 'sok_%s\n' "$(openssl rand -base64 32 | tr '+/' '-_' | tr -d '=\n')" > deploy-bot.key
   ```

2. Write a YAML file that lists the accounts:

   ```yaml
   serviceAccounts:
     - name: deploy-bot
       role: editor
       description: Posts deploy markers from CI
       keyFile: /run/secrets/deploy-bot.key
   ```

3. Point `SLASHOBSERVE_SERVICE_ACCOUNTS_FILE` at the file and start the server.
   On Kubernetes, see [Service accounts from a file](kubernetes.md#service-accounts-from-a-file).

At each start the server creates every listed account, marked as provisioned, or updates its role, description and `takesRuns` to match the file.
It keeps the key from `keyFile` as the account's one active key named `From file`, stored only as a SHA-256 hash, and never writes the key to a log or an error.

Each entry takes these fields.

| Field | Rule |
| --- | --- |
| `name` | Required. Same rule as in the web app, and each name appears once. |
| `role` | Required. `viewer`, `editor` or `admin`. |
| `description` | Optional, up to 256 characters. |
| `takesRuns` | Optional, false by default. True lets the account claim its assigned runs over MCP with `claim_run`. |
| `keyFile` | Required. Names a file that holds one key. Each account needs its own key. |

Details of `keyFile`:

- The file holds `sok_` followed by 43 base64url characters.
- The server ignores whitespace around the key.

The server refuses to start when the file is invalid.
It also refuses when an account with a listed name already exists but was created in the web app or the API, because it never takes over such an account.

### Change or rotate a provisioned account

A provisioned account is managed through the file only.
These requests answer 409 `provisioned`:

- Changing its role in the web app, or its role or `takes_runs` with `PATCH /api/v1/service-accounts/{id}`.
- Creating a key for it, or revoking one of its keys.

The web app lists such a key as "from file" and offers no **Revoke** for it.
It locks the **Takes agent runs** switch.

To rotate the file key, write a new key to its file and restart the server.
The server revokes the old file key and the new one replaces it.
A key that was revoked before cannot come back; write a fresh one.
Removing an account from the file leaves it and its key in place.

## Run with no browser sign-in

A server that only tools use, such as one a benchmark harness starts in a container, can run with no identity provider.

1. Leave all three of `SLASHOBSERVE_OIDC_ISSUER_URL`, `SLASHOBSERVE_OIDC_CLIENT_ID` and `SLASHOBSERVE_OIDC_CLIENT_SECRET` unset.
2. Declare the keys in a [service-accounts file](#declare-service-accounts-in-a-file), with an `admin` account when the tool needs Admin calls.
3. Start the server, for example with these settings and keys and datasources in files:

   ```sh
   SLASHOBSERVE_DATABASE_URL=postgres://slashobserve:<password>@postgres:5432/slashobserve
   SLASHOBSERVE_PUBLIC_URL=http://localhost:8080
   SLASHOBSERVE_SERVICE_ACCOUNTS_FILE=/etc/slashobserve/service-accounts.yaml
   SLASHOBSERVE_DATASOURCES_FILE=/etc/slashobserve/datasources.yaml
   SLASHOBSERVE_LLM_PROVIDER=anthropic
   SLASHOBSERVE_LLM_MODEL=<model>
   SLASHOBSERVE_LLM_API_KEY_FILE=/run/secrets/llm.key
   ```

4. Wait for `GET /readyz` to answer 200.
5. Start runs with the key, for example `POST /api/v1/observe` with `kind` `investigate`, and read the finding from the run; see [Investigate](../use/investigate.md).

The server starts without contacting any identity provider and logs that browser sign-in is off.
A browser sign-in goes back to the login page with the error `sign_in_off`, and a session cookie from before no longer authenticates.
Every request authenticates with a key from the service-accounts file.

The server refuses to start with only some of the three OIDC settings, so a normal install that misses one fails instead of losing its sign-in.
It also refuses the settings that only browser sign-in reads when the three are unset:

- `SLASHOBSERVE_OIDC_INTERNAL_URL`
- `SLASHOBSERVE_AUTH_ALLOWED_EMAIL_DOMAINS`
- `SLASHOBSERVE_ADMIN_EMAILS`
- `SLASHOBSERVE_OIDC_GROUP_ROLES`
- `SLASHOBSERVE_AUTH_PROVIDER_NAME`

To add browser sign-in later, set all three OIDC settings and the allowed domains, then restart.

## Reference

### Key states

Each key listed over the API has a `state`; see [States](../reference/states.md).

| State | Meaning |
| --- | --- |
| `active` | The key works. |
| `expiring` | The key works and expires within 7 days. |
| `expired` | The key's expiry has passed, and the server refuses it. |
| `revoked` | An Admin revoked the key, and the server refuses it. |

### What a key can do

- A key acts with its service account's role.
- A service-account key cannot create more keys, whatever its role, and only an Admin sees the API keys page.
- Folder access applies to a key as it does to a person, except that a key matches no folder entry.
  A restricted folder is invisible to it unless its service account is an Admin or created the folder; see [Folders and access](../teams/folders.md).
- A key has no My Dashboards folder.
- The Audit log records creating, changing and deleting an account, and creating and revoking a key, never with the key; see [Activity](../use/activity.md#admin-changes).

[The security model](security-model.md) describes what any key can do.
