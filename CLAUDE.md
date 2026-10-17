# SlashObserve

SlashObserve is an agentic Grafana replacement: an engineer types `/observe checkout health by pod, last 6h` and gets a live, Grafana-style dashboard, then refines it in plain language.
Every change is a saved version.
This repository is the Community Edition monorepo (Apache-2.0), Go module `github.com/slashobserve/slashobserve`.
The product brief lives outside this repository, at `../docs/BRIEF.md` in the SlashObserve workspace; read it for background, but this file carries every convention needed to work here.

## Commands

| Command | What it does |
| --- | --- |
| `make build` | Install web dependencies, build the Vite app into `internal/server/webdist`, then build `bin/slashobserve-server` with the UI embedded. |
| `make server` | Build only the Go binary, embedding whatever is in `internal/server/webdist`. |
| `make test` | `go test -race ./...` for the root module and `demo/shop`, then the web Vitest suite. |
| `make lint` | golangci-lint (config verify, format diff, run), `sqlc diff`, web typecheck, Spec drift and fixture parity check, web API type drift check (`api:check`), ESLint, Prettier check. |
| `make ci` | `lint`, `test`, `tidy-check` (`go mod tidy -diff` for the root module and `demo/shop`), `build`: what the CI `check` job runs. |
| `make generate` | Regenerate sqlc code, the web Spec types/validator and the web API types (`web/src/api/schema.gen.ts`) after editing migrations, queries, the schema or `api/`. |
| `make demo` / `make demo-down` | Run the full demo stack in `deploy/compose` and stop it: the app on http://localhost:8080, Prometheus on http://localhost:9090 and the shop fault API on http://localhost:8081, with Dex, Postgres, ClickHouse, the OTel Collector, node-exporter, cAdvisor and the shop behind them. |
| `make demo-reset` | Stop the demo stack and drop its volumes, so the 26 hour request backfill runs again on the next `make demo`. |
| `make demo-fault TYPE=errors\|latency PCT=N` | Inject a fault into the `checkout-0` pod through the shop fault API. |
| `make demo-fault-reset` | Clear the injected fault. |
| `make e2e` | Reset and boot the isolated `slashobserve-e2e` stack on http://localhost:18080 and run `scripts/e2e.sh`; the stack is left running. |
| `make e2e-browser` | Reuse the running, healthy e2e stack or boot one, then run Playwright in `web/e2e`; reuse does not rebuild, so rerun `make e2e` after code changes. |
| `make e2e-all` | Both suites, as CI runs them. |
| `scripts/e2e.sh down` | Tear the e2e stack down, including volumes. |

Web-only commands run from `web/`: `bun run dev` (Vite on :5173, proxying `/api`, `/healthz` and `/readyz` to :8080), `bun run typecheck`, `bun run lint`, `bun run format`, `bun run test`.

## Layout

Directories are created when their phase starts; this is where things go.

- `cmd/slashobserve-server/`: server binary (`healthcheck` subcommand backs the Docker health check).
  `cmd/observe/` (CLI) comes later.
- `internal/config/`: the only reader of `SLASHOBSERVE_*` environment variables.
- `internal/db/`: pgx pool, goose migrations (`migrations/`), sqlc queries (`queries/`) and generated code (`dbsqlc/`).
- `internal/auth/`: OIDC login, sessions, service-account keys and the auth middleware.
- `internal/server/`: Gin HTTP server, routes, embedded web build (`webdist/`), later the MCP endpoint.
- `internal/queryproxy/`: panel query runner shared by save validation and the proxy: kind by panel type, step from maxDataPoints, variable defaults, a 32-call upstream semaphore, one error-to-HTTP mapping.
- `internal/datasource/{prometheus,clickhouse,macros,provision}`: the datasource core and adapters, query macros, and registry construction from config.
- `internal/llm/`: provider-neutral gateway (`Provider` starts a `Session` that owns the native history); adapters `anthropic/`, `openaicompat/`, `replay/`; `providers/` picks one from configuration.
  `e2e/fixtures/llm/` holds the replay fixtures.
- `internal/dashboards/`: dashboard and version storage, the save validation pipeline and the `/api/v1/dashboards` routes (`Service.Register`).
- `internal/events/`: the event payload, the `slashobserve_events` Postgres channel name shared by publishers and the SSE stream, and the `Broker` that fans notifications out to subscribers; `internal/events/stream/` serves them over SSE.
- `internal/agent/tools/`: the only place agent tools are defined (14 tools, sorted, constant schemas); `Call` returns a tool error the model can fix or an internal error; ad-hoc queries run through the panel runner with tool limits.
- `internal/agent/prompts/`: embedded prompts, including the Spec authoring guide.
- Later: `pkg/extension`, `evals/`, `deploy/helm/`.
- `spec/schema/observespec.v0.json` is the ObserveSpec source of truth; `spec/examples/` holds valid examples and `invalid/` fixtures with `expected.json`, shared by Go, TS and e2e tests.
- `pkg/spec/` (public): hand-written types checked against the schema by a conformance test; `Parse` is the only validation entry point and returns `*ValidationError` with JSON-Pointer issues; `ScanQuery` and `Macros` define query reference syntax for the macros package; `Diff` returns a stable structural change list between two Specs (panels matched by `id`, variables by `name`) for version history.
- `api/`: the OpenAPI document for `/api/v1`: `openapi.yaml` holds the root, shared schemas and responses, and each feature adds `paths/<feature>.yaml` plus one `$ref` line in the root; `api.Load` embeds and validates it.
- `web/`: React app (Bun); `web/src/routes` (TanStack Router files), `web/src/components/ui` (shadcn), `web/e2e` (Playwright).
- `deploy/compose/compose.yaml`: the one compose stack; `scripts/e2e.sh`: e2e stack lifecycle and API smoke suite.
- `demo/shop/`: its own Go module (`github.com/slashobserve/slashobserve/demo/shop`) with the `serve`, `loadgen` and `healthcheck` commands; it emits the demo metrics, logs and traces.
- `deploy/compose/{prometheus,clickhouse,otelcol}/`: the configs mounted into the demo stack's Prometheus, ClickHouse and OTel Collector.
- `tools/<tool>/go.mod`: pinned Go dev tools (sqlc, golangci-lint).

## Backend conventions

- Configuration: every setting is an environment variable named `SLASHOBSERVE_*`, parsed and validated only in `internal/config` (`config.Load`).
  Current settings: `SLASHOBSERVE_HTTP_ADDR` (default `:8080`), `SLASHOBSERVE_DATABASE_URL` (required), `SLASHOBSERVE_LOG_LEVEL` (`debug|info|warn|error`, default `info`), `SLASHOBSERVE_LOG_FORMAT` (`json|text`, default `json`).
  Auth settings: `SLASHOBSERVE_PUBLIC_URL` (required; http or https origin with no path, query or user info), `SLASHOBSERVE_OIDC_ISSUER_URL` (required; used exactly as given because issuer comparison is exact), `SLASHOBSERVE_OIDC_INTERNAL_URL` (optional origin the server's back-channel uses instead of the issuer's scheme and host), `SLASHOBSERVE_OIDC_CLIENT_ID` and `SLASHOBSERVE_OIDC_CLIENT_SECRET` (required), `SLASHOBSERVE_SESSION_TTL` (default `168h`, between `5m` and `720h`), and `SLASHOBSERVE_AUTH_ALLOWED_EMAIL_DOMAINS`.
  `SLASHOBSERVE_AUTH_ALLOWED_EMAIL_DOMAINS` is required: a comma-separated list of domains matched exactly against the verified email's domain, or `*` as an explicit allow-all that logs a startup warning.
  `SLASHOBSERVE_DATASOURCES_FILE` is optional: a YAML file listing datasources, and unset means none.
  The file never holds a secret: passwords and tokens are referenced by `passwordEnv`, `bearerTokenEnv` or `headersFromEnv` (a map of header name to variable name) names that start with `SLASHOBSERVE_`, or by `passwordFile`, `bearerTokenFile` or `headersFromFile` paths.
  An env name the server's own config reads (such as `SLASHOBSERVE_DATABASE_URL`) is refused, and a header name may appear in only one of `headers`, `headersFromEnv` and `headersFromFile`.
  The file needs a top-level `datasources` key and one YAML document; unknown fields are rejected, and `deploy/compose/datasources.yaml` is the exemplar.
  An empty list logs a startup warning.
  LLM settings are all optional: `SLASHOBSERVE_LLM_PROVIDER` (`anthropic|openai|replay`; unset means no agent, so the revision and observe endpoints answer 503 `agent_unavailable`) and `SLASHOBSERVE_LLM_MODEL` (default `claude-sonnet-5-5` for `anthropic`, required for `openai`, ignored by `replay`).
  `SLASHOBSERVE_LLM_API_KEY` or `SLASHOBSERVE_LLM_API_KEY_FILE` (at most one; required for `anthropic`, optional for `openai`) supplies the credential, and `SLASHOBSERVE_LLM_BASE_URL` (absolute http or https URL, a path is allowed) overrides the provider endpoint.
  `SLASHOBSERVE_LLM_EFFORT` (`low|medium|high|xhigh|max`, default `medium`, Anthropic only), `SLASHOBSERVE_LLM_MAX_TOKENS` (`1024` to `64000`, default `16000`) and `SLASHOBSERVE_LLM_REQUEST_TIMEOUT` (`10s` to `10m`, default `120s`) bound each model call.
  `SLASHOBSERVE_LLM_REPLAY_DIR` is required for `replay`.
  The LLM API key is never logged or echoed in an error.
  Never log secret values, the database URL and the OIDC client secret included.
  `SLASHOBSERVE_API_CONTRACT_CHECK` (`true|false`, default `false`) is for the e2e stack only: it validates every `/api/v1` request and response against `api/openapi.yaml`, and never belongs in production.
- HTTP: Gin, assembled in `internal/server.New`.
  JSON API under `/api/v1`; `/healthz` is liveness with no dependencies; `/readyz` is readiness (database ping and no pending migrations).
  Every other GET is the SPA: build files from the embedded `webdist`, `/assets/*` cached as immutable, unknown `/api/*` paths get a JSON 404, missing `/assets/*` files and missing top-level names with an extension (like `/robots.txt`) get 404, and every other path gets `index.html`.
  Responses forbid framing (`X-Frame-Options: DENY`, `Content-Security-Policy: frame-ancestors 'none'`).
- Auth: every `/api/v1` route goes on the group returned by `auth.Service.Mount` except `/api/v1/auth/*`.
  Handlers read the caller with `auth.FromContext`.
  Sessions and keys are opaque random tokens stored only as SHA-256.
  Cookie-authenticated mutations pass `http.CrossOriginProtection`.
  Errors are `{"error","code"}` and 401s carry `WWW-Authenticate: Bearer`.
  The issuer is the browser-facing URL, and `SLASHOBSERVE_OIDC_INTERNAL_URL` only rewrites the server's back-channel host.
- Database: Postgres through a pgx v5 pool.
  Migrations are goose SQL files `internal/db/migrations/NNNNN_snake_case.sql` with `-- +goose Up` and `-- +goose Down`, numbered sequentially; never edit a merged migration.
  The server applies migrations at boot under a Postgres advisory lock.
  Queries live in `internal/db/queries`; sqlc generates `internal/db/dbsqlc` (committed, never hand-edited; run `make generate`).
- Dashboards: versions are immutable and linear (`parent_version = version - 1`); a save validates with `spec.Parse`, then runs every panel query through `internal/queryproxy`, outside any transaction; the conditional `latest_version` update is the concurrency check (409 `version_conflict`); events go out with `pg_notify` on channel `slashobserve_events` inside the saving transaction.
- Events: writers `pg_notify` on `slashobserve_events` inside their transaction; one `events.Broker` per process listens and fans out to `GET /api/v1/dashboards/{id}/events` (SSE) per dashboard; every stream starts with `ready` (current `latest_version`), sends `resync` after a listener reconnect, pings every 15 s, ends after 30 minutes, and clients refetch on `ready` and `resync` instead of replaying ids.
- Datasources: adapters live in `internal/datasource/{prometheus,clickhouse}` behind `datasource.Datasource`, and `internal/datasource/provision` builds the registry from `config.Datasources`.
  Variable values reach queries only through `internal/datasource/macros`.
  ClickHouse is queried only as a `readonly = 2` user with `SELECT` on the OTel database, and the server refuses a user that is not read-only.
  `GET /api/v1/datasources` returns health only, never a URL, address, user or secret, and datasources never gate `/readyz`.
- Logging: `log/slog`, JSON by default; request logs record the path, never the query string.
- API contract: every `/api/v1` route is documented in `api/`.
  The e2e stack runs with `SLASHOBSERVE_API_CONTRACT_CHECK=true`, which refuses to boot on undocumented routes (or documented operations with no route) and turns any request or response that breaks the document into a 500 `{"error":"api contract violation","code":"contract_violation"}`.
  A request that breaks the document is allowed only when the handler refuses it with a 4xx.
  `scripts/e2e.sh` fails when the server log contains `api contract violation`.
  Document every status a handler can return, and keep `additionalProperties: false` on response objects so new fields must be documented.
  Wire conventions for `/api/v1`: JSON field names are `snake_case` (the Spec document inside a response keeps its camelCase), times are RFC 3339 UTC strings, errors are `{"error","code"}` plus optional fields named per endpoint, and lists use opaque `cursor` and `next_cursor` (null at the end) with `limit`.
- Design: prefer deep modules (small interface, real behavior behind it); accept dependencies instead of creating them; add a seam only when two adapters exist.
- ClickHouse in compose has a read-only user `slashobserve_ro`, created on first boot by `deploy/compose/clickhouse/initdb/10-readonly-user.sh` from `internal/datasource/clickhouse/testdata/readonly_user.sql` (`readonly=2` with constraints), the same definition the integration tests use.
- Integration tests run against real Postgres, Prometheus and ClickHouse through testcontainers, never mocks.
- Write no docstrings or comments unless they state a constraint the code cannot show.

## Frontend conventions

- Bun is the package manager and script runtime; `web/bunfig.toml` forces the Bun runtime for every `bun run` script.
  Node and pnpm are not used.
- React 19, TypeScript strict, Vite, Tailwind CSS v4, shadcn/ui on Base UI (`components.json` style `base-nova`), TanStack Router with file-based routes.
- Add shadcn components with `bun x shadcn add <name>` from `web/`; they live in `web/src/components/ui`.
- `web/src/routeTree.gen.ts` is generated by the Vite plugin on `dev` and `build`; commit it, never edit it.
- `web/src/spec/*.gen.ts` are generated from the schema by `make generate` (commit them, never edit); import only from `web/src/spec/index.ts`, and keep that folder free of app imports so it can become the `@slashobserve/spec` package.
- `web/src/api/schema.gen.ts` is generated from `api/openapi.yaml` by `make generate` (commit it, never edit); `make lint` fails when it is stale.
- Formatting follows shadcn's Prettier style (no semicolons, double quotes); `bun run format` before committing.
- Installed stack: TanStack Query, openapi-fetch over openapi-typescript types, Zustand, Sonner, uPlot, react-grid-layout, CodeMirror 6 (`@codemirror/*` with `@prometheus-io/codemirror-promql` and `@codemirror/lang-sql`), Ajv, lucide-react and Vitest for pure helpers (`*.test.ts` next to the code, run by `bun run test` and `make test`).
  Testing Library and cmdk are not installed yet.
- Server data goes through TanStack Query with keys from `web/src/api/keys.ts` only; the typed client is `api` from `web/src/api/client.ts` (openapi-fetch over `web/src/api/schema.gen.ts`), and responses are unwrapped with `unwrap`, which throws `ApiError`.
- Zustand holds UI-only state (theme, unsaved layout edits); never copy server data into a store.
- The router uses plain `URLSearchParams` search serialization (`web/src/lib/search.ts`): search values are strings, numbers or string arrays, repeated keys become arrays, and each route coerces types in `validateSearch`.
- Theme preference lives in `localStorage` key `slashobserve-theme` and is applied before first paint by `web/public/theme-init.js`; chart colors use the `--series-1..8` and `--threshold-*` tokens in `web/src/index.css`, never ad-hoc hex values.
- Visual defaults to avoid: cream or off-white page backgrounds, italic accent words in headings, numbered `01/02` section labels, monospace for UI labels (monospace is only for query text, JSON values and ids), pill-shaped buttons, gradients, heavy drop shadows, colored left-border callouts, uppercase eyebrow labels, emoji, and chart animations.

## Dependencies

- Pin exact versions only, and only versions released at least 14 days earlier.
  `web/bunfig.toml` enforces both for npm packages (`exact = true`, `minimumReleaseAge = 1209600` seconds).
- Go dependencies are exact in `go.mod`.
  `go.mod` has `ignore ./web` so `./...` skips the frontend's `node_modules`; keep it (requires Go 1.25 or newer).
  Dev tools run as `go tool -modfile=tools/<tool>/go.mod <tool>`; bump one with `go get -modfile=tools/<tool>/go.mod -tool <package>@<version>`; never run `go mod tidy` against a tool modfile.
- LLM SDKs: `anthropic-sdk-go` and `openai-go/v3` are the only model clients; adapters keep provider history append-only and pass Anthropic responses back with `Message.ToParam()` so thinking blocks stay intact.
  The OpenAI-compatible adapter rebuilds assistant history with its own helper instead of `ToParam`, so a tool call with a missing type still round-trips.
- GitHub Actions are pinned to a commit SHA with the version in a trailing comment; Docker images are pinned by tag and digest.
- Version bumps that must change together: Go toolchain in `go.mod` (`go` line), `demo/shop/go.mod`, and the `golang` image in `Dockerfile` and `demo/shop/Dockerfile`; the distroless image digest shared by both Dockerfiles; Bun in `web/package.json` (`packageManager`, read by CI) and the `oven/bun` image in `Dockerfile`; Postgres in `deploy/compose/compose.yaml`; the `prom/prometheus` and `clickhouse/clickhouse-server` images in `deploy/compose/compose.yaml` and in the integration tests under `internal/datasource/{prometheus,clickhouse}`.
- Prefer boring, proven libraries; justify every new dependency in its PR.

## End-to-end tests

- Two suites, both required to merge: `scripts/e2e.sh`, curl-driven checks against the compose stack (API today; CLI and MCP as they land), and Playwright in `web/e2e` (Chromium, one worker, retries only in CI).
- Playwright specs import `test` and `expect` from `web/e2e/fixtures.ts`, which fails any test whose page logs a console error.
- The Playwright `setup` project (`web/e2e/auth.setup.ts`) signs in through Dex once and saves `web/e2e/.auth/user.json`.
  Every `chromium` spec reuses that session, and a spec that signs out must opt out with an empty `storageState` and sign in itself.
- The e2e stack is compose project `slashobserve-e2e`, reset with `down -v` on every `make e2e`, on host port 18080 (`SLASHOBSERVE_E2E_PORT`), so it never touches a developer's `make demo` stack.
  `scripts/e2e.sh` run, up, browser and down take a run lock, the directory `${TMPDIR:-/tmp}/slashobserve-e2e-run.lock`, so two invocations never reset the same stack; a second one fails at once with "another e2e run is active".
  The EXIT trap removes the lock; after a kill -9 or a crash, `rmdir` that directory to clear a stale one.
  The e2e stack also publishes Prometheus on 19090 (`SLASHOBSERVE_E2E_PROMETHEUS_PORT`) and the shop fault API on 18081 (`SLASHOBSERVE_E2E_SHOP_PORT`).
  CI builds the server image once as `slashobserve-e2e:ci` and the shop image as `slashobserve-demo-shop:ci` (`SLASHOBSERVE_SHOP_IMAGE`), and uploads Playwright traces, videos, the HTML report and compose logs when a test fails.
- `scripts/e2e.sh` boot waits, with bounded polling, until Prometheus has scraped every target and ClickHouse has log and trace rows, so later checks start on a stack that is already exporting; checks on telemetry that may still be arriving poll instead of asserting once.
  The shop contract (pods, routes, metric labels) is shared by the Prometheus backfill and the e2e checks, so change them together.
- Dex runs in compose on host port `SLASHOBSERVE_DEX_HOST_PORT` (5556 for `make demo`, 15556 for e2e via `SLASHOBSERVE_E2E_DEX_PORT`).
  Dev logins are `admin@example.com` / `password` (allowed domain `example.com`) and `outsider@example.org` / `password` (always refused, for tests).
  `scripts/e2e.sh` has `dex_login` for curl-driven sign-in.
- Every change that a user can see or call adds or updates an e2e check in the same PR; a feature is not done until both suites cover it.
- `scripts/e2e.sh` `api_token` creates the service account `e2e-api` and sets `API_TOKEN` once, so API checks send `Authorization: Bearer $API_TOKEN`; request bodies live in `e2e/fixtures/api/`.
- E2E tests never call a live model: they use the replay LLM provider (`SLASHOBSERVE_LLM_PROVIDER=replay`, fixtures in `e2e/fixtures/llm/`).
  A fixture is one JSON file per prompt with `prompt` and `turns` (`text`, `tool_calls`, `expect`); a tool error not declared in `expect` fails the run.
- Compose: add new services (Dex, Prometheus, ClickHouse, OTel Collector, demo/shop) to `deploy/compose/compose.yaml`; publish host ports only for what a browser or developer must reach, bound to 127.0.0.1 with a `${...}` override.
  Only `frontend-0` builds the shop image (`pull_policy: build`), so a plain `docker compose up` rebuilds the image named by `SLASHOBSERVE_SHOP_IMAGE` even when it is prebuilt; pass `--no-build` to keep it.

## Product rules

- The agent only ever outputs ObserveSpec JSON; no code path takes LLM-written HTML or JS into the UI.
- Datasource credentials never reach the browser; all panel data goes through the query proxy, which only runs queries from a saved Spec.
- Every query the agent runs is time-bounded and limited (series, rows, points, timeout) on the server, whatever the prompt says.
- Tools are defined once in `internal/agent/tools`; MCP and the built-in agent share them.
- Anything Enterprise will extend goes behind an interface in `pkg/extension`; core never imports Enterprise code.
- `razorpay/slashdocs` is a reference, not a dependency: never import its packages or copy whole files; credit any verbatim snippet in `NOTICE` under its MIT license.

## Workflow

- Make atomic commits: one self-contained change that builds and passes on its own; messages describe the change only, with no task numbers or co-author trailers.
- Do not commit ADRs, decision records or the brief; decisions live in this file, and implementation plans live outside the repository.
- Create repositories, push, and open PRs only after Ishan says go.
- CI branch protection must mark the `check` and `e2e` jobs as required.
- Markdown: one sentence per line; never use an em dash.
