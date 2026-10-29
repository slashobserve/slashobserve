---
title: Logging
description: What the server logs and what it never logs.
---

Logs are JSON by default on standard error.
The server does not log secrets, prompt text, query text or tool results.
Configuration secrets such as the database URL, the OIDC client secret and the LLM key are never logged.
Request logs record the method, the path (never the query string), the status and the duration.

Errors are logged without their message, since a message from a driver or library can echo a query, a connection detail or a credential.
Those lines carry:

- `error_code`, a short fixed code such as `tool_failed`, `session_lookup_failed` or `readiness_check_failed`
- `error_type`, the Go type of the innermost error
- `sqlstate`, when the cause is a Postgres error
- `errno`, the operating system's text (such as `connection refused`) when the cause is a system error
- the ids that locate the failure, such as `agent_request_id` (look it up with `observe status <id>` or `GET /api/v1/observe/{id}`), `dashboard_id`, `principal_id` and a datasource `uid`

A panic is logged by its Go type (`panic_type`), never its value.
A refused sign-in also logs the domain of the email address, and a failed code exchange logs the provider's `oauth_error` code (such as `invalid_grant`) when it is a plain code.

A fatal error at startup is logged as `server exiting` with an `error_code`.
A message the server wrote itself from your settings is kept, for example `SLASHOBSERVE_SESSION_TTL: must be between 5m and 720h`, and so are the listen address errors from the operating system.
Setting values that are URLs are never echoed back.
A failure to reach Postgres, apply migrations, build a datasource or set up sign-in is logged by code and error type, so the log never shows the connection string, the user or the database name.
To help you find the cause, the line adds only what the server knows from your settings or from the operating system:

- `database_unavailable` adds `db_host` and `db_port` from `SLASHOBSERVE_DATABASE_URL` and, for a network failure, `errno` (for example `connection refused`).
  A Postgres error adds `sqlstate`.
  `invalid_database_url` means the setting could not be parsed.
- `migration_failed` adds `migration_version` and `sqlstate`.
- `datasources_failed` adds `datasource_uid` and, for a validation failure the server wrote itself such as a limit or URL scheme problem, a `detail` message.

For anything else, test the connection yourself with `psql "$SLASHOBSERVE_DATABASE_URL"`.
