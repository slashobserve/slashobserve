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
- A shared folder with no permission rows is open, and each person gets the level of their role.
  A folder with rows is restricted and invisible to anyone with no row (by user or by `groups` name) who did not create it.
  A hidden folder hides its dashboards everywhere.
- A grant lifts a Viewer or Commenter and never lowers anyone: the level is the higher of the role's level and the highest grant, so an Editor with a View grant still edits.
- Service accounts match no grant, because rows hold users and groups.
  A key sees a restricted folder only as an Admin or as its creator (`created_by_kind`, `created_by_id`).
- General is never restricted, renamed or deleted.
- Every person has one personal folder (`folders.personal`, `owner_id`), made by the `users_personal_folder` trigger and backfilled by its migration; `LevelFor` reads it first: the owner has Edit whatever their role and nobody else sees it, Admins included.
  It has no permission rows, nobody manages it, so it is never renamed, shared or deleted, and service accounts have none.
  Deleting a user deletes their personal folder and its dashboards (`users_personal_dashboards` trigger and `ON DELETE CASCADE`), as it does their saved prompts.
- A dashboard in a personal folder is private, and publishing it is a move out; `folders.MayMoveInto` is the one rule that a personal folder takes no moves, `dashboards.Service.move` is its only caller, and `ErrMoveIntoPersonal` maps to `409 personal_folder`.
  The web row menu hides personal folders as move targets as a display copy.
- `pickCreationFolder` is the one default folder for every create, agent commits and Save what was built included: requested, else the caller's personal folder, else General, else 403.
  A key's default is therefore General; nothing remembers where a caller created before.
- Folders sort by `(NOT personal, general, lower(title), id)` in `FolderFacts`, `ListFolderDetails`, `ListDashboardsByFolder` and `folderCursor`; change them together.
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
  `AddChangedVersion` (the `update_panels` save) runs only the changed panels and variables, as `ChangedSince` decides against the stored parent, never against what the caller claims, unless `needsFullCheck` says a `time` change or a title used by `$__dashboard` moved every result, and then it runs `checkSave`.
  An unchanged panel that started failing stays verified until a full save runs.
  The conditional `latest_version` update is the concurrency check (409 `version_conflict`).
  Events go out with `pg_notify` inside the saving transaction.
- A save that changes an existing panel's `w` or `h` sets `responsive: false` on it in `freezeResized` (`internal/dashboards/responsive.go`), called from `persistVersion` before the replay check and from `commitRevision`; creates, imports, duplicates and restores never freeze, and the server never re-sizes a panel because of the flag.
- `agent_request_builds` holds one build per agent request: the canonical Spec, the carried `unverified` list, `planned_panels` and `panels`, so nothing parses the Spec to show progress.
  Every build write is fenced by the attempt (`status = 'running' AND attempts = @attempt`), so a worker that lost its lease writes nothing.
  Every build save with a parent is a partial check, as `ChangedSince` decides against the build, or against the base version for the first save of a revision, and placement moves change only `gridPos`, so they run no query.
  `CommitBuild` is one transaction that writes the version and deletes the build, and it runs no query because every build save was already checked.
  A create build passes through `closeLines` (`internal/dashboards/closelines.go`) in `commitBuild`, the one funnel of every commit path; revisions do not.
  A conflict at commit, or a session about to start on a build whose base is older than the latest version, discards the build, and the run restarts once from the latest version.
- A failed request keeps its build for `revisions.BuildRetentionDays`, and `claim` deletes the expired ones beside `FailExhaustedAgentRequests`.
  A lost lease or a shutdown release keeps the build, and the next holder re-seeds from it.
- `internal/revisions` cannot import `internal/dashboards`, so it takes the build through its `Builds` interface and `cmd/slashobserve-server/main.go` adapts the service with `buildStore`.
- `finish`, `planned_panels` and `row` exist only in the agent's tool schemas, through the `build` variant in `internal/agent/tools/wording.go`, and the agent's `update_panels` takes at most one item in `upsert`; a build save adds or changes at most one non-row panel (`onePanelPerSave`), and the server owns every position in a build: `x` and `y` from a panel body are never used, an unchanged panel keeps its stored `gridPos`, and a new, resized or re-rowed panel is placed by `place` (`internal/agent/tools/place.go`, first free spot of its row, a free `layout` spot first) through a panel patch, so `spec.ApplyPanelPatch` stays the only merge.
  A size field the model leaves out is chosen by `chooseSize` (`internal/agent/tools/sizes.go`, defaults by type plus the open line); a sent size is never changed.
  An MCP save that sends these fields is refused and keeps one version per accepted save.
  `import_grafana_dashboard` stays a direct first save and is refused in a run that already has a build.
- `CarryUnverified` (`internal/dashboards/unverified.go`) is the only rule for `dashboard_versions.unverified`; `CarryUnchanged` is its partial-check sibling, and `ChangedSince` reads the same `panelUnchanged` and `variableUnchanged`, which compare query text and datasource, the variables a panel reaches, and its `type`, `timeFrom`, `timeShift`, `maxDataPoints` and `interval`, so a title or `gridPos` edit is no change.
  Restore and duplicate copy the source version's list and run no query.
- `dashboard_versions.datasource_uids` is a generated column from the SQL function `observespec_datasource_uids(spec)`.
  Whenever the Spec moves datasources, change that function with `CREATE OR REPLACE` in a new migration that then runs `UPDATE dashboard_versions SET spec = spec`.

## Events, revisions and comments

- Event payloads carry ids and a capped author name, never comment text, so they stay below the 8000 byte `pg_notify` limit.
- `dashboard_versions.agent_request_id` stays unique: an agent request commits at most one version, so a retried run or a second commit returns the version it already created.
  The agent's `create_dashboard`, `update_dashboard` and `update_panels` change the build, never the history.
- A saved version always wins: before a failure is recorded, and when the lease sweep exhausts a request, the request counts as succeeded with the version saved under its id.
- A worker holds its request by a `LeaseTTL` lease (60 s) that it renews every 5 s while the run is in flight; a renewal that affects no row cancels the run and drops the attempt's work without finishing or releasing it, and a transient renewal error is retried at the next beat until it has lasted a full TTL.
  The renewal returns `cancel_requested_at`, and the first beat that sees it cancels the run's context and keeps beating until the attempt is finished, so a cancel takes effect within about one beat plus the in-flight call.
- Cancel is a flag, not a registry or a notification: `POST /api/v1/observe/{id}/cancel` sets `cancel_requested_at` and fails a queued request at once with code `canceled`, and a running one is finished `canceled` by its worker, build kept.
  `settlementFor` never settles a crashed run as canceled, and the canceled outcome is recorded after the saved-version check, so a version saved before the cancel took effect still makes the request `succeeded`.
- Only the requester may cancel or continue a request, not an editor and not an admin: `canCancel` in `internal/revisions/control.go` is the one rule, `requesterRow` answers anyone else 403 `forbidden` (404 for a new-dashboard request they cannot see), and `can_cancel` on `GET /observe/{id}` is computed from it.
  A dashboard runs one non-suggest request at a time, so a stuck revision holds back every other editor's revisions on it, and they queue behind it up to `MaxActive`.
  It ends only by its own limits, the requester's cancel, `SLASHOBSERVE_AGENT_RUN_TIMEOUT` when set, or the lease path after a crash; a graceful restart puts it back in the queue.
  Add no way for anyone else to stop a run.
- Continue (`POST /api/v1/observe/{id}/continue`) is a new request with `continues_id` set, and the stopped build moves to it with `MoveAgentRequestBuild` in the same transaction as the insert; the stopped request stays `failed`, so attempts only grow and every fenced write keeps working.
  The build row is the token that keeps Continue and save exclusive: `MoveAgentRequestBuild` needs `build_saved_at IS NULL`, `SaveStoppedBuild` returns `ErrBuildNotSavable` unless its delete removes exactly one build row, and the loser answers 409 `not_continuable` for Continue after a save, or 404 for a save after Continue because the build moved.
  A unique index on `continues_id` allows one continuation, and a revision whose dashboard has a newer version than the build's base answers 409 `version_conflict`.
- The continue note is optional, validated by `CleanContinueNote` (the `CleanPrompt` rule), stored only in `continue_note` on the continuation, and written into every seed of the continued run (continue, restart and conflict) after the seed sentence.
  The continue sentence (`seedContinue`) is used only on attempt 1 of a continuation, through `resumeReason`; a later attempt seeds as a restart.
- `queue_position` is computed on read (1 plus the queued requests earlier in `(created_at, id)`, null unless queued), never stored.
- `ListAgentRequestBuildState` is the only reader of build progress and `continued_by` for responses, one batch per page; the revisions list and the request list carry them for every row, and `buildState.progressFor` is the one place that nulls the build `title` for anyone but the requester.
- `SLASHOBSERVE_AGENT_RUN_TIMEOUT` defaults to no limit, and a set value is at least `1m`; the evals stack sets `120m` so its failures stay coded.
  `SLASHOBSERVE_LLM_REQUEST_TIMEOUT` accepts `0` for no limit too, and the run limit is config's decision, so the worker has no default of its own.
  A run has no model-call cap, and without a run limit it ends at a commit, a failure, shutdown or lease loss.
  The progress guards steer first and stop after: 3 rejected saves in a row (an accepted save resets the count); reads refused from the sixth model call of a save window (12 in suggest mode), except beside a save and in the call after a rejected save, with a nudge one call before; `no_progress` after two turns in a row in which every call was refused; a repeated save and a third identical read are refused; errors and `query_range` or `query_instant` results are never replayed; and the churn rule, `maxChangesPerPanel` 4 and `maxChurnedSavesInRow` 3, which measures progress by content (`Result.Edits`, from `editsOf` in `agent/tools/build.go`: any panel JSON, variable or dashboard-level field that differs from the build before, so a description-only save is progress) and not by `ChangedSince`, which only decides re-verification.
  A save that deletes a panel does not count the panels it adds as progress (`Edits.Added` and `Edits.Deleted`; they still count toward the per-panel changes), so one replacement passes within the 3 tolerated saves and repeated delete-and-add swaps end in `no_progress`.
  A session is compacted, not renewed: after an accepted save past `SLASHOBSERVE_AGENT_SESSION_RENEW_KIB`, on the first cut-off (`max_tokens` or `call_timeout`), and once per run when the model answers without a tool call while planned panels are missing (`Compact` then drops that answer so the session continues), `llm.Session.Compact` keeps the last 4 turns within 32 KiB (results cut to 2 KiB, thinking kept only on the last assistant turn) behind a new first message with the build, the seed sentence and the discovery digest (`internal/agent/digest.go`, 4 KiB, never logged).
  The renewal size is `SLASHOBSERVE_AGENT_SESSION_RENEW_KIB` (default 256, 32 to 4096), and its default lives only in `defaultSessionRenewBytes`.
  A second cut-off with no accepted save in between ends the run with its code.
  A provider session starts only at the start of an attempt and after a conflict restart, never as a recovery.
  Request-side pruning replaces old read results with stubs in what is sent once older prunable results reach 64 KiB and keeps the last 4 assistant turns whole; the stored history keeps everything.
  `dashboards.ErrBuildLost` is a lease loss: the reviser returns `revisions.ErrLeaseLost`, and the worker drops the attempt's work like a lost heartbeat.
- A shutdown release keeps the steps, so the re-claim continues the same step list (the most recent `MaxSteps`, 1000, per attempt).
- A stopped build is saved through `POST /api/v1/observe/{id}/build/save`, only by the requester.
  The request stays `failed` with its failure code, and the same transaction sets `build_saved_at`, `result_version` and `dashboard_id`, so `succeeded` always means the agent finished.
  A newer version on a revision answers 409 `version_conflict` and keeps the build; nothing rebases it.
- `SLASHOBSERVE_LLM_EFFORT` is validated per provider in config and reaches an OpenAI-compatible provider only when set; `llm.Tuning.ForCall` raises it one step for the planning call and halves `max_tokens` (floor 8192) for the others.
  `SLASHOBSERVE_LLM_IDLE_TIMEOUT` (default 180 s, `0` off) is the idle watch of every streamed call, and `SLASHOBSERVE_LLM_REQUEST_TIMEOUT` covers `llm.Retry`'s attempts.
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
- The second exception is `GET /api/v1/observe/{id}/build/panels/{panel_id}/data` and `.../build/variables/{name}/values`: they load the requester's stored build on the server (`proxyhttp/build.go`), run under `DraftLimits`, never cache or coalesce, and refuse a bearer key (403 `user_session_required`).
- `spec/examples/time/resolve-cases.json` keeps the Go and TS relative time resolution in step.
- Variable values reach queries only through `internal/datasource/macros`.
- A Spec names a datasource by uid or as `${name}` naming a datasource variable.
  `spec.DatasourceVariableName` is the only parser.
  A literal uid's type comes from the registry and a `${name}` reference's from the variable's `datasourceType`, so the query proxy, not `spec.Parse`, rejects a macro the type lacks (`unknown_macro`).
- `GET /api/v1/datasources` never returns a URL, address, user or secret.
  The health `message` is a fixed string chosen by error kind, never the error's own text.
  Datasources never gate `/readyz`.
- ClickHouse is queried only as a `readonly = 2` user with `SELECT` on the OTel database, and the server refuses a user that is not read-only.
  The compose user `slashobserve_ro` is created on first boot by the distroless image's `docker-init` from `internal/datasource/clickhouse/testdata/readonly_user.sql`, mounted into `/docker-entrypoint-initdb.d`: the same definition, with the same fixed development password, the integration tests use.
- Any feature that fetches a URL taken from user content goes through `egress.Client`, never its own `http.Client`.
- A rule that lives only in a schema description belongs in the authoring guide, because the model reads `prompts.Schema()`, which strips the annotations.

## Logging

- `log/slog`; request logs record the path, never the query string.
- The server never logs secrets, prompt text, query text, variable values, tool results or MCP client names.
  The `asset fetch failed` and `action run` lines never hold URLs, paths, queries, bodies or action inputs.
- The one exception to the provider-message rule is `SLASHOBSERVE_LLM_LOG_PROVIDER_ERRORS`: when on, `llm.CallError.ProviderMessage` (scrubbed of the key and control characters, capped at 1024 bytes, never in `Error()`) is logged as `provider_error_message` on a failed `llm call finished` line.
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
