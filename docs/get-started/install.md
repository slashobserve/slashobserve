---
title: Run your own deployment
description: Pull the image and run SlashObserve against your own Postgres, identity provider and datasources.
---

SlashObserve is under active development, and releases before 1.0 can include breaking changes.
A release that has any lists them under "Breaking changes" in the [changelog](https://github.com/slashobserve/slashobserve/blob/main/CHANGELOG.md); read that part before you move to a newer version.

## Deploy with Docker

1. Create a Postgres database and a user that owns it.
   The server keeps a connection pool of at least 16 connections, or 8 plus 2 for each agent worker when that is more (24 with the default 8 workers), or the CPU count when that is more still, unless the database URL sets `pool_max_conns`, so `max_connections` must allow that for each server process.
   [Run more than one server](../operate/scaling.md#connections) counts the connections of a fleet.
   The project tests against Postgres 18.6; other versions are unverified.
2. Pull the image:

   ```sh
   docker pull ghcr.io/slashobserve/slashobserve:0.1.4
   ```

   Each release is tagged with its full version, such as `0.1.4`, with its minor version, such as `0.1`, and `latest`; a prerelease gets only its full version.
   Pin the full version.
   A new server applies database migrations when it starts, so the version you run should change only when you decide to upgrade (see [Upgrades and backups](../operate/upgrades-and-backups.md)).
   To run your own build instead, run `docker build -t slashobserve-server:local .` from the repository root and use that name in step 4.

3. Write the settings to an env file, for example `slashobserve.env`.
   Docker's `--env-file` takes `NAME=value` lines with no quoting.

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
   A server that only scripts and benchmark harnesses use can leave out the OIDC settings and the allowed domains; see [Run with no browser sign-in](../operate/service-account-keys.md#run-with-no-browser-sign-in).
   Without `SLASHOBSERVE_ADMIN_EMAILS` the first person to sign in is the Admin; an address in the list counts only when your provider reports it as verified (see [Sign-in with OIDC](../connect/oidc.md) and [Roles](../teams/roles.md)).
   `SLASHOBSERVE_SECRET_KEY` encrypts what the server stores for you: the credentials of datasources added in the app, the tokens of Grafana connections and the signing secrets of webhooks.
   Keep it with your backups, because those stored values cannot be read after a restore without it.
   Without `SLASHOBSERVE_LLM_PROVIDER` the server runs without the built-in agent; see [LLM providers](../connect/llm-providers.md).

4. Optionally write a [datasources file](../reference/datasources-file.md), which an Admin can also replace by adding datasources in the app (see [Add datasources in the app](../connect/datasources-in-the-app.md)), then start the container:

   ```sh
   docker run -d --name slashobserve --restart unless-stopped \
     -p 127.0.0.1:8080:8080 \
     --env-file slashobserve.env \
     -v "$PWD/datasources.yaml:/etc/slashobserve/datasources.yaml:ro" \
     ghcr.io/slashobserve/slashobserve:0.1.4
   ```

5. Check that it is ready:

   ```sh
   curl -fsS http://localhost:8080/readyz
   ```

## Startup behavior

The server applies database migrations when it starts, so an empty database is enough.
If it cannot connect to Postgres within 30 seconds, or any setting is invalid, it logs the problem and exits; `--restart unless-stopped` retries.
Every problem found in the settings is reported in one message.

## The container image

The image runs as a non-root user and listens on `:8080`.
Files you mount, such as the datasources file or a secret file, must be readable by that user.
The image has a Docker health check that calls `slashobserve-server healthcheck`, which requests `/readyz` on the configured `SLASHOBSERVE_HTTP_ADDR`.

## Install the CLI

The `observe` command line client installs separately, with one line on macOS and Linux; see [CLI](../use/cli.md#install).
