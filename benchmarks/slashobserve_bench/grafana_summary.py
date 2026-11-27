import json
from typing import Any

_QUERY_KEYS = ("expr", "query", "rawSql", "rawQuery")
_QUERY_MAX = 300


def _panels(dashboard: dict[str, Any] | None) -> dict[Any, dict[str, Any]]:
    found: dict[Any, dict[str, Any]] = {}
    for panel in (dashboard or {}).get("panels") or []:
        if not isinstance(panel, dict):
            continue
        found[panel.get("id")] = panel
        for nested in panel.get("panels") or []:
            if isinstance(nested, dict):
                found[nested.get("id")] = nested
    return found


def _named(dashboard: dict[str, Any] | None, section: str) -> dict[str, dict[str, Any]]:
    entries = ((dashboard or {}).get(section) or {}).get("list") or []
    return {str(entry.get("name")): entry for entry in entries if isinstance(entry, dict) and not entry.get("builtIn")}


def _datasource(part: dict[str, Any], fallback: dict[str, Any] | None = None) -> str:
    source = part.get("datasource") or (fallback or {}).get("datasource") or {}
    if isinstance(source, dict):
        return str(source.get("type") or source.get("uid") or "")
    return str(source)


def _query_text(part: dict[str, Any]) -> str:
    for key in _QUERY_KEYS:
        value = part.get(key)
        if isinstance(value, dict):
            value = value.get("query")
        if isinstance(value, str) and value.strip():
            text = " ".join(value.split())
            return text if len(text) <= _QUERY_MAX else text[:_QUERY_MAX] + "..."
    return ""


def _panel_line(panel: dict[str, Any]) -> str:
    line = f'"{panel.get("title") or "untitled"}" ({panel.get("type") or "panel"})'
    queries = []
    for target in panel.get("targets") or []:
        if not isinstance(target, dict):
            continue
        text = _query_text(target)
        if text:
            source = _datasource(target, panel)
            queries.append(f"{source}: `{text}`" if source else f"`{text}`")
    return line + (", querying " + "; ".join(queries) if queries else "")


def _entry_line(name: str, entry: dict[str, Any]) -> str:
    kind = entry.get("type") or _datasource(entry)
    text = _query_text(entry) or _query_text(entry.get("target") or {})
    return f'"{name}"' + (f" ({kind})" if kind else "") + (f": `{text}`" if text else "")


def _titles(panels: list[dict[str, Any]]) -> str:
    return ", ".join(f'"{panel.get("title") or "untitled"}"' for panel in panels)


def _same(a: Any, b: Any) -> bool:
    return json.dumps(a, sort_keys=True) == json.dumps(b, sort_keys=True)


def _section(lines: list[str], label: str, before: dict[str, Any], after: dict[str, Any]) -> None:
    added = [name for name in after if name not in before]
    changed = [name for name in after if name in before and not _same(before[name], after[name])]
    removed = [name for name in before if name not in after]
    for verb, names, source in (("added", added, after), ("changed", changed, after), ("removed", removed, before)):
        if names:
            lines.append(f"{label} {verb}: " + "; ".join(_entry_line(name, source[name]) for name in names) + ".")


def describe_save(original: dict[str, Any] | None, saved: dict[str, Any]) -> str:
    title, uid = saved.get("title") or "untitled", saved.get("uid") or ""
    before, after = _panels(original), _panels(saved)
    lines = []
    if original is None:
        lines.append(f'Created the Grafana dashboard "{title}" (uid {uid}) with {len(after)} panels.')
    else:
        lines.append(f'Saved the Grafana dashboard "{title}" (uid {uid}).')
    added = [panel for key, panel in after.items() if key not in before]
    changed = [panel for key, panel in after.items() if key in before and not _same(before[key], panel)]
    removed = [panel for key, panel in before.items() if key not in after]
    kept = [panel for key, panel in after.items() if key in before and _same(before[key], panel)]
    for verb, panels in (("added", added), ("changed", changed)):
        if panels:
            lines.append(f"Panels {verb}: " + "; ".join(_panel_line(panel) for panel in panels) + ".")
    if removed:
        lines.append(f"Panels removed: {_titles(removed)}.")
    if kept:
        lines.append(f"Panels left exactly as they were: {_titles(kept)}.")
    _section(lines, "Variables", _named(original, "templating"), _named(saved, "templating"))
    _section(lines, "Annotations", _named(original, "annotations"), _named(saved, "annotations"))
    if original is not None:
        other = sorted(
            key
            for key in set(original) | set(saved)
            if key not in ("panels", "templating", "annotations", "version", "id", "uid", "schemaVersion")
            and not _same(original.get(key), saved.get(key))
        )
        lines.append(f"Other dashboard settings changed: {', '.join(other)}." if other else "No other dashboard setting changed.")
    return "\n".join(lines)
