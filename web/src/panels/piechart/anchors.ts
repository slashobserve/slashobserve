import {
  clip,
  fit,
  positionOf,
  type Anchor,
  type Point,
  type Rect,
  type Size,
} from "../anchors"
import type { Slice } from "./pie-model"

export type PieAnchorLayout = {
  body: Size
  center: Point
  outer: number
  inner: number
  slices: readonly Pick<Slice, "name" | "text" | "startAngle" | "endAngle">[]
  hidden?: readonly string[]
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
  return fit({
    kind: "slice",
    pos: positionOf(point, layout.body),
    category: clip(slice.name),
    shown: clip(slice.text),
  })
}

const cardinals = [0, Math.PI / 2, Math.PI, (Math.PI * 3) / 2]

function bounds(points: readonly Point[]): Rect {
  const xs = points.map(({ x }) => x)
  const ys = points.map(({ y }) => y)
  const left = Math.min(...xs)
  const top = Math.min(...ys)
  return {
    x: left,
    y: top,
    width: Math.max(...xs) - left,
    height: Math.max(...ys) - top,
  }
}

export function locate(layout: PieAnchorLayout, anchor: Anchor): Rect | null {
  if (anchor.kind !== "slice") return null
  const slice = layout.slices.find(({ name }) => clip(name) === anchor.category)
  if (slice === undefined) return null
  const { center, outer, inner } = layout
  const angles = [
    slice.startAngle,
    slice.endAngle,
    ...cardinals.filter(
      (angle) => angle > slice.startAngle && angle < slice.endAngle
    ),
  ]
  const radii = inner > 0 ? [inner, outer] : [outer]
  const edge = angles.flatMap((angle) =>
    radii.map((radius) => ({
      x: center.x + radius * Math.sin(angle),
      y: center.y - radius * Math.cos(angle),
    }))
  )
  return bounds(inner > 0 ? edge : [...edge, center])
}

export function hides(layout: PieAnchorLayout, anchor: Anchor): boolean {
  if (anchor.kind !== "slice") return false
  const named = ({ name }: { name: string }) => clip(name) === anchor.category
  return (
    !layout.slices.some(named) &&
    (layout.hidden ?? []).some((name) => named({ name }))
  )
}
