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
`make demo-build` builds the images from your working tree and starts the demo stack with them; `make demo` pulls the published images instead.
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

## Run the evals

The eval runner measures the agent on create, revise, fix, suggest and paste-import tasks against an isolated `slashobserve-evals` stack that never touches the demo stack.
The replay suite needs no model and no key, and CI runs it on every pull request.

| Command | What it does |
| --- | --- |
| `make evals-replay` | Boots the replay stack and runs the replay suite against it; every scenario must meet its expectation. |
| `make evals-stack-up` | Boots the stack alone on http://localhost:28080, in replay mode by default; `EVALS_STACK_MODE=<env-file>` boots it on the provider settings of that file instead; it returns once Prometheus serves the series the scenarios read. |
| `make evals-stack-down` | Removes the stack and drops its volumes only. |
| `make evals-live` | Runs the live suite once per model listed in `evals/matrix.yaml` (copy `evals/matrix.example.yaml`; the file is git-ignored) against a running stack. |

The live suite spends tokens on your own provider account, so it runs by hand and CI never runs it.
Keep each model's provider settings in a git-ignored env file; the runner never reads it.

### Large dashboards

The live suite centers on eight public dashboards, listed in `evals/corpus-sample.yaml` and in `largeDashboards` in `evals/runner/tasks.go`; a test keeps the two equal.
They are Node Exporter Full (125 panels), HAProxy, Cilium, VictoriaMetrics, vmalert, Argo CD, the ClickHouse OpenTelemetry service dashboard and the demo shop.
The replay suite keeps its `fix`, `add` and `paste` sweep over every corpus dashboard, and runs the other generators only on these eight.

Every large dashboard gets one scenario per generator, except where the generator has no target or range the stack can serve (below), and each scenario is tagged with its generator, so `--tag rebuild` selects one kind.

| Generator | Task | What it asks |
| --- | --- | --- |
| `rebuild` | create | Build the dashboard from a prompt that lists every panel title under its row, with the real panel and row counts. Replay builds it panel by panel through the real build path. |
| `change` | revise | Show one panel as a stat panel, or as a time series when it already is a stat. |
| `retime` | revise | Default the dashboard to the last 6 hours, or to a shorter range when the dashboard's query steps would outrun the few minutes of history the stack holds. |
| `suggest` | suggest | Suggest a clearer title for one panel. |
| `rowedit` | revise | Add a description to every panel in one row of 3 to 8 panels. |
| `fix` | fix unverified | Fix the one panel the runner broke. |
| `add` | revise | Add a panel for a metric or table the dashboard reads. |
| `paste` | chat import | Import the dashboard from pasted JSON; every large dashboard is over the prompt cap, so this checks the `prompt_too_long` refusal. |

The panel and the row a generator works on come from one function each (`pickWorkTarget`, `pickWorkRow`), so the replay fixture and the live seed name the same ones; both prefer panels whose metrics every host serves.
The generators pick only panels the server verifies at import, using the measured gaps in `evals/runner/importgaps.go`, and emit no scenario for a dashboard that has none, because grading a model on a panel with no data tells nothing.
A seed still fails when its chosen panel turns out unverified; remeasure the gaps from the import report of the evals stack when the corpus, the importer or the synthetic series change.

A rebuild is the slow task: the evals stack lets a run take 120 minutes, and the whole live suite takes hours per model.
The report starts with `Time to first panel`, the seconds from the request to its first accepted save, because that is what a person waits for on a large dashboard.

### Kubernetes dashboards

The public Kubernetes dashboards in the corpus verify against a local kind cluster instead of synthetic series.
`make demo-k8s` creates the cluster `slashobserve-demo` beside the compose demo and never touches it; `make demo-k8s-down` deletes only that cluster.
Kind runs as a pinned Go tool, so you need only Docker and Go.

| Command | What it does |
| --- | --- |
| `scripts/evals-stack.sh up --k8s replay` | Boots the evals stack with the cluster's Prometheus as the `prom_k8s` datasource and no synthetic series; the cluster must be running first. |
| `go run ./evals/runner --verify-k8s --server http://localhost:28080 --bootstrap-dex` | Imports the 29 Kubernetes dashboards and fails when a panel is unverified and not listed in `evals/k8s-gaps.yaml`, or when the listed gaps pass 15% of the data panels. A listed gap that verifies only prints a warning and the exit code stays 0. |
| `scripts/evals-stack.sh down` | Removes the evals stack and drops its volumes only. |

Add a panel to `evals/k8s-gaps.yaml` only after observing it fail, with a reason someone can check against the cluster's Prometheus.
Give the cluster about ten minutes after `make demo-k8s` before you verify, because recording rules and 5 minute rates need data first.

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
