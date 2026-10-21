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
  decimals: Map<string | undefined, number | undefined>
  range: Pick<DisplayOptions, "fieldMin" | "fieldMax">
}

export type RowAt = { frame: PreparedFrame; index: number }
export type RowSource = {
  frame: PreparedFrame
  fields: Record<string, PreparedField>
}
export type TableRow = {
  key: string
  values: Record<string, unknown>
  at: RowAt
  source: RowSource
}
export type FooterRow = { label: string; cells: (string | null)[] }
export type TableModel = {
  columns: TableColumn[]
  rows: TableRow[]
  footer: FooterRow[]
}

export type Cell = { text: string } & (
  | { color: null; level: null; word: "" }
  | { color: string; level: "base"; word: "" }
  | {
      color: string
      level: Exclude<ThresholdLevel, "base">
      word: string
    }
)

type Draft = {
  key: string
  name: string
  type: ColumnType
  source: ColumnSource
  contributors: ColumnSource[]
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
          contributors: [],
        },
      ]
    : []
  const byKey = new Map(drafts.map((draft) => [draft.key, draft]))
  const rows: TableRow[] = []
  frames.forEach((frame, frameIndex) => {
    const used = new Set<string>()
    const sourceFields: Record<string, PreparedField> = {}
    const keys = frame.fields.map((field, index) => {
      let key = `field:${field.name}`
      if (used.has(key)) key = `${key}#${index}`
      used.add(key)
      sourceFields[key] = field
      let draft = byKey.get(key)
      if (!draft) {
        draft = {
          key,
          name: field.name,
          type: field.type,
          source: { frame, field },
          contributors: [],
        }
        byKey.set(key, draft)
        drafts.push(draft)
      }
      draft.contributors.push({ frame, field })
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
      rows.push({
        key: `${frameIndex}:${index}`,
        values,
        at: { frame, index },
        source: { frame, fields: sourceFields },
      })
    }
  })
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
    contributors: [],
  }))
  const valueDrafts = new Map<string, Draft>()
  for (const frame of frames) {
    for (const field of frame.fields) {
      if (field.type === "time") continue
      let draft = valueDrafts.get(field.name)
      if (!draft) {
        draft = {
          key: `field:${field.name}`,
          name: field.name,
          type: field.type,
          source: { frame, field },
          contributors: [],
        }
        valueDrafts.set(field.name, draft)
        drafts.push(draft)
      }
      draft.contributors.push({ frame, field })
    }
  }
  const rows = frames.map((frame, frameIndex): TableRow => {
    const values: Record<string, unknown> = {}
    const rowFields: PreparedField[] = []
    const sourceFields: Record<string, PreparedField> = {}
    for (const key of labelNames) {
      const value = frame.labels?.[key] ?? null
      values[`label:${key}`] = value
      rowFields.push(stringField(key, [value]))
    }
    for (const field of frame.fields) {
      if (field.type === "time") continue
      const value = lastValue(field) ?? null
      values[`field:${field.name}`] = value
      sourceFields[`field:${field.name}`] = field
      rowFields.push({ ...field, values: [value] })
    }
    for (const draft of drafts) values[draft.key] ??= null
    return {
      key: String(frameIndex),
      values,
      at: { frame: { ...frame, fields: rowFields }, index: 0 },
      source: { frame, fields: sourceFields },
    }
  })
  return { drafts, rows }
}

function isColored(config: FieldConfig): boolean {
  return (
    (config.thresholds?.length ?? 0) > 0 ||
    config.mappings.some((mapping) => mapping.color !== undefined)
  )
}

export function configAt(
  fields: FieldResolver,
  column: Pick<TableColumn, "key" | "config">,
  row: TableRow
): FieldConfig {
  const field = row.source.fields[column.key]
  return field ? fields.configFor(row.source.frame, field) : column.config
}

function rangeOf(
  field: PreparedField,
  values: unknown[]
): TableColumn["range"] {
  const { min, max } = fieldRange({ ...field, values })
  return { fieldMin: min, fieldMax: max }
}

function headerOf(draft: Draft, fields: FieldResolver): string {
  const names = new Set(
    draft.contributors.map(({ frame, field }) =>
      fields.configFor(frame, field).displayName === undefined
        ? field.name
        : fields.displayName(frame, field)
    )
  )
  return names.size === 1 ? [...names][0] : draft.name
}

function decimalsByUnit(
  fields: FieldResolver,
  column: TableColumn,
  rows: TableRow[]
): TableColumn["decimals"] {
  const groups = new Map<string | undefined, (number | null)[]>()
  for (const row of rows) {
    const { unit, decimals } = configAt(fields, column, row)
    const numbers = groups.get(unit) ?? []
    groups.set(unit, numbers)
    if (decimals !== undefined) continue
    const value = row.values[column.key]
    numbers.push(typeof value === "number" ? value : null)
  }
  return new Map(
    [...groups].map(([unit, numbers]) => [unit, commonDecimals(numbers, unit)])
  )
}

function optionsAt(column: TableColumn, config: FieldConfig): DisplayOptions {
  return { ...column.range, decimals: column.decimals.get(config.unit) }
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
  return displayValue(reduced, column.config, optionsAt(column, column.config))
    .text
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
    const column: TableColumn = {
      key: draft.key,
      label: headerOf(draft, fields),
      type: draft.type,
      config,
      source: draft.source,
      decimals: new Map(),
      range: rangeOf(field, values),
    }
    if (draft.type === "number") {
      column.decimals = decimalsByUnit(fields, column, rows)
    }
    columns.push(column)
    columnValues.push(values)
  }
  const footer = footerReducers(display).map((reducer) => ({
    label: reducerLabels[reducer],
    cells: columns.map((column, index) =>
      column.type === "number" && column.decimals.size <= 1
        ? footerText(column, columnValues[index], reducer)
        : null
    ),
  }))
  return { columns, rows, footer }
}

const plain: Cell = { text: "", color: null, level: null, word: "" }

export function cellOf(
  fields: FieldResolver,
  column: TableColumn,
  row: TableRow
): Cell {
  const value = row.values[column.key]
  if (column.type === "time" && typeof value === "number") {
    return { ...plain, text: formatTimeValue(String(value)) }
  }
  if (typeof value === "object" && value !== null) {
    return { ...plain, text: JSON.stringify(value) }
  }
  const config = configAt(fields, column, row)
  const shown = displayValue(value, config, optionsAt(column, config))
  if (!isColored(config) || shown.color === null) {
    return { ...plain, text: shown.text }
  }
  const { threshold } = shown
  if (threshold.level !== "base" && threshold.color === shown.color) {
    return {
      text: shown.text,
      color: shown.color,
      level: threshold.level,
      word: threshold.label,
    }
  }
  return { text: shown.text, color: shown.color, level: "base", word: "" }
}

export function cellLinks(
  fields: FieldResolver,
  column: TableColumn,
  row: TableRow,
  cell: Cell,
  scope: LinkScope
): ResolvedLink[] {
  const links = configAt(fields, column, row).links.filter(isDataLink)
  if (links.length === 0) return []
  const { frame } = row.source
  const field = row.source.fields[column.key] ?? column.source.field
  const full: LinkScope = {
    ...scope,
    row: row.at,
    labels: frame.labels,
    seriesName: fields.seriesName(frame, field),
    fieldName: field.name,
    value: row.values[column.key],
    valueText: cell.text,
  }
  return links.flatMap((link) => resolveLink(link, full) ?? [])
}
