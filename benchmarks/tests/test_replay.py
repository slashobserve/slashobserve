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
