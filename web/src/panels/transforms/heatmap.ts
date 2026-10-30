import type { Transform } from "@/spec"

import {
  alignOnTime,
  bucketSpec,
  calculatedModel,
  logBaseOf,
} from "../heatmap/heatmap-model"
import { copyFrame, type PreparedFrame } from "./names"

type HeatmapOptions = Extract<Transform, { type: "heatmap" }>

export function heatmap(
  frames: PreparedFrame[],
  options: HeatmapOptions
): PreparedFrame[] {
  const { xs, series } = alignOnTime(frames)
  const model = calculatedModel(
    series,
    xs,
    { unit: undefined, decimals: undefined },
    {
      x: bucketSpec(options.xBuckets),
      y: bucketSpec(options.yBuckets),
      scale: options.yScale ?? "linear",
      base: logBaseOf(options.yLogBase),
    }
  )
  const kept = options.keepOriginalData === true
  const copies = frames.map(copyFrame)
  if (model.rows.length === 0) return copies
  const xMin: number[] = []
  const yMin: number[] = []
  const yMax: number[] = []
  const count: number[] = []
  model.xs.forEach((x, column) => {
    model.rows.forEach((row, at) => {
      const cells = model.cells[at][column]
      if (!cells || row.from === undefined || row.to === undefined) return
      xMin.push(x)
      yMin.push(row.from)
      yMax.push(row.to)
      count.push(cells)
    })
  })
  const counted: PreparedFrame = {
    ref: frames[0].ref,
    fields: [
      { name: "xMin", type: "time", values: xMin },
      { name: "yMin", type: "number", values: yMin },
      { name: "yMax", type: "number", values: yMax },
      { name: "count", type: "number", values: count },
    ],
  }
  return kept ? [...copies, counted] : [counted]
}
