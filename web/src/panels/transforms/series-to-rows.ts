import type { WireFrame } from "@/dashboards/queries"

import { cell, copyFrame, displayName, isNumber, rowCount } from "./names"

type Series = {
  frame: WireFrame
  time: number | null
  metric: string
  value: unknown
}

function asSeries(frame: WireFrame): Series[] | null {
  const times = frame.fields.filter((field) => field.type === "time")
  const numbers = frame.fields.filter((field) => field.type === "number")
  if (times.length === 0 || numbers.length !== 1) return null
  const metric = displayName(frame, numbers[0])
  const rows: Series[] = []
  for (let i = 0; i < rowCount(frame); i++) {
    const time = cell(times[0], i)
    rows.push({
      frame,
      time: isNumber(time) ? time : null,
      metric,
      value: cell(numbers[0], i),
    })
  }
  return rows
}

export function seriesToRows(frames: WireFrame[]): WireFrame[] {
  const rows: Series[] = []
  const rest: WireFrame[] = []
  let first: WireFrame | undefined
  for (const frame of frames) {
    const series = asSeries(frame)
    if (series === null) {
      rest.push(copyFrame(frame))
      continue
    }
    first ??= frame
    rows.push(...series)
  }
  if (!first) return rest
  rows.sort((a, b) => {
    if (a.time === null || b.time === null) {
      return a.time === b.time ? 0 : a.time === null ? 1 : -1
    }
    return b.time - a.time
  })
  const table: WireFrame = {
    ref: first.ref,
    fields: [
      { name: "Time", type: "time", values: rows.map((row) => row.time) },
      { name: "Metric", type: "string", values: rows.map((row) => row.metric) },
      { name: "Value", type: "number", values: rows.map((row) => row.value) },
    ],
  }
  return [table, ...rest]
}
