import json

import pytest

from slashobserve_bench import model_call
from slashobserve_bench.atif import Usage


class Response:
    def __init__(self, body):
        self.body = json.dumps(body).encode()

    def __enter__(self):
        return self

    def __exit__(self, *args):
        return False

    def read(self):
        return self.body


def capture(monkeypatch, body):
    sent = {}

    def urlopen(request, timeout):
        sent["url"] = request.full_url
        sent["headers"] = dict(request.header_items())
        sent["body"] = json.loads(request.data)
        sent["timeout"] = timeout
        return Response(body)

    monkeypatch.setattr(model_call.urllib.request, "urlopen", urlopen)
    return sent


ANTHROPIC_REPLY = {
    "content": [{"type": "thinking", "thinking": ""}, {"type": "text", "text": '{"panels": []}'}],
    "stop_reason": "end_turn",
    "usage": {"input_tokens": 1000, "output_tokens": 200, "cache_read_input_tokens": 50, "cache_creation_input_tokens": 10},
}


def test_an_anthropic_call_sends_the_key_only_as_a_header_and_reads_text_and_usage(monkeypatch):
    sent = capture(monkeypatch, ANTHROPIC_REPLY)
    reply = model_call.call_model("anthropic", "claude-haiku-5-5", "sk-secret", "system text", [{"role": "user", "content": "hi"}], 120)
    assert sent["url"] == model_call.ANTHROPIC_URL
    assert sent["headers"]["X-api-key"] == "sk-secret"
    assert sent["body"] == {"model": "claude-haiku-5-5", "max_tokens": model_call.MAX_TOKENS, "system": "system text", "messages": [{"role": "user", "content": "hi"}]}
    assert reply.text == '{"panels": []}'
    assert reply.usage == Usage(input_tokens=1000, output_tokens=200, cache_read_tokens=50, cache_write_tokens=10, model_calls=1)
    assert "sk-secret" not in json.dumps(reply.request) + json.dumps(reply.response)


def test_an_openai_call_puts_the_system_text_first_and_splits_cached_tokens(monkeypatch):
    body = {"choices": [{"message": {"content": "{}"}}], "usage": {"prompt_tokens": 900, "completion_tokens": 30, "prompt_tokens_details": {"cached_tokens": 100}}}
    sent = capture(monkeypatch, body)
    reply = model_call.call_model("openai", "gpt-x", "sk-o", "sys", [{"role": "user", "content": "u"}], 60)
    assert sent["headers"]["Authorization"] == "Bearer sk-o"
    assert sent["body"]["messages"][0] == {"role": "system", "content": "sys"}
    assert reply.usage == Usage(input_tokens=800, output_tokens=30, cache_read_tokens=100, model_calls=1)


def test_a_refusal_is_an_error():
    with pytest.raises(model_call.ModelCallError, match="refused"):
        model_call.anthropic_reply({"stop_reason": "refusal", "content": []})


def test_an_unknown_provider_is_an_error():
    with pytest.raises(model_call.ModelCallError, match="replay"):
        model_call.call_model("replay", "replay", "", "s", [], 10)


def test_an_http_error_names_the_status_and_not_the_key(monkeypatch):
    def urlopen(request, timeout):
        raise model_call.urllib.error.HTTPError(request.full_url, 401, "no", {}, None)

    monkeypatch.setattr(model_call.urllib.request, "urlopen", urlopen)
    with pytest.raises(model_call.ModelCallError, match="HTTP 401") as raised:
        model_call.call_model("anthropic", "m", "sk-secret", "s", [], 10)
    assert "sk-secret" not in str(raised.value)


PRICES = {
    "claude-haiku-5-5": {"input": 0.10, "output": 0.50, "cache_read": 0.01, "cache_write": 0.125, "match": ["claude-haiku-5-5"]},
    "claude-haiku": {"input": 9, "output": 9, "cache_read": 9, "cache_write": 9, "match": ["claude-haiku"]},
    "partial": {"input": 1.0, "output": 2.0},
}


def test_cost_uses_the_exact_name_then_the_longest_prefix():
    usage = Usage(input_tokens=1_000_000, output_tokens=1_000_000, cache_read_tokens=1_000_000, cache_write_tokens=1_000_000)
    assert model_call.cost_of("claude-haiku-5-5", usage, PRICES) == pytest.approx(0.735)
    assert model_call.cost_of("claude-haiku-5-5-20261001", usage, PRICES) == pytest.approx(0.735)
    assert model_call.cost_of("unknown", usage, PRICES) is None


def test_a_token_class_without_a_rate_leaves_the_cost_empty():
    assert model_call.cost_of("partial", Usage(input_tokens=10, cache_read_tokens=5), PRICES) is None
    assert model_call.cost_of("partial", Usage(input_tokens=1_000_000), PRICES) == pytest.approx(1.0)


def test_the_server_price_table_is_readable_and_prices_the_benchmark_models():
    prices = model_call.load_prices()
    for model in ("claude-haiku-5-5", "claude-sonnet-5-5", "claude-opus-5-5"):
        assert model_call.price_for(model, prices) is not None


def test_a_missing_price_table_prices_nothing(tmp_path):
    assert model_call.load_prices(tmp_path / "none.yaml") == {}
