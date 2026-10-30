import type { Selections } from "@/api/keys"

import { copyFrame, type PreparedFrame } from "./names"
import { selectedValues, variableOfReference } from "./variables"

export type LimitOptions = { count: number | string }

function resolvedCount(
  count: number | string,
  vars: Selections | undefined
): number {
  if (typeof count === "number") return count
  const name = variableOfReference(count)
  const text =
    name === undefined ? undefined : selectedValues(vars, name)?.[0]?.trim()
  return text === undefined || text === "" ? NaN : Number(text)
}

export function limit(
  frames: PreparedFrame[],
  { count }: LimitOptions,
  vars?: Selections
): PreparedFrame[] {
  const resolved = resolvedCount(count, vars)
  if (!Number.isFinite(resolved)) return frames.map(copyFrame)
  const keep = Math.max(0, Math.floor(resolved))
  return frames.map((frame) => ({
    ...frame,
    fields: frame.fields.map((field) => ({
      ...field,
      values: field.values.slice(0, keep),
    })),
  }))
}
