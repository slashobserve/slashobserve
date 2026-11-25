import asyncio
import json
from types import SimpleNamespace

import pytest
from harbor.models.agent.context import AgentContext

from slashobserve_bench import orca, orca_agent

PORTS = "export PROMETHEUS_URL=http://prometheus:9090\nexport OPENSEARCH_URL=http://opensearch:9200\n"
TASK_TOML = '[metadata]\ncurrent = "2026-04-18T14:05:00-04:00"\n'
FINDING = {
    "markdown": "Ads failed.",
    "root_cause": {"component": "adFailure", "kind": "config_or_flag", "summary": "The flag broke ads"},
    "onset": "2026-04-18T09:44:00Z",
    "impact": "14% failed",
    "ruled_out": [],
    "next_steps": ["Switch it off"],
    "no_incident": False,
}


class FakeEnvironment:
    def __init__(self, task_dir):
        self.environment_dir = task_dir / "environment"
        self.session_id = "Trial__ABC.1"
        self.commands = []
        self.uploads = {}
        self.failing = {}
        self.hostname = "abc123\n"

    async def exec(self, command, **kwargs):
        self.commands.append(command)
        for marker, result in self.failing.items():
            if marker in command:
                return SimpleNamespace(stdout="", stderr=result, return_code=1)
        if "env-ports" in command:
            return SimpleNamespace(stdout=PORTS, stderr="", return_code=0)
        if command == "cat /etc/hostname":
            return SimpleNamespace(stdout=self.hostname, stderr="", return_code=0)
        return SimpleNamespace(stdout="", stderr="", return_code=0)

    async def upload_file(self, source, target):
        self.uploads[target] = open(source, encoding="utf-8").read()


class FakeStack:
    started = None
    stopped = 0

    def __init__(self, config, datasources, logs_dir, llm_key):
        FakeStack.started = SimpleNamespace(config=config, datasources=datasources)
        self.base_url = "http://stack"
        self.key = "sok_test"

    async def start(self):
        pass

    def stop(self):
        FakeStack.stopped += 1


class FakeClient:
    finding = FINDING
    body = None
    pending = False

    def __init__(self, base_url, key):
        pass

    def start_run(self, body):
        FakeClient.body = body
        return "run-1"

    def cancel_run(self, run_id):
        pass

    def get_run(self, run_id):
        if self.pending:
            return {**self.snapshot(run_id), "state": "running", "finding": None}
        return self.snapshot(run_id)

    def snapshot(self, run_id):
        return {
            "id": run_id,
            "state": "succeeded" if self.finding else "failed",
            "failure_code": None if self.finding else "budget_exhausted",
            "created_at": "2026-11-25T10:00:00Z",
            "finished_at": "2026-11-25T10:01:00Z",
            "steps": [],
            "usage": {"cost_usd": 0.5},
            "finding": self.finding,
        }


@pytest.fixture
def setup(tmp_path, monkeypatch):
    task_dir = tmp_path / "task"
    (task_dir / "environment").mkdir(parents=True)
    (task_dir / "task.toml").write_text(TASK_TOML)
    monkeypatch.setattr(orca_agent, "Stack", FakeStack)
    monkeypatch.setattr(orca_agent, "Client", FakeClient)
    monkeypatch.setattr(orca_agent, "host_networks", lambda container: ["bridge", "otel-0123456789ab_default"])
    FakeClient.finding = FINDING
    FakeClient.pending = False
    FakeStack.stopped = 0
    logs = tmp_path / "logs" / "trial" / "agent"
    agent = orca_agent.OrcaAgent(logs_dir=logs, model_name="replay/replay")
    return agent, FakeEnvironment(task_dir), logs


def run(agent, environment):
    context = AgentContext()
    asyncio.run(agent.run("Task text. The current time is Apr 18, 2026 at 14:05 ET.", environment, context))
    return context


def test_options_come_from_agent_kwargs(tmp_path):
    agent = orca_agent.OrcaAgent(logs_dir=tmp_path, model_name="replay/replay", investigation_calls="30", extra_env={"A": "b"})
    assert agent.options.investigation_calls == 30
    assert agent.extra_env == {"A": "b"}


def test_unknown_option_is_refused(tmp_path):
    with pytest.raises(ValueError):
        orca_agent.OrcaAgent(logs_dir=tmp_path, model_name="replay/replay", bogus="1")


def test_model_must_name_a_provider(tmp_path):
    with pytest.raises(ValueError, match="<provider>/<model>"):
        orca_agent.OrcaAgent(logs_dir=tmp_path, model_name="claude")._provider_and_model()


def test_a_real_provider_needs_a_key_file(tmp_path, monkeypatch):
    monkeypatch.delenv(orca_agent.LLM_KEY_FILE_ENV, raising=False)
    agent = orca_agent.OrcaAgent(logs_dir=tmp_path, model_name="anthropic/claude-sonnet-5-5")
    with pytest.raises(ValueError, match=orca_agent.LLM_KEY_FILE_ENV):
        agent._llm_key("anthropic")


def test_run_starts_an_investigation_beside_the_trial_and_writes_the_report(setup):
    agent, environment, logs = setup
    context = run(agent, environment)
    assert FakeStack.started.config.network == "otel-0123456789ab_default"
    assert 'url: "http://opensearch:9200"' in FakeStack.started.datasources
    assert FakeClient.body["kind"] == "investigate"
    assert FakeClient.body["as_of"] == "2026-04-18T18:05:00Z"
    report = environment.uploads[orca_agent.REPORT_STAGING]
    assert "2026-04-18 09:44 UTC - adFailure (config or flag): The flag broke ads" in report
    assert f"cp {orca_agent.REPORT_STAGING} {orca.REPORT_PATH} && test -s {orca.REPORT_PATH}" in environment.commands
    assert (logs / "report.md").read_text() == report
    assert json.loads((logs / "run.json").read_text())["id"] == "run-1"
    assert (logs / "trajectory.json").exists()
    assert context.cost_usd == 0.5
    assert "code context: skipped" in (logs / "notes.txt").read_text()


def test_no_incident_leaves_an_empty_report(setup):
    agent, environment, _ = setup
    FakeClient.finding = {"markdown": "All quiet.", "no_incident": True}
    run(agent, environment)
    assert environment.uploads[orca_agent.REPORT_STAGING] == ""


def test_a_run_without_a_finding_fails_the_trial_instead_of_passing_a_control(setup):
    agent, environment, logs = setup
    FakeClient.finding = None
    with pytest.raises(orca_agent.RunEndedWithoutFinding, match="budget_exhausted"):
        run(agent, environment)
    assert orca_agent.REPORT_STAGING not in environment.uploads
    assert (logs / "run.json").exists()


def test_code_context_hook_gets_the_client_network_and_code(setup, monkeypatch, tmp_path):
    agent, environment, logs = setup
    calls = []
    monkeypatch.setattr(orca, "load_code_context", lambda path: lambda *args: calls.append(args))
    monkeypatch.setattr(orca_agent, "copy_code_out", lambda container, target: target / "opentelemetry-demo")
    agent.options = orca.Options(code_context="x:y")
    run(agent, environment)
    assert calls[0][1] == "otel-0123456789ab_default"
    assert calls[0][2].name == "opentelemetry-demo"
    assert "registered through x:y" in (logs / "notes.txt").read_text()


def test_a_failed_copy_to_the_report_path_fails_the_trial_with_the_error(setup):
    agent, environment, _ = setup
    environment.failing["cp /tmp"] = "cp: Permission denied"
    with pytest.raises(orca_agent.ReportNotWritten, match="Permission denied"):
        run(agent, environment)


def test_the_report_is_checked_to_be_non_empty_unless_the_answer_is_no_incident(setup):
    agent, environment, _ = setup
    run(agent, environment)
    assert environment.commands[-1].endswith("&& test -s /app/report.md")
    FakeClient.finding = {"markdown": "Quiet.", "no_incident": True}
    run(agent, environment)
    assert environment.commands[-1].endswith("&& test -f /app/report.md")


def test_a_timed_out_run_still_saves_its_trajectory_and_cost(setup):
    agent, environment, logs = setup
    agent.options = orca.Options(wall_timeout_sec=60)
    FakeClient.pending = True
    monkey = pytest.MonkeyPatch()
    monkey.setattr(orca_agent, "wait_for_terminal", lambda poll, timeout: poll() and (_ for _ in ()).throw(orca_agent.RunTimeout("slow")))
    context = AgentContext()
    try:
        with pytest.raises(orca_agent.RunTimeout):
            asyncio.run(agent.run("Task. The current time is Apr 18, 2026 at 14:05 ET.", environment, context))
    finally:
        monkey.undo()
    assert (logs / "trajectory.json").exists()
    assert context.cost_usd == 0.5
    assert FakeStack.stopped == 1


def test_a_failing_ready_command_names_its_error(setup):
    agent, environment, _ = setup
    environment.failing["env-ready"] = "timed out"
    with pytest.raises(orca.UnsupportedTask, match="timed out"):
        run(agent, environment)


def test_an_empty_hostname_falls_back_to_the_compose_project(setup, monkeypatch):
    agent, environment, _ = setup
    environment.hostname = "\n"
    seen = {}
    monkeypatch.setattr(orca_agent, "container_by_project", lambda project: seen.setdefault("project", project) and "def456")
    run(agent, environment)
    assert seen["project"] == "trial__abc-1"
    assert FakeStack.started.config.network == "otel-0123456789ab_default"


def test_a_hostname_that_docker_cannot_inspect_falls_back_too(setup, monkeypatch):
    agent, environment, _ = setup

    def inspect(container):
        if container == "abc123":
            raise orca.UnsupportedTask("no such container")
        return ["otel-0123456789ab_default"]

    monkeypatch.setattr(orca_agent, "host_networks", inspect)
    monkeypatch.setattr(orca_agent, "container_by_project", lambda project: "def456")
    run(agent, environment)
    assert FakeStack.started.config.network == "otel-0123456789ab_default"


def test_the_code_copy_lives_until_the_run_is_over(setup, monkeypatch):
    agent, environment, _ = setup
    seen = {}

    def hook(client, network, code_dir):
        seen["root"] = code_dir.parent

    def copy(container, target):
        (target / "opentelemetry-demo").mkdir()
        return target / "opentelemetry-demo"

    monkeypatch.setattr(orca, "load_code_context", lambda path: hook)
    monkeypatch.setattr(orca_agent, "copy_code_out", copy)
    original = orca_agent.OrcaAgent._investigate

    async def investigate(self, *args):
        seen["alive_during_run"] = seen["root"].exists()
        return await original(self, *args)

    monkeypatch.setattr(orca_agent.OrcaAgent, "_investigate", investigate)
    agent.options = orca.Options(code_context="x:y")
    run(agent, environment)
    assert seen["alive_during_run"]
    assert not seen["root"].exists()
