---
title: Run your own deployment
description: Build the image and run SlashObserve against your own Postgres, identity provider and datasources.
---

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

4. Write a [datasources file](../reference/datasources-file.md), then start the container:

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
