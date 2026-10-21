import type uPlot from "uplot"

export function isolatedIndices(
  values: ArrayLike<number | null | undefined>,
  from: number,
  to: number
): number[] {
  const present: number[] = []
  for (let index = 0; index < values.length; index++) {
    if (values[index] !== undefined) present.push(index)
  }
  const joined = (index: number | undefined) =>
    index !== undefined && typeof values[index] === "number"
  const indices: number[] = []
  present.forEach((index, position) => {
    if (index < from || index > to || typeof values[index] !== "number") return
    if (!joined(present[position - 1]) && !joined(present[position + 1])) {
      indices.push(index)
    }
  })
  return indices
}

export const isolatedPoints: uPlot.Series.Points.Filter = (u, seriesIdx) => {
  const values = u.data[seriesIdx]
  const [from, to] = u.series[0].idxs ?? [0, values.length - 1]
  const indices = isolatedIndices(values, from ?? 0, to ?? values.length - 1)
  return indices.length > 0 ? indices : null
}
