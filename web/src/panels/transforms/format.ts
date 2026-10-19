import type { WireFrame } from "@/dashboards/queries"
import { queryRef, type DataPanel } from "@/spec"

import { lastNumber, type WireField } from "../frames"
import { copyFrame, lastPresent } from "./names"

function isSeries(frame: WireFrame): boolean {
  return (
    frame.fields.some((field) => field.type === "time") ||
    Object.keys(frame.labels ?? {}).length > 0
  )
}

function tableOf(
  ref: string,
  series: WireFrame[],
  valueName: (name: string) => string
): WireFrame {
  const labelKeys = [
    ...new Set(series.flatMap((frame) => Object.keys(frame.labels ?? {}))),
  ].sort()
  const numberNames = [
    ...new Set(
      series.flatMap((frame) =>
        frame.fields
          .filter((field) => field.type === "number")
          .map((field) => field.name)
      )
    ),
  ]
  const fields: WireField[] = []
  if (series.some((frame) => frame.fields.some((f) => f.type === "time"))) {
    fields.push({
      name: "Time",
      type: "time",
      values: series.map((frame) => {
        const time = frame.fields.find((field) => field.type === "time")
        return time ? lastPresent(time.values) : null
      }),
    })
  }
  for (const key of labelKeys) {
    fields.push({
      name: key,
      type: "string",
      values: series.map((frame) => frame.labels?.[key] ?? null),
    })
  }
  for (const name of numberNames) {
    fields.push({
      name: valueName(name),
      type: "number",
      values: series.map((frame) => {
        const field = frame.fields.find(
          (candidate) => candidate.type === "number" && candidate.name === name
        )
        return field ? lastNumber(field) : null
      }),
    })
  }
  return { ref, fields }
}

export function applyQueryFormats(
  panel: DataPanel,
  frames: WireFrame[]
): WireFrame[] {
  const tableRefs = new Set(
    panel.queries.flatMap((query, index) =>
      query.format === "table" ? [queryRef(query, index)] : []
    )
  )
  if (tableRefs.size === 0) return frames.map(copyFrame)

  const several = panel.queries.length > 1
  const emitted = new Set<string>()
  const out: WireFrame[] = []
  for (const frame of frames) {
    if (!tableRefs.has(frame.ref) || !isSeries(frame)) {
      out.push(copyFrame(frame))
      continue
    }
    if (emitted.has(frame.ref)) continue
    emitted.add(frame.ref)
    const series = frames.filter(
      (candidate) => candidate.ref === frame.ref && isSeries(candidate)
    )
    out.push(
      tableOf(frame.ref, series, (name) =>
        several && name === "Value" ? `Value #${frame.ref}` : name
      )
    )
  }
  return out
}
