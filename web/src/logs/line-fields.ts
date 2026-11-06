import type { LogEntry } from "@/panels/logs/logs-model"

import { isSearchFamily } from "./search"

export const maxColumns = 20
const shownElsewhere = new Set([
  "time",
  "timens",
  "body",
  "message",
  "msg",
  "line",
  "log",
  "level",
])

export function fieldNames(entries: readonly LogEntry[], max = 200): string[] {
  const counts = new Map<string, number>()
  for (const entry of entries.slice(0, max)) {
    for (const detail of entry.details) {
      const { name } = detail
      if (shownElsewhere.has(name.toLowerCase()) || name.includes(",")) continue
      counts.set(name, (counts.get(name) ?? 0) + 1)
    }
  }
  return [...counts.keys()].sort(
    (a, b) => counts.get(b)! - counts.get(a)! || (a < b ? -1 : a > b ? 1 : 0)
  )
}

export function columnList(cols: string | undefined): string[] {
  return cols === undefined ? [] : cols.split(",")
}

export function columnsText(names: readonly string[]): string | undefined {
  return names.length === 0 ? undefined : names.join(",")
}

export function defaultColumns(
  datasourceType: string,
  names: readonly string[]
): string[] {
  return isSearchFamily(datasourceType) && names.includes("service")
    ? ["service"]
    : []
}

export function moveColumn(
  names: readonly string[],
  name: string,
  step: 1 | -1
): string[] {
  const from = names.indexOf(name)
  const to = from + step
  if (from < 0 || to < 0 || to >= names.length) return [...names]
  const moved = [...names]
  moved[from] = names[to]
  moved[to] = name
  return moved
}

function hashOf(text: string): string {
  let h1 = 0xdeadbeef
  let h2 = 0x41c6ce57
  for (let index = 0; index < text.length; index++) {
    const code = text.charCodeAt(index)
    h1 = Math.imul(h1 ^ code, 2654435761)
    h2 = Math.imul(h2 ^ code, 1597334677)
  }
  h1 =
    Math.imul(h1 ^ (h1 >>> 16), 2246822507) ^
    Math.imul(h2 ^ (h2 >>> 13), 3266489909)
  h2 =
    Math.imul(h2 ^ (h2 >>> 16), 2246822507) ^
    Math.imul(h1 ^ (h1 >>> 13), 3266489909)
  return (4294967296 * (2097151 & h2) + (h1 >>> 0)).toString(36)
}

export function lineId(entry: LogEntry): string | null {
  return entry.time === null
    ? null
    : `${Math.trunc(entry.time)}:${hashOf(entry.key)}`
}

export function linePermalink(href: string, entry: LogEntry): string {
  const id = lineId(entry)
  if (id === null) return href
  const url = new URL(href)
  url.searchParams.set("line", id)
  return url.toString()
}
