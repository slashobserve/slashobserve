export const sparklineWidth = 600
export const sparklineHeight = 40
const inset = 2

type Point = readonly [number, number]

function bucketExtremes(points: readonly Point[], columns: number): Point[] {
  const size = points.length / columns
  const kept: Point[] = []
  for (let bucket = 0; bucket < columns; bucket++) {
    const start = Math.floor(bucket * size)
    const end = Math.floor((bucket + 1) * size)
    let low = start
    let high = start
    for (let index = start + 1; index < end; index++) {
      if (points[index][1] < points[low][1]) low = index
      if (points[index][1] > points[high][1]) high = index
    }
    if (low === high) kept.push(points[low])
    else if (low < high) kept.push(points[low], points[high])
    else kept.push(points[high], points[low])
  }
  if (kept[0] !== points[0]) kept.unshift(points[0])
  if (kept.at(-1) !== points.at(-1)) kept.push(points[points.length - 1])
  return kept
}

export function sparklinePoints(
  points: readonly Point[],
  columns: number
): string[] {
  if (points.length === 0) return []
  let t0 = Infinity
  let t1 = -Infinity
  let v0 = Infinity
  let v1 = -Infinity
  for (const [time, value] of points) {
    if (time < t0) t0 = time
    if (time > t1) t1 = time
    if (value < v0) v0 = value
    if (value > v1) v1 = value
  }
  const limit = Math.max(2, Math.ceil(columns))
  const kept =
    points.length > limit * 2 ? bucketExtremes(points, limit) : points
  return kept.map(([time, value]) => {
    const x = t1 === t0 ? 0 : ((time - t0) / (t1 - t0)) * sparklineWidth
    const y =
      v1 === v0
        ? sparklineHeight / 2
        : sparklineHeight -
          inset -
          ((value - v0) / (v1 - v0)) * (sparklineHeight - 2 * inset)
    return `${x.toFixed(1)},${y.toFixed(1)}`
  })
}
