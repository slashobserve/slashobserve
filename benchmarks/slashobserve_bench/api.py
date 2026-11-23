import json
import time
import urllib.error
import urllib.request
from collections.abc import Callable
from typing import Any

TERMINAL_STATES = ("succeeded", "failed", "canceled")


class ApiError(Exception):
    def __init__(self, status: int, body: str):
        super().__init__(f"HTTP {status}: {body[:500]}")
        self.status = status


class RunTimeout(Exception):
    pass


class Client:
    def __init__(self, base_url: str, key: str):
        self.base_url = base_url
        self._key = key

    def _call(self, method: str, path: str, body: dict[str, Any] | None = None) -> dict[str, Any]:
        data = None if body is None else json.dumps(body).encode()
        request = urllib.request.Request(f"{self.base_url}/api/v1{path}", data=data, method=method)
        request.add_header("Authorization", f"Bearer {self._key}")
        if data is not None:
            request.add_header("Content-Type", "application/json")
        try:
            with urllib.request.urlopen(request, timeout=30) as response:
                return json.loads(response.read() or b"{}")
        except urllib.error.HTTPError as error:
            raise ApiError(error.code, error.read().decode(errors="replace")) from None

    def create_observation(self, title: str) -> str:
        return self._call("POST", "/observations", {"title": title})["observation"]["id"]

    def start_run(self, body: dict[str, Any]) -> str:
        return self._call("POST", "/observe", body)["request"]["id"]

    def get_run(self, run_id: str) -> dict[str, Any]:
        return self._call("GET", f"/observe/{run_id}")["request"]

    def cancel_run(self, run_id: str) -> None:
        self._call("POST", f"/observe/{run_id}/cancel", {})


def wait_for_terminal(
    fetch: Callable[[], dict[str, Any]],
    timeout_sec: float,
    interval_sec: float = 1.0,
    clock: Callable[[], float] = time.monotonic,
    sleep: Callable[[float], None] = time.sleep,
) -> dict[str, Any]:
    deadline = clock() + timeout_sec
    while True:
        request = fetch()
        if request["state"] in TERMINAL_STATES:
            return request
        if clock() >= deadline:
            raise RunTimeout(f"the run was still {request['state']} after {timeout_sec:.0f} seconds")
        sleep(interval_sec)
