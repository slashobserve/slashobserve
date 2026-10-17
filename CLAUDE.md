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
| `make test` | `go test -race ./...`. |
| `make lint` | golangci-lint (config verify, format diff, run), `sqlc diff`, web typecheck, Spec drift and fixture parity check, ESLint, Prettier check. |
| `make ci` | `lint`, `test`, `build`: what the CI `check` job runs. |
| `make generate` | Regenerate sqlc code and the web Spec types/validator after editing migrations, queries or the schema. |
| `make demo` / `make demo-down` | Run the compose stack in `deploy/compose` on http://localhost:8080. |
| `make e2e` | Reset and boot the isolated `slashobserve-e2e` stack on http://localhost:18080 and run `scripts/e2e.sh`; the stack is left running. |
| `make e2e-browser` | Reuse the running, healthy e2e stack or boot one, then run Playwright in `web/e2e`; reuse does not rebuild, so rerun `make e2e` after code changes. |
| `make e2e-all` | Both suites, as CI runs them. |
| `scripts/e2e.sh down` | Tear the e2e stack down, including volumes. |

Web-only commands run from `web/`: `bun run dev` (Vite on :5173, proxying `/api`, `/healthz` and `/readyz` to :8080), `bun run typecheck`, `bun run lint`, `bun run format`.

## Layout

Directories are created when their phase starts; this is where things go.

- `cmd/slashobserve-server/`: server binary (`healthcheck` subcommand backs the Docker health check).
  `cmd/observe/` (CLI) comes later.
- `internal/config/`: the only reader of `SLASHOBSERVE_*` environment variables.
- `internal/db/`: pgx pool, goose migrations (`migrations/`), sqlc queries (`queries/`) and generated code (`dbsqlc/`).
- `internal/server/`: Gin HTTP server, routes, embedded web build (`webdist/`), later the MCP endpoint.
- `internal/queryproxy/`: panel query runner shared by save validation and the proxy: kind by panel type, step from maxDataPoints, variable defaults, a 32-call upstream semaphore, one error-to-HTTP mapping.
- Later: `internal/auth`, `internal/dashboards`, `internal/datasource/{prometheus,clickhouse,macros}`, `internal/llm`, `internal/agent/{prompts,tools}`, `pkg/extension`, `e2e/fixtures/llm`, `evals/`, `demo/shop/`, `deploy/helm/`.
- `spec/schema/observespec.v0.json` is the ObserveSpec source of truth; `spec/examples/` holds valid examples and `invalid/` fixtures with `expected.json`, shared by Go, TS and e2e tests.
- `pkg/spec/` (public): hand-written types checked against the schema by a conformance test; `Parse` is the only validation entry point and returns `*ValidationError` with JSON-Pointer issues; `ScanQuery` and `Macros` define query reference syntax for the macros package; `Diff` returns a stable structural change list between two Specs (panels matched by `id`, variables by `name`) for version history.
- `web/`: React app (Bun); `web/src/routes` (TanStack Router files), `web/src/components/ui` (shadcn), `web/e2e` (Playwright).
- `deploy/compose/compose.yaml`: the one compose stack; `scripts/e2e.sh`: e2e stack lifecycle and API smoke suite.
- `tools/<tool>/go.mod`: pinned Go dev tools (sqlc, golangci-lint).

## Backend conventions

- Configuration: every setting is an environment variable named `SLASHOBSERVE_*`, parsed and validated only in `internal/config` (`config.Load`).
  Current settings: `SLASHOBSERVE_HTTP_ADDR` (default `:8080`), `SLASHOBSERVE_DATABASE_URL` (required), `SLASHOBSERVE_LOG_LEVEL` (`debug|info|warn|error`, default `info`), `SLASHOBSERVE_LOG_FORMAT` (`json|text`, default `json`).
  Auth settings: `SLASHOBSERVE_PUBLIC_URL` (required; http or https origin with no path, query or user info), `SLASHOBSERVE_OIDC_ISSUER_URL` (required; used exactly as given because issuer comparison is exact), `SLASHOBSERVE_OIDC_INTERNAL_URL` (optional origin the server's back-channel uses instead of the issuer's scheme and host), `SLASHOBSERVE_OIDC_CLIENT_ID` and `SLASHOBSERVE_OIDC_CLIENT_SECRET` (required), `SLASHOBSERVE_SESSION_TTL` (default `168h`, between `5m` and `720h`), and `SLASHOBSERVE_AUTH_ALLOWED_EMAIL_DOMAINS`.
  `SLASHOBSERVE_AUTH_ALLOWED_EMAIL_DOMAINS` is required: a comma-separated list of domains matched exactly against the verified email's domain, or `*` as an explicit allow-all that logs a startup warning.
  Never log secret values, the database URL and the OIDC client secret included.
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
- Logging: `log/slog`, JSON by default; request logs record the path, never the query string.
- Design: prefer deep modules (small interface, real behavior behind it); accept dependencies instead of creating them; add a seam only when two adapters exist.
- Integration tests run against real Postgres, Prometheus and ClickHouse through testcontainers, never mocks.
- Write no docstrings or comments unless they state a constraint the code cannot show.

## Frontend conventions

- Bun is the package manager and script runtime; `web/bunfig.toml` forces the Bun runtime for every `bun run` script.
  Node and pnpm are not used.
- React 19, TypeScript strict, Vite, Tailwind CSS v4, shadcn/ui on Base UI (`components.json` style `base-nova`), TanStack Router with file-based routes.
- Add shadcn components with `bun x shadcn add <name>` from `web/`; they live in `web/src/components/ui`.
- `web/src/routeTree.gen.ts` is generated by the Vite plugin on `dev` and `build`; commit it, never edit it.
- `web/src/spec/*.gen.ts` are generated from the schema by `make generate` (commit them, never edit); import only from `web/src/spec/index.ts`, and keep that folder free of app imports so it can become the `@slashobserve/spec` package.
- Formatting follows shadcn's Prettier style (no semicolons, double quotes); `bun run format` before committing.
- Planned stack for later steps: TanStack Query for server data, Zustand for UI state only, uPlot, react-grid-layout, CodeMirror 6, Ajv, openapi-typescript and openapi-fetch, Vitest with Testing Library, lucide-react, Sonner, cmdk.

## Dependencies

- Pin exact versions only, and only versions released at least 14 days earlier.
  `web/bunfig.toml` enforces both for npm packages (`exact = true`, `minimumReleaseAge = 1209600` seconds).
- Go dependencies are exact in `go.mod`.
  `go.mod` has `ignore ./web` so `./...` skips the frontend's `node_modules`; keep it (requires Go 1.25 or newer).
  Dev tools run as `go tool -modfile=tools/<tool>/go.mod <tool>`; bump one with `go get -modfile=tools/<tool>/go.mod -tool <package>@<version>`; never run `go mod tidy` against a tool modfile.
- GitHub Actions are pinned to a commit SHA with the version in a trailing comment; Docker images are pinned by tag and digest.
- Version bumps that must change together: Go toolchain in `go.mod` (`go` line) and the `golang` image in `Dockerfile`; Bun in `web/package.json` (`packageManager`, read by CI) and the `oven/bun` image in `Dockerfile`; Postgres in `deploy/compose/compose.yaml`.
- Prefer boring, proven libraries; justify every new dependency in its PR.

## End-to-end tests

- Two suites, both required to merge: `scripts/e2e.sh`, curl-driven checks against the compose stack (API today; CLI and MCP as they land), and Playwright in `web/e2e` (Chromium, one worker, retries only in CI).
- Playwright specs import `test` and `expect` from `web/e2e/fixtures.ts`, which fails any test whose page logs a console error.
- The Playwright `setup` project (`web/e2e/auth.setup.ts`) signs in through Dex once and saves `web/e2e/.auth/user.json`.
  Every `chromium` spec reuses that session, and a spec that signs out must opt out with an empty `storageState` and sign in itself.
- The e2e stack is compose project `slashobserve-e2e`, reset with `down -v` on every `make e2e`, on host port 18080 (`SLASHOBSERVE_E2E_PORT`), so it never touches a developer's `make demo` stack.
  CI builds the server image once as `slashobserve-e2e:ci` and uploads Playwright traces, videos, the HTML report and compose logs when a test fails.
- Dex runs in compose on host port `SLASHOBSERVE_DEX_HOST_PORT` (5556 for `make demo`, 15556 for e2e via `SLASHOBSERVE_E2E_DEX_PORT`).
  Dev logins are `admin@example.com` / `password` (allowed domain `example.com`) and `outsider@example.org` / `password` (always refused, for tests).
  `scripts/e2e.sh` has `dex_login` for curl-driven sign-in.
- Every change that a user can see or call adds or updates an e2e check in the same PR; a feature is not done until both suites cover it.
- E2E tests never call a live model: they use the replay LLM provider (`SLASHOBSERVE_LLM_PROVIDER=replay`, fixtures in `e2e/fixtures/llm/`) once the agent exists.
- Compose: add new services (Dex, Prometheus, ClickHouse, OTel Collector, demo/shop) to `deploy/compose/compose.yaml`; publish host ports only for what a browser or developer must reach, bound to 127.0.0.1 with a `${...}` override.

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
