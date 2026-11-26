import asyncio
import json
import time
from pathlib import Path
from typing import Literal

from harbor.agents.base import BaseAgent
from harbor.agents.capabilities import AgentCapabilities
from harbor.agents.options import AgentOptions
from harbor.environments.base import BaseEnvironment
from harbor.models.agent.context import AgentContext
from pydantic import Field, model_validator

from slashobserve_bench import atif, o11y
from slashobserve_bench.api import ApiError, Client, RunTimeout, wait_for_terminal
from slashobserve_bench.ledger import SpendLedger
from slashobserve_bench.server import POSTGRES_IMAGE, ServerConfig, Stack, datasources_yaml, ensure_image

AGENT_NAME = "slashobserve"
AGENT_VERSION = "0.1.0"
LLM_KEY_FILE_ENV = "SLASHOBSERVE_BENCH_LLM_KEY_FILE"
CANCEL_GRACE_SEC = 30


class TaskSkipped(Exception):
    pass


class Options(AgentOptions):
    server_image: str = "ghcr.io/slashobserve/slashobserve:latest"
    network: str = o11y.NETWORK
    replay_dir: str | None = None
    llm_effort: str | None = None
    investigation_calls: int | None = Field(default=None, ge=6, le=60)
    investigation_loop: bool | None = None
    investigation_calls_max: int | None = Field(default=None, ge=6, le=60)
    finding_grounding: Literal["off", "report", "enforce"] | None = None
    run_max_cost_usd: float | None = Field(default=None, gt=0)
    spend_cap_usd: float | None = Field(default=None, gt=0)
    ledger_path: str | None = None
    wall_timeout_sec: int = Field(default=540, ge=30)

    @model_validator(mode="after")
    def _settings_agree(self) -> "Options":
        if (self.spend_cap_usd is None) != (self.ledger_path is None):
            raise ValueError("spend_cap_usd and ledger_path go together")
        if None not in (self.investigation_calls, self.investigation_calls_max) and self.investigation_calls_max < self.investigation_calls:
            raise ValueError("investigation_calls_max must not be below investigation_calls")
        return self


def split_model(model_name: str | None) -> tuple[str, str | None]:
    if not model_name or "/" not in model_name:
        raise ValueError("pass the model as <provider>/<model>, for example -m anthropic/claude-haiku-5-5 or -m replay/replay")
    provider, _, model = model_name.partition("/")
    return provider, None if provider == "replay" else model


def read_environment(output: str) -> dict[str, str]:
    values = {}
    for line in output.splitlines():
        name, separator, value = line.partition("=")
        if separator:
            values[name] = value
    return values


class SlashObserveAgent(BaseAgent):
    capabilities = AgentCapabilities(atif=True, mcp_servers=True)
    options_model = Options
    DEFAULT_SETUP_TIMEOUT_SEC = 900.0
    _category: str

    @staticmethod
    def name() -> str:
        return AGENT_NAME

    def version(self) -> str | None:
        return AGENT_VERSION

    @property
    def _options(self) -> Options:
        assert isinstance(self.options, Options)
        return self.options

    @property
    def _ledger(self) -> SpendLedger | None:
        options = self._options
        if options.spend_cap_usd is None or options.ledger_path is None:
            return None
        return SpendLedger(Path(options.ledger_path), options.spend_cap_usd)

    async def setup(self, environment: BaseEnvironment) -> None:
        task_id = environment.environment_name
        reason = o11y.skip_reason(task_id)
        if reason is not None:
            self.logs_dir.mkdir(parents=True, exist_ok=True)
            (self.logs_dir / "skipped.txt").write_text(f"skipped: {reason}\n", encoding="utf-8")
            raise TaskSkipped(f"{task_id} is skipped: {reason}")
        split_model(self.model_name)
        task_toml = environment.environment_dir.parent / "task.toml"
        self._category = o11y.category_of(task_toml.read_text(encoding="utf-8"))
        o11y.run_kind(self._category)
        ledger = self._ledger
        if ledger is not None:
            await asyncio.to_thread(ledger.ensure_room)
        for image in (self._options.server_image, POSTGRES_IMAGE):
            await asyncio.to_thread(ensure_image, image)

    async def run(self, instruction: str, environment: BaseEnvironment, context: AgentContext) -> None:
        options = self._options
        deadline = time.monotonic() + options.wall_timeout_sec
        ledger = self._ledger
        if ledger is not None:
            await asyncio.to_thread(ledger.ensure_room)
        task_env = read_environment((await environment.exec("printenv")).stdout or "")
        stores = o11y.Stores.from_env(task_env)
        as_of = task_env.get(o11y.SCENARIO_TIME_ENV, "")
        if not as_of:
            raise o11y.UnsupportedTask(f"the task environment sets no {o11y.SCENARIO_TIME_ENV}")
        provider, model = split_model(self.model_name)
        config = ServerConfig(
            image=options.server_image,
            network=options.network,
            llm_provider=provider,
            llm_model=model,
            llm_effort=options.llm_effort,
            investigation_calls=options.investigation_calls,
            investigation_loop=options.investigation_loop,
            investigation_calls_max=options.investigation_calls_max,
            finding_grounding=options.finding_grounding,
            run_max_cost_usd=options.run_max_cost_usd,
            replay_dir=Path(options.replay_dir) if options.replay_dir else None,
        )
        stack = Stack(config, datasources_yaml(stores), self.logs_dir, self._llm_key(provider))
        try:
            await stack.start()
            await self._answer(stack, instruction, as_of, environment.environment_name, context, deadline)
        finally:
            await asyncio.to_thread(stack.stop)

    def _llm_key(self, provider: str) -> str | None:
        if provider == "replay":
            return None
        path = self._get_env(LLM_KEY_FILE_ENV)
        if not path:
            raise ValueError(f"set {LLM_KEY_FILE_ENV} to the path of a file that holds the model API key")
        return Path(path).read_text(encoding="utf-8").strip()

    async def _answer(
        self,
        stack: Stack,
        instruction: str,
        as_of: str,
        task_id: str,
        context: AgentContext,
        deadline: float,
    ) -> None:
        client = Client(stack.base_url, stack.key)
        observation_id = None
        if o11y.run_kind(self._category) == "ask":
            observation_id = await asyncio.to_thread(client.create_observation, f"o11y-bench {task_id}")
        body = o11y.start_body(self._category, instruction, as_of, observation_id)
        run_id = await asyncio.to_thread(client.start_run, body)
        seen: dict[str, dict] = {}

        def poll() -> dict:
            seen["request"] = client.get_run(run_id)
            return seen["request"]

        try:
            try:
                request = await asyncio.to_thread(wait_for_terminal, poll, max(deadline - time.monotonic(), 0))
            except RunTimeout:
                await asyncio.to_thread(self._cancel, client, run_id)
                try:
                    request = await asyncio.to_thread(wait_for_terminal, poll, CANCEL_GRACE_SEC)
                except RunTimeout:
                    request = seen["request"]
                self._save(request, instruction, context)
                raise
            self._save(request, instruction, context)
        finally:
            self._record_spend(seen.get("request"))

    def _record_spend(self, request: dict | None) -> None:
        ledger = self._ledger
        if ledger is None or request is None:
            return
        ledger.record(self.logs_dir.parent.name, atif.Usage.from_request(request).cost_usd or 0.0)

    @staticmethod
    def _cancel(client: Client, run_id: str) -> None:
        try:
            client.cancel_run(run_id)
        except ApiError:
            pass

    def _save(self, request: dict, instruction: str, context: AgentContext) -> None:
        self.logs_dir.mkdir(parents=True, exist_ok=True)
        trajectory = atif.build_trajectory(
            request,
            instruction,
            agent_name=AGENT_NAME,
            agent_version=AGENT_VERSION,
            model_name=self.model_name,
        )
        (self.logs_dir / "trajectory.json").write_text(json.dumps(trajectory, indent=2) + "\n", encoding="utf-8")
        (self.logs_dir / "run.json").write_text(json.dumps(request, indent=2) + "\n", encoding="utf-8")
        usage = atif.Usage.from_request(request)
        context.n_input_tokens = usage.prompt_tokens
        context.n_cache_tokens = usage.cache_read_tokens
        context.n_output_tokens = usage.output_tokens
        context.cost_usd = usage.cost_usd
