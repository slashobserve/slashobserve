import type { Reducer } from "@/spec"

import { reduceField, reducerLabels } from "../reducers"
import {
  copyFrame,
  lastPresent,
  seriesName,
  withFields,
  type PreparedField,
  type PreparedFrame,
} from "./names"

export type ReduceOptions = {
  reducers: Reducer[]
  mode?: "seriesToRows" | "reduceFields"
}

function reduceToRows(
  frames: PreparedFrame[],
  reducers: Reducer[]
): PreparedFrame[] {
  if (frames.length === 0) return []
  const names: string[] = []
  const columns: (number | null)[][] = reducers.map(() => [])
  for (const frame of frames) {
    for (const field of frame.fields) {
      if (field.type !== "number") continue
      names.push(seriesName(frame, field))
      reducers.forEach((reducer, index) => {
        columns[index].push(reduceField(field, reducer))
      })
    }
  }
  return [
    {
      ref: frames[0].ref,
      fields: [
        { name: "Field", type: "string", values: names },
        ...reducers.map((reducer, index): PreparedField => ({
          name: reducerLabels[reducer],
          type: "number",
          values: columns[index],
        })),
      ],
    },
  ]
}

function reduceEachFrame(
  frames: PreparedFrame[],
  reducers: Reducer[]
): PreparedFrame[] {
  return frames.map((frame) => {
    const fields: PreparedField[] = []
    for (const field of frame.fields) {
      if (field.type === "number") {
        for (const reducer of reducers) {
          const label = reducerLabels[reducer]
          const reduced: PreparedField = {
            name: reducers.length === 1 ? field.name : `${field.name} ${label}`,
            type: "number",
            values: [reduceField(field, reducer)],
          }
          if (field.seriesName) {
            reduced.seriesName =
              reducers.length === 1
                ? field.seriesName
                : `${field.seriesName} ${label}`
          }
          fields.push(reduced)
        }
      } else if (field.type === "string") {
        fields.push({
          name: field.name,
          type: "string",
          values: [lastPresent(field.values)],
        })
      }
    }
    return withFields(frame, fields)
  })
}

export function reduceFrames(
  frames: PreparedFrame[],
  { reducers, mode = "seriesToRows" }: ReduceOptions
): PreparedFrame[] {
  if (reducers.length === 0) return frames.map(copyFrame)
  return mode === "reduceFields"
    ? reduceEachFrame(frames, reducers)
    : reduceToRows(frames, reducers)
}
