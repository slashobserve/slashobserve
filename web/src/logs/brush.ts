export type BucketSpan = { start: number; end: number }
export type BucketSelection = { from: number; to: number }

function wholeSecond(ms: number): number {
  return Math.round(ms / 1000) * 1000
}

export function bucketAt(x: number, width: number, count: number): number {
  if (width <= 0 || count <= 0) return 0
  const clamped = Math.min(Math.max(x, 0), width)
  return Math.min(count - 1, Math.floor((clamped / width) * count))
}

export function selectionOf(
  fromIndex: number,
  toIndex: number
): BucketSelection | null {
  if (fromIndex === toIndex) return null
  return {
    from: Math.min(fromIndex, toIndex),
    to: Math.max(fromIndex, toIndex),
  }
}

export function brushRange(
  buckets: readonly BucketSpan[],
  fromIndex: number,
  toIndex: number
): { from: string; to: string } | null {
  const selection = selectionOf(fromIndex, toIndex)
  if (selection === null) return null
  const first = buckets[selection.from]
  const last = buckets[selection.to]
  if (first === undefined || last === undefined) return null
  return {
    from: new Date(wholeSecond(first.start)).toISOString(),
    to: new Date(wholeSecond(last.end)).toISOString(),
  }
}

export function bucketSpans(
  starts: readonly number[],
  stepMs: number
): BucketSpan[] {
  return starts.map((start) => ({ start, end: start + stepMs }))
}

export function walkBucket(
  key: string,
  current: number | null,
  count: number
): number | null {
  if (count <= 0) return null
  switch (key) {
    case "Home":
      return 0
    case "End":
      return count - 1
    case "ArrowRight":
      return Math.min(count - 1, (current ?? -1) + 1)
    case "ArrowLeft":
      return Math.max(0, (current ?? count) - 1)
    default:
      return null
  }
}

export function moveEdge(
  selection: BucketSelection,
  edge: "from" | "to",
  delta: -1 | 1,
  count: number
): BucketSelection {
  if (edge === "from") {
    const from = Math.min(Math.max(selection.from + delta, 0), selection.to - 1)
    return { from, to: selection.to }
  }
  const to = Math.min(
    Math.max(selection.to + delta, selection.from + 1),
    count - 1
  )
  return { from: selection.from, to }
}
