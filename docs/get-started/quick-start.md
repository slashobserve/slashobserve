---
title: Quick start
description: Run the demo stack with Docker Compose and send your first prompt.
---

Requirements: Docker with Compose v2.20 or newer, and `make`.

Run the demo stack with an Anthropic key so the agent can answer prompts:

    export SLASHOBSERVE_LLM_PROVIDER=anthropic
    export SLASHOBSERVE_LLM_API_KEY=<your Anthropic API key>
    make demo

Open http://localhost:8080 and sign in as `admin@example.com` / `password`.
The demo's Dex also has `editor@example.com`, `commenter@example.com` and `viewer@example.com` with the same password; they start as Editors, and an Admin sets their roles on the People page.
Type a prompt such as `checkout health by pod, last 6h` into the chat composer on the home page.

`make demo` builds the server image and starts it with Postgres, Dex, Prometheus, ClickHouse, an OpenTelemetry Collector, node-exporter, cAdvisor and a demo shop, and waits until they are healthy.
It passes `SLASHOBSERVE_LLM_PROVIDER`, `SLASHOBSERVE_LLM_MODEL`, `SLASHOBSERVE_LLM_API_KEY` and `SLASHOBSERVE_LLM_BASE_URL` from your shell to the server container; the other LLM settings are not forwarded.
Run `make demo` again after changing them.
Without a provider the demo still starts, but prompts get `503 agent_unavailable`.
Stop it with `make demo-down`, or drop its data as well with `make demo-reset`.

The demo's node-exporter enables the processes, TCP statistics and interrupts collectors and reports the root filesystem, so importing Grafana's Node Exporter Full finds data for those panels.
A container cannot see some hardware and host services, so these Node Exporter Full panels stay empty on the demo:

- Systemd Units State and Systemd Sockets Current, Accepted and Refused: the systemd collector needs the host's D-Bus socket, which the demo does not mount.
- Hardware Temperature Monitor, Hardware Fan Speed, Cooling Device Utilization and Power Supply: they read hwmon, thermal and power supply sysfs, which Docker Desktop's Linux VM does not expose.
- CPU Frequency Scaling: it reads cpufreq sysfs, which Docker Desktop's VM and most cloud VMs lack.
- CPU Saturation per Core and CPU Schedule Timeslices: they read `/proc/schedstat`, which Docker Desktop's kernel does not enable.
- Memory DirectMap: `/proc/meminfo` only reports it on x86 hosts, so it is empty on Apple Silicon.
- TCP Stat Transient: it only has data while connections sit in short-lived states such as SYN_SENT or FIN_WAIT1, and the exporter sees only its own container's connections.

The demo is for trying SlashObserve, not for production.
Its Postgres password, Dex logins and OIDC client secret are fixed development values.
