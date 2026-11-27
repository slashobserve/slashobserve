import shlex
from pathlib import Path

import pytest
import yaml

from slashobserve_bench import sregym

APP = {
    "app_name": "Hotel Reservation",
    "namespace": "hotel-reservation",
    "namespaces": ["hotel-reservation"],
    "descriptions": "A hotel booking app made of Go microservices.",
    "stages": ["diagnosis", "mitigation"],
}


def app(**changes):
    return sregym.App.from_get_app({**APP, **changes})


def test_get_app_is_read_with_its_description_and_stages():
    parsed = app()
    assert parsed == sregym.App(
        "Hotel Reservation", "hotel-reservation", ("hotel-reservation",), "A hotel booking app made of Go microservices.", ("diagnosis", "mitigation")
    )


def test_get_app_keeps_the_main_namespace_first_and_drops_repeats():
    parsed = app(namespaces=["kafka", "hotel-reservation", "kafka", ""])
    assert parsed.namespaces == ("hotel-reservation", "kafka")


def test_get_app_without_a_namespace_list_uses_the_namespace():
    data = {key: value for key, value in APP.items() if key != "namespaces"}
    assert sregym.App.from_get_app(data).namespaces == ("hotel-reservation",)


def test_get_app_reads_a_singular_description_and_none_at_all():
    data = {key: value for key, value in APP.items() if key != "descriptions"}
    assert sregym.App.from_get_app({**data, "description": "Shop"}).description == "Shop"
    assert sregym.App.from_get_app(data).description == ""
    assert sregym.App.from_get_app({**data, "stages": None}).stages == ()


@pytest.mark.parametrize("data", [[], "x", {"namespace": "ns"}, {"app_name": "a"}, {"app_name": " ", "namespace": "ns"}])
def test_get_app_without_app_or_namespace_is_an_error(data):
    with pytest.raises(sregym.ConductorError):
        sregym.App.from_get_app(data)


def test_status_reads_the_stage_and_the_remaining_time():
    assert sregym.Status.from_status({"stage": "diagnosis", "agent_remaining_seconds": 1700.5}) == sregym.Status("diagnosis", 1700.5)
    assert sregym.Status.from_status({"stage": "setup"}) == sregym.Status("setup", None)
    with pytest.raises(sregym.ConductorError):
        sregym.Status.from_status(["diagnosis"])


@pytest.mark.parametrize(
    ("env", "url"),
    [
        ({}, "http://127.0.0.1:8000"),
        ({"API_HOSTNAME": "0.0.0.0", "API_PORT": "8100"}, "http://127.0.0.1:8100"),
        ({"API_HOSTNAME": "conductor.local"}, "http://conductor.local:8000"),
    ],
)
def test_conductor_url_follows_what_sregym_exports(env, url):
    assert sregym.conductor_url(env) == url


def test_kubectl_server_is_reached_through_the_host_alias_on_sregyms_port():
    assert sregym.mcp_url({}) == "http://host.docker.internal:9954/kubectl/sse"
    assert sregym.mcp_url({"MCP_SERVER_PORT": "19954"}) == "http://host.docker.internal:19954/kubectl/sse"
    assert sregym.host_of("http://host.docker.internal:19954/kubectl/sse") == "host.docker.internal"


def test_submission_names_the_diagnosis_stage_only():
    assert sregym.submit_body("text") == {"solution": "text", "stage": "diagnosis"}


def test_datasources_file_names_prometheus_and_loki_and_no_traces():
    text = sregym.datasources_yaml("http://host.docker.internal:40001", "http://host.docker.internal:40002")
    parsed = yaml.safe_load(text)
    assert parsed == {
        "datasources": [
            {"uid": "prometheus", "type": "prometheus", "name": "Prometheus", "prometheus": {"url": "http://host.docker.internal:40001"}},
            {"uid": "loki", "type": "loki", "name": "Loki", "loki": {"url": "http://host.docker.internal:40002"}},
        ]
    }


def test_mcp_sources_file_approves_the_kubectl_tool_only():
    parsed = yaml.safe_load(sregym.mcp_sources_yaml("http://host.docker.internal:9954/kubectl/sse"))
    assert list(parsed) == ["mcpSources"]
    (source,) = parsed["mcpSources"]
    assert source["uid"] == "sregym-kubectl"
    assert source["url"] == "http://host.docker.internal:9954/kubectl/sse"
    assert source["allowPrivate"] is True
    assert source["approve"] == ["exec_kubectl_cmd_safely"]
    assert source["audience"] == "everyone"
    assert len(source["description"]) <= 300


def test_server_reads_the_sources_file_and_may_call_only_the_kubectl_host():
    settings = dict(sregym.server_settings("http://host.docker.internal:9954/kubectl/sse"))
    assert settings == {
        "SLASHOBSERVE_MCP_SOURCES_FILE": "/etc/slashobserve/mcp-sources.yaml",
        "SLASHOBSERVE_OUTBOUND_DOMAINS": "host.docker.internal",
    }


def test_question_names_the_app_namespace_and_description_then_the_playbook():
    text = sregym.question(app())
    assert text.startswith("Users of the application Hotel Reservation, which runs on Kubernetes in namespace hotel-reservation")
    assert "About the application: A hotel booking app made of Go microservices." in text
    assert text.endswith(sregym.playbook(True))
    assert "exec_kubectl_cmd_safely" in text
    for verb in ("get", "describe", "logs", "top", "events"):
        assert verb in text
    for check in ("Service selectors", "internalTrafficPolicy", "NetworkPolicies", "ResourceQuotas", "webhooks", "HPAs", "env vars", "resource limits"):
        assert check in text


def test_question_lists_every_namespace():
    text = sregym.question(app(namespaces=["hotel-reservation", "kafka"]))
    assert "in namespaces hotel-reservation, kafka, see it" in text


def test_question_without_kubectl_says_so_and_drops_the_cluster_steps():
    text = sregym.question(app(), kubectl=False)
    assert "cannot be read in this run" in text
    assert "exec_kubectl_cmd_safely" not in text
    assert "Service selectors" not in text


def test_question_without_a_description_has_no_about_line():
    assert "About the application" not in sregym.question(app(descriptions=""))


def test_question_clips_a_long_description_to_the_prompt_limit():
    text = sregym.question(app(descriptions="word " * 2000))
    assert len(text) == sregym.PROMPT_LIMIT
    assert "…\n\nHow to work:" in text
    assert text.endswith(sregym.playbook(True))


def test_default_question_fits_with_room_to_spare():
    assert len(sregym.question(app())) < sregym.PROMPT_LIMIT - 1500


def test_question_refuses_when_the_playbook_alone_does_not_fit():
    with pytest.raises(sregym.UnsupportedProblem):
        sregym.question(app(), limit=len(sregym.playbook(True)) + 50)
    with pytest.raises(sregym.UnsupportedProblem):
        sregym.question(app(descriptions=""), limit=100)


def test_investigation_runs_on_the_live_cluster_without_as_of():
    assert sregym.start_body("q") == {"kind": "investigate", "prompt": "q"}


@pytest.mark.parametrize(
    ("value", "text"),
    [
        ("hotel-reservation/Deployment/geo", "Deployment geo in namespace hotel-reservation"),
        ("Service/search", "Service search"),
        ("NetworkPolicy/deny-frontend", "NetworkPolicy deny-frontend"),
        ("frontend", "frontend"),
        ("astronomy-shop/frontend", "astronomy-shop/frontend"),
        ({"kind": "Secret", "name": "valkey", "namespace": "shop"}, "Secret valkey in namespace shop"),
        ({"name": "cart"}, "cart"),
        ({"kind": "Pod"}, None),
        ("", None),
        (None, None),
    ],
)
def test_component_is_written_as_kind_name_and_namespace(value, text):
    assert sregym.component_text(value) == text


STRUCTURED = {
    "markdown": "## Finding\nThe geo Service selects no pods.\n\nRuled out: the database.",
    "root_cause": {
        "component": "hotel-reservation/Service/geo",
        "kind": "config_or_flag",
        "summary": "The Service selector is app=geo-v2, which matches no pod",
    },
    "impact": "search and frontend fail every nearby-hotel request",
    "ruled_out": ["mongodb-geo"],
    "next_steps": ["Restore the selector to app=geo"],
    "no_incident": False,
}


def test_structured_finding_gives_origin_mechanism_and_impact_only():
    assert sregym.render_diagnosis(STRUCTURED) == (
        "Origin: Service geo in namespace hotel-reservation.\n"
        "Mechanism: The Service selector is app=geo-v2, which matches no pod.\n\n"
        "Impact: search and frontend fail every nearby-hotel request."
    )


def test_structured_diagnosis_leaves_out_ruled_out_next_steps_and_markdown():
    text = sregym.render_diagnosis(STRUCTURED)
    for left_out in ("mongodb-geo", "Restore", "Ruled out", "##"):
        assert left_out not in text


def test_a_list_of_causes_wins_over_root_cause_and_skips_unstated_ones():
    finding = {
        **STRUCTURED,
        "causes": [
            {
                "component": {"kind": "Deployment", "name": "frontend", "namespace": "astronomy-shop"},
                "mechanism": "An env var shadows the cart address",
                "trigger": {"key": "CART_ADDR", "value": "cart:9999"},
                "affected": ["astronomy-shop/Deployment/checkout", "cart"],
                "confidence": 0.9,
            },
            {"component": "kafka/StatefulSet/kafka", "summary": "A poison message blocks the partition", "stated": True},
            {"component": "astronomy-shop/Deployment/ad", "summary": "A guess", "stated": False},
        ],
    }
    assert sregym.render_diagnosis(finding) == (
        "Root cause 1:\n"
        "Origin: Deployment frontend in namespace astronomy-shop.\n"
        "Mechanism: An env var shadows the cart address.\n"
        "Changed value: key CART_ADDR, value cart:9999.\n"
        "Affected: Deployment checkout in namespace astronomy-shop, cart.\n\n"
        "Root cause 2:\n"
        "Origin: StatefulSet kafka in namespace kafka.\n"
        "Mechanism: A poison message blocks the partition.\n\n"
        "Impact: search and frontend fail every nearby-hotel request."
    )


def test_a_string_trigger_is_written_as_given():
    finding = {"causes": [{"component": "Secret/valkey", "summary": "The password rotated", "trigger": "password changed at 10:02 UTC"}]}
    assert sregym.render_diagnosis(finding) == "Origin: Secret valkey.\nMechanism: The password rotated.\nChanged value: password changed at 10:02 UTC."


def test_markdown_is_the_diagnosis_when_there_are_no_structured_fields():
    assert sregym.render_diagnosis({"markdown": "  The geo Service selects no pods.\n"}) == "The geo Service selects no pods."
    assert sregym.render_diagnosis({"markdown": "Text", "root_cause": None, "causes": []}) == "Text"
    assert sregym.render_diagnosis({"markdown": "Text", "causes": [{"stated": False, "component": "x", "summary": "y"}]}) == "Text"


def test_no_incident_and_no_finding_are_stated_plainly():
    assert sregym.render_diagnosis({**STRUCTURED, "no_incident": True}) == sregym.NO_FAULT
    assert sregym.render_diagnosis(None) == sregym.NO_DIAGNOSIS
    assert sregym.render_diagnosis({}) == sregym.NO_DIAGNOSIS
    assert sregym.render_diagnosis({"markdown": "   "}) == sregym.NO_DIAGNOSIS
    assert sregym.diagnosis_for({"state": "failed", "finding": None}) == sregym.NO_DIAGNOSIS
    assert sregym.diagnosis_for({"state": "succeeded", "finding": STRUCTURED}).startswith("Origin: Service geo")


def test_kubectl_tool_state_reads_the_tools_listing():
    tools = [
        {"name": "rollback_command", "state": "offered", "refusal": None, "read_only": False},
        {"name": "exec_kubectl_cmd_safely", "state": "approved", "refusal": None, "read_only": False},
    ]
    assert sregym.kubectl_tool_state(tools) == sregym.KubectlTool("approved", None, False)
    refused = [{"name": "exec_kubectl_cmd_safely", "state": "refused", "refusal": "The server does not mark this tool as read-only."}]
    assert sregym.kubectl_tool_state(refused) == sregym.KubectlTool("refused", "The server does not mark this tool as read-only.", None)
    assert sregym.kubectl_tool_state([]) == sregym.KubectlTool("missing")


@pytest.mark.parametrize(
    ("read_only", "mark"),
    [(True, "marked read-only by its server"), (False, "read-only is only the playbook's instruction"), (None, "no read-only mark")],
)
def test_approved_kubectl_note_says_whether_the_tool_is_marked_read_only(read_only, mark):
    tool = sregym.KubectlTool("approved", None, read_only)
    assert tool.approved
    assert tool.note().startswith("kubectl: exec_kubectl_cmd_safely approved on the MCP source sregym-kubectl, ")
    assert mark in tool.note()


def test_a_refused_kubectl_tool_says_the_image_is_too_old():
    reason = sregym.KubectlTool("refused", "The server does not mark this tool as read-only.").reason()
    assert reason.startswith("exec_kubectl_cmd_safely is refused (The server does not mark this tool as read-only.); the server image is too old")
    assert "branch agent-rca-accuracy or later" in reason


def test_a_missing_kubectl_tool_names_the_image_requirement():
    reason = sregym.KubectlTool("missing").reason()
    assert "may be too old or the kubectl MCP server unreachable" in reason
    assert "branch agent-rca-accuracy or later" in reason


@pytest.mark.parametrize(
    ("model", "split"),
    [("anthropic/claude-sonnet-5-5", ("anthropic", "claude-sonnet-5-5")), ("replay/replay", ("replay", None))],
)
def test_model_is_provider_and_name(model, split):
    assert sregym.split_model(model) == split


@pytest.mark.parametrize("model", [None, "", "gpt-5"])
def test_model_without_a_provider_is_refused(model):
    with pytest.raises(ValueError):
        sregym.split_model(model)


@pytest.mark.parametrize(
    ("wall", "remaining", "budget"),
    [(1500, None, 1500), (1500, 1800, 1440), (1500, 3000, 1500), (1500, 100, 0)],
)
def test_run_budget_keeps_time_for_the_submission(wall, remaining, budget):
    assert sregym.deadline_sec(wall, remaining) == budget


def test_port_forward_listens_where_the_server_container_can_reach_it():
    assert sregym.port_forward_args("prometheus-server", 40001, 80) == [
        "kubectl", "port-forward", "-n", "observe", "svc/prometheus-server", "40001:80", "--address", "0.0.0.0",
    ]


def test_subset_is_the_ten_lite_problems_the_owner_chose(capsys):
    assert len(sregym.SUBSET) == len(set(sregym.SUBSET)) == 10
    assert sregym.main(["subset"], env={}) == 0
    assert capsys.readouterr().out.split() == list(sregym.SUBSET)


def test_registry_runs_the_adapter_on_the_host_with_the_given_options(tmp_path):
    out = tmp_path / "agents.yaml"
    assert sregym.main(["registry", "--out", str(out), "--", "--server-image", "slashobserve-server:rca", "--spend-cap-usd", "5", "--ledger-path", "/tmp/a b.jsonl"], env={}) == 0
    (agent,) = yaml.safe_load(out.read_text())["agents"]
    project = Path(sregym.__file__).resolve().parents[1]
    assert agent["name"] == "slashobserve"
    assert agent["container_isolation"] is False
    assert agent["kickoff_workdir"] == str(project)
    assert shlex.split(agent["kickoff_command"]) == [
        "uv", "run", "--locked", "--project", str(project), "python", "-m", "slashobserve_bench.sregym", "run",
        "--server-image", "slashobserve-server:rca", "--spend-cap-usd", "5", "--ledger-path", "/tmp/a b.jsonl",
    ]


SREGYM_ENV = {"AGENT_MODEL_ID": "anthropic/claude-sonnet-5-5", "AGENT_LOGS_DIR": "/logs/run", "API_HOSTNAME": "0.0.0.0", "API_PORT": "8000", "MCP_SERVER_PORT": "9954"}


def _options(argv, env):
    seen = {}

    def runner(options, _env):
        seen["options"] = options
        return 0

    assert sregym.main(["run", *argv], env=env, runner=runner) == 0
    return seen["options"]


def test_run_options_default_to_what_sregym_exports():
    options = _options([], SREGYM_ENV)
    assert options.model == "anthropic/claude-sonnet-5-5"
    assert options.out == "/logs/run"
    assert options.conductor_url == "http://127.0.0.1:8000"
    assert options.mcp_url == "http://host.docker.internal:9954/kubectl/sse"
    assert options.server_image == "slashobserve-server:local"
    assert options.prometheus_url is None and options.loki_url is None
    assert options.without_kubectl is False


def test_run_options_override_the_defaults():
    options = _options(
        ["--model", "replay/replay", "--out", "/tmp/o", "--investigation-loop", "true", "--investigation-calls", "30", "--without-kubectl", "--prometheus-url", "http://kind-control-plane:32000"],
        SREGYM_ENV,
    )
    assert options.model == "replay/replay"
    assert options.out == "/tmp/o"
    assert options.investigation_loop is True
    assert options.investigation_calls == 30
    assert options.without_kubectl is True
    assert options.prometheus_url == "http://kind-control-plane:32000"


@pytest.mark.parametrize(
    ("argv", "env"),
    [
        ([], {"AGENT_MODEL_ID": "anthropic/x"}),
        ([], {"AGENT_LOGS_DIR": "/logs"}),
        (["--spend-cap-usd", "5"], SREGYM_ENV),
        (["--investigation-calls", "20", "--investigation-calls-max", "10"], SREGYM_ENV),
    ],
)
def test_run_options_are_refused_when_incomplete_or_inconsistent(argv, env):
    with pytest.raises(ValueError):
        _options(argv, env)


def test_replay_fixtures_match_both_questions_the_run_can_ask():
    template = {"prompt": "placeholder", "turns": [{"tool_calls": []}]}
    fixtures = sregym.replay_fixtures(template, app())
    assert {name: fixture["prompt"] for name, fixture in fixtures.items()} == {
        "sregym-kubectl.json": sregym.question(app(), True),
        "sregym-telemetry.json": sregym.question(app(), False),
    }
    assert all(fixture["turns"] == template["turns"] for fixture in fixtures.values())
    assert template["prompt"] == "placeholder"


def test_replay_template_saves_a_structured_finding():
    import json

    template = json.loads(sregym.REPLAY_TEMPLATE.read_text())
    saved = template["turns"][-1]["tool_calls"][0]
    assert saved["name"] == "save_finding"
    assert sregym.render_diagnosis(saved["input"]).startswith("Origin: Deployment replay in namespace replay.")
