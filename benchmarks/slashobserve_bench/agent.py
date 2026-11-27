import asyncio
import dataclasses
import json
import tempfile
import time
from pathlib import Path

from harbor.agents.base import BaseAgent
from harbor.agents.capabilities import AgentCapabilities
from harbor.agents.options import AgentOptions
from harbor.environments.base import BaseEnvironment
from harbor.models.agent.context import AgentContext
from pydantic import Field, model_validator

from slashobserve_bench import atif, grafana, grafana_write, model_call, o11y, replay
from slashobserve_bench.api import ApiError, Client, RunTimeout, wait_for_terminal
from slashobserve_bench.ledger import SpendLedger
from slashobserve_bench.loop_options import LoopOptions
from slashobserve_bench.server import POSTGRES_IMAGE, ServerConfig, Stack, datasources_yaml, ensure_image

AGENT_NAME = "slashobserve"
AGENT_VERSION = "0.1.0"
LLM_KEY_FILE_ENV = "SLASHOBSERVE_BENCH_LLM_KEY_FILE"
CANCEL_GRACE_SEC = 30


class TaskSkipped(Exception):
    pass


class Options(AgentOptions, LoopOptions):
    server_image: str = "ghcr.io/slashobserve/slashobserve:latest"
    network: str = o11y.NETWORK
    replay_dir: str | None = None
    llm_effort: str | None = None
    run_max_cost_usd: float | None = Field(default=None, gt=0)
    spend_cap_usd: float | None = Field(default=None, gt=0)
    ledger_path: str | None = None
    wall_timeout_sec: int = Field(default=540, ge=30)
    grafana_url: str | None = None
    grafana_user: str | None = None
    grafana_password: str | None = None
    grafana_write_sec: int = Field(default=150, ge=30)

    @model_validator(mode="after")
    def _cap_needs_ledger(self) -> "Options":
        if (self.spend_cap_usd is None) != (self.ledger_path is None):
            raise ValueError("spend_cap_usd and ledger_path go together")
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


@dataclasses.dataclass
class DashboardTask:
    instruction: str
    as_of: str
    state: grafana.GrafanaState
    imported: dict[str, str]
    grafana: grafana.GrafanaClient
    provider: str
    model: str | None
    key: str | None = dataclasses.field(default=None, repr=False)


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
        grafana_state = None
        if self._category in o11y.GRAFANA_CATEGORIES:
            grafana_state = await grafana.read_state(self._grafana_client(task_env, environment))
        fixtures = None
        replay_dir = None
        if options.replay_dir:
            fixtures = tempfile.TemporaryDirectory(prefix="slashobserve-bench-fixtures-")
            replay_dir = Path(fixtures.name)
            replay.copy_fixtures(Path(options.replay_dir), replay_dir)
        config = ServerConfig(
            image=options.server_image,
            network=options.network,
            llm_provider=provider,
            llm_model=model,
            llm_effort=options.llm_effort,
            **options.server_settings(),
            run_max_cost_usd=options.run_max_cost_usd,
            replay_dir=replay_dir,
        )
        grafana_uids = grafana.grafana_uids_by_type(grafana_state.datasources) if grafana_state else None
        stack = Stack(config, datasources_yaml(stores, grafana_uids), self.logs_dir, self._llm_key(provider))
        try:
            await stack.start()
            prompt = instruction
            if grafana_state is not None:
                imported = await asyncio.to_thread(self._import_dashboards, Client(stack.base_url, stack.key), grafana_state)
                if self._category == o11y.DASHBOARDING:
                    task = DashboardTask(
                        instruction=instruction,
                        as_of=as_of,
                        state=grafana_state,
                        imported=imported,
                        grafana=self._grafana_client(task_env, environment),
                        provider=provider,
                        model=model,
                        key=stack.llm_key,
                    )
                    await self._dashboard_task(Client(stack.base_url, stack.key), task, context, deadline)
                    return
                prompt = grafana.question_prompt(instruction, grafana_state, imported)
                if replay_dir is not None:
                    replay.rekey(replay_dir, environment.environment_name, instruction, prompt)
            await self._answer(stack, prompt, as_of, environment.environment_name, context, deadline)
        finally:
            await asyncio.to_thread(stack.stop)
            if fixtures is not None:
                fixtures.cleanup()

    def _grafana_client(self, task_env: dict[str, str], environment: BaseEnvironment) -> grafana.GrafanaClient:
        options = self._options
        auth = grafana.grafana_auth(task_env, options.grafana_user, options.grafana_password)
        if options.grafana_url:
            return grafana.GrafanaClient(options.grafana_url, grafana.host_transport(auth))
        url = task_env.get(grafana.GRAFANA_URL_ENV)
        if not url:
            raise o11y.UnsupportedTask(f"the task environment sets no {grafana.GRAFANA_URL_ENV}")
        return grafana.GrafanaClient(url, grafana.container_transport(environment, auth))

    def _import_dashboards(self, client: Client, state: grafana.GrafanaState) -> dict[str, str]:
        imported: dict[str, str] = {}
        records = []
        for entry in state.dashboards:
            uid = entry["dashboard"].get("uid")
            record: dict = {"grafana_uid": uid, "title": entry["dashboard"].get("title")}
            try:
                answer = client.import_grafana(entry)
                record["dashboard_id"] = answer["dashboard"]["id"]
                record["version"] = answer["version"]["version"]
                imported[uid] = answer["dashboard"]["id"]
            except ApiError as error:
                record["error"] = str(error)
            records.append(record)
        self.logs_dir.mkdir(parents=True, exist_ok=True)
        (self._detail("grafana-imports.json")).write_text(json.dumps(records, indent=2) + "\n", encoding="utf-8")
        return imported

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
        await self._run_request(client, body, instruction, context, deadline)

    async def _run_request(
        self,
        client: Client,
        body: dict,
        instruction: str,
        context: AgentContext,
        deadline: float,
        earlier: atif.Usage | None = None,
    ) -> dict:
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
                self._save(request, instruction, context, earlier)
                raise
            self._save(request, instruction, context, earlier)
            return request
        finally:
            self._record_spend(seen.get("request"))

    async def _dashboard_task(self, client: Client, task: "DashboardTask", context: AgentContext, deadline: float) -> None:
        run_deadline = deadline - self._options.grafana_write_sec
        titles = {
            entry["dashboard"]["uid"]: entry["dashboard"].get("title") or ""
            for entry in task.state.dashboards
            if entry["dashboard"].get("uid") in task.imported
        }
        target = o11y.target_dashboard(task.instruction, titles)
        body = o11y.dashboard_body(task.instruction, task.imported.get(target) if target else None)
        request = await self._run_request(client, body, task.instruction, context, run_deadline)
        earlier = atif.Usage()
        if target is None:
            asked = ((request.get("finding") or {}).get("ask_on") or {}).get("dashboard_id")
            by_id = {dashboard_id: uid for uid, dashboard_id in task.imported.items()}
            if asked in by_id and not request.get("result_version"):
                (self._detail("run-build.json")).write_text(json.dumps(request, indent=2) + "\n", encoding="utf-8")
                earlier = atif.Usage.from_request(request)
                target = by_id[asked]
                body = o11y.dashboard_body(task.instruction, asked)
                request = await self._run_request(client, body, task.instruction, context, run_deadline, earlier)
        outcome: dict = {"target_grafana_uid": target, "dashboard_id": request.get("dashboard_id"), "result_version": request.get("result_version")}
        write = grafana_write.WriteBack()
        try:
            specs = await asyncio.to_thread(self._specs, client, request, task, target)
            if specs is None:
                outcome["skipped"] = "the run saved no dashboard"
            elif task.provider == "replay" or not task.key:
                outcome["skipped"] = "the replay model has no model to write Grafana JSON with"
            else:
                before, after = specs
                entry = task.state.dashboard_by_uid(target) if target else None
                await grafana_write.write_back(
                    call=self._model_caller(task, deadline),
                    client=task.grafana,
                    instruction=task.instruction,
                    datasources=task.state.datasources,
                    original=entry["dashboard"] if entry else None,
                    meta=entry.get("meta") if entry else None,
                    before_spec=before,
                    after_spec=after,
                    window=grafana_write.check_range(task.as_of),
                    state=write,
                    on_reply=self._write_call,
                )
        finally:
            outcome.update(write.outcome())
            (self._detail("grafana-write.json")).write_text(json.dumps(outcome, indent=2) + "\n", encoding="utf-8")
            ledger = self._ledger
            if ledger is not None and write.calls:
                ledger.record(self.logs_dir.parent.name, write.usage.cost_usd or 0.0)
            self._save(request, task.instruction, context, earlier.plus(write.usage))

    @staticmethod
    def _specs(client: Client, request: dict, task: "DashboardTask", target: str | None) -> tuple[dict | None, dict] | None:
        if target is not None:
            dashboard_id = task.imported[target]
            base = request.get("base_version") or 1
            before = client.dashboard_version(dashboard_id, base)["spec"]
            after_version = request.get("result_version")
            after = client.dashboard_version(dashboard_id, after_version)["spec"] if after_version else before
            return before, after
        if request.get("dashboard_id") and request.get("result_version"):
            return None, client.dashboard_version(request["dashboard_id"], request["result_version"])["spec"]
        return None

    @staticmethod
    def _model_caller(task: "DashboardTask", deadline: float) -> grafana_write.ModelCall:
        prices = model_call.load_prices()

        async def call(system: str, messages: list[dict[str, str]]) -> model_call.ModelReply:
            timeout = min(max(deadline - time.monotonic(), 20), 300)
            reply = await asyncio.to_thread(model_call.call_model, task.provider, task.model or "", task.key or "", system, messages, timeout)
            cost = model_call.cost_of(task.model or "", reply.usage, prices)
            return dataclasses.replace(reply, usage=dataclasses.replace(reply.usage, cost_usd=cost))

        return call

    def _write_call(self, number: int, reply: model_call.ModelReply) -> None:
        self.logs_dir.mkdir(parents=True, exist_ok=True)
        for name, content in (("request", reply.request), ("response", reply.response)):
            path = self._detail(f"grafana-call-{number}-{name}.json")
            path.write_text(json.dumps(content, indent=2) + "\n", encoding="utf-8")

    def _detail(self, name: str) -> Path:
        # o11y-bench's grader parses every *.json directly under the agent folder as a transcript.
        folder = self.logs_dir / atif.DETAIL_DIR
        folder.mkdir(parents=True, exist_ok=True)
        return folder / name

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

    def _save(self, request: dict, instruction: str, context: AgentContext, extra: atif.Usage | None = None) -> None:
        self.logs_dir.mkdir(parents=True, exist_ok=True)
        trajectory = atif.build_trajectory(
            request,
            instruction,
            agent_name=AGENT_NAME,
            agent_version=AGENT_VERSION,
            model_name=self.model_name,
            extra_usage=extra,
        )
        (self.logs_dir / "trajectory.json").write_text(json.dumps(trajectory, indent=2) + "\n", encoding="utf-8")
        (self._detail("run.json")).write_text(json.dumps(request, indent=2) + "\n", encoding="utf-8")
        usage = atif.Usage.from_request(request)
        if extra is not None:
            usage = usage.plus(extra)
        context.n_input_tokens = usage.prompt_tokens
        context.n_cache_tokens = usage.cache_read_tokens
        context.n_output_tokens = usage.output_tokens
        context.cost_usd = usage.cost_usd
