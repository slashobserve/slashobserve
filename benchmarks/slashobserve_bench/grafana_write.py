import copy
import json
import re
from collections.abc import Awaitable, Callable
from dataclasses import dataclass, field
from datetime import datetime, timedelta
from typing import Any

from slashobserve_bench import grafana_merge as merge
from slashobserve_bench.atif import Usage
from slashobserve_bench.grafana_summary import describe_save
from slashobserve_bench.grafana import GrafanaClient, GrafanaError, datasource_map
from slashobserve_bench.model_call import ModelCallError, ModelReply

CHECK_WINDOW = timedelta(hours=6)
ALL_VALUE = "$__all"
ANY_VALUE = ".+"

SYSTEM = """You turn dashboard parts written in ObserveSpec, the dashboard format of SlashObserve, into Grafana dashboard JSON.
Reply with one JSON object and nothing else, in this shape:
{"panels": [{"spec_id": "<id of the Spec panel>", "panel": {<Grafana panel>}}], "variables": [{<Grafana templating entry>}], "annotations": [{<Grafana annotation entry>}]}
Write a part for every panel and variable listed under added or changed, and for nothing else; the caller places them into the Grafana dashboard and keeps every other panel as it is.
For a changed panel or variable, start from its current Grafana JSON when it is given and change only what the Spec change requires.
A panel has type, title, datasource ({"type": ..., "uid": ...}), targets with a refId each and the query field of its datasource (expr for Prometheus and Loki, query with queryType traceql for Tempo), fieldConfig and options.
Leave out a panel's id and its gridPos x and y; give gridPos w and h only for a new panel.
Use only the Grafana datasources listed in the request; a Spec datasource uid stands for the Grafana datasource the request maps it to.
Keep titles, units, variable names and query text exactly as the Spec has them, and turn Spec variable references into Grafana's $name form.
A query variable keeps the Spec's multi, includeAll and allValue settings; a multi-value or include-all variable inside a Prometheus or Loki label matcher needs =~.
The Spec has no annotations: write annotations only when the task statement asks for one, as entries of Grafana's annotations.list with name, datasource, enable true, iconColor and the query fields of its datasource (expr for Loki); otherwise give an empty list."""

ModelCall = Callable[[str, list[dict[str, str]]], Awaitable[ModelReply]]


@dataclass
class WriteBack:
    usage: Usage = field(default_factory=Usage)
    calls: int = 0
    attempts: list[dict[str, Any]] = field(default_factory=list)
    saved_uid: str | None = None
    error: str | None = None
    summary: str | None = None

    def outcome(self) -> dict[str, Any]:
        return {
            "model_calls": self.calls,
            "saved_uid": self.saved_uid,
            "error": self.error,
            "attempts": self.attempts,
            "usage": {
                "input_tokens": self.usage.input_tokens,
                "output_tokens": self.usage.output_tokens,
                "cache_read_tokens": self.usage.cache_read_tokens,
                "cache_write_tokens": self.usage.cache_write_tokens,
                "cost_usd": self.usage.cost_usd,
            },
        }


def _current_grafana_panel(original: dict[str, Any] | None, spec_id: str) -> dict[str, Any] | None:
    grafana_id = merge.grafana_id_of(spec_id)
    if original is None or grafana_id is None:
        return None
    return merge.find_panel(copy.deepcopy(original), grafana_id)


def _current_grafana_variable(original: dict[str, Any] | None, name: str) -> dict[str, Any] | None:
    for entry in ((original or {}).get("templating") or {}).get("list") or []:
        if entry.get("name") == name:
            return entry
    return None


def request_text(
    instruction: str,
    datasources: list[dict[str, Any]],
    original: dict[str, Any] | None,
    after_spec: dict[str, Any],
    changes: merge.SpecChanges,
) -> str:
    request = {
        "task_statement": instruction,
        "grafana_datasources": [{"uid": d.get("uid"), "name": d.get("name"), "type": d.get("type")} for d in datasources],
        "spec_datasource_to_grafana": datasource_map(datasources),
        "grafana_dashboard": original,
        "spec_variables_after": after_spec.get("variables") or [],
        "added_panels": changes.added_panels,
        "changed_panels": [
            {"spec_id": after["id"], "before": before, "after": after, "grafana": _current_grafana_panel(original, before["id"])}
            for before, after in changes.changed_panels
        ],
        "removed_panels": [panel["id"] for panel in changes.removed_panels],
        "added_variables": changes.added_variables,
        "changed_variables": [
            {"before": before, "after": after, "grafana": _current_grafana_variable(original, before["name"])}
            for before, after in changes.changed_variables
        ],
        "removed_variables": [variable["name"] for variable in changes.removed_variables],
    }
    return "Write these parts as Grafana JSON.\n" + json.dumps(request, indent=1)


def retry_text(problems: list[str]) -> str:
    return (
        "Grafana could not use the parts you wrote:\n"
        + "\n".join(f"- {problem}" for problem in problems)
        + "\nReply with the whole JSON object again, every part included, with these corrected."
    )


_VARIABLE = re.compile(r"\$\{(\w+)(?::[^}]*)?\}|\[\[(\w+)(?::[^\]]*)?\]\]|\$(\w+)")


def _variable_value(variable: dict[str, Any] | None) -> str:
    if variable is None:
        return ANY_VALUE
    value = (variable.get("current") or {}).get("value")
    if value in (None, "", []):
        options = variable.get("options") or []
        selected = [o.get("value") for o in options if o.get("selected")] or [o.get("value") for o in options]
        value = selected[0] if selected else None
    if value in (None, "", []):
        query = variable.get("query")
        if variable.get("type") == "custom" and isinstance(query, str) and query.strip():
            value = query.split(",")[0].strip()
    if value in (None, "", []):
        return ANY_VALUE
    values = value if isinstance(value, list) else [value]
    if ALL_VALUE in values:
        return variable.get("allValue") or ANY_VALUE
    return "|".join(str(item) for item in values) if len(values) > 1 else str(values[0])


def interpolate(text: str, variables: list[dict[str, Any]]) -> str:
    by_name = {variable.get("name"): variable for variable in variables}

    def replace(match: re.Match[str]) -> str:
        name = match.group(1) or match.group(2) or match.group(3)
        if name.startswith("__"):
            return match.group(0)
        return _variable_value(by_name.get(name))

    return _VARIABLE.sub(replace, text)


def _resolve_datasource(reference: Any, variables: list[dict[str, Any]]) -> dict[str, Any] | None:
    if isinstance(reference, str):
        reference = {"uid": reference}
    if not isinstance(reference, dict) or not reference.get("uid"):
        return None
    uid = str(reference["uid"])
    if uid.startswith("$") or uid.startswith("${"):
        uid = interpolate(uid, variables)
        if uid == ANY_VALUE:
            return None
    if uid.startswith("-- "):
        return None
    resolved = {"uid": uid}
    if reference.get("type"):
        resolved["type"] = reference["type"]
    return resolved


def panel_queries(panel: dict[str, Any], dashboard: dict[str, Any]) -> list[dict[str, Any]]:
    variables = (dashboard.get("templating") or {}).get("list") or []
    queries = []
    for index, target in enumerate(panel.get("targets") or []):
        if target.get("hide"):
            continue
        datasource = _resolve_datasource(target.get("datasource") or panel.get("datasource"), variables)
        if datasource is None:
            continue
        query = {key: interpolate(value, variables) if isinstance(value, str) else value for key, value in target.items()}
        query["datasource"] = datasource
        query["refId"] = target.get("refId") or chr(ord("A") + index % 26)
        query.setdefault("intervalMs", 15000)
        query.setdefault("maxDataPoints", 500)
        queries.append(query)
    return queries


def query_failures(status: int, response: dict[str, Any], ref_ids: list[str]) -> list[str]:
    results = response.get("results") or {}
    failures = []
    for ref_id in ref_ids:
        result = results.get(ref_id) or {}
        if result.get("error"):
            failures.append(f"{ref_id}: {result['error']}")
        elif result.get("status", 200) >= 400:
            failures.append(f"{ref_id}: HTTP {result['status']}")
    if status >= 400 and not failures:
        failures.append(f"HTTP {status}: {response.get('message') or response.get('error') or 'the query failed'}")
    return failures


def check_range(as_of: str | None) -> dict[str, str]:
    try:
        end = datetime.fromisoformat((as_of or "").replace("Z", "+00:00"))
    except ValueError:
        return {"from": "now-6h", "to": "now"}
    return {"from": str(int((end - CHECK_WINDOW).timestamp() * 1000)), "to": str(int(end.timestamp() * 1000))}


async def check_panels(
    client: GrafanaClient, dashboard: dict[str, Any], placed: dict[str, int], window: dict[str, str]
) -> dict[str, list[str]]:
    failures: dict[str, list[str]] = {}
    for spec_id, grafana_id in placed.items():
        panel = merge.find_panel(dashboard, grafana_id)
        if panel is None:
            failures[spec_id] = ["the saved dashboard has no such panel"]
            continue
        queries = panel_queries(panel, dashboard)
        if not queries:
            continue
        status, response = await client.query({"queries": queries, **window})
        problems = query_failures(status, response, [query["refId"] for query in queries])
        if problems:
            failures[spec_id] = problems
    return failures


async def write_back(
    *,
    call: ModelCall,
    client: GrafanaClient,
    instruction: str,
    datasources: list[dict[str, Any]],
    original: dict[str, Any] | None,
    meta: dict[str, Any] | None,
    before_spec: dict[str, Any] | None,
    after_spec: dict[str, Any],
    window: dict[str, str],
    state: WriteBack,
    on_reply: Callable[[int, ModelReply], None],
) -> WriteBack:
    changes = merge.spec_changes(before_spec, after_spec)
    messages = [{"role": "user", "content": request_text(instruction, datasources, original, after_spec, changes)}]
    folder_uid = (meta or {}).get("folderUid") or None
    target_uid = (original or {}).get("uid")
    for attempt in range(2):
        try:
            reply = await call(SYSTEM, messages)
        except ModelCallError as error:
            state.error = str(error)
            return state
        state.calls += 1
        state.usage = state.usage.plus(reply.usage)
        on_reply(state.calls, reply)
        record: dict[str, Any] = {"attempt": attempt + 1}
        state.attempts.append(record)
        try:
            parts = merge.parse_parts(reply.text)
        except merge.PartsError as error:
            record["parse_error"] = str(error)
            problems = [f"the reply could not be read: {error}"]
        else:
            result = merge.merge_dashboard(original, after_spec, changes, parts)
            record.update(placed=result.placed, missing=result.missing, ignored=result.ignored)
            if original is not None and result.dashboard == original:
                record["saved"] = False
                return state
            if target_uid:
                result.dashboard["uid"] = target_uid
            try:
                saved = await client.save_dashboard(result.dashboard, folder_uid, "Saved by the SlashObserve benchmark adapter")
                target_uid = saved.get("uid") or target_uid
                state.saved_uid = target_uid
                record["saved"] = True
                read_back = (await client.dashboard(target_uid))["dashboard"]
                state.summary = describe_save(original, read_back)
                failures = await check_panels(client, read_back, result.placed, window)
            except GrafanaError as error:
                record["grafana_error"] = str(error)
                state.error = str(error)
                return state
            record["failures"] = failures
            problems = [f"panel {spec_id}: {'; '.join(found)}" for spec_id, found in failures.items()]
            problems += [f"no part was written for {name}" for name in result.missing]
        if not problems:
            return state
        messages += [{"role": "assistant", "content": reply.text}, {"role": "user", "content": retry_text(problems)}]
    return state
