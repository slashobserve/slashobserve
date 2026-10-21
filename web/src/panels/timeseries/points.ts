import type uPlot from "uplot"

function nearestPresent(
  values: ArrayLike<number | null | undefined>,
  start: number,
  step: 1 | -1
): number | null | undefined {
  for (let at = start; at >= 0 && at < values.length; at += step) {
    const value = values[at]
    if (value !== undefined) return value
  }
  return undefined
}

export function isolatedIndices(
  values: ArrayLike<number | null | undefined>,
  from: number,
  to: number
): number[] {
  const indices: number[] = []
  const last = Math.min(to, values.length - 1)
  for (let index = Math.max(from, 0); index <= last; index++) {
    if (typeof values[index] !== "number") continue
    const before = nearestPresent(values, index - 1, -1)
    const after = nearestPresent(values, index + 1, 1)
    if (typeof before !== "number" && typeof after !== "number") {
      indices.push(index)
    }
  }
  return indices
}

export const isolatedPoints: uPlot.Series.Points.Filter = (u, seriesIdx) => {
  const values = u.data[seriesIdx]
  const [from, to] = u.series[0].idxs ?? [0, values.length - 1]
  const indices = isolatedIndices(values, from ?? 0, to ?? values.length - 1)
  return indices.length > 0 ? indices : null
}
