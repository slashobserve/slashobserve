import asyncio
import base64
import json
import secrets
import shlex
import tempfile
import urllib.error
import urllib.parse
import urllib.request
from collections.abc import Awaitable, Callable
from dataclasses import dataclass
from pathlib import Path
from typing import Any

GRAFANA_URL_ENV = "GRAFANA_URL"
TOKEN_ENVS = ("GRAFANA_SERVICE_ACCOUNT_TOKEN", "GRAFANA_API_KEY", "GRAFANA_TOKEN")
USER_ENVS = ("GRAFANA_USER", "GRAFANA_USERNAME", "GRAFANA_ADMIN_USER")
PASSWORD_ENVS = ("GRAFANA_PASSWORD", "GRAFANA_ADMIN_PASSWORD")
AUTH_ENV = "SLASHOBSERVE_BENCH_GRAFANA_AUTH"
STORE_TYPES = ("prometheus", "loki", "tempo")
MAX_GRAFANA_UIDS = 20
PROMPT_LIMIT = 4000

Transport = Callable[[str, str, dict[str, str], bytes | None], Awaitable[tuple[int, bytes]]]


class GrafanaError(Exception):
    def __init__(self, status: int, body: str):
        super().__init__(f"Grafana answered HTTP {status}: {body[:500]}")
        self.status = status
        self.body = body


def grafana_auth(task_env: dict[str, str], user: str | None = None, password: str | None = None) -> str | None:
    for name in TOKEN_ENVS:
        if task_env.get(name):
            return f"Bearer {task_env[name]}"
    env_user = next((task_env[name] for name in USER_ENVS if task_env.get(name)), None)
    env_password = next((task_env[name] for name in PASSWORD_ENVS if task_env.get(name)), None)
    if env_user and env_password:
        user, password = env_user, env_password
    if user and password:
        return "Basic " + base64.b64encode(f"{user}:{password}".encode()).decode()
    return None


def host_transport(auth: str | None) -> Transport:
    async def send(method: str, url: str, headers: dict[str, str], body: bytes | None) -> tuple[int, bytes]:
        def call() -> tuple[int, bytes]:
            request = urllib.request.Request(url, data=body, method=method)
            for name, value in headers.items():
                request.add_header(name, value)
            if auth:
                request.add_header("Authorization", auth)
            try:
                with urllib.request.urlopen(request, timeout=60) as response:
                    return response.status, response.read()
            except urllib.error.HTTPError as error:
                return error.code, error.read()

        return await asyncio.to_thread(call)

    return send


def curl_command(method: str, url: str, headers: dict[str, str], body_path: str | None, with_auth: bool) -> str:
    parts = ["curl", "-sS", "--max-time", "60", "-X", shlex.quote(method)]
    for name, value in headers.items():
        parts += ["-H", shlex.quote(f"{name}: {value}")]
    if with_auth:
        parts += ["-H", f'"Authorization: ${AUTH_ENV}"']
    if body_path:
        parts += ["--data-binary", shlex.quote(f"@{body_path}")]
    parts += ["-w", shlex.quote("\n%{http_code}"), shlex.quote(url)]
    return " ".join(parts)


def split_curl_output(stdout: str) -> tuple[int, bytes]:
    body, _, status = stdout.rpartition("\n")
    if not status.strip().isdigit():
        raise GrafanaError(0, f"curl printed no status: {stdout[-200:]!r}")
    return int(status.strip()), body.encode()


def container_transport(environment: Any, auth: str | None) -> Transport:
    async def send(method: str, url: str, headers: dict[str, str], body: bytes | None) -> tuple[int, bytes]:
        body_path = None
        if body is not None:
            body_path = f"/tmp/slashobserve-bench-grafana-{secrets.token_hex(6)}.json"
            with tempfile.TemporaryDirectory(prefix="slashobserve-bench-") as workdir:
                local = Path(workdir) / "body.json"
                local.write_bytes(body)
                await environment.upload_file(local, body_path)
        command = curl_command(method, url, headers, body_path, auth is not None)
        if body_path:
            command += f"; rm -f {shlex.quote(body_path)}"
        done = await environment.exec(command, env={AUTH_ENV: auth} if auth else None)
        if done.return_code not in (0, None) and not (done.stdout or "").strip():
            raise GrafanaError(0, f"curl failed in the task container: {(done.stderr or '').strip()[:300]}")
        return split_curl_output(done.stdout or "")

    return send


class GrafanaClient:
    def __init__(self, base_url: str, transport: Transport):
        self.base_url = base_url.rstrip("/")
        self._send = transport

    async def _call(self, method: str, path: str, body: Any = None) -> Any:
        data = None if body is None else json.dumps(body).encode()
        headers = {"Accept": "application/json"}
        if data is not None:
            headers["Content-Type"] = "application/json"
        status, raw = await self._send(method, f"{self.base_url}{path}", headers, data)
        text = raw.decode(errors="replace")
        if status >= 400 or status == 0:
            raise GrafanaError(status, text)
        return json.loads(text) if text.strip() else {}

    async def datasources(self) -> list[dict[str, Any]]:
        return await self._call("GET", "/api/datasources")

    async def dashboard_uids(self) -> list[dict[str, Any]]:
        return [hit for hit in await self._call("GET", "/api/search?type=dash-db&limit=5000") if hit.get("uid")]

    async def dashboard(self, uid: str) -> dict[str, Any]:
        return await self._call("GET", f"/api/dashboards/uid/{urllib.parse.quote(uid, safe='')}")

    async def save_dashboard(self, dashboard: dict[str, Any], folder_uid: str | None, message: str) -> dict[str, Any]:
        body: dict[str, Any] = {"dashboard": dashboard, "overwrite": True, "message": message}
        if folder_uid:
            body["folderUid"] = folder_uid
        return await self._call("POST", "/api/dashboards/db", body)

    async def query(self, body: dict[str, Any]) -> tuple[int, dict[str, Any]]:
        data = json.dumps(body).encode()
        status, raw = await self._send(
            "POST", f"{self.base_url}/api/ds/query", {"Accept": "application/json", "Content-Type": "application/json"}, data
        )
        try:
            parsed = json.loads(raw.decode(errors="replace") or "{}")
        except json.JSONDecodeError:
            parsed = {"message": raw.decode(errors="replace")[:500]}
        return status, parsed if isinstance(parsed, dict) else {"message": str(parsed)[:500]}


@dataclass
class GrafanaState:
    datasources: list[dict[str, Any]]
    dashboards: list[dict[str, Any]]

    def dashboard_by_uid(self, uid: str) -> dict[str, Any] | None:
        return next((entry for entry in self.dashboards if entry["dashboard"].get("uid") == uid), None)


async def read_state(client: GrafanaClient) -> GrafanaState:
    datasources = await client.datasources()
    dashboards = [await client.dashboard(hit["uid"]) for hit in await client.dashboard_uids()]
    return GrafanaState(datasources=datasources, dashboards=dashboards)


def grafana_uids_by_type(datasources: list[dict[str, Any]], own_uids: tuple[str, ...] = STORE_TYPES) -> dict[str, list[str]]:
    mapped: dict[str, list[str]] = {kind: [] for kind in STORE_TYPES}
    for datasource in datasources:
        kind = datasource.get("type")
        if kind not in mapped:
            continue
        for value in (datasource.get("uid"), datasource.get("name")):
            if not isinstance(value, str):
                continue
            value = value.strip()
            if not value or len(value.encode()) > 100 or value in own_uids:
                continue
            if any(value in values for values in mapped.values()) or len(mapped[kind]) >= MAX_GRAFANA_UIDS:
                continue
            mapped[kind].append(value)
    return {kind: values for kind, values in mapped.items() if values}


def grafana_datasource_for(datasources: list[dict[str, Any]], kind: str) -> dict[str, Any] | None:
    candidates = [datasource for datasource in datasources if datasource.get("type") == kind]
    return next((d for d in candidates if d.get("isDefault")), candidates[0] if candidates else None)


def datasource_map(datasources: list[dict[str, Any]]) -> dict[str, dict[str, str]]:
    mapping = {}
    for kind in STORE_TYPES:
        datasource = grafana_datasource_for(datasources, kind)
        if datasource is not None:
            mapping[kind] = {"uid": datasource.get("uid", ""), "type": kind, "name": datasource.get("name", "")}
    return mapping


def _datasource_line(datasource: dict[str, Any]) -> str:
    fields = [
        f"name {datasource.get('name')}",
        f"type {datasource.get('type')}",
        f"uid {datasource.get('uid')}",
    ]
    for key in ("id", "url", "access", "isDefault", "readOnly"):
        if datasource.get(key) not in (None, ""):
            fields.append(f"{key} {datasource[key]}")
    return "- " + ", ".join(fields)


def _dashboard_line(entry: dict[str, Any]) -> str:
    dashboard = entry["dashboard"]
    meta = entry.get("meta") or {}
    fields = [f"title {dashboard.get('title')}", f"uid {dashboard.get('uid')}"]
    if meta.get("folderTitle"):
        fields.append(f"folder {meta['folderTitle']}")
    if dashboard.get("tags"):
        fields.append("tags " + ", ".join(dashboard["tags"]))
    return "- " + "; ".join(fields)


def context_note(state: GrafanaState, imported: dict[str, str], limit: int) -> str:
    lines = [
        "Context from the Grafana instance this question is about, read just before the run.",
        "Its dashboards were imported here with the same titles; answer with Grafana's names, types and uids below, not with this server's.",
        "Grafana datasources:",
        *[_datasource_line(datasource) for datasource in state.datasources],
        "Grafana dashboards:" if state.dashboards else "Grafana has no dashboards.",
    ]
    for entry in state.dashboards:
        line = _dashboard_line(entry)
        uid = entry["dashboard"].get("uid")
        if uid in imported:
            line += f" (imported here as dashboard {imported[uid]})"
        lines.append(line)
    kept: list[str] = []
    size = 0
    for line in lines:
        if size + len(line) + 1 > limit - 20:
            kept.append("- (cut short)")
            break
        kept.append(line)
        size += len(line) + 1
    return "\n".join(kept)


def question_prompt(instruction: str, state: GrafanaState, imported: dict[str, str], limit: int = PROMPT_LIMIT) -> str:
    instruction = instruction.strip()
    room = limit - len(instruction) - 2
    if room < 200:
        return instruction
    return f"{instruction}\n\n{context_note(state, imported, room)}"
