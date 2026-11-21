import {
  clip,
  fit,
  inRect,
  positionOf,
  seriesLabels,
  type Anchor,
  type Point,
  type Rect,
  type Size,
} from "./anchors"

type ValueTarget = {
  rect: Rect
  name: string
  labels: Record<string, string> | undefined
  text: string
}

export type ValueAnchorLayout = {
  body: Size
  targets: readonly ValueTarget[]
}

export function hitTest(
  layout: ValueAnchorLayout,
  point: Point
): Anchor | null {
  const target = layout.targets.find(({ rect }) => inRect(rect, point))
  if (target === undefined) return null
  return fit({
    kind: "value",
    pos: positionOf(point, layout.body),
    series: seriesLabels(target.labels, target.name),
    shown: clip(target.text),
  })
}
