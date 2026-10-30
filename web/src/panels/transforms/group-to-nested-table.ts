import type { Transform } from "@/spec"

import { groupFrame } from "./group-by"
import {
  cell,
  copyFrame,
  type PreparedField,
  type PreparedFrame,
} from "./names"

type NestedOptions = Extract<Transform, { type: "groupToNestedTable" }>

const nestedFieldName = "Nested frames"

function nestedFrame(
  frame: PreparedFrame,
  keys: PreparedField[],
  rows: number[]
): PreparedFrame {
  return {
    ref: frame.ref,
    name: frame.name,
    fields: frame.fields
      .filter((field) => !keys.includes(field))
      .map((field) => ({
        ...field,
        values: rows.map((row) => cell(field, row)),
      })),
  }
}

export function groupToNestedTable(
  frames: PreparedFrame[],
  options: NestedOptions
): PreparedFrame[] {
  return frames.map((frame) => {
    const grouped = groupFrame(frame, options)
    if (!grouped) return copyFrame(frame)
    const nested: PreparedField = {
      name: nestedFieldName,
      type: "other",
      values: grouped.groups.map((group) =>
        nestedFrame(frame, grouped.keys, group.rows)
      ),
      nested: { headers: options.hideHeaders !== true },
    }
    return {
      ref: frame.ref,
      name: frame.name,
      fields: [...grouped.fields, nested],
    }
  })
}
