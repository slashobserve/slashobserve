import type { Rgb } from "../contrast"
import type { TooltipSort } from "../timeseries/tooltip-rows"

export type ShowValues = "auto" | "always" | "never"
export type AlignValue = "left" | "center" | "right"

const maxRowHeight = 32
const rowGap = 2
const autoTextMinWidth = 40
const alwaysTextMinWidth = 4
const textMinHeight = 14
const minLabelWidth = 80
const maxLabelWidth = 200
const labelShare = 0.2
const trailingGap = 1

export type TimelineDisplay = {
  showValues: ShowValues
  rowHeight?: number
  columnWidth?: number
  alignValue: AlignValue
  legend: "bottom" | "right" | "hidden"
  legendMode?: "list" | "table"
  legendWidth?: number
  tooltip: "single" | "all" | "hidden"
  tooltipSort: TooltipSort
  tooltipHideZeros: boolean
  tooltipMaxWidth?: number
  tooltipMaxHeight?: number
}

export function timelineDisplay(
  display: Record<string, unknown>
): TimelineDisplay {
  return {
    showValues: showValuesOf(display),
    rowHeight: display.rowHeight as number | undefined,
    columnWidth: display.columnWidth as number | undefined,
    alignValue:
      display.alignValue === "center" || display.alignValue === "right"
        ? display.alignValue
        : "left",
    legend: display.legend as TimelineDisplay["legend"],
    legendMode: display.legendMode as TimelineDisplay["legendMode"],
    legendWidth: display.legendWidth as number | undefined,
    tooltip: display.tooltip as TimelineDisplay["tooltip"],
    tooltipSort: display.tooltipSort as TooltipSort,
    tooltipHideZeros: display.tooltipHideZeros as boolean,
    tooltipMaxWidth: display.tooltipMaxWidth as number | undefined,
    tooltipMaxHeight: display.tooltipMaxHeight as number | undefined,
  }
}

function showValuesOf(display: Record<string, unknown>): ShowValues {
  return display.showValues === "always" || display.showValues === "never"
    ? display.showValues
    : "auto"
}

export function labelWidth(innerWidth: number): number {
  return Math.min(
    maxLabelWidth,
    Math.max(minLabelWidth, innerWidth * labelShare)
  )
}

export function timeToX(
  x: number,
  from: number,
  to: number,
  width: number
): number | null {
  if (!(from < to) || x < from || x > to) return null
  return ((x - from) / (to - from)) * width
}

export function rowHeight(available: number, rows: number): number {
  if (rows < 1) return 0
  return Math.max(1, Math.min(maxRowHeight, available / rows))
}

export type Span = { offset: number; size: number }

export function barSpan(band: number, fraction: number | undefined): Span {
  if (fraction === undefined) {
    return { offset: 0, size: Math.max(1, band - rowGap) }
  }
  const size = Math.max(1, band * fraction)
  return { offset: (band - size) / 2, size }
}

export function segmentSpan(
  start: number,
  length: number,
  fraction: number | undefined,
  cells: boolean
): Span {
  if (fraction !== undefined) {
    const size = Math.max(1, length * fraction)
    return { offset: start + (length - size) / 2, size }
  }
  const inset = cells ? 1 : 0
  return {
    offset: start + inset,
    size: Math.max(1, length - trailingGap - 2 * inset),
  }
}

export function segmentOpacity(fill: number | undefined): number {
  return fill === undefined || fill <= 0 ? 1 : Math.min(fill, 100) / 100
}

export function borderWidth(lineWidth: number | undefined): number {
  return lineWidth === undefined ? 0 : lineWidth
}

export function blendOver(color: Rgb, backdrop: Rgb, opacity: number): Rgb {
  const mixed = color.map((channel, index) =>
    Math.round(channel * opacity + backdrop[index] * (1 - opacity))
  )
  return [mixed[0], mixed[1], mixed[2]]
}

export type TextPlacement = {
  x: number
  anchor: "start" | "middle" | "end"
}

const textInset = 6

export function textPlacement(align: AlignValue, width: number): TextPlacement {
  if (align === "center") return { x: width / 2, anchor: "middle" }
  if (align === "right") return { x: width - textInset, anchor: "end" }
  return { x: textInset, anchor: "start" }
}

export function showsText(
  mode: ShowValues,
  width: number,
  height: number
): boolean {
  if (mode === "never" || height < textMinHeight) return false
  return width >= (mode === "always" ? alwaysTextMinWidth : autoTextMinWidth)
}
