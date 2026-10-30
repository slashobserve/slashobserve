import type { ColumnType } from "../frames"
import type { FieldConfig } from "../field-config"
import type { TableColumn } from "./table-model"

export type Align = "left" | "center" | "right"
export type RowHeight = "sm" | "md" | "lg"
export type Sort = { key: string; direction: "ascending" | "descending" }

export type ColumnSizes = { width?: number; minWidth?: number }
export type ColumnWidths = { widths: number[]; total: number }

const rowHeights = ["sm", "md", "lg"] as const

const freeColumnFloor = 64
const headerPx = 32
const footerRowPx = 25
const controlsPx = 40
const rowPx: Record<RowHeight, number> = { sm: 25, md: 33, lg: 41 }
const minPageSize = 1

export function alignOf(align: FieldConfig["align"], type: ColumnType): Align {
  if (align !== "auto") return align
  return type === "number" ? "right" : "left"
}

export function rowHeightOf(display: Record<string, unknown>): RowHeight {
  const { cellHeight } = display
  return rowHeights.find((height) => height === cellHeight) ?? "sm"
}

export function columnWidths(
  sizes: ColumnSizes[],
  available: number,
  forceFixed: boolean
): ColumnWidths | null {
  const sized = sizes.some(
    ({ width, minWidth }) => width !== undefined || minWidth !== undefined
  )
  if (sizes.length === 0 || (!sized && !forceFixed)) return null
  const floorOf = ({ minWidth }: ColumnSizes) => minWidth ?? freeColumnFloor
  const fixed = sizes.map(({ width, minWidth }) =>
    width === undefined ? null : Math.max(width, minWidth ?? 0)
  )
  const taken = fixed.reduce<number>((sum, width) => sum + (width ?? 0), 0)
  const freeCount = fixed.filter((width) => width === null).length
  const share = freeCount === 0 ? 0 : (available - taken) / freeCount
  const widths = sizes.map(
    (size, index) => fixed[index] ?? Math.max(Math.floor(share), floorOf(size))
  )
  return { widths, total: widths.reduce((sum, width) => sum + width, 0) }
}

export function stickyOffsets(widths: number[], frozen: number): number[] {
  const offsets: number[] = []
  let left = 0
  for (let index = 0; index < Math.min(frozen, widths.length); index++) {
    offsets.push(left)
    left += widths[index]
  }
  return offsets
}

export function frozenCount(display: Record<string, unknown>): number {
  const { frozenColumns } = display
  return typeof frozenColumns === "number" && frozenColumns > 0
    ? Math.floor(frozenColumns)
    : 0
}

export function pageSizeFor(
  height: number,
  rowHeight: RowHeight,
  chrome: { header: boolean; footerRows: number }
): number {
  const used =
    (chrome.header ? headerPx : 0) +
    chrome.footerRows * footerRowPx +
    controlsPx
  return Math.max(minPageSize, Math.floor((height - used) / rowPx[rowHeight]))
}

export type Page<T> = {
  rows: T[]
  page: number
  pageCount: number
}

export function pageRows<T>(rows: T[], page: number, size: number): Page<T> {
  const pageCount = Math.max(1, Math.ceil(rows.length / size))
  const clamped = Math.min(Math.max(0, page), pageCount - 1)
  return {
    rows: rows.slice(clamped * size, (clamped + 1) * size),
    page: clamped,
    pageCount,
  }
}

export function initialSort(
  columns: TableColumn[],
  display: Record<string, unknown>
): Sort | null {
  const { sortBy, sortDesc } = display
  if (typeof sortBy !== "string") return null
  const column =
    columns.find((candidate) => candidate.label === sortBy) ??
    columns.find((candidate) => candidate.source.field.name === sortBy)
  if (!column) return null
  return {
    key: column.key,
    direction: sortDesc === true ? "descending" : "ascending",
  }
}

const typeWords: Record<ColumnType, string> = {
  number: "number",
  string: "text",
  time: "time",
  bool: "boolean",
  other: "other",
}

export function typeWord(type: ColumnType): string {
  return typeWords[type]
}
