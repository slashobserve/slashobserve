# observe-agent

`observe-agent` is the built-in agent as a separate process: an ordinary claimer that runs the model loop and works a run through the claim tools over MCP.
The server enforces the run's guards in `internal/runsession`; this package holds no guard of its own.
The server starts it when `SLASHOBSERVE_AGENT_RUNTIME=sidecar`; the image ships it at `/usr/local/bin/observe-agent` and `make agent` builds `bin/observe-agent`.

## Layout

- `src/protocol.ts`: the claim tools and the `_meta` keys, as zod schemas.
- `src/run.ts`: the model loop for one run.
- `src/session/`: the model sessions, one per provider.
- `src/session/replay/`: ports `internal/llm/replay`.

## Rules

- Versions are exact and at least 14 days old; `bunfig.toml` enforces both.
- Never log settings, keys, prompts, tool inputs, tool results or model text.
  `log()` takes only scalar fields.
- A claim-protocol change starts in Go and its docs (`docs/use/mcp.md`), then the zod schemas follow.
- Nothing observe-agent needs may become a requirement for other claimers.

## Commands

Run from `observe-agent/`:

- `bun install --frozen-lockfile`
- `bun run typecheck`
- `bun run format:check`
- `bun test`

From the repository root, `make agent` builds `bin/observe-agent`, and `make lint` and `make test` run the checks above.
