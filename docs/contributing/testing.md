---
title: Testing
description: How to run the unit, component, API and browser suites, the two-server checks and load test, coverage and the docs lint, and what CI runs for each change.
---

SlashObserve has unit tests, component tests, an API suite, a browser suite and a docs check.
This page shows how to run each one on your machine and what CI runs for a pull request.
Start with [Check your change before you push](#check-your-change-before-you-push); the end-to-end suites and the load test need Docker and take longer.

## Check your change before you push

1. Run `make harness` once, so a server built from source can find `slashobserve-harness`.
   When a model provider is configured, the server starts `slashobserve-harness` and fails to start if it is not on the path.
2. Run `make test`.
   It runs every unit suite in [Run the unit tests](#run-the-unit-tests).
3. Run `make lint`.
   It runs the Go, web and agent linters, the docs lint and the Helm chart checks.
4. Run `make ci` for what the CI `check` job stands for: `make lint`, `make test`, a `go mod tidy -diff` check and `make build`.

Run one part of the unit tests on its own while you work.
The browser and API suites are not part of `make ci`; see [Run the API suite](#run-the-api-suite) and [Run the browser suite](#run-the-browser-suite).

## Run the unit tests

`make test` runs these, one after the other:

- `go test -race ./...`, with a 20 minute timeout
- the web Vitest suite
- `bun test` for `slashobserve-harness`
- the Python unit tests of the benchmark adapter, which need `uv`; see [Running benchmarks](benchmarks.md)
- the tests of the release, install, docs lint, CI change, Astronomy Shop and replicas scripts

Run one part with `make test-go`, `make test-web`, `make test-harness`, `make test-benchmarks` or `make test-scripts`.
`make test-go GO_TEST_PKGS=./internal/db/...` narrows the Go packages.
From the repository root you can also run `go test -race ./...`, and from `web/` you can run `bun run test`.

Go tests sit next to the code as `*_test.go` files.
Web tests for pure helpers sit next to the code as `*.test.ts` files.
Tests that need a database or a datasource start the real store through testcontainers, so Docker must be running.

## Write a component test

Web components are tested with Vitest, Testing Library and happy-dom, in `*.test.tsx` files next to the component.
They run in the same `bun run test` as the unit tests.

1. Render the component with `renderWithProviders` from `web/src/test/render.tsx`.
   It wraps the component in a fresh TanStack Query client with retries off and returns a `user` from `user-event` for clicks and typing.
2. Pass `queryClient` to seed the cache.
3. Pass `route`, a route `path` and a starting `url`, when the component reads or writes the URL.
   The component then renders inside a memory router.
4. Stub only what the test cannot run, such as `fetch` with `vi.stubGlobal`.
5. Find elements by role and accessible name, the way a person using a screen reader would.

`web/src/test/setup.ts` adds the Testing Library matchers, unmounts each test's components and stubs the browser APIs that happy-dom lacks.

## Run the API suite

The API suite checks the HTTP API, the MCP endpoint, the CLI and the shop's telemetry with real requests, against a full stack.

1. Run `make e2e`.
   It resets the isolated `slashobserve-e2e` compose stack, builds the server image from your checkout, boots the stack, and runs `scripts/e2e.sh`.
2. Read the output.
   Each check prints a `PASS` line, and the first failure prints a `FAIL` line and stops the run.
3. Leave the stack running if you want the browser suite next.
   Stop it with `scripts/e2e.sh down`, which also removes its volumes, and read its container logs with `scripts/e2e.sh logs`.

The stack is the OpenTelemetry Astronomy Shop of `deploy/astronomy` plus the server and Dex.
The server answers on http://localhost:18080, and the shop on http://localhost:18081.
The shop's images are the ones `make demo` pulls, so the first run downloads them.
The stack has its own compose project, ports and state, so it runs beside your `make demo` stack and never touches it.

The suite runs the replay model provider, so it never calls a live model.
Replay answers each prompt from a fixture in `e2e/fixtures/llm/`; `e2e/CLAUDE.md` describes the fixture format.

The suite checks:

- the API, the MCP endpoint and the CLI
- that every datasource the stack provisions reports healthy
- the shop's span metrics, container metrics and host metrics in Prometheus
- the shop's logs and traces in ClickHouse, and that the ClickHouse user is read-only
- the Grafana imports
- a `paymentFailure` fault, which has to show in metrics, traces and logs
- that every API call matched the OpenAPI contract, which the server checks while the suite runs
- that the server log holds no service-account key

The fault checks read only the last two minutes of data, so an error from an earlier run does not satisfy them.

## Run the browser suite

The browser suite runs the Playwright specs in `web/e2e` against the e2e stack.

1. Run `make e2e-browser`.
   It reuses the running, healthy e2e stack, or boots one, installs Chromium, and runs the specs.
2. When the specs finish, it also checks the OpenAPI contract and the server log, as the API suite does.
3. After a code change, run `make e2e` again first, because reuse does not rebuild the server image.

The browser suite needs no API suite first and no more history than the boot waits for.
`make e2e-up` does only the boot, which is what a job wants while it installs the browsers.
`make e2e-all` runs the API suite and then the browser suite.
Run the two one after the other and never at the same time against one stack, because both set the `paymentFailure` flag and reset it when they finish.

Specs import `test` and `expect` from `web/e2e/fixtures.ts`, which fails a test whose page logs a console error.
The `setup` project signs in through Dex once and saves `web/e2e/.auth/user.json`.

## Run part of the browser suite

Set `PW_SHARD` to the part you want, for example `PW_SHARD=2/4 make e2e-browser` for the second of four.
List what a shard holds without a stack by running `PW_SHARD=2/4 bunx playwright test --list` from `web/`.
Every shard runs the sign-in setup first.

## Start the stack early

`make e2e-warm` starts every service except the server and returns.
The shop and Prometheus then collect data while the server image builds.
The next `make e2e`, `make e2e-up` or `make e2e-browser` within 30 minutes adopts that stack, starts the server and waits for health as usual.
With no warm stack, they reset and boot as before.
The warm step needs the shop's images, never the server image.

## Read a CI failure

CI runs the browser suite as four shards, each on its own stack, with two workers and one retry.
A test that passes only on the retry still fails the run.
Playwright records a trace on the retry, a screenshot on a failure and a video it keeps on a failure.
Locally the suite runs with one worker, no retries and the list reporter.

When a shard fails, its HTML report, test results and compose logs are uploaded as the `playwright-shard-N` artifact.
When the API suite fails, its compose logs are uploaded as `e2e-logs`.
Both artifacts are kept for 7 days.

## Test two servers

`make replicas-up` builds this checkout and boots the isolated `slashobserve-replicas` compose stack.
It has two servers behind HAProxy and PgBouncer in transaction mode, with `SLASHOBSERVE_DATABASE_DIRECT_URL` pointing straight at Postgres.
It runs the replay model provider and a Prometheus that scrapes both servers, which is the data source of the load test's dashboard.
HAProxy answers on http://localhost:48080 and spreads requests over both servers.
Server 1 alone answers on http://localhost:48081 and server 2 alone on http://localhost:48082, so a check can write on one server and read on the other.

The stack's project name, ports and volumes differ from the demo and the e2e stack, so it runs beside them and never stops, resets or reads them.
`make replicas-down` removes only its containers and volumes.
Nothing here runs in CI.

`scripts/replicas.sh` also has `stop`, `kill` and `start` for one server, `workers` to recreate the servers with another `SLASHOBSERVE_AGENT_WORKERS`, `status`, `logs` and `keys`.
The service accounts the load test signs in with are created under `/tmp/slashobserve-replicas`, and their keys are never printed.

### Run the two-server checklist

Run `make replicas-check`.
It recreates the stack, runs `scripts/replicas.sh check`, prints one `PASS`, `FAIL` or `KNOWN` line per item, and takes about 15 minutes.

The checklist covers:

- migrations, provisioning and scheduled jobs run once across both servers
- the fleet count and the divided limits are right
- events, agent runs, webhook leases, logins and OAuth tokens cross servers
- the status of a Grafana import is the same on both servers
- a server stopped gracefully, killed and restarted leaves no request lost

A `KNOWN` line is a gap that [Scaling](../operate/scaling.md) documents, and whoever fixes it sees the line change.
`scripts/replicas.sh check C5` reruns one item on a running stack, except C1 to C3, which need a fresh boot.
Run the checklist before a release that changes the scheduler, the agent queue, events, OAuth, webhooks, the fleet or provisioning, and paste its output into the pull request.

## Run the load test

Run `make loadtest`.
It recreates the replicas stack and measures it with `go run ./loadtest`, which uses only the Go standard library.
The series takes about 50 minutes.

It runs this series:

1. All four scenarios on one server, with 8 agent workers.
2. All four scenarios on two servers, with 8 agent workers.
3. The agent builds alone on two servers with 2 agent workers.
4. The agent builds alone on two servers with 16 agent workers.

To run one pass on the stack as it is, with the running servers as replicas, use `scripts/replicas.sh load [--only viewers|streams|mcp|agent] <label>`.
The tool refuses to run against anything but this stack: a loopback target on port 58080 is the demo and one on 8080 is a local server, and both are rejected.
Another host needs `--remote <host>` with that exact host name.
It never prints a key and deletes or resets nothing.

### Read the load test report

Each pass writes `loadtest/reports/<time>-<label>/`, which is git-ignored.

- `report.md` has one table per scenario and then a table of gates.
- Under the tables, it lists the peak CPU and memory of every container, the Postgres connections by state and the PgBouncer pool.
- `slow-queries.txt` has the ten slowest statements.
- `report.json` holds every number.

The run fails when a gate fails.
The 300 ms server time for panel data and the 5 second queue wait at 8 workers are measurements next to the targets of [Scaling](../operate/scaling.md), not gates.

## Measure coverage

Run `COVER=1 make test`.
It writes the Go profile to `coverage.out` and prints its `total:` line, and it writes the web summary to `web/coverage/coverage-summary.json` and prints it.
Read the Go profile per function with `go tool cover -func=coverage.out`.
Generated web files (`*.gen.ts`) are left out of the web numbers.
CI does not measure coverage.

## Check the docs

Run `scripts/docs-lint.sh`.
It checks every page under `docs/`, and `make lint` runs it.
It prints one `path:line: rule` line for each problem and fails when it finds any.
Set `DOCS_DIR` to lint another folder, for example a copy of a page you are drafting.

## Reference

### Commands

| Command | What it does |
| --- | --- |
| `make test` | Every unit suite. |
| `make test-go` | `go test -race`, with `GO_TEST_PKGS` to narrow the packages. |
| `make test-web` | The web Vitest suite. |
| `make test-harness` | `bun test` for `slashobserve-harness`. |
| `make test-benchmarks` | The benchmark adapter's tests; needs `uv`. |
| `make test-scripts` | The tests of the shell scripts. |
| `make lint` | Go, web and agent linters, the docs lint and the Helm chart checks. |
| `make ci` | `lint`, `test`, a tidy check and `build`. |
| `make e2e` | Reset and boot the e2e stack, then run the API suite. |
| `make e2e-warm` | Start the e2e stack without the server. |
| `make e2e-up` | Reuse or boot the e2e stack and stop. |
| `make e2e-browser` | Reuse or boot the e2e stack and run Playwright. |
| `make e2e-all` | The API suite, then the browser suite. |
| `make replicas-up` | Boot the two-server stack. |
| `make replicas-down` | Remove the two-server stack. |
| `make replicas-check` | Recreate the stack and run the checklist. |
| `make loadtest` | Recreate the stack and run the load test series. |
| `make audit` | `govulncheck` and `bun audit` for the web and agent lockfiles. |

Details of `make test-scripts`:

- `scripts/release-scripts-test.sh` tests the release tag and notes scripts.
- `scripts/install-test.sh` tests `install.sh` against fake release assets.
- `scripts/docs-lint-test.sh` tests the docs lint rules against fixture pages.
- `scripts/ci-changes-test.sh` tests which CI jobs a set of changed files needs.
- `scripts/astronomy-test.sh` tests the Astronomy Shop's flag target against a fixture flag file.
- `scripts/replicas-test.sh` tests the replicas stack's wait for a request rate in Prometheus, against a stubbed `curl`.
- `scripts/redact-test.sh` tests `scripts/redact.sh`, the one rule that keeps every token the server mints and other secrets out of the e2e and replicas stacks' failure output.

### E2E stack settings

| Variable | Default | What it sets |
| --- | --- | --- |
| `SLASHOBSERVE_E2E_PROJECT` | `slashobserve-e2e` | The compose project. |
| `SLASHOBSERVE_E2E_PORT` | `18080` | The server's host port. |
| `SLASHOBSERVE_E2E_DEX_PORT` | `15556` | Dex's host port. |
| `SLASHOBSERVE_E2E_PROMETHEUS_PORT` | `19090` | Prometheus's host port. |
| `SLASHOBSERVE_E2E_SHOP_PORT` | `18081` | The shop's host port, which also serves its Jaeger UI and flag UI. |
| `SLASHOBSERVE_IMAGE` | `slashobserve-e2e:local` | A prebuilt server image to run instead of building the checkout. |
| `SLASHOBSERVE_E2E_BASE_URL` | `http://localhost:18080` | The address Playwright tests, when you run `bun run e2e` directly. |
| `PW_SHARD` | none | The Playwright shard to run, as `N/total`. |

Details of the e2e stack:

- The stack runs through `scripts/astronomy.sh compose` with the extra layer `deploy/astronomy/compose.e2e.yaml`.
- Every `make e2e` resets the stack with `down --volumes`.
- The shop's flag file and key live in `deploy/astronomy/.state/<project>`, apart from the demo's, so parallel stacks never share a flag file or a key.
- With `SLASHOBSERVE_IMAGE` set, `deploy/astronomy/compose.prebuilt.yaml` replaces the build layer, and compose pulls the image when it is missing.
- All ports bind to 127.0.0.1.
- A direct `bun run e2e live-fault` reads the project from `SLASHOBSERVE_E2E_PROJECT`, so it edits the e2e stack's flag file and never the demo's.

### Waits and locks

| What | How long |
| --- | --- |
| Compose waiting for every service to be healthy | 15 minutes |
| The server answering `/readyz` after that | 60 seconds |
| Prometheus holding the shop's span metrics | 5 minutes |
| ClickHouse holding log and trace rows | 3 minutes |
| Prometheus holding 4 minutes of span metrics, before the import checks | 5 minutes |
| `make e2e-up` or `make e2e-browser` finding data on a running stack | 30 seconds, then it reboots |
| A warm stack that can be adopted | 30 minutes old at most |
| `run`, `up` and `browser` waiting for a live `warm` | 10 minutes |

Details of the lock:

- `scripts/e2e.sh` holds an exclusive lock on `/tmp/<project>-run.lock` for a whole run, so a second run against the same project fails with `another e2e run is active`.
- The warm marker is `/tmp/<project>-warm`; `scripts/e2e.sh down` removes it.
- A warm stack is adopted once, and only when Prometheus is running and no server container exists.

### Replicas stack ports

| Variable | Default | What it serves |
| --- | --- | --- |
| `SLASHOBSERVE_REPLICAS_PORT` | `48080` | HAProxy, round-robin over both servers. |
| `SLASHOBSERVE_REPLICAS_ONE_PORT` | `48081` | Server 1 alone. |
| `SLASHOBSERVE_REPLICAS_TWO_PORT` | `48082` | Server 2 alone. |
| `SLASHOBSERVE_REPLICAS_DEX_PORT` | `45556` | Dex. |
| `SLASHOBSERVE_REPLICAS_PROMETHEUS_PORT` | `49090` | Prometheus. |

All ports bind to 127.0.0.1.
Loki and Tempo ports are reserved, but the stack does not start them.

### Load test scenarios

| Scenario | What it does |
| --- | --- |
| `viewers` | 200 viewers reload one dashboard of 12 panels every 10 seconds, first on one shared time range and then on a different range each. |
| `streams` | 500 open activity streams while one editor writes annotations. |
| `mcp` | 10 MCP clients that call tools. |
| `agent` | 20 and then 50 agent builds started at once on the replay model. |

Details of the agent scenario:

- Each build takes about 6 seconds of model time, so the builds measure the queue and the database, not a model.
- The server admits 20 new-dashboard requests at once and 3 per caller, so the 50 builds are 20 at once and the rest as slots free up.
- The report counts how often a submission was told to retry.
- Each viewer key is limited to 8 panel requests at once, as the server limits each caller.

### Load test gates

A gate fails the run when any of these happens:

- any 5xx response or dropped connection
- any 4xx other than 429 on more than 0.1% of requests
- a stream that was refused or dropped
- a stream event that was lost or duplicated
- an agent build that fails or runs more than once
- a JSON-RPC or tool error in the MCP scenario, or an MCP call shed while each server's share of the calls fits its cap
- a container that restarted or was killed for memory

### What CI runs

| Job | Runs when | What it runs |
| --- | --- | --- |
| `changes` | Always. | `scripts/ci-changes.sh`, which decides which of the other jobs run. |
| `go-lint` | A file outside `web/` and `harness/` changed. | `make lint-go lint-deploy tidy-check`: golangci-lint, `sqlc diff`, the docs lint, the Helm chart checks and the tidy check. |
| `go-test` | A file outside `web/` and `harness/` changed. | `make test-go` in eight shards: `grafana`, `db`, `datasource`, `agent` and `rest-1` to `rest-4`, split by `scripts/go-test-shard.sh`, which deals the packages outside the four named shards to the four rest shards by their measured test seconds. A pull request runs without the race detector; the merge queue, `main` and manual runs keep it. Each shard saves its own Go build cache. |
| `web-lint` | A file other than Go or prose changed. | `make lint-web lint-harness test-harness test-scripts test-benchmarks`. |
| `web-test` | A file other than Go or prose changed. | The web Vitest suite in two shards. |
| `build` | Non-prose files changed. | `make build`, then `make generate`, which must leave every file unchanged. |
| `image` | `Dockerfile` or `.dockerignore` changed. | A build of the `Dockerfile` from its own stages. |
| `e2e-api` | Non-test, non-prose files changed. | `make e2e`, with a 30 minute limit. |
| `e2e-browser` | Non-test, non-prose files changed. | `make e2e-browser` in four shards, each on its own stack, with a 30 minute limit. |
| `audit` | A lockfile, `package.json`, `go.mod` or `tools/` changed, on `main` and daily. | `make audit`. |
| `check` | Always. | Passes when every lint, test, build and image job that ran has passed; `audit` and the two end-to-end jobs are separate. |

Details of how CI picks jobs:

- A pull request runs only the jobs its changed files need, and a skipped job counts as passed.
- `main` and a manual run start every job.
- The daily run starts only `audit`.
- A web-only change skips the Go jobs, except when it changes `web/src/spec/defaults.gen.ts`, `harness/src/protocol.ts` or `harness/test/fixtures/prompts/guide-prometheus.md`, which Go tests read.
- A Go-only change skips the web jobs.
- A change to documentation skips every job but the Go ones, whose lint runs the docs check.
- A change to unit tests skips the two end-to-end jobs.
- A change under `.github/`, to `Makefile`, `scripts/ci-changes.sh` or `scripts/go-test-shard.sh` runs every job.
- A draft pull request runs neither end-to-end job; marking it ready for review runs them.

Details of how the end-to-end jobs start:

- Each job compiles the server and the agent on the runner and builds the server image `slashobserve-e2e:ci` from those binaries.
- Each job runs `make e2e-warm` in the background first, so the shop starts while the image builds.
- Each browser shard starts the stack with `make e2e-up` while Chromium installs, then runs `make e2e-browser` with `PW_SHARD=N/4`.

### Docs lint rules

A page passes when:

- its front matter has a `title` and a `description`
- no line starts with a level 1 `# ` heading
- it has no em dash or en dash, anywhere in the file
- every relative link to a `.md` file or an image points at a file that exists, and every image has alt text
- each prose line holds one sentence
- no paragraph runs longer than 8 prose lines without a blank line
- a page with more than 60 non-blank lines has at least one `## ` heading

A prose line is any line outside front matter and code blocks that is not a heading, a list item or a table row.
It holds more than one sentence when a period followed by a space and a capital letter appears outside inline code, parentheses and double quotes.
The lint checks relative links to files only; it does not check `#anchor` fragments.
