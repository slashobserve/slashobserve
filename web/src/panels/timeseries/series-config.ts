import { commonDecimals } from "@/lib/units"
import type { Reducer } from "@/spec"

import { styleForSlot, type LineStyle, type SlotMap } from "../colors"
import { displayValue, type FieldConfig } from "../field-config"
import { thresholdColor } from "../thresholds"
import type { AlignedSeries } from "./align"

export type DrawStyle = FieldConfig["style"]
export type AxisKey = FieldConfig["axis"]

export type StyledSeries = {
  key: string
  label: string
  color: string
  dash?: number[]
  line: LineStyle
  style: DrawStyle
  fill: number
  axis: AxisKey
  stack: FieldConfig["stack"]
  hidden: boolean
}

const explicitDashes: Record<Exclude<LineStyle, "solid">, number[]> = {
  dashed: [6, 4],
  dotted: [2, 3],
}

export function seriesStyles(
  aligned: readonly Pick<AlignedSeries, "key" | "label">[],
  configs: readonly FieldConfig[],
  slots: SlotMap
): StyledSeries[] {
  return aligned.map(({ key, label }, index) => {
    const config = configs[index]
    const slot = styleForSlot(slots.get(key) ?? 0)
    const explicit =
      config.lineStyle === "solid"
        ? undefined
        : { dash: explicitDashes[config.lineStyle], line: config.lineStyle }
    return {
      key,
      label,
      color: config.color ? thresholdColor(config.color) : slot.color,
      dash: explicit?.dash ?? slot.dash,
      line: explicit?.line ?? slot.line,
      style: config.style,
      fill: config.fill,
      axis: config.axis,
      stack: config.stack,
      hidden: config.hidden,
    }
  })
}

export function fillPercent(style: DrawStyle, fill: number): number {
  if (style === "bars") return fill || 100
  return style === "line" ? fill : 0
}

export function fillStyle(stroke: string, percent: number): string {
  return `color-mix(in oklab, ${stroke} ${percent}%, transparent)`
}

export const scaleOf: Record<AxisKey, string> = {
  left: "y",
  right: "y2",
  hidden: "y3",
}

export type AxisSettings = {
  unit?: string
  decimals?: number
  min?: number
  max?: number
  percent: boolean
  tight?: boolean
}

export type PanelAxis = Omit<AxisSettings, "percent">

export type AxesInUse = Partial<Record<AxisKey, AxisSettings>>

function settingsOf({ unit, decimals, min, max }: PanelAxis): AxisSettings {
  return { unit, decimals, min, max, percent: false }
}

export function axesInUse(
  series: readonly Pick<StyledSeries, "axis" | "stack">[],
  configs: readonly FieldConfig[],
  panel: PanelAxis,
  ghostCount: number
): AxesInUse {
  const axes: AxesInUse = {}
  series.forEach(({ axis, stack }, index) => {
    const existing = axes[axis]
    if (!existing) {
      axes[axis] = {
        ...settingsOf(configs[index]),
        percent: stack === "percent",
      }
    } else if (stack === "percent") {
      axes[axis] = { ...existing, percent: true }
    }
  })
  if (ghostCount > 0 && !axes.left) axes.left = settingsOf(panel)
  for (const key of Object.keys(axes) as AxisKey[]) {
    const axis = axes[key]
    if (axis?.percent) {
      axes[key] = {
        ...axis,
        unit: "percentunit",
        min: undefined,
        max: undefined,
      }
    }
  }
  return axes
}

export function firstStackMode(
  series: readonly Pick<StyledSeries, "stack">[]
): FieldConfig["stack"] {
  return series.find(({ stack }) => stack !== "none")?.stack ?? "none"
}

function paddedRange(min: number, max: number): [number, number] {
  const pad = max === min ? Math.abs(min) * 0.05 : (max - min) * 0.05
  return [min - pad, max + pad]
}

function autoRange(min: number, max: number): [number, number] {
  if (min >= 0) {
    if (max <= 0) return [0, 1]
    return [0, max + max * 0.05]
  }
  return paddedRange(min, max)
}

function percentRange(min: number, max: number): [number, number] {
  const lo = min < 0 ? -1 : 0
  const hi = max > 0 ? 1 : lo === -1 ? 0 : 1
  return [lo, hi]
}

export function axisRange(
  axis: AxisSettings,
  min: number | null,
  max: number | null
): [number, number] | [null, null] {
  if (min === null || max === null) return [null, null]
  if (axis.percent) return percentRange(min, max)
  const [autoMin, autoMax] = axis.tight
    ? paddedRange(min, max)
    : autoRange(min, max)
  const lo = axis.min ?? autoMin
  let hi = axis.max ?? autoMax
  if (hi <= lo) hi = lo + 1
  return [lo, hi]
}

export function formatTogether(
  items: readonly { value: number | null; config: FieldConfig }[]
): string[] {
  const groupOf = ({ config }: { config: FieldConfig }) =>
    `${config.unit ?? ""}\u0000${config.decimals ?? ""}`
  const groups = new Map<
    string,
    { config: FieldConfig; values: (number | null)[] }
  >()
  for (const { value, config } of items) {
    const key = groupOf({ config })
    const group = groups.get(key) ?? { config, values: [] }
    group.values.push(value)
    groups.set(key, group)
  }
  const decimals = new Map(
    [...groups].map(([key, { config, values }]) => [
      key,
      commonDecimals(values, config.unit, config.decimals),
    ])
  )
  return items.map(
    (item) =>
      displayValue(item.value, item.config, {
        decimals: decimals.get(groupOf(item)),
      }).text
  )
}

export type TimeseriesDisplay = {
  legend: "bottom" | "right" | "hidden"
  legendValues: Reducer[]
  legendMode?: "list" | "table"
  legendSortBy?: "name" | Reducer
  legendSortDesc: boolean
  legendWidth?: number
  legendHideZero: boolean
  legendHideEmpty: boolean
  tooltip: "single" | "all" | "hidden"
}

export function timeseriesDisplay(
  display: Record<string, unknown>
): TimeseriesDisplay {
  return {
    legend: display.legend as TimeseriesDisplay["legend"],
    legendValues: display.legendValues as Reducer[],
    legendMode: display.legendMode as TimeseriesDisplay["legendMode"],
    legendSortBy: display.legendSortBy as TimeseriesDisplay["legendSortBy"],
    legendSortDesc: display.legendSortDesc as boolean,
    legendWidth: display.legendWidth as number | undefined,
    legendHideZero: display.legendHideZero as boolean,
    legendHideEmpty: display.legendHideEmpty as boolean,
    tooltip: display.tooltip as TimeseriesDisplay["tooltip"],
  }
}
