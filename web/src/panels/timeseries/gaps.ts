import type { FieldConfig } from "../field-config"
import type { AlignedFrames } from "./align"
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
        before >= 0 && sawNull && (times[at] - times[before]) * 1000 <= maxGapMs
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

export type Grid = { xs: number[]; columns: Cell[][] }

export function insertGaps(
  grid: Grid,
  target: number,
  thresholdMs: number
): Grid {
  const { xs, columns } = grid
  const values = columns[target]
  const nulled = new Set<number>()
  const midpoints = new Map<number, number>()
  let before = -1
  values.forEach((value, at) => {
    if (typeof value !== "number") return
    if (before >= 0 && (xs[at] - xs[before]) * 1000 > thresholdMs) {
      if (at - before > 1) nulled.add(before + 1)
      else midpoints.set(at, (xs[before] + xs[at]) / 2)
    }
    before = at
  })
  if (nulled.size === 0 && midpoints.size === 0) return grid

  const outXs: number[] = []
  const out: Cell[][] = columns.map(() => [])
  xs.forEach((x, at) => {
    const midpoint = midpoints.get(at)
    if (midpoint !== undefined) {
      outXs.push(midpoint)
      columns.forEach((_column, index) =>
        out[index].push(index === target ? null : undefined)
      )
    }
    outXs.push(x)
    columns.forEach((column, index) =>
      out[index].push(index === target && nulled.has(at) ? null : column[at])
    )
  })
  return { xs: outXs, columns: out }
}

export function nullsAsZero(values: readonly Cell[]): Cell[] {
  return values.map((value) => (value === null ? 0 : value))
}

export function shapeNulls(
  aligned: AlignedFrames,
  configs: readonly Pick<FieldConfig, "insertNulls" | "nullAsZero">[]
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
    if (insertNulls !== undefined) grid = insertGaps(grid, index, insertNulls)
  })
  const columns = grid.columns.map((column, index) =>
    configs[index]?.nullAsZero ? nullsAsZero(column) : column
  )
  return {
    xs: grid.xs,
    series: aligned.series.map((entry, index) => ({
      ...entry,
      values: columns[index],
    })),
    ghosts: columns.slice(aligned.series.length),
  }
}
