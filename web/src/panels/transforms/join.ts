import type { WireFrame } from "@/dashboards/queries"

import { uniqueKey, type WireField } from "../frames"
import {
  cell,
  copyFrame,
  displayName,
  isNumber,
  matchesName,
  rowCount,
} from "./names"

export type JoinOptions = { field?: string; mode?: "outer" | "inner" }

type Joined = { key: unknown; cells: Map<number, unknown> }

function keyFieldOf(frame: WireFrame, name: string): WireField | undefined {
  return (
    frame.fields.find((field) => field.name === name) ??
    frame.fields.find((field) => matchesName(frame, field, name))
  )
}

function compareKeys(a: unknown, b: unknown): number {
  if (isNumber(a) && isNumber(b)) return a - b
  return String(a).localeCompare(String(b))
}

export function joinByField(
  frames: WireFrame[],
  { field, mode = "outer" }: JoinOptions
): WireFrame[] {
  const keyName =
    field ??
    frames[0]?.fields.find((candidate) => candidate.type === "time")?.name
  if (keyName === undefined) return frames.map(copyFrame)

  const members = frames.flatMap((frame) => {
    const key = keyFieldOf(frame, keyName)
    return key ? [{ frame, key }] : []
  })
  if (members.length === 0) return frames.map(copyFrame)
  const others = frames.filter(
    (frame) => !members.some((member) => member.frame === frame)
  )

  const taken = new Map<string, number>()
  for (const { frame, key } of members) {
    for (const candidate of frame.fields) {
      if (candidate === key) continue
      taken.set(candidate.name, (taken.get(candidate.name) ?? 0) + 1)
    }
  }
  const used = new Map<string, number>()
  const columns: WireField[] = []
  const rows = new Map<string, Joined>()
  const presence = new Map<string, Set<number>>()

  members.forEach(({ frame, key }, memberIndex) => {
    const columnOf = new Map<WireField, number>()
    for (const candidate of frame.fields) {
      if (candidate === key) continue
      const shared = (taken.get(candidate.name) ?? 0) > 1
      const name = uniqueKey(
        used,
        shared ? displayName(frame, candidate) : candidate.name
      )
      columnOf.set(candidate, columns.length)
      columns.push({ name, type: candidate.type, values: [] })
    }
    const seen = new Map<string, number>()
    for (let i = 0; i < rowCount(frame); i++) {
      const keyValue = cell(key, i)
      if (keyValue === null) continue
      const base = `${typeof keyValue}:${String(keyValue)}`
      const occurrence = seen.get(base) ?? 0
      seen.set(base, occurrence + 1)
      const id = `${base}#${occurrence}`
      let joined = rows.get(id)
      if (!joined) {
        joined = { key: keyValue, cells: new Map() }
        rows.set(id, joined)
      }
      const present = presence.get(id) ?? new Set<number>()
      present.add(memberIndex)
      presence.set(id, present)
      for (const [candidate, column] of columnOf) {
        joined.cells.set(column, cell(candidate, i))
      }
    }
  })

  const kept = [...rows.entries()]
    .filter(
      ([id]) => mode === "outer" || presence.get(id)?.size === members.length
    )
    .map(([, joined]) => joined)
    .sort((a, b) => compareKeys(a.key, b.key))

  const joined: WireFrame = {
    ref: members[0].frame.ref,
    fields: [
      {
        name: members[0].key.name,
        type: members[0].key.type,
        values: kept.map((row) => row.key),
      },
      ...columns.map((column, index) => ({
        ...column,
        values: kept.map((row) => row.cells.get(index) ?? null),
      })),
    ],
  }
  return [joined, ...others.map(copyFrame)]
}
