import type { Transform } from "@/spec"

import { formatDate } from "@/lib/date-format"

import { matchesName, withFields, type PreparedFrame } from "./names"

type FormatTimeOptions = Extract<Transform, { type: "formatTime" }>

export function formatTime(
  frames: PreparedFrame[],
  { field: name, format, utc = false }: FormatTimeOptions
): PreparedFrame[] {
  return frames.map((frame) =>
    withFields(
      frame,
      frame.fields.map((field) =>
        field.type === "time" && matchesName(frame, field, name)
          ? {
              ...field,
              type: "string",
              values: field.values.map((value) =>
                typeof value === "number" && Number.isFinite(value)
                  ? formatDate(value, format, utc)
                  : null
              ),
            }
          : field
      )
    )
  )
}
