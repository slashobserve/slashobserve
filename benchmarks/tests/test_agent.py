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
        investigation_brief="off",
    )
    assert instance._options.investigation_loop is True
    assert instance._options.investigation_brief == "off"
    assert instance._options.investigation_calls_max == 40
    assert instance._options.finding_grounding == "report"
    unset = bench_agent.SlashObserveAgent(logs_dir=tmp_path, model_name="replay/replay")
    assert unset._options.investigation_loop is None


def test_finding_grounding_is_enforced_unless_given(tmp_path):
    unset = bench_agent.SlashObserveAgent(logs_dir=tmp_path, model_name="replay/replay")._options
    assert unset.finding_grounding is None
    assert unset.effective_grounding == "enforce"
    assert unset.grounding_note().startswith("finding grounding: enforce (the adapter's default)")
    given = bench_agent.SlashObserveAgent(logs_dir=tmp_path, model_name="replay/replay", finding_grounding="off")._options
    assert given.effective_grounding == "off"
    assert given.grounding_note().startswith("finding grounding: off (as given)")


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


def test_only_the_trajectory_is_a_json_file_directly_under_the_agent_folder(tmp_path, finished_request):
    instance = bench_agent.SlashObserveAgent(logs_dir=tmp_path, model_name="anthropic/claude-haiku-5-5")
    instance._save(finished_request, "Why did checkout fail?", AgentContext())
    instance._detail("grafana-imports.json").write_text("[]\n")
    assert [path.name for path in tmp_path.glob("*.json")] == ["trajectory.json"]


def test_saving_a_run_writes_trajectory_raw_run_and_context(tmp_path, finished_request):
    instance = bench_agent.SlashObserveAgent(logs_dir=tmp_path, model_name="anthropic/claude-haiku-5-5")
    context = AgentContext()
    instance._save(finished_request, "Why did checkout fail?", context)
    trajectory = json.loads((tmp_path / "trajectory.json").read_text())
    Trajectory.model_validate(trajectory)
    assert trajectory["steps"][-1]["message"].startswith("The 5xx share is 2.1%")
    assert json.loads((tmp_path / "detail" / "run.json").read_text())["id"] == finished_request["id"]
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


def test_setup_refuses_a_skipped_task_and_records_why(tmp_path, monkeypatch):
    monkeypatch.setitem(o11y.SKIPPED, "some-task", "needs what this agent lacks")
    environment = stub_environment(tmp_path, "some-task", "prometheus_query")
    instance = bench_agent.SlashObserveAgent(logs_dir=tmp_path / "agent", model_name="replay/replay")
    with pytest.raises(bench_agent.TaskSkipped, match="some-task is skipped: needs what this agent lacks"):
        asyncio.run(instance.setup(environment))
    assert (tmp_path / "agent" / "skipped.txt").read_text().startswith("skipped: ")


def test_setup_refuses_an_unknown_category_before_touching_docker(tmp_path):
    environment = stub_environment(tmp_path, "new-task", "alerting")
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
    assert json.loads((instance.logs_dir / "detail" / "run.json").read_text())["state"] == "canceled"
    assert SpendLedger(ledger_path, 10).total() == 0.4


class ImportClient:
    def __init__(self):
        self.sent = []

    def import_grafana(self, export):
        self.sent.append(export)
        if export["dashboard"]["uid"] == "broken":
            raise bench_agent.ApiError(422, '{"code": "datasources_unresolved"}')
        return {"dashboard": {"id": f"sob-{export['dashboard']['uid']}"}, "version": {"version": 1, "spec": {}}}


def test_every_grafana_dashboard_is_imported_and_a_failed_one_is_recorded(tmp_path):
    instance = bench_agent.SlashObserveAgent(logs_dir=tmp_path / "agent", model_name="replay/replay")
    state = bench_agent.grafana.GrafanaState(
        datasources=[],
        dashboards=[{"dashboard": {"uid": "svc", "title": "Service"}, "meta": {}}, {"dashboard": {"uid": "broken", "title": "Broken"}, "meta": {}}],
    )
    client = ImportClient()
    assert instance._import_dashboards(client, state) == {"svc": "sob-svc"}
    assert client.sent == state.dashboards
    records = json.loads((tmp_path / "agent" / "detail" / "grafana-imports.json").read_text())
    assert records[0] == {"grafana_uid": "svc", "title": "Service", "dashboard_id": "sob-svc", "version": 1}
    assert "datasources_unresolved" in records[1]["error"]


def test_a_grafana_url_option_talks_to_grafana_from_the_host(tmp_path):
    instance = bench_agent.SlashObserveAgent(logs_dir=tmp_path, model_name="replay/replay", grafana_url="http://127.0.0.1:3000")
    client = instance._grafana_client({}, None)
    assert client.base_url == "http://127.0.0.1:3000"


def test_a_grafana_task_without_grafana_url_is_unsupported(tmp_path):
    instance = bench_agent.SlashObserveAgent(logs_dir=tmp_path, model_name="replay/replay")
    with pytest.raises(o11y.UnsupportedTask, match="GRAFANA_URL"):
        instance._grafana_client({}, None)


SPEC_V1 = {
    "specVersion": "0",
    "title": "Cache review",
    "time": {"from": "now-1h", "to": "now"},
    "panels": [{"id": "p1", "type": "stat", "title": "Hits", "gridPos": {"x": 0, "y": 0, "w": 6, "h": 4}, "datasource": "prometheus", "queries": [{"query": "hits"}]}],
}
SPEC_V2 = {**SPEC_V1, "panels": [*SPEC_V1["panels"], {"id": "lag", "type": "stat", "title": "Lag", "gridPos": {"x": 6, "y": 0, "w": 6, "h": 4}, "datasource": "prometheus", "queries": [{"query": "lag"}]}]}
GRAFANA_CACHE = {
    "dashboard": {"id": 3, "uid": "cache", "title": "Cache review", "panels": [{"id": 1, "type": "stat", "title": "Hits", "gridPos": {"h": 4, "w": 6, "x": 0, "y": 0}, "targets": [{"refId": "A", "expr": "hits"}]}]},
    "meta": {"folderUid": "ops"},
}


class DashboardClient:
    def __init__(self, base):
        self.base = base
        self.bodies = []
        self.replies = []

    def start_run(self, body):
        self.bodies.append(body)
        if body["kind"] == "ask":
            self.replies = [{**self.base, "id": "run-build", "kind": "build", "state": "succeeded", "dashboard_id": None, "result_version": None, "finding": {"markdown": "That dashboard exists.", "ask_on": {"dashboard_id": "sob-cache", "title": "Cache review"}}}]
        else:
            self.replies = [{**self.base, "id": "run-revise", "kind": "revise", "state": "succeeded", "dashboard_id": "sob-cache", "base_version": 1, "result_version": 2, "finding": None}]
        return "run"

    def get_run(self, run_id):
        return self.replies[0]

    def dashboard_version(self, dashboard_id, version):
        assert dashboard_id == "sob-cache"
        return {"spec": SPEC_V1 if version == 1 else SPEC_V2}


class SavingGrafana:
    def __init__(self):
        self.saved = []

    async def save_dashboard(self, dashboard, folder_uid, message):
        self.saved.append((dashboard, folder_uid))
        return {"uid": dashboard["uid"]}

    async def dashboard(self, uid):
        return {"dashboard": self.saved[-1][0]}

    async def query(self, body):
        return 200, {"results": {query["refId"]: {"frames": []} for query in body["queries"]}}


def test_a_dashboard_task_revises_the_dashboard_the_build_points_at_and_writes_it_to_grafana(tmp_path, monkeypatch, finished_request):
    calls = []

    def fake_call(provider, model, key, system, messages, timeout):
        calls.append((provider, model, key))
        text = json.dumps({"panels": [{"spec_id": "lag", "panel": {"type": "stat", "title": "Lag", "gridPos": {"w": 6, "h": 4}, "targets": [{"refId": "A", "expr": "lag", "datasource": {"type": "prometheus", "uid": "graf-prom"}}]}}]})
        usage = bench_agent.atif.Usage(input_tokens=1000, output_tokens=100, model_calls=1)
        return bench_agent.model_call.ModelReply(text=text, usage=usage, request={"model": model, "messages": messages}, response={"content": [{"type": "text", "text": text}]})

    monkeypatch.setattr(bench_agent.model_call, "call_model", fake_call)
    monkeypatch.setattr("slashobserve_bench.api.time.sleep", lambda seconds: None)
    ledger_path = tmp_path / "ledger.jsonl"
    instance = bench_agent.SlashObserveAgent(
        logs_dir=tmp_path / "trial" / "agent", model_name="anthropic/claude-haiku-5-5", spend_cap_usd=10, ledger_path=str(ledger_path)
    )
    instance._category = o11y.DASHBOARDING
    base = {key: value for key, value in finished_request.items() if key != "finding"}
    client = DashboardClient(base)
    grafana_client = SavingGrafana()
    task = bench_agent.DashboardTask(
        instruction="Add a lag panel next to the hits panel.",
        as_of="2026-11-20T10:00:00Z",
        state=bench_agent.grafana.GrafanaState(datasources=[{"uid": "graf-prom", "name": "Prometheus", "type": "prometheus"}], dashboards=[GRAFANA_CACHE]),
        imported={"cache": "sob-cache"},
        grafana=grafana_client,
        provider="anthropic",
        model="claude-haiku-5-5",
        key="sk-never-written",
    )
    context = AgentContext()
    asyncio.run(instance._dashboard_task(client, task, context, deadline=bench_agent.time.monotonic() + 600))

    assert [body["kind"] for body in client.bodies] == ["ask", "revise"]
    assert client.bodies[1]["dashboard_id"] == "sob-cache"
    assert calls == [("anthropic", "claude-haiku-5-5", "sk-never-written")]
    saved, folder = grafana_client.saved[0]
    assert folder == "ops"
    assert saved["panels"][0] == GRAFANA_CACHE["dashboard"]["panels"][0]
    assert saved["panels"][1]["title"] == "Lag" and saved["panels"][1]["id"] == 2
    logs = instance.logs_dir / "detail"
    outcome = json.loads((logs / "grafana-write.json").read_text())
    assert outcome["saved_uid"] == "cache" and outcome["model_calls"] == 1
    assert outcome["usage"]["cost_usd"] == pytest.approx(0.00015)
    assert (logs / "grafana-call-1-request.json").exists() and (logs / "grafana-call-1-response.json").exists()
    assert (logs / "run-build.json").exists()
    for path in logs.iterdir():
        assert "sk-never-written" not in path.read_text()
    assert "sk-never-written" not in repr(task)
    assert context.n_input_tokens == 2 * 1350 + 1000
    assert context.n_cache_tokens == 2 * 300
    assert context.n_output_tokens == 2 * 200 + 100
    assert context.cost_usd == pytest.approx(2 * 0.0123 + 0.00015)
    trajectory = json.loads((instance.logs_dir / "trajectory.json").read_text())
    Trajectory.model_validate(trajectory)
    assert 'Saved the Grafana dashboard' in trajectory["steps"][-1]["message"] and '"Lag"' in trajectory["steps"][-1]["message"]
    assert trajectory["final_metrics"]["total_cost_usd"] == pytest.approx(context.cost_usd)
    assert SpendLedger(ledger_path, 10).total() == pytest.approx(2 * 0.0123 + 0.00015)


def test_the_replay_model_writes_nothing_to_grafana(tmp_path, monkeypatch, finished_request):
    monkeypatch.setattr("slashobserve_bench.api.time.sleep", lambda seconds: None)
    instance = bench_agent.SlashObserveAgent(logs_dir=tmp_path / "agent", model_name="replay/replay")
    instance._category = o11y.DASHBOARDING
    client = DashboardClient({key: value for key, value in finished_request.items() if key != "finding"})
    grafana_client = SavingGrafana()
    task = bench_agent.DashboardTask(
        instruction="Change the Cache review dashboard.",
        as_of="2026-11-20T10:00:00Z",
        state=bench_agent.grafana.GrafanaState(datasources=[], dashboards=[GRAFANA_CACHE]),
        imported={"cache": "sob-cache"},
        grafana=grafana_client,
        provider="replay",
        model=None,
        key=None,
    )
    asyncio.run(instance._dashboard_task(client, task, AgentContext(), deadline=bench_agent.time.monotonic() + 600))
    assert [body["kind"] for body in client.bodies] == ["revise"]
    assert grafana_client.saved == []
    assert "replay" in json.loads((tmp_path / "agent" / "detail" / "grafana-write.json").read_text())["skipped"]


class VersionClient:
    def dashboard_version(self, dashboard_id, version):
        return {"spec": {"id": dashboard_id, "version": version}}


def test_specs_compare_a_build_with_nothing_and_a_revise_with_its_base():
    task = SimpleNamespace(imported={"cache": "sob-cache"})
    specs = bench_agent.SlashObserveAgent._specs
    assert specs(VersionClient(), {"dashboard_id": "new", "result_version": 1}, task, None) == (None, {"id": "new", "version": 1})
    assert specs(VersionClient(), {"dashboard_id": None, "result_version": None}, task, None) is None
    revised = specs(VersionClient(), {"base_version": 1, "result_version": 3}, task, "cache")
    assert revised == ({"id": "sob-cache", "version": 1}, {"id": "sob-cache", "version": 3})
    unchanged = specs(VersionClient(), {"base_version": 1, "result_version": None}, task, "cache")
    assert unchanged[0] == unchanged[1]
