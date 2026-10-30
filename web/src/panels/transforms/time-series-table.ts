import type { Reducer, Transform } from "@/spec"

import { reduce } from "../reducers"
import {
  copyFrame,
  firstTimeField,
  labelKeys,
  seriesName,
  type PreparedField,
  type PreparedFrame,
} from "./names"

type TimeSeriesTableOptions = Extract<Transform, { type: "timeSeriesTable" }>

const defaultStat: Reducer = "lastNotNull"

type Series = { frame: PreparedFrame; field: PreparedField }

function seriesOf(frame: PreparedFrame): Series[] {
  if (!firstTimeField(frame)) return []
  return frame.fields
    .filter((field) => field.type === "number")
    .map((field) => ({ frame, field }))
}

function tableOf(ref: string, members: Series[], stat: Reducer): PreparedFrame {
  const keys = [...new Set(members.flatMap(({ frame }) => labelKeys(frame)))]
  keys.sort()
  const identity: PreparedField[] =
    keys.length > 0
      ? keys.map((key) => ({
          name: key,
          type: "string",
          values: members.map(({ frame }) => frame.labels?.[key] ?? ""),
        }))
      : [
          {
            name: "Series",
            type: "string",
            values: members.map(({ frame, field }) => seriesName(frame, field)),
          },
        ]
  return {
    ref,
    fields: [
      ...identity,
      {
        name: `Trend #${ref}`,
        type: "other",
        values: members.map(({ field }) => field.values),
      },
      {
        name: `Value #${ref}`,
        type: "number",
        values: members.map(({ field }) => reduce(field.values, stat)),
      },
    ],
  }
}

export function timeSeriesTable(
  frames: PreparedFrame[],
  { stats = {} }: TimeSeriesTableOptions
): PreparedFrame[] {
  const groups = new Map<string, Series[]>()
  for (const frame of frames) {
    const members = seriesOf(frame)
    if (members.length > 0) {
      groups.set(frame.ref, [...(groups.get(frame.ref) ?? []), ...members])
    }
  }
  const emitted = new Set<string>()
  return frames.flatMap((frame) => {
    const members = groups.get(frame.ref)
    if (!members || seriesOf(frame).length === 0) return [copyFrame(frame)]
    if (emitted.has(frame.ref)) return []
    emitted.add(frame.ref)
    return [tableOf(frame.ref, members, stats[frame.ref] ?? defaultStat)]
  })
}
