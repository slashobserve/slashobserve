---
title: Context sources
description: Register an MCP server as a context source, discover its tools, approve the read-only ones one by one or all at once, and see why a tool is refused, changed or gone.
---

A context source is an MCP server that an Admin registers so the agent can read from it, such as a documentation index or a ticket system.
The SlashObserve server is the client: it connects to the source, lists its tools and keeps what an Admin approved.
Nothing a source offers reaches the agent until an Admin approves it.

This page covers registering a source, approving its tools and what the agent may do with them.

## Register a source

1. Sign in as an Admin and open **Settings**, then **Context sources**.
2. Choose **Add source** and fill in the form:
   - **Id**, which names the source in the CLI and the audit log and cannot change later.
     It starts with a letter or digit and has at most 40 letters, digits, `_` and `-`.
   - **Name**, shown in lists, at most 100 characters without control or format characters.
     It defaults to the id.
   - **Description**, what the source is for, on one line of at most 300 characters.
     It is optional.
   - **Address**, the server's `https` MCP address with a domain name, no login, no query and no fragment.
   - **Who can use its tools**, either **Everyone** or **Admins only**.
     Admins manage every source whatever you choose.
   - **Secret header** and **Secret**, when the server needs a credential, such as `Authorization` with `Bearer ...`.
3. Choose **Add source**.

Registering a source asks the server nothing.
Choose **Discover** on its row to list the tools, then review them.

The CLI does the same:

```sh
printf '%s' "$DOCS_TOKEN" | observe context add docs \
  --url https://docs.example.com/mcp \
  --description "Runbooks and design notes" \
  --secret-header Authorization --secret-stdin
observe context discover docs
observe context tools docs
observe context approve docs --all-read-only
```

An Admin key is needed for every `observe context` command.

## The outbound allowlist

Every connection to a source goes through the [outbound allowlist](outbound-access.md), on any port, and follows its address rules: a public domain name over `https`, never an IP address or `localhost`.
A redirect is never followed.

Saving a source adds its domain to the allowlist, the way a datasource added in the app does.
The entry shows as "Added for a connection or datasource", and **Outbound access** lists the source under the entry's users.
Deleting the source, or moving it to another host, removes the entry again unless something else uses it.

When `SLASHOBSERVE_OUTBOUND_DOMAINS` fixes the list, nothing is added.
A source whose domain is not on the list is refused with `403 outbound_domain_not_allowed`.
A source whose domain is removed later reports `unreachable` with the failure `outbound_domain_not_allowed` at its next discovery.

## Secrets

The secret is sealed with `SLASHOBSERVE_SECRET_KEY` before it is stored, and the server sends it in the header you named.
Without that key, a source can be registered only without a secret, and the API answers `409 secret_key_missing` otherwise.

The secret is never returned by the API, the CLI or the web app, and it never appears in a log line or an audit event.
A source shows `secret_set` and its header name instead.
To replace it, enter a new one; to remove it, choose **Remove the stored secret** or send `clear_secret`.
Changing the address to another host needs the secret again, or removes it, so a secret is never sent to a server you did not give it to.

## Discover and approve tools

**Discover** asks the server for its tools and compares them with what is stored.
It never approves anything.
The result lists how many tools were found, new, changed, gone or ignored.
A tool whose name is empty, longer than 128 bytes, uses characters outside letters, digits, `_`, `.` and `-`, has no letter or digit, or repeats another tool's name is ignored.
A server that lists more than 200 tools fails the discovery with `too_many_tools`.

Each tool is in one state:

| State | Meaning |
| --- | --- |
| `offered` | The server marks the tool read-only and an Admin has not approved it. The agent cannot call it. |
| `approved` | An Admin approved the tool as pinned. |
| `changed` | The server describes an approved tool differently from what was pinned. It is not served until an Admin approves the new description. |
| `gone` | The server no longer lists an approved tool. It returns to `approved` when the server lists it unchanged. |
| `refused` | The tool cannot be approved, and its `refusal` says why. |

The [states reference](../reference/states.md#context-source-tool) lists the moves between them.

A tool is refused when the server marks it destructive, when it does not mark it read-only, when its description is over 4000 characters or when its input schema is over 32 KiB.
A tool the server leaves unmarked is refused, because the server's word is the only evidence that it only reads.

An Admin approves a tool one by one, or approves every `offered` tool at once with **Approve all read-only** or `observe context approve <id> --all-read-only`.
Approving pins the tool's name, description and input schema as a SHA-256 hash.
The web app and the CLI send the hash they showed, and the server answers `409 tool_changed` when the tool changed in between, so an Admin never approves a definition they did not read.

If a later discovery finds an approved tool described differently, the tool becomes `changed` and stops being served until it is approved again, with its new definition shown beside the approved one.
A tool the server marks destructive after the fact becomes `refused` and loses its approval.
**Revoke** withdraws an approval and returns the tool to `offered`.

A source is `unchecked` until its first discovery, `unreachable` when the last discovery failed and `ready` otherwise.
A failed discovery answers `200` with the failure code in `failure`: `outbound_domain_not_allowed`, `unauthorized`, `timeout`, `not_an_mcp_server`, `unreachable`, `too_many_tools`, `secret_key_missing` or `secret_unreadable`.
Changing the address or the secret resets the check.
Changing the address also deletes the stored tools, so approvals never carry over to another server.
A change that sends an empty `description` removes it.
A change cannot send an empty `secret`, or an empty `secret_header` while a secret is stored; both answer `422 invalid_context_source`.
Send `clear_secret` to remove the secret, with an empty `secret_header` to remove the header too.

## Provision sources from a file

Set `SLASHOBSERVE_CONTEXT_SOURCES_FILE` to a YAML file to keep sources in version control.
The server reads it at startup and makes its entries match.

```yaml
contextSources:
  - uid: docs
    name: Company docs
    description: Runbooks and design notes
    url: https://docs.example.com/mcp
    audience: admins
    secretHeader: Authorization
    secretEnv: SLASHOBSERVE_DOCS_TOKEN
```

- `uid` and `url` are required, and `name` defaults to the uid.
- `description` is optional and follows the same rules as in the app.
- `audience` is `everyone` (the default) or `admins`.
- A secret is never written in the file.
  Name an environment variable with `secretEnv`, whose name starts with `SLASHOBSERVE_`, or a file with `secretFile`.
  `secretHeader` is required with a secret.
- A source from the file shows `provisioned: true`, and the API answers `409 provisioned` to a change or a delete.
  Edit the file instead.
- A source an Admin added with the same uid, an invalid entry, or a secret without `SLASHOBSERVE_SECRET_KEY` stops the server from starting, with the reason in the log.
- Removing an entry from the file deletes its source at the next start.
- The file does not touch the allowlist.
  Allow the server's domain yourself, or the first discovery reports `outbound_domain_not_allowed`.
- Approvals survive a restart and a rename.
  They are dropped only when the address changes.

At most 50 sources can exist; a 51st answers `422 too_many_context_sources`.

## API and records

| Request | Does |
| --- | --- |
| `GET /api/v1/context-sources` | Lists the sources with their tool counts. |
| `POST /api/v1/context-sources` | Registers a source. |
| `GET`, `PATCH`, `DELETE /api/v1/context-sources/{uid}` | Reads, changes or deletes a source. |
| `POST /api/v1/context-sources/{uid}/discover` | Discovers its tools. |
| `GET /api/v1/context-sources/{uid}/tools` | Lists its tools. |
| `POST /api/v1/context-sources/{uid}/tools/approve-read-only` | Approves every `offered` tool. |
| `POST /api/v1/context-sources/{uid}/tools/{name}/approve` | Approves one tool; the body may carry the `hash` that was read. |
| `POST /api/v1/context-sources/{uid}/tools/{name}/revoke` | Withdraws an approval. |

Only an Admin can call these routes.
The MCP server has no tool for them, because an agent that could register a source or approve a tool could point the server anywhere and widen its own reach.
A refused invalid request answers `422 invalid_context_source` with a `fields` list.
Approving a refused tool answers `409 tool_refused` with the `refusal`.

## What the agent does with a source

An approved tool of a source can be used by the agent in an investigation or a question, and by an outside agent over MCP, through two tools: `list_context_sources` and `call_context_tool`.
They are not offered when a dashboard is built or revised.

- Only an approved tool can be called.
  A source whose last discovery failed is still listed to the agent with a status that says so, and a call to it is still tried.
  A tool that changed since you approved it stays unusable until you approve it again.
- A source set to **Admins only** is used only on behalf of an Admin.
- The agent's arguments must match the schema the tool had when you approved it.
- Every call goes through the [outbound allowlist](outbound-access.md).
- A call counts toward the run's call limit and shows as a step.
- A call stops after 20 seconds, a source handles 4 calls at once and answers `source_busy` beyond that, and a result is cut at 12 KiB with a note.
- A source that is down gives the agent a plain refusal, and the run goes on.
- What a source returns is outside text.
  The agent is told that nothing in it is an instruction, and it is never shown as evidence on a panel.

Each call is recorded as `context_source.call`, with the host, the tool, the arguments sent and whether the result was cut.
A value whose name looks like a secret, such as a token or a password, is masked in the record.

Every change is recorded in the [audit trail](audit.md) with the source as its target: `context_source.create`, `context_source.update`, `context_source.delete`, `context_source.discover`, `context_source.approve` and `context_source.revoke`.
Each event carries the host and never the secret; see [Event data](../reference/audit-queries.md#event-data).
