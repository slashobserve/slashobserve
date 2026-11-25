import asyncio
import json
import os
import subprocess
import tempfile
import time
from pathlib import Path

from harbor.agents.base import BaseAgent
from harbor.environments.base import BaseEnvironment
from harbor.models.agent.context import AgentContext

from slashobserve_bench import atif, orca
from slashobserve_bench.api import ApiError, Client, RunTimeout, wait_for_terminal
from slashobserve_bench.ledger import SpendLedger
from slashobserve_bench.server import POSTGRES_IMAGE, ServerConfig, Stack, ensure_image

AGENT_NAME = "slashobserve"
AGENT_VERSION = "0.1.0"
LLM_KEY_FILE_ENV = "SLASHOBSERVE_BENCH_LLM_KEY_FILE"
CANCEL_GRACE_SEC = 30
REPORT_STAGING = "/tmp/slashobserve-report.md"


class RunEndedWithoutFinding(Exception):
    pass


class ReportNotWritten(Exception):
    pass


def stdout_of(result, what: str) -> str:
    if result.return_code != 0:
        raise orca.UnsupportedTask(f"{what} failed with {result.return_code}: {(result.stderr or '').strip()}")
    return (result.stdout or "").strip()


def host_networks(container_id: str) -> list[str]:
    done = subprocess.run(
        ["docker", "inspect", container_id, "--format", "{{json .NetworkSettings.Networks}}"],
        capture_output=True,
        text=True,
        check=False,
    )
    if done.returncode != 0:
        raise orca.UnsupportedTask(f"docker inspect {container_id} failed: {done.stderr.strip()}")
    return list(json.loads(done.stdout))


def container_by_project(project: str) -> str:
    done = subprocess.run(
        [
            "docker",
            "ps",
            "-q",
            "--filter",
            f"label=com.docker.compose.project={project}",
            "--filter",
            "label=com.docker.compose.service=main",
        ],
        capture_output=True,
        text=True,
        check=False,
    )
    found = done.stdout.split()
    if done.returncode != 0 or len(found) != 1:
        raise orca.UnsupportedTask(f"expected one main container in compose project {project}: {done.stderr.strip() or found}")
    return found[0]


def copy_code_out(container_id: str, target: Path) -> Path:
    done = subprocess.run(
        ["docker", "cp", f"{container_id}:{orca.CODE_DIR}", str(target)], capture_output=True, text=True, check=False
    )
    if done.returncode != 0:
        raise orca.UnsupportedTask(f"docker cp of {orca.CODE_DIR} failed: {done.stderr.strip()}")
    return target / Path(orca.CODE_DIR).name


class OrcaAgent(BaseAgent):
    SUPPORTS_ATIF = True

    def __init__(self, logs_dir: Path, model_name: str | None = None, *args, **kwargs):
        harness = {name: kwargs.pop(name) for name in ("logger", "mcp_servers", "skills_dir", "extra_env") if name in kwargs}
        super().__init__(logs_dir, model_name, *args, **harness)
        self.options = orca.Options(**kwargs)

    @staticmethod
    def name() -> str:
        return AGENT_NAME

    def version(self) -> str | None:
        return AGENT_VERSION

    @property
    def _ledger(self) -> SpendLedger | None:
        options = self.options
        if options.spend_cap_usd is None or options.ledger_path is None:
            return None
        return SpendLedger(Path(options.ledger_path), options.spend_cap_usd)

    def _provider_and_model(self) -> tuple[str, str | None]:
        provider = self._parsed_model_provider
        if not provider or not self._parsed_model_name:
            raise ValueError("pass the model as <provider>/<model>, for example -m anthropic/claude-sonnet-5-5 or -m replay/replay")
        return provider, None if provider == "replay" else self._parsed_model_name

    def _llm_key(self, provider: str) -> str | None:
        if provider == "replay":
            return None
        path = self.extra_env.get(LLM_KEY_FILE_ENV) or os.environ.get(LLM_KEY_FILE_ENV)
        if not path:
            raise ValueError(f"set {LLM_KEY_FILE_ENV} in the environment or with --ae to the path of a file that holds the model API key")
        return Path(path).read_text(encoding="utf-8").strip()

    async def setup(self, environment: BaseEnvironment) -> None:
        self._provider_and_model()
        if self.options.code_context:
            orca.load_code_context(self.options.code_context)
        ledger = self._ledger
        if ledger is not None:
            await asyncio.to_thread(ledger.ensure_room)
        for image in (self.options.server_image, POSTGRES_IMAGE):
            await asyncio.to_thread(ensure_image, image)

    async def run(self, instruction: str, environment: BaseEnvironment, context: AgentContext) -> None:
        options = self.options
        deadline = time.monotonic() + options.wall_timeout_sec
        ledger = self._ledger
        if ledger is not None:
            await asyncio.to_thread(ledger.ensure_room)
        task_toml = (environment.environment_dir.parent / "task.toml").read_text(encoding="utf-8")
        as_of = orca.as_of(task_toml, instruction)
        wait = max(int(deadline - time.monotonic()), 1)
        ready = await environment.exec(
            f"until [ -f {orca.ENV_READY} ]; do sleep 2; done; cat {orca.ENV_PORTS}", timeout_sec=wait
        )
        endpoints = orca.Endpoints.from_ports_file(stdout_of(ready, "waiting for the trial's services"))
        container_id, network = await self._locate(environment)
        provider, model = self._provider_and_model()
        config = ServerConfig(
            image=options.server_image,
            network=network,
            llm_provider=provider,
            llm_model=model,
            llm_effort=options.llm_effort,
            investigation_calls=options.investigation_calls,
            run_max_cost_usd=options.run_max_cost_usd,
            replay_dir=Path(options.replay_dir) if options.replay_dir else None,
        )
        stack = Stack(config, orca.datasources_yaml(endpoints), self.logs_dir, self._llm_key(provider))
        notes = [
            "traces: read through the OpenSearch datasource, because the trial's Jaeger is older than the 2.21 the Jaeger datasource needs",
            "scores: judged outside the published ORCA-bench judge, so not comparable with leaderboard numbers",
        ]
        workdir = tempfile.TemporaryDirectory(prefix="orca-code-")
        try:
            await stack.start()
            client = Client(stack.base_url, stack.key)
            notes.append(await self._code_context(client, network, container_id, Path(workdir.name)))
            orca.write_notes(self.logs_dir, notes)
            request = await self._investigate(client, orca.question(instruction, as_of), as_of, deadline, instruction, context)
            await self._finish(request, instruction, context, environment)
        finally:
            await asyncio.to_thread(stack.stop)
            workdir.cleanup()

    async def _locate(self, environment: BaseEnvironment) -> tuple[str, str]:
        container_id = stdout_of(await environment.exec("cat /etc/hostname"), "reading the main container's hostname")
        try:
            if not container_id:
                raise orca.UnsupportedTask("the main container reports no hostname")
            return container_id, orca.otel_network(await asyncio.to_thread(host_networks, container_id))
        except orca.UnsupportedTask:
            project = orca.compose_project(environment.session_id)
            container_id = await asyncio.to_thread(container_by_project, project)
            return container_id, orca.otel_network(await asyncio.to_thread(host_networks, container_id))

    async def _code_context(self, client: Client, network: str, container_id: str, workdir: Path) -> str:
        if not self.options.code_context:
            return "code context: skipped, no code source is configured (--ak code_context=<module>:<function>)"
        hook = orca.load_code_context(self.options.code_context)
        code_dir = await asyncio.to_thread(copy_code_out, container_id, workdir)
        await asyncio.to_thread(hook, client, network, code_dir)
        return f"code context: registered through {self.options.code_context}"

    async def _investigate(
        self, client: Client, question: str, as_of: str, deadline: float, instruction: str, context: AgentContext
    ) -> dict:
        run_id = await asyncio.to_thread(client.start_run, orca.start_body(question, as_of))
        seen: dict[str, dict] = {}

        def poll() -> dict:
            seen["request"] = client.get_run(run_id)
            return seen["request"]

        try:
            try:
                return await asyncio.to_thread(wait_for_terminal, poll, max(deadline - time.monotonic(), 0))
            except RunTimeout:
                await asyncio.to_thread(self._cancel, client, run_id)
                try:
                    request = await asyncio.to_thread(wait_for_terminal, poll, CANCEL_GRACE_SEC)
                except RunTimeout:
                    request = seen["request"]
                self._save(request, instruction, context)
                raise
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
        (self.logs_dir / "run.json").write_text(json.dumps(request, indent=2) + "\n", encoding="utf-8")
        trajectory = atif.build_trajectory(
            request, instruction, agent_name=AGENT_NAME, agent_version=AGENT_VERSION, model_name=self.model_name
        )
        (self.logs_dir / "trajectory.json").write_text(json.dumps(trajectory, indent=2) + "\n", encoding="utf-8")
        usage = atif.Usage.from_request(request)
        context.n_input_tokens = usage.prompt_tokens
        context.n_cache_tokens = usage.cache_read_tokens
        context.n_output_tokens = usage.output_tokens
        context.cost_usd = usage.cost_usd

    async def _finish(
        self, request: dict, instruction: str, context: AgentContext, environment: BaseEnvironment
    ) -> None:
        self._save(request, instruction, context)
        report = orca.report_for(request)
        if report is None:
            state = request.get("failure_code") or request.get("state")
            raise RunEndedWithoutFinding(f"the run ended {state} with no finding, so no report was written")
        local = self.logs_dir / "report.md"
        local.write_text(report, encoding="utf-8")
        await environment.upload_file(local, REPORT_STAGING)
        check = "test -s" if report else "test -f"
        written = await environment.exec(f"cp {REPORT_STAGING} {orca.REPORT_PATH} && {check} {orca.REPORT_PATH}")
        if written.return_code != 0:
            raise ReportNotWritten(f"writing {orca.REPORT_PATH} failed with {written.return_code}: {(written.stderr or '').strip()}")
