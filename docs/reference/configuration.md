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
| `SLASHOBSERVE_ADMIN_EMAILS`               | unset   | Optional. Comma-separated email addresses that are always Admins, at most 100. See [Roles](../teams/roles.md).                              |
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
| `SLASHOBSERVE_LLM_EFFORT`          | `medium`                            | `low`, `medium`, `high`, `xhigh` or `max`. Used by `anthropic` only; the value is checked whatever the provider.            |
| `SLASHOBSERVE_LLM_MAX_TOKENS`      | `16000`                             | Output token cap for each model call, `1024` to `64000`.                                                                    |
| `SLASHOBSERVE_LLM_REQUEST_TIMEOUT` | `120s`                              | A Go duration from `10s` to `10m`, applied to each model call.                                                              |
| `SLASHOBSERVE_LLM_REPLAY_DIR`      | unset                               | Required for `replay`: a folder of recorded model scripts. This provider exists for end-to-end tests, not for real use.     |

## Testing only

`SLASHOBSERVE_API_CONTRACT_CHECK` (`true` or `false`, default `false`) makes the server validate every `/api/v1` request and response against the OpenAPI document.
It is for the end-to-end test stack only; never set it in production.
