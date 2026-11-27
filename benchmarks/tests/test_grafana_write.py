import asyncio
import copy
import json
from datetime import UTC, datetime

from slashobserve_bench import grafana_write as write
from slashobserve_bench.atif import Usage
from slashobserve_bench.grafana_merge import spec_changes
from slashobserve_bench.model_call import ModelCallError, ModelReply

VARIABLES = [
    {"name": "service", "type": "query", "current": {"text": "All", "value": ["$__all"]}, "multi": True, "includeAll": True},
    {"name": "env", "type": "custom", "query": "prod,dev", "current": {}},
    {"name": "pod", "type": "query", "current": {"value": ["a", "b"]}},
    {"name": "region", "type": "query", "current": {"value": "eu"}, "allValue": "eu|us"},
    {"name": "ds", "type": "datasource", "current": {"value": "graf-loki"}},
]


def test_interpolation_uses_each_variable_value_and_keeps_builtins():
    text = 'sum(rate(x{service=~"$service", env="${env}", pod=~"[[pod]]", region="${region:raw}"}[$__rate_interval]))'
    assert write.interpolate(text, VARIABLES) == 'sum(rate(x{service=~".+", env="prod", pod=~"a|b", region="eu"}[$__rate_interval]))'


def test_all_uses_the_variable_all_value_and_an_unknown_variable_matches_anything():
    variables = [{"name": "region", "current": {"value": "$__all"}, "allValue": "eu|us"}]
    assert write.interpolate("$region/$nope", variables) == "eu|us/.+"


def test_panel_queries_resolve_datasources_skip_hidden_targets_and_name_ref_ids():
    panel = {
        "datasource": {"type": "prometheus", "uid": "graf-prom"},
        "targets": [
            {"expr": 'up{service=~"$service"}'},
            {"refId": "B", "expr": "down", "hide": True},
            {"refId": "C", "expr": '{app="x"}', "datasource": {"type": "loki", "uid": "${ds}"}},
            {"refId": "D", "datasource": {"uid": "-- Dashboard --"}},
        ],
    }
    queries = write.panel_queries(panel, {"templating": {"list": VARIABLES}})
    assert [query["refId"] for query in queries] == ["A", "C"]
    assert queries[0]["expr"] == 'up{service=~".+"}'
    assert queries[0]["datasource"] == {"uid": "graf-prom", "type": "prometheus"}
    assert queries[1]["datasource"] == {"uid": "graf-loki", "type": "loki"}
    assert queries[0]["maxDataPoints"] == 500


def test_query_failures_name_each_failed_ref_id():
    response = {"results": {"A": {"status": 200, "frames": []}, "B": {"error": "parse error", "status": 400}}}
    assert write.query_failures(400, response, ["A", "B"]) == ["B: parse error"]
    assert write.query_failures(200, {"results": {"A": {"frames": []}}}, ["A"]) == []
    assert write.query_failures(502, {"message": "bad gateway"}, ["A"]) == ["HTTP 502: bad gateway"]


def test_the_check_range_ends_at_the_scenario_time():
    end = int(datetime(2026, 11, 20, 10, tzinfo=UTC).timestamp() * 1000)
    assert write.check_range("2026-11-20T10:00:00Z") == {"from": str(end - 6 * 3600 * 1000), "to": str(end)}
    assert write.check_range("") == {"from": "now-6h", "to": "now"}


BEFORE = {
    "specVersion": "0",
    "title": "Cache review",
    "time": {"from": "now-1h", "to": "now"},
    "panels": [{"id": "p1", "type": "stat", "title": "Hits", "gridPos": {"x": 0, "y": 0, "w": 6, "h": 4}, "datasource": "prometheus", "queries": [{"query": "hits"}]}],
}
ORIGINAL = {
    "id": 3,
    "uid": "cache",
    "title": "Cache review",
    "version": 2,
    "panels": [{"id": 1, "type": "stat", "title": "Hits", "gridPos": {"h": 4, "w": 6, "x": 0, "y": 0}, "targets": [{"refId": "A", "expr": "hits"}]}],
}
DATASOURCES = [{"uid": "graf-prom", "name": "Prometheus", "type": "prometheus"}]


def after_with_lag():
    after = copy.deepcopy(BEFORE)
    after["panels"].append({"id": "lag", "type": "timeseries", "title": "Lag", "gridPos": {"x": 0, "y": 4, "w": 12, "h": 8}, "datasource": "prometheus", "queries": [{"query": "lag"}]})
    return after


def parts(expr):
    return json.dumps({"panels": [{"spec_id": "lag", "panel": {"type": "timeseries", "title": "Lag", "datasource": {"type": "prometheus", "uid": "graf-prom"}, "targets": [{"refId": "A", "expr": expr}]}}]})


def reply(text):
    return ModelReply(text=text, usage=Usage(input_tokens=100, output_tokens=10, cost_usd=0.01, model_calls=1), request={"messages": []}, response={})


class FakeGrafana:
    def __init__(self, query_answers):
        self.saved = []
        self.queries = []
        self.query_answers = list(query_answers)

    async def save_dashboard(self, dashboard, folder_uid, message):
        self.saved.append((copy.deepcopy(dashboard), folder_uid))
        return {"uid": dashboard.get("uid") or "new-uid", "status": "success"}

    async def dashboard(self, uid):
        return {"dashboard": copy.deepcopy(self.saved[-1][0])}

    async def query(self, body):
        self.queries.append(body)
        return self.query_answers.pop(0)


def run_write_back(replies, grafana, original=ORIGINAL, before=BEFORE, after=None):
    sent = []
    seen = []

    async def call(system, messages):
        sent.append(copy.deepcopy(messages))
        answer = replies.pop(0)
        if isinstance(answer, Exception):
            raise answer
        return reply(answer)

    state = write.WriteBack()
    asyncio.run(
        write.write_back(
            call=call,
            client=grafana,
            instruction="Add a lag panel.",
            datasources=DATASOURCES,
            original=original,
            meta={"folderUid": "f1"},
            before_spec=before,
            after_spec=after if after is not None else after_with_lag(),
            window={"from": "1", "to": "2"},
            state=state,
            on_reply=lambda number, answer: seen.append(number),
        )
    )
    return state, sent, seen


def test_a_new_panel_is_saved_into_the_original_and_checked_once():
    grafana = FakeGrafana([(200, {"results": {"A": {"frames": []}}})])
    state, sent, seen = run_write_back([parts("lag")], grafana)
    assert state.calls == 1 and seen == [1]
    dashboard, folder = grafana.saved[0]
    assert folder == "f1"
    assert dashboard["panels"][0] == ORIGINAL["panels"][0]
    assert dashboard["panels"][1]["id"] == 2
    assert dashboard["uid"] == "cache"
    assert grafana.queries[0]["queries"][0]["expr"] == "lag"
    assert state.saved_uid == "cache"
    assert state.attempts[0]["failures"] == {}
    assert state.usage.cost_usd == 0.01
    request = json.loads(sent[0][0]["content"].split("\n", 1)[1])
    assert request["grafana_dashboard"] == ORIGINAL
    assert [panel["id"] for panel in request["added_panels"]] == ["lag"]
    assert request["spec_datasource_to_grafana"] == {"prometheus": {"uid": "graf-prom", "type": "prometheus", "name": "Prometheus"}}


def test_a_failed_query_goes_back_to_the_model_exactly_once():
    failing = (400, {"results": {"A": {"error": "bad expr", "status": 400}}})
    grafana = FakeGrafana([failing, failing])
    state, sent, _ = run_write_back([parts("lag{"), parts("lag{{"), parts("never")], grafana)
    assert state.calls == 2
    assert len(grafana.saved) == 2
    assert "panel lag: A: bad expr" in sent[1][-1]["content"]
    assert sent[1][-2] == {"role": "assistant", "content": parts("lag{")}
    assert state.attempts[1]["failures"] == {"lag": ["A: bad expr"]}
    assert state.usage.input_tokens == 200


def test_a_retry_that_fixes_the_query_stops_there():
    grafana = FakeGrafana([(400, {"results": {"A": {"error": "bad"}}}), (200, {"results": {"A": {}}})])
    state, _, _ = run_write_back([parts("lag{"), parts("lag")], grafana)
    assert state.calls == 2
    assert grafana.saved[1][0]["panels"][1]["targets"][0]["expr"] == "lag"
    assert grafana.saved[1][0]["panels"][0] == ORIGINAL["panels"][0]


def test_an_unreadable_reply_is_retried_without_saving():
    grafana = FakeGrafana([(200, {"results": {"A": {}}})])
    state, sent, _ = run_write_back(["no json", parts("lag")], grafana)
    assert len(grafana.saved) == 1
    assert "could not be read" in sent[1][-1]["content"]
    assert state.attempts[0]["parse_error"]


def test_no_change_and_no_annotation_saves_nothing():
    grafana = FakeGrafana([])
    state, _, _ = run_write_back(['{"panels": [], "variables": [], "annotations": []}'], grafana, after=copy.deepcopy(BEFORE))
    assert grafana.saved == []
    assert state.attempts == [{"attempt": 1, "placed": {}, "missing": [], "ignored": [], "saved": False}]


def test_a_new_dashboard_keeps_its_grafana_uid_on_the_retry():
    grafana = FakeGrafana([(400, {"results": {"A": {"error": "bad"}}}), (200, {"results": {"A": {}}})])
    after = after_with_lag()
    after["title"] = "Lag review"
    state, _, _ = run_write_back([parts("x{"), parts("x")], grafana, original=None, before=None, after=after)
    assert "uid" not in grafana.saved[0][0]
    assert grafana.saved[1][0]["uid"] == "new-uid"
    assert state.saved_uid == "new-uid"
    assert spec_changes(None, after).written_panel_ids == ["p1", "lag"]


def test_a_failed_model_call_is_recorded_and_ends_the_write():
    grafana = FakeGrafana([])
    state, _, _ = run_write_back([ModelCallError("the model API answered HTTP 529")], grafana)
    assert state.error == "the model API answered HTTP 529"
    assert grafana.saved == [] and state.calls == 0
