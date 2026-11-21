import {
  clip,
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
  return {
    kind: "element",
    pos: positionOf(point, layout.body),
    id: clip(element.id),
  }
}
