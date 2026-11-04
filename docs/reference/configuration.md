---
title: Configuration
description: Every `SLASHOBSERVE_*` environment variable with its default and notes.
---

Every setting is an environment variable named `SLASHOBSERVE_*`.
The server reads them once at startup, so changing one means restarting the server.
Values are trimmed of surrounding whitespace, and an empty value counts as unset.

## Server and logging

| Variable                    | Default | Notes                                                                                                                              |
| --------------------------- | ------- | ---------------------------------------------------------------------------------------------------------------------------------- |
| `SLASHOBSERVE_HTTP_ADDR`    | `:8080` | Listen address as `host:port`; the port must be 1 to 65535. The image sets it to `:8080`.                                          |
| `SLASHOBSERVE_DATABASE_URL` | none    | Required. A Postgres connection string (a URL or key/value form, parsed by pgx). Add `pool_max_conns=N` to size the pool above 16. |
| `SLASHOBSERVE_LOG_LEVEL`    | `info`  | `debug`, `info`, `warn` or `error`. `/healthz` and `/readyz` requests are logged at `debug` only.                                  |
| `SLASHOBSERVE_LOG_FORMAT`   | `json`  | `json` or `text`. Logs go to standard error.                                                                                       |

## Sign-in

| Variable                                  | Default | Notes                                                                                                                            |
| ----------------------------------------- | ------- | -------------------------------------------------------------------------------------------------------------------------------- |
| `SLASHOBSERVE_PUBLIC_URL`                 | none    | Required. An `http` or `https` origin with no path, query or user info. See [Put TLS in front](../operate/production.md#put-tls-in-front).               |
| `SLASHOBSERVE_OIDC_ISSUER_URL`            | none    | Required. An absolute `http` or `https` URL, used exactly as written because the identity provider's issuer is compared exactly. |
| `SLASHOBSERVE_OIDC_INTERNAL_URL`          | unset   | Optional origin (no path) that the server uses instead of the issuer's scheme and host when it calls the identity provider.      |
| `SLASHOBSERVE_OIDC_CLIENT_ID`             | none    | Required.                                                                                                                        |
| `SLASHOBSERVE_OIDC_CLIENT_SECRET`         | none    | Required. Never logged.                                                                                                          |
| `SLASHOBSERVE_SESSION_TTL`                | `168h`  | A Go duration from `5m` to `720h`. A session lasts this long from sign-in.                                                       |
| `SLASHOBSERVE_AUTH_ALLOWED_EMAIL_DOMAINS` | none    | Required. Comma-separated domains, or `*` for every account. See [Sign-in with OIDC](../connect/oidc.md).                        |
| `SLASHOBSERVE_AUTH_PROVIDER_NAME`         | unset   | Optional. The identity provider's display name, up to 64 characters, shown to signed-in clients next to the sign-in rules.       |
| `SLASHOBSERVE_ADMIN_EMAILS`               | unset   | Optional. Comma-separated email addresses, compared without regard to case, that are always Admins, at most 100. Unset, the earliest person to sign in is the Admin. See [Roles](../teams/roles.md). |
| `SLASHOBSERVE_OIDC_GROUP_ROLES`           | unset   | Optional. Comma-separated `group=role` entries that map identity-provider groups to roles, at most 100. See [Roles](../teams/roles.md).      |

## Datasources

| Variable                        | Default | Notes                                                                          |
| ------------------------------- | ------- | ------------------------------------------------------------------------------ |
| `SLASHOBSERVE_DATASOURCES_FILE` | unset   | Path to the [datasources file](datasources-file.md). Unset means no datasources. |

## Outbound access

| Variable                        | Default | Notes                                                                                                                                                                    |
| ------------------------------- | ------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `SLASHOBSERVE_OUTBOUND_DOMAINS` | unset   | Unset: Admins manage the allowed domains in Settings. `none`, or a comma-separated list of domains, locks the list. See [Outbound access](../operate/outbound-access.md). |

## LLM

All LLM settings are optional.
See [LLM providers](../connect/llm-providers.md) for how they combine.

| Variable                           | Default                             | Notes                                                                                                                       |
| ---------------------------------- | ----------------------------------- | --------------------------------------------------------------------------------------------------------------------------- |
| `SLASHOBSERVE_LLM_PROVIDER`        | unset                               | `anthropic`, `openai` or `replay` (not case-sensitive). Unset turns the agent off.                                          |
| `SLASHOBSERVE_LLM_MODEL`           | `claude-sonnet-5-5` for `anthropic` | Required for `openai`. Ignored by `replay`.                                                                                 |
| `SLASHOBSERVE_LLM_API_KEY`         | unset                               | The provider credential. Required for `anthropic`, optional for `openai`. Never logged or echoed in an error.               |
| `SLASHOBSERVE_LLM_API_KEY_FILE`    | unset                               | A file holding the credential, with surrounding whitespace trimmed. Set at most one of this and `SLASHOBSERVE_LLM_API_KEY`. |
| `SLASHOBSERVE_LLM_BASE_URL`        | the provider's own endpoint         | An absolute `http` or `https` URL. A path is allowed; a query, fragment or user info is not.                                |
| `SLASHOBSERVE_LLM_EFFORT`          | unset                               | For `anthropic`: `low`, `medium`, `high`, `xhigh` or `max`; unset means `medium`. For `openai`: `none`, `minimal`, `low`, `medium`, `high` or `xhigh`; unset sends nothing to the provider. The value is checked against the provider's list and ignored by `replay`. The first call of a new build runs one step higher (`low` becomes `medium`, `medium` becomes `high`, anything else stays); every later call runs at this value. |
| `SLASHOBSERVE_LLM_MAX_TOKENS`      | `16000`                             | Output token cap for a model call, `1024` to `64000`. The first call of a new build uses it as is; later calls cap their output at half of it, but at least `8192` and never above it, and a call cut off at that lower cap is sent once more at this value. |
| `SLASHOBSERVE_LLM_REQUEST_TIMEOUT` | `10m`                               | A Go duration from `10s` to `60m`, applied to each model call including its retries (up to 4 attempts with backoff). `0` sets no limit on a call. A call that hits the limit fails with `call_timeout`. |
| `SLASHOBSERVE_LLM_IDLE_TIMEOUT`    | `180s`                              | A Go duration from `10s` to `60m`. Model calls stream, and an attempt that receives no bytes from the provider for this long, from its start or since the last byte, is retried with backoff within the request timeout. `0` turns the check off. |
| `SLASHOBSERVE_AGENT_RUN_TIMEOUT`   | `0`                                 | `0` is no limit: a run ends at the agent's own limits, a cancel or a shutdown. Set a Go duration of at least `1m` to cap each run of an agent request; a run that hits the cap fails with `run_timeout`. |
| `SLASHOBSERVE_AGENT_SESSION_RENEW_KIB` | `256` | A whole number of KiB from `32` to `4096`. After a save to a build, the agent shortens its conversation to its last few turns, the build and the names it discovered once the session's tool calls, results and text pass this size, so a long build stays inside the model's context window. Lower it for a model with a small context window. |
| `SLASHOBSERVE_LLM_REPLAY_DIR`      | unset                               | Required for `replay`: a folder of recorded model scripts. This provider exists for end-to-end tests, not for real use.     |
| `SLASHOBSERVE_LLM_LOG_PROVIDER_ERRORS` | `false` | `true` or `false`. `true` adds the provider's own error message to the `llm call finished` line of a failed call, as `provider_error_message`, with the API key removed, control characters replaced and at most 1024 bytes kept. A provider can echo parts of the prompt back in that message, so use it for evals and debugging, not on a server whose logs are widely readable. See [Logging](../operate/logging.md). |

## Testing only

`SLASHOBSERVE_API_CONTRACT_CHECK` (`true` or `false`, default `false`) makes the server validate every `/api/v1` request and response against the OpenAPI document.
It is for the end-to-end test stack only; never set it in production.
