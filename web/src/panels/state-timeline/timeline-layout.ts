import type { TimelineState } from "./timeline-model"

export type ShowValues = "auto" | "always" | "never"

const maxRowHeight = 32
export const rowGap = 2
const maxLegendItems = 12
const autoTextMinWidth = 40
const alwaysTextMinWidth = 4
const textMinHeight = 14
const minLabelWidth = 80
const maxLabelWidth = 200
const labelShare = 0.2
const characterWidth = 7
const legendItemChrome = 30
const legendLineHeight = 18

export function showValuesOf(display: Record<string, unknown>): ShowValues {
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

export function showsText(
  mode: ShowValues,
  width: number,
  height: number
): boolean {
  if (mode === "never" || height < textMinHeight) return false
  return width >= (mode === "always" ? alwaysTextMinWidth : autoTextMinWidth)
}

export type LegendItems = {
  shown: TimelineState[]
  more: number
  labels: string[]
}

export function legendItems(states: TimelineState[]): LegendItems {
  const shown = states.slice(0, maxLegendItems)
  const more = states.length - shown.length
  return {
    shown,
    more,
    labels: [
      ...shown.map((state) => state.text),
      ...(more > 0 ? [moreLabel(more)] : []),
    ],
  }
}

export function moreLabel(more: number): string {
  return `and ${more} more`
}

export function legendHeight(labels: readonly string[], width: number): number {
  if (labels.length === 0) return 0
  let lines = 1
  let used = 0
  for (const label of labels) {
    const item = legendItemChrome + label.length * characterWidth
    if (used > 0 && used + item > width) {
      lines++
      used = 0
    }
    used += item
  }
  return lines * legendLineHeight + 8
}
