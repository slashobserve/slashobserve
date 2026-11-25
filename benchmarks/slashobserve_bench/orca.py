import argparse
import importlib
import json
import re
import sys
import tomllib
from collections.abc import Callable, Iterable
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any
from zoneinfo import ZoneInfo

from pydantic import BaseModel, ConfigDict, Field, model_validator

DATASET = "orca-bench/orca-bench"
REPORT_PATH = "/app/report.md"
CODE_DIR = "/app/opentelemetry-demo"
ENV_READY = "/tmp/env-ready"
ENV_PORTS = "/tmp/env-ports"
NETWORK_PATTERN = re.compile(r"^otel-[0-9a-f]{12}_default$")
INSTRUCTION_TIME = re.compile(r"The current time is (?P<stamp>[A-Z][a-z]{2} \d{1,2}, \d{4} at \d{1,2}:\d{2}) ET\b")
TIMESTAMP = re.compile(r"\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}|\b\d{1,2}:\d{2}(?::\d{2})?\s*(?:UTC|Z)\b")
TIMELINE_LINES_MAX = 8
LINE_RUNES_MAX = 240

LOGS_UID = "logs"
SPANS_UID = "traces"
METRICS_UID = "prometheus"
LOGS_INDEX = "otel-logs-*"
SPANS_INDEX = "jaeger-main-jaeger-span-*"


@dataclass(frozen=True)
class Task:
    package: str
    tier: str
    control: bool


SUBSET: tuple[Task, ...] = (
    Task("a56417089b997106", "medium", False),
    Task("744d0493fb36583e", "medium", False),
    Task("8f6ac0a21de339d0", "medium", False),
    Task("4ac60dcbae70cfa6", "medium", False),
    Task("9bdcf561ef5292ea", "hard", False),
    Task("8b1ceddb8266f879", "hard", False),
    Task("4bde361822297c8d", "hard", False),
    Task("66b6de8ca5be11a0", "hard", False),
    Task("293235fad0bce255", "medium", True),
    Task("7be0e30896da6467", "hard", True),
)


class UnsupportedTask(Exception):
    pass


class Options(BaseModel):
    model_config = ConfigDict(extra="forbid")

    server_image: str = "ghcr.io/slashobserve/slashobserve:latest"
    replay_dir: str | None = None
    llm_effort: str | None = None
    investigation_calls: int | None = Field(default=None, ge=6, le=60)
    run_max_cost_usd: float | None = Field(default=None, gt=0)
    spend_cap_usd: float | None = Field(default=None, gt=0)
    ledger_path: str | None = None
    wall_timeout_sec: int = Field(default=3300, ge=60)
    code_context: str | None = None

    @model_validator(mode="after")
    def _cap_needs_ledger(self) -> "Options":
        if (self.spend_cap_usd is None) != (self.ledger_path is None):
            raise ValueError("spend_cap_usd and ledger_path go together")
        return self


def include_flags(tasks: Iterable[Task] = SUBSET) -> list[str]:
    return [part for task in tasks for part in ("--include-task-name", f"*{task.package}")]


@dataclass(frozen=True)
class Endpoints:
    prometheus: str
    opensearch: str

    @classmethod
    def from_ports_file(cls, text: str) -> "Endpoints":
        values = {}
        for line in text.splitlines():
            name, separator, value = line.removeprefix("export ").partition("=")
            if separator:
                values[name.strip()] = value.strip()
        missing = [name for name in ("PROMETHEUS_URL", "OPENSEARCH_URL") if not values.get(name)]
        if missing:
            raise UnsupportedTask(f"{ENV_PORTS} sets no {', '.join(missing)}")
        return cls(values["PROMETHEUS_URL"], values["OPENSEARCH_URL"])


def otel_network(names: Iterable[str]) -> str:
    found = sorted(name for name in names if NETWORK_PATTERN.match(name))
    if len(found) != 1:
        raise UnsupportedTask(f"expected one otel-<hash>_default network on the main container, found {found}")
    return found[0]


def datasources_yaml(endpoints: Endpoints) -> str:
    prometheus, opensearch = json.dumps(endpoints.prometheus), json.dumps(endpoints.opensearch)
    return "\n".join(
        [
            "datasources:",
            f"  - uid: {METRICS_UID}",
            "    type: prometheus",
            "    name: Prometheus",
            "    prometheus:",
            f"      url: {prometheus}",
            f"  - uid: {LOGS_UID}",
            "    type: opensearch",
            "    name: Logs",
            "    opensearch:",
            f"      url: {opensearch}",
            f"      index: {json.dumps(LOGS_INDEX)}",
            f"  - uid: {SPANS_UID}",
            "    type: opensearch",
            "    name: Spans",
            "    opensearch:",
            f"      url: {opensearch}",
            f"      index: {json.dumps(SPANS_INDEX)}",
            "      timeField: startTimeMillis",
            "      fields:",
            "        message: operationName",
            "        service: process.serviceName",
            "        traceId: traceID",
            "        spanId: spanID",
            "",
        ]
    )


def _utc_text(moment: datetime) -> str:
    return moment.astimezone(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def _eastern(moment: datetime) -> datetime:
    return moment if moment.tzinfo else moment.replace(tzinfo=ZoneInfo("America/New_York"))


def current_from_task(task_toml: str) -> datetime | None:
    value = tomllib.loads(task_toml).get("metadata", {}).get("current")
    if isinstance(value, datetime):
        return _eastern(value)
    if isinstance(value, str):
        try:
            return _eastern(datetime.fromisoformat(value))
        except ValueError:
            return None
    return None


def current_from_instruction(instruction: str) -> datetime | None:
    found = INSTRUCTION_TIME.search(instruction)
    if found is None:
        return None
    return _eastern(datetime.strptime(found["stamp"], "%b %d, %Y at %H:%M"))


def as_of(task_toml: str, instruction: str) -> str:
    moment = current_from_task(task_toml) or current_from_instruction(instruction)
    if moment is None:
        raise UnsupportedTask("the task gives no current time in task.toml or in its instruction")
    return _utc_text(moment)


def question(instruction: str, as_of_utc: str) -> str:
    notes = [
        f"The current time is {as_of_utc} (UTC). Times in the task above are US Eastern; the telemetry is stored in UTC, so convert them before querying and give every time in UTC.",
        f"Metrics are the datasource `{METRICS_UID}`, logs are `{LOGS_UID}` and spans are `{SPANS_UID}` (Jaeger span documents in OpenSearch).",
        "The Grafana and file instructions above do not apply: your answer is saved as a finding, and the report is written from it. When nothing was wrong, say so and set no_incident.",
    ]
    return instruction.strip() + "\n\nNotes for this harness:\n" + "\n".join(f"- {note}" for note in notes) + "\n"


def start_body(question_text: str, as_of_utc: str) -> dict[str, Any]:
    return {"kind": "investigate", "prompt": question_text, "as_of": as_of_utc}


def load_code_context(path: str) -> Callable[..., None]:
    module, separator, name = path.partition(":")
    if not separator or not module or not name:
        raise ValueError("code_context is <module>:<function>")
    return getattr(importlib.import_module(module), name)


def _moment(value: str) -> datetime | None:
    try:
        parsed = datetime.fromisoformat(value.strip())
    except ValueError:
        return None
    return parsed if parsed.tzinfo else parsed.replace(tzinfo=timezone.utc)


def clock(value: str) -> str:
    moment = _moment(value)
    return value.strip() if moment is None else moment.astimezone(timezone.utc).strftime("%Y-%m-%d %H:%M UTC")


@dataclass(frozen=True)
class Answer:
    markdown: str
    component: str | None = None
    kind: str | None = None
    cause: str | None = None
    onset: str | None = None
    impact: str | None = None
    ruled_out: tuple[str, ...] = ()
    next_steps: tuple[str, ...] = ()
    no_incident: bool = False

    @classmethod
    def from_finding(cls, finding: dict[str, Any]) -> "Answer":
        root = finding.get("root_cause") or {}
        return cls(
            markdown=(finding.get("markdown") or "").strip(),
            component=root.get("component"),
            kind=root.get("kind"),
            cause=root.get("summary"),
            onset=finding.get("onset"),
            impact=finding.get("impact"),
            ruled_out=tuple(finding.get("ruled_out") or ()),
            next_steps=tuple(finding.get("next_steps") or ()),
            no_incident=bool(finding.get("no_incident")),
        )

    @property
    def cause_line(self) -> str | None:
        if not self.cause:
            return None
        label = f"{self.component} ({self.kind.replace('_', ' ')})" if self.component and self.kind else self.component
        return f"{label}: {self.cause}" if label else self.cause


def _clip(line: str) -> str:
    line = line.strip().lstrip("#-*> ").strip()
    return line if len(line) <= LINE_RUNES_MAX else line[: LINE_RUNES_MAX - 1] + "…"


def demote_headings(markdown: str) -> str:
    lines, fenced = [], False
    for line in markdown.splitlines():
        if line.lstrip().startswith("```"):
            fenced = not fenced
        elif not fenced and (found := re.match(r"(#{1,6})(\s)", line)):
            line = "#" * min(len(found[1]) + 2, 6) + line[len(found[1]) :]
        lines.append(line)
    return "\n".join(lines)


def _first_paragraph(markdown: str) -> str:
    for block in re.split(r"\n\s*\n", markdown):
        text = " ".join(line.strip() for line in block.splitlines() if not line.lstrip().startswith("#")).strip()
        if text:
            return text
    return ""


def _summary(answer: Answer) -> str:
    lines = [line for line in (answer.cause_line, answer.impact and f"Impact: {answer.impact}") if line]
    if answer.onset:
        lines.append(f"Started at {clock(answer.onset)}.")
    return " ".join(lines) if lines else _first_paragraph(answer.markdown) or "No summary was written."


def _timeline(answer: Answer) -> str:
    if answer.onset:
        entry = f"{clock(answer.onset)} - {answer.cause_line or 'incident began'}"
        return entry if not answer.impact else f"{entry}\n{clock(answer.onset)} - impact: {answer.impact}"
    lines = [_clip(line) for line in answer.markdown.splitlines() if TIMESTAMP.search(line)]
    return "\n".join(lines[:TIMELINE_LINES_MAX]) or "No incident start time was reported."


def _whys(answer: Answer) -> str:
    parts = [f"Problem statement:\n{answer.impact or _first_paragraph(answer.markdown) or 'Not stated.'}"]
    if answer.markdown:
        parts.append(f"Analysis:\n{demote_headings(answer.markdown)}")
    if answer.cause_line:
        parts.append(f"Root cause:\n{answer.cause_line}")
    if answer.ruled_out:
        parts.append("Unexplored branches (checked and ruled out):\n" + "\n".join(f"- {item}" for item in answer.ruled_out))
    return "\n\n".join(parts)


def _remediation(answer: Answer) -> str:
    if not answer.next_steps:
        return "No remediation was suggested."
    rows = [f"| {step} | Root cause | Unclassified |" for step in answer.next_steps]
    return "\n".join(["| Action | Targets | Type |", "|--------|---------|------|", *rows])


def render_report(finding: dict[str, Any]) -> str:
    answer = Answer.from_finding(finding)
    if answer.no_incident:
        return ""
    sections = (
        ("Summary", _summary(answer)),
        ("Timeline", _timeline(answer)),
        ("5 Whys", _whys(answer)),
        ("Remediation", _remediation(answer)),
    )
    return "\n\n".join(f"## {title}\n\n{body}" for title, body in sections) + "\n"


def report_for(request: dict[str, Any]) -> str | None:
    finding = request.get("finding")
    return None if not finding else render_report(finding)


def compose_project(session_id: str) -> str:
    name = session_id.lower()
    if not re.match(r"[a-z0-9]", name):
        name = "0" + name
    return re.sub(r"[^a-z0-9_-]", "-", name)


def write_notes(logs_dir: Path, lines: Iterable[str]) -> None:
    logs_dir.mkdir(parents=True, exist_ok=True)
    (logs_dir / "notes.txt").write_text("".join(f"{line}\n" for line in lines), encoding="utf-8")


def replay_fixture(template: dict[str, Any], question_text: str) -> dict[str, Any]:
    return {**template, "prompt": question_text.strip()}


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(prog="python -m slashobserve_bench.orca")
    commands = parser.add_subparsers(dest="command", required=True)
    commands.add_parser("include-flags", help="print --include-task-name flags for the fixed ten-task subset, one per line")
    fixture = commands.add_parser("replay-fixture", help="write the replay fixture for one downloaded task")
    fixture.add_argument("--task-dir", type=Path, required=True)
    fixture.add_argument("--template", type=Path, required=True)
    fixture.add_argument("--out", type=Path, required=True)
    args = parser.parse_args(argv)
    if args.command == "include-flags":
        print("\n".join(include_flags()))
        return 0
    instruction = (args.task_dir / "instruction.md").read_text(encoding="utf-8")
    moment = as_of((args.task_dir / "task.toml").read_text(encoding="utf-8"), instruction)
    template = json.loads(args.template.read_text(encoding="utf-8"))
    args.out.mkdir(parents=True, exist_ok=True)
    (args.out / f"{args.task_dir.name}.json").write_text(
        json.dumps(replay_fixture(template, question(instruction, moment)), indent=2) + "\n", encoding="utf-8"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
