import json
from pathlib import Path

import pytest

from slashobserve_bench import results

SERVER_LOG = json.dumps(
    {"time": "2026-11-23T11:00:00Z", "level": "INFO", "msg": "starting slashobserve-server", "version": "0.1.5", "commit": "abc1234"}
)


def write_trial(
    job: Path,
    task: str,
    suffix: str,
    reward: float | None,
    *,
    cost: float = 0.05,
    started: str = "2026-11-23T11:00:00Z",
    model: str = "anthropic/claude-sonnet-5-5",
    kwargs: dict | None = None,
    exception: dict | None = None,
    version: str = "0.1.5",
    agent_dir: bool = True,
    skipped_marker: str | None = None,
    graded: bool = True,
    run: dict | None = None,
):
    trial = job / f"{task}__{suffix}"
    trial.mkdir(parents=True)
    (trial / "verifier").mkdir()
    if agent_dir:
        (trial / "agent").mkdir()
        (trial / "agent" / "server.log").write_text(SERVER_LOG.replace("0.1.5", version) + "\n")
    if run is not None:
        (trial / "agent" / "run.json").write_text(json.dumps(run))
    if skipped_marker:
        (trial / "agent" / "skipped.txt").write_text(skipped_marker + "\n")
    if reward is not None and graded:
        (trial / "verifier" / "grading_details.json").write_text(json.dumps({"score": reward, "grading_model": "claude-haiku-5-5@high"}))
    (trial / "result.json").write_text(
        json.dumps(
            {
                "task_name": task,
                "trial_name": trial.name,
                "task_checksum": f"sum-{task}",
                "config": {"agent": {"model_name": model, "kwargs": kwargs or {"server_image": "x", "run_max_cost_usd": 0.5}}},
                "agent_result": {"n_input_tokens": 1000, "n_cache_tokens": 900, "n_output_tokens": 100, "cost_usd": cost},
                "verifier_result": None if reward is None else {"rewards": {"reward": reward}},
                "exception_info": exception,
                "started_at": started,
                "finished_at": "2026-11-23T11:01:30Z",
                "agent_execution": {"started_at": "2026-11-23T11:00:10Z", "finished_at": "2026-11-23T11:00:40.5Z"},
            }
        )
    )
    return trial


def write_job(root: Path, name: str = "job") -> Path:
    job = root / name
    job.mkdir(parents=True)
    (job / "lock.json").write_text(json.dumps({"harbor": {"version": "0.24.0"}}))
    (job / "result.json").write_text("{}")
    return job


def build_from(jobs: list[Path], classifications: dict | None = None, previous=None):
    trials, harbor = results.read_jobs(jobs)
    return results.build(trials, harbor, "o11y-bench", None, [], classifications or {}, previous)


def failure(cause="agent_mistake", summary="The agent read the wrong figure."):
    return {"cause": cause, "summary": summary}


def test_one_attempt_per_task(tmp_path):
    job = write_job(tmp_path)
    write_trial(job, "task-a", "AAA", 1.0, cost=0.1)
    failed = write_trial(job, "task-b", "BBB", 0.4, cost=0.2)
    run = build_from([job], {failed.name: failure()})
    assert run["benchmark"] == "o11y-bench"
    assert run["harbor_version"] == "0.24.0"
    assert run["server"] == {"version": "0.1.5", "commit": "abc1234"}
    assert run["model"] == "anthropic/claude-sonnet-5-5"
    assert run["judge"] == "claude-haiku-5-5@high"
    assert run["settings"] == {
        "investigation_calls": "server default",
        "investigation_loop": True,
        "investigation_calls_max": 40,
        "finding_grounding": "enforce",
        "llm_effort": "server default",
        "run_max_cost_usd": 0.5,
        "wall_timeout_sec": None,
    }
    assert [task["task"] for task in run["tasks"]] == ["task-a", "task-b"]
    attempt = run["tasks"][1]["attempts"][0]
    assert attempt["reward"] == 0.4
    assert attempt["passed"] is False
    assert attempt["cost_usd"] == 0.2
    assert attempt["agent_seconds"] == 30.5
    assert attempt["trial_seconds"] == 90.0
    assert attempt["failure"] == failure()
    assert run["tasks"][0]["attempts"][0]["failure"] is None
    assert run["summary"] == {
        "tasks": 2,
        "attempts_per_task": 1,
        "trials": 2,
        "skipped_trials": 0,
        "mean_reward": 0.7,
        "trials_passed": 1,
        "tasks_passed_every_attempt": 1,
        "pass_all_attempts": 0.5,
        "cost_usd": 0.3,
        "failures_by_cause": {"adapter_fault": 0, "missing_capability": 0, "agent_mistake": 1},
    }


def test_a_task_passes_only_when_every_attempt_passes(tmp_path):
    job = write_job(tmp_path)
    for index, rewards in enumerate([(1.0, 1.0, 1.0), (1.0, 1.0, 0.5), (0.0, 1.0, 1.0)]):
        for attempt, reward in enumerate(rewards):
            write_trial(job, f"task-{index}", f"T{index}{attempt}", reward, started=f"2026-11-23T11:0{attempt}:00Z")
    classifications = {
        f"task-{index}__T{index}{attempt}": failure()
        for index, attempt in ((1, 2), (2, 0))
    }
    run = build_from([job], classifications)
    flags = [task["passed_every_attempt"] for task in run["tasks"]]
    assert flags == [True, False, False]
    assert [task["attempts_passed"] for task in run["tasks"]] == [3, 2, 2]
    assert [attempt["trial"][-1] for attempt in run["tasks"][2]["attempts"]] == ["0", "1", "2"]
    assert run["summary"]["attempts_per_task"] == 3
    assert run["summary"]["trials"] == 9
    assert run["summary"]["trials_passed"] == 7
    assert run["summary"]["tasks_passed_every_attempt"] == 1
    assert run["summary"]["pass_all_attempts"] == pytest.approx(0.3333, abs=1e-4)


def test_one_run_can_span_several_job_folders(tmp_path):
    first = write_job(tmp_path, "first")
    second = write_job(tmp_path, "second")
    write_trial(first, "task-a", "AAA", 1.0)
    write_trial(second, "task-b", "BBB", 1.0)
    run = build_from([first, second])
    assert run["summary"]["tasks"] == 2


def test_an_attempt_split_over_two_jobs_counts_for_one_task(tmp_path):
    first = write_job(tmp_path, "first")
    second = write_job(tmp_path, "second")
    write_trial(first, "task-a", "AAA", 1.0)
    write_trial(second, "task-a", "BBB", 1.0, started="2026-11-23T12:00:00Z")
    run = build_from([first, second])
    assert run["summary"]["attempts_per_task"] == 2
    assert run["tasks"][0]["passed_every_attempt"] is True


def test_tasks_with_different_attempt_counts_are_refused(tmp_path):
    job = write_job(tmp_path)
    write_trial(job, "task-a", "AAA", 1.0)
    write_trial(job, "task-a", "AAB", 1.0)
    write_trial(job, "task-b", "BBB", 1.0)
    with pytest.raises(results.ResultsError, match="different numbers of attempts"):
        build_from([job])


def test_a_failure_without_a_classification_is_refused(tmp_path):
    job = write_job(tmp_path)
    write_trial(job, "task-a", "AAA", 0.9)
    with pytest.raises(results.ResultsError, match="task-a"):
        build_from([job])


@pytest.mark.parametrize(
    "entry",
    [
        {"cause": "bad luck", "summary": "x."},
        {"cause": "agent_mistake", "summary": ""},
        {"cause": "agent_mistake", "summary": "two\nlines"},
        "agent_mistake",
    ],
)
def test_a_classification_needs_a_known_cause_and_one_line(tmp_path, entry):
    job = write_job(tmp_path)
    trial = write_trial(job, "task-a", "AAA", 0.9)
    with pytest.raises(results.ResultsError):
        build_from([job], {trial.name: entry})


def test_a_classification_for_a_passed_or_unknown_trial_is_refused(tmp_path):
    job = write_job(tmp_path)
    trial = write_trial(job, "task-a", "AAA", 1.0)
    with pytest.raises(results.ResultsError, match="carry a failure"):
        build_from([job], {trial.name: failure()})
    with pytest.raises(results.ResultsError, match="not in the job folders"):
        build_from([job], {"missing__X": failure()})


def test_a_trial_that_errored_is_a_failure_with_no_reward(tmp_path):
    job = write_job(tmp_path)
    trial = write_trial(job, "task-a", "AAA", None, exception={"exception_type": "SpendCapReached"})
    run = build_from([job], {trial.name: failure("adapter_fault", "The job spend cap stopped the trial.")})
    attempt = run["tasks"][0]["attempts"][0]
    assert attempt["reward"] is None
    assert attempt["error"] == "SpendCapReached"
    assert run["summary"]["mean_reward"] == 0.0
    assert run["summary"]["failures_by_cause"]["adapter_fault"] == 1


def test_trials_that_disagree_on_the_model_are_refused(tmp_path):
    job = write_job(tmp_path)
    write_trial(job, "task-a", "AAA", 1.0)
    write_trial(job, "task-b", "BBB", 1.0, model="anthropic/claude-haiku-5-5")
    with pytest.raises(results.ResultsError, match="the model"):
        build_from([job])


def test_trials_that_disagree_on_the_settings_or_server_are_refused(tmp_path):
    job = write_job(tmp_path)
    write_trial(job, "task-a", "AAA", 1.0)
    write_trial(job, "task-b", "BBB", 1.0, kwargs={"investigation_calls": 30})
    with pytest.raises(results.ResultsError, match="the settings"):
        build_from([job])
    other = write_job(tmp_path, "other")
    write_trial(other, "task-a", "AAA", 1.0)
    write_trial(other, "task-b", "BBB", 1.0, version="0.1.6")
    with pytest.raises(results.ResultsError, match="the server build"):
        build_from([other])


def test_loop_settings_are_recorded_with_their_effective_values(tmp_path):
    kwargs = {"investigation_loop": "true", "investigation_calls_max": "40", "finding_grounding": "report", "run_max_cost_usd": "0.5"}
    job = write_job(tmp_path)
    write_trial(job, "task-a", "AAA", 1.0, kwargs=kwargs)
    settings = build_from([job])["settings"]
    assert settings["investigation_loop"] is True
    assert settings["investigation_calls_max"] == 40
    assert settings["finding_grounding"] == "report"
    assert settings["run_max_cost_usd"] == 0.5


def test_unset_loop_settings_record_what_the_adapter_sent(tmp_path):
    job = write_job(tmp_path)
    write_trial(job, "task-a", "AAA", 1.0, kwargs={"investigation_calls": 20})
    settings = build_from([job])["settings"]
    assert (settings["investigation_calls"], settings["investigation_loop"], settings["investigation_calls_max"], settings["finding_grounding"]) == (20, True, 40, "enforce")


def test_the_older_baseline_is_recorded_as_given(tmp_path):
    job = write_job(tmp_path)
    write_trial(job, "task-a", "AAA", 1.0, kwargs={"investigation_calls": "12", "investigation_loop": "false", "finding_grounding": "off", "llm_effort": "medium"})
    settings = build_from([job])["settings"]
    assert (settings["investigation_calls"], settings["investigation_loop"], settings["finding_grounding"], settings["llm_effort"]) == (12, False, "off", "medium")


def test_jobs_that_set_the_defaults_explicitly_agree_with_jobs_that_leave_them_unset(tmp_path):
    job = write_job(tmp_path)
    write_trial(job, "task-a", "AAA", 1.0)
    write_trial(job, "task-b", "BBB", 1.0, kwargs={"run_max_cost_usd": 0.5, "investigation_loop": True, "investigation_calls_max": 40, "finding_grounding": "enforce"})
    build_from([job])
    other = write_job(tmp_path, "other")
    write_trial(other, "task-a", "AAA", 1.0)
    write_trial(other, "task-b", "BBB", 1.0, kwargs={"run_max_cost_usd": 0.5, "investigation_loop": False})
    with pytest.raises(results.ResultsError, match="the settings"):
        build_from([other])


def test_each_attempt_records_the_task_type_and_the_call_limit_the_run_returned(tmp_path):
    job = write_job(tmp_path)
    newer = {
        "steps": [
            {"message": "Listed 3 services on prom"},
            {"message": "Routed the question to the measure task type"},
        ],
        "usage": {"call_limit": 19, "stage_at_save": "conclude", "model_calls": 9},
    }
    write_trial(job, "task-a", "AAA", 1.0, run=newer)
    write_trial(job, "task-b", "BBB", 1.0, run={"steps": [{"message": "Listed 3 services on prom"}], "usage": {"model_calls": 12}})
    write_trial(job, "task-c", "CCC", 1.0)
    attempts = {task["task"]: task["attempts"][0] for task in build_from([job])["tasks"]}
    assert (attempts["task-a"]["task_type"], attempts["task-a"]["call_limit"], attempts["task-a"]["stage_at_save"]) == ("measure", 19, "conclude")
    for older in ("task-b", "task-c"):
        assert (attempts[older]["task_type"], attempts[older]["call_limit"], attempts[older]["stage_at_save"]) == (None, None, None)


def test_a_folder_without_trials_is_refused(tmp_path):
    job = write_job(tmp_path)
    with pytest.raises(results.ResultsError, match="no trial folders"):
        results.read_jobs([job])


def test_classifications_survive_a_rewrite_with_more_jobs(tmp_path):
    first = write_job(tmp_path, "first")
    failed = write_trial(first, "task-a", "AAA", 0.5)
    previous = build_from([first], {failed.name: failure("missing_capability", "The tool gave no range figure.")})
    second = write_job(tmp_path, "second")
    write_trial(second, "task-b", "BBB", 1.0)
    run = build_from([first, second], previous=previous)
    assert run["tasks"][0]["attempts"][0]["failure"]["cause"] == "missing_capability"


def test_cli_writes_the_file_and_the_readme_table(tmp_path, capsys):
    job = write_job(tmp_path / "jobs")
    failed = write_trial(job, "task-a", "AAA", 0.5)
    write_trial(job, "task-b", "BBB", 1.0)
    classifications = tmp_path / "classifications.json"
    classifications.write_text(json.dumps({failed.name: failure()}))
    out_dir = tmp_path / "results"
    out_dir.mkdir()
    (out_dir / "README.md").write_text("# Results\n\nIntro.\n\n## Runs\n\nstale\n")
    out = out_dir / "run-one.json"
    code = results.main(
        ["write", "--job", str(job), "--benchmark", "o11y-bench", "--classifications", str(classifications), "--note", "a note", "--out", str(out)]
    )
    assert code == 0
    written = json.loads(out.read_text())
    assert written["notes"] == ["a note"]
    readme = (out_dir / "README.md").read_text()
    assert readme.startswith("# Results\n\nIntro.\n\n## Runs\n\n| Run |")
    assert "stale" not in readme
    assert "| [run-one.json](run-one.json) | 2026-11-23 | 0.1.5 | anthropic/claude-sonnet-5-5 | 2 | 1 | 0.750 | 1/2 | 1/2 | 0 | 0.100 | 1 agent mistake |" in readme
    assert results.main(["write", "--job", str(job), "--benchmark", "o11y-bench", "--out", str(out)]) == 0


def test_cli_reports_an_unclassified_failure_and_writes_nothing(tmp_path, capsys):
    job = write_job(tmp_path / "jobs")
    write_trial(job, "task-a", "AAA", 0.5)
    out = tmp_path / "run.json"
    assert results.main(["write", "--job", str(job), "--benchmark", "o11y-bench", "--out", str(out)]) == 1
    assert "task-a" in capsys.readouterr().err
    assert not out.exists()


def test_skipped_trials_stay_out_of_the_scores_and_are_counted(tmp_path):
    job = write_job(tmp_path)
    write_trial(job, "task-a", "AAA", 1.0)
    write_trial(job, "skip-marker", "SSS", None, skipped_marker="skipped: graded on Grafana state")
    write_trial(job, "skip-exception", "EEE", None, exception={"exception_type": "TaskSkipped", "exception_message": "no store"})
    run = build_from([job])
    assert [task["task"] for task in run["tasks"]] == ["task-a"]
    assert run["summary"]["tasks"] == 1
    assert run["summary"]["mean_reward"] == 1.0
    assert run["summary"]["pass_all_attempts"] == 1.0
    assert run["summary"]["skipped_trials"] == 2
    assert run["skipped"] == [
        {"task": "skip-exception", "trial": "skip-exception__EEE", "reason": "no store"},
        {"task": "skip-marker", "trial": "skip-marker__SSS", "reason": "skipped: graded on Grafana state"},
    ]


def test_a_run_of_only_skipped_trials_is_refused(tmp_path):
    job = write_job(tmp_path)
    write_trial(job, "skip-marker", "SSS", None, skipped_marker="skipped: x")
    with pytest.raises(results.ResultsError, match="skipped"):
        build_from([job])


def test_a_trial_without_an_agent_folder_is_an_errored_attempt(tmp_path):
    job = write_job(tmp_path)
    write_trial(job, "task-a", "AAA", 1.0)
    write_trial(job, "task-b", "BBB", None, agent_dir=False)
    run = build_from([job])
    attempt = run["tasks"][1]["attempts"][0]
    assert attempt["reward"] is None
    assert attempt["failure"]["cause"] == "adapter_fault"
    assert run["summary"]["trials"] == 2
    assert run["summary"]["failures_by_cause"]["adapter_fault"] == 1
    reclassified = build_from([job], {"task-b__BBB": failure("agent_mistake", "A person recorded another cause.")})
    assert reclassified["tasks"][1]["attempts"][0]["failure"]["cause"] == "agent_mistake"


def test_a_graded_trial_needs_server_and_judge_information(tmp_path):
    job = write_job(tmp_path)
    write_trial(job, "task-a", "AAA", 1.0)
    write_trial(job, "task-b", "BBB", 1.0, agent_dir=False)
    with pytest.raises(results.ResultsError, match=r"task-b__BBB \(no server\)"):
        build_from([job])
    other = write_job(tmp_path, "other")
    write_trial(other, "task-a", "AAA", 1.0)
    write_trial(other, "task-b", "BBB", 1.0, graded=False)
    with pytest.raises(results.ResultsError, match=r"task-b__BBB \(no judge\)"):
        build_from([other])


def test_a_rewrite_keeps_the_notes_and_benchmark_version_it_is_not_given(tmp_path):
    job = write_job(tmp_path / "jobs")
    write_trial(job, "task-a", "AAA", 1.0)
    out = tmp_path / "run.json"
    base = ["write", "--job", str(job), "--benchmark", "o11y-bench", "--out", str(out)]
    assert results.main([*base, "--benchmark-version", "abc123", "--note", "first", "--note", "second"]) == 0
    assert results.main(base) == 0
    kept = json.loads(out.read_text())
    assert kept["benchmark_version"] == "abc123"
    assert kept["notes"] == ["first", "second"]
    assert results.main([*base, "--benchmark-version", "def456", "--note", "only"]) == 0
    replaced = json.loads(out.read_text())
    assert replaced["benchmark_version"] == "def456"
    assert replaced["notes"] == ["only"]


def test_load_runs_ignores_files_without_a_schema_and_refuses_bad_ones(tmp_path):
    job = write_job(tmp_path / "jobs")
    write_trial(job, "task-a", "AAA", 1.0)
    out_dir = tmp_path / "results"
    assert results.main(["write", "--job", str(job), "--benchmark", "o11y-bench", "--out", str(out_dir / "run.json")]) == 0
    (out_dir / "classifications.json").write_text(json.dumps({"task-a__AAA": failure()}))
    assert [name for name, _ in results.load_runs(out_dir)] == ["run.json"]
    (out_dir / "broken.json").write_text("{not json")
    with pytest.raises(results.ResultsError, match="broken.json"):
        results.load_runs(out_dir)
    (out_dir / "broken.json").write_text(json.dumps({"schema": results.SCHEMA_VERSION}))
    with pytest.raises(results.ResultsError, match="broken.json: missing"):
        results.load_runs(out_dir)
    (out_dir / "broken.json").write_text(json.dumps({"schema": 99}))
    with pytest.raises(results.ResultsError, match="schema 99"):
        results.load_runs(out_dir)


def test_the_readme_sync_survives_a_classifications_file_beside_the_results(tmp_path):
    job = write_job(tmp_path / "jobs")
    write_trial(job, "task-a", "AAA", 1.0)
    out_dir = tmp_path / "results"
    out_dir.mkdir()
    (out_dir / "README.md").write_text("# Results\n\n## Runs\n")
    (out_dir / "classifications.json").write_text("{}")
    assert results.main(["write", "--job", str(job), "--benchmark", "o11y-bench", "--out", str(out_dir / "run.json")]) == 0
    assert "run.json" in (out_dir / "README.md").read_text()


def test_cli_prints_an_error_and_exits_1_on_unreadable_input(tmp_path, capsys):
    job = write_job(tmp_path / "jobs")
    write_trial(job, "task-a", "AAA", 1.0)
    out = tmp_path / "run.json"
    base = ["write", "--job", str(job), "--benchmark", "o11y-bench", "--out", str(out)]
    assert results.main([*base, "--classifications", str(tmp_path / "missing.json")]) == 1
    assert capsys.readouterr().err.startswith("error:")
    bad = tmp_path / "bad.json"
    bad.write_text("{not json")
    assert results.main([*base, "--classifications", str(bad)]) == 1
    assert capsys.readouterr().err.startswith("error:")
    bad.write_text("[]")
    assert results.main([*base, "--classifications", str(bad)]) == 1
    assert "keyed by trial name" in capsys.readouterr().err
    out.write_text("{not json")
    assert results.main(base) == 1
    assert capsys.readouterr().err.startswith("error:")


def test_a_trial_without_the_fields_harbor_writes_is_refused_by_name(tmp_path):
    job = write_job(tmp_path)
    trial = write_trial(job, "task-a", "AAA", 1.0)
    (trial / "result.json").write_text("{}")
    with pytest.raises(results.ResultsError, match="task-a__AAA"):
        results.read_jobs([job])


def test_readme_needs_its_runs_heading():
    with pytest.raises(results.ResultsError):
        results.render_readme("# Results\n", [])


def test_the_kept_results_are_valid_and_the_readme_table_matches_them():
    runs = results.load_runs(results.RESULTS_DIR)
    assert runs
    for name, run in runs:
        assert run["schema"] == results.SCHEMA_VERSION, name
        failures = [attempt for task in run["tasks"] for attempt in task["attempts"] if not attempt["passed"]]
        assert all(attempt["failure"] and attempt["failure"]["cause"] in results.CAUSES for attempt in failures), name
        assert all(attempt["failure"] is None for task in run["tasks"] for attempt in task["attempts"] if attempt["passed"]), name
        assert sum(run["summary"]["failures_by_cause"].values()) == len(failures), name
    for path in results.RESULTS_DIR.glob("*.json"):
        assert "schema" in json.loads(path.read_text()), path.name
    readme = (results.RESULTS_DIR / "README.md").read_text()
    assert readme == results.render_readme(readme, runs)
