---
title: Logging
description: Set the log level and format, find the cause of a failed request, run or startup in the logs, and see what the server never logs.
---

The server writes JSON logs to standard error by default.
Use this page to set the level, to follow a failure from a line to its cause, and to know what a log line never holds.

The server does not log secrets, prompt text, query text or tool results.
It never logs the database URL, the OIDC client secret or the LLM key.

## Set the log level and format

Set `SLASHOBSERVE_LOG_LEVEL` to `debug`, `info`, `warn` or `error`; the default is `info`.
Set `SLASHOBSERVE_LOG_FORMAT` to `json` or `text`; the default is `json`.
On Kubernetes, the chart values `logLevel` and `logFormat` set them.

```sh
SLASHOBSERVE_LOG_LEVEL=debug SLASHOBSERVE_LOG_FORMAT=text slashobserve-server
```

The built-in agent runs in a child process, `observe-agent`, and logs at the same level.
The server copies the child's log lines into its own log with `component=observe-agent`.
When the child stops it logs `observe-agent stopped` with `shutdown` set to `drained`, or to `forced` when it did not finish within 5 seconds of `SIGTERM`; the server's `observe-agent exited` line then has `exit_code` 0 or 124.

## Find out why a request failed

Every line about a failure carries a short fixed `error_code` and the ids that locate it.
The server logs no error message from a driver or library, because it can echo a query, a connection detail or a credential.

1. Find the line for the failure and read its `error_code`, such as `tool_failed`, `session_lookup_failed` or `readiness_check_failed`.
2. Read the ids on the line, such as `agent_request_id`, `dashboard_id`, `principal_id` or a datasource `uid`.
3. For an agent request, look it up with `observe status <id>` or `GET /api/v1/observe/{id}`.
4. For an HTTP request, take the `request_id` and match it to the response's `X-Request-Id` header and to the audit event the request records.
   One id joins all three.

Details of the other fields on an error line:

- `error_type` is the Go type of the innermost error.
- `sqlstate` is set when the cause is a Postgres error.
- `errno` is the operating system's text, such as `connection refused`, when the cause is a system error.

## Find out why the server did not start

A fatal error at startup logs `server exiting` with an `error_code`.
The line keeps a message the server wrote itself from your settings, for example `SLASHOBSERVE_SESSION_TTL: must be between 5m and 720h`, and the listen address errors from the operating system.
It never echoes a setting that is a URL.

A failure to reach Postgres, apply migrations, build a datasource or set up sign-in is logged by code and error type, so the log never shows the connection string, the user or the database name.
The line adds only what the server knows from your settings or from the operating system.

| `error_code` | What the line adds |
| --- | --- |
| `database_unavailable` | `db_host` and `db_port` from `SLASHOBSERVE_DATABASE_URL`, `errno` for a network failure such as `connection refused`, and `sqlstate` for a Postgres error. |
| `invalid_database_url` | Nothing. The setting could not be parsed. |
| `migration_failed` | `migration_version` and `sqlstate`. |
| `datasources_failed` | `datasource_uid`, and for a validation failure the server wrote itself, such as a limit or URL scheme problem, a `detail` message. |

For anything else, test the connection yourself:

```sh
psql "$SLASHOBSERVE_DATABASE_URL"
```

## Find out why an agent run failed

The server logs one line when an agent request ends:

- `agent request succeeded` at `INFO`, with `agent_request_id`, `dashboard_id` when the request has a dashboard, `version` when it saved one, and `attempt`.
- `agent request failed` at `WARN`, with the same ids and `error_code`, the failure code the request shows, such as `rate_limited`, `run_timeout`, `budget_exhausted` or `lease_expired`.

The agent logs `run started` (`run_id`, `attempt`, `kind`, `mode`) and `run finished` at `INFO`.
`run finished` carries `model_calls`, `retries`, `compactions`, `renewals` and `model_ms`.
It logs `run failed` at `WARN` with the same counters and its `failure_code`.
No line holds a tool input or result.

A failed call to the model provider logs one `llm call finished` line at `WARN`.
A call that succeeded logs nothing.
The line carries these fields:

- `run_id`, the id of the agent request.
- `provider`.
- `status`, the provider's last HTTP status, or 0 when the last attempt got no answer.
- `failure_code`, one of `call_timeout`, `rate_limited` or `provider_error`.
- `idle_timeout`, when the call stalled.
- When the provider sent them, `provider_error_type`, `provider_error_code`, `provider_error_param`, `request_id` and `retry_after`.

The server logs these values only when they are plain tokens.
The line never holds the prompt, the model's reply, a tool input or result, or the provider's error message, unless you turn the message on.

## Provider error messages

Set `SLASHOBSERVE_LLM_LOG_PROVIDER_ERRORS=true` to add the provider's own error message to a failed `llm call finished` line as `provider_error_message`.
Use it for debugging only.
A provider can echo parts of the prompt in its message, and the server logs one `WARN` at startup while the setting is on.

The message has the API key removed, newlines and other control characters replaced by spaces, and is cut to 1024 bytes.
It is never added to the error stored on an agent request or returned by the API.

## Reference

### Log levels

| Level | Meaning |
| --- | --- |
| `DEBUG` | Detail for each step, such as the health checks, scrapes of `/metrics` and the agent's `run brief loaded`. |
| `INFO` | Normal lifecycle: startup, `observe-agent started`, an agent request that saved, a run that started, finished or was ended by the server. |
| `WARN` | Degraded or recoverable: a failed model call, an agent request or run that failed, a datasource that turned unhealthy. |
| `ERROR` | A failure you must act on: the database, a migration, a run result the server could not record, or `run loop crashed` in the agent. |

### Request lines

A request log line records the method, the path, the status, the response size in `bytes` and the duration.
It also carries the `route` template.
For `/api/v1`, `/mcp` and the OAuth routes it carries the `request_id` and, once the caller is signed in, the `principal` (`user:<id>` or `service_account:<id>`).

The path never includes the query string, and the token of a public link is replaced by `:token`.

### Other lines

- A panic is logged by its Go type (`panic_type`), never its value.
- A datasource that turns unhealthy logs `datasource unhealthy` once, with its `uid` and `type`.
  It does not log again until the datasource has recovered.
- A refused sign-in also logs the domain of the email address.
  A failed code exchange logs the provider's `oauth_error` code, such as `invalid_grant`, when it is a plain code.
- A line of the `observe-agent` child that is not JSON is logged as `observe-agent output` with the text in `line`.
  A line longer than 4096 bytes is cut and marked `truncated=true`.
