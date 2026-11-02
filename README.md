# SlashObserve

SlashObserve is an open-source, agentic Grafana replacement.
Type `/observe checkout health by pod, last 6h`, get a live dashboard on your Prometheus and ClickHouse data, and refine it in plain language.

Website and docs: https://slashobserve.github.io.

## Quick start

You need Docker with Compose v2.20 or newer.

    git clone https://github.com/slashobserve/slashobserve && cd slashobserve
    export SLASHOBSERVE_LLM_PROVIDER=anthropic SLASHOBSERVE_LLM_API_KEY=<your Anthropic API key>
    make demo

`make demo` pulls the published images, so nothing compiles on your machine.
The first run downloads about 2.8 GB of unpacked images and took about a minute on a fast connection.
Without `make`, run `docker compose -f deploy/compose/compose.yaml up -d --wait`; it reads the same environment variables.

Open http://localhost:8080, sign in as `admin@example.com` with the password `password`, and type a prompt such as `checkout health by pod, last 6h`.
Stop the stack with `make demo-down`, or drop its data as well with `make demo-reset`.
Without an LLM provider the demo still starts, but prompts answer `503 agent_unavailable`.

The demo is local only.
It uses fixed logins (`admin@example.com`, `editor@example.com`, `commenter@example.com` and `viewer@example.com`, all with the password `password`), a fixed Postgres password and a fixed OIDC client secret.
Every port it publishes binds to 127.0.0.1.
Do not expose it to a network or run it on a cloud VM; for a real deployment, follow [Run your own deployment](docs/get-started/install.md).

Contributors build the demo from source with `make demo-build`.

## What works today

- Prompt to dashboard from the web chat, from the `observe` CLI, and from MCP clients such as Claude Code, OpenCode and Cursor.
  A save runs every panel query and rejects a panel whose query fails or returns no data.
- Follow-up prompts revise the same dashboard.
  Every change is a saved version that you can compare and restore.
- Comments on panels, which you can send to the agent as a revision request.
- A panel editor for queries, settings and JSON, for changes that do not need the agent.
- 21 data panel types, among them time series, stat, table, heatmap, logs, traces, geomap and canvas, plus text, dashboard list, news and rows.
- Dashboard variables, including ad hoc filters and group-by.
- Import of Grafana dashboard JSON, with a report of what carried over and what did not.
- Sign-in through your OIDC provider, four roles, folders with access lists, and service-account keys for the CLI and MCP.

The [known limitations](docs/get-started/known-limitations.md) list what is missing.

## How it differs

From Grafana.
You describe a dashboard instead of assembling it panel by panel, and the result is one JSON document that the server validates against a schema before it saves it.
SlashObserve is not a Grafana fork or plugin, and it reads Prometheus and ClickHouse only.
Grafana supports many more datasources and has alerting, which SlashObserve does not.
If you already have Grafana dashboards, import them as JSON.

From chat-with-your-metrics tools.
The answer is a saved dashboard and not a chat reply: it has an address, versions, comments and folder access, and it keeps refreshing from your datasources after the conversation ends.
The agent can only save a dashboard definition, which the server checks before it keeps it.
You can also bring your own agent: MCP clients use the same tools and the same checks.

## LLM providers

The agent needs a model provider, and the server runs without one with the agent off.
`anthropic` is the default; `openai` works with OpenAI and with any server that implements the Chat Completions API with tool calling, including a local Ollama.
The agent sends what its tools return, such as metric names, label values and sampled rows, to the provider, so pick one that fits your data.
See [LLM providers](docs/connect/llm-providers.md) for the settings; the project does not vouch for any particular local model.

## Privacy

SlashObserve sends no telemetry and checks for no updates.
The server calls only the datasources, identity provider and LLM provider you configure, plus the domains on an outbound allowlist that starts empty.
The web app loads nothing from other sites.
See the [security model](docs/operate/security-model.md).

## CLI

The `observe` command is a thin client of the public API.
Download it from the [releases page](https://github.com/slashobserve/slashobserve/releases) or build it with `make cli`.
An Admin creates a service-account key on the **API keys** page; then:

    observe login --server http://localhost:8080
    observe "checkout p99 latency, last 6h"

See [CLI](docs/use/cli.md) and [MCP](docs/use/mcp.md).

## Stability

SlashObserve is pre-1.0.
Between minor releases the ObserveSpec format (version `0`), the HTTP API under `/api/v1`, the settings and the database schema can change in ways that need action from you.
[CHANGELOG.md](CHANGELOG.md) lists each release's changes.
Back up Postgres before every upgrade; see [Upgrades and backups](docs/operate/upgrades-and-backups.md).

## Contributing

Bug reports, feature requests and pull requests are welcome; see [CONTRIBUTING.md](CONTRIBUTING.md).
Report a security problem privately, as [SECURITY.md](SECURITY.md) describes.
This project follows a [code of conduct](CODE_OF_CONDUCT.md).

## License

Apache-2.0; see [LICENSE](LICENSE) and [NOTICE](NOTICE).
