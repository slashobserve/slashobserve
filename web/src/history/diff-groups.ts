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

const reorderLabels: Record<string, string> = {
  panels: "panel order",
  queries: "query order",
  overrides: "override order",
}

function overrideField(rest: string[]): string | undefined {
  const separator = rest[1]?.indexOf(":") ?? -1
  if (rest[0] !== "overrides" || separator < 0) return undefined
  const kind = rest[1].slice(0, separator)
  const value = JSON.stringify(rest[1].slice(separator + 1))
  const field = rest.slice(2).join(".")
  return `override ${kind} ${value}${field === "" ? "" : ` > ${field}`}`
}

function panelField(op: DiffChange["op"], rest: string[]): string {
  if (
    op === "reorder" &&
    rest.length === 1 &&
    Object.hasOwn(reorderLabels, rest[0])
  ) {
    return reorderLabels[rest[0]]
  }
  return overrideField(rest) ?? rest.join(".")
}

function collectTitles(panels: unknown, titles: Record<string, string>) {
  if (!Array.isArray(panels)) return
  for (const panel of panels) {
    if (typeof panel !== "object" || panel === null) continue
    const {
      id,
      title,
      panels: children,
    } = panel as {
      id?: unknown
      title?: unknown
      panels?: unknown
    }
    if (typeof id === "string") {
      titles[id] = typeof title === "string" ? title : id
    }
    collectTitles(children, titles)
  }
}

export function panelTitles(spec: unknown): Record<string, string> {
  const titles: Record<string, string> = {}
  if (typeof spec !== "object" || spec === null) return titles
  collectTitles((spec as { panels?: unknown }).panels, titles)
  return titles
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
