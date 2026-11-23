from dataclasses import dataclass
from typing import Any

SCHEMA_VERSION = "ATIF-v1.7"


@dataclass(frozen=True)
class Usage:
    input_tokens: int = 0
    output_tokens: int = 0
    cache_read_tokens: int = 0
    cache_write_tokens: int = 0
    cost_usd: float | None = None
    model_calls: int = 0
    tool_calls: int = 0

    @classmethod
    def from_request(cls, request: dict[str, Any]) -> "Usage":
        usage = request.get("usage") or {}
        return cls(
            input_tokens=usage.get("input_tokens", 0),
            output_tokens=usage.get("output_tokens", 0),
            cache_read_tokens=usage.get("cache_read_tokens", 0),
            cache_write_tokens=usage.get("cache_write_tokens", 0),
            cost_usd=usage.get("cost_usd"),
            model_calls=usage.get("model_calls", 0),
            tool_calls=usage.get("tool_calls", 0),
        )

    @property
    def prompt_tokens(self) -> int:
        return self.input_tokens + self.cache_read_tokens + self.cache_write_tokens


def answer_text(request: dict[str, Any]) -> str:
    finding = request.get("finding")
    if finding:
        return finding["markdown"]
    state = request.get("state", "unknown")
    if state == "succeeded":
        return "The run finished without saving a finding."
    reason = request.get("failure_code") or state
    detail = request.get("error")
    return f"The run ended {state} ({reason}) with no answer." + (f" {detail}" if detail else "")


def _tool_step(index: int, step: dict[str, Any]) -> dict[str, Any]:
    call_id = f"call-{index}"
    return {
        "step_id": index + 1,
        "timestamp": step["at"],
        "source": "agent",
        "message": "",
        "tool_calls": [
            {"tool_call_id": call_id, "function_name": step["tool"], "arguments": {"phase": step["phase"]}}
        ],
        "observation": {"results": [{"source_call_id": call_id, "content": f'{step["status"]}: {step["message"]}'}]},
        "extra": {"duration_ms": step["duration_ms"], "attempt": step["attempt"]},
    }


def build_trajectory(
    request: dict[str, Any],
    instruction: str,
    *,
    agent_name: str,
    agent_version: str,
    model_name: str | None,
) -> dict[str, Any]:
    usage = Usage.from_request(request)
    steps: list[dict[str, Any]] = [
        {"step_id": 1, "timestamp": request["created_at"], "source": "user", "message": instruction}
    ]
    for step in request.get("steps") or []:
        steps.append(_tool_step(len(steps), step))
    final_step: dict[str, Any] = {
        "step_id": len(steps) + 1,
        "source": "agent",
        "message": answer_text(request),
    }
    if model_name:
        final_step["model_name"] = model_name
    if request.get("finished_at"):
        final_step["timestamp"] = request["finished_at"]
    steps.append(final_step)
    agent: dict[str, Any] = {"name": agent_name, "version": agent_version}
    if model_name:
        agent["model_name"] = model_name
    return {
        "schema_version": SCHEMA_VERSION,
        "session_id": request["id"],
        "agent": agent,
        "steps": steps,
        "final_metrics": {
            "total_prompt_tokens": usage.prompt_tokens,
            "total_completion_tokens": usage.output_tokens,
            "total_cached_tokens": usage.cache_read_tokens,
            "total_cost_usd": usage.cost_usd,
            "total_steps": len(steps),
            "extra": {
                "model_calls": usage.model_calls,
                "tool_calls": usage.tool_calls,
                "state": request.get("state"),
                "failure_code": request.get("failure_code"),
                "as_of": request.get("as_of"),
            },
        },
    }
