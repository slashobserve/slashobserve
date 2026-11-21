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

export function revealKey(
  reveal: ((key: string) => void) | undefined,
  key: string | undefined
): boolean {
  if (reveal === undefined || key === undefined) return false
  reveal(key)
  return true
}

export function scrollWithin(scroller: Element, row: Element): void {
  const view = scroller.getBoundingClientRect()
  const box = row.getBoundingClientRect()
  if (box.top < view.top) scroller.scrollTop += box.top - view.top
  else if (box.bottom > view.bottom) {
    scroller.scrollTop += box.bottom - view.bottom
  }
}
