import uPlot from "uplot"

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

export function squareCorners(
  x: number,
  y: number,
  side: number
): [number, number, number, number] {
  return [x - side / 2, y - side / 2, side, side]
}

export const squarePoints: uPlot.Series.Points.PathBuilder = (
  u,
  seriesIdx,
  idx0,
  idx1,
  filtIdxs
) => {
  const series = u.series[seriesIdx]
  const xs = u.data[0]
  const ys = u.data[seriesIdx]
  const ratio = uPlot.pxRatio
  const outline = (series.points?.width ?? 0) * ratio
  const side = (series.points?.size ?? 0) * ratio - outline
  const { left, top, width, height } = u.bbox
  const fill = new Path2D()
  const clip = new Path2D()
  clip.rect(left - side, top - side, width + side * 2, height + side * 2)
  const add = (index: number) => {
    const y = ys[index]
    if (y === null || y === undefined) return
    const px = left + u.valToPos(xs[index], "x", true)
    const py = top + u.valToPos(y, series.scale!, true)
    fill.rect(...squareCorners(px, py, side))
  }
  if (filtIdxs) filtIdxs.forEach(add)
  else for (let index = idx0; index <= idx1; index++) add(index)
  return { stroke: outline > 0 ? fill : null, fill, clip, flags: 0 }
}

export type LabelBox = { x: number; width: number }

export function labelsClearOfEachOther(
  boxes: readonly LabelBox[],
  gap: number
): number[] {
  const kept: number[] = []
  let edge = -Infinity
  boxes.forEach(({ x, width }, index) => {
    const left = x - width / 2
    if (left < edge + gap) return
    kept.push(index)
    edge = x + width / 2
  })
  return kept
}
