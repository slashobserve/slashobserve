import type uPlot from "uplot"

export function isolatedIndices(
  values: ArrayLike<number | null | undefined>,
  from: number,
  to: number
): number[] {
  const indices: number[] = []
  const last = Math.min(to, values.length - 1)
  for (let index = Math.max(from, 0); index <= last; index++) {
    if (
      values[index] != null &&
      values[index - 1] == null &&
      values[index + 1] == null
    ) {
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
