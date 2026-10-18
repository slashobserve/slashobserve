import type { WireFrame } from "@/dashboards/queries"
import { queryRef, type DataPanel } from "@/spec"

export type WireField = WireFrame["fields"][number]
export type ColumnType = WireField["type"]

export type TableColumn = { key: string; label: string; type: ColumnType }
export type TableData = {
  columns: TableColumn[]
  rows: Record<string, unknown>[]
}

const legendToken = /\{\{\s*([^{}\s]+)\s*\}\}/g

function escapeLabelValue(value: string): string {
  return value.replaceAll("\\", "\\\\").replaceAll('"', '\\"')
}

export function seriesLabel(
  frame: WireFrame,
  field: WireField,
  legend: string | undefined,
  numberFieldCount: number
): string {
  let label: string
  if (legend) {
    label = legend.replace(legendToken, (_match, key: string) =>
      key === "__name__" ? (frame.name ?? "") : (frame.labels?.[key] ?? "")
    )
  } else {
    const keys = Object.keys(frame.labels ?? {}).sort()
    if (keys.length > 0) {
      const pairs = keys.map(
        (key) => `${key}="${escapeLabelValue(frame.labels?.[key] ?? "")}"`
      )
      label = `${frame.name ?? ""}{${pairs.join(", ")}}`
    } else {
      label = frame.name ?? field.name
    }
  }
  return numberFieldCount > 1 ? `${label} ${field.name}` : label
}

export function legendFor(
  panel: DataPanel,
  frame: WireFrame
): string | undefined {
  return panel.queries.find(
    (query, index) => queryRef(query, index) === frame.ref
  )?.legend
}

export function seriesKey(frame: WireFrame, fieldName: string): string {
  const labels = Object.entries(frame.labels ?? {})
    .sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0))
    .map(([key, value]) => `${key}=${value}`)
    .join(",")
  return `${frame.ref}:${frame.name ?? ""}{${labels}}#${fieldName}`
}

export function uniqueKey(seen: Map<string, number>, base: string): string {
  const count = seen.get(base) ?? 0
  seen.set(base, count + 1)
  return count === 0 ? base : `${base}#${count}`
}

export function lastNumber(field: WireField): number | null {
  for (let i = field.values.length - 1; i >= 0; i--) {
    const value = field.values[i]
    if (typeof value === "number" && Number.isFinite(value)) return value
  }
  return null
}

function lastValue(field: WireField): unknown {
  if (field.type === "number") return lastNumber(field)
  for (let i = field.values.length - 1; i >= 0; i--) {
    const value = field.values[i]
    if (value !== null && value !== undefined) return value
  }
  return null
}

export type NumberSeries = {
  key: string
  label: string
  value: number | null
}

export function firstNumberSeries(
  frames: WireFrame[],
  legendOf: (frame: WireFrame) => string | undefined
): NumberSeries[] {
  const seen = new Map<string, number>()
  return frames.flatMap((frame) => {
    const numbers = frame.fields.filter((field) => field.type === "number")
    if (numbers.length === 0) return []
    return [
      {
        key: uniqueKey(seen, seriesKey(frame, numbers[0].name)),
        label: seriesLabel(frame, numbers[0], legendOf(frame), numbers.length),
        value: lastNumber(numbers[0]),
      },
    ]
  })
}

export function hasData(frames: WireFrame[]): boolean {
  return frames.some((frame) =>
    frame.fields.some(
      (field) =>
        field.type !== "time" &&
        field.values.some((value) => value !== null && value !== undefined)
    )
  )
}

function hasLabels(frame: WireFrame): boolean {
  return Object.keys(frame.labels ?? {}).length > 0
}

export function toTable(frames: WireFrame[]): TableData {
  if (frames.length === 1 && !hasLabels(frames[0])) {
    const frame = frames[0]
    const used = new Set<string>()
    const columns = frame.fields.map((field, index) => {
      let key = `field:${field.name}`
      if (used.has(key)) key = `${key}#${index}`
      used.add(key)
      return { key, label: field.name, type: field.type }
    })
    const rowCount = Math.max(0, ...frame.fields.map((f) => f.values.length))
    const rows: Record<string, unknown>[] = []
    for (let i = 0; i < rowCount; i++) {
      const row: Record<string, unknown> = {}
      frame.fields.forEach((field, index) => {
        row[columns[index].key] = field.values[i] ?? null
      })
      rows.push(row)
    }
    return { columns, rows }
  }

  const labelKeys = [
    ...new Set(frames.flatMap((frame) => Object.keys(frame.labels ?? {}))),
  ].sort()
  const valueFields = new Map<string, ColumnType>()
  for (const frame of frames) {
    for (const field of frame.fields) {
      if (field.type !== "time" && !valueFields.has(field.name)) {
        valueFields.set(field.name, field.type)
      }
    }
  }
  const columns: TableColumn[] = [
    ...labelKeys.map((key) => ({
      key: `label:${key}`,
      label: key,
      type: "string" as const,
    })),
    ...[...valueFields].map(([name, type]) => ({
      key: `field:${name}`,
      label: name,
      type,
    })),
  ]
  const rows = frames.map((frame) => {
    const row: Record<string, unknown> = {}
    for (const key of labelKeys)
      row[`label:${key}`] = frame.labels?.[key] ?? null
    for (const field of frame.fields) {
      if (field.type !== "time") row[`field:${field.name}`] = lastValue(field)
    }
    return row
  })
  return { columns, rows }
}
