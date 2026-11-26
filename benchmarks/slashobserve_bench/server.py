import asyncio
import base64
import json
import secrets
import subprocess
import tempfile
import time
import urllib.error
import urllib.request
from collections.abc import Callable
from dataclasses import dataclass
from pathlib import Path

from slashobserve_bench.o11y import Stores

POSTGRES_IMAGE = "postgres:18.6-alpine3.23@sha256:885cf05d376c7cf27afef02073e6bdac3841252537f16e244fd1c1e6a7c99fb1"
SERVER_PORT = 8080
SERVICE_ACCOUNT = "benchmark"
CONFIG_DIR = "/etc/slashobserve"
FIXTURES_DIR = "/var/lib/slashobserve-bench/llm-fixtures"
LABEL = "slashobserve-bench"


class StackError(Exception):
    pass


@dataclass(frozen=True)
class ServerConfig:
    image: str
    network: str
    llm_provider: str
    llm_model: str | None = None
    llm_effort: str | None = None
    investigation_calls: int | None = None
    investigation_loop: bool | None = None
    investigation_calls_max: int | None = None
    finding_grounding: str | None = None
    run_max_cost_usd: float | None = None
    replay_dir: Path | None = None


def new_key() -> str:
    return "sok_" + base64.urlsafe_b64encode(secrets.token_bytes(32)).decode().rstrip("=")


def service_accounts_yaml(key_file: str) -> str:
    return (
        "serviceAccounts:\n"
        f"  - name: {SERVICE_ACCOUNT}\n"
        "    role: admin\n"
        f"    keyFile: {json.dumps(key_file)}\n"
    )


def datasources_yaml(stores: Stores) -> str:
    entries = (
        ("prometheus", "prometheus", "Prometheus", stores.prometheus),
        ("loki", "loki", "Loki", stores.loki),
        ("tempo", "tempo", "Tempo", stores.tempo),
    )
    lines = ["datasources:"]
    for uid, kind, name, url in entries:
        lines += [
            f"  - uid: {uid}",
            f"    type: {kind}",
            f"    name: {name}",
            f"    {kind}:",
            f"      url: {json.dumps(url)}",
        ]
    return "\n".join(lines) + "\n"


def server_env(config: ServerConfig, database_url: str) -> dict[str, str]:
    env = {
        "SLASHOBSERVE_DATABASE_URL": database_url,
        "SLASHOBSERVE_PUBLIC_URL": f"http://localhost:{SERVER_PORT}",
        "SLASHOBSERVE_SERVICE_ACCOUNTS_FILE": f"{CONFIG_DIR}/service-accounts.yaml",
        "SLASHOBSERVE_DATASOURCES_FILE": f"{CONFIG_DIR}/datasources.yaml",
        "SLASHOBSERVE_LLM_PROVIDER": config.llm_provider,
    }
    optional = {
        "SLASHOBSERVE_LLM_MODEL": config.llm_model,
        "SLASHOBSERVE_LLM_EFFORT": config.llm_effort,
        "SLASHOBSERVE_AGENT_INVESTIGATION_CALLS": config.investigation_calls,
        "SLASHOBSERVE_AGENT_INVESTIGATION_LOOP": None if config.investigation_loop is None else str(config.investigation_loop).lower(),
        "SLASHOBSERVE_AGENT_INVESTIGATION_CALLS_MAX": config.investigation_calls_max,
        "SLASHOBSERVE_AGENT_FINDING_GROUNDING": config.finding_grounding,
        "SLASHOBSERVE_AGENT_RUN_MAX_COST_USD": config.run_max_cost_usd,
    }
    env.update({name: str(value) for name, value in optional.items() if value is not None})
    if config.replay_dir is not None:
        env["SLASHOBSERVE_LLM_REPLAY_DIR"] = FIXTURES_DIR
    return env


def postgres_args(name: str, network: str, password: str) -> list[str]:
    return [
        "run",
        "-d",
        "--name",
        name,
        "--network",
        network,
        "--label",
        LABEL,
        "--tmpfs",
        "/var/lib/postgresql",
        "-e",
        "POSTGRES_USER=slashobserve",
        "-e",
        f"POSTGRES_PASSWORD={password}",
        "-e",
        "POSTGRES_DB=slashobserve",
        POSTGRES_IMAGE,
    ]


def server_args(
    name: str,
    config: ServerConfig,
    env: dict[str, str],
    config_dir: Path,
    env_file: Path | None,
) -> list[str]:
    args = [
        "run",
        "-d",
        "--name",
        name,
        "--network",
        config.network,
        "--label",
        LABEL,
        "-p",
        f"127.0.0.1::{SERVER_PORT}",
        "-v",
        f"{config_dir}:{CONFIG_DIR}:ro",
    ]
    if config.replay_dir is not None:
        args += ["-v", f"{config.replay_dir.resolve()}:{FIXTURES_DIR}:ro"]
    if env_file is not None:
        args += ["--env-file", str(env_file)]
    for key, value in env.items():
        args += ["-e", f"{key}={value}"]
    return [*args, config.image]


def parse_published_port(output: str) -> int:
    first = output.strip().splitlines()[0] if output.strip() else ""
    port = first.rsplit(":", 1)[-1]
    if not port.isdigit():
        raise StackError(f"docker port printed no host port: {output!r}")
    return int(port)


async def wait_until(check: Callable[[], bool], what: str, timeout_sec: float, interval_sec: float = 0.5) -> None:
    deadline = time.monotonic() + timeout_sec
    while True:
        if await asyncio.to_thread(check):
            return
        if time.monotonic() >= deadline:
            raise StackError(f"{what} did not become ready within {timeout_sec:.0f} seconds")
        await asyncio.sleep(interval_sec)


def _docker(*args: str, check: bool = True) -> str:
    done = subprocess.run(["docker", *args], capture_output=True, text=True, check=False)
    if check and done.returncode != 0:
        raise StackError(f"docker {args[0]} failed: {done.stderr.strip()}")
    return done.stdout


def ensure_image(image: str) -> None:
    if subprocess.run(["docker", "image", "inspect", image], capture_output=True, check=False).returncode == 0:
        return
    _docker("pull", image)


def _ready(url: str) -> bool:
    try:
        with urllib.request.urlopen(url, timeout=3) as response:
            return response.status == 200
    except (urllib.error.URLError, OSError):
        return False


class Stack:
    def __init__(self, config: ServerConfig, datasources: str, logs_dir: Path, llm_key: str | None):
        suffix = secrets.token_hex(4)
        self.config = config
        self.datasources = datasources
        self.logs_dir = logs_dir
        self.llm_key = llm_key
        self.postgres_name = f"slashobserve-bench-pg-{suffix}"
        self.server_name = f"slashobserve-bench-server-{suffix}"
        self.key = new_key()
        self.base_url = ""
        self._workdir = tempfile.TemporaryDirectory(prefix="slashobserve-bench-")

    async def start(self) -> None:
        config_dir = self._write_config()
        env_file = self._write_env_file()
        password = secrets.token_hex(16)
        await asyncio.to_thread(_docker, *postgres_args(self.postgres_name, self.config.network, password))
        await wait_until(self._postgres_ready, "postgres", 60)
        url = f"postgres://slashobserve:{password}@{self.postgres_name}:5432/slashobserve?sslmode=disable"
        env = server_env(self.config, url)
        await asyncio.to_thread(_docker, *server_args(self.server_name, self.config, env, config_dir, env_file))
        port = parse_published_port(await asyncio.to_thread(_docker, "port", self.server_name, f"{SERVER_PORT}/tcp"))
        self.base_url = f"http://127.0.0.1:{port}"
        await wait_until(lambda: _ready(f"{self.base_url}/readyz"), "the server", 120)

    def stop(self) -> None:
        self.logs_dir.mkdir(parents=True, exist_ok=True)
        logs = subprocess.run(
            ["docker", "logs", self.server_name], capture_output=True, text=True, check=False
        )
        if logs.returncode == 0:
            (self.logs_dir / "server.log").write_text(logs.stdout + logs.stderr, encoding="utf-8")
        subprocess.run(["docker", "rm", "-f", self.server_name, self.postgres_name], capture_output=True, check=False)
        self._workdir.cleanup()

    def _write_config(self) -> Path:
        config_dir = Path(self._workdir.name) / "config"
        config_dir.mkdir()
        config_dir.chmod(0o755)
        files = {
            "service-accounts.yaml": service_accounts_yaml(f"{CONFIG_DIR}/benchmark.key"),
            "datasources.yaml": self.datasources,
            "benchmark.key": self.key + "\n",
        }
        for name, content in files.items():
            path = config_dir / name
            path.write_text(content, encoding="utf-8")
            path.chmod(0o644)
        return config_dir

    def _write_env_file(self) -> Path | None:
        if not self.llm_key:
            return None
        path = Path(self._workdir.name) / "llm.env"
        path.touch(mode=0o600)
        path.write_text(f"SLASHOBSERVE_LLM_API_KEY={self.llm_key}\n", encoding="utf-8")
        return path

    def _postgres_ready(self) -> bool:
        done = subprocess.run(
            ["docker", "exec", self.postgres_name, "pg_isready", "-h", "127.0.0.1", "-U", "slashobserve", "-d", "slashobserve"],
            capture_output=True,
            check=False,
        )
        return done.returncode == 0
