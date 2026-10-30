import type { Transform } from "@/spec"

import { reduce, uniqueValues } from "../reducers"
import {
  cell,
  copyFrame,
  matchesName,
  rowCount,
  type PreparedField,
  type PreparedFrame,
} from "./names"
import { groupRows, type Group } from "./partition-by-values"

type GroupByOptions = Extract<Transform, { type: "groupBy" }>
export type Grouping = Pick<GroupByOptions, "by" | "aggregate">

export type Grouped = {
  keys: PreparedField[]
  groups: Group[]
  fields: PreparedField[]
}

function findField(
  frame: PreparedFrame,
  name: string
): PreparedField | undefined {
  return frame.fields.find((field) => matchesName(frame, field, name))
}

export function groupFrame(
  frame: PreparedFrame,
  { by = [], aggregate = [] }: Grouping
): Grouped | null {
  const keys = [...new Set(by.flatMap((name) => findField(frame, name) ?? []))]
  const aggregates = aggregate.flatMap(({ field, reducers }) => {
    const source = findField(frame, field)
    return source ? [{ source, reducers }] : []
  })
  const asked = by.length + aggregate.length > 0
  if (asked && keys.length === 0 && aggregates.length === 0) return null
  const groups = groupRows(keys, rowCount(frame))
  const fields: PreparedField[] = keys.map((key) => ({
    ...key,
    values: groups.map((group) => cell(key, group.rows[0])),
  }))
  for (const { source, reducers } of aggregates) {
    for (const reducer of reducers) {
      const name = `${source.name} (${reducer})`
      const column = (group: Group) =>
        group.rows.map((row) => cell(source, row))
      fields.push(
        reducer === "uniqueValues"
          ? {
              name,
              type: "string",
              values: groups.map((group) => uniqueValues(column(group))),
            }
          : {
              name,
              type: "number",
              values: groups.map((group) => reduce(column(group), reducer)),
            }
      )
    }
  }
  return { keys, groups, fields }
}

export function groupBy(
  frames: PreparedFrame[],
  options: GroupByOptions
): PreparedFrame[] {
  return frames.map((frame) => {
    const grouped = groupFrame(frame, options)
    return grouped
      ? { ref: frame.ref, name: frame.name, fields: grouped.fields }
      : copyFrame(frame)
  })
}
