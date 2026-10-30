import type { Transform } from "@/spec"

import {
  copyFrame,
  labelKeys,
  rowCount,
  type PreparedField,
  type PreparedFrame,
} from "./names"

type LabelsToFieldsOptions = Extract<Transform, { type: "labelsToFields" }>

function shownLabels(
  frame: PreparedFrame,
  include: readonly string[] | undefined
): string[] {
  const keys = labelKeys(frame)
  return include === undefined
    ? keys
    : include.filter((key) => keys.includes(key))
}

function asColumns(
  frame: PreparedFrame,
  include: readonly string[] | undefined,
  valueLabel: string | undefined
): PreparedFrame {
  const labels = frame.labels ?? {}
  const named = valueLabel === undefined ? undefined : labels[valueLabel]
  const numbers = frame.fields.filter((field) => field.type === "number")
  const rename = named !== undefined && numbers.length === 1
  const rows = rowCount(frame)
  const fields: PreparedField[] = frame.fields.map((field) => {
    if (field.type !== "number") return field
    const name = rename ? named : field.name
    return { ...field, name, seriesName: name }
  })
  const labelFields: PreparedField[] = shownLabels(frame, include)
    .filter((key) => key !== valueLabel)
    .map((key) => ({
      name: key,
      type: "string",
      values: Array.from({ length: rows }, () => labels[key]),
    }))
  return { ...frame, labels: undefined, fields: [...fields, ...labelFields] }
}

function asRows(
  frame: PreparedFrame,
  include: readonly string[] | undefined
): PreparedFrame {
  const labels = frame.labels ?? {}
  const keys = shownLabels(frame, include)
  return {
    ref: frame.ref,
    name: frame.name,
    fields: [
      { name: "label", type: "string", values: keys },
      { name: "value", type: "string", values: keys.map((key) => labels[key]) },
    ],
  }
}

export function labelsToFields(
  frames: PreparedFrame[],
  { mode = "columns", include, valueLabel }: LabelsToFieldsOptions
): PreparedFrame[] {
  return frames.map((frame) => {
    if (labelKeys(frame).length === 0) return copyFrame(frame)
    return mode === "rows"
      ? asRows(frame, include)
      : asColumns(frame, include, valueLabel)
  })
}
