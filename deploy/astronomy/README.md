# Astronomy Shop stack

The OpenTelemetry Astronomy Shop running with SlashObserve's stores, for investigating faults you switch on with feature flags.
The full guide is `docs/operate/astronomy-shop.md`.

```sh
make astronomy-build
make astronomy-check
make astronomy-flag FLAG=paymentFailure VARIANT=50%
make astronomy-flag FLAG=paymentFailure VARIANT=off
make astronomy-down
```

`scripts/astronomy.sh` runs all of it, and `make astronomy-search` adds Elasticsearch and OpenSearch.

## What it needs

Measured on an 8 CPU, 22 GB Linux host, with the shop running for a few minutes:

- Memory: 4.3 GiB for the 31 containers, 6.2 GiB with the `search` profile.
- Disk: 12 GB of images, 0.5 GB of volumes after the first hour, 3.5 GB more of images for Elasticsearch and OpenSearch.
- CPU: the load generator's headless browser is the largest consumer.

## How it is put together

- `upstream.env` pins the release by tag and commit; `scripts/astronomy.sh` fetches that commit into `.upstream/`, which Git ignores, and resets it on every start.
- `compose.yaml` is the layer on top of upstream's `compose.yaml` and `compose.full.yaml`.
  It pins every image by tag and digest, closes upstream's published ports and fixed container names, and adds Jaeger, Prometheus, ClickHouse, Loki, Tempo, Postgres, Dex and the server.
  The stores and the server extend their definitions in `deploy/compose/compose.yaml`, so their images, versions and settings have one home.
- `otelcol/config.yaml` is loaded last by upstream's collector, in the place upstream calls the extras layer, and adds the exporters.
  `otelcol/config.search.yaml` adds the Elasticsearch and OpenSearch exporters when the stack starts with `--search`.
- `compose.build.yaml` builds the server from this checkout; `compose.search.yaml` wires the collector to the search stores.
- `.state/` holds the service-account key and the flag file the make target edits, and Git ignores it.

Nothing is forked.
Differences from upstream: Jaeger 2.23.0 for the Jaeger datasource, 64 MB of memory for `checkout` and `product-catalog`, and no Grafana, OpenSearch, OpAMP server or upstream Prometheus.
