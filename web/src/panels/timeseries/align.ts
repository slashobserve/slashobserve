import { qualifyCollidingLabels, seriesKey, uniqueKey } from "../frames"
import type { PreparedField, PreparedFrame } from "../transforms"
import type { Cell } from "./stack"

export type AlignedSeries = {
  key: string
  label: string
  values: Cell[]
  frame: PreparedFrame
  field: PreparedField
}

export type AlignedFrames = {
  xs: number[]
  series: AlignedSeries[]
  ghosts: Cell[][]
}

type Column = {
  frame: PreparedFrame
  field: PreparedField
  times: unknown[]
}

export type XSource = {
  fieldOf: (frame: PreparedFrame) => PreparedField | undefined
  divisor: number
}

function asNumber(value: unknown): number | null {
  return typeof value === "number" && Number.isFinite(value) ? value : null
}

function timeOf(frame: PreparedFrame): PreparedField | undefined {
  return frame.fields.find((field) => field.type === "time")
}

const timeSource: XSource = { fieldOf: timeOf, divisor: 1000 }

function columnsOf(frames: PreparedFrame[], source: XSource): Column[] {
  return frames.flatMap((frame) => {
    const x = source.fieldOf(frame)
    if (!x) return []
    return frame.fields
      .filter((field) => field.type === "number" && field !== x)
      .map((field) => ({ frame, field, times: x.values }))
  })
}

export function alignFrames(
  frames: PreparedFrame[],
  nameOf: (frame: PreparedFrame, field: PreparedField) => string,
  ghostFrames: PreparedFrame[] = [],
  source: XSource = timeSource
): AlignedFrames {
  const timestamps = new Set<number>()
  for (const frame of [...frames, ...ghostFrames]) {
    for (const value of source.fieldOf(frame)?.values ?? []) {
      if (typeof value === "number" && Number.isFinite(value)) {
        timestamps.add(value)
      }
    }
  }
  const sorted = [...timestamps].sort((a, b) => a - b)
  const index = new Map(sorted.map((ms, position) => [ms, position]))

  function alignColumn({ field, times }: Column): Cell[] {
    const aligned: Cell[] = new Array(sorted.length).fill(undefined)
    times.forEach((ms, row) => {
      const position = typeof ms === "number" ? index.get(ms) : undefined
      if (position !== undefined)
        aligned[position] = asNumber(field.values[row])
    })
    return aligned
  }

  const seen = new Map<string, number>()
  const series = qualifyCollidingLabels(
    columnsOf(frames, source).map((column) => ({
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
    xs: sorted.map((x) => x / source.divisor),
    series,
    ghosts: columnsOf(ghostFrames, source).map(alignColumn),
  }
}
