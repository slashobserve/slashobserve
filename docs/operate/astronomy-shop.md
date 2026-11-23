---
title: The Astronomy Shop
description: Run the OpenTelemetry Astronomy Shop on SlashObserve's stores with one command, and switch on its feature-flag faults to have an incident to investigate.
---

The Astronomy Shop is the OpenTelemetry project's own demo: about twenty services in many languages that sell telescopes, a browser and HTTP load generator, and faults you switch on with feature flags.
This stack runs a pinned release of it next to SlashObserve, with every signal the shop emits sent to Prometheus, Loki, Tempo, ClickHouse and Jaeger and one SlashObserve server reading all five.
It is a bigger and more realistic target than the [demo shop](demo.md), and it runs beside the demo stack because the two share no names or ports.

## Start it

You need Docker with Compose 2.24.4 or newer, `git`, `curl`, `jq` and `openssl`.

```sh
make astronomy-build
```

This builds the server from your checkout and starts the stack as the Compose project `slashobserve-astronomy`.
The first start fetches the pinned upstream release and pulls about 12 GB of images, and the stack is healthy about a minute after that.

| Address | What it is |
| --- | --- |
| http://localhost:58081 | The shop. |
| http://localhost:58081/jaeger/ui | Its Jaeger UI. |
| http://localhost:58081/feature/ | Its feature flag UI. |
| http://localhost:58080 | SlashObserve, signed in as `admin@example.com` with `password` through Dex (development only). |

Set `ASTRONOMY_SHOP_HOST_PORT`, `SLASHOBSERVE_HOST_PORT` or `SLASHOBSERVE_DEX_HOST_PORT` to move a port.
Every port binds to 127.0.0.1.

## What it needs

On a host with 8 CPUs and 22 GB of memory, the 31 containers use 3.2 to 4.3 GiB of memory once the shop has run for a few minutes, and up to four cores in the first minutes and one or two afterwards.
The images take about 12 GB of disk, and the stores' volumes about 0.3 GB after the first half hour.
`make astronomy-search` adds Elasticsearch and OpenSearch, which take about 2 GiB more memory and, if you have not pulled them for the demo, 3.5 GB of disk.
The stores keep 72 hours of data.

## What is running

The shop is the upstream release in `deploy/astronomy/upstream.env`, fetched by commit into `deploy/astronomy/.upstream/`, with its full profile of Kafka, accounting and fraud detection.
Nothing in it is forked: the stack adds a Compose file and an OpenTelemetry Collector config on top of the upstream files.
The collector sends metrics to Prometheus, logs to Loki and ClickHouse, and traces to Tempo, ClickHouse and Jaeger.
Upstream's own Grafana, OpenSearch, OpAMP server and Prometheus are not started.

Two settings differ from upstream.
Jaeger is 2.23.0 and not upstream's 2.19.0, because the [Jaeger datasource](../connect/jaeger.md) needs 2.21 or newer.
The `checkout` and `product-catalog` services get 64 MB of memory instead of 20 MB, because at 20 MB their working set pages in and out of the file cache all the time, and on a busy host their health checks fail.

The server reads `deploy/astronomy/datasources.yaml`: `prom`, `ch`, `loki`, `tempo` and `jaeger`, plus `es` and `os`, which answer only with `make astronomy-search`.
It also reads `deploy/astronomy/service-accounts.yaml`, which declares the `astronomy` service account.
Its key is generated into `deploy/astronomy/.state/keys/astronomy.key` on the first start and never leaves your machine; `scripts/astronomy.sh key` prints the path.

## Check the datasources

```sh
make astronomy-check
```

The check lists the datasources through the API, runs a fresh health check on each, and reads one row of the shop's data from it: a span-metrics series from Prometheus, frontend log lines from ClickHouse and Loki, and frontend traces from ClickHouse, Tempo and Jaeger.
It waits up to three minutes for each, and exits 1 if any fails.
`scripts/astronomy.sh check --search` also reads the logs from `es` and `os`.

## Switch on a fault

```sh
make astronomy-flag
make astronomy-flag FLAG=paymentFailure VARIANT=50%
make astronomy-flag FLAG=paymentFailure VARIANT=off
```

The first command lists the shop's flags with their current and possible variants.
The second sets one, and `on` is the default variant for a flag that has one.
The third switches it off again.
The change reaches the services within a few seconds, because flagd watches the file the target edits, and the flag UI at `/feature/` edits the same file.

With `paymentFailure` at `50%`, the `payment` service starts to return errors for about half of the charges, so error spans appear on `payment` and on `checkout` above it, and fall back to none a minute after you switch it off.

## Turn on the agent

The stack has no model provider, so the agent is off.
Put the provider settings in `deploy/astronomy/.env`, which Git ignores, and run `make astronomy-build` again:

```sh
SLASHOBSERVE_LLM_PROVIDER=anthropic
SLASHOBSERVE_LLM_MODEL=<model>
SLASHOBSERVE_LLM_API_KEY=<key>
```

Then start an investigation from the web app, or with the `astronomy` key over the API, the CLI or MCP.

## Stop it

`make astronomy-down` stops the stack and keeps its data.
`make astronomy-reset` also drops the volumes and the flag state, so the next start begins empty.
