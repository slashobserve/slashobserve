export type DiffChange = {
  op: "add" | "remove" | "replace" | "reorder"
  path: string
  from?: unknown
  to?: unknown
}

export type GroupedChange = {
  op: DiffChange["op"]
  field: string
  from?: unknown
  to?: unknown
}

export type DiffGroup = {
  key: string
  kind: "dashboard" | "panel" | "variable"
  label: string
  changes: GroupedChange[]
}

export type TitleMaps = {
  from: Record<string, string>
  to: Record<string, string>
}

export function unescapePointer(token: string): string {
  return token.replaceAll("~1", "/").replaceAll("~0", "~")
}

function ownTitle(map: Record<string, string>, id: string): string | undefined {
  return Object.hasOwn(map, id) ? map[id] : undefined
}

function pointerTokens(path: string): string[] {
  return path.split("/").slice(1).map(unescapePointer)
}

function innermostPanel(
  tokens: string[]
): { id: string; rest: string[] } | undefined {
  let i = 0
  let id: string | undefined
  while (tokens[i] === "panels" && tokens[i + 1] !== undefined) {
    id = tokens[i + 1]
    i += 2
  }
  return id === undefined ? undefined : { id, rest: tokens.slice(i) }
}

function panelField(op: DiffChange["op"], rest: string[]): string {
  if (op === "reorder" && rest.length === 1 && rest[0] === "panels") {
    return "panel order"
  }
  return rest.join(".")
}

export function groupChanges(
  changes: DiffChange[],
  titles: TitleMaps
): DiffGroup[] {
  const dashboard: DiffGroup = {
    key: "dashboard",
    kind: "dashboard",
    label: "Dashboard settings",
    changes: [],
  }
  const panels = new Map<string, DiffGroup>()
  const variables = new Map<string, DiffGroup>()

  for (const change of changes) {
    const { op, path } = change
    const payload = { from: change.from, to: change.to }
    const tokens = pointerTokens(path)
    const [head, name, ...rest] = tokens
    const panel = innermostPanel(tokens)

    if (path === "") {
      dashboard.changes.push({ op, field: "whole dashboard", ...payload })
    } else if (op === "reorder" && tokens.length === 1 && head === "panels") {
      dashboard.changes.push({ op, field: "panel order", ...payload })
    } else if (
      op === "reorder" &&
      tokens.length === 1 &&
      head === "variables"
    ) {
      dashboard.changes.push({ op, field: "variable order", ...payload })
    } else if (panel !== undefined) {
      let group = panels.get(panel.id)
      if (!group) {
        const title =
          ownTitle(titles.to, panel.id) ??
          ownTitle(titles.from, panel.id) ??
          panel.id
        group = {
          key: `panel:${panel.id}`,
          kind: "panel",
          label: `Panel "${title}"`,
          changes: [],
        }
        panels.set(panel.id, group)
      }
      group.changes.push({
        op,
        field: panelField(op, panel.rest),
        ...payload,
      })
    } else if (head === "variables" && name !== undefined) {
      let group = variables.get(name)
      if (!group) {
        group = {
          key: `variable:${name}`,
          kind: "variable",
          label: `Variable "${name}"`,
          changes: [],
        }
        variables.set(name, group)
      }
      group.changes.push({ op, field: rest.join("."), ...payload })
    } else {
      dashboard.changes.push({ op, field: tokens.join("."), ...payload })
    }
  }

  const inToOrder = Object.keys(titles.to).flatMap((id) => panels.get(id) ?? [])
  const removed = [...panels.entries()]
    .filter(([id]) => !Object.hasOwn(titles.to, id))
    .map(([, group]) => group)

  return [
    ...(dashboard.changes.length > 0 ? [dashboard] : []),
    ...inToOrder,
    ...removed,
    ...variables.values(),
  ]
}
