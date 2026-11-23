---
title: The demo
description: The OpenTelemetry Astronomy Shop running on SlashObserve's stores, with feature-flag faults you switch on to have an incident to investigate.
---

The demo is the OpenTelemetry project's own Astronomy Shop: about twenty services in many languages that sell telescopes, a browser and HTTP load generator, and faults you switch on with feature flags.
It runs a pinned release of the shop next to SlashObserve, with every signal the shop emits sent to Prometheus, Loki, Tempo, ClickHouse and Jaeger and one SlashObserve server reading all five.
The [quick start](../get-started/quick-start.md) takes you from nothing to a signed-in server on it.

## Start it

You need Docker with Compose 2.24.4 or newer, `git`, `curl`, `jq` and `openssl`, and a clone of the repository, which holds the Compose files.

```sh
make demo
```

This pulls the published SlashObserve image and starts the stack as the Compose project `slashobserve-astronomy`.
`make demo-build` builds the server from your checkout instead, which is for people changing the code.
The first start fetches the pinned upstream release of the shop and pulls about 12 GB of images, and the stack is healthy about a minute after that.

| Address | What it is |
| --- | --- |
| http://localhost:58081 | The shop. |
| http://localhost:58081/jaeger/ui | Its Jaeger UI. |
| http://localhost:58081/feature/ | Its feature flag UI. |
| http://localhost:58080 | SlashObserve, signed in as `admin@example.com` with `password` through Dex (development only). |

Set `ASTRONOMY_SHOP_HOST_PORT`, `SLASHOBSERVE_HOST_PORT` or `SLASHOBSERVE_DEX_HOST_PORT` to move a port.
Every port binds to 127.0.0.1.

The stores start empty, and the shop's load generator sends traffic all the time, so every chart has live data a minute or two after the start and gains history while the demo runs.
Prometheus keeps 3 days and the other stores 72 hours.
Stopping the demo keeps the data, and starting it again carries on from it.

## What it needs

On a host with 8 CPUs and 22 GB of memory, the 31 containers use 3.2 to 4.3 GiB of memory once the shop has run for a few minutes, and up to four cores in the first minutes and one or two afterwards.
The images take about 12 GB of disk, and the stores' volumes about 0.3 GB after the first half hour.
`make demo-search` adds Elasticsearch and OpenSearch, which take about 2 GiB more memory and 3.5 GB more disk.

## What is running

The shop is the upstream release in `deploy/astronomy/upstream.env`, fetched by commit into `deploy/astronomy/.upstream/`, with its full profile of Kafka, accounting and fraud detection.
Nothing in it is forked: the stack adds a Compose file and an OpenTelemetry Collector config on top of the upstream files.
The collector sends metrics to Prometheus, logs to Loki and ClickHouse, and traces to Tempo, ClickHouse and Jaeger.
Upstream's own Grafana, OpenSearch, OpAMP server and Prometheus are not started.

Two settings differ from upstream.
Jaeger is 2.23.0 and not upstream's 2.19.0, because the [Jaeger datasource](../connect/jaeger.md) needs 2.21 or newer.
The `checkout` and `product-catalog` services get 64 MB of memory instead of 20 MB, because at 20 MB their working set pages in and out of the file cache all the time, and on a busy host their health checks fail.

The server reads `deploy/astronomy/datasources.yaml`: `prom`, `ch`, `loki`, `tempo` and `jaeger`, plus `es` and `os`, which answer only after `make demo-search`.
It also reads `deploy/astronomy/service-accounts.yaml`, which declares the `astronomy` service account.
Its key is generated into `deploy/astronomy/.state/keys/astronomy.key` on the first start and never leaves your machine; `scripts/astronomy.sh key` prints the path.

## The dashboard

The server provisions one dashboard, **Astronomy Shop overview**, into the folder Demo, from `deploy/compose/dashboards/Demo/astronomy-overview.json`.
It reads the shop's span metrics and the Postgres metrics from Prometheus, and its logs and traces from ClickHouse, and it holds one panel of every type SlashObserve draws.
A provisioned file is saved only when every panel returns data, so the dashboard appears a few minutes after the start, once the shop has sent its first spans, and not before.

## Search stores

`make demo-search` builds the server from your checkout, as `make demo-build` does, and adds Elasticsearch 8 on the `es` datasource and OpenSearch 2 on the `os` datasource, which the shop's collector feeds its logs.
Elasticsearch gets ECS field names (`message`, `log.level`, `service.name`, `trace.id`, `span.id`) in the `logs-demo-shop` data stream, and OpenSearch gets the Simple Schema for Observability fields (`body`, `severity.text`, `resource.service.name`, `traceId`, `spanId`) in `ss4o_logs-demo-shop`.
Both run with their security features off, which is for this local demo only; a real cluster needs the API key or credentials described on its [datasource page](../connect/elasticsearch.md).
Running `make demo-build` again removes the two stores.

## Check the datasources

```sh
make demo-check
```

The check lists the datasources through the API, runs a fresh health check on each, and reads one row of the shop's data from it: a span-metrics series from Prometheus, frontend log lines from ClickHouse and Loki, and frontend traces from ClickHouse, Tempo and Jaeger.
It waits up to three minutes for each, and exits 1 if any fails.
`scripts/astronomy.sh check --search` also reads the logs from `es` and `os`.

## Faults

```sh
make demo-fault
make demo-fault FLAG=paymentFailure VARIANT=50%
make demo-fault FLAG=paymentFailure VARIANT=off
make demo-fault-reset
```

The first command lists the shop's flags with their current and possible variants.
The second sets one, and `on` is the default variant for a flag that has one.
The third switches it off again, and the fourth sets every flag back to the variant of the upstream release.
The change reaches the services within a few seconds, because flagd watches the file the targets edit.
The flag UI at `/feature/` reads the file only when it starts and writes its own copy back whole on every save.
It does not show a change the targets made, and a save in the UI overwrites that change, so use one of the two, or restart the UI with `docker restart slashobserve-astronomy-flagd-ui-1` after the target.

With `paymentFailure` at `50%`, the `payment` service starts to return errors for about half of the charges, so error spans appear on `payment` and on `checkout` above it, and fall back to none a minute after you switch it off.

## Turn on the agent

The stack has no model provider, so the agent is off.
Export the provider settings before `make demo`, or put them in `deploy/astronomy/.env`, which Git ignores, and run the target again:

```sh
SLASHOBSERVE_LLM_PROVIDER=anthropic
SLASHOBSERVE_LLM_MODEL=<model>
SLASHOBSERVE_LLM_API_KEY=<key>
```

Then start an investigation from the web app, or with the `astronomy` key over the API, the CLI or MCP.

## Stop it

`make demo-down` stops the stack and keeps its data.
`make demo-reset` also drops the volumes and the flag state, so the next start begins empty.
