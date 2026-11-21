import {
  clip,
  positionOf,
  type Anchor,
  type Point,
  type Size,
} from "../anchors"

export type HistogramAnchorLayout = {
  body: Size
  origin: Point
  plot: Size
  edges: readonly number[]
  format: (value: number) => string
}

export function hitTest(
  { body, origin, plot, edges, format }: HistogramAnchorLayout,
  point: Point
): Anchor | null {
  const x = point.x - origin.x
  const y = point.y - origin.y
  const buckets = edges.length - 1
  if (x < 0 || y < 0 || x > plot.width || y > plot.height) return null
  if (buckets < 1) return null
  const bucket = Math.min(buckets - 1, Math.floor((x / plot.width) * buckets))
  return {
    kind: "bar",
    pos: positionOf(point, body),
    category: clip(`${format(edges[bucket])} to ${format(edges[bucket + 1])}`),
  }
}
