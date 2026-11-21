import {
  clip,
  positionOf,
  type Anchor,
  type Point,
  type Size,
} from "../anchors"
import type { Slice } from "./pie-model"

export type PieAnchorLayout = {
  body: Size
  center: Point
  outer: number
  inner: number
  slices: readonly Pick<Slice, "name" | "text" | "startAngle" | "endAngle">[]
}

const fullTurn = Math.PI * 2

export function hitTest(layout: PieAnchorLayout, point: Point): Anchor | null {
  const dx = point.x - layout.center.x
  const dy = point.y - layout.center.y
  const radius = Math.hypot(dx, dy)
  if (radius > layout.outer || radius < layout.inner) return null
  const angle = (Math.atan2(dx, -dy) + fullTurn) % fullTurn
  const slice = layout.slices.find(
    ({ startAngle, endAngle }) => angle >= startAngle && angle < endAngle
  )
  if (slice === undefined) return null
  return {
    kind: "slice",
    pos: positionOf(point, layout.body),
    category: clip(slice.name),
    shown: clip(slice.text),
  }
}
