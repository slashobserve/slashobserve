import asyncio
import json
from types import SimpleNamespace

import pytest
from harbor.models.agent.context import AgentContext
from harbor.models.trajectories import Trajectory

from slashobserve_bench import agent as bench_agent
from slashobserve_bench import o11y
from slashobserve_bench.ledger import SpendCapReached, SpendLedger


def test_agent_declares_atif_and_mcp_capabilities():
    assert bench_agent.SlashObserveAgent.capabilities.atif
    assert bench_agent.SlashObserveAgent.capabilities.mcp_servers


def test_defaults_need_no_options(tmp_path):
    instance = bench_agent.SlashObserveAgent(logs_dir=tmp_path, model_name="replay/replay")
    assert instance._options.server_image.startswith("ghcr.io/slashobserve/slashobserve:")
    assert instance._options.wall_timeout_sec == 540
    assert instance._ledger is None


@pytest.mark.parametrize("calls", [5, 61])
def test_investigation_calls_stay_within_server_limits(tmp_path, calls):
    with pytest.raises(ValueError):
        bench_agent.SlashObserveAgent(logs_dir=tmp_path, model_name="replay/replay", investigation_calls=calls)


def test_investigation_calls_are_accepted_from_the_command_line(tmp_path):
    instance = bench_agent.SlashObserveAgent(logs_dir=tmp_path, model_name="replay/replay", investigation_calls="30")
    assert instance._options.investigation_calls == 30


def test_loop_settings_are_accepted_from_the_command_line(tmp_path):
    instance = bench_agent.SlashObserveAgent(
        logs_dir=tmp_path,
        model_name="replay/replay",
        investigation_loop="true",
        investigation_calls="20",
        investigation_calls_max="40",
        finding_grounding="report",
    )
    assert instance._options.investigation_loop is True
    assert instance._options.investigation_calls_max == 40
    assert instance._options.finding_grounding == "report"
    unset = bench_agent.SlashObserveAgent(logs_dir=tmp_path, model_name="replay/replay")
    assert unset._options.investigation_loop is None


@pytest.mark.parametrize(
    "kwargs",
    [{"investigation_calls_max": 11}, {"investigation_calls_max": 5}, {"investigation_calls_max": 61}, {"finding_grounding": "strict"}, {"investigation_loop": "maybe"}, {"investigation_calls": 30, "investigation_calls_max": 20}],
)
def test_loop_settings_outside_the_server_limits_are_refused(tmp_path, kwargs):
    with pytest.raises(ValueError):
        bench_agent.SlashObserveAgent(logs_dir=tmp_path, model_name="replay/replay", **kwargs)


def test_calls_max_equal_to_the_default_calls_is_accepted(tmp_path):
    instance = bench_agent.SlashObserveAgent(logs_dir=tmp_path, model_name="replay/replay", investigation_calls_max=12)
    assert instance._options.investigation_calls_max == 12


def test_spend_cap_needs_a_ledger_and_the_other_way_round(tmp_path):
    with pytest.raises(ValueError, match="go together"):
        bench_agent.SlashObserveAgent(logs_dir=tmp_path, model_name="replay/replay", spend_cap_usd=5)
    with pytest.raises(ValueError, match="go together"):
        bench_agent.SlashObserveAgent(logs_dir=tmp_path, model_name="replay/replay", ledger_path="ledger.jsonl")


def test_spend_cap_and_ledger_build_a_ledger(tmp_path):
    instance = bench_agent.SlashObserveAgent(
        logs_dir=tmp_path, model_name="replay/replay", spend_cap_usd=5, ledger_path=str(tmp_path / "l.jsonl")
    )
    assert instance._ledger.cap_usd == 5


def test_unknown_option_is_refused(tmp_path):
    with pytest.raises(ValueError, match="Unknown option"):
        bench_agent.SlashObserveAgent(logs_dir=tmp_path, model_name="replay/replay", bogus="1")


def test_model_is_split_into_provider_and_model():
    assert bench_agent.split_model("anthropic/claude-haiku-5-5") == ("anthropic", "claude-haiku-5-5")
    assert bench_agent.split_model("replay/replay") == ("replay", None)


@pytest.mark.parametrize("model_name", [None, "", "claude-haiku-5-5"])
def test_model_without_a_provider_is_refused(model_name):
    with pytest.raises(ValueError, match="<provider>/<model>"):
        bench_agent.split_model(model_name)


def test_environment_lines_become_a_dict():
    assert bench_agent.read_environment("A=1\nB=x=y\nnoise\n") == {"A": "1", "B": "x=y"}


def test_saving_a_run_writes_trajectory_raw_run_and_context(tmp_path, finished_request):
    instance = bench_agent.SlashObserveAgent(logs_dir=tmp_path, model_name="anthropic/claude-haiku-5-5")
    context = AgentContext()
    instance._save(finished_request, "Why did checkout fail?", context)
    trajectory = json.loads((tmp_path / "trajectory.json").read_text())
    Trajectory.model_validate(trajectory)
    assert trajectory["steps"][-1]["message"].startswith("The 5xx share is 2.1%")
    assert json.loads((tmp_path / "run.json").read_text())["id"] == finished_request["id"]
    assert (context.n_input_tokens, context.n_cache_tokens, context.n_output_tokens) == (1350, 300, 200)
    assert context.cost_usd == 0.0123


def test_llm_key_is_read_from_the_file_named_by_the_environment(tmp_path, monkeypatch):
    key_file = tmp_path / "key"
    key_file.write_text("sk-test\n")
    monkeypatch.setenv(bench_agent.LLM_KEY_FILE_ENV, str(key_file))
    instance = bench_agent.SlashObserveAgent(logs_dir=tmp_path, model_name="anthropic/claude-haiku-5-5")
    assert instance._llm_key("anthropic") == "sk-test"
    assert instance._llm_key("replay") is None


def test_missing_key_file_variable_is_an_error(tmp_path, monkeypatch):
    monkeypatch.delenv(bench_agent.LLM_KEY_FILE_ENV, raising=False)
    instance = bench_agent.SlashObserveAgent(logs_dir=tmp_path, model_name="anthropic/claude-haiku-5-5")
    with pytest.raises(ValueError, match=bench_agent.LLM_KEY_FILE_ENV):
        instance._llm_key("anthropic")


def stub_environment(tmp_path, task_id, category):
    task_dir = tmp_path / "tasks" / task_id
    (task_dir / "environment").mkdir(parents=True)
    (task_dir / "task.toml").write_text(f'[metadata]\ncategory = "{category}"\n')
    return SimpleNamespace(environment_name=task_id, environment_dir=task_dir / "environment")


def test_setup_refuses_a_grafana_state_task_and_records_why(tmp_path):
    environment = stub_environment(tmp_path, "list-datasources", "grafana_api")
    instance = bench_agent.SlashObserveAgent(logs_dir=tmp_path / "agent", model_name="replay/replay")
    with pytest.raises(bench_agent.TaskSkipped, match="list-datasources is skipped"):
        asyncio.run(instance.setup(environment))
    assert (tmp_path / "agent" / "skipped.txt").read_text().startswith("skipped: ")


def test_setup_refuses_an_unknown_category_before_touching_docker(tmp_path):
    environment = stub_environment(tmp_path, "new-task", "dashboarding")
    instance = bench_agent.SlashObserveAgent(logs_dir=tmp_path / "agent", model_name="replay/replay")
    with pytest.raises(o11y.UnsupportedTask):
        asyncio.run(instance.setup(environment))


def test_setup_refuses_to_start_a_trial_once_the_cap_is_reached(tmp_path):
    ledger_path = tmp_path / "ledger.jsonl"
    SpendLedger(ledger_path, 1.0).record("earlier", 1.0)
    environment = stub_environment(tmp_path, "promql-error-rate", "prometheus_query")
    instance = bench_agent.SlashObserveAgent(
        logs_dir=tmp_path / "agent", model_name="replay/replay", spend_cap_usd=1, ledger_path=str(ledger_path)
    )
    with pytest.raises(SpendCapReached):
        asyncio.run(instance.setup(environment))


class FakeClient:
    def __init__(self, replies):
        self.replies = list(replies)
        self.canceled = False

    def create_observation(self, title):
        return "obs-1"

    def start_run(self, body):
        return "run-1"

    def get_run(self, run_id):
        reply = self.replies.pop(0)
        if isinstance(reply, Exception):
            raise reply
        return reply

    def cancel_run(self, run_id):
        self.canceled = True


def run_state(base, state, cost):
    return {**base, "state": state, "usage": {**base["usage"], "cost_usd": cost}}


def answer_with(tmp_path, monkeypatch, client, deadline):
    monkeypatch.setattr(bench_agent, "Client", lambda base_url, key: client)
    ledger_path = tmp_path / "ledger.jsonl"
    instance = bench_agent.SlashObserveAgent(
        logs_dir=tmp_path / "trial" / "agent",
        model_name="replay/replay",
        spend_cap_usd=10,
        ledger_path=str(ledger_path),
    )
    instance._category = "prometheus_query"
    stack = SimpleNamespace(base_url="http://127.0.0.1:1", key="sok_test")
    run = instance._answer(stack, "How many errors?", "2026-11-20T10:00:00Z", "promql-error-rate", AgentContext(), deadline)
    return instance, ledger_path, run


def test_spend_is_recorded_when_polling_fails_after_the_run_spent(tmp_path, monkeypatch, finished_request):
    client = FakeClient([run_state(finished_request, "running", 0.25), bench_agent.ApiError(502, "bad gateway")])
    monkeypatch.setattr(bench_agent.time, "monotonic", lambda: 0.0)
    monkeypatch.setattr("slashobserve_bench.api.time.sleep", lambda seconds: None)
    _, ledger_path, run = answer_with(tmp_path, monkeypatch, client, deadline=100.0)
    with pytest.raises(bench_agent.ApiError):
        asyncio.run(run)
    assert SpendLedger(ledger_path, 10).total() == 0.25


def test_a_timed_out_run_is_canceled_saved_and_charged_its_final_cost(tmp_path, monkeypatch, finished_request):
    client = FakeClient([run_state(finished_request, "running", 0.25), run_state(finished_request, "canceled", 0.4)])
    instance, ledger_path, run = answer_with(tmp_path, monkeypatch, client, deadline=0.0)
    with pytest.raises(bench_agent.RunTimeout):
        asyncio.run(run)
    assert client.canceled
    assert json.loads((instance.logs_dir / "run.json").read_text())["state"] == "canceled"
    assert SpendLedger(ledger_path, 10).total() == 0.4
