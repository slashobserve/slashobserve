import type { Selections } from "@/api/keys"
import { compileNamePattern } from "@/spec"

import {
  copyFrame,
  seriesName,
  withFields,
  type PreparedField,
  type PreparedFrame,
} from "./names"
import { selectedValues } from "./variables"

export type FilterOptions = {
  include?: string[]
  includeRegex?: string
  exclude?: string[]
  excludeRegex?: string
  variable?: string
}

type Matcher = (frame: PreparedFrame, field: PreparedField) => boolean

export function nameMatcher(
  names: readonly string[],
  regex: string | undefined
): Matcher {
  const set = new Set(names)
  const pattern = regex === undefined ? null : compileNamePattern(regex)
  return (frame, field) => {
    const series = seriesName(frame, field)
    return (
      set.has(field.name) ||
      set.has(series) ||
      (pattern?.test(field.name) ?? false) ||
      (pattern?.test(series) ?? false)
    )
  }
}

export function filterFieldsByName(
  frames: PreparedFrame[],
  options: FilterOptions,
  vars?: Selections
): PreparedFrame[] {
  const fromVariable =
    options.variable === undefined
      ? undefined
      : selectedValues(vars, options.variable)
  const includeNames = [...(options.include ?? []), ...(fromVariable ?? [])]
  const hasInclude =
    includeNames.length > 0 || options.includeRegex !== undefined
  const hasExclude =
    options.exclude !== undefined || options.excludeRegex !== undefined
  if (!hasInclude && !hasExclude) return frames.map(copyFrame)
  const included = nameMatcher(includeNames, options.includeRegex)
  const excluded = nameMatcher(options.exclude ?? [], options.excludeRegex)
  return frames.flatMap((frame) => {
    const fields = frame.fields.filter(
      (field) =>
        (!hasInclude || included(frame, field)) &&
        !(hasExclude && excluded(frame, field))
    )
    return fields.length > 0 ? [withFields(frame, fields)] : []
  })
}
