import {
  cell,
  compareValues,
  copyFrame,
  matchesName,
  rowCount,
  type PreparedField,
  type PreparedFrame,
} from "./names"

type SortKey = { field: string; desc?: boolean }
export type SortOptions = SortKey & { then?: SortKey[] }

function compareCells(a: unknown, b: unknown, desc: boolean | undefined) {
  if (a === null || b === null) return a === b ? 0 : a === null ? 1 : -1
  return desc ? compareValues(b, a) : compareValues(a, b)
}

function sortFrame(
  frame: PreparedFrame,
  { field, desc, then = [] }: SortOptions
): PreparedFrame {
  const keys = [{ field, desc }, ...then].flatMap((key) => {
    const found: PreparedField | undefined = frame.fields.find((candidate) =>
      matchesName(frame, candidate, key.field)
    )
    return found ? [{ field: found, desc: key.desc }] : []
  })
  if (keys.length === 0) return copyFrame(frame)
  const order = Array.from({ length: rowCount(frame) }, (_, index) => index)
  order.sort((a, b) => {
    for (const key of keys) {
      const result = compareCells(
        cell(key.field, a),
        cell(key.field, b),
        key.desc
      )
      if (result !== 0) return result
    }
    return 0
  })
  return {
    ...frame,
    fields: frame.fields.map((candidate) => ({
      ...candidate,
      values: order.map((row) => cell(candidate, row)),
    })),
  }
}

export function sortBy(
  frames: PreparedFrame[],
  options: SortOptions
): PreparedFrame[] {
  return frames.map((frame) => sortFrame(frame, options))
}
