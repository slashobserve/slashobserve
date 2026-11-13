import type { WireFrame } from "@/dashboards/queries"
import { messageNames } from "@/panels/logs/logs-model"

type Keep = (frame: WireFrame, row: number) => boolean

type RowSource = Pick<WireFrame, "labels"> & {
  fields: readonly Pick<
    WireFrame["fields"][number],
    "name" | "type" | "values"
  >[]
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

function rowKey(frame: RowSource, row: number): string {
  const stamp =
    frame.fields.find((field) => field.name === "TimeNs") ??
    frame.fields.find((field) => field.type === "time")
  const body =
    frame.fields.find(
      (field) =>
        field.name !== "TimeNs" && messageNames.has(field.name.toLowerCase())
    ) ??
    frame.fields.find((field) => field.type === "string" && field !== stamp)
  const parts =
    stamp === undefined || body === undefined
      ? frame.fields.map((field) => field.values[row])
      : [stamp.values[row], body.values[row]]
  return JSON.stringify([sortedLabelPairs(frame.labels), ...parts])
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
  added: string[]
  received: number
  startedAt: number | null
}

export const emptyTail: TailBuffer = {
  frames: [],
  keys: new Set(),
  newest: null,
  added: [],
  received: 0,
  startedAt: null,
}

export function oldestFirstTimes(frames: readonly WireFrame[]): number[] {
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

function olderThan(
  frame: WireFrame,
  row: number,
  oldestMs: number | undefined
): boolean {
  if (oldestMs === undefined) return false
  const time = rowTime(frame, row)
  return time !== null && time < oldestMs
}

export function mergeTail(
  buffer: TailBuffer,
  incoming: readonly WireFrame[],
  cap = 1000,
  oldestMs?: number
): TailBuffer {
  const keys = new Set(buffer.keys)
  const freshKeys: string[] = []
  const fresh = keepRows(incoming, (frame, row) => {
    if (olderThan(frame, row, oldestMs)) return false
    const key = rowKey(frame, row)
    if (keys.has(key)) return false
    keys.add(key)
    freshKeys.push(key)
    return true
  })
  const current = keepRows(
    buffer.frames,
    (frame, row) => !olderThan(frame, row, oldestMs)
  )
  const frames = dropOldest([...current, ...fresh], cap)
  const kept = keysOf(frames)
  return {
    frames,
    keys: kept,
    newest: newestTime(frames) ?? buffer.newest,
    added: freshKeys.filter((key) => kept.has(key)),
    received: buffer.received + freshKeys.length,
    startedAt: buffer.startedAt,
  }
}

export function startTail(
  seed: readonly WireFrame[],
  now: number,
  oldestMs: number
): TailBuffer {
  return {
    ...mergeTail(emptyTail, seed, 1000, oldestMs),
    added: [],
    received: 0,
    startedAt: now,
  }
}

export function timesWhere(
  frames: readonly WireFrame[],
  matches: (key: string) => boolean
): Set<number> {
  const times = new Set<number>()
  for (const frame of frames) {
    for (let row = 0; row < frameRows(frame); row++) {
      const time = rowTime(frame, row)
      if (time !== null && matches(rowKey(frame, row))) times.add(time)
    }
  }
  return times
}

export function withoutKeys(
  frames: WireFrame[],
  hidden: ReadonlySet<string>
): WireFrame[] {
  if (hidden.size === 0) return frames
  return keepRows(frames, (frame, row) => !hidden.has(rowKey(frame, row)))
}

export function keysOf(frames: readonly WireFrame[]): Set<string> {
  const keys = new Set<string>()
  for (const frame of frames) {
    for (let row = 0; row < frameRows(frame); row++)
      keys.add(rowKey(frame, row))
  }
  return keys
}
