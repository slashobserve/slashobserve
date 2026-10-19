import { compileNamePattern } from "@/spec"

import {
  copyFrame,
  seriesName,
  withFields,
  type PreparedField,
  type PreparedFrame,
} from "./names"

export type FilterOptions = { include?: string[]; includeRegex?: string }

export function filterFieldsByName(
  frames: PreparedFrame[],
  { include, includeRegex }: FilterOptions
): PreparedFrame[] {
  if (include === undefined && includeRegex === undefined) {
    return frames.map(copyFrame)
  }
  const names = new Set(include)
  const pattern =
    includeRegex === undefined ? null : compileNamePattern(includeRegex)
  const tested = new Map<string, boolean>()
  const matches = (text: string): boolean => {
    let result = tested.get(text)
    if (result === undefined) {
      result = pattern?.test(text) ?? false
      tested.set(text, result)
    }
    return result
  }
  return frames.flatMap((frame) => {
    const keep = (field: PreparedField): boolean => {
      const series = seriesName(frame, field)
      return (
        names.has(field.name) ||
        names.has(series) ||
        matches(field.name) ||
        matches(series)
      )
    }
    const fields = frame.fields.filter(keep)
    return fields.length > 0 ? [withFields(frame, fields)] : []
  })
}
