import json
from pathlib import Path

import pytest

from slashobserve_bench import answer_checks as checks

FULL_ID = "4bf92f3577b34da6a3ce929d0e0e4736"


def _names(markdown: str) -> list[str]:
    return [issue.check for issue in checks.check_markdown(markdown)]


@pytest.mark.parametrize(
    "markdown",
    [
        f"The failing trace is {FULL_ID}, and it took 412.7 ms.",
        "The origin is span 00f067aa0ba902b7 of trace id " + FULL_ID + ".",
        "Errors rose from 10:15 to 10:45 UTC on 2026-04-20.",
        "The p99 was 1.84 s over the last 6 hours.",
        "Checked `rate(http_requests_total[5m])` and `{service_name=~\"a|b\"} |= \"about 5 ms\"`.",
        "Build 3f2a9c1d rolled out at 09:02.",
    ],
)
def test_clean_answers_raise_nothing(markdown: str) -> None:
    assert _names(markdown) == []


@pytest.mark.parametrize(
    ("markdown", "check"),
    [
        ("See trace 4bf92f3577b34da6… for the slow call.", checks.SHORT_TRACE_ID),
        ("See 4bf92f3577b34da6a3ce... for the slow call.", checks.SHORT_TRACE_ID),
        ("Trace ID: 4bf92f3577b3 shows the error.", checks.SHORT_TRACE_ID),
        ("The peak was between 120 and 135 ms.", checks.RANGE_FOR_VALUE),
        ("The peak was 120-135 ms.", checks.RANGE_FOR_VALUE),
        ("The error share was 4.1%–4.6%.", checks.RANGE_FOR_VALUE),
        ("The queue drained in about 20 minutes.", checks.HEDGED_VALUE),
        ("The error rate was ~4.2%.", checks.HEDGED_VALUE),
        ("It handled at least 300 requests.", checks.HEDGED_VALUE),
    ],
)
def test_each_check_fires(markdown: str, check: str) -> None:
    assert _names(markdown) == [check]


def test_a_run_without_a_finding_is_flagged_with_its_failure() -> None:
    issues = checks.check_run({"state": "failed", "failure_code": "budget_exhausted"})
    assert issues == [checks.Issue(checks.NO_FINDING, "budget_exhausted")]


def _trial(job: Path, name: str, markdown: str | None, reward: float | None) -> None:
    agent = job / name / "agent"
    agent.mkdir(parents=True)
    run = {"state": "succeeded", "finding": {"markdown": markdown}} if markdown else {"state": "failed"}
    (agent / "run.json").write_text(json.dumps(run))
    if reward is not None:
        result = {"verifier_result": {"rewards": {"reward": reward}}}
        (job / name / "result.json").write_text(json.dumps(result))


def test_summary_splits_flags_by_pass_and_fail(tmp_path: Path) -> None:
    _trial(tmp_path, "a__1", "The peak was about 20 ms.", 0.65)
    _trial(tmp_path, "a__2", "The peak was 21.4 ms.", 1.0)
    _trial(tmp_path, "b__1", "The peak was 120-135 ms, roughly 128 ms.", 0.5)
    _trial(tmp_path, "b__2", "The p50 was about 3 ms.", 1.0)
    _trial(tmp_path, "c__1", None, 0.0)
    _trial(tmp_path, "d__1", "The count was 7.", 0.4)
    _trial(tmp_path, "e__1", "Unjudged at about 5 ms.", None)

    summary = checks.summarize(checks.check_job(tmp_path))

    assert summary == {
        "trials": 7,
        "failed": 4,
        "failed_flagged": 3,
        "failed_by_check": {checks.NO_FINDING: 1, checks.RANGE_FOR_VALUE: 1, checks.HEDGED_VALUE: 2},
        "passed": 2,
        "passed_flagged": 1,
        "passed_by_check": {checks.HEDGED_VALUE: 1},
    }


def test_main_fails_on_a_folder_without_trials(tmp_path: Path, capsys: pytest.CaptureFixture[str]) -> None:
    assert checks.main(["--job", str(tmp_path)]) == 1
    assert "no trial" in capsys.readouterr().err
