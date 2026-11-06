import { downloadFile } from "@/dashboards/export-spec"
import { slugOf } from "@/lib/links"
import {
  cell,
  firstTimeField,
  rowCount,
  seriesName,
  type PreparedField,
  type PreparedFrame,
} from "@/panels/transforms/names"

export type DataTable = { columns: string[]; rows: string[][] }

const lineEnd = "\r\n"
const needsQuotes = /[",\r\n]/

function timeText(value: unknown): string {
  if (typeof value !== "number" || !Number.isFinite(value)) return ""
  return new Date(value).toISOString()
}

function valueText(field: PreparedField, value: unknown): string {
  if (field.type === "time") return timeText(value)
  if (value === null || value === undefined) return ""
  if (typeof value === "string") return value
  if (typeof value === "number" || typeof value === "boolean") {
    return String(value)
  }
  return JSON.stringify(value)
}

function frameLabel(frame: PreparedFrame): string {
  const number = frame.fields.find((field) => field.type === "number")
  if (number) return seriesName(frame, number)
  return frame.name ?? frame.ref
}

function joinsOnTime(frames: readonly PreparedFrame[]): boolean {
  return frames.every((frame) => {
    const times = frame.fields.filter((field) => field.type === "time")
    if (times.length !== 1) return false
    return new Set(times[0].values).size === times[0].values.length
  })
}

function singleFrame(frame: PreparedFrame): DataTable {
  const rows: string[][] = []
  for (let row = 0; row < rowCount(frame); row++) {
    rows.push(frame.fields.map((field) => valueText(field, cell(field, row))))
  }
  return {
    columns: frame.fields.map((field) => seriesName(frame, field)),
    rows,
  }
}

function rowsByTime(frame: PreparedFrame): Map<unknown, number> {
  const time = firstTimeField(frame)
  return new Map(time?.values.map((value, row) => [value, row]) ?? [])
}

function joined(frames: readonly PreparedFrame[]): DataTable {
  const columns = frames.flatMap((frame) => {
    const rows = rowsByTime(frame)
    return frame.fields
      .filter((field) => field.type !== "time")
      .map((field) => ({ name: seriesName(frame, field), field, rows }))
  })
  const times = [
    ...new Set(frames.flatMap((frame) => firstTimeField(frame)?.values ?? [])),
  ]
    .filter((value): value is number => typeof value === "number")
    .sort((a, b) => a - b)
  return {
    columns: [
      firstTimeField(frames[0])?.name ?? "Time",
      ...columns.map((column) => column.name),
    ],
    rows: times.map((time) => [
      timeText(time),
      ...columns.map(({ field, rows }) => {
        const row = rows.get(time)
        return row === undefined ? "" : valueText(field, cell(field, row))
      }),
    ]),
  }
}

function concatenated(frames: readonly PreparedFrame[]): DataTable {
  const names = [
    ...new Set(frames.flatMap((frame) => frame.fields.map((f) => f.name))),
  ]
  const rows: string[][] = []
  for (const frame of frames) {
    const label = frameLabel(frame)
    const byName = new Map(frame.fields.map((field) => [field.name, field]))
    for (let row = 0; row < rowCount(frame); row++) {
      rows.push([
        label,
        ...names.map((name) => {
          const field = byName.get(name)
          return field ? valueText(field, cell(field, row)) : ""
        }),
      ])
    }
  }
  return { columns: ["series", ...names], rows }
}

export function framesToTable(frames: readonly PreparedFrame[]): DataTable {
  if (frames.length === 0) return { columns: [], rows: [] }
  if (frames.length === 1) return singleFrame(frames[0])
  return joinsOnTime(frames) ? joined(frames) : concatenated(frames)
}

function quoted(text: string): string {
  return needsQuotes.test(text) ? `"${text.replaceAll('"', '""')}"` : text
}

function line(cells: readonly string[]): string {
  return cells.map(quoted).join(",") + lineEnd
}

export function framesToCsv(frames: readonly PreparedFrame[]): string {
  const table = framesToTable(frames)
  if (table.columns.length === 0) return ""
  return line(table.columns) + table.rows.map(line).join("")
}

export function panelFileName(title: string, extension: string): string {
  return `${slugOf(title) || "panel"}.${extension}`
}

export function downloadCsv(frames: readonly PreparedFrame[], title: string) {
  const blob = new Blob([framesToCsv(frames)], { type: "text/csv" })
  downloadFile(blob, panelFileName(title, "csv"))
}
