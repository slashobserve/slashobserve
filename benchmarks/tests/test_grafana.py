import asyncio
import base64
import json
from types import SimpleNamespace

import pytest

from slashobserve_bench import grafana

DATASOURCES = [
    {"id": 1, "uid": "graf-prom", "name": "Prometheus", "type": "prometheus", "url": "http://prometheus:9090", "access": "proxy", "isDefault": True, "basicAuthUser": "ignored"},
    {"id": 2, "uid": "graf-loki", "name": "Loki", "type": "loki", "url": "http://loki:3100", "access": "proxy", "isDefault": False},
    {"id": 3, "uid": "tempo", "name": "Traces", "type": "tempo", "url": "http://tempo:3200"},
    {"id": 4, "uid": "graf-pg", "name": "Postgres", "type": "grafana-postgresql-datasource"},
]


def state(dashboards=None):
    return grafana.GrafanaState(
        datasources=DATASOURCES,
        dashboards=dashboards
        if dashboards is not None
        else [
            {"dashboard": {"uid": "svc", "title": "Service Overview", "tags": ["team-a"]}, "meta": {"folderTitle": "Platform"}},
            {"dashboard": {"uid": "cache", "title": "Cache Review"}, "meta": {}},
        ],
    )


def test_a_token_in_the_task_environment_wins():
    env = {"GRAFANA_API_KEY": "glsa_x", "GRAFANA_USER": "admin", "GRAFANA_PASSWORD": "pw"}
    assert grafana.grafana_auth(env, "other", "secret") == "Bearer glsa_x"


def test_a_user_and_password_in_the_task_environment_win_over_options():
    auth = grafana.grafana_auth({"GRAFANA_USER": "admin", "GRAFANA_PASSWORD": "pw"}, "other", "x")
    assert base64.b64decode(auth.removeprefix("Basic ")).decode() == "admin:pw"


def test_options_give_basic_auth_when_the_task_has_none():
    auth = grafana.grafana_auth({}, "viewer", "pw")
    assert base64.b64decode(auth.removeprefix("Basic ")).decode() == "viewer:pw"


def test_no_credentials_mean_no_header():
    assert grafana.grafana_auth({"GRAFANA_USER": "admin"}) is None


def test_grafana_uids_and_names_map_to_the_store_of_their_type():
    assert grafana.grafana_uids_by_type(DATASOURCES) == {
        "prometheus": ["graf-prom", "Prometheus"],
        "loki": ["graf-loki", "Loki"],
        "tempo": ["Traces"],
    }


def test_a_grafana_uid_listed_twice_is_kept_once():
    datasources = [
        {"uid": "shared", "name": "A", "type": "prometheus"},
        {"uid": "shared", "name": "B", "type": "loki"},
    ]
    assert grafana.grafana_uids_by_type(datasources) == {"prometheus": ["shared", "A"], "loki": ["B"]}


def test_the_store_map_prefers_the_default_datasource_of_each_type():
    datasources = [{"uid": "a", "name": "A", "type": "prometheus"}, {"uid": "b", "name": "B", "type": "prometheus", "isDefault": True}]
    assert grafana.datasource_map(datasources) == {"prometheus": {"uid": "b", "type": "prometheus", "name": "B"}}


def test_the_context_note_names_datasources_and_dashboards_without_credentials():
    note = grafana.context_note(state(), {"svc": "d-1"}, 4000)
    assert "- name Prometheus, type prometheus, uid graf-prom, id 1, url http://prometheus:9090, access proxy, isDefault True" in note
    assert "- name Postgres, type grafana-postgresql-datasource, uid graf-pg, id 4" in note
    assert "- title Service Overview; uid svc; folder Platform; tags team-a (imported here as dashboard d-1)" in note
    assert "- title Cache Review; uid cache" in note
    assert "ignored" not in note


def test_the_context_note_is_cut_to_its_limit():
    many = [{"dashboard": {"uid": f"d{i}", "title": f"Dashboard {i}"}, "meta": {}} for i in range(200)]
    note = grafana.context_note(state(many), {}, 1000)
    assert len(note) <= 1000
    assert note.endswith("- (cut short)")


def test_the_question_prompt_keeps_the_instruction_first_and_fits_the_server_limit():
    many = [{"dashboard": {"uid": f"d{i}", "title": f"Dashboard {i}"}, "meta": {}} for i in range(400)]
    prompt = grafana.question_prompt("  Which datasources exist?  ", state(many), {})
    assert prompt.startswith("Which datasources exist?\n\nContext from the Grafana instance")
    assert len(prompt) <= grafana.PROMPT_LIMIT


def test_a_long_instruction_goes_without_a_note():
    instruction = "x" * 3900
    assert grafana.question_prompt(instruction, state(), {}) == instruction


def test_the_curl_command_quotes_everything_and_reads_auth_from_the_environment():
    command = grafana.curl_command("POST", "http://grafana:3000/api/x?a=1&b=2", {"Content-Type": "application/json"}, "/tmp/b.json", True)
    assert "'http://grafana:3000/api/x?a=1&b=2'" in command
    assert f'-H "Authorization: ${grafana.AUTH_ENV}"' in command
    assert "--data-binary @/tmp/b.json" in command
    assert "Bearer" not in command and "Basic" not in command


def test_curl_output_splits_into_status_and_body():
    assert grafana.split_curl_output('{"a": 1}\n200') == (200, b'{"a": 1}')
    with pytest.raises(grafana.GrafanaError):
        grafana.split_curl_output("curl: (6) Could not resolve host")


class FakeEnvironment:
    def __init__(self, stdout):
        self.stdout = stdout
        self.uploaded = {}
        self.commands = []

    async def upload_file(self, source, target):
        self.uploaded[target] = open(source, "rb").read()

    async def exec(self, command, env=None):
        self.commands.append((command, env))
        return SimpleNamespace(stdout=self.stdout, stderr="", return_code=0)


def test_the_container_transport_uploads_the_body_and_passes_auth_as_env():
    environment = FakeEnvironment('{"status": "success", "uid": "new"}\n200')
    client = grafana.GrafanaClient("http://grafana:3000/", grafana.container_transport(environment, "Bearer t"))
    answer = asyncio.run(client.save_dashboard({"title": "T"}, "folder-1", "saved"))
    assert answer["uid"] == "new"
    (body,) = environment.uploaded.values()
    assert json.loads(body) == {"dashboard": {"title": "T"}, "overwrite": True, "message": "saved", "folderUid": "folder-1"}
    command, env = environment.commands[0]
    assert "http://grafana:3000/api/dashboards/db" in command
    assert env == {grafana.AUTH_ENV: "Bearer t"}


def test_an_error_status_raises_with_the_body():
    environment = FakeEnvironment('{"message": "Unauthorized"}\n401')
    client = grafana.GrafanaClient("http://grafana:3000", grafana.container_transport(environment, None))
    with pytest.raises(grafana.GrafanaError, match="401"):
        asyncio.run(client.datasources())
    assert environment.commands[0][1] is None


def test_read_state_fetches_every_dashboard_the_search_finds():
    answers = {
        "/api/datasources": DATASOURCES,
        "/api/search?type=dash-db&limit=5000": [{"uid": "svc"}, {"uid": ""}, {"uid": "cache"}],
        "/api/dashboards/uid/svc": {"dashboard": {"uid": "svc"}, "meta": {}},
        "/api/dashboards/uid/cache": {"dashboard": {"uid": "cache"}, "meta": {}},
    }

    async def transport(method, url, headers, body):
        return 200, json.dumps(answers[url.removeprefix("http://g")]).encode()

    read = asyncio.run(grafana.read_state(grafana.GrafanaClient("http://g", transport)))
    assert [entry["dashboard"]["uid"] for entry in read.dashboards] == ["svc", "cache"]
    assert read.dashboard_by_uid("cache") == answers["/api/dashboards/uid/cache"]
