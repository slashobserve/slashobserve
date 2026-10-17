# SlashObserve

SlashObserve turns a prompt into a live, Grafana-style dashboard.
Type `/observe checkout health by pod, last 6h`, get a dashboard backed by your Prometheus and ClickHouse data, and refine it in plain language.

This repository is the Community Edition: one Go binary with the web UI embedded, plus Postgres.
It is at an early stage: the scaffold is in place and features land step by step.

## Run it locally

Requirements: Docker with Compose v2.20 or newer.

    make demo

Open http://localhost:8080, and stop the stack with `make demo-down`.

## Develop

Requirements: Go 1.27.2 (a Go 1.21 or newer toolchain downloads it automatically), Bun 1.4.2, Docker.
See [CLAUDE.md](CLAUDE.md) for commands, layout and conventions.

## License

Apache-2.0; see [LICENSE](LICENSE) and [NOTICE](NOTICE).
