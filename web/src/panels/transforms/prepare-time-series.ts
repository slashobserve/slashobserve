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

function emptyField(name: string, type: PreparedField["type"]): PreparedField {
  return { name, type, values: [] }
}

function toLong(
  frames: readonly [PreparedFrame, ...PreparedFrame[]]
): PreparedFrame {
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
  const labelFields = labels.map((name) => ({
    name,
    field: emptyField(name, "string"),
  }))
  const valueFields = names.map((name) => ({
    name,
    field: emptyField(name, "number"),
  }))
  for (const frame of frames) {
    const source = firstTimeField(frame)
    const count = rowCount(frame)
    for (let row = 0; row < count; row++) {
      time.values.push(source ? cell(source, row) : null)
      for (const { name, field } of labelFields) {
        field.values.push(frame.labels?.[name] ?? "")
      }
      for (const { name, field: out } of valueFields) {
        const field = frame.fields.find(
          (candidate) => candidate.type === "number" && candidate.name === name
        )
        out.values.push(field ? cell(field, row) : null)
      }
    }
  }
  return {
    ref: frames[0].ref,
    fields: [
      time,
      ...labelFields.map(({ field }) => field),
      ...valueFields.map(({ field }) => field),
    ],
  }
}

export function prepareTimeSeries(
  frames: PreparedFrame[],
  { format }: PrepareOptions
): PreparedFrame[] {
  const series = frames.filter(hasSeries)
  const [head, ...tail] = series
  if (head === undefined) return frames.map(copyFrame)
  const rest = frames.filter((frame) => !series.includes(frame)).map(copyFrame)
  switch (format) {
    case "wide":
      return [...joinByField(series, { mode: "outer" }), ...rest]
    case "many":
      return [...series.flatMap(splitNumbers), ...rest]
    case "long":
      return [toLong([head, ...tail]), ...rest]
  }
}
