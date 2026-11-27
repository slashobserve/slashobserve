import argparse
import json
import re
import sys
from collections import Counter
from collections.abc import Iterable
from dataclasses import dataclass
from pathlib import Path
from typing import Any

NO_FINDING = "no_finding"
SHORT_TRACE_ID = "short_trace_id"
RANGE_FOR_VALUE = "range_for_value"
HEDGED_VALUE = "hedged_value"

CHECKS = (NO_FINDING, SHORT_TRACE_ID, RANGE_FOR_VALUE, HEDGED_VALUE)

_NUMBER = r"\d[\d,]*(?:\.\d+)?"
_UNIT = r"(?:%|ms|µs|us|ns|s|sec|seconds?|minutes?|min|hours?|h|rps|req/s|/s|MiB|GiB|MB|GB|KB|bytes?)"
_HEX = re.compile(r"(?<![0-9a-f])[0-9a-f]{8,31}(?![0-9a-f])")
_CUT = re.compile(r"\s*(?:…|\.\.\.)")
_NEAR_TRACE = re.compile(r"trace(?:\s+id)?\W{0,6}$", re.IGNORECASE)
_SPAN_ID_LENGTH = 16
_RANGES = (
    re.compile(rf"\bbetween\s+{_NUMBER}\s*{_UNIT}?\s+and\s+{_NUMBER}\s*{_UNIT}?", re.IGNORECASE),
    re.compile(rf"(?<![\d:.\-–])({_NUMBER})\s*{_UNIT}?\s*[-–]\s*({_NUMBER})\s*{_UNIT}(?![\w:])"),
)
_HEDGES = re.compile(
    rf"(?:\b(?:about|around|roughly|approximately|nearly|almost|at least|at most)\s+|[~≈]\s*)({_NUMBER})\s*{_UNIT}?",
    re.IGNORECASE,
)
_CODE = re.compile(r"```.*?```|`[^`\n]*`", re.DOTALL)
_EXCERPT_MAX = 80


@dataclass(frozen=True)
class Issue:
    check: str
    excerpt: str


def _excerpt(text: str, start: int, end: int) -> str:
    return " ".join(text[max(0, start - 20) : end + 20].split())[:_EXCERPT_MAX]


def _short_trace_ids(text: str) -> Iterable[Issue]:
    for match in _HEX.finditer(text):
        run = match.group()
        if run.isdigit() or run.isalpha():
            continue
        cut = _CUT.match(text, match.end()) is not None
        named = _NEAR_TRACE.search(text[: match.start()]) is not None and len(run) != _SPAN_ID_LENGTH
        if cut or named:
            yield Issue(SHORT_TRACE_ID, _excerpt(text, match.start(), match.end()))


def check_markdown(markdown: str) -> list[Issue]:
    issues = list(_short_trace_ids(markdown))
    prose = _CODE.sub(" ", markdown)
    for pattern in _RANGES:
        issues += [Issue(RANGE_FOR_VALUE, _excerpt(prose, m.start(), m.end())) for m in pattern.finditer(prose)]
    issues += [Issue(HEDGED_VALUE, _excerpt(prose, m.start(), m.end())) for m in _HEDGES.finditer(prose)]
    return issues


def check_run(run: dict[str, Any]) -> list[Issue]:
    finding = run.get("finding")
    if not finding or not finding.get("markdown"):
        return [Issue(NO_FINDING, str(run.get("failure_code") or run.get("state") or "unknown"))]
    return check_markdown(finding["markdown"])


def _reward(trial_dir: Path) -> float | None:
    path = trial_dir / "result.json"
    if not path.is_file():
        return None
    result = json.loads(path.read_text())
    return ((result.get("verifier_result") or {}).get("rewards") or {}).get("reward")


def check_job(job_dir: Path) -> list[dict[str, Any]]:
    trials = []
    for run_path in sorted(job_dir.glob("*/agent/run.json")):
        trial_dir = run_path.parent.parent
        issues = check_run(json.loads(run_path.read_text()))
        trials.append({"trial": trial_dir.name, "reward": _reward(trial_dir), "issues": issues})
    return trials


def summarize(trials: list[dict[str, Any]]) -> dict[str, Any]:
    failed = [t for t in trials if t["reward"] is not None and t["reward"] != 1.0]
    passed = [t for t in trials if t["reward"] == 1.0]

    def flagged(group: list[dict[str, Any]]) -> dict[str, int]:
        counts = Counter(check for t in group for check in {issue.check for issue in t["issues"]})
        return {check: counts[check] for check in CHECKS if counts[check]}

    return {
        "trials": len(trials),
        "failed": len(failed),
        "failed_flagged": sum(1 for t in failed if t["issues"]),
        "failed_by_check": flagged(failed),
        "passed": len(passed),
        "passed_flagged": sum(1 for t in passed if t["issues"]),
        "passed_by_check": flagged(passed),
    }


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(prog="slashobserve_bench.answer_checks")
    parser.add_argument("--job", action="append", required=True, type=Path)
    parser.add_argument("--list", action="store_true")
    args = parser.parse_args(argv)
    trials = [trial for job in args.job for trial in check_job(job)]
    if not trials:
        print("no trial with agent/run.json found", file=sys.stderr)
        return 1
    if args.list:
        for trial in trials:
            for issue in trial["issues"]:
                print(f'{trial["trial"]}\t{trial["reward"]}\t{issue.check}')
    print(json.dumps(summarize(trials), indent=2))
    return 0


if __name__ == "__main__":
    sys.exit(main())
