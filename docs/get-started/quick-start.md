---
title: Quick start
description: Run the demo stack with Docker Compose and send your first prompt.
---

Requirements: Docker with Compose v2.20 or newer, and git.
You do not need Go or Bun: the server image is pulled, and the demo shop is built inside Docker from the repository.

Get the repository, which holds the Compose file and the configs it mounts, and run the demo stack with an Anthropic key so the agent can answer prompts:

```sh
git clone https://github.com/slashobserve/slashobserve && cd slashobserve
export SLASHOBSERVE_LLM_PROVIDER=anthropic
export SLASHOBSERVE_LLM_API_KEY=<your Anthropic API key>
make demo
```

Without `make`, run the same stack with `docker compose -f deploy/compose/compose.yaml up -d --build --wait`.

Open http://localhost:8080 and sign in as `admin@example.com` / `password`.
The demo's Dex also has `editor@example.com`, `commenter@example.com` and `viewer@example.com` with the same password; they start as Editors, and an Admin sets their roles on the People page.
Type a prompt such as `checkout health by pod, last 6h` into the chat composer on the home page.

`make demo` pulls the server image, builds the demo shop image from `demo/shop`, starts them with Postgres, Dex, Prometheus, ClickHouse, an OpenTelemetry Collector, node-exporter, cAdvisor and the shop, and waits until they are healthy.
The first run downloads about 0.7 GB of images, which take about 2.7 GB on disk, plus the Go build image for the shop.
It passes `SLASHOBSERVE_LLM_PROVIDER`, `SLASHOBSERVE_LLM_MODEL`, `SLASHOBSERVE_LLM_API_KEY` and `SLASHOBSERVE_LLM_BASE_URL` from your shell to the server container; the other LLM settings are not forwarded.
Run `make demo` again after changing them.
Without a provider the demo still starts, but prompts get `503 agent_unavailable`.
To build the server image from your working tree as well instead of pulling it, which is for people changing the code, run `make demo-build`.
Prometheus is on http://localhost:9090 and the shop's fault API on http://localhost:8081/fault.
Stop the stack with `make demo-down`, or drop its data as well with `make demo-reset`.
On first boot, and again after `make demo-reset`, Prometheus backfills 26 hours of synthetic request and latency history, so prompts that compare today with yesterday have data.

To see the dashboards react to a failure, break the checkout service with `make demo-fault TYPE=errors PCT=50` (or `TYPE=latency`) and undo it with `make demo-fault-reset`.

The demo's node-exporter enables the processes, TCP statistics and interrupts collectors and reports the root filesystem, so importing Grafana's Node Exporter Full finds data for those panels.
A container cannot see some hardware and host services, so these Node Exporter Full panels stay empty on the demo:

- Systemd Units State and Systemd Sockets Current, Accepted and Refused: the systemd collector needs the host's D-Bus socket, which the demo does not mount.
- Hardware Temperature Monitor, Hardware Fan Speed, Cooling Device Utilization and Power Supply: they read hwmon, thermal and power supply sysfs, which Docker Desktop's Linux VM does not expose.
- CPU Frequency Scaling: it reads cpufreq sysfs, which Docker Desktop's VM and most cloud VMs lack.
- CPU Saturation per Core and CPU Schedule Timeslices: they read `/proc/schedstat`, which Docker Desktop's kernel does not enable.
- Memory DirectMap: `/proc/meminfo` only reports it on x86 hosts, so it is empty on Apple Silicon.
- TCP Stat Transient: it only has data while connections sit in short-lived states such as SYN_SENT or FIN_WAIT1, and the exporter sees only its own container's connections.

If ClickHouse stays unhealthy after an upgrade, an old demo volume predates the `slashobserve_ro` user or holds another password for it; run `make demo-reset` to recreate the volumes.

The demo is for trying SlashObserve on your own machine, not for production.
Its Postgres password, Dex logins and OIDC client secret are fixed development values that anyone can read in this repository.
Every port it publishes binds to 127.0.0.1, so keep it that way and do not expose the demo on a cloud VM or any shared network.
For a real deployment, see [Run your own deployment](install.md).
