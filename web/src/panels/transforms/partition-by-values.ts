import type { Transform } from "@/spec"

import {
  cell,
  copyFrame,
  matchesName,
  rowCount,
  type PreparedField,
  type PreparedFrame,
} from "./names"

type PartitionOptions = Extract<Transform, { type: "partitionByValues" }>

type Group = { values: string[]; rows: number[] }

function groupRows(keys: PreparedField[], count: number): Group[] {
  const groups = new Map<string, Group>()
  for (let row = 0; row < count; row++) {
    const values = keys.map((key) => String(cell(key, row) ?? ""))
    const id = JSON.stringify(values)
    const group = groups.get(id) ?? { values, rows: [] }
    group.rows.push(row)
    groups.set(id, group)
  }
  return [...groups.values()]
}

function partition(
  frame: PreparedFrame,
  keys: PreparedField[],
  group: Group,
  { keepFields = false, naming = "labels" }: PartitionOptions
): PreparedFrame {
  const fields = frame.fields
    .filter((field) => keepFields || !keys.includes(field))
    .map((field) => ({
      ...field,
      values: group.rows.map((row) => cell(field, row)),
      seriesName: undefined,
    }))
  switch (naming) {
    case "name":
      return { ...frame, name: group.values.join(" "), fields }
    case "append":
      return {
        ...frame,
        name: [frame.name, ...group.values].filter(Boolean).join(" "),
        fields,
      }
    case "labels":
      return {
        ...frame,
        labels: {
          ...frame.labels,
          ...Object.fromEntries(
            keys.map((key, at) => [key.name, group.values[at]])
          ),
        },
        fields,
      }
  }
}

export function partitionByValues(
  frames: PreparedFrame[],
  options: PartitionOptions
): PreparedFrame[] {
  return frames.flatMap((frame) => {
    const keys = options.fields.map((name) =>
      frame.fields.find((field) => matchesName(frame, field, name))
    )
    if (keys.some((key) => key === undefined)) return [copyFrame(frame)]
    const found = keys.filter((key) => key !== undefined)
    const groups = groupRows(found, rowCount(frame))
    if (groups.length === 0) return [copyFrame(frame)]
    return groups.map((group) => partition(frame, found, group, options))
  })
}
