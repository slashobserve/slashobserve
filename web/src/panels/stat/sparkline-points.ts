export function sparklineHasArea(
  filledTile: boolean,
  graphStyle: string | undefined
): boolean {
  return !filledTile && graphStyle !== "line"
}

export const sparklineWidth = 600
export const sparklineHeight = 40
const inset = 2
const baseline = sparklineHeight - inset
const flatHeight = sparklineHeight / 2

type Point = readonly [number, number]

function bucketExtremes(points: readonly Point[], columns: number): Point[] {
  const size = points.length / columns
  const kept: Point[] = []
  for (let bucket = 0; bucket < columns; bucket++) {
    const start = Math.floor(bucket * size)
    const end = Math.floor((bucket + 1) * size)
    const startPoint = points[start]
    if (startPoint === undefined) continue
    let low = startPoint
    let high = startPoint
    let lowIndex = start
    let highIndex = start
    for (let index = start + 1; index < end; index++) {
      const point = points[index]
      if (point === undefined) break
      if (point[1] < low[1]) {
        low = point
        lowIndex = index
      }
      if (point[1] > high[1]) {
        high = point
        highIndex = index
      }
    }
    if (lowIndex === highIndex) kept.push(low)
    else if (lowIndex < highIndex) kept.push(low, high)
    else kept.push(high, low)
  }
  const first = points[0]
  const last = points.at(-1)
  if (first !== undefined && kept[0] !== first) kept.unshift(first)
  if (last !== undefined && kept.at(-1) !== last) kept.push(last)
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
  const flatY = v0 === 0 ? baseline : flatHeight
  const limit = Math.max(2, Math.ceil(columns))
  const kept =
    points.length > limit * 2 ? bucketExtremes(points, limit) : points
  return kept.map(([time, value]) => {
    const x = t1 === t0 ? 0 : ((time - t0) / (t1 - t0)) * sparklineWidth
    const y =
      v1 === v0
        ? flatY
        : baseline - ((value - v0) / (v1 - v0)) * (sparklineHeight - 2 * inset)
    return `${x.toFixed(1)},${y.toFixed(1)}`
  })
}
