import type { WireFrame } from "@/dashboards/queries"

type Keep = (frame: WireFrame, row: number) => boolean

type RowSource = Pick<WireFrame, "labels"> & {
  fields: readonly Pick<WireFrame["fields"][number], "type" | "values">[]
}

function sortedLabelPairs(labels: WireFrame["labels"]): [string, string][] {
  return Object.entries(labels ?? {}).sort(([a], [b]) =>
    a < b ? -1 : a > b ? 1 : 0
  )
}

function frameRows(frame: RowSource): number {
  return frame.fields.reduce(
    (most, field) => Math.max(most, field.values.length),
    0
  )
}

export function rowKey(frame: RowSource, row: number): string {
  return JSON.stringify([
    sortedLabelPairs(frame.labels),
    ...frame.fields.map((field) => field.values[row]),
  ])
}

export function rowTime(frame: RowSource, row: number): number | null {
  const field = frame.fields.find((candidate) => candidate.type === "time")
  const value = field?.values[row]
  return typeof value === "number" ? value : null
}

export function newestTime(frames: readonly WireFrame[]): number | null {
  let newest: number | null = null
  for (const frame of frames) {
    for (let row = 0; row < frameRows(frame); row++) {
      const time = rowTime(frame, row)
      if (time !== null && (newest === null || time > newest)) newest = time
    }
  }
  return newest
}

export function rowCount(frames: readonly WireFrame[]): number {
  return frames.reduce((total, frame) => total + frameRows(frame), 0)
}

export function keepRows(
  frames: readonly WireFrame[],
  keep: Keep
): WireFrame[] {
  const out: WireFrame[] = []
  for (const frame of frames) {
    const rows: number[] = []
    for (let row = 0; row < frameRows(frame); row++) {
      if (keep(frame, row)) rows.push(row)
    }
    if (rows.length === 0) continue
    out.push({
      ...frame,
      fields: frame.fields.map((field) => ({
        ...field,
        values: rows.map((row) => field.values[row]),
      })),
    })
  }
  return out
}

export type TailBuffer = {
  frames: WireFrame[]
  keys: ReadonlySet<string>
  newest: number | null
}

export const emptyTail: TailBuffer = {
  frames: [],
  keys: new Set(),
  newest: null,
}

function oldestFirstTimes(frames: readonly WireFrame[]): number[] {
  const times: number[] = []
  for (const frame of frames) {
    for (let row = 0; row < frameRows(frame); row++) {
      times.push(rowTime(frame, row) ?? Number.NEGATIVE_INFINITY)
    }
  }
  return times.sort((a, b) => a - b)
}

function dropOldest(frames: WireFrame[], cap: number): WireFrame[] {
  const excess = rowCount(frames) - cap
  if (excess <= 0) return frames
  const dropped = oldestFirstTimes(frames).slice(0, excess)
  const cutoff = dropped[excess - 1]
  let atCutoff = dropped.filter((time) => time === cutoff).length
  return keepRows(frames, (frame, row) => {
    const time = rowTime(frame, row) ?? Number.NEGATIVE_INFINITY
    if (time < cutoff) return false
    if (time === cutoff && atCutoff > 0) {
      atCutoff--
      return false
    }
    return true
  })
}

export function mergeTail(
  buffer: TailBuffer,
  incoming: readonly WireFrame[],
  cap = 1000
): TailBuffer {
  const keys = new Set(buffer.keys)
  const fresh = keepRows(incoming, (frame, row) => {
    const key = rowKey(frame, row)
    if (keys.has(key)) return false
    keys.add(key)
    return true
  })
  const frames = dropOldest([...buffer.frames, ...fresh], cap)
  const kept = new Set<string>()
  for (const frame of frames) {
    for (let row = 0; row < frameRows(frame); row++)
      kept.add(rowKey(frame, row))
  }
  return {
    frames,
    keys: kept,
    newest: newestTime(frames) ?? buffer.newest,
  }
}

export function tailWindow(
  newest: number | null,
  now: number
): { from: string; to: string } {
  const start = newest ?? now - 10_000
  return {
    from: String(start),
    to: String(Math.max(now, (newest ?? now) + 1000)),
  }
}
