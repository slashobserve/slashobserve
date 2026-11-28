import type { PanelDataMeta } from "@/dashboards/queries"

import type { FieldConfig } from "../field-config"
import type { AlignedFrames } from "./align"
import { pairedAt } from "./paired"
import type { Cell } from "./stack"

export function spanShortGaps(
  times: readonly number[],
  values: readonly Cell[],
  maxGapMs: number
): Cell[] {
  let out: Cell[] | null = null
  let before = -1
  let runStart = 0
  let sawNull = false
  for (let at = 0; at < values.length; at++) {
    const value = values[at]
    if (value === null) {
      sawNull = true
    } else if (typeof value === "number") {
      const joinable =
        before >= 0 &&
        sawNull &&
        (pairedAt(times, at) - pairedAt(times, before)) * 1000 <= maxGapMs
      if (joinable) {
        out ??= [...values]
        for (let fill = runStart; fill < at; fill++) out[fill] = undefined
      }
      before = at
      runStart = at + 1
      sawNull = false
    }
  }
  return out ?? (values as Cell[])
}

export type QueryIntervals = Pick<PanelDataMeta, "step_ms" | "queries">

export function queryIntervalMs(meta: QueryIntervals, ref: string): number {
  return (
    meta.queries.find((query) => query.ref === ref)?.step_ms || meta.step_ms
  )
}

export function insertNullsThresholdMs(
  insertNulls: FieldConfig["insertNulls"],
  intervalMs: number
): number | undefined {
  if (insertNulls !== true) return insertNulls
  return intervalMs > 0 ? intervalMs : undefined
}

export function breakingGaps<
  T extends Pick<FieldConfig, "insertNulls" | "spanNulls">,
>(config: T): T {
  if (config.insertNulls !== undefined || config.spanNulls === true) {
    return config
  }
  return { ...config, insertNulls: true }
}

export type Grid = { xs: number[]; columns: Cell[][] }

export function insertGaps(
  grid: Grid,
  target: number,
  thresholdMs: number
): Grid {
  const { xs, columns } = grid
  const values = pairedAt(columns, target)
  const nulled = new Set<number>()
  const midpoints = new Map<number, number>()
  let before = -1
  values.forEach((value, at) => {
    if (typeof value !== "number") return
    if (before < 0) {
      before = at
      return
    }
    const from = pairedAt(xs, before)
    const to = pairedAt(xs, at)
    if ((to - from) * 1000 > thresholdMs) {
      if (at - before > 1) nulled.add(before + 1)
      else midpoints.set(at, (from + to) / 2)
    }
    before = at
  })
  if (nulled.size === 0 && midpoints.size === 0) return grid

  const outXs: number[] = []
  xs.forEach((x, at) => {
    const midpoint = midpoints.get(at)
    if (midpoint !== undefined) outXs.push(midpoint)
    outXs.push(x)
  })
  const out = columns.map((column, index) => {
    const cells: Cell[] = []
    xs.forEach((_x, at) => {
      if (midpoints.has(at)) cells.push(index === target ? null : undefined)
      cells.push(index === target && nulled.has(at) ? null : column[at])
    })
    return cells
  })
  return { xs: outXs, columns: out }
}

export function nullsAsZero(values: readonly Cell[]): Cell[] {
  return values.map((value) => (value === null ? 0 : value))
}

export function shapeNulls(
  aligned: AlignedFrames,
  configs: readonly Pick<FieldConfig, "insertNulls" | "nullAsZero">[],
  meta: QueryIntervals
): AlignedFrames {
  const shaped = configs.some(
    ({ insertNulls, nullAsZero }) => insertNulls !== undefined || nullAsZero
  )
  if (!shaped) return aligned
  let grid: Grid = {
    xs: aligned.xs,
    columns: [...aligned.series.map(({ values }) => values), ...aligned.ghosts],
  }
  configs.forEach(({ insertNulls }, index) => {
    const threshold = insertNullsThresholdMs(
      insertNulls,
      queryIntervalMs(meta, pairedAt(aligned.series, index).frame.ref)
    )
    if (threshold !== undefined) grid = insertGaps(grid, index, threshold)
  })
  const columns = grid.columns.map((column, index) =>
    configs[index]?.nullAsZero ? nullsAsZero(column) : column
  )
  return {
    xs: grid.xs,
    series: aligned.series.map((entry, index) => ({
      ...entry,
      values: pairedAt(columns, index),
    })),
    ghosts: columns.slice(aligned.series.length),
  }
}
