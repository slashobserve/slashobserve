import { copyFrame, type PreparedFrame } from "./names"

export type LimitOptions = { count: number }

export function limit(
  frames: PreparedFrame[],
  { count }: LimitOptions
): PreparedFrame[] {
  if (!Number.isFinite(count)) return frames.map(copyFrame)
  const keep = Math.max(0, Math.floor(count))
  return frames.map((frame) => ({
    ...frame,
    fields: frame.fields.map((field) => ({
      ...field,
      values: field.values.slice(0, keep),
    })),
  }))
}
