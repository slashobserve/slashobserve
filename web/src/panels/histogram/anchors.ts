import {
  clip,
  fit,
  positionOf,
  type Anchor,
  type Point,
  type Rect,
  type Size,
} from "../anchors"

export type HistogramAnchorLayout = {
  body: Size
  origin: Point
  plot: Size
  edges: readonly number[]
  format: (value: number) => string
}

function bucketLabel(layout: HistogramAnchorLayout, bucket: number): string {
  const { edges, format } = layout
  const low = edges[bucket]
  const high = edges[bucket + 1]
  if (low === undefined || high === undefined) return ""
  return clip(`${format(low)} to ${format(high)}`)
}

export function hitTest(
  layout: HistogramAnchorLayout,
  point: Point
): Anchor | null {
  const { body, origin, plot, edges } = layout
  const x = point.x - origin.x
  const y = point.y - origin.y
  const buckets = edges.length - 1
  if (x < 0 || y < 0 || x > plot.width || y > plot.height) return null
  if (buckets < 1 || !(plot.width > 0 && plot.height > 0)) return null
  const bucket = Math.min(buckets - 1, Math.floor((x / plot.width) * buckets))
  return fit({
    kind: "bar",
    pos: positionOf(point, body),
    category: bucketLabel(layout, bucket),
  })
}

export function locate(
  layout: HistogramAnchorLayout,
  anchor: Anchor
): Rect | null {
  if (anchor.kind !== "bar") return null
  const { origin, plot } = layout
  const buckets = layout.edges.length - 1
  for (let bucket = 0; bucket < buckets; bucket++) {
    if (bucketLabel(layout, bucket) !== anchor.category) continue
    const width = plot.width / buckets
    return {
      x: origin.x + bucket * width,
      y: origin.y,
      width,
      height: plot.height,
    }
  }
  return null
}
