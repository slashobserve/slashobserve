# SlashObserve

SlashObserve is an agentic Grafana replacement: an engineer types `/observe checkout health by pod, last 6h` and gets a live dashboard, then refines it in plain language.
Every change is a saved version.
This repository is the Community Edition monorepo (Apache-2.0), Go module `github.com/slashobserve/slashobserve`.
The product brief lives outside this repository, at `../docs/BRIEF.md` in the SlashObserve workspace; read it for background, but this file and the area files below carry every convention needed to work here.

## Commands

| Command | What it does |
| --- | --- |
| `make build` | Install web dependencies, build the Vite app into `internal/server/webdist`, then build `bin/slashobserve-server` with the UI embedded. |
| `make server` | Build only the Go binary, embedding whatever is in `internal/server/webdist`. |
| `make cli` | Build only `bin/observe`; `make build` builds it too. |
| `make test` | `go test -race ./...` for the root module and `demo/shop`, the web Vitest suite, then `scripts/release-scripts-test.sh` (the release tag and release notes scripts) and `scripts/install-test.sh` (the root `install.sh`, against fake release assets). |
| `make lint` | golangci-lint (config verify, format diff, run), `sqlc diff`, web typecheck, Spec drift and fixture parity check, web API type drift check (`api:check`), ESLint, knip (unused web files, exports and dependencies), Prettier check. |
| `make ci` | `lint`, `test`, `tidy-check` (`go mod tidy -diff` for the root module and `demo/shop`), `build`: what the CI `check` job runs. |
| `make audit` | `govulncheck` for the root module and `demo/shop`, `bun audit --prod` and `bun audit --audit-level=high` for the web lockfile (minus the advisories in `WEB_AUDIT_IGNORE`): what the CI `audit` job runs. |
| `make generate` | Regenerate sqlc code, the web Spec types/validator and the web API types (`web/src/api/schema.gen.ts`) after editing migrations, queries, the schema or `api/`. |
| `make golden` | Rewrite the `pkg/grafana` corpus goldens and the generated option map (`optionmap.json`). |
| `make demo` / `make demo-down` | Run the full demo stack in `deploy/compose` from the published images `ghcr.io/slashobserve/slashobserve` and `ghcr.io/slashobserve/demo-shop` (tag `latest`, overridable with `SLASHOBSERVE_IMAGE` and `SLASHOBSERVE_SHOP_IMAGE`) with no compile, and stop it; it cannot pull until the first release tag is published, so use `make demo-build` until then. The stack: the app on http://localhost:8080, Prometheus on http://localhost:9090 and the shop fault API on http://localhost:8081, with Dex, Postgres, ClickHouse, the OTel Collector, node-exporter, cAdvisor and the shop behind them. |
| `make demo-build` | The same stack built from this checkout (`deploy/compose/compose.build.yaml` on top of `compose.yaml`, images `slashobserve-server:local` and `slashobserve-demo-shop:local`); the path for contributors. |
| `make demo-reset` | Stop the demo stack and drop its volumes, so the 26 hour request and latency backfill runs again on the next `make demo`. |
| `make demo-fault TYPE=errors\|latency PCT=N` | Inject a fault into the `checkout-0` pod through the shop fault API. |
| `make demo-fault-reset` | Clear the injected fault. |
| `make e2e` | Reset and boot the isolated `slashobserve-e2e` stack on http://localhost:18080 and run `scripts/e2e.sh`; the stack is left running. |
| `make e2e-browser` | Reuse the running, healthy e2e stack or boot one, then run Playwright in `web/e2e`; reuse does not rebuild, so rerun `make e2e` after code changes. |
| `make e2e-all` | Both suites, as CI runs them. |
| `scripts/e2e.sh down` | Tear the e2e stack down, including volumes. |
| `make evals EVALS_ARGS=...` | Run the eval runner (`go run ./evals/runner`) against a stack with a real LLM provider; needs `OBSERVE_SERVER` and `OBSERVE_TOKEN` (a service-account key) in the environment. |

Web-only commands run from `web/`: `bun run dev` (Vite on :5173, proxying `/api`, `/healthz` and `/readyz` to :8080), `bun run typecheck`, `bun run lint`, `bun run format`, `bun run test`.

## Area conventions

Claude Code loads a subdirectory `CLAUDE.md` only when it reads a file there.

- `internal/CLAUDE.md`: backend conventions for `internal/` and `cmd/`.
- `web/CLAUDE.md`: frontend conventions.
- `e2e/CLAUDE.md`: rules for both end-to-end suites; read it before editing `scripts/e2e.sh`, the demo data under `deploy/compose/` or `demo/shop`, or a Playwright spec in `web/e2e`.
- `pkg/` has no file of its own; its rules are under "pkg conventions" below.

## Layout

- `cmd/`: the `slashobserve-server` binary (its `healthcheck` subcommand backs the Docker health check) and the `observe` CLI, whose client code is in `internal/cli/`.
- `internal/`: server code, one package per feature; the packages with rules are listed in `internal/CLAUDE.md`.
- `pkg/spec/`: public ObserveSpec types, `Parse`, `Upgrade`, `Diff` and the query macro syntax.
- `pkg/grafana/`: the Grafana importer, with its golden corpus (`testdata/corpus/`) and option conformance catalogs (`testdata/conformance/`).
- `pkg/extension/`, `deploy/helm/`: planned, not created yet.
- `spec/`: `schema/observespec.v0.json` and `schema/defaults.json`, plus `examples/` (valid examples and `invalid/` fixtures with `expected.json`) shared by Go, TS and e2e tests.
- `api/`: the OpenAPI document for `/api/v1`; each feature adds `paths/<feature>.yaml` plus one `$ref` line in `openapi.yaml`.
- `web/`: the React app (Bun) with `src/routes`, `src/shell`, `src/components/ui` (shadcn) and `e2e` (Playwright).
- `e2e/`: `fixtures/api/` request bodies and `fixtures/llm/` replay fixtures.
- `install.sh`: the one-line `observe` CLI installer; `scripts/install-test.sh` tests it offline through `OBSERVE_RELEASES_URL`.
- `scripts/`: `e2e.sh` (e2e stack lifecycle and API smoke suite) and the release tag and notes scripts.
- `deploy/compose/`: `compose.yaml` (the one stack), `compose.build.yaml` (the override that builds images from the checkout) and the configs mounted into the stack's services.
- `demo/shop/`: its own Go module (`github.com/slashobserve/slashobserve/demo/shop`) with the `serve`, `loadgen` and `healthcheck` commands.
- `evals/`: eval scenarios, pricing and the runner.
- `tools/<tool>/go.mod`: pinned Go dev tools.
- `docs/`: the operator guide, one page per topic, grouped by folder.

## Conventions

- `spec/schema/observespec.v0.json` is the ObserveSpec source of truth and is never edited without Ishan's sign-off.
- `spec/schema/defaults.json` (`spec.Defaults()`) is read by Go and the web, and the web copy is generated.
  An absent value means its default, the importer drops values equal to it, and renderers read the same file.
- Generated files are committed and never hand-edited: `internal/db/dbsqlc`, the web Spec types and validator, and `web/src/api/schema.gen.ts`.
  CI runs `make generate` and fails when it changes a file.
- Prefer deep modules (small interface, real behavior behind it), accept dependencies instead of creating them, and add a seam only when two adapters exist.
- Write no docstrings or comments unless they state a constraint the code cannot show.
- Compose: add new services to `deploy/compose/compose.yaml` and publish host ports only for what a browser or developer must reach, bound to 127.0.0.1 with a `${...}` override.
  `compose.yaml` has no `build:` sections; builds belong in `compose.build.yaml`.

## pkg conventions

- `spec.Parse` is the only validation entry point and accepts only the evolved Spec shape.
  `spec.Upgrade` is the pure rewrite of the legacy v0 shape; use it for stored versions and fixtures.
- `pkg/grafana` is pure: no network, no database.
  A Grafana option is handled in this order: converted when the Spec has the field; else silent only when it equals a Grafana default (`grafanaDefaults`) or is listed in `cosmeticOptions` with the renderer behaviour that makes it invisible; else reported as a path.
  There are no silent drops.
- A converted option must change what the renderer draws.
  A Spec key that the renderer ignores is not a conversion, and the option stays reported.
- When Grafana's default differs from the Spec default, the importer writes Grafana's value whenever Grafana has one (stat `wideLayout`, for example).
- `decisions` and `excludedOptions` in `pkg/grafana/exclusions.go` are the one place that decides which options are excluded, and `propGates` in `pkg/grafana/fieldprops.go` is the one place that decides which panel types a field prop converts on.
- Canvas and field actions are built by one builder (`pkg/grafana/actions.go`) and validated by parsing them in a probe Spec dashboard, so the importer never repeats the credential, reference or address rules.
- The conformance catalogs hold Grafana option names and values only, because Grafana is AGPL-3.0.
  A file under `<area>/gaps/` fails the tests, and the `seed.txt` files are frozen history that no test reads, never edited.
- Goldens are reviewed diffs: run `make golden` after a change that alters conversions, read every changed golden, and say in the commit message which converter rule changed them.

## Dependencies

- Pin exact versions only, and only versions released at least 14 days earlier.
  `web/bunfig.toml` enforces both for npm packages; Go dependencies are exact in `go.mod`.
- `go.mod` has `ignore ./web` so `./...` skips the frontend's `node_modules`; keep it (requires Go 1.25 or newer).
- Dev tools run as `go tool -modfile=tools/<tool>/go.mod <tool>`; bump one with `go get -modfile=tools/<tool>/go.mod -tool <package>@<version>`; never run `go mod tidy` against a tool modfile.
- `mark3labs/mcp-go` serves `/mcp`, and only `internal/server/mcp` imports it.
- `anthropic-sdk-go` and `openai-go/v3` are the only model clients.
  Adapters keep provider history append-only and pass Anthropic responses back with `Message.ToParam()` so thinking blocks stay intact.
  The OpenAI-compatible adapter rebuilds assistant history with its own helper, so a tool call with a missing type still round-trips.
- GitHub Actions are pinned to a commit SHA with the version in a trailing comment; Docker images are pinned by tag and digest.
- Version bumps that must change together: Go toolchain in `go.mod` (`go` line), `demo/shop/go.mod`, and the `golang` image in `Dockerfile` and `demo/shop/Dockerfile`; the distroless image digest shared by both Dockerfiles; Bun in `web/package.json` (`packageManager`, read by CI) and the `oven/bun` image in `Dockerfile`; Postgres in `deploy/compose/compose.yaml` and in `internal/db/dbtest`; the `prom/prometheus` and `clickhouse/clickhouse-server` images in `deploy/compose/compose.yaml` and in the integration tests under `internal/datasource/{prometheus,clickhouse}`.
  Dependabot ignores the `golang` and `oven/bun` images because they move with those pins.
- Justify every new dependency in its PR.

## Product rules

- The agent only ever outputs ObserveSpec JSON, and its text output is discarded: only tool-saved Specs, or in suggest mode one tool-validated panel, leave a run.
  No code path takes LLM-written HTML or JS into the UI.
- Text panels render in `markdown`, `html` or `code` mode.
  HTML in markdown and html modes is sanitized by one schema (`web/src/panels/text/html.tsx`) before rendering, people may author it but the agent and MCP may not, variable values are escaped for the mode before parsing, and links and images are limited to http(s), mailto and same-origin URLs.
- The agent and MCP clients never create or edit a placeholder panel (they come from Grafana imports), write HTML into a text panel, or add or change a canvas element's or field's `actions`; `internal/agent/tools/importonly.go` rejects these.
- Datasource credentials never reach the browser.
  All panel data goes through the query proxy, which only runs queries from a saved Spec; the one exception is the draft preview described in `internal/CLAUDE.md`.
- Every query the agent runs is time-bounded and limited (series, rows, points, timeout) on the server, whatever the prompt says.
- Tools are defined once in `internal/agent/tools`; MCP and the built-in agent share them, except the MCP-only comment tools, `list_folders` and `get_spec_guide`.
  No tool deletes, moves or duplicates a dashboard; people do that in the web app or the HTTP API, and delete also with the CLI.
- Imports are the one tolerant first save: `POST /api/v1/imports` saves version 1 even when queries fail and records those panels and variables in `version.unverified` and the import report.
  Later saves run every query, and a failing panel or variable is kept only while the parent version lists it as unverified and it is unchanged.
  The agent is told never to delete an unverified panel unless asked.
- The original Grafana JSON is stored in `imports.original` and is never returned by the API or shown to the agent.
- Anything Enterprise will extend goes behind an interface in `pkg/extension`; core never imports Enterprise code.
- `razorpay/slashdocs` is a reference, not a dependency: never import its packages or copy whole files; credit any verbatim snippet in `NOTICE` under its MIT license.

## Workflow

- Make atomic commits: one self-contained change that builds and passes on its own; messages describe the change only, with no task numbers or co-author trailers.
- A user-visible change updates the `docs/` page that describes it in the same commit, because slashobserve.com builds its docs from `docs/` on `main`.
  Each page has `title` and `description` front matter and no `#` heading, and links other pages by relative `.md` path.
- A change that a user can see or call adds no new e2e check: workers write unit tests for pure logic only, and neither suite is extended unless the owner asks for a specific e2e test.
  Fixing an existing failing check is allowed.
- Do not commit ADRs, decision records or the brief; decisions live in the `CLAUDE.md` files, and implementation plans live outside the repository.
- Create repositories, push, and open PRs only after Ishan says go.
- CI branch protection must mark the `check` job as required, and `e2e` once that job is re-enabled.
- Markdown: one sentence per line; never use an em dash.
