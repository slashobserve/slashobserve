import {
  cell,
  compareValues,
  copyFrame,
  matchesName,
  rowCount,
  type PreparedField,
  type PreparedFrame,
} from "./names"

export type SortOptions = { field: string; desc?: boolean }

function sortFrame(
  frame: PreparedFrame,
  { field, desc }: SortOptions
): PreparedFrame {
  const key: PreparedField | undefined = frame.fields.find((candidate) =>
    matchesName(frame, candidate, field)
  )
  if (!key) return copyFrame(frame)
  const order = Array.from({ length: rowCount(frame) }, (_, index) => index)
  order.sort((a, b) => {
    const left = cell(key, a)
    const right = cell(key, b)
    if (left === null || right === null) {
      return left === right ? 0 : left === null ? 1 : -1
    }
    return desc ? compareValues(right, left) : compareValues(left, right)
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
