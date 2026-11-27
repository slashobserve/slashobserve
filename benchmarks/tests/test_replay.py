import json

from slashobserve_bench import replay


def test_fixture_takes_the_instruction_as_its_prompt():
    fixture = replay.fixture_for("  What is the error rate?\n")
    assert fixture["prompt"] == "What is the error rate?"
    assert fixture["turns"][-1]["tool_calls"][0]["name"] == "save_finding"


def test_command_writes_one_fixture_per_task(tmp_path):
    task_dir = tmp_path / "tasks" / "promql-error-rate"
    task_dir.mkdir(parents=True)
    (task_dir / "instruction.md").write_text("What is the error rate?\n")
    out = tmp_path / "out"
    assert replay.main(["--tasks", str(tmp_path / "tasks"), "--task", "promql-error-rate", "--out", str(out)]) == 0
    assert json.loads((out / "promql-error-rate.json").read_text())["prompt"] == "What is the error rate?"


def test_fixtures_are_copied_readable_for_the_server_user(tmp_path):
    source = tmp_path / "source"
    source.mkdir()
    (source / "a.json").write_text("{}")
    (source / "notes.txt").write_text("x")
    target = tmp_path / "target"
    replay.copy_fixtures(source, target)
    assert sorted(path.name for path in target.iterdir()) == ["a.json"]
    assert target.stat().st_mode & 0o777 == 0o755
    assert (target / "a.json").stat().st_mode & 0o777 == 0o644


def test_a_prompt_with_a_note_gets_the_task_fixture_under_its_own_prompt(tmp_path):
    (tmp_path / "list-datasources.json").write_text(json.dumps(replay.fixture_for("Which datasources?\n")))
    written = replay.rekey(tmp_path, "list-datasources", "Which datasources?\n", "Which datasources?\n\nContext from Grafana.")
    assert written == tmp_path / "list-datasources-sent.json"
    fixture = json.loads(written.read_text())
    assert fixture["prompt"] == "Which datasources?\n\nContext from Grafana."
    assert fixture["turns"] == replay.fixture_for("x")["turns"]
    assert json.loads((tmp_path / "list-datasources.json").read_text())["prompt"] == "Which datasources?"


def test_rekey_leaves_an_unchanged_prompt_or_another_fixture_alone(tmp_path):
    (tmp_path / "task.json").write_text(json.dumps(replay.fixture_for("Other question")))
    assert replay.rekey(tmp_path, "task", "Question", "Question") is None
    assert replay.rekey(tmp_path, "task", "Question", "Question with a note") is None
    assert replay.rekey(tmp_path, "missing", "Question", "Question with a note") is None
    assert sorted(path.name for path in tmp_path.iterdir()) == ["task.json"]
