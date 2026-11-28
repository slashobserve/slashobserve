import { flattenFrames } from "./format"
import {
  carriedProps,
  copyFrame,
  rowCount,
  type PreparedField,
  type PreparedFrame,
} from "./names"

type Column = { name: string; type: PreparedField["type"]; out: unknown[] }

type KeyPart = {
  column: Column
  parts: Map<unknown, number>
  folds: Map<number, number>
  lastValue: unknown
  lastPart: number
}

export function merge(frames: PreparedFrame[]): PreparedFrame[] {
  const flat = frames
    .filter((frame) => frame.fields.length > 0)
    .map((frame) => flattenFrames([frame]))
  const first = frames[0]
  if (first === undefined || flat.length < 2) return frames.map(copyFrame)

  const columns = new Map<string, Column>()
  const sources = new Map<string, PreparedField[]>()
  for (const frame of flat) {
    for (const field of frame.fields) {
      sources.set(field.name, [...(sources.get(field.name) ?? []), field])
      if (!columns.has(field.name)) {
        columns.set(field.name, { name: field.name, type: field.type, out: [] })
      }
    }
  }
  const columnList = [...columns.values()]
  const keyColumns = columnList.filter((column) =>
    flat.every((frame) => frame.fields.some((f) => f.name === column.name))
  )
  if (keyColumns.length === 0) return frames.map(copyFrame)

  const rowByKey = new Map<number, number>()
  let total = 0

  // Ids stay below capacity, so id * capacity + part is exact and unique.
  const capacity = flat.reduce((sum, frame) => sum + rowCount(frame), 1)
  const last = keyColumns.length - 1
  const unseen = {}
  const keys: KeyPart[] = keyColumns.map((column) => ({
    column,
    parts: new Map(),
    folds: new Map(),
    lastValue: unseen,
    lastPart: 0,
  }))

  for (const frame of flat) {
    const valuesOf = (column: Column) =>
      frame.fields.find((field) => field.name === column.name)?.values
    const cells = columnList.map((column) => ({
      column,
      values: valuesOf(column),
    }))
    const present = cells.flatMap(({ column, values }) =>
      values ? [{ column, values }] : []
    )
    const keyed = keys.map((state) => ({
      state,
      values: valuesOf(state.column) ?? [],
    }))
    const rows = rowCount(frame)
    for (let row = 0; row < rows; row++) {
      let key = 0
      for (const [k, { state, values }] of keyed.entries()) {
        const value = values[row] ?? null
        let part = state.lastPart
        if (value !== state.lastValue) {
          const found = state.parts.get(value)
          if (found === undefined) {
            part = state.parts.size
            state.parts.set(value, part)
          } else {
            part = found
          }
          state.lastValue = value
          state.lastPart = part
        }
        if (k === 0) {
          key = part
          continue
        }
        const pair = key * capacity + part
        if (k === last) {
          key = pair
          continue
        }
        let folded = state.folds.get(pair)
        if (folded === undefined) {
          folded = state.folds.size
          state.folds.set(pair, folded)
        }
        key = folded
      }

      const existing = rowByKey.get(key)
      let compatible = existing !== undefined
      if (existing !== undefined) {
        for (const { column, values } of present) {
          const current = column.out[existing]
          if (current !== null && current !== (values[row] ?? null)) {
            compatible = false
            break
          }
        }
      }
      if (existing !== undefined && compatible) {
        for (const { column, values } of present) {
          column.out[existing] = values[row] ?? null
        }
        continue
      }
      rowByKey.set(key, total)
      total++
      for (const { column, values } of cells) {
        column.out.push(values?.[row] ?? null)
      }
    }
  }

  return [
    {
      ref: first.ref,
      fields: columnList.map((column) => ({
        name: column.name,
        type: column.type,
        values: column.out,
        ...carriedProps(sources.get(column.name) ?? []),
      })),
    },
  ]
}
