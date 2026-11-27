from dataclasses import dataclass
from pathlib import Path
from typing import Any

SCHEMA_VERSION = "ATIF-v1.7"
DETAIL_DIR = "detail"


def run_file(trial_dir: Path) -> Path:
    detail = trial_dir / "agent" / DETAIL_DIR / "run.json"
    return detail if detail.is_file() else trial_dir / "agent" / "run.json"


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

    def plus(self, other: "Usage") -> "Usage":
        cost = None
        if self.cost_usd is not None or other.cost_usd is not None:
            cost = (self.cost_usd or 0.0) + (other.cost_usd or 0.0)
        return Usage(
            input_tokens=self.input_tokens + other.input_tokens,
            output_tokens=self.output_tokens + other.output_tokens,
            cache_read_tokens=self.cache_read_tokens + other.cache_read_tokens,
            cache_write_tokens=self.cache_write_tokens + other.cache_write_tokens,
            cost_usd=cost,
            model_calls=self.model_calls + other.model_calls,
            tool_calls=self.tool_calls + other.tool_calls,
        )

    @property
    def prompt_tokens(self) -> int:
        return self.input_tokens + self.cache_read_tokens + self.cache_write_tokens


def _component(value: Any) -> str:
    if not isinstance(value, dict):
        return str(value or "")
    return f'{value.get("namespace")}/{value.get("name")}' if value.get("namespace") else str(value.get("name") or "")


def cause_line(number: int, cause: dict[str, Any]) -> str:
    state = "stated" if cause.get("stated", True) is not False else "unconfirmed"
    band = f'{cause["band"]} confidence, ' if cause.get("band") else ""
    relation = f' knock-on of cause {cause["knock_on_of"] + 1},' if cause.get("relation") == "knock_on" and isinstance(cause.get("knock_on_of"), int) else ""
    line = f'Cause {number} ({band}{state}):{relation} {_component(cause.get("component"))} ({cause.get("kind")}) - {cause.get("mechanism")}'
    trigger = cause.get("trigger")
    if isinstance(trigger, dict) and trigger.get("key"):
        line += f' Trigger: {trigger["key"]}' + (f'={trigger["value"]}' if trigger.get("value") else "")
    return line


def structured_lines(finding: dict[str, Any]) -> list[str]:
    lines = []
    causes = finding.get("causes")
    cause = finding.get("root_cause")
    if isinstance(causes, list) and causes:
        lines.extend(cause_line(number, item) for number, item in enumerate(causes, 1) if isinstance(item, dict))
    elif cause:
        lines.append(f'Root cause: {cause["component"]} ({cause["kind"]}) - {cause["summary"]}')
    if finding.get("no_incident"):
        lines.append("No incident found.")
    for label, key in (("Onset", "onset"), ("Impact", "impact"), ("Confidence", "confidence")):
        if finding.get(key):
            lines.append(f"{label}: {finding[key]}")
    for label, key in (("Ruled out", "ruled_out"), ("Next steps", "next_steps")):
        if finding.get(key):
            lines.append(f'{label}: {"; ".join(finding[key])}')
    return lines


def answer_text(request: dict[str, Any]) -> str:
    finding = request.get("finding")
    if finding:
        header = structured_lines(finding)
        return "\n".join([*header, "", finding["markdown"]]) if header else finding["markdown"]
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
    extra_usage: Usage | None = None,
) -> dict[str, Any]:
    usage = Usage.from_request(request)
    if extra_usage is not None:
        usage = usage.plus(extra_usage)
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
                **({"added_model_calls": extra_usage.model_calls} if extra_usage is not None else {}),
            },
        },
    }
