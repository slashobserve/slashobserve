import argparse
import json
import sys
from datetime import datetime
from pathlib import Path
from typing import Any, Iterable

SCHEMA_VERSION = 1
RESULTS_DIR = Path(__file__).resolve().parents[1] / "results"
README_TABLE_HEADING = "## Runs\n"

CAUSES = ("adapter_fault", "missing_capability", "agent_mistake")
SETTING_KEYS = (
    "investigation_calls",
    "investigation_loop",
    "investigation_calls_max",
    "finding_grounding",
    "llm_effort",
    "run_max_cost_usd",
    "wall_timeout_sec",
)
SERVER_START_MESSAGE = "starting slashobserve-server"
SKIPPED_EXCEPTION = "TaskSkipped"
NO_AGENT_OUTPUT = {"cause": "adapter_fault", "summary": "The trial left no agent output folder, so the run never produced an answer."}


class ResultsError(Exception):
    pass


def _read_json(path: Path) -> Any:
    return json.loads(path.read_text(encoding="utf-8"))


def _seconds(span: dict[str, Any] | None) -> float | None:
    if not span or not span.get("started_at") or not span.get("finished_at"):
        return None
    started = datetime.fromisoformat(span["started_at"])
    finished = datetime.fromisoformat(span["finished_at"])
    return round((finished - started).total_seconds(), 1)


def _server_of(log: Path) -> dict[str, str] | None:
    if not log.is_file():
        return None
    for line in log.read_text(encoding="utf-8", errors="replace").splitlines():
        try:
            entry = json.loads(line)
        except json.JSONDecodeError:
            continue
        if isinstance(entry, dict) and entry.get("msg") == SERVER_START_MESSAGE:
            return {"version": entry.get("version", ""), "commit": entry.get("commit", "")}
    return None


def _judge_of(trial_dir: Path) -> str | None:
    details = trial_dir / "verifier" / "grading_details.json"
    if not details.is_file():
        return None
    return _read_json(details).get("grading_model")


def _error_of(info: Any) -> str | None:
    if not info:
        return None
    if isinstance(info, dict):
        return str(info.get("exception_type") or info.get("type") or "error")
    return "error"


def _skipped_reason(trial_dir: Path, info: Any, error: str | None) -> str | None:
    marker = trial_dir / "agent" / "skipped.txt"
    if marker.is_file():
        return marker.read_text(encoding="utf-8").strip() or "skipped"
    if error == SKIPPED_EXCEPTION:
        return str((info or {}).get("exception_message") or "skipped") if isinstance(info, dict) else "skipped"
    return None


def read_trial(trial_dir: Path) -> dict[str, Any]:
    try:
        return _read_trial(trial_dir)
    except KeyError as missing:
        raise ResultsError(f"{trial_dir}: result.json has no {missing}") from None


def _read_trial(trial_dir: Path) -> dict[str, Any]:
    result = _read_json(trial_dir / "result.json")
    error = _error_of(result.get("exception_info"))
    agent = result.get("agent_result") or {}
    config_agent = result["config"]["agent"]
    kwargs = config_agent.get("kwargs") or {}
    rewards = (result.get("verifier_result") or {}).get("rewards") or {}
    return {
        "task": result["task_name"],
        "trial": result["trial_name"],
        "digest": result.get("task_checksum"),
        "model": config_agent["model_name"],
        "settings": {key: kwargs.get(key) for key in SETTING_KEYS},
        "server": _server_of(trial_dir / "agent" / "server.log"),
        "judge": _judge_of(trial_dir),
        "reward": rewards.get("reward"),
        "input_tokens": agent.get("n_input_tokens"),
        "cache_tokens": agent.get("n_cache_tokens"),
        "output_tokens": agent.get("n_output_tokens"),
        "cost_usd": agent.get("cost_usd"),
        "agent_seconds": _seconds(result.get("agent_execution")),
        "trial_seconds": _seconds(result),
        "started_at": result["started_at"],
        "finished_at": result["finished_at"],
        "error": error,
        "skipped": _skipped_reason(trial_dir, result.get("exception_info"), error),
        "no_agent_output": not (trial_dir / "agent").is_dir(),
    }


def _harbor_version(job_dir: Path) -> str | None:
    lock = job_dir / "lock.json"
    if not lock.is_file():
        return None
    return (_read_json(lock).get("harbor") or {}).get("version")


def read_jobs(job_dirs: Iterable[Path]) -> tuple[list[dict[str, Any]], set[str]]:
    trials: list[dict[str, Any]] = []
    harbor_versions: set[str] = set()
    for job_dir in job_dirs:
        if not job_dir.is_dir():
            raise ResultsError(f"{job_dir} is not a job folder")
        version = _harbor_version(job_dir)
        if version:
            harbor_versions.add(version)
        found = [path.parent for path in sorted(job_dir.glob("*/result.json"))]
        if not found:
            raise ResultsError(f"{job_dir} holds no trial folders")
        trials.extend(read_trial(path) for path in found)
    names = [trial["trial"] for trial in trials]
    duplicates = sorted({name for name in names if names.count(name) > 1})
    if duplicates:
        raise ResultsError(f"trial folders appear more than once: {', '.join(duplicates)}")
    return trials, harbor_versions


def _single(values: Iterable[Any], what: str) -> Any:
    distinct = []
    for value in values:
        if value is not None and value not in distinct:
            distinct.append(value)
    if len(distinct) > 1:
        raise ResultsError(f"the trials disagree on {what}: {distinct}")
    return distinct[0] if distinct else None


def passed(reward: float | None) -> bool:
    return reward == 1.0


def _round(value: float | None, digits: int) -> float | None:
    return None if value is None else round(value, digits)


def _check_failure(trial: str, entry: Any) -> dict[str, str]:
    if not isinstance(entry, dict) or entry.get("cause") not in CAUSES:
        raise ResultsError(f"{trial}: cause must be one of {', '.join(CAUSES)}")
    summary = str(entry.get("summary") or "").strip()
    if not summary or "\n" in summary:
        raise ResultsError(f"{trial}: the summary must be one sentence on one line")
    return {"cause": entry["cause"], "summary": summary}


def _kept_failures(previous: dict[str, Any] | None) -> dict[str, Any]:
    kept: dict[str, Any] = {}
    for task in (previous or {}).get("tasks", []):
        for attempt in task["attempts"]:
            if attempt.get("failure"):
                kept[attempt["trial"]] = attempt["failure"]
    return kept


def _attempt(trial: dict[str, Any], failure: dict[str, str] | None) -> dict[str, Any]:
    return {
        "trial": trial["trial"],
        "reward": trial["reward"],
        "passed": passed(trial["reward"]),
        "input_tokens": trial["input_tokens"],
        "cache_tokens": trial["cache_tokens"],
        "output_tokens": trial["output_tokens"],
        "cost_usd": _round(trial["cost_usd"], 6),
        "agent_seconds": trial["agent_seconds"],
        "trial_seconds": trial["trial_seconds"],
        "error": trial["error"],
        "failure": failure,
    }


def _task(name: str, trials: list[dict[str, Any]], failures: dict[str, dict[str, str]]) -> dict[str, Any]:
    ordered = sorted(trials, key=lambda trial: (trial["started_at"], trial["trial"]))
    attempts = [_attempt(trial, failures.get(trial["trial"])) for trial in ordered]
    rewards = [attempt["reward"] or 0.0 for attempt in attempts]
    return {
        "task": name,
        "digest": _single((trial["digest"] for trial in ordered), f"the digest of task {name}"),
        "mean_reward": round(sum(rewards) / len(rewards), 4),
        "attempts_passed": sum(attempt["passed"] for attempt in attempts),
        "passed_every_attempt": all(attempt["passed"] for attempt in attempts),
        "attempts": attempts,
    }


def _summary(tasks: list[dict[str, Any]], skipped: int) -> dict[str, Any]:
    attempts = [attempt for task in tasks for attempt in task["attempts"]]
    counts = {len(task["attempts"]) for task in tasks}
    if len(counts) != 1:
        raise ResultsError(f"the tasks have different numbers of attempts: {sorted(counts)}")
    by_cause = {cause: 0 for cause in CAUSES}
    for attempt in attempts:
        if attempt["failure"]:
            by_cause[attempt["failure"]["cause"]] += 1
    rewards = [attempt["reward"] or 0.0 for attempt in attempts]
    tasks_every = sum(task["passed_every_attempt"] for task in tasks)
    return {
        "tasks": len(tasks),
        "attempts_per_task": counts.pop(),
        "trials": len(attempts),
        "skipped_trials": skipped,
        "mean_reward": round(sum(rewards) / len(rewards), 4),
        "trials_passed": sum(attempt["passed"] for attempt in attempts),
        "tasks_passed_every_attempt": tasks_every,
        "pass_all_attempts": round(tasks_every / len(tasks), 4),
        "cost_usd": round(sum(attempt["cost_usd"] or 0.0 for attempt in attempts), 6),
        "failures_by_cause": by_cause,
    }


def _needs_provenance(trials: list[dict[str, Any]]) -> None:
    lacking = sorted(
        f"{trial['trial']} (no {' or '.join(name for name in ('server', 'judge') if not trial[name])})"
        for trial in trials
        if trial["reward"] is not None and not (trial["server"] and trial["judge"])
    )
    if lacking:
        raise ResultsError("graded trials without server or judge information: " + "; ".join(lacking))


def build(
    all_trials: list[dict[str, Any]],
    harbor_versions: set[str],
    benchmark: str,
    benchmark_version: str | None,
    notes: list[str],
    classifications: dict[str, Any],
    previous: dict[str, Any] | None = None,
) -> dict[str, Any]:
    if not isinstance(classifications, dict):
        raise ResultsError("the classifications file must hold one JSON object keyed by trial name")
    names = {trial["trial"] for trial in all_trials}
    unknown = sorted(set(classifications) - names)
    if unknown:
        raise ResultsError(f"classified trials not in the job folders: {', '.join(unknown)}")
    trials = [trial for trial in all_trials if not trial["skipped"]]
    skipped = [
        {"task": trial["task"], "trial": trial["trial"], "reason": trial["skipped"]}
        for trial in sorted(all_trials, key=lambda trial: (trial["task"], trial["trial"]))
        if trial["skipped"]
    ]
    if not trials:
        raise ResultsError("every trial was skipped, so there is nothing to record")
    _needs_provenance(trials)
    scored = {trial["trial"] for trial in trials}
    defaults = {trial["trial"]: NO_AGENT_OUTPUT for trial in trials if trial["no_agent_output"]}
    failures = {**defaults, **_kept_failures(previous), **classifications}
    failures = {name: _check_failure(name, entry) for name, entry in failures.items() if name in scored}
    skipped_but_classified = sorted(set(classifications) - scored)
    if skipped_but_classified:
        raise ResultsError(f"skipped trials carry a failure: {', '.join(skipped_but_classified)}")
    wrongly_classified = sorted(
        trial["trial"] for trial in trials if passed(trial["reward"]) and trial["trial"] in failures
    )
    if wrongly_classified:
        raise ResultsError(f"passed trials carry a failure: {', '.join(wrongly_classified)}")
    unclassified = sorted(
        f"{trial['task']} ({trial['trial']}, reward {trial['reward']})"
        for trial in trials
        if not passed(trial["reward"]) and trial["trial"] not in failures
    )
    if unclassified:
        raise ResultsError("failures without a cause and a sentence: " + "; ".join(unclassified))
    by_task: dict[str, list[dict[str, Any]]] = {}
    for trial in trials:
        by_task.setdefault(trial["task"], []).append(trial)
    tasks = [_task(name, by_task[name], failures) for name in sorted(by_task)]
    server = _single((trial["server"] for trial in trials), "the server build") or {"version": None, "commit": None}
    return {
        "schema": SCHEMA_VERSION,
        "benchmark": benchmark,
        "benchmark_version": benchmark_version or (previous or {}).get("benchmark_version"),
        "harbor_version": _single(harbor_versions, "the Harbor version"),
        "server": server,
        "model": _single((trial["model"] for trial in all_trials), "the model"),
        "judge": _single((trial["judge"] for trial in trials), "the judge model"),
        "settings": _single((trial["settings"] for trial in all_trials), "the settings"),
        "started_at": min(trial["started_at"] for trial in all_trials),
        "finished_at": max(trial["finished_at"] for trial in all_trials),
        "notes": notes or (previous or {}).get("notes", []),
        "summary": _summary(tasks, len(skipped)),
        "skipped": skipped,
        "tasks": tasks,
    }


def dumps(result: dict[str, Any]) -> str:
    return json.dumps(result, indent=2, ensure_ascii=False) + "\n"


REQUIRED_RUN_KEYS = ("benchmark", "server", "model", "started_at", "summary", "tasks", "skipped")
REQUIRED_SUMMARY_KEYS = (
    "tasks",
    "attempts_per_task",
    "trials",
    "skipped_trials",
    "mean_reward",
    "trials_passed",
    "tasks_passed_every_attempt",
    "cost_usd",
    "failures_by_cause",
)


def _check_run(name: str, run: dict[str, Any]) -> None:
    if run["schema"] != SCHEMA_VERSION:
        raise ResultsError(f"{name}: schema {run['schema']!r} is not {SCHEMA_VERSION}")
    missing = [key for key in REQUIRED_RUN_KEYS if key not in run]
    summary = run.get("summary")
    if isinstance(summary, dict):
        missing += [f"summary.{key}" for key in REQUIRED_SUMMARY_KEYS if key not in summary]
    if missing:
        raise ResultsError(f"{name}: missing {', '.join(missing)}")


def load_runs(results_dir: Path) -> list[tuple[str, dict[str, Any]]]:
    runs = []
    for path in sorted(results_dir.glob("*.json")):
        try:
            run = _read_json(path)
        except json.JSONDecodeError as error:
            raise ResultsError(f"{path.name} is not valid JSON: {error}") from None
        if not isinstance(run, dict) or "schema" not in run:
            continue
        _check_run(path.name, run)
        runs.append((path.name, run))
    return sorted(runs, key=lambda run: (run[1]["started_at"], run[0]))


def _cause_text(by_cause: dict[str, int]) -> str:
    parts = [f"{count} {cause.replace('_', ' ')}" for cause, count in by_cause.items() if count]
    return ", ".join(parts) or "none"


def render_table(runs: list[tuple[str, dict[str, Any]]]) -> str:
    header = (
        "| Run | Date | Server | Model | Tasks | Attempts | Mean reward | Trials passed | Pass^n | Skipped | Cost (USD) | Failures |\n"
        "| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |\n"
    )
    rows = []
    for name, run in runs:
        summary = run["summary"]
        server = run["server"]["version"] or "unknown"
        rows.append(
            f"| [{name}]({name}) | {run['started_at'][:10]} | {server} | {run['model']} | {summary['tasks']} "
            f"| {summary['attempts_per_task']} | {summary['mean_reward']:.3f} "
            f"| {summary['trials_passed']}/{summary['trials']} "
            f"| {summary['tasks_passed_every_attempt']}/{summary['tasks']} "
            f"| {summary['skipped_trials']} | {summary['cost_usd']:.3f} | {_cause_text(summary['failures_by_cause'])} |\n"
        )
    return header + "".join(rows)


def render_readme(current: str, runs: list[tuple[str, dict[str, Any]]]) -> str:
    head, found, _ = current.partition(README_TABLE_HEADING)
    if not found:
        raise ResultsError(f"the README has no {README_TABLE_HEADING.strip()!r} heading")
    return head + README_TABLE_HEADING + "\n" + render_table(runs)


def sync_readme(results_dir: Path) -> bool:
    readme = results_dir / "README.md"
    current = readme.read_text(encoding="utf-8")
    updated = render_readme(current, load_runs(results_dir))
    if updated == current:
        return False
    readme.write_text(updated, encoding="utf-8")
    return True


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(prog="python -m slashobserve_bench.results")
    commands = parser.add_subparsers(dest="command", required=True)
    write = commands.add_parser("write", help="write one results file from Harbor job folders")
    write.add_argument("--job", type=Path, action="append", required=True, help="a Harbor job folder; repeat for a run split over several")
    write.add_argument("--benchmark", required=True)
    write.add_argument("--benchmark-version")
    write.add_argument("--classifications", type=Path, help='JSON: {"<trial>": {"cause": ..., "summary": "..."}} for failures')
    write.add_argument("--note", action="append", default=[])
    write.add_argument("--out", type=Path, required=True)
    commands.add_parser("readme", help="rewrite the README table from the results files").add_argument(
        "--dir", type=Path, default=RESULTS_DIR
    )
    args = parser.parse_args(argv)
    try:
        if args.command == "readme":
            sync_readme(args.dir)
            return 0
        trials, harbor_versions = read_jobs(args.job)
        classifications = _read_json(args.classifications) if args.classifications else {}
        previous = _read_json(args.out) if args.out.is_file() else None
        result = build(
            trials, harbor_versions, args.benchmark, args.benchmark_version, args.note, classifications, previous
        )
        args.out.parent.mkdir(parents=True, exist_ok=True)
        args.out.write_text(dumps(result), encoding="utf-8")
        if (args.out.parent / "README.md").is_file():
            sync_readme(args.out.parent)
    except (ResultsError, OSError, json.JSONDecodeError) as error:
        print(f"error: {error}", file=sys.stderr)
        return 1
    print(f"wrote {args.out}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
