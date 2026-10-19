import type { WireFrame } from "@/dashboards/queries"

import { displayName, matchesName, withFields } from "./names"

export type FilterOptions = { include?: string[]; includeRegex?: string }

function compile(pattern: string): RegExp | "invalid" {
  try {
    return new RegExp(pattern)
  } catch {
    return "invalid"
  }
}

export function filterFieldsByName(
  frames: WireFrame[],
  { include, includeRegex }: FilterOptions
): WireFrame[] {
  const regex = includeRegex === undefined ? undefined : compile(includeRegex)
  const keepAll =
    (include === undefined && regex === undefined) || regex === "invalid"
  return frames.flatMap((frame) => {
    const fields = frame.fields.filter((field) => {
      if (keepAll) return true
      if (include?.some((name) => matchesName(frame, field, name))) return true
      return (
        regex !== undefined &&
        (regex.test(field.name) || regex.test(displayName(frame, field)))
      )
    })
    return fields.length > 0 || keepAll ? [withFields(frame, fields)] : []
  })
}
