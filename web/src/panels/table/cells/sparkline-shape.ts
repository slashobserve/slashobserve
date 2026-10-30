export const sparkWidth = 96
export const sparkHeight = 24
const inset = 2

type SparkStyle = "line" | "bars" | "points"
type SparkInterpolation = "linear" | "smooth" | "stepBefore" | "stepAfter"

export type SparkOptions = {
  style: SparkStyle
  interpolation: SparkInterpolation
  lineWidth: number
  fill: number
  points: "auto" | "always" | "never"
  pointSize: number
  spanNulls: boolean | number
  min?: number
  max?: number
  softMin?: number
  softMax?: number
}

type Point = readonly [number, number]

export type SparkShape = {
  paths: string[]
  areas: string[]
  dots: Point[]
  bars: { x: number; y: number; width: number; height: number }[]
}

export function sparkRange(
  values: readonly (number | null)[],
  options: Pick<SparkOptions, "min" | "max" | "softMin" | "softMax">
): { low: number; high: number } {
  let low = Infinity
  let high = -Infinity
  for (const value of values) {
    if (value === null) continue
    if (value < low) low = value
    if (value > high) high = value
  }
  if (options.softMin !== undefined) low = Math.min(low, options.softMin)
  if (options.softMax !== undefined) high = Math.max(high, options.softMax)
  if (options.min !== undefined) low = options.min
  if (options.max !== undefined) high = options.max
  if (high <= low) high = low + 1
  return { low, high }
}

function fixed(n: number): string {
  return n.toFixed(1)
}

function runs(
  values: readonly (number | null)[],
  joinGaps: boolean
): number[][] {
  const out: number[][] = []
  let current: number[] = []
  values.forEach((value, index) => {
    if (value === null) {
      if (!joinGaps && current.length > 0) {
        out.push(current)
        current = []
      }
      return
    }
    current.push(index)
  })
  if (current.length > 0) out.push(current)
  return out
}

function pathOf(
  points: readonly Point[],
  interpolation: SparkInterpolation
): string {
  const [first, ...rest] = points
  let d = `M${fixed(first[0])},${fixed(first[1])}`
  let [px, py] = first
  for (const [x, y] of rest) {
    if (interpolation === "stepBefore") {
      d += ` L${fixed(px)},${fixed(y)} L${fixed(x)},${fixed(y)}`
    } else if (interpolation === "stepAfter") {
      d += ` L${fixed(x)},${fixed(py)} L${fixed(x)},${fixed(y)}`
    } else if (interpolation === "smooth") {
      const mid = (px + x) / 2
      d += ` C${fixed(mid)},${fixed(py)} ${fixed(mid)},${fixed(y)} ${fixed(x)},${fixed(y)}`
    } else {
      d += ` L${fixed(x)},${fixed(y)}`
    }
    px = x
    py = y
  }
  return d
}

export function sparkShape(
  values: readonly (number | null)[],
  options: SparkOptions
): SparkShape {
  const shape: SparkShape = { paths: [], areas: [], dots: [], bars: [] }
  if (values.length === 0) return shape
  const { low, high } = sparkRange(values, options)
  const span = sparkWidth - 2 * inset
  const stepX = values.length > 1 ? span / (values.length - 1) : 0
  const xAt = (index: number) =>
    values.length > 1 ? inset + index * stepX : sparkWidth / 2
  const yAt = (value: number) => {
    const fraction = Math.min(1, Math.max(0, (value - low) / (high - low)))
    return sparkHeight - inset - fraction * (sparkHeight - 2 * inset)
  }
  const joinGaps = options.spanNulls !== false
  const segments = runs(values, joinGaps)
  const pointsOf = (indexes: readonly number[]): Point[] =>
    indexes.map((index) => [xAt(index), yAt(values[index] as number)])

  if (options.style === "bars") {
    const width = Math.max(1, (span / values.length) * 0.8)
    values.forEach((value, index) => {
      if (value === null) return
      const y = yAt(value)
      shape.bars.push({
        x: xAt(index) - width / 2,
        y,
        width,
        height: sparkHeight - inset - y,
      })
    })
    return shape
  }

  for (const indexes of segments) {
    const points = pointsOf(indexes)
    const isolated = indexes.length === 1
    if (options.style === "line") {
      if (!isolated) {
        shape.paths.push(pathOf(points, options.interpolation))
        if (options.fill > 0) {
          const base = sparkHeight - inset
          const last = points[points.length - 1]
          shape.areas.push(
            `${pathOf(points, options.interpolation)} L${fixed(last[0])},${fixed(base)} L${fixed(points[0][0])},${fixed(base)} Z`
          )
        }
      }
    }
    const marked =
      options.style === "points" ||
      options.points === "always" ||
      (options.points === "auto" && isolated)
    if (marked) shape.dots.push(...points)
  }
  return shape
}
