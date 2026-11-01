# End-to-end tests

Rules for both suites: `scripts/e2e.sh` (curl-driven checks against the compose stack) and Playwright in `web/e2e`.
A change adds no new check unless the owner asks for one (see Workflow in the root `CLAUDE.md`).

## Playwright

- Specs import `test` and `expect` from `web/e2e/fixtures.ts`, which fails any test whose page logs a console error.
- The `setup` project signs in through Dex once and saves `web/e2e/.auth/user.json`.
  A spec that signs out must opt out with an empty `storageState` and sign in itself.
- Never send a real `DELETE /api/v1/users/{id}/sessions` for the shared e2e user, because it ends the session every later spec uses; mock it.

## The e2e stack

- It is the compose project `slashobserve-e2e` on host port 18080, reset with `down -v` on every `make e2e`, so it never touches a developer's `make demo` stack.
- `scripts/e2e.sh` holds an exclusive kernel `flock` on `/tmp/slashobserve-e2e-run.lock` for a whole run.
  The path is fixed because `TMPDIR` differs between shells and agents.
  Do not remove or rename the file while a run is active.
- Every `docker compose exec` in the script uses `-T` with stdin from `/dev/null`, because an inherited stdin once hung a run for 29 minutes.
- Checks on telemetry that may still be arriving poll with a bound instead of asserting once.

## Replay model

- E2E tests never call a live model.
  They run the replay provider (`SLASHOBSERVE_LLM_PROVIDER=replay`) with fixtures in `e2e/fixtures/llm/`.
- A fixture is one JSON file per prompt with `prompt` and `turns`; a tool error not declared in `expect` fails the run, and a prompt or comment with no fixture fails its revision.
- The scenarios tagged `replay-smoke` (`red-metrics-by-endpoint`, `log-volume-by-service`) run in `evals_smoke_suite`, so those two prompts need fixtures.

## Demo data

- The shop contract (pods, routes, metric labels) is shared by the Prometheus backfill and the e2e checks; change them together.
- The p95 of every route that `backfill_joins_live` covers must sit in a well populated histogram bucket (at least 40% of the route's requests) whose upper edge is far above the slowest latency the route can produce.
  Otherwise CPU starvation (sleeps overshooting) moves the p95 across the bucket and out of the check's band.
  Changing a sleep range in `demo/shop` changes that route's bucket, the buckets of the routes that call it and its profile in `deploy/compose/prometheus/backfill.awk`, so recalibrate them together.
- Backfill samples are spaced from 5 seconds at `end` to 30 seconds going back, with milliseconds on every timestamp, because whole-second spacing makes `rate()` extrapolation dip.
- The cAdvisor job sets `honor_timestamps: false` because its `machine_*` series carry a fixed timestamp that falls before the backfill's end and would be dropped as out of bounds.
  cAdvisor runs with `--housekeeping_interval=5s` so `rate()` of `container_cpu_usage_seconds_total` does not step.
