import type { WireFrame } from "@/dashboards/queries"
import type { Reducer } from "@/spec"

import type { WireField } from "../frames"
import { reduceField, reducerLabels } from "../reducers"
import { copyFrame, displayName, lastPresent, withFields } from "./names"

export type ReduceOptions = {
  reducers: Reducer[]
  mode?: "seriesToRows" | "reduceFields"
}

function reduceToRows(frames: WireFrame[], reducers: Reducer[]): WireFrame[] {
  if (frames.length === 0) return []
  const names: string[] = []
  const columns: (number | null)[][] = reducers.map(() => [])
  for (const frame of frames) {
    for (const field of frame.fields) {
      if (field.type !== "number") continue
      names.push(displayName(frame, field))
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
        ...reducers.map((reducer, index): WireField => ({
          name: reducerLabels[reducer],
          type: "number",
          values: columns[index],
        })),
      ],
    },
  ]
}

function reduceEachFrame(
  frames: WireFrame[],
  reducers: Reducer[]
): WireFrame[] {
  return frames.map((frame) => {
    const fields: WireField[] = []
    for (const field of frame.fields) {
      if (field.type === "number") {
        for (const reducer of reducers) {
          fields.push({
            name:
              reducers.length === 1
                ? field.name
                : `${field.name} ${reducerLabels[reducer]}`,
            type: "number",
            values: [reduceField(field, reducer)],
          })
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
  frames: WireFrame[],
  { reducers, mode = "seriesToRows" }: ReduceOptions
): WireFrame[] {
  if (reducers.length === 0) return frames.map(copyFrame)
  return mode === "reduceFields"
    ? reduceEachFrame(frames, reducers)
    : reduceToRows(frames, reducers)
}
