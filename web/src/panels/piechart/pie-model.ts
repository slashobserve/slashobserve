import { decimalsByUnit } from "@/lib/units"
import { displayWithDefaults } from "@/spec"

import { styleForSlot } from "../colors"
import { displayValue, type FieldResolver } from "../field-config"
import { reduceItems, reduceOptionsOf } from "../reduce-items"
import type { PreparedFrame } from "../transforms"

type PieType = "pie" | "donut"
type PieLegend = "right" | "bottom" | "hidden"
export type SliceLabel = "name" | "value" | "percent"
export type LegendColumn = "value" | "percent"
type LegendSort = "name" | "value"
type TooltipMode = "single" | "all" | "hidden"
type TooltipSort = "none" | "asc" | "desc"

export type PieOptions = {
  pieType: PieType
  legend: PieLegend
  sliceLabels: SliceLabel[]
  titleSize: number
  valueSize: number
  legendColumns: LegendColumn[]
  legendMode: "list" | "table"
  legendSortBy?: LegendSort
  legendSortDesc: boolean
  legendWidth?: number
  tooltip: TooltipMode
  tooltipSort: TooltipSort
  tooltipHideZeros: boolean
  tooltipMaxWidth?: number
  tooltipMaxHeight?: number
}

export type PieEntry = {
  key: string
  name: string
  value: number | null
  text: string
  color: string
  hideInLegend: boolean
  hideInTooltip: boolean
}

export type Slice = PieEntry & {
  value: number
  percent: number
  startAngle: number
  endAngle: number
}

export type PieModel = {
  total: number
  slices: Slice[]
  percents: ReadonlyMap<string, number | null>
}

export const donutRatio = 0.6
const defaultLabelSize = 12

const fullTurn = Math.PI * 2
const sliceLabelOrder: readonly SliceLabel[] = ["name", "value", "percent"]
const legendColumnOrder: readonly LegendColumn[] = ["value", "percent"]
const noneHidden: ReadonlySet<string> = new Set()

function chosen<T extends string>(order: readonly T[], value: unknown): T[] {
  const items: unknown[] = Array.isArray(value) ? value : []
  return order.filter((id) => items.includes(id))
}

function size(value: unknown): number | undefined {
  return typeof value === "number" ? value : undefined
}

export function pieOptions(display: Record<string, unknown>): PieOptions {
  const merged = displayWithDefaults("piechart", display)
  const sortBy = merged.legendSortBy
  return {
    pieType: merged.pieType === "donut" ? "donut" : "pie",
    legend:
      merged.legend === "bottom" || merged.legend === "hidden"
        ? merged.legend
        : "right",
    sliceLabels: chosen(sliceLabelOrder, merged.sliceLabels),
    titleSize: size(merged.titleSize) ?? defaultLabelSize,
    valueSize: size(merged.valueSize) ?? defaultLabelSize,
    legendColumns: chosen(legendColumnOrder, merged.legendShow),
    legendMode: merged.legendMode === "table" ? "table" : "list",
    legendSortBy: sortBy === "name" || sortBy === "value" ? sortBy : undefined,
    legendSortDesc: merged.legendSortDesc === true,
    legendWidth: size(merged.legendWidth),
    tooltip:
      merged.tooltip === "all" || merged.tooltip === "hidden"
        ? merged.tooltip
        : "single",
    tooltipSort:
      merged.tooltipSort === "asc" || merged.tooltipSort === "desc"
        ? merged.tooltipSort
        : "none",
    tooltipHideZeros: merged.tooltipHideZeros === true,
    tooltipMaxWidth: size(merged.tooltipMaxWidth),
    tooltipMaxHeight: size(merged.tooltipMaxHeight),
  }
}

function isDrawn(value: number | null): value is number {
  return value !== null && value > 0
}

export function pieEntries(
  frames: PreparedFrame[],
  fields: FieldResolver,
  display: Record<string, unknown>
): PieEntry[] {
  const items = reduceItems(
    frames,
    fields,
    reduceOptionsOf("piechart", display)
  )
  const decimals = decimalsByUnit(
    items.map(({ config, value }) => ({
      unit: config.unit,
      decimals: config.decimals,
      value,
    }))
  )
  return items.map(({ key, label, config, value, raw }, index) => {
    const shown = displayValue(raw, config, {
      decimals: decimals.get(config.unit),
    })
    return {
      key,
      name: label,
      value,
      text: shown.text,
      color: shown.color ?? styleForSlot(index).color,
      hideInLegend: config.hideInLegend === true,
      hideInTooltip: config.hideInTooltip === true,
    }
  })
}

export function pieModel(
  entries: readonly PieEntry[],
  hidden: ReadonlySet<string> = noneHidden
): PieModel {
  const total = entries.reduce(
    (sum, { key, value }) =>
      isDrawn(value) && !hidden.has(key) ? sum + value : sum,
    0
  )
  const slices: Slice[] = []
  const percents = new Map<string, number | null>()
  let angle = 0
  for (const entry of entries) {
    const { key, value } = entry
    if (!isDrawn(value)) {
      percents.set(key, value === null ? null : 0)
      continue
    }
    if (hidden.has(key)) {
      percents.set(key, (value / (total + value)) * 100)
      continue
    }
    const sweep = (value / total) * fullTurn
    const percent = (value / total) * 100
    percents.set(key, percent)
    slices.push({
      ...entry,
      value,
      percent,
      startAngle: angle,
      endAngle: angle + sweep,
    })
    angle += sweep
  }
  return { total, slices, percents }
}

export function percentText(percent: number): string {
  return `${percent.toFixed(1)}%`
}

function sliceLabel(slice: Slice): string {
  return `${slice.name} ${slice.text} (${percentText(slice.percent)})`
}

export function summaryLabel(slices: Slice[]): string {
  return slices.slice(0, 5).map(sliceLabel).join(", ")
}

function percentCell(percent: number | null | undefined): string {
  return percent === null || percent === undefined ? "" : percentText(percent)
}

export function legendCells(
  entries: readonly PieEntry[],
  percents: ReadonlyMap<string, number | null>,
  columns: readonly LegendColumn[]
): string[][] {
  return entries.map((entry) =>
    columns.map((column) =>
      column === "value" ? entry.text : percentCell(percents.get(entry.key))
    )
  )
}

export type TooltipRow = {
  key: string
  name: string
  text: string
  percent: string
  color: string
  active: boolean
}

export function tooltipRows(
  entries: readonly PieEntry[],
  percents: ReadonlyMap<string, number | null>,
  hovered: string | null,
  options: Pick<PieOptions, "tooltip" | "tooltipSort" | "tooltipHideZeros">,
  hidden: ReadonlySet<string> = noneHidden
): TooltipRow[] {
  const target = entries.find((entry) => entry.key === hovered)
  if (!target || target.hideInTooltip || options.tooltip === "hidden") return []
  const row = (entry: PieEntry): TooltipRow => ({
    key: entry.key,
    name: entry.name,
    text: entry.text,
    percent: percentCell(percents.get(entry.key)),
    color: entry.color,
    active: entry.key === target.key,
  })
  if (options.tooltip === "single") return [row(target)]
  const listed = entries.filter(
    (entry) =>
      !entry.hideInTooltip &&
      !hidden.has(entry.key) &&
      !(options.tooltipHideZeros && entry.value === 0)
  )
  const direction = options.tooltipSort === "asc" ? 1 : -1
  if (options.tooltipSort !== "none") {
    listed.sort((a, b) => {
      if (a.value === null || b.value === null) {
        return a.value === b.value ? 0 : a.value === null ? 1 : -1
      }
      return direction * (a.value - b.value)
    })
  }
  return listed.map(row)
}

export type LabelLine = { text: string; size: number }

export function sliceLabelLines(
  slice: Slice,
  labels: readonly SliceLabel[],
  sizes: { title: number; value: number }
): LabelLine[] {
  const lines: LabelLine[] = []
  if (labels.includes("name"))
    lines.push({ text: slice.name, size: sizes.title })
  const value = labels.includes("value") ? slice.text : ""
  const percent = labels.includes("percent") ? percentText(slice.percent) : ""
  const joined = value && percent ? `${value} (${percent})` : value || percent
  if (joined) lines.push({ text: joined, size: sizes.value })
  return lines
}

const glyphWidth = 0.58
const lineHeight = 1.25
const pieLabelRadius = 0.62

export function lineOffsets(lines: readonly LabelLine[]): number[] {
  const height = lines.reduce((sum, line) => sum + line.size * lineHeight, 0)
  let top = -height / 2
  return lines.map((line) => {
    const middle = top + (line.size * lineHeight) / 2
    top += line.size * lineHeight
    return middle
  })
}

export type LabelPlacement = { x: number; y: number; fits: boolean }

export function labelPlacement(
  slice: Pick<Slice, "startAngle" | "endAngle">,
  lines: readonly LabelLine[],
  radius: number,
  inner: number
): LabelPlacement {
  const sweep = slice.endAngle - slice.startAngle
  const angle = slice.startAngle + sweep / 2
  const middle = inner > 0 ? (inner + radius) / 2 : radius * pieLabelRadius
  const radial = inner > 0 ? radius - inner : 2 * (radius - middle)
  const tangential = 2 * middle * Math.sin(Math.min(sweep, Math.PI) / 2)
  const width = Math.max(
    0,
    ...lines.map((line) => line.text.length * line.size * glyphWidth)
  )
  const height = lines.reduce((sum, line) => sum + line.size * lineHeight, 0)
  const sin = Math.abs(Math.sin(angle))
  const cos = Math.abs(Math.cos(angle))
  return {
    x: middle * Math.sin(angle),
    y: -middle * Math.cos(angle),
    fits:
      lines.length > 0 &&
      width * sin + height * cos <= radial &&
      width * cos + height * sin <= tangential,
  }
}

function round(value: number): number {
  return Math.round(value * 1000) / 1000
}

function point(cx: number, cy: number, r: number, angle: number): string {
  return `${round(cx + r * Math.sin(angle))} ${round(cy - r * Math.cos(angle))}`
}

function circle(cx: number, cy: number, r: number, clockwise: boolean) {
  const sweep = clockwise ? 1 : 0
  return (
    `M ${point(cx, cy, r, 0)} ` +
    `A ${r} ${r} 0 1 ${sweep} ${point(cx, cy, r, Math.PI)} ` +
    `A ${r} ${r} 0 1 ${sweep} ${point(cx, cy, r, 0)} Z`
  )
}

export function slicePath(
  cx: number,
  cy: number,
  r: number,
  inner: number,
  start: number,
  end: number
): string {
  if (end - start >= fullTurn - 1e-9) {
    const ring = inner > 0 ? ` ${circle(cx, cy, inner, false)}` : ""
    return `${circle(cx, cy, r, true)}${ring}`
  }
  const large = end - start > Math.PI ? 1 : 0
  const outer = `M ${point(cx, cy, r, start)} A ${r} ${r} 0 ${large} 1 ${point(cx, cy, r, end)}`
  if (inner <= 0) return `${outer} L ${cx} ${cy} Z`
  return `${outer} L ${point(cx, cy, inner, end)} A ${inner} ${inner} 0 ${large} 0 ${point(cx, cy, inner, start)} Z`
}
