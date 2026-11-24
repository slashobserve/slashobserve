---
title: Run your own deployment
description: Pull the image and run SlashObserve against your own Postgres, identity provider and datasources.
---

This page shows how to run SlashObserve from its container image, against a Postgres database you own and an identity provider your team signs in with.
To try it on your own machine first, use the [quick start](quick-start.md) instead.

SlashObserve is under active development, and releases before 1.0 can include breaking changes.
A release that has any lists them under "Breaking changes" in the [changelog](https://github.com/slashobserve/slashobserve/blob/main/CHANGELOG.md).
Read that part before you move to a newer version.

## Deploy with Docker

You need Docker, a Postgres database and an OIDC client from your identity provider (its issuer URL, client ID and client secret).
Register `<public url>/api/v1/auth/callback` as the client's redirect URI; [Sign-in with OIDC](../connect/oidc.md) has the rest of the setup.

1. Create a Postgres database and a user that owns it.
   The project tests against Postgres 18.6, and other versions are unverified.
   Check that `max_connections` allows the pool described under [Database connections](#database-connections).
2. Pull the image:

   ```sh
   docker pull ghcr.io/slashobserve/slashobserve:0.1.5
   ```

3. Write the settings to an env file such as `slashobserve.env`.
   Docker's `--env-file` takes `NAME=value` lines with no quoting.
   Replace each value in angle brackets with your own.

   ```text
   SLASHOBSERVE_DATABASE_URL=postgres://slashobserve:<password>@<postgres-host>:5432/slashobserve?sslmode=require
   SLASHOBSERVE_PUBLIC_URL=https://observe.example.com
   SLASHOBSERVE_OIDC_ISSUER_URL=https://idp.example.com
   SLASHOBSERVE_OIDC_CLIENT_ID=slashobserve
   SLASHOBSERVE_OIDC_CLIENT_SECRET=<client secret>
   SLASHOBSERVE_AUTH_ALLOWED_EMAIL_DOMAINS=example.com
   SLASHOBSERVE_ADMIN_EMAILS=you@example.com
   SLASHOBSERVE_SECRET_KEY=<output of openssl rand -hex 32>
   SLASHOBSERVE_DATASOURCES_FILE=/etc/slashobserve/datasources.yaml
   SLASHOBSERVE_LLM_PROVIDER=anthropic
   SLASHOBSERVE_LLM_API_KEY=<Anthropic API key>
   ```

   Only the database URL, the public URL, the three OIDC settings and the allowed domains are required.
   [Settings in the example](#settings-in-the-example) says what each one does.

4. Optionally write a [datasources file](../reference/datasources-file.md), then start the container.
   An Admin can also add datasources in the app instead; see [Add datasources in the app](../connect/datasources-in-the-app.md).

   ```sh
   docker run -d --name slashobserve --restart unless-stopped \
     -p 127.0.0.1:8080:8080 \
     --env-file slashobserve.env \
     -v "$PWD/datasources.yaml:/etc/slashobserve/datasources.yaml:ro" \
     ghcr.io/slashobserve/slashobserve:0.1.5
   ```

5. Check that the server is ready:

   ```sh
   curl -fsS http://localhost:8080/readyz
   ```

   A ready server answers `{"status":"ok","checks":{"database":"ok","migrations":"ok"}}`.
6. Open the public URL in a browser and sign in.
   With `SLASHOBSERVE_ADMIN_EMAILS` unset, the first person to sign in is the Admin.
   The example publishes the port on 127.0.0.1 only, and the server speaks plain HTTP.
   Serve the public URL from a reverse proxy that terminates TLS; see [Put TLS in front](../operate/production.md#put-tls-in-front).

## Choose an image tag

Each release gets three tags: its full version such as `0.1.5`, its minor version such as `0.1`, and `latest`.
A prerelease gets only its full version.
Pin the full version.
A new server applies database migrations when it starts, so the version you run should change only when you decide to upgrade.
See [Upgrades and backups](../operate/upgrades-and-backups.md) before you do.

## Run your own build

1. Run `docker build -t slashobserve-server:local .` from the repository root.
2. Use `slashobserve-server:local` in place of the `ghcr.io` image name in the `docker run` command.

## Run with no browser sign-in

A server that only scripts and benchmark harnesses use can leave out the OIDC settings and the allowed domains.
Leave all three OIDC settings unset, because setting only some of them is refused at start.
The settings that only browser sign-in reads are refused too, `SLASHOBSERVE_ADMIN_EMAILS` among them.
[Run with no browser sign-in](../operate/service-account-keys.md#run-with-no-browser-sign-in) has the details.

## Run without the built-in agent

Leave `SLASHOBSERVE_LLM_PROVIDER` unset.
The server then runs without the built-in agent.
To turn the agent on, see [LLM providers](../connect/llm-providers.md).

## Install the CLI

The `observe` command line client installs separately, with one line on macOS and Linux.
See [CLI](../use/cli.md#install).

## Reference

### Settings in the example

| Setting | Required | What it does |
| --- | --- | --- |
| `SLASHOBSERVE_DATABASE_URL` | Yes | The Postgres connection string. |
| `SLASHOBSERVE_PUBLIC_URL` | Yes | The address people use to reach the server. |
| `SLASHOBSERVE_OIDC_ISSUER_URL` | Yes | Your identity provider's issuer URL. |
| `SLASHOBSERVE_OIDC_CLIENT_ID` | Yes | The OIDC client ID. |
| `SLASHOBSERVE_OIDC_CLIENT_SECRET` | Yes | The OIDC client secret. |
| `SLASHOBSERVE_AUTH_ALLOWED_EMAIL_DOMAINS` | Yes | The email domains that may sign in. |
| `SLASHOBSERVE_ADMIN_EMAILS` | No | Addresses that become Admin. |
| `SLASHOBSERVE_SECRET_KEY` | No | Encrypts the secrets the server stores. |
| `SLASHOBSERVE_DATASOURCES_FILE` | No | The path of the datasources file in the container. |
| `SLASHOBSERVE_LLM_PROVIDER` | No | Turns the built-in agent on. |
| `SLASHOBSERVE_LLM_API_KEY` | With a provider | The key for the model provider. |

[Configuration](../reference/configuration.md) lists every setting.

#### Admin emails

An address in `SLASHOBSERVE_ADMIN_EMAILS` counts only when your provider reports it as verified.
See [Sign-in with OIDC](../connect/oidc.md) and [Roles](../teams/roles.md).

#### Secret key

`SLASHOBSERVE_SECRET_KEY` encrypts three kinds of stored value.
These are the credentials of datasources added in the app, the tokens of Grafana connections and the signing secrets of webhooks.
Keep the key with your backups, because the server cannot read those values after a restore without it.
Without the key, saving one of them answers `409 secret_key_missing`.

### Database connections

Each server process keeps a pool of at least 16 connections.
The pool grows to 8 plus 2 for each agent worker when that is more, which is 24 with the default 8 workers.
It grows to the CPU count when that is more still.
A `pool_max_conns` setting in the database URL replaces that size.
Set `max_connections` in Postgres to allow the pool for each server process.
[Run more than one server](../operate/scaling.md#connections) counts the connections of a fleet.

### Startup behavior

The server applies database migrations when it starts, so an empty database is enough.
If it cannot connect to Postgres within 30 seconds, or any setting is invalid, it logs the problem and exits.
`--restart unless-stopped` then retries.
The server reports every problem it finds in the settings in one message.

### The container image

The image runs as a non-root user and listens on `:8080`.
Files you mount, such as the datasources file or a secret file, must be readable by that user.
The image has a Docker health check that runs `slashobserve-server healthcheck`.
That command requests `/readyz` on the configured `SLASHOBSERVE_HTTP_ADDR`.
