import type { WireFrame } from "@/dashboards/queries"

import type { ColumnType, WireField } from "../frames"
import { cell, rowCount } from "./names"

type Column = { id: string; name: string; type: ColumnType; key: boolean }
type Source = Column & { at: (row: number) => unknown }

function sourcesOf(frame: WireFrame): Source[] {
  const sources: Source[] = []
  for (const key of Object.keys(frame.labels ?? {}).sort()) {
    const value = frame.labels?.[key] ?? null
    sources.push({
      id: `k:${key}`,
      name: key,
      type: "string",
      key: true,
      at: () => value,
    })
  }
  for (const field of frame.fields) {
    const key = field.type === "time" || field.type === "string"
    sources.push({
      id: `${key ? "k" : "v"}:${field.name}`,
      name: field.name,
      type: field.type,
      key,
      at: (row) => cell(field, row),
    })
  }
  return sources
}

function columnRank(column: Column): number {
  if (column.type === "time" && column.key) return 0
  return column.key ? 1 : 2
}

export function merge(frames: WireFrame[]): WireFrame[] {
  if (frames.length === 0) return []
  const columns = new Map<string, Column>()
  const rows: Map<string, unknown>[] = []
  const rowsByKey = new Map<string, Map<string, unknown>[]>()

  for (const frame of frames) {
    const sources = sourcesOf(frame)
    for (const source of sources) {
      if (!columns.has(source.id)) {
        columns.set(source.id, {
          id: source.id,
          name: source.name,
          type: source.type,
          key: source.key,
        })
      }
    }
    const seen = new Map<string, number>()
    for (let i = 0; i < rowCount(frame); i++) {
      const keyParts = sources
        .filter((source) => source.key)
        .map((source) => [source.id, source.at(i)] as const)
        .filter(([, value]) => value !== null)
        .sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0))
      const key = JSON.stringify(keyParts)
      const occurrence = seen.get(key) ?? 0
      seen.set(key, occurrence + 1)
      const sameKey = rowsByKey.get(key) ?? []
      rowsByKey.set(key, sameKey)
      let row = sameKey[occurrence]
      if (!row) {
        row = new Map()
        sameKey.push(row)
        rows.push(row)
      }
      for (const source of sources) {
        const value = source.at(i)
        if (row.get(source.id) == null) row.set(source.id, value)
      }
    }
  }

  const ordered = [...columns.values()]
    .map((column, index) => ({ column, index }))
    .sort(
      (a, b) => columnRank(a.column) - columnRank(b.column) || a.index - b.index
    )
    .map(({ column }) => column)
  const fields: WireField[] = ordered.map((column) => ({
    name: column.name,
    type: column.type,
    values: rows.map((row) => row.get(column.id) ?? null),
  }))
  return [{ ref: frames[0].ref, fields }]
}
