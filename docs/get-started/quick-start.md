---
title: Quick start
description: Run the demo stack with Docker Compose and send your first prompt.
---

## Requirements

Requirements: Docker with Compose v2.20 or newer, and git.
You do not need Go or Bun, and the images are pulled, not built.

## Start the demo

Get the repository, which holds the Compose file and the configs it mounts, and run the demo stack with an Anthropic key so the agent can answer prompts:

```sh
git clone https://github.com/slashobserve/slashobserve && cd slashobserve
export SLASHOBSERVE_LLM_PROVIDER=anthropic
export SLASHOBSERVE_LLM_API_KEY=<your Anthropic API key>
make demo
```

Without `make`, run the same stack with `docker compose -f deploy/compose/compose.yaml up -d --wait`.

## Sign in

Open http://localhost:8080 and sign in as `admin@example.com` / `password`.

The demo's Dex also has `editor@example.com`, `commenter@example.com` and `viewer@example.com` with the same password; they start as Editors, and an Admin sets their roles on the People page.

## Send a prompt

Type a prompt such as `checkout health by pod, last 6h` into the chat composer on the home page.

## Dashboards the demo provisions

The demo provisions three dashboards from `deploy/compose/dashboards`, so the Dashboards page has live data before you send a prompt, with or without a key:

- Demo shop overview, in the Demo folder: is the shop's traffic healthy, are orders flowing, is the database keeping up, and what checkout's logs and traces say.
- Node health, in the Demo folder: CPU, memory, disks and network of the machine running the stack, and what each of its containers uses.
- SlashObserve internals, in the SlashObserve folder: the server's own API, agent, model calls, query proxy, database pool and scheduler, read from its `/metrics`.

They appear within a minute of the stack becoming healthy; [Provisioning](../operate/provisioning.md) explains how a file in that directory becomes a dashboard.

## What make demo does

### Services and images

`make demo` pulls the server and demo shop images and starts them with Postgres, Dex, Prometheus, ClickHouse, Loki, Tempo, an OpenTelemetry Collector, node-exporter, cAdvisor and the shop, and waits until they are healthy.
The first run downloads about 0.7 GB of images, which take about 2.7 GB on disk, and took about a minute on a fast connection.

### Model provider settings

It passes `SLASHOBSERVE_LLM_PROVIDER`, `SLASHOBSERVE_LLM_MODEL`, `SLASHOBSERVE_LLM_API_KEY`, `SLASHOBSERVE_LLM_BASE_URL` and `SLASHOBSERVE_LLM_LOG_PROVIDER_ERRORS` from your shell to the server container; the other LLM settings are not forwarded.
Run `make demo` again after changing them.
Without a provider the demo still starts, but prompts get `503 agent_unavailable`.
The home page then shows the three dashboards and the **Import from Grafana** link in place of the composer, so you can look around before you add a key.

### Ports, rebuilds and shutdown

To build the server and shop images from your working tree instead of pulling them, which is for people changing the code, run `make demo-build`.
Prometheus is on http://localhost:9090 and the shop's fault API on http://localhost:8081/fault.
Stop the stack with `make demo-down`, or drop its data as well with `make demo-reset`.

On first boot, and again after `make demo-reset`, Prometheus backfills 26 hours of synthetic request and latency history, so prompts that compare today with yesterday have data.

## Add Elasticsearch and OpenSearch

To see the shop's logs in Elasticsearch and OpenSearch as well, run `make demo-search` instead of `make demo-build`.
It adds Elasticsearch 8 on http://localhost:9200, OpenSearch 2 on http://localhost:9201 and a second OpenTelemetry Collector that writes the shop's logs to both, and the Elasticsearch and OpenSearch datasources of the demo then answer.
Without it those two datasources report that they are not answering, and nothing else depends on them.

Each store is capped at 1 GiB of memory and the two images take about 3.5 GB on disk, so leave this for when you want to look at them.
Elasticsearch gets ECS field names (`message`, `log.level`, `service.name`, `trace.id`, `span.id`) in the `logs-demo-shop` data stream, and OpenSearch gets the Simple Schema for Observability fields (`body`, `severity.text`, `resource.service.name`, `traceId`, `spanId`) in `ss4o_logs-demo-shop`.

Both run with their security features off, which is for this local demo only; a real cluster needs the API key or credentials described on its [datasource page](../connect/elasticsearch.md).
`make demo-search-down` stops only the two stores and their collector, and `make demo-down` and `make demo-reset` remove them too.

## Run with a local model

The agent also runs on a model served on your own machine through any OpenAI-compatible server, such as Ollama, with `SLASHOBSERVE_LLM_PROVIDER=openai`, `SLASHOBSERVE_LLM_BASE_URL` and `SLASHOBSERVE_LLM_MODEL` set before `make demo`.
[Local models](../connect/local-models.md) gives the address the demo's server container uses to reach it, the need for tool calling, and what to expect from a small model.

## Inject a fault

To see the dashboards react to a failure, break the checkout service with `make demo-fault TYPE=errors PCT=50` (or `TYPE=latency`) and undo it with `make demo-fault-reset`.

## Node Exporter Full on the demo

The demo's node-exporter enables the processes, TCP statistics and interrupts collectors and reports the root filesystem, so importing Grafana's Node Exporter Full finds data for those panels.
A container cannot see some hardware and host services, so these Node Exporter Full panels stay empty on the demo:

- Systemd Units State and Systemd Sockets Current, Accepted and Refused: the systemd collector needs the host's D-Bus socket, which the demo does not mount.
- Hardware Temperature Monitor, Hardware Fan Speed, Cooling Device Utilization and Power Supply: they read hwmon, thermal and power supply sysfs, which Docker Desktop's Linux VM does not expose.
- CPU Frequency Scaling: it reads cpufreq sysfs, which Docker Desktop's VM and most cloud VMs lack.
- CPU Saturation per Core and CPU Schedule Timeslices: they read `/proc/schedstat`, which Docker Desktop's kernel does not enable.
- Memory DirectMap: `/proc/meminfo` only reports it on x86 hosts, so it is empty on Apple Silicon.
- TCP Stat Transient: it only has data while connections sit in short-lived states such as SYN_SENT or FIN_WAIT1, and the exporter sees only its own container's connections.

## ClickHouse unhealthy after an upgrade

If ClickHouse stays unhealthy after an upgrade, an old demo volume predates the `slashobserve_ro` user or holds another password for it; run `make demo-reset` to recreate the volumes.

## Limits of the demo

The demo is for trying SlashObserve on your own machine, not for production.
Its Postgres password, Dex logins and OIDC client secret are fixed development values that anyone can read in this repository.
Every port it publishes binds to 127.0.0.1, so keep it that way and do not expose the demo on a cloud VM or any shared network.
For a real deployment, see [Run your own deployment](install.md).
