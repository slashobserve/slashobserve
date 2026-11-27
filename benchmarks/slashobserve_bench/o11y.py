import argparse
import sys
import tomllib
from dataclasses import dataclass
from pathlib import Path
from typing import Any

NETWORK = "o11y-bench-shared"
SCENARIO_TIME_ENV = "O11Y_SCENARIO_TIME_ISO"

STORE_QUESTION_CATEGORIES = ("prometheus_query", "loki_query", "tempo_query")
STORE_CATEGORIES = (*STORE_QUESTION_CATEGORIES, "investigation")
GRAFANA_QUESTION = "grafana_api"
DASHBOARDING = "dashboarding"
GRAFANA_CATEGORIES = (GRAFANA_QUESTION, DASHBOARDING)
QUESTION_CATEGORIES = (*STORE_QUESTION_CATEGORIES, GRAFANA_QUESTION)
CATEGORIES = (*STORE_CATEGORIES, *GRAFANA_CATEGORIES)

SUBSET: tuple[str, ...] = (
    "promql-error-rate",
    "promql-topk-5xx-share",
    "promql-offset-traffic-compare",
    "logql-top-5xx-endpoint",
    "logql-unwrap-orders-p95-latency",
    "traceql-checkout-p99-by-service",
    "traceql-error-chain-orders",
    "incident-triage",
    "payments-path-root-cause",
    "dependency-outage-false-lead",
)

_DASHBOARD_STATE = "graded on the dashboard saved in Grafana, which this agent never writes"

SKIPPED: dict[str, str] = {
    "dashboard-add-cache-lag-panels": _DASHBOARD_STATE,
    "dashboard-add-deployment-annotation": _DASHBOARD_STATE,
    "dashboard-add-retry-backlog-panels": _DASHBOARD_STATE,
    "dashboard-create-cache-incident-review": _DASHBOARD_STATE,
    "dashboard-create-service-overview": _DASHBOARD_STATE,
    "dashboard-repair-cache-review": _DASHBOARD_STATE,
    "dashboard-update-add-service-variable": _DASHBOARD_STATE,
}


class UnsupportedTask(Exception):
    pass


@dataclass(frozen=True)
class Stores:
    prometheus: str
    loki: str
    tempo: str

    @classmethod
    def from_env(cls, env: dict[str, str]) -> "Stores":
        missing = [name for name in ("PROMETHEUS_URL", "LOKI_URL", "TEMPO_URL") if not env.get(name)]
        if missing:
            raise UnsupportedTask(f"the task environment sets no {', '.join(missing)}")
        return cls(env["PROMETHEUS_URL"], env["LOKI_URL"], env["TEMPO_URL"])


def skip_reason(task_id: str) -> str | None:
    return SKIPPED.get(task_id)


def category_of(task_toml: str) -> str:
    category = tomllib.loads(task_toml).get("metadata", {}).get("category")
    if not category:
        raise UnsupportedTask("task.toml has no metadata.category")
    return category


def run_kind(category: str) -> str:
    if category == "investigation":
        return "investigate"
    if category in QUESTION_CATEGORIES:
        return "ask"
    raise UnsupportedTask(f"the category {category!r} is not one this agent can answer")


def start_body(category: str, prompt: str, as_of: str, observation_id: str | None) -> dict[str, Any]:
    kind = run_kind(category)
    body: dict[str, Any] = {"kind": kind, "prompt": prompt, "as_of": as_of}
    if kind == "ask":
        if not observation_id:
            raise UnsupportedTask("a question needs an observation to carry as_of")
        body["observation_id"] = observation_id
    return body


def task_names(tasks_dir: Path) -> list[str]:
    names = []
    for task_toml in sorted(tasks_dir.glob("*/task.toml")):
        task_id = task_toml.parent.name
        if skip_reason(task_id) is None and category_of(task_toml.read_text(encoding="utf-8")) in CATEGORIES:
            names.append(task_id)
    return names


def include_flags(names: list[str]) -> list[str]:
    return [part for name in names for part in ("--include-task-name", name)]


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(prog="python -m slashobserve_bench.o11y")
    commands = parser.add_subparsers(dest="command", required=True)
    flags = commands.add_parser("include-flags", help="print --include-task-name flags for the tasks this agent runs")
    flags.add_argument("--tasks", type=Path, required=True)
    flags.add_argument("--subset", action="store_true", help="keep only the tasks in SUBSET")
    commands.add_parser("skipped", help="print the skipped tasks and why")
    args = parser.parse_args(argv)
    if args.command == "skipped":
        for task_id, reason in SKIPPED.items():
            print(f"{task_id}\tskipped: {reason}")
        return 0
    names = task_names(args.tasks)
    if args.subset:
        names = [name for name in names if name in SUBSET]
    print(" ".join(include_flags(names)))
    return 0


if __name__ == "__main__":
    sys.exit(main())
