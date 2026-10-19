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
    } else if (frame.name !== undefined) {
      label = frame.name
    } else {
      return field.name
    }
  }
  return numberFieldCount > 1 ? `${label} ${field.name}` : label
}

export function qualifyCollidingLabels<
  T extends { ref: string; label: string },
>(series: T[]): T[] {
  const refsByLabel = new Map<string, Set<string>>()
  for (const { ref, label } of series) {
    const refs = refsByLabel.get(label) ?? new Set<string>()
    refs.add(ref)
    refsByLabel.set(label, refs)
  }
  return series.map((item) =>
    (refsByLabel.get(item.label)?.size ?? 0) > 1
      ? { ...item, label: `${item.ref}: ${item.label}` }
      : item
  )
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
  if (count === 0) {
    seen.set(base, 1)
    return base
  }
  let next = count
  while (seen.has(`${base}#${next}`)) next++
  const name = `${base}#${next}`
  seen.set(base, next + 1)
  seen.set(name, 1)
  return name
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
  const series = frames.flatMap((frame) => {
    const numbers = frame.fields.filter((field) => field.type === "number")
    if (numbers.length === 0) return []
    return [
      {
        ref: frame.ref,
        key: uniqueKey(seen, seriesKey(frame, numbers[0].name)),
        label: seriesLabel(frame, numbers[0], legendOf(frame), numbers.length),
        value: lastNumber(numbers[0]),
      },
    ]
  })
  return qualifyCollidingLabels(series).map(({ key, label, value }) => ({
    key,
    label,
    value,
  }))
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

const refColumn: TableColumn = {
  key: "query:ref",
  label: "Query",
  type: "string",
}

function unlabeledTable(frames: WireFrame[]): TableData {
  const withRef = new Set(frames.map((frame) => frame.ref)).size > 1
  const columns: TableColumn[] = withRef ? [refColumn] : []
  const known = new Set(columns.map((column) => column.key))
  const rows: Record<string, unknown>[] = []
  for (const frame of frames) {
    const used = new Set<string>()
    const keys = frame.fields.map((field, index) => {
      let key = `field:${field.name}`
      if (used.has(key)) key = `${key}#${index}`
      used.add(key)
      if (!known.has(key)) {
        known.add(key)
        columns.push({ key, label: field.name, type: field.type })
      }
      return key
    })
    const rowCount = Math.max(0, ...frame.fields.map((f) => f.values.length))
    for (let i = 0; i < rowCount; i++) {
      const row: Record<string, unknown> = withRef
        ? { [refColumn.key]: frame.ref }
        : {}
      frame.fields.forEach((field, index) => {
        row[keys[index]] = field.values[i] ?? null
      })
      rows.push(row)
    }
  }
  for (const row of rows) {
    for (const column of columns) row[column.key] ??= null
  }
  return { columns, rows }
}

export function toTable(frames: WireFrame[]): TableData {
  if (frames.length > 0 && !frames.some(hasLabels)) {
    return unlabeledTable(frames)
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
