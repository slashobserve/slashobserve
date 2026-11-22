import {
  clip,
  fit,
  inRect,
  namesLabels,
  positionOf,
  seriesLabels,
  type Anchor,
  type Point,
  type Rect,
  type Size,
} from "./anchors"
import { bottomEdge } from "./list-anchors"

type ValueTarget = {
  rect: Rect | null
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
  const target = layout.targets.find(
    ({ rect }) => rect !== null && inRect(rect, point)
  )
  if (target === undefined) return null
  return fit({
    kind: "value",
    pos: positionOf(point, layout.body),
    series: seriesLabels(target.labels, target.name),
    shown: clip(target.text),
  })
}

export function locate(layout: ValueAnchorLayout, anchor: Anchor): Rect | null {
  if (anchor.kind !== "value") return null
  const target = layout.targets.find(({ labels, name }) =>
    namesLabels(anchor.series, seriesLabels(labels, name))
  )
  if (target === undefined) return null
  return target.rect ?? bottomEdge(layout.body)
}
