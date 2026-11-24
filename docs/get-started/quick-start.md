---
title: Quick start
description: Run the demo stack with Docker Compose, sign in, open the provisioned dashboard and send your first prompt to the agent.
---

This page starts the SlashObserve demo on your own machine: the OpenTelemetry Astronomy Shop with its telemetry stores and a SlashObserve server that reads them.
You sign in, see a dashboard that already holds live data, and ask the agent for a new one.
The demo is for trying SlashObserve, not for production; for a real deployment, see [Run your own deployment](install.md).

## What you need

- Docker with Compose v2.24.4 or newer.
- `git`, `curl`, `jq` and `openssl`.
- About 4 GiB of free memory and 12 GB of disk for the images.
- An Anthropic API key, so the agent can answer prompts.
  Without one the demo still starts, and you can look around.

You do not need Go or Bun, because the stack pulls the published SlashObserve image instead of building it.
The demo runs about thirty containers; [The demo](../operate/demo.md) has the measurements.

## Start the demo

1. Get the repository, which holds the Compose files and the configs they mount:

   ```sh
   git clone https://github.com/slashobserve/slashobserve && cd slashobserve
   ```

2. Give the server your Anthropic key.
   The demo reads it from your shell:

   ```sh
   export SLASHOBSERVE_LLM_PROVIDER=anthropic
   export SLASHOBSERVE_LLM_API_KEY=<your Anthropic API key>
   ```

3. Start the stack:

   ```sh
   make demo
   ```

   The first run fetches the pinned release of the shop and pulls about 12 GB of images.
   The stack is healthy about a minute after that, and the command then prints the addresses:

   ```text
   The Astronomy Shop is running at http://localhost:58081
   Its Jaeger UI is at http://localhost:58081/jaeger/ui and its feature flags at http://localhost:58081/feature/
   SlashObserve is running at http://localhost:58080
   Sign in as admin@example.com / password (Dex, development only)
   The service-account key is in /path/to/slashobserve/deploy/astronomy/.state/keys/astronomy.key
   ```

   The command exits with an error when the stack is not healthy after 15 minutes.

## Sign in

1. Open http://localhost:58080.
2. Sign in as `admin@example.com` with the password `password`.

The demo's Dex has three more people, `editor@example.com`, `commenter@example.com` and `viewer@example.com`, with the same password.
They all start as Editors, and an Admin sets their roles on the People page.
A person appears on that page only after their first sign-in, so sign in once as each of them, then sign back in as the Admin to change their roles.
To give a role before the first sign-in, [invite](../teams/people.md#invite-people) the email with that role.

## Open the dashboard

The shop's load generator sends traffic from the first minute.
A few minutes after the start, the folder Demo holds **Astronomy Shop overview**.
It shows traffic, errors and latency by service, orders and payments, the shop's database, its logs and traces.

Until it appears, the stores are still filling; [The demo](../operate/demo.md#the-dashboard) says why.
The stores start empty, so history builds up while the demo runs.
A prompt that compares today with yesterday needs a day of running.

## Send a prompt

1. Sign-in opens [Home](../use/home.md).
2. On its **Ask Observe Agent** tab, type a prompt such as `checkout health by service, last hour` into the box.
3. Press Enter.

The page switches to a conversation where you watch the agent build the dashboard.
[Your first dashboard](first-dashboard.md) walks through what you see and how to change the result.

Once a dashboard exists, you can also ask the agent about it from the dashboard itself.
Use its **Chat** tab, a comment thread, or **Ask** on a spot of a panel; see [Ask and comment on a dashboard](../use/ask-and-comment.md).

## Run without a model

Leave the Anthropic settings unset and run `make demo`.
The stack starts, but a prompt answers `503 agent_unavailable`.
Home's **Ask Observe Agent** tab then says no model is connected and offers **Import from Grafana**, so you can look around before you add a key.

To add a key later, export the settings and run `make demo` again, because the server reads them only when its container starts.

## Run with a local model

The agent also runs on a model served on your own machine through any OpenAI-compatible server, such as Ollama.
Set these before `make demo`:

```sh
export SLASHOBSERVE_LLM_PROVIDER=openai
export SLASHOBSERVE_LLM_BASE_URL=<address of your model server>
export SLASHOBSERVE_LLM_MODEL=<model name>
```

[Local models](../connect/local-models.md) gives the address the demo's server container uses to reach it, the need for tool calling, and what to expect from a small model.

## Inject a fault

To see the dashboards react to a failure, switch on one of the shop's feature flags and undo it afterwards:

```sh
make demo-fault FLAG=paymentFailure VARIANT=50%
make demo-fault-reset
```

`make demo-fault` with no flag lists every flag and its variants.
[The demo](../operate/demo.md#faults) explains what each flag does to the shop.

## Use the demo from the CLI or the API

The first start creates a service-account key with the Editor role for scripts, the CLI and MCP clients.
`scripts/astronomy.sh key` prints the path of the file that holds it.

```sh
export OBSERVE_SERVER=http://localhost:58080
export OBSERVE_TOKEN=$(cat "$(scripts/astronomy.sh key)")
observe dashboards list
```

You see the dashboard **Astronomy Shop overview** in the folder Demo.
[CLI](../use/cli.md#install) shows how to install `observe`.

## Add Elasticsearch and OpenSearch

To see the shop's logs in Elasticsearch and OpenSearch too, run `make demo-search` instead of `make demo`.
It builds the server image from your checkout, as `make demo-build` does.
It also adds Elasticsearch 8 and OpenSearch 2, which the shop's collector feeds its logs, and the `es` and `os` datasources then answer.
Without them those two datasources report that they are not answering, and nothing else depends on them.

Each store is capped at 1 GiB of memory and the two images take about 3.5 GB on disk, so use this only when you want to look at them.
`make demo-build` removes the two stores again, and `make demo-down` and `make demo-reset` remove them with the rest.

## Build the server from your checkout

`make demo-build` starts the same stack with the server image built from your working tree, for people changing the code.
The image is stamped with `git describe`, so Settings shows that version instead of `dev`.

## Stop or reset the demo

```sh
make demo-down
make demo-reset
```

`make demo-down` stops the stack and keeps its data.
`make demo-reset` also drops the volumes and the flag state, so the next start begins empty.

## Fix ClickHouse staying unhealthy after an upgrade

If ClickHouse stays unhealthy after an upgrade, an old demo volume predates the `slashobserve_ro` user or holds another password for it.
Run `make demo-reset` to recreate the volumes.

## Reference

### What make demo runs

`make demo` pulls the published SlashObserve image and starts it with the OpenTelemetry Astronomy Shop, Postgres, Dex, Prometheus, ClickHouse, Loki, Tempo and Jaeger.
It then waits until they are healthy.
The shop's addresses, the stores it feeds and the datasources the server reads are on [The demo](../operate/demo.md).

| Address | What it is |
| --- | --- |
| http://localhost:58080 | The SlashObserve server. |
| http://localhost:58081 | The shop. |
| http://localhost:58081/jaeger/ui | The shop's Jaeger UI. |
| http://localhost:58081/feature/ | The shop's feature flag UI. |

Dex listens on port 55556.
Set `SLASHOBSERVE_HOST_PORT`, `ASTRONOMY_SHOP_HOST_PORT` or `SLASHOBSERVE_DEX_HOST_PORT` to move a port.

### Model provider settings

The stack passes these settings from your shell to the server container:

- `SLASHOBSERVE_LLM_PROVIDER`
- `SLASHOBSERVE_LLM_MODEL`
- `SLASHOBSERVE_LLM_API_KEY`
- `SLASHOBSERVE_LLM_BASE_URL`
- `SLASHOBSERVE_LLM_LOG_PROVIDER_ERRORS`

The other LLM settings are not forwarded.
You can also put them in `deploy/astronomy/.env`, which Git ignores.
Run `make demo` again after changing them.

### Limits of the demo

Its Postgres password, Dex logins and OIDC client secret are fixed development values that anyone can read in this repository.
Every port it publishes binds to 127.0.0.1, so keep it that way.
Do not expose the demo on a cloud VM or any shared network.

### Commands

| Command | What it does |
| --- | --- |
| `make demo` | Pulls the published image and starts the stack. |
| `make demo-build` | Starts the stack with the server built from your checkout. |
| `make demo-search` | Builds from your checkout and adds Elasticsearch and OpenSearch. |
| `make demo-check` | Checks each datasource's health and reads one row of the shop's data from each. |
| `make demo-fault` | Lists the shop's flags, or sets one with `FLAG` and `VARIANT`. |
| `make demo-fault-reset` | Sets every flag back to its upstream variant. |
| `make demo-down` | Stops the stack and keeps its data. |
| `make demo-reset` | Stops the stack and drops its volumes and flag state. |
