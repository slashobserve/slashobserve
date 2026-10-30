import { isExpressionQuery, queryRef, type DataPanel } from "@/spec"

import { uniqueKey } from "../frames"
import {
  copyFrame,
  rowCount,
  type PreparedField,
  type PreparedFrame,
} from "./names"

const nameKey = "__name__"

function firstTime(frame: PreparedFrame): PreparedField | undefined {
  return frame.fields.find((field) => field.type === "time")
}

export function flattenFrames(frames: PreparedFrame[]): PreparedFrame {
  const only = frames.length === 1 ? frames[0] : undefined
  if (
    only &&
    Object.keys(only.labels ?? {}).length === 0 &&
    only.name === undefined
  ) {
    return copyFrame(only)
  }

  const counts = frames.map(rowCount)
  const times = frames.map(firstTime)
  const fields: PreparedField[] = []
  const used = new Map<string, number>()

  if (times.some((time) => time !== undefined)) {
    uniqueKey(used, "Time")
    const values: unknown[] = []
    frames.forEach((_frame, index) => {
      const time = times[index]
      for (let row = 0; row < counts[index]; row++) {
        values.push(time ? (time.values[row] ?? null) : null)
      }
    })
    fields.push({ name: "Time", type: "time", values })
  }

  const byName = frames.map((frame) => {
    const named = new Map<string, PreparedField>()
    const local = new Map<string, number>()
    const time = firstTime(frame)
    for (const field of frame.fields) {
      if (field !== time) named.set(uniqueKey(local, field.name), field)
    }
    return named
  })
  const types = new Map<string, PreparedField["type"]>()
  for (const named of byName) {
    for (const [name, field] of named) {
      if (!types.has(name)) types.set(name, field.type)
    }
  }
  const columnNames = new Map(
    [...types.keys()].map((name) => [name, uniqueKey(used, name)])
  )
  const valueFields: PreparedField[] = []
  for (const [name, type] of types) {
    const values: unknown[] = []
    byName.forEach((named, index) => {
      const source = named.get(name)
      for (let row = 0; row < counts[index]; row++) {
        values.push(source ? (source.values[row] ?? null) : null)
      }
    })
    valueFields.push({ name: columnNames.get(name) ?? name, type, values })
  }

  const keys = new Set(
    frames.flatMap((frame) => Object.keys(frame.labels ?? {}))
  )
  if (frames.some((frame) => frame.name !== undefined)) keys.add(nameKey)
  for (const key of [...keys].sort()) {
    const values: unknown[] = []
    frames.forEach((frame, index) => {
      const value =
        key === nameKey
          ? (frame.name ?? frame.labels?.[nameKey] ?? null)
          : (frame.labels?.[key] ?? null)
      for (let row = 0; row < counts[index]; row++) values.push(value)
    })
    fields.push({ name: uniqueKey(used, key), type: "string", values })
  }

  fields.push(...valueFields)

  return { ref: frames[0].ref, fields }
}

function renameValues(frame: PreparedFrame, ref: string): PreparedFrame {
  return {
    ...frame,
    fields: frame.fields.map((field) =>
      field.name === "Value" ? { ...field, name: `Value #${ref}` } : field
    ),
  }
}

export function applyQueryFormats(
  panel: DataPanel,
  frames: PreparedFrame[]
): PreparedFrame[] {
  const tableRefs = new Set(
    panel.queries.flatMap((query, index) =>
      !isExpressionQuery(query) && query.format === "table"
        ? [queryRef(query, index)]
        : []
    )
  )
  if (tableRefs.size === 0) return frames.map(copyFrame)

  const groups = new Map<string, PreparedFrame[]>()
  for (const frame of frames) {
    if (frame.ref === undefined || !tableRefs.has(frame.ref)) continue
    const group = groups.get(frame.ref) ?? []
    group.push(frame)
    groups.set(frame.ref, group)
  }
  const several = groups.size > 1

  const emitted = new Set<string>()
  const out: PreparedFrame[] = []
  for (const frame of frames) {
    const ref = frame.ref
    if (ref === undefined || !groups.has(ref)) {
      out.push(copyFrame(frame))
      continue
    }
    if (emitted.has(ref)) continue
    emitted.add(ref)
    const table = flattenFrames(groups.get(ref) ?? [])
    out.push(several ? renameValues(table, ref) : table)
  }
  return out
}
