# Self-hosting SlashObserve

This guide covers running the Community Edition yourself: what to run, how to configure it, how people and tools sign in, and how to upgrade and back it up.

## What you run

SlashObserve is one container plus one Postgres database.
The container is a single Go binary with the web UI embedded; it has no other runtime dependencies.
Postgres holds everything the server owns: dashboards and their versions, comments, users and sessions, service accounts and key hashes, and the agent request queue.
Your Prometheus and ClickHouse servers stay where they are; SlashObserve only reads from them.

You also need:

- An OpenID Connect (OIDC) identity provider for sign-in.
  The demo stack uses Dex, a development-only provider.
- Optionally, an LLM provider.
  Without one the server runs, but the built-in agent is off (see [LLM providers](#llm-providers)).

This repository does not reference a published container image.
Build the image from the repository root (see [Run your own deployment](#run-your-own-deployment)).

## Quick start

Requirements: Docker with Compose v2.20 or newer, and `make`.

Run the demo stack with an Anthropic key so the agent can answer prompts:

    export SLASHOBSERVE_LLM_PROVIDER=anthropic
    export SLASHOBSERVE_LLM_API_KEY=<your Anthropic API key>
    make demo

Open http://localhost:8080 and sign in as `admin@example.com` / `password`.
Type a prompt such as `checkout health by pod, last 6h` into the prompt bar on the home page.

`make demo` builds the server image and starts it with Postgres, Dex, Prometheus, ClickHouse, an OpenTelemetry Collector, node-exporter, cAdvisor and a demo shop, and waits until they are healthy.
It passes `SLASHOBSERVE_LLM_PROVIDER`, `SLASHOBSERVE_LLM_MODEL`, `SLASHOBSERVE_LLM_API_KEY` and `SLASHOBSERVE_LLM_BASE_URL` from your shell to the server container; the other LLM settings are not forwarded.
Run `make demo` again after changing them.
Without a provider the demo still starts, but prompts get `503 agent_unavailable`.
Stop it with `make demo-down`, or drop its data as well with `make demo-reset`.

The demo is for trying SlashObserve, not for production.
Its Postgres password, Dex logins and OIDC client secret are fixed development values.

## Run your own deployment

1. Create a Postgres database and a user that owns it.
   The server keeps a connection pool of at least 16 connections, so `max_connections` must allow that for each server process.
   The project tests against Postgres 18.6; other versions are unverified.
2. Build the image from the repository root:

       docker build -t slashobserve-server:local .

3. Write the settings to an env file, for example `slashobserve.env`.
   Docker's `--env-file` takes `NAME=value` lines with no quoting.

       SLASHOBSERVE_DATABASE_URL=postgres://slashobserve:<password>@<postgres-host>:5432/slashobserve?sslmode=require
       SLASHOBSERVE_PUBLIC_URL=https://observe.example.com
       SLASHOBSERVE_OIDC_ISSUER_URL=https://idp.example.com
       SLASHOBSERVE_OIDC_CLIENT_ID=slashobserve
       SLASHOBSERVE_OIDC_CLIENT_SECRET=<client secret>
       SLASHOBSERVE_AUTH_ALLOWED_EMAIL_DOMAINS=example.com
       SLASHOBSERVE_DATASOURCES_FILE=/etc/slashobserve/datasources.yaml
       SLASHOBSERVE_LLM_PROVIDER=anthropic
       SLASHOBSERVE_LLM_API_KEY=<Anthropic API key>

4. Write a [datasources file](#datasources-file), then start the container:

       docker run -d --name slashobserve --restart unless-stopped \
         -p 127.0.0.1:8080:8080 \
         --env-file slashobserve.env \
         -v "$PWD/datasources.yaml:/etc/slashobserve/datasources.yaml:ro" \
         slashobserve-server:local

5. Check that it is ready:

       curl -fsS http://localhost:8080/readyz

The server applies database migrations when it starts, so an empty database is enough.
If it cannot connect to Postgres within 30 seconds, or any setting is invalid, it logs the problem and exits; `--restart unless-stopped` retries.
Every problem found in the settings is reported in one message.

The image runs as a non-root user and listens on `:8080`.
Files you mount, such as the datasources file or a secret file, must be readable by that user.
The image has a Docker health check that calls `slashobserve-server healthcheck`, which requests `/readyz` on the configured `SLASHOBSERVE_HTTP_ADDR`.

### Put TLS in front

The server speaks plain HTTP; it has no TLS settings.
Terminate TLS in a reverse proxy or load balancer and set `SLASHOBSERVE_PUBLIC_URL` to the address people use in the browser, scheme and host included.

That address matters beyond links:

- It is the OIDC redirect URI base.
  Register `<public url>/api/v1/auth/callback` with your identity provider.
- With an `https` address, session cookies are marked `Secure` and use the `__Host-` prefix.
  With `http` they are not, so use `http` only for local development.
- It is the one origin the MCP endpoint accepts in an `Origin` header.
- It is the trusted origin for the cross-origin check on cookie-authenticated writes.

Do not buffer or time out `GET /api/v1/dashboards/{id}/events`.
It is a Server-Sent Events stream that sends a ping every 15 seconds, ends at the first ping after its session or key stops being valid, and ends after 30 minutes; browsers reconnect on their own.
The server ignores `X-Forwarded-For`.

### Health endpoints

| Endpoint       | Meaning                                                                                                |
| -------------- | ------------------------------------------------------------------------------------------------------ |
| `GET /healthz` | Liveness. It checks nothing else, so use it to decide whether to restart.                              |
| `GET /readyz`  | Readiness. It pings Postgres and confirms no migration is pending; it returns 200 only when both pass. |

Datasources never affect `/readyz`.
An unreachable Prometheus shows up as a datasource health problem, not as a server that is down.

## Configuration

Every setting is an environment variable named `SLASHOBSERVE_*`.
The server reads them once at startup, so changing one means restarting the server.
Values are trimmed of surrounding whitespace, and an empty value counts as unset.

### Server and logging

| Variable                    | Default | Notes                                                                                                                              |
| --------------------------- | ------- | ---------------------------------------------------------------------------------------------------------------------------------- |
| `SLASHOBSERVE_HTTP_ADDR`    | `:8080` | Listen address as `host:port`; the port must be 1 to 65535. The image sets it to `:8080`.                                          |
| `SLASHOBSERVE_DATABASE_URL` | none    | Required. A Postgres connection string (a URL or key/value form, parsed by pgx). Add `pool_max_conns=N` to size the pool above 16. |
| `SLASHOBSERVE_LOG_LEVEL`    | `info`  | `debug`, `info`, `warn` or `error`. `/healthz` and `/readyz` requests are logged at `debug` only.                                  |
| `SLASHOBSERVE_LOG_FORMAT`   | `json`  | `json` or `text`. Logs go to standard error.                                                                                       |

### Sign-in

| Variable                                  | Default | Notes                                                                                                                            |
| ----------------------------------------- | ------- | -------------------------------------------------------------------------------------------------------------------------------- |
| `SLASHOBSERVE_PUBLIC_URL`                 | none    | Required. An `http` or `https` origin with no path, query or user info. See [Put TLS in front](#put-tls-in-front).               |
| `SLASHOBSERVE_OIDC_ISSUER_URL`            | none    | Required. An absolute `http` or `https` URL, used exactly as written because the identity provider's issuer is compared exactly. |
| `SLASHOBSERVE_OIDC_INTERNAL_URL`          | unset   | Optional origin (no path) that the server uses instead of the issuer's scheme and host when it calls the identity provider.      |
| `SLASHOBSERVE_OIDC_CLIENT_ID`             | none    | Required.                                                                                                                        |
| `SLASHOBSERVE_OIDC_CLIENT_SECRET`         | none    | Required. Never logged.                                                                                                          |
| `SLASHOBSERVE_SESSION_TTL`                | `168h`  | A Go duration from `5m` to `720h`. A session lasts this long from sign-in.                                                       |
| `SLASHOBSERVE_AUTH_ALLOWED_EMAIL_DOMAINS` | none    | Required. Comma-separated domains, or `*` for every account. See [Sign-in with OIDC](#sign-in-with-oidc).                        |
| `SLASHOBSERVE_AUTH_PROVIDER_NAME`         | unset   | Optional. The identity provider's display name, up to 64 characters, shown to signed-in clients next to the sign-in rules.       |

### Datasources

| Variable                        | Default | Notes                                                                          |
| ------------------------------- | ------- | ------------------------------------------------------------------------------ |
| `SLASHOBSERVE_DATASOURCES_FILE` | unset   | Path to the [datasources file](#datasources-file). Unset means no datasources. |

### LLM

All LLM settings are optional.
See [LLM providers](#llm-providers) for how they combine.

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

### Testing only

`SLASHOBSERVE_API_CONTRACT_CHECK` (`true` or `false`, default `false`) makes the server validate every `/api/v1` request and response against the OpenAPI document.
It is for the end-to-end test stack only; never set it in production.

## Datasources file

The datasources file lists the Prometheus and ClickHouse servers SlashObserve queries.
It is a single YAML document with a top-level `datasources` list; unknown fields are rejected, and an empty list logs a warning at startup.
The server reads it once at startup.

The file never holds a secret.
Passwords and tokens are referenced by environment variable name or by file path instead, so the file can live in version control.

    datasources:
      - uid: prom
        type: prometheus
        name: Prometheus
        prometheus:
          url: http://prometheus:9090
          scrapeInterval: 15s
        limits:
          timeout: 30s
          maxSeries: 1000
      - uid: ch
        type: clickhouse
        name: ClickHouse
        clickhouse:
          addr: clickhouse:9000
          database: otel
          username: slashobserve_ro
          passwordEnv: SLASHOBSERVE_CLICKHOUSE_PASSWORD

`deploy/compose/datasources.yaml` is the minimal exemplar used by the demo.

A Prometheus behind a bearer token references the token by variable name, and the variable goes in the same env file as the other settings:

    prometheus:
      url: https://prometheus.example.com
      bearerTokenEnv: SLASHOBSERVE_PROMETHEUS_TOKEN

Fields common to every entry:

| Field    | Notes                                                                                                                            |
| -------- | -------------------------------------------------------------------------------------------------------------------------------- |
| `uid`    | Required, unique. Up to 40 letters, digits, `_` or `-`, starting with a letter or digit. Dashboards refer to a datasource by it. |
| `type`   | Required: `prometheus` or `clickhouse`. Give only the block that matches (`prometheus:` or `clickhouse:`).                       |
| `name`   | A display name.                                                                                                                  |
| `limits` | Optional: `timeout` (a Go duration), `maxSeries`, `maxPoints` and `maxRows` (whole numbers, not negative). See below.            |

Prometheus fields:

| Field                               | Notes                                                                                                   |
| ----------------------------------- | ------------------------------------------------------------------------------------------------------- |
| `url`                               | Required. `http` or `https`, with a host, and no user info, query or fragment.                          |
| `scrapeInterval`                    | A Go duration, default `15s`. Feeds the `$__rate_interval` macro and the smallest query step.           |
| `headers`                           | Plain request headers. Not for secrets.                                                                 |
| `headersFromEnv`, `headersFromFile` | Maps of header name to an environment variable name or a file path, for header values that are secrets. |
| `basicAuth`                         | `username` plus one of `passwordEnv` or `passwordFile`. The password is required.                       |
| `bearerTokenEnv`, `bearerTokenFile` | A bearer token from an environment variable or a file. Not combinable with `basicAuth`.                 |

ClickHouse fields:

| Field                         | Notes                                                           |
| ----------------------------- | --------------------------------------------------------------- |
| `addr`                        | Required. The native-protocol `host:port`.                      |
| `database`                    | Default `otel`.                                                 |
| `username`                    | Required.                                                       |
| `passwordEnv`, `passwordFile` | Exactly one is required.                                        |
| `tls`                         | `true` to connect over TLS (TLS 1.2 or newer). Default `false`. |

Rules for secrets:

- An environment variable name must start with `SLASHOBSERVE_`, must be set and non-empty, and must not be one of the server's own settings (such as `SLASHOBSERVE_DATABASE_URL`).
- A file's content is used with trailing newlines removed; an empty file is an error.
- Set only one of the env and file forms for the same secret.
- A header name may appear in only one of `headers`, `headersFromEnv` and `headersFromFile`.
  Prometheus rejects an `Authorization` header there; use `basicAuth` or a bearer token instead.

Limits bound every query the server runs for that datasource.
The defaults are a 30 second timeout, 1000 series, 1,000,000 points and 10,000 rows.
A value you set replaces the default, up or down; a ClickHouse settings profile can still cap it further.
The agent's and MCP's ad-hoc queries use tighter limits still (15 seconds, 200 series, 100,000 points and 1,000 rows), or the datasource's limits when those are lower.

### ClickHouse needs a read-only user

SlashObserve queries ClickHouse only as a user with `readonly = 2`, and it refuses a user that is not read-only.
`readonly = 2` rather than `1` is required because the server sets per-query limits.
The demo defines such a user with a settings profile; use `internal/datasource/clickhouse/testdata/readonly_user.sql` as the template, replacing `__PASSWORD__`:

    CREATE SETTINGS PROFILE IF NOT EXISTS slashobserve_ro_profile SETTINGS
        readonly = 2 CONST,
        max_execution_time = 30 MIN 1 MAX 300,
        max_result_rows = 10000 MIN 1 MAX 1000000,
        max_result_bytes = 104857600 MIN 1 MAX 1073741824,
        result_overflow_mode = 'throw' CONST;

    CREATE USER IF NOT EXISTS slashobserve_ro IDENTIFIED WITH sha256_password BY '__PASSWORD__'
        SETTINGS PROFILE 'slashobserve_ro_profile';

    GRANT SELECT ON otel.* TO slashobserve_ro;

Grant `SELECT` only on the database that holds your OpenTelemetry tables.

## Sign-in with OIDC

People sign in through an OIDC identity provider; there are no local passwords.
Create a client in your provider and give SlashObserve its settings:

- Register the redirect URI `<SLASHOBSERVE_PUBLIC_URL>/api/v1/auth/callback`.
- Use the authorization code flow.
  The server sends PKCE (S256) and a nonce, and asks for the `openid`, `email` and `profile` scopes.
- Set `SLASHOBSERVE_OIDC_ISSUER_URL` to the issuer exactly as the provider reports it in its discovery document.
  The server fetches `<issuer>/.well-known/openid-configuration` on the first sign-in attempt, not at startup, so a provider that is down when the server boots does not stop it.
- If the server reaches the provider at a different address than browsers do (for example by a Docker service name), set `SLASHOBSERVE_OIDC_INTERNAL_URL` to that origin.
  Only the server's own calls to the provider are rewritten; the issuer and the browser redirects keep the public address.
  The demo stack does this: browsers use `http://localhost:5556/dex` and the server uses `http://dex:5556`.

`SLASHOBSERVE_AUTH_ALLOWED_EMAIL_DOMAINS` decides who gets in:

- A comma-separated list such as `example.com,example.org`.
  A person may sign in when the provider says their email is verified and its domain is in the list; the match is exact, so `example.com` does not admit `sub.example.com`.
- `*` allows every account the provider authenticates that has an email address, verified or not, and logs a warning at startup.
  Use it only when the provider itself is the access control, for example a private tenant.
- The setting is required, so a deployment can never be open by accident.

Users are identified by the provider's issuer and subject, and are created on first sign-in.

## LLM providers

The agent turns a prompt into a saved dashboard by calling tools against your datasources.
It is on only when `SLASHOBSERVE_LLM_PROVIDER` is set.
With it unset the server starts normally, and:

- `POST /api/v1/observe` and revision prompts answer `503 agent_unavailable`.
- Comments are saved but do not queue a revision, and turning the agent on later does not replay old comments.
- You can still create and edit dashboards through the API or through [MCP](#mcp) with an agent of your own.

The agent sends what its tools return to the provider: datasource and metric names, label values, sampled rows and query results.
Choose a provider whose data handling fits what your datasources contain.

### Anthropic

Anthropic is the default model family.
Only the key is required:

    SLASHOBSERVE_LLM_PROVIDER=anthropic
    SLASHOBSERVE_LLM_API_KEY=<your Anthropic API key>

The default model is `claude-sonnet-5-5`.
Set `SLASHOBSERVE_LLM_MODEL` to use another, for example `claude-opus-5-5` for harder prompts or `claude-haiku-5-5` for lower cost.
The server does not validate the model name itself.
`SLASHOBSERVE_LLM_EFFORT` sets the model's effort level and applies to this provider only.
To read the key from a file, such as a Docker or Kubernetes secret, use `SLASHOBSERVE_LLM_API_KEY_FILE` instead.

### OpenAI-compatible

`openai` talks to any server that implements the Chat Completions API with tool calling.
The model is required, because there is no safe default:

    SLASHOBSERVE_LLM_PROVIDER=openai
    SLASHOBSERVE_LLM_MODEL=<model name>
    SLASHOBSERVE_LLM_API_KEY=<API key>

Leave `SLASHOBSERVE_LLM_BASE_URL` unset to use OpenAI.
Point it at another server to use that instead.
For a local Ollama:

    SLASHOBSERVE_LLM_PROVIDER=openai
    SLASHOBSERVE_LLM_MODEL=llama3.1
    SLASHOBSERVE_LLM_BASE_URL=http://localhost:11434/v1

The key is optional for servers that need none, as Ollama does.
The model must support tool calling.
Inside a container, `localhost` is the container itself; use an address the container can reach, such as `http://host.docker.internal:11434/v1` on Docker Desktop.
The project does not vouch for any particular local model, and dashboard quality depends heavily on the model.

## Service-account keys

The CLI, MCP clients and scripts authenticate with a service-account key, not a browser session.

1. Sign in to the web app and choose **API keys** in the sidebar, or open `<public url>/keys` (the old `/settings/keys` address redirects).
2. Choose **New service account**, give it a name (lowercase letters, digits and dashes, up to 63 characters, for example `ci-bot`), then give the first key a name and an expiry.
   The expiry choices are 30 days, 90 days (the default), 1 year or no expiry.
3. Copy the key.
   It is shown once, in that dialog, and the server stores only a hash of it, so a lost key cannot be recovered; create a new one and revoke the old one.

Keys start with `sok_`.
Add more keys to an account, and revoke a key, from the same page.
A revoked or expired key is refused immediately.
The page also shows ready-made client setup snippets for your server's address; they refer to `OBSERVE_TOKEN` and never contain the key.

Only a signed-in person can create service accounts and keys; a service-account key cannot create more keys.
Beyond that, see [the security model](#security-model) for what any key can do.

## CLI

Build the `observe` command, which needs only Go:

    make cli

It writes `bin/observe`.
Sign in once; it prompts for the key without echoing it:

    bin/observe login --server https://observe.example.com

To pass the key on standard input instead, for scripts:

    printf '%s' "$OBSERVE_TOKEN" | bin/observe login --server https://observe.example.com --with-token

`login` checks the key with the server and then saves the server and key to `~/.config/slashobserve/cli.json` (under `$XDG_CONFIG_HOME` when that is an absolute path), readable by you only.
Then give it a prompt:

    bin/observe "checkout p99 latency, last 6h"

It prints progress and then the dashboard address.
`bin/observe dashboards list` lists dashboards, and `bin/observe --help` shows the rest.

The CLI reads its settings in this order: flags, the environment, then the saved file.

| Setting | Flag       | Environment variable |
| ------- | ---------- | -------------------- |
| Server  | `--server` | `OBSERVE_SERVER`     |
| Key     | `--token`  | `OBSERVE_TOKEN`      |

A saved key is sent only to the server it was saved for.
To use another server, pass `--token` or set `OBSERVE_TOKEN`.
Prefer `OBSERVE_TOKEN` to `--token`, since flags end up in shell history and the process list.
Add `--json` to any command to get the API response as it is.

## MCP

The server exposes its tools to MCP clients over Streamable HTTP at `<public url>/mcp`, so Claude Code, OpenCode, Cursor and other clients can build and edit dashboards with their own model.
`GET /api/v1/instance` returns the exact address as `mcp_url` next to `public_url`; it needs a signed-in session or a key.

Requests must carry `Authorization: Bearer <key>` with a [service-account key](#service-account-keys).
Browser sessions are not accepted on `/mcp`.
The endpoint is stateless, refuses an `Origin` that is not `SLASHOBSERVE_PUBLIC_URL`, answers `413` above 8 MiB, `429` above 8 concurrent requests per key and `503` above 32 across all keys.

Export the key in the shell that starts your client, so the key never lands in a config file:

    export OBSERVE_TOKEN=<your service-account key>

### Claude Code

Add this to `.mcp.json` in your project, with your server's address:

    {
      "mcpServers": {
        "slashobserve": {
          "type": "http",
          "url": "https://observe.example.com/mcp",
          "headers": { "Authorization": "Bearer ${OBSERVE_TOKEN}" }
        }
      }
    }

Claude Code expands `${OBSERVE_TOKEN}` from its environment when it starts.
`.mcp.json` is the documented place for that expansion.

`claude mcp add` can create the same entry.
Use single quotes so your shell does not expand the variable and write the key into the config:

    claude mcp add --transport http slashobserve https://observe.example.com/mcp --header 'Authorization: Bearer ${OBSERVE_TOKEN}'

Prefer `.mcp.json`.

### OpenCode

Add this to `opencode.json`:

    {
      "$schema": "https://opencode.ai/config.json",
      "mcp": {
        "slashobserve": {
          "type": "remote",
          "url": "https://observe.example.com/mcp",
          "enabled": true,
          "oauth": false,
          "headers": { "Authorization": "Bearer {env:OBSERVE_TOKEN}" }
        }
      }
    }

`"oauth": false` stops OpenCode from trying an OAuth flow; SlashObserve takes only the bearer key.

### Cursor

Cursor reads remote MCP servers from `.cursor/mcp.json` (per project) or `~/.cursor/mcp.json`.
The following is a generic example that this project has not tested with Cursor.
The `url` and `headers` fields apply to any client that supports remote MCP servers with custom headers.
How Cursor expands environment variables inside `headers` is defined by Cursor, so check its current MCP documentation before relying on the `${env:OBSERVE_TOKEN}` form below:

    {
      "mcpServers": {
        "slashobserve": {
          "url": "https://observe.example.com/mcp",
          "headers": { "Authorization": "Bearer ${env:OBSERVE_TOKEN}" }
        }
      }
    }

If your Cursor version cannot expand variables there, put the key in the file only for a user-level `~/.cursor/mcp.json` that is never committed.

## Upgrades and backups

### Back up Postgres

Postgres is the only state.
The datasources file, the OIDC settings and the LLM key are configuration you keep elsewhere; keep them in your own secret and config management.

Take a dump before every upgrade, and on a schedule:

    pg_dump --format=custom --file slashobserve.dump --dbname "<the SLASHOBSERVE_DATABASE_URL value>"

Restore into an empty database with `pg_restore --dbname "<database URL>" slashobserve.dump`.
For the demo stack, which runs Postgres in Compose:

    docker compose -f deploy/compose/compose.yaml exec -T postgres pg_dump -U slashobserve slashobserve > slashobserve.sql

If your connection string carries pgx-only parameters such as `pool_max_conns`, remove them first, because `pg_dump` does not understand them.

Dumps contain session and key hashes and your dashboards, so protect them like the database itself.

### Upgrade

1. Take a backup.
2. Build or pull the new image and replace the container.
3. Watch the log: the server applies pending migrations at startup, each logged as `applied migration` with its version, and `/readyz` returns 200 once they are done.

Migrations run under a Postgres lock, so starting several servers at once is safe: one migrates and the others wait.
The milestone 1 release changes the dashboard format: migration 10 rewrites every stored dashboard version from the earlier ObserveSpec v0 shape to evolved v0, in one transaction at startup.
Take the backup first; this migration cannot be reverted, and an older image cannot read the rewritten versions.
The server refuses to start if a stored version cannot be upgraded to a valid Spec, for example one that grows past the size limit; the error names the dashboard and version, the database is left unchanged, and you either restore the backup or fix that version.
The server runs migrations forward only; it never runs a migration's down section.
To go back to an older version, restore the backup you took before upgrading rather than running the older image against a migrated database.

On `SIGTERM` the server stops accepting requests, gives running requests 15 seconds, and hands a running agent request back to the queue without using up an attempt.
A crash instead leaves the request's lease to expire, so the next process picks it up after the lease ends.

## Logging

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

## Security model

- **Sign-in.**
  Only accounts from your OIDC provider, and only in the allowed email domains, get a session.
  Sessions and keys are random tokens stored only as SHA-256 hashes.
  Session cookies are `HttpOnly` and `SameSite=Lax`, and a cross-origin write made with a cookie is refused; `SLASHOBSERVE_PUBLIC_URL` is a trusted origin.
- **No roles yet.**
  In this release any signed-in user can read and write everything: every dashboard, comment and version.
  Any signed-in user can also create service accounts and keys, and revoke anyone's keys, so a key is as powerful as a person.
  Any signed-in user can also see everyone who has signed in and sign any of them out everywhere.
  Roles arrive in a later milestone.
  Until then, allow into SlashObserve only the people you would trust with all of its dashboards and datasources.
- **Datasource credentials stay on the server.**
  They never reach the browser.
  The datasources API returns health only, with no address, user or secret.
  The browser gets panel data from the server's query proxy, which runs only the queries stored in a saved dashboard and rejects any query text sent to it with `403 query_not_in_spec`.
- **Every query is bounded.**
  Whatever a prompt or a client asks for, each query has a timeout and limits on series, points and rows, applied on the server.
  ClickHouse is queried only as a read-only user.
- **The agent can only save dashboards.**
  Its only product is a dashboard definition that the server validates, and no code or markup written by the model is rendered.
  Through MCP, a key can also list datasources, run bounded queries and sample rows, and create and update dashboards.
- **Framing is refused.**
  Responses carry `X-Frame-Options: DENY` and `Content-Security-Policy: frame-ancestors 'none'`.
- **Keep keys short-lived.**
  Prefer an expiry, one service account per tool or person, and revoke on the API keys page when a laptop or pipeline is retired.

## Not covered here

- Evals, the scored prompt suite for model quality, are a development tool and not part of running SlashObserve.
- Roles and access control, and a Helm chart, are planned for later milestones.
