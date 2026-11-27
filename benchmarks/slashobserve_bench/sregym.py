import argparse
import asyncio
import json
import os
import re
import secrets
import shlex
import socket
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request
from collections.abc import Callable, Iterable
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from pydantic import ConfigDict, Field, model_validator

from slashobserve_bench import atif
from slashobserve_bench.api import ApiError, Client, RunTimeout, wait_for_terminal
from slashobserve_bench.ledger import SpendLedger
from slashobserve_bench.loop_options import LoopOptions
from slashobserve_bench.server import LABEL, POSTGRES_IMAGE, ServerConfig, Stack, ensure_image

AGENT_NAME = "slashobserve"
AGENT_VERSION = "0.1.0"
LLM_KEY_FILE_ENV = "SLASHOBSERVE_BENCH_LLM_KEY_FILE"
PROMPT_LIMIT = 4000
DIAGNOSIS = "diagnosis"
WAITING_STAGES = ("setup", "tearing_down", None)

HOST_ALIAS = "host.docker.internal"
OBSERVE_NAMESPACE = "observe"
PROMETHEUS_SERVICE = ("prometheus-server", 80)
LOKI_SERVICE = ("loki", 3100)
DEFAULT_MCP_PORT = 9954
KUBECTL_PATH = "/kubectl/sse"
KUBECTL_SOURCE = "sregym-kubectl"
KUBECTL_TOOL = "exec_kubectl_cmd_safely"
CONFIG_DIR = "/etc/slashobserve"
MCP_SOURCES_FILE = "mcp-sources.yaml"
IMAGE_BRANCH = "agent-rca-accuracy"

CANCEL_GRACE_SEC = 30
SUBMIT_TIMEOUT_SEC = 330
SUBMIT_MARGIN_SEC = 360
STAGE_WAIT_SEC = 120
REPLAY_TEMPLATE = Path(__file__).resolve().parent.parent / "fixtures" / "sregym-answer.json"
NO_DIAGNOSIS = "The investigation produced no diagnosis."
NO_FAULT = "The investigation found no faulty component."

SUBSET: tuple[str, ...] = (
    "edge_request_filter_cpu_saturation",
    "kafka_poison_pill_hol_block",
    "valkey_auth_disruption",
    "secret_rotation_stale_env_credentials_astronomy_shop",
    "search_rate_retry_collapse_hotel_reservation",
    "env_variable_shadowing_astronomy_shop",
    "internal_traffic_policy_local_astronomy_shop",
    "network_policy_block",
    "service_dns_resolution_failure_social_network",
    "service_wrong_pod_selection_hotel_reservation",
)

COMPONENT = re.compile(
    r"^\s*(?:(?P<namespace>[a-z0-9][a-z0-9-]*)/)?(?P<kind>[A-Z][A-Za-z]+)/(?P<name>[A-Za-z0-9][A-Za-z0-9._-]*)\s*$"
)

TELEMETRY_STEP = "Start from the telemetry: the datasource `prometheus` holds metrics and `loki` holds logs. There is no trace datasource. Find the symptom first: failed or slow requests, error logs, queue lag."
KUBECTL_STEPS = (
    "Read the cluster through the MCP source `sregym-kubectl` and its tool `exec_kubectl_cmd_safely`. Every command starts with `kubectl` and only reads: get, describe, logs, top and get events. Never run apply, create, delete, edit, patch, replace, scale, rollout, set, label, annotate, exec, cp or any other command that changes the cluster.",
    "When telemetry shows only a symptom, look in the cluster for the change behind it: pods not Ready, restarts, recent events, Service selectors against pod labels and endpoints, internalTrafficPolicy, NetworkPolicies, ResourceQuotas and LimitRanges, admission webhooks, HPAs, and the env vars, Secrets, ConfigMaps and resource limits of running pods that differ from their workload spec.",
)
NO_KUBECTL_STEP = "The cluster's Kubernetes objects cannot be read in this run, so work from telemetry alone."
ORIGIN_STEP = "Follow the failing path to the first component whose own configuration or state is wrong, and check that configuration before you conclude. A component that only suffers from another one is not the origin."
ANSWER_STEPS = (
    "Fill causes with one entry per origin. Its component has the kind (deployment, service, pod, config or other), the name exactly as the cluster spells it, written Kind/name for any other Kubernetes kind such as NetworkPolicy/deny-all, and the namespace. Its mechanism says what is wrong with the concrete value, and its trigger names the key that changed (the selector, port, policy, env var, limit or secret) with its wrong value.",
    "List a component that only suffers from an origin under that cause's affected, or as a cause with knock_on_of set to the origin's index, never as an origin. Set impact to the affected components by name. Name no other component as a cause; put what you checked and excluded under ruled_out only.",
)


def playbook(kubectl: bool = True) -> str:
    steps = (TELEMETRY_STEP, *(KUBECTL_STEPS if kubectl else (NO_KUBECTL_STEP,)), ORIGIN_STEP)
    work = "\n".join(f"- {step}" for step in steps)
    answer = "\n".join(f"- {step}" for step in ANSWER_STEPS)
    return f"How to work:\n{work}\n\nHow to answer:\n{answer}"


class UnsupportedProblem(Exception):
    pass


class ConductorError(Exception):
    pass


@dataclass(frozen=True)
class App:
    name: str
    namespace: str
    namespaces: tuple[str, ...]
    description: str
    stages: tuple[str, ...]

    @classmethod
    def from_get_app(cls, data: Any) -> "App":
        if not isinstance(data, dict):
            raise ConductorError(f"GET /get_app answered {type(data).__name__}, not an object")
        name, namespace = data.get("app_name"), data.get("namespace")
        if not isinstance(name, str) or not name.strip() or not isinstance(namespace, str) or not namespace.strip():
            raise ConductorError("GET /get_app gave no app_name or namespace")
        listed = [item for item in data.get("namespaces") or () if isinstance(item, str) and item.strip()]
        namespaces = tuple(dict.fromkeys([namespace.strip(), *(item.strip() for item in listed)]))
        description = data.get("descriptions", data.get("description"))
        stages = tuple(str(stage) for stage in data.get("stages") or ())
        return cls(name.strip(), namespace.strip(), namespaces, "" if description is None else str(description).strip(), stages)


@dataclass(frozen=True)
class Status:
    stage: str | None
    remaining_sec: float | None

    @classmethod
    def from_status(cls, data: Any) -> "Status":
        if not isinstance(data, dict):
            raise ConductorError(f"GET /status answered {type(data).__name__}, not an object")
        remaining = data.get("agent_remaining_seconds")
        return cls(data.get("stage"), float(remaining) if isinstance(remaining, int | float) else None)


def conductor_url(env: dict[str, str]) -> str:
    host = env.get("API_HOSTNAME") or "127.0.0.1"
    if host in ("0.0.0.0", "::"):
        host = "127.0.0.1"
    return f"http://{host}:{env.get('API_PORT') or '8000'}"


def mcp_url(env: dict[str, str], host: str = HOST_ALIAS) -> str:
    return f"http://{host}:{env.get('MCP_SERVER_PORT') or DEFAULT_MCP_PORT}{KUBECTL_PATH}"


def host_of(url: str) -> str:
    return url.split("://", 1)[-1].split("/", 1)[0].rsplit(":", 1)[0]


def submit_body(solution: str) -> dict[str, str]:
    return {"solution": solution, "stage": DIAGNOSIS}


class Conductor:
    def __init__(self, base_url: str):
        self.base_url = base_url.rstrip("/")

    def _call(self, method: str, path: str, body: dict[str, Any] | None = None, timeout: float = 30) -> Any:
        data = None if body is None else json.dumps(body).encode()
        request = urllib.request.Request(f"{self.base_url}{path}", data=data, method=method)
        if data is not None:
            request.add_header("Content-Type", "application/json")
        try:
            with urllib.request.urlopen(request, timeout=timeout) as response:
                return json.loads(response.read() or b"{}")
        except urllib.error.HTTPError as error:
            raise ConductorError(f"{method} {path} answered {error.code}: {error.read().decode(errors='replace')[:500]}") from None
        except (urllib.error.URLError, OSError) as error:
            raise ConductorError(f"{method} {path} failed: {error}") from None

    def app(self) -> App:
        return App.from_get_app(self._call("GET", "/get_app"))

    def status(self) -> Status:
        return Status.from_status(self._call("GET", "/status"))

    def submit_diagnosis(self, solution: str) -> Any:
        return self._call("POST", "/submit", submit_body(solution), timeout=SUBMIT_TIMEOUT_SEC)


def _yaml_list(values: Iterable[str]) -> str:
    return "[" + ", ".join(json.dumps(value) for value in values) + "]"


def datasources_yaml(prometheus: str, loki: str) -> str:
    lines = ["datasources:"]
    for uid, name, url in (("prometheus", "Prometheus", prometheus), ("loki", "Loki", loki)):
        lines += [f"  - uid: {uid}", f"    type: {uid}", f"    name: {name}", f"    {uid}:", f"      url: {json.dumps(url)}"]
    return "\n".join(lines) + "\n"


def mcp_sources_yaml(url: str) -> str:
    return "\n".join(
        [
            "mcpSources:",
            f"  - uid: {KUBECTL_SOURCE}",
            "    name: SREGym kubectl",
            "    description: Read the benchmark cluster's Kubernetes objects, events and pod logs with kubectl.",
            f"    url: {json.dumps(url)}",
            "    audience: everyone",
            "    allowPrivate: true",
            f"    approve: {_yaml_list([KUBECTL_TOOL])}",
            "",
        ]
    )


def server_settings(source_url: str) -> tuple[tuple[str, str], ...]:
    return (
        ("SLASHOBSERVE_MCP_SOURCES_FILE", f"{CONFIG_DIR}/{MCP_SOURCES_FILE}"),
        ("SLASHOBSERVE_OUTBOUND_DOMAINS", host_of(source_url)),
    )


def _clip(text: str, room: int) -> str:
    if len(text) <= room:
        return text
    return text[: max(room - 1, 0)].rstrip() + "…"


def question(app: App, kubectl: bool = True, limit: int = PROMPT_LIMIT) -> str:
    where = "namespace " + app.namespaces[0] if len(app.namespaces) == 1 else "namespaces " + ", ".join(app.namespaces)
    head = f"Users of the application {app.name}, which runs on Kubernetes in {where}, see it misbehave. Find the root cause."
    tail = "\n\n" + playbook(kubectl)
    if not app.description:
        text = head + tail
    else:
        label = "\n\nAbout the application: "
        room = limit - len(head) - len(label) - len(tail)
        if room < 40:
            raise UnsupportedProblem(f"the app name and namespaces leave no room for the question within {limit} characters")
        text = head + label + _clip(app.description, room) + tail
    if len(text) > limit:
        raise UnsupportedProblem(f"the question is {len(text)} characters, over the server's {limit}")
    return text


def replay_fixtures(template: dict[str, Any], app: App) -> dict[str, dict[str, Any]]:
    return {
        f"sregym-{'kubectl' if kubectl else 'telemetry'}.json": {**template, "prompt": question(app, kubectl)}
        for kubectl in (True, False)
    }


def start_body(question_text: str) -> dict[str, Any]:
    return {"kind": "investigate", "prompt": question_text}


def component_text(value: Any) -> str | None:
    if isinstance(value, dict):
        kind, name, namespace = (str(value.get(key) or "").strip() for key in ("kind", "name", "namespace"))
        if not name:
            return None
        if (found := COMPONENT.match(name)) is not None:
            kind, name, namespace = found["kind"], found["name"], namespace or found["namespace"] or ""
        text = f"{kind} {name}" if kind and kind != "other" else name
        return f"{text} in namespace {namespace}" if namespace else text
    if not isinstance(value, str) or not value.strip():
        return None
    found = COMPONENT.match(value)
    if found is None:
        return value.strip()
    text = f"{found['kind']} {found['name']}"
    return f"{text} in namespace {found['namespace']}" if found["namespace"] else text


def _trigger_text(value: Any) -> str | None:
    if isinstance(value, dict):
        key, item = (str(value.get(name) or "").strip() for name in ("key", "value"))
        if not key:
            return None
        return f"{key} set to {item}" if item else key
    if isinstance(value, str) and value.strip():
        return value.strip()
    return None


def _sentence(text: str) -> str:
    text = text.strip()
    return text if text.endswith((".", "!", "?")) else text + "."


@dataclass(frozen=True)
class Cause:
    origin: str | None
    mechanism: str | None
    trigger: str | None
    affected: tuple[str, ...]

    @classmethod
    def from_entry(cls, entry: dict[str, Any], followers: Iterable[dict[str, Any]] = ()) -> "Cause":
        origin = component_text(entry.get("component"))
        scope = [*(entry.get("affected") or ()), *(part for follower in followers for part in (follower.get("component"), *(follower.get("affected") or ())))]
        affected = [text for text in dict.fromkeys(component_text(item) for item in scope if item) if text and text != origin]
        mechanism = entry.get("mechanism") or entry.get("summary")
        return cls(
            origin=origin,
            mechanism=mechanism.strip() if isinstance(mechanism, str) and mechanism.strip() else None,
            trigger=_trigger_text(entry.get("trigger")),
            affected=tuple(affected),
        )

    def lines(self) -> list[str]:
        lines = []
        if self.origin:
            lines.append(f"Origin: {_sentence(self.origin)}")
        if self.mechanism:
            lines.append(f"Mechanism: {_sentence(self.mechanism)}")
        if self.trigger:
            lines.append(f"Changed value: {_sentence(self.trigger)}")
        if self.affected:
            lines.append(f"Affected: {_sentence(', '.join(self.affected))}")
        return lines


def _knock_on_of(entry: dict[str, Any]) -> int | None:
    index = entry.get("knock_on_of")
    if entry.get("relation") == "independent" or not isinstance(index, int) or isinstance(index, bool):
        return None
    return index


def _from_causes(entries: list[Any]) -> list[Cause]:
    stated = {index: entry for index, entry in enumerate(entries) if isinstance(entry, dict) and entry.get("stated", True) is not False}
    origins = {index: entry for index, entry in stated.items() if _knock_on_of(entry) is None}
    followers: dict[int, list[dict[str, Any]]] = {index: [] for index in origins}
    for entry in stated.values():
        if (parent := _knock_on_of(entry)) in followers:
            followers[parent].append(entry)
    return [Cause.from_entry(entry, followers[index]) for index, entry in origins.items()]


def stated_causes(finding: dict[str, Any]) -> list[Cause]:
    entries = finding.get("causes")
    if isinstance(entries, list) and entries:
        causes = _from_causes(entries)
    elif isinstance(finding.get("root_cause"), dict):
        causes = [Cause.from_entry(finding["root_cause"])]
    else:
        causes = []
    return [cause for cause in causes if cause.origin or cause.mechanism]


def render_diagnosis(finding: dict[str, Any] | None) -> str:
    if not finding:
        return NO_DIAGNOSIS
    if finding.get("no_incident"):
        return NO_FAULT
    causes = stated_causes(finding)
    if not causes:
        return (finding.get("markdown") or "").strip() or NO_DIAGNOSIS
    blocks = []
    for number, cause in enumerate(causes, 1):
        lines = cause.lines()
        if len(causes) > 1:
            lines = [f"Root cause {number}:", *lines]
        blocks.append("\n".join(lines))
    impact = finding.get("impact")
    if isinstance(impact, str) and impact.strip():
        blocks.append(f"Impact: {_sentence(impact)}")
    return "\n\n".join(blocks)


def diagnosis_for(request: dict[str, Any]) -> str:
    return render_diagnosis(request.get("finding"))


@dataclass(frozen=True)
class KubectlTool:
    state: str
    refusal: str | None = None
    read_only: bool | None = None

    @property
    def approved(self) -> bool:
        return self.state == "approved"

    def note(self) -> str:
        mark = {
            True: "marked read-only by its server",
            False: "not marked read-only by its server, so read-only is only the playbook's instruction",
            None: "with no read-only mark in the tools listing",
        }[self.read_only]
        return f"kubectl: {KUBECTL_TOOL} approved on the MCP source {KUBECTL_SOURCE}, {mark}"

    def reason(self) -> str:
        reason = f"{KUBECTL_TOOL} is {self.state}" + (f" ({self.refusal})" if self.refusal else "")
        cause = "the server image is too old" if self.state == "refused" else "the server image may be too old or the kubectl MCP server unreachable"
        return f"{reason}; {cause}: the run needs an image built from branch {IMAGE_BRANCH} or later, which speaks SSE to an MCP source and approves a tool its server does not mark read-only"


def kubectl_tool_state(tools: Iterable[dict[str, Any]]) -> KubectlTool:
    for tool in tools:
        if tool.get("name") == KUBECTL_TOOL:
            read_only = tool.get("read_only")
            return KubectlTool(str(tool.get("state")), tool.get("refusal"), read_only if isinstance(read_only, bool) else None)
    return KubectlTool("missing")


def split_model(model_name: str | None) -> tuple[str, str | None]:
    if not model_name or "/" not in model_name:
        raise ValueError("pass the model as <provider>/<model>, for example --model anthropic/claude-sonnet-5-5 or --model replay/replay")
    provider, _, model = model_name.partition("/")
    return provider, None if provider == "replay" else model


class Options(LoopOptions):
    model_config = ConfigDict(extra="forbid")

    model: str
    server_image: str = "slashobserve-server:local"
    replay_dir: str | None = None
    llm_effort: str | None = None
    run_max_cost_usd: float | None = Field(default=None, gt=0)
    spend_cap_usd: float | None = Field(default=None, gt=0)
    ledger_path: str | None = None
    wall_timeout_sec: int = Field(default=1500, ge=60)
    conductor_url: str
    mcp_url: str
    prometheus_url: str | None = None
    loki_url: str | None = None
    kubectl_wait_sec: int = Field(default=120, ge=10)
    without_kubectl: bool = False
    out: str

    @model_validator(mode="after")
    def _cap_needs_ledger(self) -> "Options":
        if (self.spend_cap_usd is None) != (self.ledger_path is None):
            raise ValueError("spend_cap_usd and ledger_path go together")
        return self


def registry_yaml(project: Path, run_args: list[str]) -> str:
    command = shlex.join(["uv", "run", "--locked", "--project", str(project), "python", "-m", "slashobserve_bench.sregym", "run", *run_args])
    return "\n".join(
        [
            "agents:",
            f"  - name: {AGENT_NAME}",
            f"    kickoff_command: {json.dumps(command)}",
            f"    kickoff_workdir: {json.dumps(str(project))}",
            "    kickoff_env: null",
            "    install_script: null",
            "    agent_version: null",
            "    container_isolation: false",
            "",
        ]
    )


def deadline_sec(wall_timeout_sec: float, remaining_sec: float | None, margin_sec: float = SUBMIT_MARGIN_SEC) -> float:
    if remaining_sec is None:
        return wall_timeout_sec
    return max(min(wall_timeout_sec, remaining_sec - margin_sec), 0)


def free_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as probe:
        probe.bind(("127.0.0.1", 0))
        return probe.getsockname()[1]


def port_forward_args(service: str, local_port: int, remote_port: int) -> list[str]:
    return [
        "kubectl",
        "port-forward",
        "-n",
        OBSERVE_NAMESPACE,
        f"svc/{service}",
        f"{local_port}:{remote_port}",
        "--address",
        "0.0.0.0",
    ]


def _listening(port: int) -> bool:
    try:
        with socket.create_connection(("127.0.0.1", port), timeout=2):
            return True
    except OSError:
        return False


class PortForward:
    def __init__(self, service: str, remote_port: int):
        self.service = service
        self.remote_port = remote_port
        self.local_port = free_port()
        self.process: subprocess.Popen | None = None

    def start(self, timeout_sec: float = 30) -> str:
        self.process = subprocess.Popen(
            port_forward_args(self.service, self.local_port, self.remote_port),
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )
        deadline = time.monotonic() + timeout_sec
        while time.monotonic() < deadline:
            if self.process.poll() is not None:
                raise UnsupportedProblem(f"kubectl port-forward to {OBSERVE_NAMESPACE}/{self.service} exited")
            if _listening(self.local_port):
                return f"http://{HOST_ALIAS}:{self.local_port}"
            time.sleep(0.5)
        raise UnsupportedProblem(f"kubectl port-forward to {OBSERVE_NAMESPACE}/{self.service} did not listen within {timeout_sec:.0f} seconds")

    def stop(self) -> None:
        if self.process is not None and self.process.poll() is None:
            self.process.terminate()
            try:
                self.process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                self.process.kill()


def _docker(*args: str) -> None:
    done = subprocess.run(["docker", *args], capture_output=True, text=True, check=False)
    if done.returncode != 0:
        raise UnsupportedProblem(f"docker {args[0]} {args[1] if len(args) > 1 else ''} failed: {done.stderr.strip()}")


def _write_json(path: Path, value: Any) -> None:
    path.write_text(json.dumps(value, indent=2) + "\n", encoding="utf-8")


class Runner:
    def __init__(self, options: Options, conductor: Conductor, env: dict[str, str]):
        self.options = options
        self.conductor = conductor
        self.env = env
        self.out = Path(options.out)
        self.notes: list[str] = []
        self.ledger = SpendLedger(Path(options.ledger_path), options.spend_cap_usd) if options.ledger_path and options.spend_cap_usd else None

    def _llm_key(self, provider: str) -> str | None:
        if provider == "replay":
            return None
        path = self.env.get(LLM_KEY_FILE_ENV)
        if not path:
            raise ValueError(f"set {LLM_KEY_FILE_ENV} to the path of a file that holds the model API key")
        return Path(path).read_text(encoding="utf-8").strip()

    def _note(self, line: str) -> None:
        self.notes.append(line)
        self.out.mkdir(parents=True, exist_ok=True)
        (self.out / "notes.txt").write_text("".join(f"{note}\n" for note in self.notes), encoding="utf-8")

    def wait_for_diagnosis_stage(self) -> Status:
        deadline = time.monotonic() + STAGE_WAIT_SEC
        while True:
            status = self.conductor.status()
            if status.stage == DIAGNOSIS:
                return status
            if status.stage not in WAITING_STAGES or time.monotonic() >= deadline:
                raise UnsupportedProblem(f"the conductor is at stage {status.stage!r}, not {DIAGNOSIS!r}")
            time.sleep(2)

    def run(self) -> int:
        started = time.monotonic()
        provider, model = split_model(self.options.model)
        if self.ledger is not None:
            self.ledger.ensure_room()
        app = self.conductor.app()
        if DIAGNOSIS not in app.stages:
            raise UnsupportedProblem(f"this problem has no {DIAGNOSIS} stage (stages: {', '.join(app.stages) or 'none'})")
        status = self.wait_for_diagnosis_stage()
        for image in (self.options.server_image, POSTGRES_IMAGE):
            ensure_image(image)
        self._note("traces: not read, because SREGym's Jaeger is 1.57 and the Jaeger datasource needs 2.21 or later")
        self._note("mitigation: not submitted; this adapter answers the diagnosis stage only")
        self._note(self.options.grounding_note())
        forwards: list[PortForward] = []
        network = f"slashobserve-sregym-{secrets.token_hex(4)}"
        stack = None
        fixtures = tempfile.TemporaryDirectory(prefix="slashobserve-sregym-replay-")
        try:
            replay_dir = self.options.replay_dir
            if provider == "replay" and replay_dir is None:
                replay_dir = self._write_replay_fixtures(app, Path(fixtures.name))
            prometheus = self.options.prometheus_url or self._forward(forwards, *PROMETHEUS_SERVICE)
            loki = self.options.loki_url or self._forward(forwards, *LOKI_SERVICE)
            _docker("network", "create", "--label", LABEL, network)
            config = ServerConfig(
                image=self.options.server_image,
                network=network,
                llm_provider=provider,
                llm_model=model,
                llm_effort=self.options.llm_effort,
                investigation_calls=self.options.investigation_calls,
                investigation_loop=self.options.investigation_loop,
                investigation_calls_max=self.options.investigation_calls_max,
                finding_grounding=self.options.effective_grounding,
                run_max_cost_usd=self.options.run_max_cost_usd,
                replay_dir=Path(replay_dir) if replay_dir else None,
                add_hosts=(f"{HOST_ALIAS}:host-gateway",),
                extra_env=server_settings(self.options.mcp_url),
            )
            files = {MCP_SOURCES_FILE: mcp_sources_yaml(self.options.mcp_url)}
            stack = Stack(config, datasources_yaml(prometheus, loki), self.out, self._llm_key(provider), files)
            asyncio.run(stack.start())
            client = Client(stack.base_url, stack.key)
            kubectl = self._kubectl_ready(client)
            text = question(app, kubectl)
            elapsed = time.monotonic() - started
            remaining = None if status.remaining_sec is None else status.remaining_sec - elapsed
            budget = deadline_sec(self.options.wall_timeout_sec - elapsed, remaining)
            request = self._investigate(client, text, budget)
        finally:
            if stack is not None:
                stack.stop()
            subprocess.run(["docker", "network", "rm", network], capture_output=True, check=False)
            for forward in forwards:
                forward.stop()
            fixtures.cleanup()
        return self._submit(request)

    def _write_replay_fixtures(self, app: App, directory: Path) -> str:
        directory.chmod(0o755)
        template = json.loads(REPLAY_TEMPLATE.read_text(encoding="utf-8"))
        for name, fixture in replay_fixtures(template, app).items():
            path = directory / name
            _write_json(path, fixture)
            path.chmod(0o644)
        self._note("model: the replay model, with a fixture written for this problem's question")
        return str(directory)

    def _forward(self, forwards: list[PortForward], service: str, port: int) -> str:
        forward = PortForward(service, port)
        forwards.append(forward)
        url = forward.start()
        self._note(f"{service}: read through kubectl port-forward on host port {forward.local_port}")
        return url

    def _kubectl_ready(self, client: Client) -> bool:
        deadline = time.monotonic() + self.options.kubectl_wait_sec
        tool = KubectlTool("missing")
        while time.monotonic() < deadline:
            try:
                tool = kubectl_tool_state(client.mcp_tools(KUBECTL_SOURCE))
            except ApiError as error:
                tool = KubectlTool("unavailable", str(error))
            if tool.state in ("approved", "refused"):
                break
            time.sleep(2)
        if tool.approved:
            self._note(tool.note())
            return True
        if not self.options.without_kubectl:
            raise UnsupportedProblem(f"the cluster cannot be read: {tool.reason()}; pass --without-kubectl for a telemetry-only baseline")
        self._note(f"kubectl: not available, {tool.reason()}; the run used telemetry alone")
        return False

    def _investigate(self, client: Client, text: str, budget_sec: float) -> dict:
        run_id = client.start_run(start_body(text))
        seen: dict[str, dict] = {}

        def poll() -> dict:
            seen["request"] = client.get_run(run_id)
            return seen["request"]

        try:
            try:
                request = wait_for_terminal(poll, budget_sec)
            except RunTimeout:
                try:
                    client.cancel_run(run_id)
                except ApiError:
                    pass
                try:
                    request = wait_for_terminal(poll, CANCEL_GRACE_SEC)
                except RunTimeout:
                    request = seen["request"]
                self._note(f"run: cancelled after {budget_sec:.0f} seconds")
            self._save(request, text)
            return request
        finally:
            if self.ledger is not None and "request" in seen:
                self.ledger.record(self.env.get("SREGYM_ARTIFACT_ID") or self.out.name, atif.Usage.from_request(seen["request"]).cost_usd or 0.0)

    def _save(self, request: dict, text: str) -> None:
        self.out.mkdir(parents=True, exist_ok=True)
        _write_json(self.out / "run.json", request)
        trajectory = atif.build_trajectory(request, text, agent_name=AGENT_NAME, agent_version=AGENT_VERSION, model_name=self.options.model)
        _write_json(self.out / "trajectory.json", trajectory)

    def _submit(self, request: dict) -> int:
        diagnosis = diagnosis_for(request)
        (self.out / "diagnosis.txt").write_text(diagnosis + "\n", encoding="utf-8")
        try:
            answer = self.conductor.submit_diagnosis(diagnosis)
        except ConductorError as error:
            _write_json(self.out / "submission.json", {"stage": DIAGNOSIS, "error": str(error)})
            raise
        _write_json(self.out / "submission.json", {"stage": DIAGNOSIS, "response": answer})
        return 0


def _parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="python -m slashobserve_bench.sregym")
    commands = parser.add_subparsers(dest="command", required=True)
    commands.add_parser("subset", help="print the ten SREGym-Lite problem ids of the fixed subset, one per line")
    registry = commands.add_parser("registry", help="write an agent registry for SREGYM_AGENT_REGISTRY that runs this adapter")
    registry.add_argument("--out", type=Path, required=True)
    registry.add_argument("run_args", nargs=argparse.REMAINDER, help="options for `run`, after --")
    run = commands.add_parser("run", help="diagnose the problem the SREGym conductor is serving and submit the diagnosis")
    run.add_argument("--model", help="<provider>/<model>; defaults to SREGym's AGENT_MODEL_ID")
    run.add_argument("--server-image")
    run.add_argument("--replay-dir")
    run.add_argument("--llm-effort")
    run.add_argument("--investigation-calls", type=int)
    run.add_argument("--investigation-loop", choices=("true", "false"))
    run.add_argument("--investigation-calls-max", type=int)
    run.add_argument("--finding-grounding", choices=("off", "report", "enforce"))
    run.add_argument("--run-max-cost-usd", type=float)
    run.add_argument("--spend-cap-usd", type=float)
    run.add_argument("--ledger-path")
    run.add_argument("--wall-timeout-sec", type=int)
    run.add_argument("--conductor-url", help="defaults to http://API_HOSTNAME:API_PORT, as SREGym sets them")
    run.add_argument("--mcp-url", help=f"the kubectl MCP server as the server container sees it; defaults to http://{HOST_ALIAS}:MCP_SERVER_PORT{KUBECTL_PATH}")
    run.add_argument("--prometheus-url", help="skip the port-forward and reach Prometheus here")
    run.add_argument("--loki-url", help="skip the port-forward and reach Loki here")
    run.add_argument("--kubectl-wait-sec", type=int)
    run.add_argument("--without-kubectl", action="store_true", help="diagnose from telemetry alone when the kubectl tool is not approved, as a telemetry-only baseline or on a server image older than branch agent-rca-accuracy")
    run.add_argument("--out", help="defaults to SREGym's AGENT_LOGS_DIR")
    return parser


def options_from(args: argparse.Namespace, env: dict[str, str]) -> Options:
    values = {
        key: value
        for key, value in vars(args).items()
        if key not in ("command", "without_kubectl") and value is not None
    }
    if "investigation_loop" in values:
        values["investigation_loop"] = values["investigation_loop"] == "true"
    values.setdefault("model", env.get("AGENT_MODEL_ID"))
    values.setdefault("conductor_url", conductor_url(env))
    values.setdefault("mcp_url", mcp_url(env))
    values.setdefault("out", env.get("AGENT_LOGS_DIR"))
    if not values.get("out"):
        raise ValueError("pass --out, or run under SREGym, which sets AGENT_LOGS_DIR")
    if not values.get("model"):
        raise ValueError("pass --model, or run under SREGym, which sets AGENT_MODEL_ID")
    return Options(**values, without_kubectl=args.without_kubectl)


def main(argv: list[str] | None = None, env: dict[str, str] | None = None, runner: Callable[[Options, dict[str, str]], int] | None = None) -> int:
    env = dict(os.environ if env is None else env)
    args = _parser().parse_args(argv)
    if args.command == "subset":
        print("\n".join(SUBSET))
        return 0
    if args.command == "registry":
        run_args = args.run_args[1:] if args.run_args[:1] == ["--"] else args.run_args
        args.out.write_text(registry_yaml(Path(__file__).resolve().parents[1], run_args), encoding="utf-8")
        return 0
    options = options_from(args, env)
    if runner is None:
        return Runner(options, Conductor(options.conductor_url), env).run()
    return runner(options, env)


if __name__ == "__main__":
    sys.exit(main())
