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
| `make cli` | Build only `bin/observe`; `make build` builds it too. |
| `make test` | `go test -race ./...` for the root module and `demo/shop`, then the web Vitest suite. |
| `make lint` | golangci-lint (config verify, format diff, run), `sqlc diff`, web typecheck, Spec drift and fixture parity check, web API type drift check (`api:check`), ESLint, Prettier check. |
| `make ci` | `lint`, `test`, `tidy-check` (`go mod tidy -diff` for the root module and `demo/shop`), `build`: what the CI `check` job runs. |
| `make generate` | Regenerate sqlc code, the web Spec types/validator and the web API types (`web/src/api/schema.gen.ts`) after editing migrations, queries, the schema or `api/`. |
| `make demo` / `make demo-down` | Run the full demo stack in `deploy/compose` and stop it: the app on http://localhost:8080, Prometheus on http://localhost:9090 and the shop fault API on http://localhost:8081, with Dex, Postgres, ClickHouse, the OTel Collector, node-exporter, cAdvisor and the shop behind them. |
| `make demo-reset` | Stop the demo stack and drop its volumes, so the 26 hour request and latency backfill runs again on the next `make demo`. |
| `make demo-fault TYPE=errors\|latency PCT=N` | Inject a fault into the `checkout-0` pod through the shop fault API. |
| `make demo-fault-reset` | Clear the injected fault. |
| `make e2e` | Reset and boot the isolated `slashobserve-e2e` stack on http://localhost:18080 and run `scripts/e2e.sh`; the stack is left running. |
| `make e2e-browser` | Reuse the running, healthy e2e stack or boot one, then run Playwright in `web/e2e`; reuse does not rebuild, so rerun `make e2e` after code changes. |
| `make e2e-all` | Both suites, as CI runs them. |
| `scripts/e2e.sh down` | Tear the e2e stack down, including volumes. |
| `make evals EVALS_ARGS=...` | Run the eval runner (`go run ./evals/runner`) against a stack with a real LLM provider; needs `OBSERVE_SERVER` and `OBSERVE_TOKEN` (a service-account key) in the environment. |

Web-only commands run from `web/`: `bun run dev` (Vite on :5173, proxying `/api`, `/healthz` and `/readyz` to :8080), `bun run typecheck`, `bun run lint`, `bun run format`, `bun run test`.

## Layout

Directories are created when their phase starts; this is where things go.

- `cmd/slashobserve-server/`: server binary (`healthcheck` subcommand backs the Docker health check).
- `cmd/observe/` + `internal/cli/`: the `observe` CLI (API client with a service-account key; settings from flags, `OBSERVE_SERVER`/`OBSERVE_TOKEN`, then `~/.config/slashobserve/cli.json`; the saved key goes only to the server it was saved for).
- `internal/config/`: the only reader of `SLASHOBSERVE_*` environment variables.
- `internal/logattrs/`: turns an error into log attributes that are safe to write (the innermost Go error type, for a Postgres error the SQLSTATE, and for a system error the `errno` text) without its message, which can echo queries, tool results or credentials; not test-only, and used by every server log line that reports an error.
- `internal/logtest/`: test-only capture logger and assertions for log lines (see the Logging convention).
- `internal/db/`: pgx pool, goose migrations (`migrations/`), sqlc queries (`queries/`) and generated code (`dbsqlc/`); `dbtest/` starts one Postgres testcontainer per test binary and hands each test its own migrated database (`dbtest.Main` in `TestMain`, `dbtest.New(t)`, or `dbtest.NewWithLogger(t, logger)` to capture the DB's own log lines; skipped under `-short`).
- `internal/tctest/`: test-only retry around testcontainers starts for the v0.44.0 shared-reaper race; every container start in tests goes through it.
- `internal/auth/`: OIDC login, sessions, service-account keys and the auth middleware.
- `internal/users/`: the People routes (`GET /api/v1/users`, paged newest first sign-in by `(created_at, id)`, and `DELETE /api/v1/users/{id}/sessions`, which also expires the caller's cookie when they sign themselves out), user principals only (`auth.RequireUser`).
- `internal/server/`: Gin HTTP server, routes, embedded web build (`webdist/`).
- `internal/server/mcp/`: remote MCP at `/mcp` (stateless Streamable HTTP, JSON responses), bearer service-account keys only, origin-checked; every tool comes from `internal/agent/tools`.
- `internal/queryproxy/`: panel query runner shared by save validation and the proxy: kind by panel type, step from maxDataPoints, variable defaults, a 32-call upstream semaphore, one error-to-HTTP mapping.
- `internal/queryproxy/proxyhttp/`: the query proxy HTTP handlers (panel data, variable values, datasource lint), registered as a `server.Registrar`; a subpackage because it reads versions through `internal/dashboards`, which imports `queryproxy`.
- `internal/ttlcache/`: the byte-bounded TTL LRU and the coalescing call group behind the query proxy's caches.
- `internal/datasource/{prometheus,clickhouse,macros,provision}`: the datasource core and adapters, query macros, and registry construction from config.
- `internal/llm/`: provider-neutral gateway (`Provider` starts a `Session` that owns the native history); adapters `anthropic/`, `openaicompat/`, `replay/`; `providers/` picks one from configuration.
  `e2e/fixtures/llm/` holds the replay fixtures.
- `internal/dashboards/`: dashboard and version storage, the save validation pipeline and the `/api/v1/dashboards` routes (`Service.Register`).
- `internal/httpapi/`: the HTTP helpers shared by `internal/dashboards` and `internal/revisions` (error responses, body decoding, path and limit parameters, time formatting, the `(time, id)` cursor codec).
- `internal/events/`: the event payload, the `slashobserve_events` Postgres channel name shared by publishers and the SSE stream, and the `Broker` that fans notifications out to subscribers; `internal/events/stream/` serves them over SSE.
- `internal/revisions/`: the durable agent request queue (`agent_requests`), its in-process workers, the `Reviser` contract the agent implements (`agent.Reviser`) and the `/api/v1/dashboards/{id}/revisions` routes.
- `internal/agent/tools/`: the only place agent tools are defined (14 tools, sorted, constant schemas); `Call` returns a tool error the model can fix or an internal error; ad-hoc queries run through the panel runner with tool limits.
- `internal/agent/`: the loop (discover, draft, lint, save, up to 3 repairs; a successful save ends the run); a run's only product is a Spec saved through `internal/agent/tools`; a revise conflict restarts once from the latest version.
  The agent, and any MCP client, may move or delete a placeholder panel but never create or edit one; `create_dashboard` and `update_dashboard` reject that with `placeholder_not_allowed` before saving.
- `internal/agent/prompts/`: embedded prompts, including the system prompt and the Spec authoring guide.
- `evals/`: scenarios (`scenarios/*.yaml`), pricing, and the runner (`go run ./evals/runner`), which scores validity, data, source, metrics, tables, units, panel count, thresholds and latency through the public API and reports cost; scenarios tagged `replay-smoke` run in e2e with replay fixtures.
  A scenario's `expect` takes `datasources`, `metrics`, `tables`, `units` (ids from the Spec unit enum), `min_panels`, `max_panels`, `max_seconds`, `thresholds`, and three optional feature checks: `query_contains_any` (a list of groups; each group needs at least one alternative to appear, case and whitespace insensitive and on word boundaries, in some panel query; PromQL `#` comments are ignored), `time_range_from` (accepted values of the dashboard's `time.from`) and `threshold_at` (unit to threshold value, matched on a single panel).
  The runner holds a slot per scenario until its request finishes server-side (a timed-out request is polled for up to 3 minutes) and retries a 429 on create, so `--parallel` stays within the server's 3 active observe requests per key and is capped at 3.
- Later: `pkg/extension`, `deploy/helm/`.
- `spec/schema/observespec.v0.json` is the ObserveSpec source of truth: the evolved v0 frozen for milestone 1 (same `$id` and `specVersion` as before), copied byte for byte and never edited without Ishan's sign-off; `spec/examples/` holds valid examples and `invalid/` fixtures with `expected.json`, shared by Go, TS and e2e tests.
- `pkg/spec/` (public): hand-written types checked against the schema by a conformance test; `Parse` is the only validation entry point, accepts only the evolved shape and returns `*ValidationError` with JSON-Pointer issues; `Upgrade` is the pure rewrite of the legacy v0 shape (`timeRange`, `{type, uid}` datasources, panel `query` and `legend`) to the evolved one, for migrating stored versions and converting fixtures; `ScanQuery` and `Macros` define query reference syntax for the macros package; `Diff` returns a stable structural change list between two Specs (panels matched by `id`, variables by `name`) for version history.
  Panels are flat structs covering every panel kind; `AllPanels` and `FindPanel` walk top-level panels and row children with their JSON Pointers, `DataPanels` and `FindDataPanel` narrow that to the panels that run queries, and `QueryRef` gives a query's ref (`A`, `B`, ... by position unless written).
- `api/`: the OpenAPI document for `/api/v1`: `openapi.yaml` holds the root, shared schemas and responses, and each feature adds `paths/<feature>.yaml` plus one `$ref` line in the root; `api.Load` embeds and validates it.
- `web/`: React app (Bun); `web/src/routes` (TanStack Router files), `web/src/shell` (the sidebar shell), `web/src/components/ui` (shadcn), `web/e2e` (Playwright).
- `deploy/compose/compose.yaml`: the one compose stack; `scripts/e2e.sh`: e2e stack lifecycle and API smoke suite.
- `docs/self-hosting.md`: the operator guide (settings, datasources file, sign-in, LLM providers, keys, CLI, MCP, upgrades, security model); update it in the same change as any setting, default or behavior it describes.
- `demo/shop/`: its own Go module (`github.com/slashobserve/slashobserve/demo/shop`) with the `serve`, `loadgen` and `healthcheck` commands; it emits the demo metrics, logs and traces.
- `deploy/compose/{prometheus,clickhouse,otelcol}/`: the configs mounted into the demo stack's Prometheus, ClickHouse and OTel Collector.
- `tools/<tool>/go.mod`: pinned Go dev tools (sqlc, golangci-lint).

## Backend conventions

- Configuration: every setting is an environment variable named `SLASHOBSERVE_*`, parsed and validated only in `internal/config` (`config.Load`).
  Current settings: `SLASHOBSERVE_HTTP_ADDR` (default `:8080`), `SLASHOBSERVE_DATABASE_URL` (required), `SLASHOBSERVE_LOG_LEVEL` (`debug|info|warn|error`, default `info`), `SLASHOBSERVE_LOG_FORMAT` (`json|text`, default `json`).
  Auth settings: `SLASHOBSERVE_PUBLIC_URL` (required; http or https origin with no path, query or user info), `SLASHOBSERVE_OIDC_ISSUER_URL` (required; used exactly as given because issuer comparison is exact), `SLASHOBSERVE_OIDC_INTERNAL_URL` (optional origin the server's back-channel uses instead of the issuer's scheme and host), `SLASHOBSERVE_OIDC_CLIENT_ID` and `SLASHOBSERVE_OIDC_CLIENT_SECRET` (required), `SLASHOBSERVE_SESSION_TTL` (default `168h`, between `5m` and `720h`), and `SLASHOBSERVE_AUTH_ALLOWED_EMAIL_DOMAINS`.
  `SLASHOBSERVE_AUTH_ALLOWED_EMAIL_DOMAINS` is required: a comma-separated list of domains matched exactly against the verified email's domain, or `*` as an explicit allow-all that logs a startup warning.
  `SLASHOBSERVE_AUTH_PROVIDER_NAME` is optional: the identity provider's display name (trimmed, at most 64 characters, default empty), shown to signed-in clients as `oidc_provider_name`; the demo and e2e stacks set it to `Dex`.
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
- MCP: `/mcp` is mounted through `server.Options.Mounts` outside `/api/v1`, so the API contract check does not cover it.
  It accepts only `Authorization: Bearer` service-account keys (a session cookie is ignored), refuses an `Origin` other than `SLASHOBSERVE_PUBLIC_URL`, answers 413 above 8 MiB, 429 above 8 in-flight requests per principal and 503 `busy` above 32 in flight across all principals, and keeps no session state.
  Connect Claude Code with `claude mcp add --transport http slashobserve http://localhost:8080/mcp --header "Authorization: Bearer <key>"`.
  Signed-in users create keys and copy Claude Code, OpenCode and CLI setup snippets on the web page `/keys` (API keys in the sidebar; `/settings/keys` redirects).
- Auth: every `/api/v1` route goes on the group returned by `auth.Service.Mount` except `/api/v1/auth/*`.
  Handlers read the caller with `auth.FromContext`.
  Sessions and keys are opaque random tokens stored only as SHA-256.
  Cookie-authenticated mutations pass `http.CrossOriginProtection`.
  `users.last_seen_at` is touched at most once a minute (`userSeenInterval` is the only definition, passed to the query) by session requests that passed authentication and, for mutations, the cross-origin check.
  Errors are `{"error","code"}` and 401s carry `WWW-Authenticate: Bearer`.
  The issuer is the browser-facing URL, and `SLASHOBSERVE_OIDC_INTERNAL_URL` only rewrites the server's back-channel host.
  `GET /api/v1/instance` returns the public and MCP URLs, the issuer URL, the optional provider display name, allowed email domains and session TTL, never a secret or the internal OIDC URL.
- Database: Postgres through a pgx v5 pool.
  Migrations are goose SQL files `internal/db/migrations/NNNNN_snake_case.sql` with `-- +goose Up` and `-- +goose Down`, numbered sequentially; never edit a merged migration.
  Migration 10 is a Go migration (`internal/db/specupgrade.go`, registered in `newMigrator`) that rewrites every stored Spec with `spec.Upgrade`; it has no file, so SQL files continue at `00011`, and a file reusing a Go migration's version stops the server at boot.
  The server applies migrations at boot under a Postgres advisory lock.
  Queries live in `internal/db/queries`; sqlc generates `internal/db/dbsqlc` (committed, never hand-edited; run `make generate`).
- Dashboards: versions are immutable and linear (`parent_version = version - 1`); a save validates with `spec.Parse`, then runs every panel query through `internal/queryproxy`, outside any transaction; the conditional `latest_version` update is the concurrency check (409 `version_conflict`); events go out with `pg_notify` on channel `slashobserve_events` inside the saving transaction.
- Events: writers `pg_notify` on `slashobserve_events` inside their transaction; one `events.Broker` per process listens and fans out to `GET /api/v1/dashboards/{id}/events` (SSE) per dashboard; every stream starts with `ready` (current `latest_version`), sends `resync` after a listener reconnect, pings every 15 s and ends at the first ping after the caller's session or key stops being valid (so a sign out closes open streams within 15 s), ends after 30 minutes, and clients refetch on `ready` and `resync` instead of replaying ids.
- Revisions: comments (unless `request_revision` is false) and `POST /dashboards/{id}/revisions` prompts enqueue `agent_requests` rows, a comment's inside its own transaction; at most 5 are queued or running per dashboard (429 `too_many_revisions`, enforced under a per-dashboard advisory lock).
  In-process workers (2 per process) claim under a global advisory lock with `FOR UPDATE SKIP LOCKED`, hold a 5 minute lease, run at most 2 attempts and run one request per dashboard at a time (a partial unique index allows one running row per dashboard, and a queued request waits behind an expired running one until it is retried or failed); wake-ups come from `pg_notify` on `slashobserve_agent_requests` plus a 5 s poll, and a run gets 3 minutes.
  `dashboard_versions.agent_request_id` is unique and the agent's `update_dashboard` saves through it, so a retried run returns the version it already created instead of a second one.
  A finished request keeps a status, `result_version` and an error of at most 500 characters taken only from the agent's own fixed messages, and every status change sends a `revision.updated` event.
  A saved version always wins: before a failure is recorded, and when the lease sweep exhausts a request, the request counts as succeeded with that version if one exists under its id.
  The saved version's `version.created` arrives before the `revision.updated` that reports `succeeded`, and clients must not depend on the order between them.
  A shutdown cancels the running request and releases it back to the queue without spending an attempt, and so does a failed check for a saved version; only a crash leaves a lease to expire for the next process.
  Without an LLM provider prompts get 503 `agent_unavailable` and comments do not enqueue, so turning an agent on later never replays old comments.
- Observe requests: `POST /api/v1/observe` enqueues an `agent_requests` row with `source = 'observe'` and no dashboard (at most 3 queued or running per principal, 429 `too_many_requests`, and 20 per instance, 429 `agent_busy`), or, with `dashboard_id`, a follow-up revision with `source = 'prompt'`.
  Observe rows never block each other in the claim; `dashboard_id` is filled from the version saved under the request id when the request finishes, so queue lookups of a saved version use the request id only.
  `create_dashboard` saves through `dashboard_versions.agent_request_id`, so a retried run returns the dashboard it already created.
  `GET /api/v1/observe/{id}` returns any agent request with progress `steps` (the tools' fixed step strings, at most 100 per attempt, tagged by attempt) and `usage` (tokens, model calls and repair rounds of the finishing attempt, null when the run made no model call); a shutdown release deletes the released attempt's steps and the re-claimed run reuses its attempt number, so clients tell a restarted list by each step's `at`, not by `attempt` alone; the `observe` CLI polls it every second, rides out two consecutive transport or 5xx failures and gives up on the third, and the web client does not call it; observe rows send no `revision.updated` events.
  `GET /api/v1/agent` tells clients whether an agent is configured and which provider, model and (Anthropic only) effort it uses.
- Comments: anchored to `(dashboard, version, panel_id)`, never moved when panels change (`panel_present` tells clients); resolve and reopen are idempotent `PATCH` status changes that send `comment.updated` only when the status really changed; `revision` holds the latest agent request for the comment (null when none was requested); event payloads carry ids and a capped author name, never comment text, so they stay far below the 8000 byte `pg_notify` limit.
  Comment lists page by `(created_at, id)` with `created_at` taken at insert, so a comment that commits after a later one was already listed can be missed by a client paging at that moment, and a fresh listing or its `comment.created` event recovers it.
- Query proxy: panel data is `GET /api/v1/dashboards/{id}/versions/{version}/panels/{panel_id}/data`; it accepts no query text (403 `query_not_in_spec`), aligns ranges to the step, caches successes for 10 s in process, limits each principal to 8 in-flight requests, and returns time values as epoch milliseconds.
  Variable selections are repeated `var-<name>` parameters, `$__all` selects All, custom variables accept only their options, and All expands to the full value list.
  Variable value lists are served by `GET .../variables/{name}/values`, and `POST /api/v1/datasources/{uid}/lint` parses a query without running it.
  `Datasource.Lint` returns an error, not a warning diagnostic, when the datasource is unavailable or times out, so the lint endpoint answers 502 `datasource_unavailable` or 504 `query_timeout` and the editor shows "Lint is unavailable right now."
  The proxy's runner is built with `queryproxy.WithValueCache()`, which caches query-variable value lookups for 10 s and coalesces concurrent identical ones; the runner that validates saves stays uncached, and both share the 32-call semaphore.
  A malformed query string is a 400, and a forbidden parameter name is a 403 even when its value is malformed.
  Coalesced calls run on a shared context that is canceled when the last waiting caller leaves, and lint takes the process-wide upstream slot like any other datasource call.
  The proxy caches stay near 88 MiB: panel data 64 MiB, variable value lists 8 MiB (1 MiB per entry), and the runner's value lookups 16 MiB.
  Never log query text or variable values.
- Datasources: adapters live in `internal/datasource/{prometheus,clickhouse}` behind `datasource.Datasource`, and `internal/datasource/provision` builds the registry from `config.Datasources`.
  Variable values reach queries only through `internal/datasource/macros`.
  ClickHouse is queried only as a `readonly = 2` user with `SELECT` on the OTel database, and the server refuses a user that is not read-only.
  `GET /api/v1/datasources` returns health only, never a URL, address, user or secret, and datasources never gate `/readyz`.
  A Spec names a datasource by uid only and its type comes from the registry, so the query proxy, not `spec.Parse`, rejects a macro that the datasource's type does not have (`unknown_macro`).
- Logging: `log/slog`, JSON by default; request logs record the path, never the query string.
  The server never logs secrets, prompt text, query text or tool results, so a failure is logged as a fixed message plus `logattrs.Failure(code, err, ids...)` (`error_code`, `error_type`, `sqlstate`) and the relevant ids, never `err.Error()`, and a panic by its type only.
  `sloglint` (in `make lint`) forbids the literal attribute keys `error` and `err` in slog calls under `internal/`, `cmd/` and `pkg/`, tests included; `internal/logattrs` is exempt, and `demo/` is out of scope.
  It cannot see a key built through a variable or a helper, or `err.Error()` under another key, so review those by hand.
  A fatal startup error keeps its text only through `described` in `cmd/slashobserve-server/main.go`, for messages the server wrote from settings (never echo a URL setting); driver, DSN and library causes are reduced to `error_code`, `error_type`, `sqlstate` and `errno`, plus ids the server owns (`db_host` and `db_port` parsed from the setting, `datasource_uid` and the datasource's own `ConfigError` text from `provision.BuildError`, `migration_version`).
  `internal/logtest` captures a logger for tests (`AssertSafeFailure` checks the code and type and that the error text is absent).
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
- `web/src/components/ui` primitives are restyled to the M1 rules, and every `bun x shadcn add` must be restyled the same way before commit: controls `rounded-control` and 32px high (`h-8`) with 13px text, popups and dialogs `border border-border` with no shadow, ring or backdrop blur, badges never pill-shaped, and focus a solid 2px `ring-ring` (brand blue).
  Workspace pages are built from `PageHeader`, `TableCard` (`HeaderCell`, `Cell`), `StatusLabel` (icon plus word, tones `ok|warning|critical|running|none`) and `RelativeTime` (`formatShortRelative`: "12 s ago", "Yesterday", "In 74 days"; its shared clock ticks every second while a mounted one shows an offset under a minute, else every 30 s) in `web/src/components`.
  Import `cn` only from `@/lib/utils`: it is configured with the custom `control` and `card` radii, so `cn("rounded-control", "rounded-full")` keeps one; a `bun x shadcn add` file that imports `cn` from `cn` gets rewritten to `@/lib/utils` in the same restyle.
- `web/src/routeTree.gen.ts` is generated by the Vite plugin on `dev` and `build`; commit it, never edit it.
- `web/src/spec/*.gen.ts` are generated from the schema by `make generate` (commit them, never edit); import only from `web/src/spec/index.ts`, and keep that folder free of app imports so it can become the `@slashobserve/spec` package.
  `web/src/spec/panels.ts` holds the pure panel helpers (`allPanels`, `findPanel`, `isDataPanel`, `queryRef`).
- `web/src/api/schema.gen.ts` is generated from `api/openapi.yaml` by `make generate` (commit it, never edit); `make lint` fails when it is stale.
- Formatting follows shadcn's Prettier style (no semicolons, double quotes); `bun run format` before committing.
- The logo is `Wordmark` (Geist 600, tracking -0.035em, slash in `--brand`, at least 12px) and `Mark` (the brand-kit slash cut out of a 5.8/24 rounded square, at least 16px) in `web/src/components/brand/`; never type a styled "/observe" anywhere else.
  `web/public/favicon.svg` (follows the system theme), `favicon-32.png`, `favicon-16.png` and `apple-touch-icon.png` are generated by `bun run icons:generate` (Playwright Chromium); change the script, never the files.
- Installed stack: TanStack Query, openapi-fetch over openapi-typescript types, Zustand, Sonner, uPlot, react-grid-layout, CodeMirror 6 (`@codemirror/*` with `@prometheus-io/codemirror-promql` and `@codemirror/lang-sql`), Ajv, lucide-react, Geist and Geist Mono (`@fontsource-variable/geist`, `@fontsource-variable/geist-mono`) and Vitest for pure helpers (`*.test.ts` next to the code, run by `bun run test` and `make test`).
  Testing Library is not installed.
  cmdk is deliberately not used: the ⌘K palette (`web/src/shell/command-palette.tsx`) is Base UI `Autocomplete` rendered `inline` in a `Dialog`, searches the first 200 dashboard titles (`dashboardKeys.titles()`) and the sidebar pages, offers "Sign out" (the same `useSignOut` as the sidebar account menu), opens dashboards in a new tab, and is the only page navigation and sign out under 760px.
- Server data goes through TanStack Query with keys from `web/src/api/keys.ts` only; the typed client is `api` from `web/src/api/client.ts` (openapi-fetch over `web/src/api/schema.gen.ts`), and responses are unwrapped with `unwrap`, which throws `ApiError`.
- Zustand holds UI-only state (theme, unsaved layout edits); never copy server data into a store.
- The router uses plain `URLSearchParams` search serialization (`web/src/lib/search.ts`): search values are strings, numbers or string arrays, repeated keys become arrays, and each route coerces types in `validateSearch`.
- Routes: `_authed` only checks the session; every page except the dashboard renders inside the pathless `_authed/_shell` layout (`web/src/shell/app-shell.tsx`: sidebar, top row, content card, right-card side panels), and dashboard routes under `_authed/dashboards/$dashboardId` have no shell (until the dashboard page phase they keep `web/src/shell/legacy-header.tsx`).
  A shell route declares its breadcrumb with `staticData.crumbs`, sets its title with `useDocumentTitle`, and never renders `<main>` (the shell owns it).
  Side panels (Create key, History) are `SidePanel` right cards portaled next to the content card, never dialogs or sheets; top-row page actions go through `TopRowActions`.
  The sidebar is 240px by default, resizable from 200 to 360 (grip, arrow keys, Home, End, Enter or double-click to reset), collapses to 44px icons with tooltips, remembers its state in `localStorage` keys `slashobserve-sidebar-width` and `slashobserve-sidebar-collapsed`, and is hidden under 760px.
- Dashboards live at `/dashboards/$dashboardId`; the URL holds `from`, `to`, `refresh`, `version`, `panel`, `tab` and repeated `var-<name>` values, parsed only by `web/src/dashboards/search.ts` (`withVariables` adds `var-` params to API calls).
  Panel data requests always send an explicit `var-` selection for every declared `query` and `custom` variable (other variable types get no control yet and take the server's default), resolved by `web/src/dashboards/variables.ts`, so the panel and the variable list agree on defaults.
  A selection that needs a variable's values list (no URL value and no All) is sent once those values have loaded for the current parent selections, never from a previous parent's list, and if the values request fails the dependent panels wait in an error state with the variable control's retry.
  An empty values list leaves the selection to the server.
  While a variable's values reload, its control keeps the last values visible but disabled with a spinner, and it shows no selected value unless the URL or All supplies one when the parents changed; the hook keeps the last loaded values itself because `useQueries` does not carry data across keys.
- `/observe`: the home page prompt bar posts to `POST /api/v1/observe` and opens `/observe/$requestId`, which polls every second and replaces itself with `/dashboards/$dashboardId` on success; only server step messages and errors are shown, never model text.
- Panel data requests go through `panelRequestLimiter` (6 concurrent) because the proxy allows 8 in-flight requests per principal; panels fetch once they scroll near the viewport.
- Panel renderers implement `PanelRendererProps` in `web/src/panels/types.ts` and are registered only in `web/src/panels/index.tsx`; panel states are exposed as `data-state` on the panel `article` for tests.
  A data panel type without a renderer, a text panel and a placeholder panel show the unsupported state (`data-state="unsupported"`) until rendering breadth lands; a row is drawn as a static full-width header (title and a bottom border, no panel chrome) at its own `gridPos`, an expanded row's children sit at their own `gridPos`, and a collapsed row's children are neither laid out nor drawn and keep their stored `gridPos` on save.
- Timeseries panels use one uPlot instance per panel, re-created only when series identity, units, thresholds or theme change, with `cursor.sync.key` set to the dashboard id.
  Data refreshes call `setData`.
- Series colors come from `--series-1..8` in a fixed order and stick to a series key for the panel's lifetime, which includes the table view toggle and empty refreshes (color slots and hidden series live in a store keyed by dashboard and panel id); series past eight reuse hues with dash patterns.
  The legend hides only when one series remains, and then every series is drawn.
- Threshold states always pair the threshold color with an icon and a word, and expose `data-threshold` for tests.
- Values that appear together (a table column, stat tiles, bargauge rows, tooltip rows, axis ticks) share one decimal count from `commonDecimals` in `web/src/lib/units.ts`, so `2.40` and `2.45` never mix with `2.4`.
  The count is capped at about four significant figures of the largest value, so tiny values next to large ones round instead of widening every value; axis ticks (`axisDecimals`) widen past the cap until neighbouring ticks read differently, and a threshold label never rounds below the threshold's own decimals.
- Browser specs create their own dashboards through `web/e2e/api.ts` with `uniqueTitle`, from Specs in `web/e2e/data/`.
- The People page (`/people`) lists every user from `usersQuery` in `web/src/people/queries.ts` (it fetches every `GET /api/v1/users` page and sorts by `last_seen_at`, newest first, refetching every minute), shows "Online now" within 2 minutes of `last_seen_at`, states the sign-in policy from `GET /api/v1/instance` (`any_email_domain`, `allowed_email_domains`, `oidc_provider_name`), and signs a user out everywhere after a confirmation; signing yourself out goes straight to `/login` without refetching the list.
  Browser specs never send a real `DELETE /api/v1/users/{id}/sessions` for the shared e2e user (it would end the session every later spec uses); they mock it.
- Theme preference lives in `localStorage` key `slashobserve-theme` and is applied before first paint by `web/public/theme-init.js`; chart colors use the `--series-1..8` and `--threshold-*` tokens in `web/src/index.css`, never ad-hoc hex values.
- Design tokens come from the frozen M1 design canvas and live only in `web/src/index.css`, with light and dark values: `--brand` (the logo slash, selection and focus only, never body text), `--chrome` (sidebar and app edge only), `--background`, `--foreground`, `--border`, `--muted-foreground`, `--primary`, `--canvas`, `--selection` (selected-row and selected-text background), `--ghost`, `--status-ok|warning|critical` (UI status icons; charts keep `--threshold-*`) and `--syntax-function|string|number|keyword`.
  Radii are `rounded-control` (6px, controls) and `rounded-card` (10px, panels and cards), the `narrow:` variant is `@media (max-width: 760px)`, body text is 14px, and focus uses `--ring`, the brand blue.
  Geist is the UI font; Geist Mono (`font-mono`) is only for queries, JSON, ids, key prefixes and commands.
- Visual defaults to avoid: cream or off-white page backgrounds, italic accent words in headings, numbered `01/02` section labels, monospace for UI labels (monospace is only for queries, JSON, ids, key prefixes and commands), pill-shaped buttons, gradients, shadows or elevation of any kind (popups, dialogs and toasts separate by a 1px border), backdrop blur, colored left-border callouts, uppercase eyebrow labels, emoji, chart animations, grey fills inside a dashboard, and brand blue on anything but the logo slash, selection and focus.
- Each open dashboard holds one `EventSource` (`useDashboardEvents`); events only invalidate TanStack Query keys (never write server data into stores), and `ready` and `resync` refetch whatever is stale.
  Fatal stream errors re-check the session and reconnect with jittered backoff, and every third consecutive failure also refetches the dashboard so a deleted one shows not found.
  The indicator shows "Reconnecting" only after 2 seconds, version toasts skip a save this tab is still sending (a mutation under `dashboardKeys.saves`) and are dismissed with the page, and hidden tabs drop the stream after 5 minutes.
- Version history lives at `/dashboards/$dashboardId/versions?from=&to=&back=`; the browser never computes diffs, it groups the server's `pkg/spec.Diff` changes with `groupChanges`, and restore sends `parent_version` = the latest version known when its dialog opened (only a 409 raises it).
  `back` carries the dashboard's search string through the page and is re-parsed with `validateDashboardSearch`, so only dashboard keys survive; "Back to dashboard" and a restore from the page return to it.
  The page holds its own `useDashboardEvents` stream, so versions saved elsewhere appear live.
- Layout editing keeps the base Spec and the edited layout in the Zustand `useLayoutStore`; saving sends `applyLayout(baseSpec, layout)` with `parent_version = baseVersion`, and a 409 offers to reapply the layout onto the latest version, carrying over only the panels the user changed and settling the result.
  Saving a Spec that has vertical gaps compacts it to the layout the user saw.
  The grid renders from the session's base Spec while editing, so a version that arrives over the event stream never moves panels.
  Every panel's move handle also moves (arrow keys) and resizes (Shift and arrow keys) it, and vertical moves step past the neighbouring panel because the grid compacts upward.
- The query editor (`web/src/query-editor`) is lazy-loaded CodeMirror with `classHighlighter` tokens styled in `query-editor.css`; lint comes only from `POST /api/v1/datasources/{uid}/lint`, whose byte offsets go through `byteRangeToIndices`, and codemirror-promql never gets a `remote` (the browser never calls a datasource).
- Panel inspector views open from the URL (`panel` + `tab`) in `web/src/dashboards/panel-inspector.tsx`; add a view as a tab there.
  The query tab keeps an unsaved draft when a newer version arrives live and saves with the `parent_version` it was opened on, so a newer version turns the save button into "Apply my query to version N".
- Comments load once per dashboard (`dashboardKeys.comments`), per-panel lists and counts are derived, resolve and reopen are optimistic with rollback, bodies render as plain text only, and live updates come from `useDashboardEvents` invalidations.
  The Comments tab and the dashboard-wide `tab=comments` view live in the panel inspector, which follows the query editor's layout-session rule (unavailable while a layout is being edited) and guards a comment draft and a query draft with one discard prompt.
  The composer posts `request_revision: false` unless its `requestRevision` prop is set.
  `CommentsTab` forwards `requestRevision`, `composerExtraFields` and `renderRevision` (and `DashboardComments` forwards `renderRevision`), so a parent owns the revision checkbox state and renders each comment's revision status.
  Comment paging stops after a failed page until the next successful refetch, the flattened list is deduplicated by id, and status announcements follow a saved change through `liveRegionText`.
- The API keys page (`/keys`, in the sidebar) keeps a new key only in the create dialog's component state: the create-key mutation uses `gcTime: 0`, the response is never written to the query cache, and snippets reference `OBSERVE_TOKEN` instead of the key (Claude Code through `.mcp.json` `${OBSERVE_TOKEN}` expansion, OpenCode through `{env:OBSERVE_TOKEN}` with `oauth: false`).
- Revision requests come from comments (`request_revision`, on by default through the checkbox `useCommentRevision` builds, whose last choice persists in `localStorage`) and from the follow-up bar (`POST /dashboards/{id}/revisions`, on the latest version only).
  On a pinned older version the checkbox is disabled with a visible reason, because a comment revision would start from that old version, and the comment itself still posts.
  Their live status is read from `dashboardKeys.revisions` (invalidated by `revision.updated`, polled every 5 seconds only while one is queued or running), never from the status embedded in a comment; a revision missing from that newest-50 page is fetched by id (`dashboardKeys.revision`), and `RevisionStatus` links to `/observe/$requestId` for step-by-step progress.
  Only the follow-up bar's tracked chip is a live region (`RevisionStatus live`).
  Agent availability comes from `GET /api/v1/agent` (`observeKeys.agent()`), and a 503 `agent_unavailable` or a comment saved with no `revision` writes "unavailable" into that query, which hides the follow-up form and the checkbox until the query refetches.
  `FollowUpBar` always renders its header row with the revision list (pinned versions, layout editing and an unavailable agent included), and only the form needs the latest version outside layout editing.
  A follow-up draft, the revision it started and every revision requested from a comment live in `useFollowUpStore`, so they survive History, pinned versions and live updates, and `useRevisionAppliedToast` (called by `DashboardPage`) shows "Revision applied as version N" for each of them.
  The cap of 5 queued or running revisions is `maxQueuedRevisions` in `web/src/revisions/queries.ts`, which builds the copy the composer and the follow-up bar show.

## Dependencies

- Pin exact versions only, and only versions released at least 14 days earlier.
  `web/bunfig.toml` enforces both for npm packages (`exact = true`, `minimumReleaseAge = 1209600` seconds).
- Go dependencies are exact in `go.mod`.
  `go.mod` has `ignore ./web` so `./...` skips the frontend's `node_modules`; keep it (requires Go 1.25 or newer).
  Dev tools run as `go tool -modfile=tools/<tool>/go.mod <tool>`; bump one with `go get -modfile=tools/<tool>/go.mod -tool <package>@<version>`; never run `go mod tidy` against a tool modfile.
- MCP: `mark3labs/mcp-go` serves `/mcp`, and only `internal/server/mcp` imports it.
- LLM SDKs: `anthropic-sdk-go` and `openai-go/v3` are the only model clients; adapters keep provider history append-only and pass Anthropic responses back with `Message.ToParam()` so thinking blocks stay intact.
  The OpenAI-compatible adapter rebuilds assistant history with its own helper instead of `ToParam`, so a tool call with a missing type still round-trips.
- GitHub Actions are pinned to a commit SHA with the version in a trailing comment; Docker images are pinned by tag and digest.
- Version bumps that must change together: Go toolchain in `go.mod` (`go` line), `demo/shop/go.mod`, and the `golang` image in `Dockerfile` and `demo/shop/Dockerfile`; the distroless image digest shared by both Dockerfiles; Bun in `web/package.json` (`packageManager`, read by CI) and the `oven/bun` image in `Dockerfile`; Postgres in `deploy/compose/compose.yaml` and in `internal/db/dbtest`; the `prom/prometheus` and `clickhouse/clickhouse-server` images in `deploy/compose/compose.yaml` and in the integration tests under `internal/datasource/{prometheus,clickhouse}`.
- Prefer boring, proven libraries; justify every new dependency in its PR.

## End-to-end tests

- Two suites, both required to merge: `scripts/e2e.sh`, curl-driven checks against the compose stack (API, MCP and CLI), and Playwright in `web/e2e` (Chromium, one worker, retries only in CI).
- Playwright specs import `test` and `expect` from `web/e2e/fixtures.ts`, which fails any test whose page logs a console error.
- The Playwright `setup` project (`web/e2e/auth.setup.ts`) signs in through Dex once and saves `web/e2e/.auth/user.json`.
  Every `chromium` spec reuses that session, and a spec that signs out must opt out with an empty `storageState` and sign in itself.
- The e2e stack is compose project `slashobserve-e2e`, reset with `down -v` on every `make e2e`, on host port 18080 (`SLASHOBSERVE_E2E_PORT`), so it never touches a developer's `make demo` stack.
  `scripts/e2e.sh` run, up, browser and down hold an exclusive kernel `flock` on `/tmp/slashobserve-e2e-run.lock` (a fixed path, because `TMPDIR` differs between shells and agents) for their whole run, and write their pid into it, so two invocations never reset the same stack; a second one fails at once with "another e2e run is active (pid N)", or with "cannot open /tmp/slashobserve-e2e-run.lock" when `/tmp` is not writable.
  The kernel drops the lock when the script exits, even after a crash, so there is nothing to clean up by hand; the file itself stays.
  Do not remove or rename the lock file while a run is active.
  After a kill -9, leftover child processes of the dead script can still hold the lock: it clears within seconds, or up to 90 seconds if the script was killed during an API call, and meanwhile the message says "pid N exited; a leftover child still holds the lock, retry shortly".
  A directory at that path, left by an older `scripts/e2e.sh`, fails with "stale directory from an older e2e.sh"; `rmdir` it.
  A symlink at that path fails with "is a symlink"; the script never follows one.
  SIGINT, SIGTERM and SIGHUP run the same cleanup and exit 130, 143 and 129; a script started in the background by a non-interactive shell ignores SIGINT, so stop it with SIGTERM.
  Every `docker compose exec` in the script uses `-T` with stdin from `/dev/null` (an inherited stdin once hung a run for 29 minutes), and `exec` (60 seconds), `ps` (30 seconds) and the failure `logs` dumps (30 seconds; 120 seconds for `scripts/e2e.sh logs`, which CI uploads as an artifact) run under a small perl wrapper in their own process group, since macOS has no `timeout`; `docker compose up` and `down`, `bun run e2e:install` and the Playwright run use the same wrapper without a timeout, so an interrupt stops the whole process tree.
  Polls run their commands in the main shell with output in files, so a signal is handled at once; a failed poll prints the last stderr of the command it polled, and every failure message and compose log dump, including `scripts/e2e.sh logs`, passes through a redaction filter for tokens, cookies, passwords and URL credentials (numeric token counts such as `"input_tokens":42` stay readable); the live output of `docker compose`, `bun` and Playwright is not filtered.
  The e2e stack also publishes Prometheus on 19090 (`SLASHOBSERVE_E2E_PROMETHEUS_PORT`) and the shop fault API on 18081 (`SLASHOBSERVE_E2E_SHOP_PORT`).
  CI builds the server image once as `slashobserve-e2e:ci` and the shop image as `slashobserve-demo-shop:ci` (`SLASHOBSERVE_SHOP_IMAGE`), and uploads Playwright traces, videos, the HTML report and compose logs when a test fails.
- `scripts/e2e.sh` boot waits, with bounded polling, until Prometheus has scraped every target and ClickHouse has log and trace rows, so later checks start on a stack that is already exporting; checks on telemetry that may still be arriving poll instead of asserting once.
  The shop contract (pods, routes, metric labels) is shared by the Prometheus backfill and the e2e checks, so change them together.
  The Prometheus entrypoint picks the backfill's `end` 20 seconds after it starts (generating and importing the backfill takes 5 to 8 seconds on an idle host) and writes it to `/prometheus/backfill-end`.
  The entrypoint removes any old `backfill-end` first, so the load generator waits for the new one.
  It regenerates when generation finishes less than 7 seconds before `end` (as on a CPU starved host) or when it finds a stale `.ready` from an earlier start whose `end` is not the one still published in `backfill-end` (when it is, a load generator may already be running on it, so the entrypoint reuses those blocks and that `end` instead of moving it), with a lead of twice the time the last try took plus 10 seconds, capped at 55 seconds because the load generator starts at once when `end` is more than its one minute `maxStartLead` away and would double count the backfill (`demo/shop/internal/loadgen/loadgen_test.go` fails when the two drift apart).
  It stops retrying after 4 tries or 90 seconds in total and warns that the junction will show a gap, so boot usually stays inside the Prometheus healthcheck window of about 210 seconds; a loaded host therefore boots later instead of starting Prometheus and the load generator after `end`, which would leave a gap of the missed seconds in the junction.
  The load generator mounts that volume read-only (`LOADGEN_START_FILE`), waits for the file and sends its first request exactly at `end` (or at once if `end` is more than a minute ahead), so it does not depend on Prometheus being up.
  Business counters and histograms are created on the first business request, so a live counter counts requests since `end` and the junction is a counter reset that neither double counts nor loses requests.
  Prometheus starts 5 seconds before `end`: its first shop scrape comes 5 to 10 seconds after it starts, so the first live sample lands at most about 5 seconds after `end` (a scrape before `end` would be dropped as out of bounds).
  A 1 minute `rate()` whose window starts after `end` still misses the requests before its first live sample, less what Prometheus extrapolates, so the junction shows a small dip.
  `backfill_joins_live` in `scripts/e2e.sh` compares, per `service`, `route` and `pod` above 1.5 requests per second, the 1 minute request rate and p95 around the junction with their values 35 seconds before `end`, and requires every 5 second sample from 30 seconds before to 90 seconds after `end` to stay within 0.85 to 1.15 for the rate and 0.95 to 1.6 for the p95.
  Every route's 95th percentile must sit inside a well populated histogram bucket whose upper edge is far above the slowest latency the route can produce, or CPU starvation (sleeps overshooting) moves it across the bucket: frontend `/api/products` used to sleep 5 to 25 ms, which put its p95 on the 25 ms edge and let it reach 1.7 on a loaded host.
  The p95 bucket must also hold at least 40% of the requests of every route the check covers, because a histogram quantile is interpolated inside its bucket and a thin bucket turns sampling noise and timer overshoot into p95 movement.
  With `F` the share of requests below the bucket, `n` the requests in the 1 minute window and nothing above the bucket, p95 is the lower edge plus the width times (0.95 - F) / (1 - F), its slope in `F` is the width times 0.05 / (1 - F)^2, and `F` has a standard deviation of sqrt(F (1 - F) / n); the backfill side has no noise.
  With every covered route above 40%, the 0.95 floor sits clear of that sampling noise instead of inside it.
  The profiles in `backfill.awk` are calibrated from the shop containers alone (`docker run`, 30 requests per second, 6 minutes), and the highest bucket of each profile is the diurnal tail that is zero at the junction.
  Per route at the junction, the share of requests in the p95 bucket, that bucket, and the slowest latency the sleeps allow (including about 1 ms of overhead):

  | Route | Sleeps | p95 bucket | Share in bucket | Slowest latency |
  | --- | --- | --- | --- | --- |
  | frontend `/api/products` | 10 to 35 ms | 25 to 50 ms | 43% | 36 ms |
  | frontend `/api/products/{id}` | 10 to 35 ms | 25 to 50 ms | 43% | 36 ms |
  | frontend `/api/orders/{id}` | 2 to 5 ms and 5 to 30 ms | 25 to 50 ms | 45% | 38 ms |
  | checkout `/orders/{id}` | 5 to 30 ms | 25 to 50 ms | 30% | 31 ms |
  | payments `/charge` | 30 to 70 ms | 50 to 100 ms | 52% | 72 ms |
  | checkout `/checkout` | 20 to 40, 30 to 70, 20 to 40 ms | 100 to 250 ms | 75% | 153 ms |
  | frontend `/api/checkout` | 2 to 8 ms plus checkout | 100 to 250 ms | 85% | 162 ms |

  Changing a sleep range changes its row, the rows of every route that calls it, and the profile in `backfill.awk`, so recalibrate them together.
  The p95 band stays wide above (1.6) and tight below, where a gap or a double count would show.
  The check waits until the window ending 90 seconds after `end` is 10 seconds old, then judges it once: a band violation fails at once with the series outside each band and its value (`query failed` when the re-query broke, `none on re-query` when it found none), next to the loadgen's dropped count, instead of polling out the full timeout.
  The `/orders/{id}` routes are left out because their low request rate swings the rate on seeded traffic alone.
  The backfill rates equal the load generator's steady state at the junction and the diurnal shape has zero offset and slope there.
  The sample spacing grows geometrically from 5 seconds at `end` to 30 seconds going back, and every timestamp carries milliseconds: whole-second spacing makes `rate()` extrapolation dip on the sparser samples.
  The cAdvisor job sets `honor_timestamps: false` because its `machine_*` series carry a fixed explicit timestamp from cAdvisor's start, which falls before the backfill's end and would be dropped as out of bounds, and cAdvisor runs with `--housekeeping_interval=5s` so every 15 second scrape sees counters of the same age and `rate()` over `container_cpu_usage_seconds_total` does not step.
- Dex runs in compose on host port `SLASHOBSERVE_DEX_HOST_PORT` (5556 for `make demo`, 15556 for e2e via `SLASHOBSERVE_E2E_DEX_PORT`).
  Dev logins are `admin@example.com` / `password` (allowed domain `example.com`) and `outsider@example.org` / `password` (always refused, for tests).
  `scripts/e2e.sh` has `dex_login` for curl-driven sign-in.
- Every change that a user can see or call adds or updates an e2e check in the same PR; a feature is not done until both suites cover it.
- `deploy/compose/datasources.e2e.yaml` (mounted through `SLASHOBSERVE_DATASOURCES_HOST_FILE`, which `scripts/e2e.sh` sets) adds `prom_tight` and `ch_tight` to `prom` and `ch` for deterministic limit checks; `make demo` uses `datasources.yaml`.
- `scripts/e2e.sh` `api_token` creates the service account `e2e-api` and sets `API_TOKEN` once, so API checks send `Authorization: Bearer $API_TOKEN`; request bodies live in `e2e/fixtures/api/`.
- E2E tests never call a live model: they use the replay LLM provider (`SLASHOBSERVE_LLM_PROVIDER=replay`, fixtures in `e2e/fixtures/llm/`).
  A fixture is one JSON file per prompt with `prompt` and `turns` (`text`, `tool_calls`, `expect`); a tool error not declared in `expect` fails the run.
  A revise fixture may leave `dashboard_id` and `parent_version` out of `update_dashboard`, so it never depends on generated ids; a comment or prompt with no fixture fails its revision with `model call failed: no replay fixture`.
  With the replay provider on, a comment queues a revision by default, so e2e comment checks that are not about revisions send `"request_revision": false`.
  `evals_smoke_suite` runs the eval runner on the scenarios tagged `replay-smoke` (`red-metrics-by-endpoint`, `log-volume-by-service`) and expects every check to pass, so those two prompts need fixtures.
  The runner exits 1 when the pass rate is below `--min-pass` (0.8) or, with all 10 `done_when` scenarios run, fewer than `--min-done-when` (8, or the number of `done_when` scenarios in a custom folder when fewer) pass, and 2 on a usage or setup error.
  Observe fixtures cover the brief's broken-query repair path (`observe-red-metrics.json` saves a query with an unbalanced parenthesis, gets `query_invalid`, and repairs it).
- Compose: add new services (Dex, Prometheus, ClickHouse, OTel Collector, demo/shop) to `deploy/compose/compose.yaml`; publish host ports only for what a browser or developer must reach, bound to 127.0.0.1 with a `${...}` override.
  Only `frontend-0` builds the shop image (`pull_policy: build`), so a plain `docker compose up` rebuilds the image named by `SLASHOBSERVE_SHOP_IMAGE` even when it is prebuilt; pass `--no-build` to keep it.

## Product rules

- The agent only ever outputs ObserveSpec JSON; no code path takes LLM-written HTML or JS into the UI.
- Datasource credentials never reach the browser; all panel data goes through the query proxy, which only runs queries from a saved Spec.
- Every query the agent runs is time-bounded and limited (series, rows, points, timeout) on the server, whatever the prompt says.
- Tools are defined once in `internal/agent/tools`; MCP and the built-in agent share them.
- The agent's text output is discarded; only tool-saved Specs leave a run.
- Placeholder panels come only from Grafana imports.
- Anything Enterprise will extend goes behind an interface in `pkg/extension`; core never imports Enterprise code.
- `razorpay/slashdocs` is a reference, not a dependency: never import its packages or copy whole files; credit any verbatim snippet in `NOTICE` under its MIT license.

## Workflow

- Make atomic commits: one self-contained change that builds and passes on its own; messages describe the change only, with no task numbers or co-author trailers.
- Do not commit ADRs, decision records or the brief; decisions live in this file, and implementation plans live outside the repository.
- Create repositories, push, and open PRs only after Ishan says go.
- CI branch protection must mark the `check` and `e2e` jobs as required.
- Markdown: one sentence per line; never use an em dash.
