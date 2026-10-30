import type { Transform } from "@/spec"

import { commonType, fillValue } from "./empty-value"
import {
  cell,
  copyFrame,
  matchesName,
  rowCount,
  type PreparedField,
  type PreparedFrame,
} from "./names"

type MatrixOptions = Extract<Transform, { type: "groupingToMatrix" }>

function pick(
  frame: PreparedFrame,
  name: string | undefined,
  position: number
): PreparedField | undefined {
  return name === undefined
    ? frame.fields[position]
    : frame.fields.find((field) => matchesName(frame, field, name))
}

function text(value: unknown): string {
  return value === null || value === undefined ? "" : String(value)
}

function matrix(frame: PreparedFrame, options: MatrixOptions): PreparedFrame {
  const column = pick(frame, options.column, 0)
  const row = pick(frame, options.row, 1)
  const value = pick(frame, options.value, 2)
  if (!column || !row || !value) return copyFrame(frame)
  const rowKeys = new Set<string>()
  const columnKeys = new Set<string>()
  const cells = new Map<string, unknown>()
  for (let at = 0; at < rowCount(frame); at++) {
    const rowKey = text(cell(row, at))
    const columnKey = text(cell(column, at))
    rowKeys.add(rowKey)
    columnKeys.add(columnKey)
    cells.set(JSON.stringify([rowKey, columnKey]), cell(value, at))
  }
  const fill = fillValue(options.empty)
  const columns = [...columnKeys].map((columnKey): PreparedField => {
    const values = [...rowKeys].map((rowKey) => {
      const found = cells.get(JSON.stringify([rowKey, columnKey]))
      return found === undefined ? fill : found
    })
    return { name: columnKey, type: commonType(values), values }
  })
  return {
    ref: frame.ref,
    name: frame.name,
    fields: [
      {
        name: `${row.name}\\${column.name}`,
        type: "string",
        values: [...rowKeys],
      },
      ...columns,
    ],
  }
}

export function groupingToMatrix(
  frames: PreparedFrame[],
  options: MatrixOptions
): PreparedFrame[] {
  return frames.map((frame) => matrix(frame, options))
}
