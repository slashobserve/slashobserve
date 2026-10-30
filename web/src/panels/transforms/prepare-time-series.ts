import type { Transform } from "@/spec"

import { joinByField } from "./join"
import {
  cell,
  copyFrame,
  firstTimeField,
  labelKeys,
  rowCount,
  type PreparedField,
  type PreparedFrame,
} from "./names"

type PrepareOptions = Extract<Transform, { type: "prepareTimeSeries" }>

function hasSeries(frame: PreparedFrame): boolean {
  return (
    firstTimeField(frame) !== undefined &&
    frame.fields.some((field) => field.type === "number")
  )
}

function splitNumbers(frame: PreparedFrame): PreparedFrame[] {
  const numbers = frame.fields.filter((field) => field.type === "number")
  if (numbers.length < 2) return [copyFrame(frame)]
  return numbers.map((number) => ({
    ...frame,
    fields: frame.fields.filter(
      (field) => field.type !== "number" || field === number
    ),
  }))
}

function toLong(frames: PreparedFrame[]): PreparedFrame {
  const labels = [...new Set(frames.flatMap(labelKeys))].sort()
  const names = [
    ...new Set(
      frames.flatMap((frame) =>
        frame.fields
          .filter((field) => field.type === "number")
          .map((field) => field.name)
      )
    ),
  ]
  const time: PreparedField = {
    name: firstTimeField(frames[0])?.name ?? "Time",
    type: "time",
    values: [],
  }
  const labelFields: PreparedField[] = labels.map((name) => ({
    name,
    type: "string",
    values: [],
  }))
  const valueFields: PreparedField[] = names.map((name) => ({
    name,
    type: "number",
    values: [],
  }))
  for (const frame of frames) {
    const source = firstTimeField(frame)
    const count = rowCount(frame)
    for (let row = 0; row < count; row++) {
      time.values.push(source ? cell(source, row) : null)
      labels.forEach((name, at) =>
        labelFields[at].values.push(frame.labels?.[name] ?? "")
      )
      names.forEach((name, at) => {
        const field = frame.fields.find(
          (candidate) => candidate.type === "number" && candidate.name === name
        )
        valueFields[at].values.push(field ? cell(field, row) : null)
      })
    }
  }
  return { ref: frames[0].ref, fields: [time, ...labelFields, ...valueFields] }
}

export function prepareTimeSeries(
  frames: PreparedFrame[],
  { format }: PrepareOptions
): PreparedFrame[] {
  const series = frames.filter(hasSeries)
  if (series.length === 0) return frames.map(copyFrame)
  const rest = frames.filter((frame) => !series.includes(frame)).map(copyFrame)
  switch (format) {
    case "wide":
      return [...joinByField(series, { mode: "outer" }), ...rest]
    case "many":
      return [...series.flatMap(splitNumbers), ...rest]
    case "long":
      return [toLong(series), ...rest]
  }
}
