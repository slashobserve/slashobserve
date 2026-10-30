export type Point = { x: number; y: number }
export type ConnectionPath = "straight" | "curved"
export type LineStyle = "solid" | "dashed" | "dotted"
export type ArrowMode = "none" | "forward" | "reverse" | "both"

const dashes: Record<LineStyle, string | null> = {
  solid: null,
  dashed: "6 4",
  dotted: "1 3",
}

const defaultMinWidth = 1
const defaultMaxWidth = 10

function coordinate(value: number): string {
  return String(Math.round(value * 100) / 100)
}

function at(point: Point): string {
  return `${coordinate(point.x)} ${coordinate(point.y)}`
}

function mirror(point: Point, through: Point): Point {
  return { x: 2 * through.x - point.x, y: 2 * through.y - point.y }
}

function curve(points: Point[]): string {
  const last = points.length - 1
  const segments = points.slice(0, -1).map((from, index) => {
    const to = points[index + 1]
    const before =
      index === 0 ? mirror(points[1], points[0]) : points[index - 1]
    const after =
      index + 1 === last
        ? mirror(points[last - 1], points[last])
        : points[index + 2]
    const first = {
      x: from.x + (to.x - before.x) / 6,
      y: from.y + (to.y - before.y) / 6,
    }
    const second = {
      x: to.x - (after.x - from.x) / 6,
      y: to.y - (after.y - from.y) / 6,
    }
    return `C ${at(first)} ${at(second)} ${at(to)}`
  })
  return `M ${at(points[0])} ${segments.join(" ")}`
}

export function connectionPath(points: Point[], path: ConnectionPath): string {
  if (points.length < 2) return ""
  if (path === "curved") return curve(points)
  return `M ${at(points[0])} ${points
    .slice(1)
    .map((point) => `L ${at(point)}`)
    .join(" ")}`
}

export function dashArray(style: LineStyle): string | null {
  return dashes[style]
}

export function lineCap(style: LineStyle): "round" | "butt" {
  return style === "dotted" ? "round" : "butt"
}

export function arrowEnds(arrow: ArrowMode): { start: boolean; end: boolean } {
  return {
    start: arrow === "reverse" || arrow === "both",
    end: arrow === "forward" || arrow === "both",
  }
}

export function scaledWidth(
  value: number,
  range: { min: number; max: number },
  limits: { min?: number; max?: number }
): number {
  const low = limits.min ?? defaultMinWidth
  const high = limits.max ?? defaultMaxWidth
  const [narrow, wide] = low <= high ? [low, high] : [high, low]
  const span = range.max - range.min
  if (!(span > 0)) return narrow
  const share = Math.min(1, Math.max(0, (value - range.min) / span))
  return narrow + share * (wide - narrow)
}
