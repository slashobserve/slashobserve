import type { Transform } from "@/spec"

import { isNumber } from "../reducers"
import {
  cell,
  copyFrame,
  firstTimeField,
  labelKeys,
  rowCount,
  type PreparedField,
  type PreparedFrame,
} from "./names"

type JoinByLabelsOptions = Extract<Transform, { type: "joinByLabels" }>

type Row = { time: unknown; keys: string[]; cells: Map<string, unknown> }

function joinKeys(members: PreparedFrame[], value: string): string[] {
  const keys = new Set(members.flatMap(labelKeys))
  keys.delete(value)
  return [...keys].sort()
}

function byTime(a: Row, b: Row): number {
  const left = isNumber(a.time) ? a.time : Infinity
  const right = isNumber(b.time) ? b.time : Infinity
  return left === right ? 0 : left < right ? -1 : 1
}

export function joinByLabels(
  frames: PreparedFrame[],
  { value, on }: JoinByLabelsOptions
): PreparedFrame[] {
  const members = frames.filter(
    (frame) =>
      frame.labels?.[value] !== undefined &&
      frame.fields.some((field) => field.type === "number")
  )
  if (members.length === 0) return frames.map(copyFrame)
  const keys = on ?? joinKeys(members, value)
  const rows = new Map<string, Row>()
  const columns = new Map<string, PreparedField["type"]>()
  let timeName: string | undefined
  for (const frame of members) {
    const time = firstTimeField(frame)
    timeName ??= time?.name
    const numbers = frame.fields.filter((field) => field.type === "number")
    const keyValues = keys.map((key) => frame.labels?.[key] ?? "")
    for (const field of numbers) {
      const column =
        numbers.length > 1
          ? `${frame.labels?.[value]} ${field.name}`
          : (frame.labels?.[value] ?? field.name)
      columns.set(column, field.type)
      const count = rowCount(frame)
      for (let index = 0; index < count; index++) {
        const at = time ? cell(time, index) : null
        const id = JSON.stringify([at, keyValues])
        let row = rows.get(id)
        if (!row) {
          row = { time: at, keys: keyValues, cells: new Map() }
          rows.set(id, row)
        }
        row.cells.set(column, cell(field, index))
      }
    }
  }
  const ordered = [...rows.values()]
  if (timeName !== undefined) ordered.sort(byTime)
  const fields: PreparedField[] = [
    ...(timeName === undefined
      ? []
      : [
          {
            name: timeName,
            type: "time" as const,
            values: ordered.map((row) => row.time),
          },
        ]),
    ...keys.map((key, at) => ({
      name: key,
      type: "string" as const,
      values: ordered.map((row) => row.keys[at]),
    })),
    ...[...columns].map(([name, type]) => ({
      name,
      type,
      values: ordered.map((row) => row.cells.get(name) ?? null),
    })),
  ]
  const rest = frames.filter((frame) => !members.includes(frame))
  return [{ ref: members[0].ref, fields }, ...rest.map(copyFrame)]
}
