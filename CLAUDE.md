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
| `make test` | `go test -race ./...` for the root module and `demo`, the web Vitest suite (`COVER=1` adds Go and web coverage), then `scripts/release-scripts-test.sh` (the release tag and release notes scripts), `scripts/install-test.sh` (the root `install.sh`, against fake release assets) and `scripts/docs-lint-test.sh` (the docs lint rules, against fixture pages). |
| `make lint` | golangci-lint (config verify, format diff, run), `sqlc diff`, web typecheck, Spec drift and fixture parity check, web API type drift check (`api:check`), ESLint, knip (unused web files, exports and dependencies), Prettier check, `scripts/docs-lint.sh` on the docs pages, and `scripts/helm-check.sh` (Helm lint and renders of the chart, no Secret in them, and its internals dashboard equal to the compose copy). |
| `make ci` | `lint`, `test`, `tidy-check` (`go mod tidy -diff` for the root module and `demo`), `build`: what the CI `check` job runs. |
| `make audit` | `govulncheck` for the root module and `demo`, `bun audit --prod` and `bun audit --audit-level=high` for the web lockfile (minus the advisories in `WEB_AUDIT_IGNORE`): what the CI `audit` job runs. |
| `make generate` | Regenerate sqlc code, the web Spec types/validator and the web API types (`web/src/api/schema.gen.ts`) after editing migrations, queries, the schema or `api/`. |
| `make golden` | Rewrite the `pkg/grafana` corpus goldens and the generated option map (`optionmap.json`). |
| `make demo` / `make demo-down` | Run the full demo stack in `deploy/compose` from the published images `ghcr.io/slashobserve/slashobserve` and `ghcr.io/slashobserve/demo-shop` (tag `latest`, overridable with `SLASHOBSERVE_IMAGE` and `SLASHOBSERVE_SHOP_IMAGE`) with no compile, and stop it; it cannot pull until the first release tag is published, so use `make demo-build` until then. The stack: the app on http://localhost:8080, Prometheus on http://localhost:9090 and the shop fault API on http://localhost:8081, with Dex, Postgres, ClickHouse, the OTel Collector, node-exporter, cAdvisor and the shop behind them. |
| `make demo-build` | The same stack built from this checkout (`deploy/compose/compose.build.yaml` on top of `compose.yaml`, images `slashobserve-server:local` and `slashobserve-demo-shop:local`); the path for contributors. |
| `make demo-search` / `make demo-search-down` | `make demo-build` with the `search` compose profile: it adds Elasticsearch 8 (http://localhost:9200), OpenSearch 2 (http://localhost:9201, security off in both) and `otel-collector-search`, which the main collector feeds with the shop's logs; the `es` and `os` datasources in `datasources.yaml` answer only then. The two images take about 3.5 GB on disk and each store is capped at 1 GiB of memory. `demo-search-down` stops only those three services. |
| `make demo-reset` | Stop the demo stack, search profile included, and drop its volumes, so the 26 hour request and latency backfill runs again on the next `make demo`. |
| `make demo-fault TYPE=errors\|latency PCT=N` | Inject a fault into the `checkout-0` pod through the shop fault API. |
| `make demo-fault-reset` | Clear the injected fault. |
| `make demo-k8s` / `make demo-k8s-down` | Create the local three-node kind cluster `slashobserve-demo` (the shop in the `default` namespace, kube-state-metrics, node-exporter and a Prometheus with the kube-prometheus recording rules; Prometheus on http://localhost:39090, the `checkout-0` and `checkout-1` fault APIs on :38081 and :38082), and delete only that cluster. `scripts/demo-k8s.sh` also has `status`, `fault <pod> <type> <pct>` and `connect <container>`; kind runs as a pinned Go tool and kubectl inside the control-plane node, so neither needs installing. It runs beside the compose demo and never touches it. |
| `make helm-kind` | Create the local one-node kind cluster `slashobserve-helm` if absent, build `slashobserve-server:helm` from this checkout and load it, deploy the throwaway Postgres and Prometheus of `deploy/helm/kind`, create generated Secrets (never printed) and install the chart, then check `/readyz`, `/metrics`, the Prometheus scrape and the provisioned internals dashboard, and upgrade and roll back the release. kind and Helm run as pinned Go tools and kubectl inside the node. |
| `make helm-kind-down` | Delete only the `slashobserve-helm` cluster. |
| `make e2e` | Reset and boot the isolated `slashobserve-e2e` stack on http://localhost:18080 and run `scripts/e2e.sh`; the stack is left running. |
| `make e2e-browser` | Reuse the running, healthy e2e stack or boot one, then run Playwright in `web/e2e`; reuse does not rebuild, so rerun `make e2e` after code changes. |
| `make e2e-all` | Both suites, as CI runs them. |
| `scripts/e2e.sh down` | Tear the e2e stack down, including volumes. |
| `make evals EVALS_ARGS=...` | Run the eval runner (`go run ./evals/runner`) against a stack with a real LLM provider; needs `OBSERVE_SERVER` and `OBSERVE_TOKEN` (a service-account key) in the environment. |
| `make evals-stack-up [EVALS_STACK_MODE=replay\|<env-file>]` | Generate the replay fixtures and synthetic series, then boot the isolated `slashobserve-evals` stack on http://localhost:28080 (Prometheus on :29090, the `checkout-0` and `checkout-1` fault APIs on :28081 and :28082); the default `replay` mode never reads `deploy/compose/.env`, and an env file path boots it on the provider settings of that file; it returns once Prometheus serves the node, shop and synthetic series the scenarios read, about 3 minutes after the containers are healthy, once the synthetic series hold the history a retime step needs. |
| `make evals-stack-down` | Remove the `slashobserve-evals` containers and drop its volumes only. |
| `scripts/evals-stack.sh up --k8s replay` | Boot the evals stack against the running Kubernetes demo: it adds the `prom_k8s` datasource, joins the server to the cluster's docker network and serves no synthetic series. Then `go run ./evals/runner --verify-k8s --server http://localhost:28080 --bootstrap-dex` imports the 29 Kubernetes corpus dashboards and compares their unverified panels with `evals/k8s-gaps.yaml`; `--profile k8s` runs the live suite on them. |
| `make evals-replay` | Boot the replay stack and run `--suite replay --bootstrap-dex --min-pass 1` against it with no key or model: what the CI `evals-replay` job runs. |
| `make evals-live [EVALS_MATRIX=evals/matrix.yaml] [EVALS_REPORT_DIR=evals/reports/live]` | Run the live suite once per model of the git-ignored matrix file (see `evals/matrix.example.yaml`) against a running evals stack, recreating only the server between models; spends tokens, so it runs by hand and CI never runs it. |

Web-only commands run from `web/`: `bun run dev` (Vite on :5173, proxying `/api`, `/healthz` and `/readyz` to :8080), `bun run typecheck`, `bun run lint`, `bun run format`, `bun run test`.

## Area conventions

Claude Code loads a subdirectory `CLAUDE.md` only when it reads a file there.

- `internal/CLAUDE.md`: backend conventions for `internal/` and `cmd/`.
- `web/CLAUDE.md`: frontend conventions.
- `e2e/CLAUDE.md`: rules for both end-to-end suites; read it before editing `scripts/e2e.sh`, the demo data under `deploy/compose/` or `demo`, or a Playwright spec in `web/e2e`.
- `pkg/` has no file of its own; its rules are under "pkg conventions" below.

## Layout

- `cmd/`: the `slashobserve-server` binary (its `healthcheck` subcommand backs the Docker health check) and the `observe` CLI, whose client code is in `internal/cli/`.
- `internal/`: server code, one package per feature; the packages with rules are listed in `internal/CLAUDE.md`.
- `pkg/spec/`: public ObserveSpec types, `Parse`, `Upgrade`, `Diff` and the query macro syntax.
- `pkg/grafana/`: the Grafana importer, with its golden corpus (`testdata/corpus/`) and option conformance catalogs (`testdata/conformance/`).
- `pkg/extension/`: planned, not created yet.
- `deploy/helm/`: `slashobserve/` (the Helm chart: an external Postgres, every secret by Secret reference, and its own copy of the internals dashboard) and `kind/` (the cluster config, throwaway Postgres and Prometheus, and values that `scripts/helm-kind.sh` uses).
- `spec/`: `schema/observespec.v0.json` and `schema/defaults.json`, plus `examples/` (valid examples and `invalid/` fixtures with `expected.json`) shared by Go, TS and e2e tests.
- `api/`: the OpenAPI document for `/api/v1`; each feature adds `paths/<feature>.yaml` plus one `$ref` line in `openapi.yaml`.
- `web/`: the React app (Bun) with `src/routes`, `src/shell`, `src/components/ui` (shadcn) and `e2e` (Playwright).
- `e2e/`: `fixtures/api/` request bodies and `fixtures/llm/` replay fixtures.
- `install.sh`: the one-line `observe` CLI installer; `scripts/install-test.sh` tests it offline through `OBSERVE_RELEASES_URL`.
- `scripts/`: `e2e.sh` (e2e stack lifecycle and API smoke suite), `evals-stack.sh` (lifecycle of the isolated `slashobserve-evals` stack: `up [--k8s] replay|<env-file>`, `server [--k8s] <env-file>`, `down`, `status`), `demo-k8s.sh` (lifecycle of the Kubernetes demo cluster) and the release tag and notes scripts.
- `deploy/compose/`: `compose.yaml` (the one stack), `compose.build.yaml` (the override that builds images from the checkout), `compose.evals.yaml` (the override for the `slashobserve-evals` stack: replay fixtures, the `synth` series exporter, the `checkout-1` fault port and the LLM tuning variables), `datasources.evals-k8s.yaml` (the datasources of `up --k8s`) and the configs mounted into the stack's services.
- `demo/`: its own Go module (`github.com/slashobserve/slashobserve/demo`) with the `serve`, `loadgen`, `synth`, `keygen` and `healthcheck` commands; `demo/k8s/` holds the kind config, the manifests and the recording rules of the Kubernetes demo.
- `evals/`: eval scenarios, the live corpus sample (`corpus-sample.yaml`), the Kubernetes dashboards' known empty panels (`k8s-gaps.yaml`), the model matrix example and the runner; `go run ./evals/runner prepare` generates the replay fixtures and synthetic series the evals stack mounts.
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
- `spec.ApplyPanelPatch` is the only merge of a panel patch into a dashboard, and it ends in `Parse`, so a patch adds no second copy of a format rule.
  A placement's `gridPos` stays raw JSON and is checked against the schema's `gridPos` before the merge, so a missing field is reported as required at `/layout/N/gridPos`.
  An overlap it finds is reported at the panel the patch upserted or placed, so `failing_panels` names what the caller changed; plain `Parse` keeps document order.
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
  Adapters stream, retry in one place (`llm.Retry`, the SDKs' retries off) and keep provider history append-only except `Session.Compact`; request-side pruning changes only what is sent.
  They pass Anthropic responses back with `Message.ToParam()` so thinking blocks stay intact (compaction drops thinking only from older kept turns).
  The OpenAI-compatible adapter rebuilds assistant history with its own helper, so a tool call with a missing type still round-trips.
- GitHub Actions are pinned to a commit SHA with the version in a trailing comment; Docker images are pinned by tag and digest.
- Version bumps that must change together: Go toolchain in `go.mod` (`go` line), `demo/go.mod`, and the `golang` image in `Dockerfile` and `demo/Dockerfile`; the distroless image digest shared by both Dockerfiles; Bun in `web/package.json` (`packageManager`, read by CI) and the `oven/bun` image in `Dockerfile`; Postgres in `deploy/compose/compose.yaml`, in `internal/db/dbtest` and in `deploy/helm/kind/postgres.yaml`; the `prom/prometheus`, `clickhouse/clickhouse-server`, `grafana/loki`, `grafana/tempo`, `elasticsearch` and `opensearch` images in `deploy/compose/compose.yaml` and in the integration tests under `internal/datasource/{prometheus,clickhouse,loki,tempo,elasticsearch}`, and `prom/prometheus` also in `deploy/helm/kind/prometheus.yaml`; the `kindest/node` image in `demo/k8s/kind.yaml` and `deploy/helm/kind/kind.yaml`.
  Dependabot ignores the `golang` and `oven/bun` images because they move with those pins.
- Justify every new dependency in its PR.

## Product rules

- The agent only ever outputs ObserveSpec JSON, and its text output is discarded: only tool-saved Specs, or in suggest mode one tool-validated panel, leave a run.
  No code path takes LLM-written HTML or JS into the UI.
  The agent outputs ObserveSpec JSON or, in an investigate run or a chat request that only asks a question, a tool-saved finding; nothing else leaves a run.
- The agent saves into a build one panel per save, and only the finished build becomes a version, one per request.
  Tool-saved Specs still are the only agent output that leaves a run; the agent's planned panel count is an integer and may be shown.
- A build is visible only to the person who made the request, and for a revision only while they can still view the dashboard.
- Text panels render in `markdown`, `html` or `code` mode.
  HTML in markdown and html modes is sanitized by one schema (`web/src/panels/text/html.tsx`) before rendering, people may author it but the agent and MCP may not, variable values are escaped for the mode before parsing, and links and images are limited to http(s), mailto and same-origin URLs.
- The agent and MCP clients never create or edit a placeholder panel (they come from Grafana imports), write HTML into a text panel, or add or change a canvas element's or field's `actions`; `internal/agent/tools/importonly.go` rejects these.
- Datasource credentials never reach the browser.
  All panel data goes through the query proxy, which only runs queries from a saved Spec; the five exceptions are the draft preview, the requester's own build under `GET /api/v1/observe/{id}/build/...`, the explore query `POST /api/v1/datasources/{uid}/query` behind the logs page and trace view, a finding's evidence under `GET /api/v1/findings/{id}/panels/{panel_id}/data`, and a public link's read-only view under `GET /api/v1/public/{token}/...` (`internal/queryproxy/proxyhttp/public.go`), all described in `internal/CLAUDE.md`.
- A public link is a person's decision: no agent or MCP tool creates, lists or revokes one.
- Every query the agent runs is time-bounded and limited (series, rows, points, timeout) on the server, whatever the prompt says.
- Tools are defined once in `internal/agent/tools`, with one description per tool and per parameter; MCP and the built-in agent read the same text except the four named variants in `wording.go` (`guide`, `folder`, `run target`, `build`), and `TestSurfacesDifferOnlyByNamedVariants` fails on any other difference; the comment tools, `list_folders`, `create_finding`, `list_dashboard_health`, `create_annotation`, `list_activity` and `draft_query` are MCP-only, `list_annotations` reaches the agent only in investigate runs, and `get_spec_guide` returns only the JSON Schema inside a run, because the guide and example are in the agent's system text.
  No tool deletes, moves or duplicates a dashboard; people do that in the web app or the HTTP API, and delete also with the CLI.
- The authoring guide is `internal/agent/prompts/spec_guide.md` plus one `guide/<type>.md` section per datasource type; `prompts.GuideFor` is the only place that picks sections, and a run's system text and `get_spec_guide` carry only the sections of the configured types.
- Imports are the one tolerant first save: `POST /api/v1/imports` saves version 1 even when queries fail and records those panels and variables in `version.unverified` and the import report.
  Later saves run every query, and a failing panel or variable is kept only while the parent version lists it as unverified and it is unchanged.
  `update_panels` is the exception: it runs only the changed panels and variables, but every query when it changes the dashboard's default `time`, because each check depends on that range.
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
- CI branch protection must mark the `check` job as required, and `e2e-api` and the four `e2e-browser` shards once those jobs are re-enabled.
- Markdown: one sentence per line; never use an em dash.
