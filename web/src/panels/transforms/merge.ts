import { flattenFrames } from "./format"
import {
  copyFrame,
  rowCount,
  type PreparedField,
  type PreparedFrame,
} from "./names"

type Column = { name: string; type: PreparedField["type"] }

export function merge(frames: PreparedFrame[]): PreparedFrame[] {
  const flat = frames
    .filter((frame) => frame.fields.length > 0)
    .map((frame) => flattenFrames([frame]))
  if (flat.length < 2) return frames.map(copyFrame)

  const columns = new Map<string, Column & { index: number }>()
  for (const frame of flat) {
    for (const field of frame.fields) {
      if (!columns.has(field.name)) {
        columns.set(field.name, {
          name: field.name,
          type: field.type,
          index: columns.size,
        })
      }
    }
  }
  const keyColumns = [...columns.values()]
    .filter((column) =>
      flat.every((frame) => frame.fields.some((f) => f.name === column.name))
    )
    .map((column) => column.index)
  if (keyColumns.length === 0) return frames.map(copyFrame)

  const out: unknown[][] = [...columns.values()].map(() => [])
  const rowByKey = new Map<number, number>()

  // Ids stay below capacity, so id * capacity + part is exact and unique.
  const capacity = flat.reduce((sum, frame) => sum + rowCount(frame), 1)
  const last = keyColumns.length - 1
  const parts = keyColumns.map(() => new Map<unknown, number>())
  const folds = keyColumns.map(() => new Map<number, number>())
  const unseen = {}
  const lastValue: unknown[] = keyColumns.map(() => unseen)
  const lastPart: number[] = keyColumns.map(() => 0)

  for (const frame of flat) {
    const values = [...columns.keys()].map(
      (name) => frame.fields.find((field) => field.name === name)?.values
    )
    const present: number[] = []
    values.forEach((column, index) => {
      if (column) present.push(index)
    })
    const keyValues = keyColumns.map((column) => values[column] ?? [])
    const rows = rowCount(frame)
    for (let row = 0; row < rows; row++) {
      let key = 0
      for (let k = 0; k <= last; k++) {
        const value = keyValues[k][row] ?? null
        let part = lastPart[k]
        if (value !== lastValue[k]) {
          const found = parts[k].get(value)
          if (found === undefined) {
            part = parts[k].size
            parts[k].set(value, part)
          } else {
            part = found
          }
          lastValue[k] = value
          lastPart[k] = part
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
        let folded = folds[k].get(pair)
        if (folded === undefined) {
          folded = folds[k].size
          folds[k].set(pair, folded)
        }
        key = folded
      }

      const existing = rowByKey.get(key)
      let compatible = existing !== undefined
      if (existing !== undefined) {
        for (const column of present) {
          const current = out[column][existing]
          if (current !== null && current !== (values[column]?.[row] ?? null)) {
            compatible = false
            break
          }
        }
      }
      if (existing !== undefined && compatible) {
        for (const column of present) {
          out[column][existing] = values[column]?.[row] ?? null
        }
        continue
      }
      rowByKey.set(key, out[0].length)
      for (let column = 0; column < out.length; column++) {
        out[column].push(values[column]?.[row] ?? null)
      }
    }
  }

  return [
    {
      ref: frames[0].ref,
      fields: [...columns.values()].map((column) => ({
        name: column.name,
        type: column.type,
        values: out[column.index],
      })),
    },
  ]
}
