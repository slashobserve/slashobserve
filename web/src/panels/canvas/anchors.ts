import {
  clip,
  fit,
  inRect,
  positionOf,
  type Anchor,
  type Point,
  type Rect,
  type Size,
} from "../anchors"

export type CanvasAnchorLayout = {
  body: Size
  elements: readonly { rect: Rect; id: string }[]
}

export function hitTest(
  layout: CanvasAnchorLayout,
  point: Point
): Anchor | null {
  const element = layout.elements.findLast(({ rect }) => inRect(rect, point))
  if (element === undefined) return null
  return fit({
    kind: "element",
    pos: positionOf(point, layout.body),
    id: clip(element.id),
  })
}

export function locate(
  layout: CanvasAnchorLayout,
  anchor: Anchor
): Rect | null {
  if (anchor.kind !== "element") return null
  return (
    layout.elements.findLast(({ id }) => clip(id) === anchor.id)?.rect ?? null
  )
}
