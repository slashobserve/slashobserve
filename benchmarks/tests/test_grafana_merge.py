import copy
import json

import pytest

from slashobserve_bench import grafana_merge as merge


def spec_panel(panel_id, title, expr="up", y=0):
    return {
        "id": panel_id,
        "type": "timeseries",
        "title": title,
        "gridPos": {"x": 0, "y": y, "w": 12, "h": 8},
        "datasource": "prometheus",
        "queries": [{"query": expr}],
    }


def spec(*panels, variables=None, title="Checkout"):
    document = {"specVersion": "0", "title": title, "time": {"from": "now-1h", "to": "now"}, "panels": list(panels)}
    if variables is not None:
        document["variables"] = variables
    return document


def grafana_panel(panel_id, title, y=0):
    return {
        "id": panel_id,
        "type": "timeseries",
        "title": title,
        "gridPos": {"h": 8, "w": 12, "x": 0, "y": y},
        "datasource": {"type": "prometheus", "uid": "graf-prom"},
        "targets": [{"refId": "A", "expr": "up"}],
        "options": {"legend": {"displayMode": "list"}},
    }


def original_dashboard():
    return {
        "id": 7,
        "uid": "checkout",
        "title": "Checkout",
        "version": 3,
        "panels": [grafana_panel(1, "Requests"), grafana_panel(2, "Errors", y=8)],
        "templating": {"list": [{"name": "env", "type": "custom", "query": "prod,dev"}]},
        "annotations": {"list": [{"name": "Annotations & Alerts", "builtIn": 1}]},
    }


BEFORE = spec(
    spec_panel("p1", "Requests"),
    spec_panel("p2", "Errors", y=8),
    variables=[{"name": "env", "type": "custom", "values": ["prod", "dev"]}],
)


def test_identical_specs_have_no_changes():
    assert merge.spec_changes(BEFORE, copy.deepcopy(BEFORE)).empty


def test_zero_changes_return_the_original_json_unchanged():
    original = original_dashboard()
    kept = copy.deepcopy(original)
    result = merge.merge_dashboard(original, BEFORE, merge.spec_changes(BEFORE, BEFORE), merge.ModelParts())
    assert result.dashboard == kept
    assert json.dumps(result.dashboard, sort_keys=True) == json.dumps(kept, sort_keys=True)
    assert original == kept
    assert result.placed == {} and result.missing == []


def test_a_moved_panel_is_not_a_changed_panel():
    after = copy.deepcopy(BEFORE)
    after["panels"][1]["gridPos"] = {"x": 12, "y": 0, "w": 12, "h": 8}
    assert merge.spec_changes(BEFORE, after).empty


def test_one_added_panel_appears_alone_with_a_fresh_id_below_the_others():
    after = copy.deepcopy(BEFORE)
    after["panels"].append(spec_panel("cache_lag", "Cache lag", expr="cache_lag_seconds"))
    changes = merge.spec_changes(BEFORE, after)
    assert [panel["id"] for panel in changes.added_panels] == ["cache_lag"]
    assert not changes.changed_panels and not changes.removed_panels
    new = {"type": "timeseries", "title": "Cache lag", "id": 1, "targets": [{"refId": "A", "expr": "cache_lag_seconds"}]}
    parts = merge.ModelParts(panels={"cache_lag": new})
    original = original_dashboard()
    result = merge.merge_dashboard(original, after, changes, parts)
    panels = result.dashboard["panels"]
    assert panels[:2] == original["panels"]
    assert len(panels) == 3
    assert panels[2]["id"] == 3
    assert panels[2]["gridPos"] == {"h": 8, "w": 12, "x": 0, "y": 16}
    assert result.placed == {"cache_lag": 3}
    assert {key: value for key, value in result.dashboard.items() if key != "panels"} == {
        key: value for key, value in original.items() if key != "panels"
    }


def test_added_panels_fill_a_row_before_starting_the_next():
    after = copy.deepcopy(BEFORE)
    after["panels"] += [spec_panel("a", "A"), spec_panel("b", "B"), spec_panel("c", "C")]
    parts = merge.ModelParts(panels={name: {"type": "stat", "title": name.upper()} for name in "abc"})
    result = merge.merge_dashboard(original_dashboard(), after, merge.spec_changes(BEFORE, after), parts)
    positions = [panel["gridPos"] for panel in result.dashboard["panels"][2:]]
    assert positions == [
        {"h": 8, "w": 12, "x": 0, "y": 16},
        {"h": 8, "w": 12, "x": 12, "y": 16},
        {"h": 8, "w": 12, "x": 0, "y": 24},
    ]


def test_an_edited_panel_replaces_only_itself_and_keeps_its_id_and_place():
    after = copy.deepcopy(BEFORE)
    after["panels"][1]["queries"] = [{"query": "sum(rate(errors_total[5m]))"}]
    changes = merge.spec_changes(BEFORE, after)
    assert [old["id"] for old, _ in changes.changed_panels] == ["p2"]
    edited = {"type": "timeseries", "title": "Errors", "id": 99, "gridPos": {"x": 5, "y": 5, "w": 3, "h": 3}, "targets": [{"refId": "A", "expr": "sum(rate(errors_total[5m]))"}]}
    original = original_dashboard()
    result = merge.merge_dashboard(original, after, changes, merge.ModelParts(panels={"p2": edited}))
    panels = result.dashboard["panels"]
    assert panels[0] == original["panels"][0]
    assert panels[1]["id"] == 2
    assert panels[1]["gridPos"] == original["panels"][1]["gridPos"]
    assert panels[1]["targets"][0]["expr"] == "sum(rate(errors_total[5m]))"
    assert len(panels) == 2
    assert result.placed == {"p2": 2}


def test_a_panel_inside_a_collapsed_row_is_replaced_in_the_row():
    before = spec({"id": "p10", "type": "row", "title": "Cache", "gridPos": {"x": 0, "y": 0, "w": 24, "h": 1}, "collapsed": True, "panels": [spec_panel("p11", "Hits")]})
    after = copy.deepcopy(before)
    after["panels"][0]["panels"][0]["title"] = "Hit ratio"
    original = {
        "title": "Checkout",
        "panels": [{"id": 10, "type": "row", "title": "Cache", "collapsed": True, "gridPos": {"h": 1, "w": 24, "x": 0, "y": 0}, "panels": [grafana_panel(11, "Hits", y=1)]}],
    }
    result = merge.merge_dashboard(original, after, merge.spec_changes(before, after), merge.ModelParts(panels={"p11": {"type": "timeseries", "title": "Hit ratio"}}))
    row = result.dashboard["panels"][0]
    assert row["panels"][0] == {"type": "timeseries", "title": "Hit ratio", "id": 11, "gridPos": {"h": 8, "w": 12, "x": 0, "y": 1}}


def test_a_changed_row_keeps_its_children():
    before = spec({"id": "p10", "type": "row", "title": "Cache", "gridPos": {"x": 0, "y": 0, "w": 24, "h": 1}, "panels": [spec_panel("p11", "Hits")]})
    after = copy.deepcopy(before)
    after["panels"][0]["title"] = "Cache layer"
    changes = merge.spec_changes(before, after)
    assert [old["id"] for old, _ in changes.changed_panels] == ["p10"]
    children = [grafana_panel(11, "Hits", y=1)]
    original = {"title": "Checkout", "panels": [{"id": 10, "type": "row", "title": "Cache", "collapsed": True, "panels": children}]}
    result = merge.merge_dashboard(original, after, changes, merge.ModelParts(panels={"p10": {"type": "row", "title": "Cache layer", "collapsed": True}}))
    assert result.dashboard["panels"][0]["panels"] == children
    assert result.dashboard["panels"][0]["title"] == "Cache layer"


def test_a_variable_is_added_without_touching_the_others():
    after = copy.deepcopy(BEFORE)
    after["variables"].append({"name": "service", "type": "query", "datasource": "prometheus", "query": "label_values(up, service)", "multi": True, "includeAll": True})
    changes = merge.spec_changes(BEFORE, after)
    assert [variable["name"] for variable in changes.added_variables] == ["service"]
    service = {"name": "service", "type": "query", "multi": True, "includeAll": True, "query": "label_values(up, service)"}
    original = original_dashboard()
    result = merge.merge_dashboard(original, after, changes, merge.ModelParts(variables=[service]))
    entries = result.dashboard["templating"]["list"]
    assert entries == [original["templating"]["list"][0], service]
    assert result.dashboard["panels"] == original["panels"]


def test_a_changed_variable_is_replaced_in_place():
    after = copy.deepcopy(BEFORE)
    after["variables"][0]["values"] = ["prod", "dev", "staging"]
    original = original_dashboard()
    original["templating"]["list"].append({"name": "region", "type": "custom", "query": "eu"})
    env = {"name": "env", "type": "custom", "query": "prod,dev,staging"}
    result = merge.merge_dashboard(original, after, merge.spec_changes(BEFORE, after), merge.ModelParts(variables=[env]))
    assert result.dashboard["templating"]["list"] == [env, original["templating"]["list"][1]]


def test_removed_panels_and_variables_leave_the_grafana_json():
    after = spec(spec_panel("p1", "Requests"), variables=[])
    changes = merge.spec_changes(BEFORE, after)
    result = merge.merge_dashboard(original_dashboard(), after, changes, merge.ModelParts())
    assert [panel["id"] for panel in result.dashboard["panels"]] == [1]
    assert result.dashboard["templating"]["list"] == []


def test_parts_for_untouched_panels_or_variables_are_ignored():
    after = copy.deepcopy(BEFORE)
    after["panels"].append(spec_panel("new", "New"))
    parts = merge.ModelParts(
        panels={"new": {"type": "stat", "title": "New"}, "p1": {"type": "stat", "title": "Rewritten"}},
        variables=[{"name": "env", "type": "custom", "query": "x"}],
    )
    original = original_dashboard()
    result = merge.merge_dashboard(original, after, merge.spec_changes(BEFORE, after), parts)
    assert result.dashboard["panels"][0] == original["panels"][0]
    assert result.dashboard["templating"] == original["templating"]
    assert result.ignored == ["p1", "variable env"]


def test_a_changed_part_the_model_left_out_is_reported_and_the_panel_stays():
    after = copy.deepcopy(BEFORE)
    after["panels"][0]["title"] = "Requests per second"
    original = original_dashboard()
    result = merge.merge_dashboard(original, after, merge.spec_changes(BEFORE, after), merge.ModelParts())
    assert result.missing == ["p1"]
    assert result.dashboard == original


def test_annotations_are_added_or_replaced_by_name():
    parts = merge.ModelParts(annotations=[{"name": "Deploys", "datasource": {"type": "loki", "uid": "graf-loki"}, "expr": "{app=\"deploy\"}"}])
    original = original_dashboard()
    result = merge.merge_dashboard(original, BEFORE, merge.spec_changes(BEFORE, BEFORE), parts)
    assert result.dashboard["annotations"]["list"] == [original["annotations"]["list"][0], parts.annotations[0]]
    again = merge.merge_dashboard(result.dashboard, BEFORE, merge.spec_changes(BEFORE, BEFORE), parts)
    assert len(again.dashboard["annotations"]["list"]) == 2


def test_a_new_dashboard_is_built_from_the_spec_title_and_time():
    after = spec(spec_panel("req", "Requests"), spec_panel("err", "Errors"), title="Service overview", variables=[{"name": "svc", "type": "query"}])
    changes = merge.spec_changes(None, after)
    assert [panel["id"] for panel in changes.added_panels] == ["req", "err"]
    parts = merge.ModelParts(panels={"req": {"type": "timeseries", "title": "Requests"}, "err": {"type": "timeseries", "title": "Errors"}}, variables=[{"name": "svc", "type": "query"}])
    result = merge.merge_dashboard(None, after, changes, parts)
    dashboard = result.dashboard
    assert dashboard["title"] == "Service overview"
    assert dashboard["time"] == {"from": "now-1h", "to": "now"}
    assert [panel["id"] for panel in dashboard["panels"]] == [1, 2]
    assert dashboard["templating"]["list"] == [{"name": "svc", "type": "query"}]
    assert "id" not in dashboard and "uid" not in dashboard


def test_imported_panel_ids_map_back_to_grafana_ids():
    assert merge.grafana_id_of("p12") == 12
    assert merge.grafana_id_of("p12_2") is None
    assert merge.grafana_id_of("cache_lag") is None


def test_a_changed_panel_with_no_grafana_twin_is_added():
    before = spec(spec_panel("extra", "Extra"))
    after = copy.deepcopy(before)
    after["panels"][0]["title"] = "Extra 2"
    result = merge.merge_dashboard(
        {"title": "T", "panels": [grafana_panel(4, "Other")]},
        after,
        merge.spec_changes(before, after),
        merge.ModelParts(panels={"extra": {"type": "stat", "title": "Extra 2"}}),
    )
    assert [panel["id"] for panel in result.dashboard["panels"]] == [4, 5]


def test_parts_are_read_from_a_fenced_reply():
    reply = 'Here:\n```json\n{"panels": [{"spec_id": "a", "panel": {"type": "stat"}}], "variables": [{"name": "v"}]}\n```'
    parts = merge.parse_parts(reply)
    assert parts.panels == {"a": {"type": "stat"}}
    assert parts.variables == [{"name": "v"}]
    assert parts.annotations == []


@pytest.mark.parametrize(
    "reply",
    ["no json here", "{not json}", '{"panels": [{"panel": {}}]}', '{"variables": [{"type": "query"}]}', '{"annotations": {}}'],
)
def test_malformed_parts_are_refused(reply):
    with pytest.raises(merge.PartsError):
        merge.parse_parts(reply)
