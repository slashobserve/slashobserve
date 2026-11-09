---
title: Testing
description: The unit, component, API and browser suites, coverage and the docs lint, and how to run each one locally.
---

SlashObserve has four test suites and one docs check.
CI runs all of them on every pull request: the `check` job runs the unit and component tests, the lint and the docs lint, and the `e2e-api` and `e2e-browser` jobs run the two end-to-end suites once `check` passes.

## Unit tests

`make test` runs `go test -race ./...` for the root module and the `demo` module, the web Vitest suite, and the tests of the release, install and docs lint scripts.
Run one part on its own with `go test -race ./...` from the repository root or `bun run test` from `web/`.

Go tests sit next to the code as `*_test.go` files.
Web tests for pure helpers sit next to the code as `*.test.ts` files.

## Component tests

Web components are tested with Vitest, Testing Library and happy-dom, in `*.test.tsx` files next to the component.
They run in the same `bun run test` as the unit tests.

Render a component with `renderWithProviders` from `web/src/test/render.tsx`.
It wraps the component in a fresh TanStack Query client with retries off and returns a `user` from `user-event` for clicks and typing.
Pass `queryClient` to seed the cache, and `route` (a route `path` and a starting `url`) when the component reads or writes the URL, so it renders inside a memory router.
Stub only what a test cannot run, such as `fetch` with `vi.stubGlobal`.
Find elements by role and accessible name, the way a person using a screen reader would.

`web/src/test/setup.ts` adds the Testing Library matchers, unmounts each test's components and stubs the browser APIs happy-dom lacks.

## API suite

`make e2e` resets and boots the isolated `slashobserve-e2e` compose stack on http://localhost:18080, then runs `scripts/e2e.sh`, which checks the HTTP API, the MCP endpoint, the CLI and the demo data with real requests.
The stack is left running for the browser suite; stop it with `scripts/e2e.sh down`.
The suite runs the replay model provider, so it never calls a live model.

## Browser suite

`make e2e-browser` reuses the running e2e stack, or boots one, and runs the Playwright specs in `web/e2e` against it.
Reuse does not rebuild the images, so run `make e2e` again after a code change.
`make e2e-all` runs the API suite and then the browser suite.

Set `PW_SHARD` to run one part of the suite, for example `PW_SHARD=2/4 make e2e-browser` for the second of four.
List what a shard holds without a stack with `PW_SHARD=2/4 bunx playwright test --list` from `web/`.
Every shard runs the sign-in setup first.

CI runs the browser suite as four shards, each on its own stack, with two workers and one retry.
A trace is recorded on the retry.
When a shard fails, its HTML report and test results are uploaded as the `playwright-shard-N` artifact; when the API suite fails, the compose logs are uploaded as `e2e-logs`.
Locally the suite runs with one worker, no retries and the list reporter.

## Coverage

`COVER=1 make test` also measures coverage.
It writes the Go profile to `coverage.out` and prints its `total:` line, and it writes the web summary to `web/coverage/coverage-summary.json` and prints it.
Read the Go profile per function with `go tool cover -func=coverage.out`.
Generated web files (`*.gen.ts`) are left out of the web numbers.

The CI `check` job runs with `COVER=1` and adds the Go and web statement coverage to its job summary.

## Docs lint

`scripts/docs-lint.sh` checks every page under `docs/`, and `make lint` runs it.
It prints one `path:line: rule` line for each problem and fails when it finds any.

A page passes when:

- its front matter has a `title` and a `description`
- no line starts with a level 1 `# ` heading
- it has no em dash or en dash
- every relative link to a `.md` file points at a file that exists
- each prose line holds one sentence
- no paragraph runs longer than 8 prose lines without a blank line
- a page with more than 60 non-blank lines has at least one `## ` heading

A prose line is any line outside front matter and code blocks that is not a heading, a list item or a table row.
It holds more than one sentence when a period followed by a space and a capital letter appears outside inline code, parentheses and double quotes.
Set `DOCS_DIR` to lint another folder, for example a copy of a page you are drafting.
