import type { Reducer } from "@/spec"

import { reduceField, reducerLabels } from "../reducers"
import { nameMatcher } from "./filter"
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
  includeTime?: boolean
  labelsToFields?: boolean
  fields?: string[]
  fieldsRegex?: string
}

const timeReducers: ReadonlySet<Reducer> = new Set([
  "last",
  "lastNotNull",
  "first",
  "firstNotNull",
  "min",
  "max",
  "mean",
  "median",
])

function reducibleFields(
  frame: PreparedFrame,
  { includeTime, fields, fieldsRegex }: ReduceOptions
): PreparedField[] {
  const selected =
    fields === undefined && fieldsRegex === undefined
      ? undefined
      : nameMatcher(fields ?? [], fieldsRegex)
  return frame.fields.filter((field) => {
    if (field.type === "time") return includeTime === true
    return (
      field.type === "number" &&
      (selected === undefined || selected(frame, field))
    )
  })
}

function labelColumns(rows: { frame: PreparedFrame }[]): PreparedField[] {
  const keys = [
    ...new Set(rows.flatMap(({ frame }) => Object.keys(frame.labels ?? {}))),
  ].sort()
  return keys.map((key) => ({
    name: key,
    type: "string",
    values: rows.map(({ frame }) => frame.labels?.[key] ?? null),
  }))
}

function reduceToRows(
  frames: PreparedFrame[],
  options: ReduceOptions
): PreparedFrame[] {
  if (frames.length === 0) return []
  const { reducers } = options
  const rows: { frame: PreparedFrame; field: PreparedField }[] = []
  for (const frame of frames) {
    for (const field of reducibleFields(frame, options)) {
      rows.push({ frame, field })
    }
  }
  return [
    {
      ref: frames[0].ref,
      fields: [
        {
          name: "Field",
          type: "string",
          values: rows.map(({ frame, field }) => seriesName(frame, field)),
        },
        ...(options.labelsToFields ? labelColumns(rows) : []),
        ...reducers.map((reducer): PreparedField => ({
          name: reducerLabels[reducer],
          type: "number",
          values: rows.map(({ field }) => reduceField(field, reducer)),
        })),
      ],
    },
  ]
}

function reduceEachFrame(
  frames: PreparedFrame[],
  options: ReduceOptions
): PreparedFrame[] {
  const { reducers } = options
  return frames.map((frame) => {
    const reducible = new Set(reducibleFields(frame, options))
    const fields: PreparedField[] = []
    for (const field of frame.fields) {
      if (reducible.has(field)) {
        for (const reducer of reducers) {
          const label = reducerLabels[reducer]
          const reduced: PreparedField = {
            name: reducers.length === 1 ? field.name : `${field.name} ${label}`,
            type:
              field.type === "time" && timeReducers.has(reducer)
                ? "time"
                : "number",
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
    if (options.labelsToFields) {
      for (const column of labelColumns([{ frame }])) {
        if (!fields.some((field) => field.name === column.name)) {
          fields.push(column)
        }
      }
    }
    return withFields(frame, fields)
  })
}

export function reduceFrames(
  frames: PreparedFrame[],
  options: ReduceOptions
): PreparedFrame[] {
  if (options.reducers.length === 0) return frames.map(copyFrame)
  return options.mode === "reduceFields"
    ? reduceEachFrame(frames, options)
    : reduceToRows(frames, options)
}
