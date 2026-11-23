# End-to-end tests

Rules for both suites: `scripts/e2e.sh` (curl-driven checks against the compose stack) and Playwright in `web/e2e`.
A change adds no new check unless the owner asks for one (see Workflow in the root `CLAUDE.md`).

## Playwright

- Specs import `test` and `expect` from `web/e2e/fixtures.ts`, which fails any test whose page logs a console error.
- The `setup` project signs in through Dex once and saves `web/e2e/.auth/user.json`.
  A spec that signs out must opt out with an empty `storageState` and sign in itself.
- Never send a real `DELETE /api/v1/users/{id}/sessions` for the shared e2e user, because it ends the session every later spec uses; mock it.

## The e2e stack

- It is the full OpenTelemetry Astronomy Shop of `deploy/astronomy`, run through `scripts/astronomy.sh compose` with the extra layer `deploy/astronomy/compose.e2e.yaml`, plus the server and Dex.
  The compose project is `slashobserve-e2e` on host port 18080 (override it with `SLASHOBSERVE_E2E_PROJECT` and the `SLASHOBSERVE_E2E_*_PORT` variables), reset with `down --volumes` on every `make e2e`, so it never touches a developer's `make astronomy-build` stack.
- The shop's state (its flag file and key) lives in `deploy/astronomy/.state/<project>`, apart from the developer stack's and from the state of an e2e stack of another project, so parallel stacks never share a flag file or a key, and one run's `down --volumes` cannot wipe another's.
  The live fault spec derives the same directory from `SLASHOBSERVE_E2E_PROJECT`, so a direct `bun run e2e live-fault` edits the e2e stack's flag file and never the developer stack's.
  `SLASHOBSERVE_IMAGE` runs a prebuilt server image: `deploy/astronomy/compose.prebuilt.yaml` replaces the build layer, and compose pulls the image when it is missing.
- `scripts/e2e.sh` holds an exclusive kernel `flock` on `/tmp/<project>-run.lock` for a whole run.
  The path is fixed because `TMPDIR` differs between shells and agents.
  Do not remove or rename the file while a run is active.
- Every `docker compose exec` in the script uses `-T` with stdin from `/dev/null`, because an inherited stdin once hung a run for 29 minutes.
- Checks on telemetry that may still be arriving poll with a bound instead of asserting once.
- `warm` starts every service except the server and writes `/tmp/<project>-warm` (`pending <pid>`, then `ready <epoch>`); `boot` consumes that marker once and skips its reset when it is `ready`, under 30 minutes old, with Prometheus running and no server container, and `run`, `up` and `browser` wait up to 10 minutes for the lock while a live `warm` holds it; `down` removes the marker.
- `boot` waits for the shop's span metrics and for logs and traces in ClickHouse, and places orders through the shop port if checkout has none yet.
  A check that reads four minutes of Prometheus history (the imports) calls `await_history` first and runs near the end of `run`, so the wait elapses while other checks run.
- Faults are upstream flagd flags, set with `scripts/astronomy.sh flag <name> <variant>`.
  The API suite and the Playwright live fault spec both set `paymentFailure` and reset it when they finish, so run them one after the other (`make e2e-all` does) and never at the same time against one stack.
  The API suite's fault checks read only the last two minutes of metrics, traces and logs, so an error from an earlier run does not satisfy them.

## Replay model

- E2E tests never call a live model.
  They run the replay provider (`SLASHOBSERVE_LLM_PROVIDER=replay`) with fixtures in `e2e/fixtures/llm/`.
- A fixture is one JSON file per prompt with `prompt` and `turns`; a tool error not declared in `expect` fails the run, and a prompt or comment with no fixture fails its revision.
- A turn may carry `"fault": {"kind": "...", "delay_ms": N}`, and a turn with a fault needs no `text` or `tool_calls`.
  `rate_limited`, `server_error` and `call_timeout` fail the model call once with status 429, 500 or none, then the same turn answers on the next call.
  `max_tokens` and `refusal` end the turn with that stop reason.
  `delay` waits `delay_ms` (honouring cancellation), then answers the turn.
- A session that restarts mid-build resumes at `resumes."<k>"`, where k is the build's save count, and a missing segment fails the revision.
  `"resume_after_saves": true` replaces the segments: a session started with k saves resumes after the k-th turn that holds a `create_dashboard`, `update_dashboard` or `update_panels` call, so a long build needs no segment per save.
  Every save in such a fixture is assumed accepted, because the position is counted in saves and not in turns.
  The fixture is refused with `resumes`, with more than one save call in a turn, and with `expect` on the turn after a save.
- Replay reports `output_tokens` for each turn as the tool call input bytes divided by 3.2, rounded up.
  A turn whose estimate exceeds `SLASHOBSERVE_LLM_MAX_TOKENS` ends as `max_tokens` and its tool calls do not run, so keep an e2e fixture's tool call input well under that limit (16,000 tokens is about 51 KB).

## Shop data

- The shop contract is what the checks and fixtures select: span metrics `traces_span_metrics_calls_total` and `traces_span_metrics_duration_milliseconds_bucket` (labels `service_name`, `span_name`, `span_kind`, `status_code`, `job`), service metrics such as `http_server_request_duration_seconds_bucket`, container metrics labelled `container_name`, host `system_*` metrics, traces and logs in ClickHouse (`otel.otel_traces`, `otel.otel_logs`), and the services `frontend`, `checkout`, `payment`, `cart` and their neighbours.
  Change the checks and the fixtures together when the collector pipeline changes.
- Prometheus ingests everything over OTLP and the collector pushes every 60 seconds, so the e2e datasources declare `scrapeInterval: 60s` and a rate needs at least two pushes in its window.
  Use `$__rate_interval` or a window of three minutes or more, never `[1m]`.
- The stack has no backfill and no `up` metric, so a check that needs a long history cannot run, and a series a dashboard needs must come from the traffic the shop and `place_order` generate.
- The "pod" dimension of the old shop is `service_name` over `checkout|payment` in the Playwright data, and the "route" dimension is `span_name`.
