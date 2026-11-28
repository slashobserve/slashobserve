import pytest
import yaml

from slashobserve_bench import adapter
from slashobserve_bench.api import ApiError, RunTimeout


class Runs:
    def __init__(self, replies, cancel_fails=False):
        self.replies = list(replies)
        self.canceled = False
        self.cancel_fails = cancel_fails

    def start_run(self, body):
        return "run-1"

    def get_run(self, run_id):
        return self.replies.pop(0) if len(self.replies) > 1 else self.replies[0]

    def cancel_run(self, run_id):
        self.canceled = True
        if self.cancel_fails:
            raise ApiError(409, "already ending")


def test_a_run_that_ends_in_time_is_returned_and_recorded():
    recorded = []
    request, timeout = adapter.run_to_end(Runs([{"state": "succeeded"}]), {}, 10, recorded.append)
    assert (request, timeout) == ({"state": "succeeded"}, None)
    assert recorded == [{"state": "succeeded"}]


def test_a_run_past_its_budget_is_canceled_and_its_last_state_kept(monkeypatch):
    monkeypatch.setattr("slashobserve_bench.api.time.sleep", lambda seconds: None)
    runs = Runs([{"state": "running"}, {"state": "canceled"}], cancel_fails=True)
    recorded = []
    request, timeout = adapter.run_to_end(runs, {}, 0, recorded.append)
    assert runs.canceled
    assert request == {"state": "canceled"}
    assert isinstance(timeout, RunTimeout)
    assert recorded == [{"state": "canceled"}]


def test_a_run_that_outlives_the_grace_keeps_what_was_seen(monkeypatch):
    monkeypatch.setattr("slashobserve_bench.api.time.sleep", lambda seconds: None)
    request, timeout = adapter.run_to_end(Runs([{"state": "running"}]), {}, 0, grace_sec=0)
    assert request == {"state": "running"}
    assert isinstance(timeout, RunTimeout)


def test_the_options_need_a_cap_and_a_ledger_together(tmp_path):
    assert adapter.RunOptions().spend_ledger() is None
    assert adapter.RunOptions(spend_cap_usd=2, ledger_path=str(tmp_path / "l.jsonl")).spend_ledger().cap_usd == 2
    with pytest.raises(ValueError, match="go together"):
        adapter.RunOptions(spend_cap_usd=2)


def test_the_model_names_its_provider_and_the_key_comes_from_its_file(tmp_path):
    assert adapter.split_model("anthropic/claude-sonnet-5-5") == ("anthropic", "claude-sonnet-5-5")
    assert adapter.split_model("replay/replay") == ("replay", None)
    with pytest.raises(ValueError, match="--model anthropic"):
        adapter.split_model("claude", "--model")
    key = tmp_path / "key"
    key.write_text(" sk-test\n")
    assert adapter.llm_key("anthropic", str(key)) == "sk-test"
    assert adapter.llm_key("replay", None) is None
    with pytest.raises(ValueError, match=f"{adapter.LLM_KEY_FILE_ENV} with --ae to"):
        adapter.llm_key("anthropic", None, " with --ae")


def test_an_mcp_source_file_approves_only_the_tools_it_names():
    text = adapter.mcp_sources_yaml("code", "Code", "Reads code.", "http://code:8080/mcp", ["read_file"])
    (source,) = yaml.safe_load(text)["mcpSources"]
    assert source == {
        "uid": "code",
        "name": "Code",
        "description": "Reads code.",
        "url": "http://code:8080/mcp",
        "audience": "everyone",
        "allowPrivate": True,
        "approve": ["read_file"],
    }
    assert adapter.mcp_source_settings("code") == (
        ("SLASHOBSERVE_MCP_SOURCES_FILE", "/etc/slashobserve/mcp-sources.yaml"),
        ("SLASHOBSERVE_OUTBOUND_DOMAINS", "code"),
    )


def test_causes_split_into_roots_their_knock_ons_and_the_rest():
    entries = [
        {"component": "a"},
        {"component": "b", "knock_on_of": 0},
        {"component": "c", "stated": False},
        {"component": "d", "knock_on_of": 2},
        "not an entry",
        {"component": "e", "knock_on_of": 0, "relation": "independent"},
        {"component": "f", "knock_on_of": True},
    ]
    found = adapter.read_causes(entries)
    assert [index for index, _ in found.roots] == [0, 5, 6]
    assert [(index, parent) for index, _, parent in found.knock_ons] == [(1, 0)]
    assert [index for index, _ in found.unconfirmed] == [2, 3]
