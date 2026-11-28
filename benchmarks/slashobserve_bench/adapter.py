import json
from collections.abc import Callable, Iterable
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from pydantic import Field, model_validator

from slashobserve_bench import atif
from slashobserve_bench.api import ApiError, Client, RunTimeout, wait_for_terminal
from slashobserve_bench.ledger import SpendLedger
from slashobserve_bench.loop_options import LoopOptions
from slashobserve_bench.server import CONFIG_DIR

AGENT_NAME = "slashobserve"
AGENT_VERSION = "0.1.0"
LLM_KEY_FILE_ENV = "SLASHOBSERVE_BENCH_LLM_KEY_FILE"
CANCEL_GRACE_SEC = 30
MCP_SOURCES_SETTING = "SLASHOBSERVE_MCP_SOURCES_FILE"
MCP_SOURCES_FILE = "mcp-sources.yaml"


class RunOptions(LoopOptions):
    replay_dir: str | None = None
    llm_effort: str | None = None
    run_max_cost_usd: float | None = Field(default=None, gt=0)
    spend_cap_usd: float | None = Field(default=None, gt=0)
    ledger_path: str | None = None

    @model_validator(mode="after")
    def _cap_needs_ledger(self) -> "RunOptions":
        if (self.spend_cap_usd is None) != (self.ledger_path is None):
            raise ValueError("spend_cap_usd and ledger_path go together")
        return self

    def spend_ledger(self) -> SpendLedger | None:
        if self.spend_cap_usd is None or self.ledger_path is None:
            return None
        return SpendLedger(Path(self.ledger_path), self.spend_cap_usd)


def split_model(model_name: str | None, flag: str = "-m") -> tuple[str, str | None]:
    if not model_name or "/" not in model_name:
        raise ValueError(f"pass the model as <provider>/<model>, for example {flag} anthropic/claude-sonnet-5-5 or {flag} replay/replay")
    provider, _, model = model_name.partition("/")
    return provider, None if provider == "replay" else model


def llm_key(provider: str, path: str | None, where: str = "") -> str | None:
    if provider == "replay":
        return None
    if not path:
        raise ValueError(f"set {LLM_KEY_FILE_ENV}{where} to the path of a file that holds the model API key")
    return Path(path).read_text(encoding="utf-8").strip()


def record_spend(ledger: SpendLedger | None, trial: str, request: dict[str, Any] | None) -> None:
    if ledger is not None and request is not None:
        ledger.record(trial, atif.Usage.from_request(request).cost_usd or 0.0)


def run_to_end(
    client: Client,
    body: dict[str, Any],
    budget_sec: float,
    record: Callable[[dict[str, Any]], None] = lambda request: None,
    grace_sec: float = CANCEL_GRACE_SEC,
) -> tuple[dict[str, Any], RunTimeout | None]:
    run_id = client.start_run(body)
    seen: dict[str, dict[str, Any]] = {}

    def poll() -> dict[str, Any]:
        seen["request"] = client.get_run(run_id)
        return seen["request"]

    try:
        try:
            return wait_for_terminal(poll, max(budget_sec, 0)), None
        except RunTimeout as timeout:
            try:
                client.cancel_run(run_id)
            except ApiError:
                pass
            try:
                return wait_for_terminal(poll, grace_sec), timeout
            except RunTimeout:
                return seen["request"], timeout
    finally:
        if "request" in seen:
            record(seen["request"])


def mcp_sources_yaml(uid: str, name: str, description: str, url: str, approve: Iterable[str]) -> str:
    return "\n".join(
        [
            "mcpSources:",
            f"  - uid: {uid}",
            f"    name: {name}",
            f"    description: {description}",
            f"    url: {json.dumps(url)}",
            "    audience: everyone",
            "    allowPrivate: true",
            "    approve: [" + ", ".join(json.dumps(tool) for tool in approve) + "]",
            "",
        ]
    )


def mcp_source_settings(host: str) -> tuple[tuple[str, str], ...]:
    return ((MCP_SOURCES_SETTING, f"{CONFIG_DIR}/{MCP_SOURCES_FILE}"), ("SLASHOBSERVE_OUTBOUND_DOMAINS", host))


def knock_on_of(entry: dict[str, Any]) -> int | None:
    index = entry.get("knock_on_of")
    if entry.get("relation") == "independent" or not isinstance(index, int) or isinstance(index, bool):
        return None
    return index


@dataclass(frozen=True)
class CauseEntries:
    roots: tuple[tuple[int, dict[str, Any]], ...]
    knock_ons: tuple[tuple[int, dict[str, Any], int], ...]
    unconfirmed: tuple[tuple[int, dict[str, Any]], ...]


def read_causes(entries: list[Any]) -> CauseEntries:
    found = [(index, entry) for index, entry in enumerate(entries) if isinstance(entry, dict)]
    stated = [(index, entry) for index, entry in found if entry.get("stated", True) is not False]
    roots = {index: entry for index, entry in stated if knock_on_of(entry) is None}
    knock_ons = tuple((index, entry, parent) for index, entry in stated if (parent := knock_on_of(entry)) in roots)
    placed = {*roots, *(index for index, _, _ in knock_ons)}
    unconfirmed = tuple((index, entry) for index, entry in found if index not in placed)
    return CauseEntries(tuple(roots.items()), knock_ons, unconfirmed)
