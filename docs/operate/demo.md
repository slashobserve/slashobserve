---
title: The demo
description: The OpenTelemetry Astronomy Shop running on SlashObserve's stores, with feature-flag faults you switch on to have an incident to investigate.
---

The demo runs the OpenTelemetry project's own Astronomy Shop, about twenty services in many languages that sell telescopes, beside a SlashObserve server that reads the shop's telemetry.
Use it to try SlashObserve on real traffic, and to switch on a fault in the shop so there is an incident to investigate.
The [quick start](../get-started/quick-start.md) takes you from nothing to a signed-in server on it.

## Start the demo

You need Docker with Compose 2.24.4 or newer, `git`, `curl`, `jq` and `openssl`, and a clone of the repository, which holds the Compose files.

1. Start the stack from the root of the clone:

   ```sh
   make demo
   ```

   This pulls the latest published SlashObserve image and starts the stack as the Compose project `slashobserve-astronomy`.
   The first start fetches the pinned upstream release of the shop and pulls about 12 GB of images.
   The stack is healthy about a minute after that, and the command waits until it is.
2. Open http://localhost:58080 and sign in as `admin@example.com` with the password `password`.
   Sign-in goes through Dex, which is for development only.
3. Open the shop at http://localhost:58081 to see what the telemetry describes.
4. A few minutes after the start, open **Astronomy Shop overview** in the folder Demo.
   It is not there earlier, because [the dashboard](#the-dashboard) is saved only once every panel returns data.

The stores start empty, and the shop's load generator sends traffic all the time.
Every chart has live data a minute or two after the start and gains history while the demo runs.
Prometheus keeps 3 days, and Loki, Tempo and ClickHouse keep 72 hours.
Jaeger holds only the latest 25,000 traces, in memory.
Stopping the demo keeps the data of every store except Jaeger, which starts empty, and starting the demo again carries on from it.

| Address | What it is |
| --- | --- |
| http://localhost:58081 | The shop. |
| http://localhost:58081/jaeger/ui | Its Jaeger UI. |
| http://localhost:58081/feature/ | Its feature flag UI. |
| http://localhost:58080 | SlashObserve. |
| http://localhost:55556 | Dex, the sign-in provider. |

Every port binds to 127.0.0.1.
Set `ASTRONOMY_SHOP_HOST_PORT`, `SLASHOBSERVE_HOST_PORT` or `SLASHOBSERVE_DEX_HOST_PORT` to move the shop, SlashObserve or Dex to another port.

### Build the server from your checkout

`make demo-build` builds the server image from your working tree instead of pulling it, which is for people changing the code.
To run another server image with `make demo`, set `SLASHOBSERVE_IMAGE`.
`make demo` then skips the pull and uses the image as it is on your machine, so it can be a local build.

## Check that every datasource answers

```sh
make demo-check
```

The check lists the datasources through the API and runs a fresh health check on each.
It then reads one row of the shop's data from each, and prints one line per read:

```text
datasources: ch es jaeger loki os prom tempo
ok   prom     healthy, shop span metrics: 1 rows
```

It reads these rows:

- A span-metrics series from Prometheus.
- Frontend log lines from ClickHouse and Loki.
- Frontend traces from ClickHouse, Tempo and Jaeger.

It waits up to three minutes for each read, prints `FAIL` with the reason for one that does not answer, and exits 1 if any fails.
`scripts/astronomy.sh check --search` also reads the logs from `es` and `os`; see [Add Elasticsearch and OpenSearch](#add-elasticsearch-and-opensearch).

## Switch on a fault

A fault is a feature flag in the shop.
This switches on `paymentFailure` at 50%, so about half of the charges fail, and then switches it off:

```sh
make demo-fault FLAG=paymentFailure VARIANT=50%
make demo-fault FLAG=paymentFailure VARIANT=off
```

With `paymentFailure` at `50%`, the `payment` service returns errors for about half of the charges.
Error spans appear on `payment` and on `checkout` above it, and fall back to none a minute after you switch the flag off.
Then start an investigation from the web app, or ask the agent what is failing; [Turn on the agent](#turn-on-the-agent) says how.

To see every flag with its current and possible variants, run the target with no flag:

```sh
make demo-fault
```

Each line holds a flag, its current variant and its possible variants:

```text
paymentFailure  off  10%,100%,25%,50%,75%,90%,off
```

To set every flag back to the variant of the upstream release, run `make demo-fault-reset`.
[Faults](#faults) says what each flag does.

### Which flag UI to use

The change reaches the services within a few seconds, because flagd watches the file the targets edit.
The flag UI at `/feature/` reads the file only when it starts and writes its own copy back whole on every save.
It does not show a change the targets made, and a save in the UI overwrites that change.
Use one of the two.
Or restart the UI after a target with `docker restart slashobserve-astronomy-flagd-ui-1`.

## Turn on the agent

The stack has no model provider, so the agent is off.
Without one, prompts answer `503 agent_unavailable`.
Export the provider settings before `make demo`, or put them in `deploy/astronomy/.env`, which Git ignores, and run the target again:

```sh
SLASHOBSERVE_LLM_PROVIDER=anthropic
SLASHOBSERVE_LLM_MODEL=<model>
SLASHOBSERVE_LLM_API_KEY=<key>
```

The stack also passes `SLASHOBSERVE_LLM_BASE_URL` and `SLASHOBSERVE_LLM_LOG_PROVIDER_ERRORS` to the server, and no other LLM setting.
Then start an investigation from the web app, or with the `astronomy` key over the API, the CLI or MCP.
`scripts/astronomy.sh key` prints the path of the key.

## Add Elasticsearch and OpenSearch

```sh
make demo-search
```

This builds the server from your checkout, as `make demo-build` does.
It adds Elasticsearch 8 on the `es` datasource and OpenSearch 2 on the `os` datasource, which the shop's collector feeds its logs.
Elasticsearch gets ECS field names (`message`, `log.level`, `service.name`, `trace.id`, `span.id`) in the `logs-demo-shop` data stream.
OpenSearch gets the Simple Schema for Observability fields (`body`, `severity.text`, `resource.service.name`, `traceId`, `spanId`) in `ss4o_logs-demo-shop`.
Both run with their security features off, which is for this local demo only.
A real cluster needs the API key or credentials described on its [datasource page](../connect/elasticsearch.md).

Without `make demo-search`, the `es` and `os` datasources do not answer.
Running `make demo-build` or `make demo` again removes the two stores.

## Let the agent read the shop's code

```sh
make demo-code
```

This builds the server from your checkout, as `make demo-build` does, and adds a small read-only code server over the shop's source.
It registers the server as the [context source](context-sources.md) `shop-code`, with three tools approved: `list_dir`, `read_file` and `grep`.
The code server sees only the source of the pinned release in `deploy/astronomy/.upstream/src`, which it gets as a read-only mount, and it has no tool that writes.
`make demo-check` then also checks that `shop-code` is ready and has its three approved tools.

To see the agent use it, with [the agent turned on](#turn-on-the-agent):

1. Switch on a fault, for example `make demo-fault FLAG=paymentFailure VARIANT=50%`.
2. Wait a minute for error spans to appear on `payment`, then ask the agent why payments fail, from a new investigation in the web app or with the `astronomy` key.
3. The run shows its calls to `shop-code`.
   It lists a service's directory, searches for the flag's name and reads the file that uses it, and its finding names that file.
4. Switch the fault off with `make demo-fault FLAG=paymentFailure VARIANT=off`.

The same source works for every other flag in [Faults](#faults).
`make demo-build` or `make demo` removes the code server and the source again.

The layer is a little different from a source you would add yourself:

- The code server is on the Compose network as `codeserver.demo.internal` and speaks plain `http`.
  `deploy/astronomy/context-sources.yaml` sets `allowPrivate: true` for it, the provisioning-file switch that [Context sources](context-sources.md#provision-sources-from-a-file) describes, so the server may call a private address.
- The layer fixes `SLASHOBSERVE_OUTBOUND_DOMAINS` to `codeserver.demo.internal`, so the allowlist holds only that name while it runs.
- Provisioning cannot approve tools.
  `make demo-code` adds the service account `astronomy-admin`, with its own key in `deploy/astronomy/.state/keys/astronomy-admin.key`, and uses it once the stack is healthy to discover the tools and approve the read-only ones.

## Stop the demo or start over

`make demo-down` stops the stack and keeps its data.
`make demo-reset` also drops the volumes and the flag state, so the next start begins empty.

## Reference

### What it needs

On a host with 8 CPUs and 22 GB of memory, the 31 containers use 3.2 to 4.3 GiB of memory once the shop has run for a few minutes.
They use up to four cores in the first minutes and one or two afterwards.
The images take about 12 GB of disk, and the stores' volumes about 0.3 GB after the first half hour.
`make demo-search` adds Elasticsearch and OpenSearch, which take about 2 GiB more memory and 3.5 GB more disk.

### What is running

The shop is the upstream release in `deploy/astronomy/upstream.env`, fetched by commit into `deploy/astronomy/.upstream/`, with its full profile of Kafka, accounting and fraud detection.
Nothing in it is forked.
The stack adds a Compose file and an OpenTelemetry Collector config on top of the upstream files.
The collector sends metrics to Prometheus, logs to Loki and ClickHouse, and traces to Tempo, ClickHouse and Jaeger.
One SlashObserve server reads all five stores.
Upstream's own Grafana, OpenSearch, OpAMP server and Prometheus are not started.

Two settings differ from upstream:

- Jaeger is 2.23.0 and not upstream's 2.19.0, because the [Jaeger datasource](../connect/jaeger.md) needs 2.21 or newer.
- The `checkout` and `product-catalog` services get 64 MB of memory instead of 20 MB.
  At 20 MB their working set pages in and out of the file cache all the time, and on a busy host their health checks fail.

The server reads `deploy/astronomy/datasources.yaml`, which declares `prom`, `ch`, `loki`, `tempo` and `jaeger`, plus `es` and `os`, which answer only after `make demo-search`.
It also reads `deploy/astronomy/service-accounts.yaml`, which declares the `astronomy` service account.
With `make demo-code` it reads `deploy/astronomy/service-accounts.code.yaml` instead, which adds `astronomy-admin`, and `deploy/astronomy/context-sources.yaml`.
Its key is generated into `deploy/astronomy/.state/keys/astronomy.key` on the first start and never leaves your machine.

### The dashboard

The server provisions one dashboard, **Astronomy Shop overview**, into the folder Demo, from `deploy/compose/dashboards/Demo/astronomy-overview.json`.
It reads the shop's span metrics and the Postgres metrics from Prometheus, and its logs and traces from ClickHouse.
It holds one panel of every type SlashObserve draws.
A provisioned file is saved only when every panel returns data.
So the dashboard appears a few minutes after the start, once the shop has sent its first spans, and not before.
[Provisioning](provisioning.md) has the rules.

### Faults

These are the flags of the upstream release the stack pins, from its `demo.flagd.json`.
Every flag starts `off`.

| Flag | What it does | Variants |
| --- | --- | --- |
| `adFailure` | Fails the ad service. | `off`, `on` |
| `adHighCpu` | Loads the ad service's CPU. | `off`, `on` |
| `adManualGc` | Triggers full manual garbage collections in the ad service. | `off`, `on` |
| `cartFailure` | Fails the cart service that share of the time. | `off`, `10%`, `25%`, `50%`, `75%`, `90%`, `100%` |
| `emailMemoryLeak` | Leaks memory in the email service, faster at a higher multiple. | `off`, `1x`, `10x`, `100x`, `1000x`, `10000x` |
| `emitRawPii` | Emits raw PII attributes from checkout and payment; it is not a failure. | `off`, `on` |
| `failedReadinessProbe` | Fails the readiness probe of the cart service. | `off`, `on` |
| `imageSlowLoad` | Slows image loading in the frontend. | `off`, `5sec`, `10sec` |
| `intlShippingSlowdown` | Delays international shipping responses. | `off`, `5sec`, `10sec` |
| `kafkaQueueProblems` | Overloads the Kafka queue and delays the consumer, so lag spikes. | `off`, `on` |
| `loadGeneratorFloodHomepage` | Floods the frontend with requests. | `off`, `on` |
| `paymentFailure` | Fails payment charges that share of the time. | `off`, `10%`, `25%`, `50%`, `75%`, `90%`, `100%` |
| `paymentUnreachable` | Makes the payment service unavailable. | `off`, `on` |
| `productCatalogFailure` | Fails the product catalog service on one product. | `off`, `on` |
| `productCatalogLockContention` | Causes lock contention on the product catalog database. | `off`, `on` |
| `recommendationCacheFailure` | Fails the recommendation service's cache. | `off`, `on` |
| `aiRunawayAgent` | Keeps the shop's AI agent looping on tool calls. | `off`, `on` |
| `aiSlowResponse` | Slows the shop's AI agent's model responses. | `off`, `5sec`, `10sec` |

The stack does not start the shop's AI agent service, so the last two flags change nothing.
`make demo-fault` sets a flag to `on` when you give no variant, and refuses a flag that has no `on` variant, such as `paymentFailure`, with the variants it has.
It refuses an unknown flag or variant the same way.
