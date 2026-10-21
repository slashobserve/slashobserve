import { formatTimeValue } from "@/lib/time"
import { commonDecimals } from "@/lib/units"
import type { Reducer } from "@/spec"

import {
  displayValue,
  fieldRange,
  isDataLink,
  resolveLink,
  type DisplayOptions,
  type FieldConfig,
  type FieldResolver,
  type LinkScope,
  type ResolvedLink,
} from "../field-config"
import type { ColumnType } from "../frames"
import { reduce, reducerLabels } from "../reducers"
import type { ThresholdLevel } from "../thresholds"
import type { PreparedField, PreparedFrame } from "../transforms"

export type ColumnSource = { frame: PreparedFrame; field: PreparedField }

export type TableColumn = {
  key: string
  label: string
  type: ColumnType
  config: FieldConfig
  source: ColumnSource
  options: DisplayOptions
  colored: boolean
}

export type RowAt = { frame: PreparedFrame; index: number }
export type TableRow = { values: Record<string, unknown>; at: RowAt }
export type FooterRow = { label: string; cells: (string | null)[] }
export type TableModel = {
  columns: TableColumn[]
  rows: TableRow[]
  footer: FooterRow[]
}

export type Cell = {
  text: string
  color: string | null
  level: ThresholdLevel | null
  word: string
}

type Draft = {
  key: string
  name: string
  type: ColumnType
  source: ColumnSource
}

type Built = { drafts: Draft[]; rows: TableRow[] }

const refKey = "query:ref"
const countReducers: ReadonlySet<Reducer> = new Set([
  "count",
  "distinctCount",
  "changeCount",
])

function stringField(name: string, values: unknown[] = []): PreparedField {
  return { name, type: "string", values }
}

function hasLabels(frame: PreparedFrame): boolean {
  return Object.keys(frame.labels ?? {}).length > 0
}

function lastValue(field: PreparedField): unknown {
  if (field.type === "number") return reduce(field.values, "lastNotNull")
  return field.values.findLast((value) => value !== null && value !== undefined)
}

function unlabeledTable(frames: PreparedFrame[]): Built {
  const withRef = new Set(frames.map((frame) => frame.ref)).size > 1
  const drafts: Draft[] = withRef
    ? [
        {
          key: refKey,
          name: "Query",
          type: "string",
          source: { frame: frames[0], field: stringField("Query") },
        },
      ]
    : []
  const known = new Set(drafts.map((draft) => draft.key))
  const rows: TableRow[] = []
  for (const frame of frames) {
    const used = new Set<string>()
    const keys = frame.fields.map((field, index) => {
      let key = `field:${field.name}`
      if (used.has(key)) key = `${key}#${index}`
      used.add(key)
      if (!known.has(key)) {
        known.add(key)
        drafts.push({
          key,
          name: field.name,
          type: field.type,
          source: { frame, field },
        })
      }
      return key
    })
    const count = Math.max(0, ...frame.fields.map((f) => f.values.length))
    for (let index = 0; index < count; index++) {
      const values: Record<string, unknown> = withRef
        ? { [refKey]: frame.ref }
        : {}
      frame.fields.forEach((field, at) => {
        values[keys[at]] = field.values[index] ?? null
      })
      rows.push({ values, at: { frame, index } })
    }
  }
  for (const row of rows) {
    for (const draft of drafts) row.values[draft.key] ??= null
  }
  return { drafts, rows }
}

function labeledTable(frames: PreparedFrame[]): Built {
  const firstWith = new Map<string, PreparedFrame>()
  for (const frame of frames) {
    for (const key of Object.keys(frame.labels ?? {})) {
      if (!firstWith.has(key)) firstWith.set(key, frame)
    }
  }
  const labelled = [...firstWith].sort(([a], [b]) => (a < b ? -1 : 1))
  const labelNames = labelled.map(([key]) => key)
  const drafts: Draft[] = labelled.map(([key, frame]) => ({
    key: `label:${key}`,
    name: key,
    type: "string",
    source: { frame, field: stringField(key) },
  }))
  const seen = new Set<string>()
  for (const frame of frames) {
    for (const field of frame.fields) {
      if (field.type === "time" || seen.has(field.name)) continue
      seen.add(field.name)
      drafts.push({
        key: `field:${field.name}`,
        name: field.name,
        type: field.type,
        source: { frame, field },
      })
    }
  }
  const rows = frames.map((frame): TableRow => {
    const values: Record<string, unknown> = {}
    const rowFields: PreparedField[] = []
    for (const key of labelNames) {
      const value = frame.labels?.[key] ?? null
      values[`label:${key}`] = value
      rowFields.push(stringField(key, [value]))
    }
    for (const field of frame.fields) {
      if (field.type === "time") continue
      const value = lastValue(field) ?? null
      values[`field:${field.name}`] = value
      rowFields.push({ ...field, values: [value] })
    }
    for (const draft of drafts) values[draft.key] ??= null
    return { values, at: { frame: { ...frame, fields: rowFields }, index: 0 } }
  })
  return { drafts, rows }
}

function isColored(config: FieldConfig): boolean {
  return (
    (config.thresholds?.length ?? 0) > 0 ||
    config.mappings.some((mapping) => mapping.color !== undefined)
  )
}

function optionsFor(
  draft: Draft,
  config: FieldConfig,
  values: unknown[]
): DisplayOptions {
  if (draft.type !== "number") return {}
  const numbers = values.map((value) =>
    typeof value === "number" ? value : null
  )
  const { min, max } = fieldRange({ ...draft.source.field, values })
  return {
    decimals: commonDecimals(numbers, config.unit, config.decimals),
    fieldMin: min,
    fieldMax: max,
  }
}

function footerReducers(display: Record<string, unknown>): Reducer[] {
  const { footer } = display
  if (!Array.isArray(footer)) return []
  return footer.filter(
    (reducer): reducer is Reducer =>
      typeof reducer === "string" && Object.hasOwn(reducerLabels, reducer)
  )
}

function footerText(
  column: TableColumn,
  values: unknown[],
  reducer: Reducer
): string {
  const reduced = reduce(values, reducer)
  if (countReducers.has(reducer)) {
    const counted = { ...column.config, unit: undefined, mappings: [] }
    return displayValue(reduced, { ...counted, decimals: 0 }).text
  }
  return displayValue(reduced, column.config, column.options).text
}

export function tableModel(
  frames: PreparedFrame[],
  fields: FieldResolver,
  display: Record<string, unknown>
): TableModel {
  const { drafts, rows } =
    frames.length > 0 && !frames.some(hasLabels)
      ? unlabeledTable(frames)
      : labeledTable(frames)
  const columns: TableColumn[] = []
  const columnValues: unknown[][] = []
  for (const draft of drafts) {
    const { frame, field } = draft.source
    const config = fields.configFor(frame, field)
    if (config.hidden) continue
    const values = rows.map((row) => row.values[draft.key])
    columns.push({
      key: draft.key,
      label:
        config.displayName === undefined
          ? draft.name
          : fields.displayName(frame, field),
      type: draft.type,
      config,
      source: draft.source,
      options: optionsFor(draft, config, values),
      colored: isColored(config),
    })
    columnValues.push(values)
  }
  const footer = footerReducers(display).map((reducer) => ({
    label: reducerLabels[reducer],
    cells: columns.map((column, index) =>
      column.type === "number"
        ? footerText(column, columnValues[index], reducer)
        : null
    ),
  }))
  return { columns, rows, footer }
}

const plain: Cell = { text: "", color: null, level: null, word: "" }

export function cellOf(column: TableColumn, value: unknown): Cell {
  if (column.type === "time" && typeof value === "number") {
    return { ...plain, text: formatTimeValue(String(value)) }
  }
  if (typeof value === "object" && value !== null) {
    return { ...plain, text: JSON.stringify(value) }
  }
  const shown = displayValue(value, column.config, column.options)
  if (!column.colored || shown.color === null) {
    return { ...plain, text: shown.text }
  }
  const { threshold } = shown
  const byThreshold =
    threshold.level !== "base" && threshold.color === shown.color
  return {
    text: shown.text,
    color: shown.color,
    level: byThreshold ? threshold.level : "base",
    word: byThreshold ? threshold.label : "",
  }
}

export function cellLinks(
  column: TableColumn,
  row: TableRow,
  cell: Cell,
  scope: LinkScope
): ResolvedLink[] {
  const links = column.config.links.filter(isDataLink)
  if (links.length === 0) return []
  const { frame } = row.at
  const field = column.source.field
  const full: LinkScope = {
    ...scope,
    row: row.at,
    labels: frame.labels,
    seriesName: scope.fields?.seriesName(frame, field),
    fieldName: field.name,
    value: row.values[column.key],
    valueText: cell.text,
  }
  return links.flatMap((link) => resolveLink(link, full) ?? [])
}
