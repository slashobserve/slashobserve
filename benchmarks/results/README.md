# Benchmark results kept on record

Each file here is one measured run of the Observe Agent on an external benchmark, written by `python -m slashobserve_bench.results write` from the Harbor job folder or folders of the run, never typed by hand.
A pass is a trial with reward 1.0.
`Pass^n` counts the tasks whose every attempt passed, which is the leaderboard notion for n attempts.
Every failed trial carries one cause (`adapter_fault`, `missing_capability` or `agent_mistake`) and one sentence, read from its trajectory and grading details.
No task statement and no benchmark text is copied in.

The command refuses a failure that has no cause, and `benchmarks/README.md` shows how to run it.
The table below is rewritten by the command and checked by `make test-benchmarks`.

## Runs

| Run | Date | Server | Model | Tasks | Attempts | Mean reward | Trials passed | Pass^n | Skipped | Cost (USD) | Failures |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| [o11y-bench-subset-0.1.4-6-gc75cceda2-2026-11-23.json](o11y-bench-subset-0.1.4-6-gc75cceda2-2026-11-23.json) | 2026-11-23 | 0.1.4-6-gc75cceda2 | anthropic/claude-sonnet-5-5 | 10 | 1 | 0.889 | 8/10 | 8/10 | 0 | 0.687 | 2 agent mistake |
| [o11y-bench-full-dev-f8c7753c0-2026-11-25.json](o11y-bench-full-dev-f8c7753c0-2026-11-25.json) | 2026-11-25 | dev | anthropic/claude-sonnet-5-5 | 51 | 3 | 0.914 | 125/153 | 35/51 | 0 | 13.836 | 3 missing capability, 25 agent mistake |
