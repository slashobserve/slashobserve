import json
import urllib.error
import urllib.request
from dataclasses import dataclass
from pathlib import Path
from typing import Any

import yaml

from slashobserve_bench.atif import Usage

ANTHROPIC_URL = "https://api.anthropic.com/v1/messages"
ANTHROPIC_VERSION = "2023-06-01"
OPENAI_URL = "https://api.openai.com/v1/chat/completions"
MAX_TOKENS = 16000
PRICES_FILE = Path(__file__).resolve().parents[2] / "internal" / "llm" / "pricing" / "prices.yaml"


class ModelCallError(Exception):
    pass


@dataclass(frozen=True)
class ModelReply:
    text: str
    usage: Usage
    request: dict[str, Any]
    response: dict[str, Any]


def anthropic_request(model: str, system: str, messages: list[dict[str, str]]) -> dict[str, Any]:
    return {"model": model, "max_tokens": MAX_TOKENS, "system": system, "messages": messages}


def openai_request(model: str, system: str, messages: list[dict[str, str]]) -> dict[str, Any]:
    return {"model": model, "messages": [{"role": "system", "content": system}, *messages]}


def anthropic_reply(response: dict[str, Any]) -> tuple[str, Usage]:
    if response.get("stop_reason") == "refusal":
        raise ModelCallError("the model refused the request")
    text = "".join(block.get("text", "") for block in response.get("content") or [] if block.get("type") == "text")
    usage = response.get("usage") or {}
    return text, Usage(
        input_tokens=usage.get("input_tokens") or 0,
        output_tokens=usage.get("output_tokens") or 0,
        cache_read_tokens=usage.get("cache_read_input_tokens") or 0,
        cache_write_tokens=usage.get("cache_creation_input_tokens") or 0,
        model_calls=1,
    )


def openai_reply(response: dict[str, Any]) -> tuple[str, Usage]:
    choices = response.get("choices") or []
    if not choices:
        raise ModelCallError("the model returned no choice")
    text = (choices[0].get("message") or {}).get("content") or ""
    usage = response.get("usage") or {}
    cached = (usage.get("prompt_tokens_details") or {}).get("cached_tokens") or 0
    return text, Usage(
        input_tokens=(usage.get("prompt_tokens") or 0) - cached,
        output_tokens=usage.get("completion_tokens") or 0,
        cache_read_tokens=cached,
        model_calls=1,
    )


def _post(url: str, headers: dict[str, str], body: dict[str, Any], timeout: float) -> dict[str, Any]:
    request = urllib.request.Request(url, data=json.dumps(body).encode(), method="POST")
    request.add_header("Content-Type", "application/json")
    for name, value in headers.items():
        request.add_header(name, value)
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            return json.loads(response.read())
    except urllib.error.HTTPError as error:
        raise ModelCallError(f"the model API answered HTTP {error.code}: {error.read().decode(errors='replace')[:500]}") from None
    except (urllib.error.URLError, TimeoutError, OSError) as error:
        raise ModelCallError(f"the model API could not be reached: {error}") from None


def call_model(provider: str, model: str, key: str, system: str, messages: list[dict[str, str]], timeout: float) -> ModelReply:
    if provider == "anthropic":
        body = anthropic_request(model, system, messages)
        response = _post(ANTHROPIC_URL, {"x-api-key": key, "anthropic-version": ANTHROPIC_VERSION}, body, timeout)
        text, usage = anthropic_reply(response)
    elif provider == "openai":
        body = openai_request(model, system, messages)
        response = _post(OPENAI_URL, {"Authorization": f"Bearer {key}"}, body, timeout)
        text, usage = openai_reply(response)
    else:
        raise ModelCallError(f"the provider {provider!r} has no model call here")
    return ModelReply(text=text, usage=usage, request=body, response=response)


def load_prices(path: Path = PRICES_FILE) -> dict[str, dict[str, Any]]:
    try:
        data = yaml.safe_load(path.read_text(encoding="utf-8"))
    except OSError:
        return {}
    return (data or {}).get("models") or {}


def price_for(model: str, prices: dict[str, dict[str, Any]]) -> dict[str, Any] | None:
    if model in prices:
        return prices[model]
    best, best_length = None, -1
    for entry in prices.values():
        for prefix in entry.get("match") or []:
            if model.startswith(prefix) and len(prefix) > best_length:
                best, best_length = entry, len(prefix)
    return best


def cost_of(model: str, usage: Usage, prices: dict[str, dict[str, Any]]) -> float | None:
    price = price_for(model, prices)
    if price is None:
        return None
    total = 0.0
    for tokens, rate in (
        (usage.input_tokens, "input"),
        (usage.output_tokens, "output"),
        (usage.cache_read_tokens, "cache_read"),
        (usage.cache_write_tokens, "cache_write"),
    ):
        if not tokens:
            continue
        if price.get(rate) is None:
            return None
        total += tokens * float(price[rate]) / 1_000_000
    return total
