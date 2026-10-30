import type { Transform } from "@/spec"

import { commonType, fillValue } from "./empty-value"
import { cell, rowCount, type PreparedField, type PreparedFrame } from "./names"

type TransposeOptions = Extract<Transform, { type: "transpose" }>

const defaultFirstName = "Field"
const defaultRestName = "Value"

function transposed(
  frame: PreparedFrame,
  { firstName, restName, empty }: TransposeOptions
): PreparedFrame {
  const fill = fillValue(empty)
  const rest: PreparedField[] = []
  for (let row = 0; row < rowCount(frame); row++) {
    const values = frame.fields.map((field) => {
      const value = cell(field, row)
      return value === null ? fill : value
    })
    rest.push({
      name: `${restName ?? defaultRestName} ${row + 1}`,
      type: commonType(values),
      values,
    })
  }
  return {
    ref: frame.ref,
    name: frame.name,
    fields: [
      {
        name: firstName ?? defaultFirstName,
        type: "string",
        values: frame.fields.map((field) => field.name),
      },
      ...rest,
    ],
  }
}

export function transpose(
  frames: PreparedFrame[],
  options: TransposeOptions
): PreparedFrame[] {
  return frames.map((frame) => transposed(frame, options))
}
