import type { DataPanel, Reducer, Threshold } from "@/spec"

import { assignSlots, type SlotMap } from "../colors"
import type { FieldConfig } from "../field-config"
import { reduce } from "../reducers"
import type { AlignedSeries } from "./align"
import {
  formatTogether,
  seriesStyles,
  type AxisSettings,
  type StyledSeries,
} from "./series-config"
import { stackSeries, type Cell } from "./stack"

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
  bands: { series: [number, number]; dir: 1 | -1 }[]
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
    series.map(({ values }) => values),
    modes,
    series.map(({ axis }) => axis),
    series.map(({ style }) => style)
  )
  const first = 1 + ghosts.length
  return {
    state: {
      data: [xs, ...ghosts, ...drawn],
      bands: below.flatMap((lower, index) =>
        lower === null
          ? []
          : [
              {
                series: [first + index, first + lower],
                dir: directions[index] === 1 ? -1 : 1,
              },
            ]
      ),
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
  panel: Pick<DataPanel, "thresholds" | "thresholdMode">
): ThresholdLines | null {
  if (!left || left.percent) return null
  const firstLeft = series.find(({ axis }) => axis === "left")
  const list = firstLeft ? firstLeft.config.thresholds : panel.thresholds
  if (!list) return null
  const mode = firstLeft
    ? firstLeft.config.thresholdMode
    : (panel.thresholdMode ?? "absolute")
  return { list, mode }
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
