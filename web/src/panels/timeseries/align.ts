import { qualifyCollidingLabels, seriesKey, uniqueKey } from "../frames"
import type { PreparedField, PreparedFrame } from "../transforms"

export type AlignedSeries = {
  key: string
  label: string
  values: (number | null)[]
  frame: PreparedFrame
  field: PreparedField
}

export type AlignedFrames = {
  xs: number[]
  series: AlignedSeries[]
  ghosts: (number | null)[][]
}

type Column = {
  frame: PreparedFrame
  field: PreparedField
  times: unknown[]
}

function asNumber(value: unknown): number | null {
  return typeof value === "number" && Number.isFinite(value) ? value : null
}

function timeOf(frame: PreparedFrame): PreparedField | undefined {
  return frame.fields.find((field) => field.type === "time")
}

function columnsOf(frames: PreparedFrame[]): Column[] {
  return frames.flatMap((frame) => {
    const time = timeOf(frame)
    if (!time) return []
    return frame.fields
      .filter((field) => field.type === "number")
      .map((field) => ({ frame, field, times: time.values }))
  })
}

export function alignFrames(
  frames: PreparedFrame[],
  nameOf: (frame: PreparedFrame, field: PreparedField) => string,
  ghostFrames: PreparedFrame[] = []
): AlignedFrames {
  const timestamps = new Set<number>()
  for (const frame of [...frames, ...ghostFrames]) {
    for (const value of timeOf(frame)?.values ?? []) {
      if (typeof value === "number" && Number.isFinite(value)) {
        timestamps.add(value)
      }
    }
  }
  const sorted = [...timestamps].sort((a, b) => a - b)
  const index = new Map(sorted.map((ms, position) => [ms, position]))

  function alignColumn({ field, times }: Column): (number | null)[] {
    const aligned: (number | null)[] = new Array(sorted.length).fill(null)
    times.forEach((ms, row) => {
      const position = typeof ms === "number" ? index.get(ms) : undefined
      if (position !== undefined)
        aligned[position] = asNumber(field.values[row])
    })
    return aligned
  }

  const seen = new Map<string, number>()
  const series = qualifyCollidingLabels(
    columnsOf(frames).map((column) => ({
      ...column,
      ref: column.frame.ref,
      key: uniqueKey(seen, seriesKey(column.frame, column.field.name)),
      label: nameOf(column.frame, column.field),
    }))
  ).map((column) => ({
    key: column.key,
    label: column.label,
    values: alignColumn(column),
    frame: column.frame,
    field: column.field,
  }))

  return {
    xs: sorted.map((ms) => ms / 1000),
    series,
    ghosts: columnsOf(ghostFrames).map(alignColumn),
  }
}
