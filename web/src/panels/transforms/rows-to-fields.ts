import type { FieldProps, Transform } from "@/spec"

import { isNumber } from "../reducers"
import {
  cell,
  copyFrame,
  rowCount,
  matchesName,
  type PreparedField,
  type PreparedFrame,
} from "./names"

type RowsToFieldsOptions = Extract<Transform, { type: "rowsToFields" }>
type Mapping = NonNullable<RowsToFieldsOptions["mappings"]>[number]
export type ConfigTarget = Exclude<Mapping["to"], "name" | "value">

const numberTargets: readonly ConfigTarget[] = ["min", "max", "decimals"]

function asNumber(value: unknown): number | undefined {
  if (isNumber(value)) return value
  if (typeof value === "string" && value.trim() !== "") {
    const parsed = Number(value)
    return Number.isFinite(parsed) ? parsed : undefined
  }
  return undefined
}

export function configProps(
  entries: readonly [ConfigTarget, unknown][]
): FieldProps | undefined {
  const props: Record<string, number | string> = {}
  for (const [target, value] of entries) {
    if (numberTargets.includes(target)) {
      const number = asNumber(value)
      const valid =
        number !== undefined &&
        (target !== "decimals" ||
          (Number.isInteger(number) && number >= 0 && number <= 10))
      if (valid) props[target] = number
    } else if (typeof value === "string" && value !== "") {
      props[target] = value
    }
  }
  return Object.keys(props).length > 0 ? (props as FieldProps) : undefined
}

function columnFor(
  frame: PreparedFrame,
  mappings: readonly Mapping[],
  to: Mapping["to"]
): PreparedField | undefined {
  const mapping = mappings.find((candidate) => candidate.to === to)
  return mapping
    ? frame.fields.find((field) => matchesName(frame, field, mapping.field))
    : undefined
}

function toFields(
  frame: PreparedFrame,
  mappings: readonly Mapping[]
): PreparedFrame {
  const nameColumn =
    columnFor(frame, mappings, "name") ??
    frame.fields.find((field) => field.type === "string")
  const valueColumn =
    columnFor(frame, mappings, "value") ??
    frame.fields.find(
      (field) => field !== nameColumn && field.type === "number"
    ) ??
    frame.fields.find((field) => field !== nameColumn)
  if (!nameColumn || !valueColumn) return copyFrame(frame)
  const configured = mappings.flatMap((mapping) => {
    const column = frame.fields.find((field) =>
      matchesName(frame, field, mapping.field)
    )
    return column && mapping.to !== "name" && mapping.to !== "value"
      ? [{ target: mapping.to, column }]
      : []
  })
  const fields: PreparedField[] = []
  const count = rowCount(frame)
  for (let row = 0; row < count; row++) {
    const config = configProps(
      configured.map(({ target, column }) => [target, cell(column, row)])
    )
    fields.push({
      name: String(cell(nameColumn, row) ?? ""),
      type: valueColumn.type,
      values: [cell(valueColumn, row)],
      ...(config ? { config } : {}),
    })
  }
  return { ref: frame.ref, name: frame.name, fields }
}

export function rowsToFields(
  frames: PreparedFrame[],
  { mappings = [] }: RowsToFieldsOptions
): PreparedFrame[] {
  return frames.map((frame) => toFields(frame, mappings))
}
