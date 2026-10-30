import type { Transform } from "@/spec"

import {
  cell,
  rowCount,
  seriesName,
  type PreparedField,
  type PreparedFrame,
} from "./names"

type ConcatenateOptions = Extract<Transform, { type: "concatenate" }>

const defaultLabel = "frame"

function labelled(
  frame: PreparedFrame,
  field: PreparedField,
  options: ConcatenateOptions
): PreparedField {
  const { nameMode = "drop", nameLabel = defaultLabel } = options
  const frameName = frame.name ?? frame.ref
  if (nameMode === "field") {
    const name = `${frameName} ${field.name}`
    return {
      ...field,
      name,
      ...(field.type === "number" ? { seriesName: name } : {}),
    }
  }
  if (nameMode === "label" && field.type === "number") {
    const base = seriesName(frame, field)
    return { ...field, seriesName: `${base}{${nameLabel}="${frameName}"}` }
  }
  return field
}

export function concatenate(
  frames: PreparedFrame[],
  options: ConcatenateOptions
): PreparedFrame[] {
  if (frames.length === 0) return []
  const rows = Math.max(...frames.map(rowCount))
  const fields = frames.flatMap((frame) =>
    frame.fields.map((field) => {
      const named = labelled(frame, field, options)
      return {
        ...named,
        values: Array.from({ length: rows }, (_, row) => cell(field, row)),
      }
    })
  )
  return [{ ref: frames[0].ref, fields }]
}
