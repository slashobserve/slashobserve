---
title: Logging
description: What the server logs and what it never logs.
---

Logs are JSON by default on standard error.
The server does not log secrets, prompt text, query text or tool results.
Configuration secrets such as the database URL, the OIDC client secret and the LLM key are never logged.
Request logs record the method, the path (never the query string, and with the token of a public link replaced by `:token`), the status, the response size in `bytes` and the duration.
They also carry the `route` template, and for `/api/v1`, `/mcp` and the OAuth routes the `request_id` and, once the caller is signed in, the `principal` (`user:<id>` or `service_account:<id>`).
The `request_id` is the value of the response's `X-Request-Id` header and of the audit event the request records, so one id joins the three.

Errors are logged without their message, since a message from a driver or library can echo a query, a connection detail or a credential.
Those lines carry:

- `error_code`, a short fixed code such as `tool_failed`, `session_lookup_failed` or `readiness_check_failed`
- `error_type`, the Go type of the innermost error
- `sqlstate`, when the cause is a Postgres error
- `errno`, the operating system's text (such as `connection refused`) when the cause is a system error
- the ids that locate the failure, such as `agent_request_id` (look it up with `observe status <id>` or `GET /api/v1/observe/{id}`), `dashboard_id`, `principal_id` and a datasource `uid`

The built-in agent runs in a child process, `observe-agent`, and the server copies its log lines into its own log with `component=observe-agent`.
The child logs at the server's `SLASHOBSERVE_LOG_LEVEL`.
A line of the child that is not JSON is logged as `observe-agent output` with the text in `line`, and a line longer than 4096 bytes is cut and marked `truncated=true`.

A failed call to the model provider logs one `llm call finished` line at `WARN`; a call that succeeded logs nothing.
The line carries `run_id` (the id of the agent request), `provider`, `status` (the provider's last HTTP status, or 0 when the last attempt got no answer) and `failure_code` (`call_timeout`, `rate_limited` or `provider_error`), plus `idle_timeout` when the call stalled, and, when the provider sent them, `provider_error_type`, `provider_error_code`, `provider_error_param`, `request_id` and `retry_after`.
These values are logged only when they are plain tokens.
The line never holds the prompt, the model's reply, a tool input or result, or the provider's error message, because a provider can echo any of them back, unless you turn the message on (see [Provider error messages](#provider-error-messages)).

A panic is logged by its Go type (`panic_type`), never its value.
A datasource that turns unhealthy logs `datasource unhealthy` once, with its `uid` and `type`; it does not log again until the datasource has recovered.
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

## Log levels

Set the level with `SLASHOBSERVE_LOG_LEVEL` (`debug`, `info`, `warn` or `error`, default `info`) and the format with `SLASHOBSERVE_LOG_FORMAT` (`json` or `text`).

| Level   | Meaning                                                                                                                      |
| ------- | ---------------------------------------------------------------------------------------------------------------------------- |
| `DEBUG` | Detail for each step, such as the health checks, scrapes of `/metrics` and the agent's `run brief loaded`.                   |
| `INFO`  | Normal lifecycle: startup, `observe-agent started`, an agent request that saved, a run that started, finished or was ended by the server. |
| `WARN`  | Degraded or recoverable: a failed model call, an agent request or run that failed, a datasource that turned unhealthy.        |
| `ERROR` | A failure you must act on: the database, a migration, a run result the server could not record, or `run loop crashed` in the agent. |

## Agent runs

The server logs one line when an agent request ends.
`agent request succeeded` is at `INFO` and carries `agent_request_id`, `dashboard_id` when the request has a dashboard, `version` when it saved one, and `attempt`.
`agent request failed` is at `WARN` and carries the same ids with `error_code`, the failure code the request shows, such as `rate_limited`, `run_timeout`, `budget_exhausted` or `lease_expired`.
The agent logs `run started` (`run_id`, `attempt`, `kind`, `mode`) and `run finished` at `INFO` with `model_calls`, `retries`, `compactions`, `renewals` and `model_ms`, and `run failed` at `WARN` with the same counters and its `failure_code`.
No line holds a tool input or result.

## Provider error messages

Set `SLASHOBSERVE_LLM_LOG_PROVIDER_ERRORS=true` to add the provider's own error message to a failed `llm call finished` line as `provider_error_message`.
The message has the API key removed, newlines and other control characters replaced by spaces, and is cut to 1024 bytes.
It is never added to the error stored on an agent request or returned by the API.
A provider can echo parts of the prompt in its message, and the server logs one `WARN` at startup while the setting is on, so use it for debugging only.
