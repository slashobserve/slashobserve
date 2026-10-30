import type { DataPanel, Reducer, Threshold } from "@/spec"

import { assignSlots, type SlotMap } from "../colors"
import type { FieldConfig } from "../field-config"
import { reduce } from "../reducers"
import type { AlignedSeries } from "./align"
import { spanShortGaps } from "./gaps"
import {
  fillPercent,
  formatTogether,
  seriesStyles,
  type AxisSettings,
  type StyledSeries,
} from "./series-config"
import { stackSeries, type Cell } from "./stack"
import { transformValues } from "./value-transform"

const defaultBandFill = 20

export type ChartSeries = StyledSeries & {
  values: Cell[]
  fieldValues: readonly unknown[]
  config: FieldConfig
}

export type XAxis =
  { kind: "time" } | { kind: "number"; format: (value: number) => string }

export const timeAxis: XAxis = { kind: "time" }

export type PlotState = {
  data: [number[], ...Cell[][]]
  bands: {
    series: [number, number]
    dir: 1 | -1
    fill?: { color: string; percent: number }
  }[]
  hidden: ReadonlySet<string>
}

export type PlotStateFor = {
  state: PlotState
  below: (number | null)[]
  stacked: boolean[]
}

export type ThresholdLines = {
  list: Threshold[]
  mode: FieldConfig["thresholdMode"]
  style: FieldConfig["thresholdsStyle"]
}

type Aligned = Pick<AlignedSeries, "key" | "label" | "values" | "field">

export function configuredKeys(
  aligned: readonly Pick<AlignedSeries, "key">[],
  configs: readonly FieldConfig[]
): string[] {
  return aligned.flatMap(({ key }, index) =>
    configs[index].hidden ? [] : [key]
  )
}

export function chartSeries(
  aligned: readonly Aligned[],
  configs: readonly FieldConfig[],
  slots: SlotMap
): ChartSeries[] {
  const styles = seriesStyles(
    aligned,
    configs,
    assignSlots(slots, configuredKeys(aligned, configs))
  )
  return styles.flatMap((style, index) =>
    style.hidden
      ? []
      : [
          {
            ...style,
            values: aligned[index].values,
            fieldValues: aligned[index].field.values,
            config: configs[index],
          },
        ]
  )
}

function drawnValues(
  xs: readonly number[],
  { values, spanNulls, config }: ChartSeries
): Cell[] {
  const joined =
    typeof spanNulls === "number"
      ? spanShortGaps(xs, values, spanNulls)
      : values
  return transformValues(joined, config.valueTransform)
}

function fillBelowBands(
  series: readonly ChartSeries[],
  first: number
): PlotState["bands"] {
  return series.flatMap((entry, index) => {
    const { fillBelowTo } = entry.config
    if (fillBelowTo === undefined || entry.stack !== "none") return []
    const lower = series.findIndex(
      (other, at) => at !== index && other.label === fillBelowTo
    )
    if (lower < 0) return []
    return [
      {
        series: [first + index, first + lower] as [number, number],
        dir: -1 as const,
        fill: {
          color: entry.color,
          percent: fillPercent(entry.style, entry.fill) || defaultBandFill,
        },
      },
    ]
  })
}

export function plotStateFor(
  xs: number[],
  ghosts: readonly Cell[][],
  series: readonly ChartSeries[],
  hidden: ReadonlySet<string>
): PlotStateFor {
  const modes = series.map(({ key, stack }) =>
    hidden.has(key) ? "none" : stack
  )
  const { drawn, below, directions } = stackSeries(
    series.map((entry) => drawnValues(xs, entry)),
    modes,
    series.map(({ axis }) => axis),
    series.map(({ style }) => style),
    series.map(({ config }) => config.stackGroup)
  )
  const first = 1 + ghosts.length
  return {
    state: {
      data: [xs, ...ghosts, ...drawn],
      bands: [
        ...below.flatMap((lower, index) =>
          lower === null
            ? []
            : [
                {
                  series: [first + index, first + lower] as [number, number],
                  dir: directions[index] === 1 ? (-1 as const) : (1 as const),
                },
              ]
        ),
        ...fillBelowBands(series, first),
      ],
      hidden,
    },
    below,
    stacked: modes.map((mode) => mode !== "none"),
  }
}

export function legendCells(
  series: readonly Pick<ChartSeries, "fieldValues" | "config">[],
  reducers: readonly Reducer[]
): string[][] {
  const columns = reducers.map((reducer) =>
    formatTogether(
      series.map(({ fieldValues, config }) => ({
        value: reduce(fieldValues, reducer),
        config,
      }))
    )
  )
  return series.map((_entry, row) => columns.map((column) => column[row]))
}

export function ghostsDrawn(
  series: readonly Pick<ChartSeries, "axis" | "stack">[],
  ghosts: readonly Cell[][]
): Cell[][] {
  const percent = series.some(
    ({ axis, stack }) => axis === "left" && stack === "percent"
  )
  return percent ? [] : [...ghosts]
}

export function thresholdsFor(
  series: readonly Pick<ChartSeries, "axis" | "config">[],
  left: AxisSettings | undefined,
  panel: Pick<DataPanel, "thresholds" | "thresholdMode" | "thresholdsStyle">
): ThresholdLines | null {
  if (!left || left.percent) return null
  const firstLeft = series.find(({ axis }) => axis === "left")
  const list = firstLeft ? firstLeft.config.thresholds : panel.thresholds
  if (!list) return null
  const mode = firstLeft
    ? firstLeft.config.thresholdMode
    : (panel.thresholdMode ?? "absolute")
  const style = firstLeft
    ? firstLeft.config.thresholdsStyle
    : (panel.thresholdsStyle ?? "dashed")
  return { list, mode, style }
}

export function thresholdValueAt(
  value: number,
  mode: FieldConfig["thresholdMode"],
  min: number,
  max: number
): number {
  return mode === "percentage" ? min + (value / 100) * (max - min) : value
}

export function bandBase(
  drawn: number,
  stacked: boolean,
  lower: { drawn: Cell } | null
): number {
  if (lower) return lower.drawn ?? 0
  return stacked ? 0 : drawn
}

export function nearestSpan(
  spans: readonly (readonly [number, number])[],
  cursorTop: number
): number {
  let best = 0
  let bestDistance = Infinity
  spans.forEach(([a, b], index) => {
    const lo = Math.min(a, b)
    const hi = Math.max(a, b)
    const distance =
      cursorTop < lo ? lo - cursorTop : cursorTop > hi ? cursorTop - hi : 0
    if (distance < bestDistance) {
      best = index
      bestDistance = distance
    }
  })
  return best
}

export type TooltipAnchor = {
  idx: number
  left: number
  top: number
  peer: boolean
}

export function tooltipAnchor(
  cursor: {
    idx?: number | null
    left?: number | null
    top?: number | null
  },
  pointerOver: boolean,
  peerTooltip: boolean
): TooltipAnchor | null {
  const { idx, left } = cursor
  if (idx == null || left == null || left < 0) return null
  if (!pointerOver && !peerTooltip) return null
  return { idx, left, top: cursor.top ?? 0, peer: !pointerOver }
}

export const hiddenCursor = { left: -10, top: -10 }

export function peerCursorAt(
  left: number | null,
  width: number,
  height: number
): { left: number; top: number } {
  if (left === null || !Number.isFinite(left) || left < 0 || left > width) {
    return hiddenCursor
  }
  return { left, top: height / 2 }
}
