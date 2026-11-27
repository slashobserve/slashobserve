import json
from pathlib import Path

import pytest

from slashobserve_bench import orca, sregym

FIXTURES = Path(__file__).resolve().parents[1] / "fixtures"
FINDING_KEYS = {"title", "markdown", "causes", "onset", "impact", "confidence", "ruled_out", "next_steps", "no_incident"}
CAUSE_KEYS = {"component", "kind", "mechanism", "trigger", "onset", "affected", "knock_on_of", "evidence", "estimate"}
COMPONENT_KINDS = {"service", "pod", "deployment", "config", "flag", "datastore", "dependency", "node", "other"}
CAUSE_KINDS = {"deploy", "config_or_flag", "dependency", "resource", "data", "traffic", "infrastructure", "unknown"}


def saves(name):
    turns = json.loads((FIXTURES / name).read_text())["turns"]
    return [call for turn in turns for call in turn["tool_calls"] if call["name"] == "save_finding"]


@pytest.mark.parametrize("name", ["orca-answer.json", "sregym-answer.json"])
def test_replay_answer_saves_causes_in_the_tool_input_shape(name):
    found = saves(name)
    assert len(found) == 2
    assert found[0] == found[1]
    finding = found[0]["input"]
    assert set(finding) <= FINDING_KEYS
    assert "root_cause" not in finding
    for cause in finding["causes"]:
        assert set(cause) <= CAUSE_KEYS
        assert {"component", "kind", "mechanism"} <= set(cause)
        assert cause["component"]["kind"] in COMPONENT_KINDS
        assert set(cause["component"]) <= {"kind", "name", "namespace"}
        assert cause["kind"] in CAUSE_KINDS


@pytest.mark.parametrize("name", ["store-answer.json", "orca-answer.json", "sregym-answer.json"])
def test_replay_answer_saves_again_only_after_a_send_back(name):
    turns = json.loads((FIXTURES / name).read_text())["turns"]
    first, second = (i for i, turn in enumerate(turns) if turn["tool_calls"][0]["name"] == "save_finding")
    assert second == first + 1 == len(turns) - 1
    assert "expect" not in turns[first]
    assert turns[second]["expect"] == [{"is_error": True}]
    assert turns[first]["tool_calls"] == turns[second]["tool_calls"]


def test_replay_answers_render_as_the_benchmarks_read_them():
    report = orca.render_report(saves("orca-answer.json")[0]["input"])
    assert "Root cause:\nreplay: The replay model names no real cause." in report
    assert "2026-04-18 00:00 UTC - Root cause: replay" in report
    diagnosis = sregym.render_diagnosis(saves("sregym-answer.json")[0]["input"])
    assert diagnosis.startswith("Origin: deployment replay in namespace replay.\nMechanism: The replay model names no real cause.")
