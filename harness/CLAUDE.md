# harness

The harness is the built-in agent (the Observe Agent people see in the app) as a separate process, `slashobserve-harness`: an ordinary claimer that runs the model loop and works a run through the claim tools over MCP.
The server enforces the run's guards in `internal/runsession`; `src/guard/` is their port, which the run loop does not call yet.
The server starts it whenever a model provider is configured; the image ships it at `/usr/local/bin/slashobserve-harness` and `make harness` builds `bin/slashobserve-harness`.

## Layout

- `src/protocol.ts`: the claim tools and the `_meta` keys, as zod schemas.
- `src/run.ts`: the model loop for one run.
- `src/shutdown.ts`: how a stop ended, `drained` or `forced`, and its exit code; `main.ts` keeps listening for `SIGTERM` and `SIGINT` after the first one (`process.on`), because a second signal on a one-shot listener kills the process before it releases its runs.
- `src/guard/`: ports `internal/agent/guard` without the save classification and the parent version, which stay with the server: a `Guard` admits each call of a turn, settles it from its result and run meta, and ends the turn with its nudges, stage line and stop; `snapshot()` is plain JSON for `agent_state`.
  Its scenarios are the traces under `spec/examples/agent/guard/`, which the Go guard tests read too; a trace marked `ts_only` is a behaviour the port changed on purpose, and one marked `go_only` pins the Go behaviour it replaced.
  It has one call key for reads, saves and writes (`callKey`, canonical JSON), and each of the five send-backs of a finding (absence, evidence, title, ledger, grounding) goes back once per attempt, never re-armed by a restart; a send-back whose fix needs a read lets the next turn read past the call limit.
- `src/session/`: the model sessions, one per provider.
- `src/session/replay/`: ports `internal/llm/replay`.
- `src/route/`: ports `internal/agent/route` (`classify`) and the task-tool narrowing of `internal/agent/tools/tasktools.go` (`toolsForTask`), with the per-task call limits read from `claim_run`'s `agent.task_calls` (`callsForTask`); not yet called by the run loop.
  Its tests read the Go tests' cases in `spec/examples/agent/route/`.
- `src/prompts/`: ports the run system text assembly of `internal/agent/prompts/run.go` (`investigateFor`, `buildSystem`), importing the `.md` files from `internal/agent/prompts/` and the example Spec with Bun text imports, which the Dockerfile's agent stage copies; the authoring guide comes from `get_run_brief`'s `brief.guide.text`; not yet called by the run loop.
  Its tests compare against the Go goldens in `spec/examples/agent/prompts/`.
- `src/brief/`: ports `internal/agent/brief` (`render`, the digest, `resumeReason`, the seed, context and anchor notes, the `<hypotheses>` block) over `get_run_brief`'s `brief` and the agent's own state (route, loop, facts, ledger, triage block, digest); `gofmt.ts` reproduces Go's `encoding/json` escaping, `%q` quoting, `strings.Fields` and time layouts, so the text stays byte-identical; not yet called by the run loop.
  Its tests compare against the Go goldens in `spec/examples/agent/brief/` and the anchor cases in `spec/examples/anchor/`.
  `gofmt.ts` also holds Go's `strconv.FormatFloat` (exact, ties to even), `ParseFloat`, `strings.TrimSpace` and `time.Duration.String`.
- `src/grounding/`: ports `internal/agent/grounding` (modes `off` and `enforce`; `report` is dropped) and the grounding glue of `internal/runsession/grounding.go`: `noteResult` issues facts into a fact tool's result and records a read (`readsData` or a result with discovery, `call_mcp_tool` text as names only), `checkSave` reviews a `save_finding` input into the send-back text, the `unverified` items and each cause's signals, score, band and queries, and `savedFinding` renders `{{fN}}` and adds those fields to the input it sends; the record is plain JSON for `agent_state`; not yet called by the run loop.
  Its tests read the Go cases in `spec/examples/agent/grounding/` (`figures`, `render`, `scenarios`, and `sessions`, which `internal/runsession` writes with `-update`).
- `src/triage/`: ports `internal/agent/triage`: `sweep` (the preflight sweep within `defaultLimits`, 40 reads, 20 s, 4 at once) reads through `ToolReader` over the run's MCP tools (`list_services`, `list_metrics`, `read_series`, `trace_stats`, `trace_error_origins`, `log_patterns`, `list_annotations`), over `sourcesOf(brief.datasources)` (`everyone` and not `slashobserve-audit`), and gives the `<triage>` block (`render` under `maxBlockBytes`), the `leads` and `sweptStep`, the step text; `conclude.ts` ports `seedLedger`, `noteTransient` and the conclude rule (`candidateProblems`, `checkCandidates`) of `internal/runsession/candidates.go`; not yet called by the run loop.
  Its tests read `spec/examples/agent/triage/` (`onset`, `cluster` with the leads, `render`, `conclude`, and `sweep`, the recorded tool calls of three sweeps, which they replay through a stub MCP server).

## Rules

- Versions are exact and at least 14 days old; `bunfig.toml` enforces both.
- Never log settings, keys, prompts, tool inputs, tool results or model text.
  `log()` takes only scalar fields.
- A claim-protocol change starts in Go and its docs (`docs/use/mcp.md`), then the zod schemas follow.
- Nothing the harness needs may become a requirement for other claimers.

## Commands

Run from `harness/`:

- `bun install --frozen-lockfile`
- `bun run typecheck`
- `bun run format:check`
- `bun test`

From the repository root, `make harness` builds `bin/slashobserve-harness`, and `make lint` and `make test` run the checks above.
