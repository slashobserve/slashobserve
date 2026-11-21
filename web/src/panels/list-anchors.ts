import type { Rect, Size } from "./anchors"

export const edgeHeight = 2

export function bottomEdge(body: Size): Rect {
  return {
    x: 0,
    y: body.height - edgeHeight,
    width: body.width,
    height: edgeHeight,
  }
}

export function visibleBand(rect: Rect, view: Rect): Rect | null {
  const middle = rect.y + rect.height / 2
  if (middle < view.y || middle > view.y + view.height) return null
  const top = Math.max(rect.y, view.y)
  const bottom = Math.min(rect.y + rect.height, view.y + view.height)
  return { ...rect, y: top, height: bottom - top }
}
