---
title: Quick start
description: Run the demo stack with Docker Compose and send your first prompt.
---

## Requirements

Requirements: Docker with Compose v2.24.4 or newer, `git`, `curl`, `jq` and `openssl`.
You do not need Go or Bun, and the SlashObserve image is pulled, not built.
The demo runs about thirty containers and needs about 4 GiB of memory and 12 GB of disk; [The demo](../operate/demo.md) has the measurements.

## Start the demo

Get the repository, which holds the Compose files and the configs they mount, and run the demo with an Anthropic key so the agent can answer prompts:

```sh
git clone https://github.com/slashobserve/slashobserve && cd slashobserve
export SLASHOBSERVE_LLM_PROVIDER=anthropic
export SLASHOBSERVE_LLM_API_KEY=<your Anthropic API key>
make demo
```

## Sign in

Open http://localhost:58080 and sign in as `admin@example.com` / `password`.

The demo's Dex also has `editor@example.com`, `commenter@example.com` and `viewer@example.com` with the same password; they start as Editors, and an Admin sets their roles on the People page.
A person appears on that page only after their first sign-in, so sign in once as each of them, then sign back in as the Admin to change their roles.
To give a role before the first sign-in, [invite](../teams/people.md#invite-people) the email with that role.

## Open the dashboard

The shop's load generator sends traffic from the first minute, and a few minutes after the start the folder Demo holds **Astronomy Shop overview**: traffic, errors and latency by service, orders and payments, the shop's database, its logs and traces.
Until it appears, the stores are still filling; [The demo](../operate/demo.md#the-dashboard) says why.

## Send a prompt

Sign-in opens [Home](../use/home.md).
Type a prompt such as `checkout health by service, last 6h` into the chat composer on its **Ask Observe Agent** tab.
Once a dashboard exists you can also ask the agent about it from the dashboard itself: in its **Chat** tab, in a comment thread, or by pointing at a spot on a panel with **Ask**; see [Ask and comment on a dashboard](../use/ask-and-comment.md).

## What make demo does

### Services and images

`make demo` pulls the published SlashObserve image and starts it with the OpenTelemetry Astronomy Shop, Postgres, Dex, Prometheus, ClickHouse, Loki, Tempo and Jaeger, and waits until they are healthy.
The first run fetches the pinned release of the shop and pulls about 12 GB of images, and the stack is healthy about a minute after that.
The shop's addresses, the stores it feeds and the datasources the server reads are on [The demo](../operate/demo.md).

### Model provider settings

It passes `SLASHOBSERVE_LLM_PROVIDER`, `SLASHOBSERVE_LLM_MODEL`, `SLASHOBSERVE_LLM_API_KEY`, `SLASHOBSERVE_LLM_BASE_URL` and `SLASHOBSERVE_LLM_LOG_PROVIDER_ERRORS` from your shell to the server container; the other LLM settings are not forwarded.
Run `make demo` again after changing them.
Without a provider the demo still starts, but prompts get `503 agent_unavailable`.
Home's **Ask Observe Agent** tab then says no model is connected and offers **Import from Grafana**, so you can look around before you add a key.

### Ports, rebuilds and shutdown

To build the server image from your working tree instead of pulling it, which is for people changing the code, run `make demo-build`.
The image is stamped with `git describe`, so Settings shows that version instead of `dev`.
The shop is on http://localhost:58081.
Stop the stack with `make demo-down`, or drop its data as well with `make demo-reset`.

The stores start empty, so history builds up while the demo runs; a prompt that compares today with yesterday needs a day of running.

## Add Elasticsearch and OpenSearch

To see the shop's logs in Elasticsearch and OpenSearch as well, run `make demo-search` instead of `make demo`.
It builds the server image from your checkout, as `make demo-build` does, and adds Elasticsearch 8 and OpenSearch 2, which the shop's collector feeds its logs, and the Elasticsearch and OpenSearch datasources of the demo then answer.
Without it those two datasources report that they are not answering, and nothing else depends on them.

Each store is capped at 1 GiB of memory and the two images take about 3.5 GB on disk, so leave this for when you want to look at them.
`make demo-build` removes the two stores again, and `make demo-down` and `make demo-reset` remove them with the rest.

## Run with a local model

The agent also runs on a model served on your own machine through any OpenAI-compatible server, such as Ollama, with `SLASHOBSERVE_LLM_PROVIDER=openai`, `SLASHOBSERVE_LLM_BASE_URL` and `SLASHOBSERVE_LLM_MODEL` set before `make demo`.
[Local models](../connect/local-models.md) gives the address the demo's server container uses to reach it, the need for tool calling, and what to expect from a small model.

## Inject a fault

To see the dashboards react to a failure, switch on one of the shop's feature flags, for example `make demo-fault FLAG=paymentFailure VARIANT=50%`, and undo it with `make demo-fault-reset`.
`make demo-fault` with no flag lists them all; [The demo](../operate/demo.md#faults) explains what each does to the shop.

## ClickHouse unhealthy after an upgrade

If ClickHouse stays unhealthy after an upgrade, an old demo volume predates the `slashobserve_ro` user or holds another password for it; run `make demo-reset` to recreate the volumes.

## Limits of the demo

The demo is for trying SlashObserve on your own machine, not for production.
Its Postgres password, Dex logins and OIDC client secret are fixed development values that anyone can read in this repository.
Every port it publishes binds to 127.0.0.1, so keep it that way and do not expose the demo on a cloud VM or any shared network.
For a real deployment, see [Run your own deployment](install.md).
