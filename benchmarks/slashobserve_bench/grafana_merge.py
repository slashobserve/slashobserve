import copy
import json
import re
from dataclasses import dataclass, field
from typing import Any

GRID_WIDTH = 24
DEFAULT_PANEL_SIZE = (12, 8)
NEW_DASHBOARD_SCHEMA_VERSION = 39

_IMPORTED_ID = re.compile(r"^p(\d+)$")


@dataclass
class SpecChanges:
    added_panels: list[dict[str, Any]] = field(default_factory=list)
    changed_panels: list[tuple[dict[str, Any], dict[str, Any]]] = field(default_factory=list)
    removed_panels: list[dict[str, Any]] = field(default_factory=list)
    added_variables: list[dict[str, Any]] = field(default_factory=list)
    changed_variables: list[tuple[dict[str, Any], dict[str, Any]]] = field(default_factory=list)
    removed_variables: list[dict[str, Any]] = field(default_factory=list)

    @property
    def empty(self) -> bool:
        return not (
            self.added_panels
            or self.changed_panels
            or self.removed_panels
            or self.added_variables
            or self.changed_variables
            or self.removed_variables
        )

    @property
    def written_panel_ids(self) -> list[str]:
        return [panel["id"] for panel in self.added_panels] + [after["id"] for _, after in self.changed_panels]

    @property
    def written_variable_names(self) -> list[str]:
        return [variable["name"] for variable in self.added_variables] + [after["name"] for _, after in self.changed_variables]


@dataclass
class ModelParts:
    panels: dict[str, dict[str, Any]] = field(default_factory=dict)
    variables: list[dict[str, Any]] = field(default_factory=list)
    annotations: list[dict[str, Any]] = field(default_factory=list)


@dataclass
class MergeResult:
    dashboard: dict[str, Any]
    placed: dict[str, int]
    missing: list[str]
    ignored: list[str]


class PartsError(Exception):
    pass


def flat_spec_panels(spec: dict[str, Any] | None) -> dict[str, dict[str, Any]]:
    panels: dict[str, dict[str, Any]] = {}
    for panel in (spec or {}).get("panels") or []:
        panels[panel["id"]] = panel
        if panel.get("type") == "row":
            for child in panel.get("panels") or []:
                panels[child["id"]] = child
    return panels


def _comparable(panel: dict[str, Any]) -> dict[str, Any]:
    drop = {"gridPos"}
    if panel.get("type") == "row":
        drop.add("panels")
    return {key: value for key, value in panel.items() if key not in drop}


def spec_changes(before: dict[str, Any] | None, after: dict[str, Any]) -> SpecChanges:
    changes = SpecChanges()
    old_panels = flat_spec_panels(before)
    new_panels = flat_spec_panels(after)
    for panel_id, panel in new_panels.items():
        old = old_panels.get(panel_id)
        if old is None:
            changes.added_panels.append(panel)
        elif _comparable(old) != _comparable(panel):
            changes.changed_panels.append((old, panel))
    changes.removed_panels = [panel for panel_id, panel in old_panels.items() if panel_id not in new_panels]
    old_variables = {variable["name"]: variable for variable in (before or {}).get("variables") or []}
    new_variables = {variable["name"]: variable for variable in after.get("variables") or []}
    for name, variable in new_variables.items():
        old = old_variables.get(name)
        if old is None:
            changes.added_variables.append(variable)
        elif old != variable:
            changes.changed_variables.append((old, variable))
    changes.removed_variables = [variable for name, variable in old_variables.items() if name not in new_variables]
    return changes


def grafana_id_of(spec_panel_id: str) -> int | None:
    match = _IMPORTED_ID.match(spec_panel_id)
    return int(match.group(1)) if match else None


def _panel_lists(dashboard: dict[str, Any]) -> list[list[dict[str, Any]]]:
    top = dashboard.setdefault("panels", [])
    lists = [top]
    for panel in top:
        if panel.get("type") == "row" and isinstance(panel.get("panels"), list):
            lists.append(panel["panels"])
    return lists


def _find_panel(dashboard: dict[str, Any], grafana_id: int) -> tuple[list[dict[str, Any]], int] | None:
    for panels in _panel_lists(dashboard):
        for index, panel in enumerate(panels):
            if panel.get("id") == grafana_id:
                return panels, index
    return None


def _all_ids(dashboard: dict[str, Any]) -> list[int]:
    return [panel["id"] for panels in _panel_lists(dashboard) for panel in panels if isinstance(panel.get("id"), int)]


def _bottom(dashboard: dict[str, Any]) -> int:
    bottom = 0
    for panel in dashboard.get("panels") or []:
        pos = panel.get("gridPos") or {}
        bottom = max(bottom, int(pos.get("y", 0)) + int(pos.get("h", 0)))
    return bottom


def _size(panel: dict[str, Any]) -> tuple[int, int]:
    pos = panel.get("gridPos") or {}
    width, height = DEFAULT_PANEL_SIZE
    if isinstance(pos.get("w"), int) and 1 <= pos["w"] <= GRID_WIDTH:
        width = pos["w"]
    if isinstance(pos.get("h"), int) and pos["h"] >= 1:
        height = pos["h"]
    return width, height


def new_dashboard(spec: dict[str, Any]) -> dict[str, Any]:
    dashboard: dict[str, Any] = {
        "title": spec["title"],
        "panels": [],
        "templating": {"list": []},
        "annotations": {"list": []},
        "schemaVersion": NEW_DASHBOARD_SCHEMA_VERSION,
        "time": dict(spec.get("time") or {"from": "now-6h", "to": "now"}),
    }
    if spec.get("description"):
        dashboard["description"] = spec["description"]
    if spec.get("tags"):
        dashboard["tags"] = list(spec["tags"])
    return dashboard


def _place_panels(dashboard: dict[str, Any], changes: SpecChanges, parts: ModelParts, result: MergeResult) -> None:
    for old, _ in changes.changed_panels:
        grafana_id = grafana_id_of(old["id"])
        part = parts.panels.get(old["id"])
        if part is None:
            result.missing.append(old["id"])
            continue
        found = None if grafana_id is None else _find_panel(dashboard, grafana_id)
        if found is None:
            changes.added_panels.append(old)
            continue
        panels, index = found
        replacement = copy.deepcopy(part)
        replacement["id"] = grafana_id
        if "gridPos" in panels[index]:
            replacement["gridPos"] = copy.deepcopy(panels[index]["gridPos"])
        if panels[index].get("type") == "row" and "panels" in panels[index]:
            replacement["panels"] = panels[index]["panels"]
        panels[index] = replacement
        result.placed[old["id"]] = grafana_id
    next_id = max(_all_ids(dashboard), default=0) + 1
    x, y, row_height = 0, _bottom(dashboard), 0
    for panel in changes.added_panels:
        part = parts.panels.get(panel["id"])
        if part is None:
            result.missing.append(panel["id"])
            continue
        placed = copy.deepcopy(part)
        width, height = _size(placed)
        if x + width > GRID_WIDTH:
            x, y, row_height = 0, y + row_height, 0
        placed["id"] = next_id
        placed["gridPos"] = {"h": height, "w": width, "x": x, "y": y}
        dashboard["panels"].append(placed)
        result.placed[panel["id"]] = next_id
        next_id += 1
        x += width
        row_height = max(row_height, height)


def _remove_panels(dashboard: dict[str, Any], changes: SpecChanges) -> None:
    for panel in changes.removed_panels:
        grafana_id = grafana_id_of(panel["id"])
        found = None if grafana_id is None else _find_panel(dashboard, grafana_id)
        if found is not None:
            panels, index = found
            del panels[index]


def _merge_variables(dashboard: dict[str, Any], changes: SpecChanges, parts: ModelParts, result: MergeResult) -> None:
    wanted = set(changes.written_variable_names)
    by_name = {variable.get("name"): variable for variable in parts.variables}
    removed = {variable["name"] for variable in changes.removed_variables}
    if not (wanted or removed):
        return
    templating = dashboard.setdefault("templating", {})
    entries = templating.setdefault("list", [])
    entries[:] = [entry for entry in entries if entry.get("name") not in removed]
    for name in changes.written_variable_names:
        part = by_name.get(name)
        if part is None:
            result.missing.append(f"variable {name}")
            continue
        index = next((i for i, entry in enumerate(entries) if entry.get("name") == name), None)
        if index is None:
            entries.append(copy.deepcopy(part))
        else:
            entries[index] = copy.deepcopy(part)


def _merge_annotations(dashboard: dict[str, Any], parts: ModelParts) -> None:
    if not parts.annotations:
        return
    entries = dashboard.setdefault("annotations", {}).setdefault("list", [])
    for annotation in parts.annotations:
        index = next((i for i, entry in enumerate(entries) if entry.get("name") == annotation.get("name")), None)
        if index is None:
            entries.append(copy.deepcopy(annotation))
        else:
            entries[index] = copy.deepcopy(annotation)


def merge_dashboard(
    original: dict[str, Any] | None,
    after_spec: dict[str, Any],
    changes: SpecChanges,
    parts: ModelParts,
) -> MergeResult:
    dashboard = copy.deepcopy(original) if original is not None else new_dashboard(after_spec)
    result = MergeResult(dashboard=dashboard, placed={}, missing=[], ignored=[])
    wanted_panels = set(changes.written_panel_ids)
    result.ignored = sorted(set(parts.panels) - wanted_panels)
    wanted_variables = set(changes.written_variable_names)
    result.ignored += sorted(f"variable {v.get('name')}" for v in parts.variables if v.get("name") not in wanted_variables)
    pending = SpecChanges(
        added_panels=list(changes.added_panels),
        changed_panels=list(changes.changed_panels),
        removed_panels=list(changes.removed_panels),
        added_variables=list(changes.added_variables),
        changed_variables=list(changes.changed_variables),
        removed_variables=list(changes.removed_variables),
    )
    _remove_panels(dashboard, pending)
    _place_panels(dashboard, pending, parts, result)
    _merge_variables(dashboard, pending, parts, result)
    _merge_annotations(dashboard, parts)
    return result


def _json_object(text: str) -> Any:
    start, end = text.find("{"), text.rfind("}")
    if start < 0 or end < start:
        raise PartsError("the reply holds no JSON object")
    try:
        return json.loads(text[start : end + 1])
    except json.JSONDecodeError as error:
        raise PartsError(f"the reply's JSON does not parse: {error}") from None


def parse_parts(text: str) -> ModelParts:
    data = _json_object(text)
    if not isinstance(data, dict):
        raise PartsError("the reply's JSON is not an object")
    parts = ModelParts()
    panels = data.get("panels")
    if panels is not None and not isinstance(panels, list):
        raise PartsError("panels must be a list")
    for entry in panels or []:
        if not isinstance(entry, dict) or not isinstance(entry.get("spec_id"), str) or not isinstance(entry.get("panel"), dict):
            raise PartsError("each panel entry needs a string spec_id and a panel object")
        parts.panels[entry["spec_id"]] = entry["panel"]
    for key in ("variables", "annotations"):
        values = data.get(key)
        if values is None:
            values = []
        if not isinstance(values, list) or not all(isinstance(value, dict) and value.get("name") for value in values):
            raise PartsError(f"{key} must be a list of objects with a name")
        setattr(parts, key, values)
    return parts
