import type { RowWindow } from "../traces/trace-model"

export const estimatedRowHeight = 28

export function rowOffsets(
  keys: readonly string[],
  heights: ReadonlyMap<string, number>,
  estimate = estimatedRowHeight
): number[] {
  const offsets = [0]
  for (const key of keys) {
    offsets.push(offsets[offsets.length - 1] + (heights.get(key) ?? estimate))
  }
  return offsets
}

function rowAt(offsets: readonly number[], offset: number): number {
  let low = 0
  let high = offsets.length - 1
  while (low < high) {
    const middle = Math.ceil((low + high) / 2)
    if (offsets[middle] <= offset) low = middle
    else high = middle - 1
  }
  return low
}

export function logWindow({
  offsets,
  scrollTop,
  viewport,
  overscan,
}: {
  offsets: readonly number[]
  scrollTop: number
  viewport: number
  overscan: number
}): RowWindow {
  const count = offsets.length - 1
  if (count <= 0) return { start: 0, end: 0, before: 0, after: 0 }
  const clamp = (index: number) => Math.min(Math.max(index, 0), count)
  const start = clamp(rowAt(offsets, scrollTop) - overscan)
  const end = clamp(rowAt(offsets, scrollTop + viewport) + overscan + 1)
  return {
    start,
    end,
    before: offsets[start],
    after: offsets[count] - offsets[end],
  }
}
