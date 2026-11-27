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
    fail = None

    def __init__(self, config, datasources, logs_dir, llm_key, extra_files=None):
        FakeStack.started = SimpleNamespace(config=config, datasources=datasources, extra_files=extra_files)
        self.base_url = "http://stack"
        self.key = "sok_test"

    async def start(self):
        if isinstance(FakeStack.fail, Exception):
            raise FakeStack.fail
        if FakeStack.fail:
            raise RuntimeError(FakeStack.fail)

    def stop(self):
        FakeStack.stopped += 1


class FakeCodeServer:
    started = None
    stopped = 0

    def __init__(self, image, network, code_dir, logs_dir):
        FakeCodeServer.started = SimpleNamespace(image=image, network=network, code_dir=code_dir)

    def start(self):
        pass

    def stop(self):
        FakeCodeServer.stopped += 1


APPROVED = [{"name": name, "state": "approved"} for name in orca.CODE_TOOLS]


class FakeClient:
    finding = FINDING
    body = None
    pending = False
    tools = APPROVED
    collections = []

    def __init__(self, base_url, key):
        pass

    def mcp_tools(self, uid, collection="mcp-sources"):
        FakeClient.collections.append((uid, collection))
        return self.tools

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
    monkeypatch.setattr(orca_agent, "CodeServer", FakeCodeServer)
    monkeypatch.setattr(orca_agent, "copy_code_out", lambda container, target: target)
    FakeClient.finding = FINDING
    FakeClient.pending = False
    FakeClient.tools = APPROVED
    FakeClient.collections = []
    FakeStack.stopped = 0
    FakeStack.fail = None
    FakeCodeServer.started = None
    FakeCodeServer.stopped = 0
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


def test_loop_settings_come_from_agent_kwargs(tmp_path):
    agent = orca_agent.OrcaAgent(logs_dir=tmp_path, model_name="replay/replay", investigation_loop="true", finding_grounding="report", investigation_brief="off")
    assert agent.options.investigation_loop is True
    assert agent.options.investigation_brief == "off"
    assert agent.options.finding_grounding == "report"


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
    assert "code context: the code server slashobserve-codeserver:local" in (logs / "notes.txt").read_text()


def test_the_loop_settings_are_always_sent_and_noted(setup):
    agent, environment, logs = setup
    run(agent, environment)
    config = FakeStack.started.config
    assert (config.investigation_calls, config.investigation_loop, config.investigation_calls_max, config.finding_grounding, config.investigation_brief) == (None, True, 40, "enforce", "on")
    assert "investigation loop: true, calls max 40, calls server default" in (logs / "notes.txt").read_text()


def test_by_default_the_span_index_answers_the_trace_tools(setup):
    agent, environment, logs = setup
    run(agent, environment)
    assert "      spans: jaeger\n" in FakeStack.started.datasources
    assert "fields:" not in FakeStack.started.datasources
    assert "trace_slow_path" in FakeClient.body["prompt"]
    assert "spans: jaeger, so the trace tools answer on it" in (logs / "notes.txt").read_text()


def test_an_older_image_gets_plain_span_documents(setup):
    agent, environment, logs = setup
    agent.options = orca.Options(span_index=False)
    run(agent, environment)
    assert "spans:" not in FakeStack.started.datasources
    assert "traceId: traceID" in FakeStack.started.datasources
    assert "trace_slow_path" not in FakeClient.body["prompt"]
    assert "plain documents" in (logs / "notes.txt").read_text()


def test_a_server_that_does_not_start_names_the_span_index_option(setup):
    agent, environment, _ = setup
    FakeStack.fail = orca_agent.StackError("the server did not become ready within 120 seconds")
    with pytest.raises(orca_agent.StackError, match="pass --ak span_index=false"):
        run(agent, environment)
    agent.options = orca.Options(span_index=False)
    with pytest.raises(orca_agent.StackError) as raised:
        run(agent, environment)
    assert str(raised.value) == "the server did not become ready within 120 seconds"


def test_by_default_the_code_server_serves_the_task_source_to_the_run(setup):
    agent, environment, _ = setup
    run(agent, environment)
    assert FakeCodeServer.started.network == "otel-0123456789ab_default"
    assert FakeCodeServer.started.image == orca.CODESERVER_IMAGE
    assert list(FakeStack.started.extra_files) == ["mcp-sources.yaml"]
    assert FakeStack.started.extra_files["mcp-sources.yaml"].startswith("mcpSources:\n")
    assert ("SLASHOBSERVE_MCP_SOURCES_FILE", "/etc/slashobserve/mcp-sources.yaml") in FakeStack.started.config.extra_env
    assert ("SLASHOBSERVE_OUTBOUND_DOMAINS", orca.CODE_HOST) in FakeStack.started.config.extra_env
    assert FakeClient.collections == [(orca.CODE_SOURCE, "mcp-sources")]
    assert f"MCP source `{orca.CODE_SOURCE}`" in FakeClient.body["prompt"]
    assert FakeCodeServer.stopped == 1


def test_the_old_source_names_are_one_option_away(setup):
    agent, environment, logs = setup
    agent.options = orca.Options(source_names="context")
    run(agent, environment)
    assert FakeStack.started.extra_files["context-sources.yaml"].startswith("contextSources:\n")
    assert ("SLASHOBSERVE_CONTEXT_SOURCES_FILE", "/etc/slashobserve/context-sources.yaml") in FakeStack.started.config.extra_env
    assert FakeClient.collections == [(orca.CODE_SOURCE, "context-sources")]
    assert "SLASHOBSERVE_CONTEXT_SOURCES_FILE with the key contextSources" in (logs / "notes.txt").read_text()


def test_code_off_runs_without_a_code_server_or_note(setup):
    agent, environment, logs = setup
    agent.options = orca.Options(code_context="off")
    run(agent, environment)
    assert FakeCodeServer.started is None
    assert FakeStack.started.extra_files is None
    assert FakeStack.started.config.extra_env == ()
    assert "feature flag" not in FakeClient.body["prompt"]
    assert "code context: off" in (logs / "notes.txt").read_text()


def test_the_code_server_is_removed_when_the_server_fails_to_start(setup):
    agent, environment, _ = setup
    FakeStack.fail = "no server"
    with pytest.raises(RuntimeError, match="no server"):
        run(agent, environment)
    assert FakeCodeServer.stopped == 1
    assert FakeStack.stopped == 1


def test_unapproved_code_tools_fail_the_trial(setup, monkeypatch):
    agent, environment, _ = setup
    FakeClient.tools = [{"name": "list_dir", "state": "offered"}]
    monkeypatch.setattr(orca_agent, "CODE_TOOLS_WAIT_SEC", 0)
    with pytest.raises(orca_agent.CodeContextUnavailable, match="list_dir is offered, read_file is missing, grep is missing"):
        run(agent, environment)
    assert FakeCodeServer.stopped == 1


def test_a_server_without_the_source_names_the_other_naming():
    class Missing:
        def mcp_tools(self, uid, collection):
            raise orca_agent.ApiError(404, "not found")

    with pytest.raises(orca_agent.CodeContextUnavailable, match="pass --ak source_names=context"):
        orca_agent.wait_for_code_tools(Missing(), orca.SOURCE_NAMES["mcp"], 0)


def test_setup_fails_when_the_code_server_image_is_absent(tmp_path, monkeypatch):
    monkeypatch.setattr(orca_agent, "image_present", lambda image: False)
    agent = orca_agent.OrcaAgent(logs_dir=tmp_path, model_name="replay/replay")
    with pytest.raises(orca_agent.CodeContextUnavailable, match="docker build -f tools/codeserver/Dockerfile -t slashobserve-codeserver:local"):
        asyncio.run(agent.setup(None))


def test_setup_with_code_off_does_not_need_the_code_server_image(tmp_path, monkeypatch):
    monkeypatch.setattr(orca_agent, "image_present", lambda image: False)
    pulled = []
    monkeypatch.setattr(orca_agent, "ensure_image", pulled.append)
    agent = orca_agent.OrcaAgent(logs_dir=tmp_path, model_name="replay/replay", code_context="off")
    asyncio.run(agent.setup(None))
    assert orca.CODESERVER_IMAGE not in pulled


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
