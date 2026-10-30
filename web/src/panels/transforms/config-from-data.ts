import { compileMatch } from "@/panels/field-config/match"
import type { Reducer, Transform } from "@/spec"

import { reduce } from "../reducers"
import {
  copyFrame,
  seriesName,
  type PreparedField,
  type PreparedFrame,
} from "./names"
import { configProps, type ConfigTarget } from "./rows-to-fields"

type ConfigFromDataOptions = Extract<Transform, { type: "configFromData" }>

const defaultReducer: Reducer = "lastNotNull"

function present(values: readonly unknown[]): unknown[] {
  return values.filter((value) => value !== null && value !== undefined)
}

function pick(field: PreparedField, reducer: Reducer): unknown {
  if (field.type === "number") return reduce(field.values, reducer)
  const values = present(field.values)
  if (reducer === "first" || reducer === "firstNotNull") return values[0]
  if (reducer === "last" || reducer === "lastNotNull") return values.at(-1)
  return reduce(values, reducer)
}

export function configFromData(
  frames: PreparedFrame[],
  { configQuery, mappings, applyTo }: ConfigFromDataOptions
): PreparedFrame[] {
  const source = frames.find((frame) => frame.ref === configQuery)
  if (!source) return frames.map(copyFrame)
  const entries = mappings.flatMap((mapping) => {
    const column = source.fields.find((field) => field.name === mapping.field)
    return column
      ? [
          [mapping.to, pick(column, mapping.reducer ?? defaultReducer)] as [
            ConfigTarget,
            unknown,
          ],
        ]
      : []
  })
  const config = configProps(entries)
  const matches = applyTo ? compileMatch(applyTo) : undefined
  return frames
    .filter((frame) => frame !== source)
    .map((frame) => {
      if (!config) return copyFrame(frame)
      return {
        ...frame,
        fields: frame.fields.map((field) => {
          const applies = matches
            ? matches({ frame, field, seriesName: seriesName(frame, field) })
            : field.type === "number"
          return applies
            ? { ...field, config: { ...field.config, ...config } }
            : field
        }),
      }
    })
}
