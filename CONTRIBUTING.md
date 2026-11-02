# Contributing

This page covers how to report a problem, set up a build and send a change.

## Before you start

- Search the [issues](https://github.com/slashobserve/slashobserve/issues) first.
- For a new feature or a large change, open an issue before you write code, so we can agree on the approach.
- Report a security problem privately, as [SECURITY.md](SECURITY.md) describes, never in a public issue.
- Everyone taking part follows the [code of conduct](CODE_OF_CONDUCT.md).

A Grafana dashboard that imports badly is a useful report.
Use the "Grafana import failure" issue template and attach the JSON, with credentials, hostnames and private query text removed.

## Set up

You need Go 1.27.3 (a Go 1.21 or newer toolchain downloads it automatically), Bun 1.4.2 and Docker.
Docker runs the database-backed tests through testcontainers, and the demo stack.

    git clone https://github.com/slashobserve/slashobserve
    cd slashobserve
    make build
    make demo-build

`make build` installs the web dependencies and builds `bin/slashobserve-server` and `bin/observe`.
`make demo-build` builds the server and demo shop images from your working tree and starts the demo stack with them; `make demo` pulls the published server image instead and builds only the shop.
The demo is for local use only; see the [quick start](docs/get-started/quick-start.md).

[CLAUDE.md](CLAUDE.md) describes the layout, every command and the conventions that apply across the repository.
The conventions for each area are in [internal/CLAUDE.md](internal/CLAUDE.md), [web/CLAUDE.md](web/CLAUDE.md) and [e2e/CLAUDE.md](e2e/CLAUDE.md).

## Check your change

| Command | What it does |
| --- | --- |
| `make lint` | Go and web linters, formatting, type checks, and checks that generated files match their sources. |
| `make test` | Go tests with the race detector for the server and the demo shop, then the web unit tests. |
| `make ci` | `lint`, `test`, a `go mod tidy` check and `make build`, which is what the `check` job in CI runs. |
| `make generate` | Regenerates the sqlc code, the Spec types and validator for the web app, and the API types, after you edit migrations, queries, the schema or `api/`. |

CI also runs `make generate` and fails when it changes any file, so commit generated files with their sources and never edit them by hand.
A change to how the Grafana importer converts a dashboard needs `make golden`; read every changed golden file before you commit it.

## Conventions

- Add a unit test for every change in behavior.
  Integration tests run against real Postgres, Prometheus and ClickHouse through testcontainers, not mocks.
- Write no comments or docstrings unless they state a constraint that the code cannot show.
  Prefer a name, a small helper or a type that carries the reason.
- Configuration is an environment variable named `SLASHOBSERVE_*`, read only in `internal/config`.
- Never edit a migration that has been merged; add a new one.
- Do not edit `spec/schema/observespec.v0.json` without agreeing the change in an issue first.
- The server never logs secrets, prompt text, query text or tool results.
- A user-visible change updates the page under `docs/` that describes it, in the same pull request.
  Docs pages have `title` and `description` front matter, no `#` heading, and one sentence per line.
- Write plain prose, and use a plain dash instead of an em dash.

## Send a pull request

1. Fork the repository and branch from `main`.
   Name the branch for what it changes, such as `add-top-limit`.
2. Make atomic commits.
   Each commit is one complete change that builds and passes on its own.
   The subject describes the change; the body explains any decision the code cannot show.
3. Run `make ci`.
4. Open a pull request and fill in the template.

By submitting a contribution, you agree that it is licensed under the Apache License 2.0, as section 5 of the [license](LICENSE) provides.
