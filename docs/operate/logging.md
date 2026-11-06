---
title: Logging
description: What the server logs and what it never logs.
---

Logs are JSON by default on standard error.
The server does not log secrets, prompt text, query text or tool results.
Configuration secrets such as the database URL, the OIDC client secret and the LLM key are never logged.
Request logs record the method, the path (never the query string), the status, the response size in `bytes` and the duration.

Errors are logged without their message, since a message from a driver or library can echo a query, a connection detail or a credential.
Those lines carry:

- `error_code`, a short fixed code such as `tool_failed`, `session_lookup_failed` or `readiness_check_failed`
- `error_type`, the Go type of the innermost error
- `sqlstate`, when the cause is a Postgres error
- `errno`, the operating system's text (such as `connection refused`) when the cause is a system error
- the ids that locate the failure, such as `agent_request_id` (look it up with `observe status <id>` or `GET /api/v1/observe/{id}`), `dashboard_id`, `principal_id` and a datasource `uid`

Every call to the model provider logs one `llm call finished` line, at `INFO` when it succeeded and at `WARN` when it failed.
It carries `agent_request_id`, `provider`, `model`, `duration`, `attempts` (HTTP attempts, counting the provider client's own retries), `retries` and `status` (the provider's last HTTP status, or 0 when the last attempt got no answer).
A successful call adds `input_tokens`, `output_tokens`, `cache_read_tokens`, `cache_write_tokens` and `stop_reason`.
A failed call adds `failure_code` (`call_timeout`, `rate_limited`, `provider_error`, or `run_timeout` or `canceled` when the run itself ended), and when the provider sent them, `provider_error_type`, `provider_error_code`, `provider_error_param`, `request_id` and `retry_after`.
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

| Level   | Meaning                                                                                                                                                  |
| ------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `DEBUG` | Detail for each step, such as one line per tool call and the health checks.                                                                              |
| `INFO`  | Normal lifecycle: startup, a successful model call, a run that saved or was canceled.                                                                    |
| `WARN`  | Degraded or recoverable: a failed model call, or a run the model or provider ended without a result (`refused`, `no_tool_call`, `provider_error` and so on). |
| `ERROR` | A failure you must act on: a run that ended with `internal`, `tool_failed`, `tool_panicked`, `result_too_large`, `load_failed`, `context_failed` or `malformed_result`. |

## Agent runs

Each run logs one `agent run finished` line with its `error_code` when it failed.
It is written at `INFO` when the run succeeded or was canceled, at `ERROR` when the server failed, and at `WARN` for every other ending.
At `DEBUG`, each tool call also logs a `tool call finished` line with `agent_request_id`, `tool`, `duration` and `is_error`; it holds no tool input and no result.
Set `SLASHOBSERVE_AGENT_LOG_TOOL_INPUTS=true` to log an `INFO` `agent tool input` line per tool call with `input_sha256` (12 hex characters) and `input_bytes`, plus `panel_ids` and `panel_count` for a save and `issue_paths` for a rejected one, never the input's text; the server logs one `WARN` at startup while it is on, and the evals stack sets it.

## Provider error messages

Set `SLASHOBSERVE_LLM_LOG_PROVIDER_ERRORS=true` to add the provider's own error message to a failed `llm call finished` line as `provider_error_message`.
The message has the API key removed, newlines and other control characters replaced by spaces, and is cut to 1024 bytes.
It is never added to the error stored on an agent request or returned by the API.
A provider can echo parts of the prompt in its message, and the server logs one `WARN` at startup while the setting is on, so use it for evals and debugging only.
