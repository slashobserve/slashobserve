# SlashObserve

SlashObserve turns a prompt into a live, Grafana-style dashboard.
Type `/observe checkout health by pod, last 6h`, get a dashboard backed by your Prometheus and ClickHouse data, and refine it in plain language.

This repository is the Community Edition: one Go binary with the web UI embedded, plus Postgres.
It is at an early stage: the scaffold is in place and features land step by step.
To run it on your own infrastructure, read the [self-hosting guide](docs/self-hosting.md).

## Run it locally

Requirements: Docker with Compose v2.20 or newer.

    make demo

Open http://localhost:8080, and stop the stack with `make demo-down`.
The agent answers prompts only when an LLM provider is configured, for example by exporting `SLASHOBSERVE_LLM_PROVIDER=anthropic` and `SLASHOBSERVE_LLM_API_KEY` before `make demo`; see [LLM providers](docs/self-hosting.md#llm-providers).

## Demo stack

`make demo` starts SlashObserve with Postgres, Dex, Prometheus, ClickHouse, an OpenTelemetry Collector, node-exporter, cAdvisor and a small demo shop with a load generator.
The app is on http://localhost:8080, Prometheus on http://localhost:9090 and the shop fault API on http://localhost:8081/fault.
Sign in as `admin@example.com` / `password`.
Break the checkout service with `make demo-fault TYPE=errors PCT=50` (or `TYPE=latency`), and undo it with `make demo-fault-reset`.
A fresh stack has no real history.
Prometheus backfills 26 hours of synthetic `http_requests_total` and `http_request_duration_seconds` histograms on first boot so "today vs yesterday" prompts and latency quantile panels have history.
Node and container metrics, logs and traces start when the stack starts.
`make demo-reset` drops the volumes so the backfill replays.

If ClickHouse stays unhealthy after an upgrade, an old demo volume predates the `slashobserve_ro` user or holds another password for it; run `make demo-reset` to recreate the volumes.

## CLI

`make cli` builds `bin/observe`, a thin client of the public API that uses a service-account key.
Create a service-account key on the web page `/keys` (**API keys** in the sidebar; a key is shown once), then sign in once:

    bin/observe login --server http://localhost:8080 --with-token

`login` reads the key from standard input with `--with-token`, or prompts for it on a terminal.
Then give it a prompt and it prints the dashboard address:

    bin/observe "checkout p99 latency, last 6h"
    bin/observe dashboards list

Add `--json` to any command to get the API response as it is, and see `bin/observe --help` for the rest.
Settings come from flags, then `OBSERVE_SERVER` and `OBSERVE_TOKEN`, then `~/.config/slashobserve/cli.json`.
The saved key is sent only to the server it was saved for; for another server, pass `--token` or set `OBSERVE_TOKEN`.
The same key connects Claude Code, OpenCode and Cursor to the server's MCP endpoint; see [MCP](docs/self-hosting.md#mcp).

## Develop

Requirements: Go 1.27.2 (a Go 1.21 or newer toolchain downloads it automatically), Bun 1.4.2, Docker.
See [CLAUDE.md](CLAUDE.md) for commands, layout and conventions.

## License

Apache-2.0; see [LICENSE](LICENSE) and [NOTICE](NOTICE).
