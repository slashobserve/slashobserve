# Backend conventions

Rules for `internal/` and `cmd/`.
The rules for `pkg/` are in the root `CLAUDE.md`.

## Packages

This lists only the packages whose purpose the name does not make obvious.

- `config`: the only reader of `SLASHOBSERVE_*` environment variables (`config.Load`).
- `logattrs`: turns an error into log attributes that are safe to write.
- `db`: pgx pool, goose `migrations/`, sqlc `queries/`, generated `dbsqlc/`.
  `dbtest` gives each test its own migrated database from one Postgres testcontainer per test binary (`dbtest.Main` in `TestMain`, `dbtest.New(t)`; skipped under `-short`).
- `tctest`: retry for testcontainers starts that fail on testcontainers-go v0.44.0 reaper (ryuk) start errors; every container start in tests goes through it.
- `folders`: folders, permissions and the one place that decides who sees and changes a dashboard.
- `forbidden`: the `forbidden_role` code and its 403 texts, a leaf package so the CLI can compare against them without the server.
- `egress`: the only fetcher of user-supplied URLs; `egress/allowlist/` holds the outbound domain table, routes and the `SLASHOBSERVE_OUTBOUND_DOMAINS` lock.
- `queryproxy`: panel query runner shared by save validation and the proxy.
  `queryproxy/proxyhttp` holds its HTTP handlers, a subpackage because it reads versions through `dashboards`, which imports `queryproxy`.
- `agent`: the agent loop; `agent/tools` is the only place agent tools are defined; `agent/prompts` holds the embedded prompts.

## Secrets and configuration

- Never log secret values, the database URL and the OIDC client secret included.
  The LLM API key is never logged or echoed in an error.
- The datasources file never holds a secret.
  Passwords and tokens are referenced by `*Env` names that start with `SLASHOBSERVE_` or by `*File` paths.
- `SLASHOBSERVE_OIDC_ISSUER_URL` is used exactly as given because issuer comparison is exact.
  The issuer is the browser-facing URL, and `SLASHOBSERVE_OIDC_INTERNAL_URL` only rewrites the host of the server's back-channel.
- The CLI sends its saved key only to the server it was saved for.

## HTTP, MCP and auth

- The strict `Content-Security-Policy` (`contentSecurityPolicy` in `internal/server/server.go`) allows images only from self, `data:` and `blob:`.
  Remote images, tiles and GeoJSON must load through `/api/v1/assets/image` and `/api/v1/assets/geojson`.
- `/mcp` is mounted through `server.Options.Mounts` outside `/api/v1`, so the API contract check does not cover it.
  It accepts only `Authorization: Bearer` service-account keys (a session cookie is ignored) and refuses an `Origin` other than `SLASHOBSERVE_PUBLIC_URL`.
- `Tool.MCPOnly` tools are left out of the model's tool list by `agent.New`.
  A comment tool called inside an agent run is rejected with `invalid_request`, because the server answers and resolves threads after a run; comment tools never queue revisions.
- A tool's `MinRole` is checked when the tool is called, not when tools are listed, and only for keys outside agent runs.
  Inside a run the requester's folder level decides.
- Every `/api/v1` route except `/api/v1/auth/*` is registered through `auth.Routes` with its lowest role (`auth.RequireRole`).
  Feature packages register through `Register(api auth.Routes)` and never see a raw gin group.
  Handlers never check roles.
  `auth.RequireUser` is a separate principal-kind check after the role, so a key cannot manage keys whatever its role.
- The OIDC `groups` scope is requested only when discovery lists it in `scopes_supported`.
- Sessions and keys are opaque random tokens stored only as SHA-256.
  Cookie-authenticated mutations pass `http.CrossOriginProtection`.
- Origin is a display label and never an access decision.

## Folders and access

- `folders.LevelFor` is the one rule for what a caller may do with a dashboard, shared by web, API, MCP, CLI and agent runs.
  Stored rows are never rewritten when a role changes.
- Every dashboard lives in exactly one folder.
  `dashboards.folder_id` defaults to General through `general_folder_id()`, so raw inserts land there.
- A folder with no permission rows is open, and each person gets the level of their role.
  A folder with rows is restricted and invisible to anyone with no row (by user or by `groups` name) who did not create it.
  A hidden folder hides its dashboards everywhere.
- A grant lifts a Viewer or Commenter and never lowers anyone: the level is the higher of the role's level and the highest grant, so an Editor with a View grant still edits.
- Service accounts match no grant, because rows hold users and groups.
  A key sees a restricted folder only as an Admin or as its creator (`created_by_kind`, `created_by_id`).
- General is never restricted, renamed or deleted.
- A dashboard in a folder the caller cannot see answers 403 `forbidden_role` with `forbidden.AccessMessage`, and a visible one with too low a level answers 403 with `forbidden.RoleMessage`.
  An invisible folder answers 404 `not_found`, and tools answer `not_found` for both.
- Every HTTP route keyed by a dashboard id declares `RequireDashboard`.
  List queries filter by `Levels.Visible()` and fail closed, services check writes themselves, and every count (lists, tag counts, `dashboard_count`) covers only folders the caller can see.
- An agent run is checked when it is enqueued and again when a worker claims it, after reloading the requester.
  The event stream re-reads the caller's access at every ping.
- Deleting a dashboard is permanent: foreign keys cascade its versions, comments, agent requests and import record.
  Inserts that lose the race against a delete map the foreign-key error (`db.IsForeignKeyViolation`) to 404.

## Database and dashboards

- Migrations are goose SQL files `internal/db/migrations/NNNNN_snake_case.sql`, numbered sequentially; never edit a merged migration.
- Migration 10 is a Go migration (`internal/db/specupgrade.go`, registered in `newMigrator`) with no file, so SQL numbering skips `00010`.
  A new SQL file takes the next number above both the highest file and every Go migration version, because a file reusing a Go migration's version stops the server at boot.
- Versions are immutable and linear (`parent_version = version - 1`).
  A save validates with `spec.Parse`, then runs every panel query through `internal/queryproxy` outside any transaction.
  The conditional `latest_version` update is the concurrency check (409 `version_conflict`).
  Events go out with `pg_notify` inside the saving transaction.
- `CarryUnverified` (`internal/dashboards/unverified.go`) is the only rule for `dashboard_versions.unverified`.
  Restore and duplicate copy the source version's list and run no query.
- `dashboard_versions.datasource_uids` is a generated column from the SQL function `observespec_datasource_uids(spec)`.
  Whenever the Spec moves datasources, change that function with `CREATE OR REPLACE` in a new migration that then runs `UPDATE dashboard_versions SET spec = spec`.

## Events, revisions and comments

- Event payloads carry ids and a capped author name, never comment text, so they stay below the 8000 byte `pg_notify` limit.
- `dashboard_versions.agent_request_id` is unique, and the agent's `create_dashboard` and `update_dashboard` save through it, so a retried run returns the version it already created.
- A saved version always wins: before a failure is recorded, and when the lease sweep exhausts a request, the request counts as succeeded with the version saved under its id.
- A request from a comment resolves the open thread in the same transaction.
  A resolution that errors runs in a savepoint and is logged as `agent_request_thread_resolve_failed`, so the request still records its success.
- Without an LLM provider, comments do not enqueue, so turning an agent on later never replays old comments.
- An observe request has no dashboard until it finishes.
  `dashboard_id` is filled from the version saved under the request id, so queue lookups of a saved version use the request id only.

## Query proxy and datasources

- The proxy accepts no query text (403 `query_not_in_spec`).
  The one exception is `POST /api/v1/datasources/{uid}/preview`, which places a signed-in user's draft panel into a saved version with `spec.WithPanel`, runs it with `DraftLimits` (which apply to every query whose text did not come from a saved Spec), never caches or coalesces it and refuses a bearer key (403 `user_session_required`).
- The proxy's caches are shared by everyone, so a version whose queries reference `${__user.*}` is cached per principal.
  The runner that validates saves stays uncached.
- `spec/examples/time/resolve-cases.json` keeps the Go and TS relative time resolution in step.
- Variable values reach queries only through `internal/datasource/macros`.
- A Spec names a datasource by uid or as `${name}` naming a datasource variable.
  `spec.DatasourceVariableName` is the only parser.
  A literal uid's type comes from the registry and a `${name}` reference's from the variable's `datasourceType`, so the query proxy, not `spec.Parse`, rejects a macro the type lacks (`unknown_macro`).
- `GET /api/v1/datasources` never returns a URL, address, user or secret.
  The health `message` is a fixed string chosen by error kind, never the error's own text.
  Datasources never gate `/readyz`.
- ClickHouse is queried only as a `readonly = 2` user with `SELECT` on the OTel database, and the server refuses a user that is not read-only.
  The compose user `slashobserve_ro` is created by `deploy/compose/clickhouse/initdb/10-readonly-user.sh` from `internal/datasource/clickhouse/testdata/readonly_user.sql`, the same definition the integration tests use.
- Any feature that fetches a URL taken from user content goes through `egress.Client`, never its own `http.Client`.
- A rule that lives only in a schema description belongs in the authoring guide, because the model reads `prompts.Schema()`, which strips the annotations.

## Logging

- `log/slog`; request logs record the path, never the query string.
- The server never logs secrets, prompt text, query text, variable values, tool results or MCP client names.
  The `asset fetch failed` and `action run` lines never hold URLs, paths, queries, bodies or action inputs.
- A failure is a fixed message plus `logattrs.Failure(code, err, ids...)` and the relevant ids, never `err.Error()`, and a panic is logged by its type only.
- `sloglint` forbids the literal attribute keys `error` and `err` under `internal/`, `cmd/` and `pkg/`.
  It cannot see a key built through a variable or helper, or `err.Error()` under another key, so review those by hand.
- A fatal startup error keeps its text only through `described` in `cmd/slashobserve-server/main.go`, for messages the server wrote from settings; never echo a URL setting.
- Handlers pass an error through `clientgone.Handle` before logging at ERROR or answering 500, because a caller that went away mid-request is not a server failure (a cancellation on a live request context still is).

## API contract

- Every `/api/v1` route is documented in `api/`.
  Only the e2e stack runs with `SLASHOBSERVE_API_CONTRACT_CHECK=true`, which turns a request or response that breaks the document into a 500 `contract_violation` and makes `scripts/e2e.sh` fail; it never belongs in production.
- Document every status a handler can return, and keep `additionalProperties: false` on response objects so new fields must be documented.
- Wire conventions: JSON field names are `snake_case` (the Spec document inside a response keeps its camelCase), times are RFC 3339 UTC strings, errors are `{"error","code"}` plus optional named fields, 401s carry `WWW-Authenticate: Bearer`, and lists use opaque `cursor` and `next_cursor` (null at the end) with `limit`.

## Tests

- Integration tests use real Postgres, Prometheus and ClickHouse through testcontainers, never mocks.
