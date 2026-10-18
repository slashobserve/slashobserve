import type { WireFrame } from "@/dashboards/queries"

import {
  qualifyCollidingLabels,
  seriesKey,
  seriesLabel,
  uniqueKey,
} from "../frames"

export type AlignedSeries = {
  key: string
  label: string
  values: (number | null)[]
}

export type AlignedFrames = { xs: number[]; series: AlignedSeries[] }

function asNumber(value: unknown): number | null {
  return typeof value === "number" && Number.isFinite(value) ? value : null
}

export function alignFrames(
  frames: WireFrame[],
  legendOf: (frame: WireFrame) => string | undefined
): AlignedFrames {
  const timestamps = new Set<number>()
  const pending: {
    ref: string
    key: string
    label: string
    times: unknown[]
    values: unknown[]
  }[] = []
  const seen = new Map<string, number>()

  for (const frame of frames) {
    const time = frame.fields.find((field) => field.type === "time")
    if (!time) continue
    for (const value of time.values) {
      if (typeof value === "number" && Number.isFinite(value)) {
        timestamps.add(value)
      }
    }
    const numbers = frame.fields.filter((field) => field.type === "number")
    for (const field of numbers) {
      pending.push({
        ref: frame.ref,
        key: uniqueKey(seen, seriesKey(frame, field.name)),
        label: seriesLabel(frame, field, legendOf(frame), numbers.length),
        times: time.values,
        values: field.values,
      })
    }
  }

  const sorted = [...timestamps].sort((a, b) => a - b)
  const index = new Map(sorted.map((ms, position) => [ms, position]))
  const series = qualifyCollidingLabels(pending).map(
    ({ key, label, times, values }) => {
      const aligned: (number | null)[] = new Array(sorted.length).fill(null)
      times.forEach((ms, row) => {
        const position = typeof ms === "number" ? index.get(ms) : undefined
        if (position !== undefined) aligned[position] = asNumber(values[row])
      })
      return { key, label, values: aligned }
    }
  )

  return { xs: sorted.map((ms) => ms / 1000), series }
}
