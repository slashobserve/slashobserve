import type { Transform } from "@/spec"

import { bucketIndex, bucketsOf, edgesOf } from "../histogram/histogram-model"
import { isNumber } from "../reducers"
import { seriesName, type PreparedField, type PreparedFrame } from "./names"

type HistogramOptions = Extract<Transform, { type: "histogram" }>

const combinedName = "Count"

type Sample = { name: string; values: number[] }

function samples(frames: PreparedFrame[], combine: boolean): Sample[] {
  const found = frames.flatMap((frame) =>
    frame.fields
      .filter((field) => field.type === "number")
      .map((field) => ({
        name: seriesName(frame, field),
        values: field.values.filter(isNumber),
      }))
  )
  if (!combine || found.length === 0) return found
  return [{ name: combinedName, values: found.flatMap((s) => s.values) }]
}

export function histogram(
  frames: PreparedFrame[],
  options: HistogramOptions
): PreparedFrame[] {
  const found = samples(frames, options.combine === true)
  const all = found.flatMap((sample) => sample.values)
  if (all.length === 0) return frames.map((frame) => ({ ...frame }))
  const lowest = all.reduce((a, b) => Math.min(a, b))
  const highest = all.reduce((a, b) => Math.max(a, b))
  const buckets = bucketsOf(lowest, highest, options)
  const edges = edgesOf(buckets)
  const counts = found.map((sample): PreparedField => {
    const values = new Array<number>(buckets.count).fill(0)
    for (const value of sample.values) values[bucketIndex(value, buckets)]++
    return { name: sample.name, type: "number", values }
  })
  return [
    {
      ref: frames[0].ref,
      fields: [
        { name: "xMin", type: "number", values: edges.slice(0, -1) },
        { name: "xMax", type: "number", values: edges.slice(1) },
        ...counts,
      ],
    },
  ]
}
