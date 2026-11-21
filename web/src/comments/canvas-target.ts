import {
  fit,
  positionOf,
  type Anchor,
  type AnchorsModule,
  type Point,
  type Rect,
  type Size,
} from "@/panels/anchors"
import type { PanelState } from "@/dashboards/panel-chrome"
import type { PublishedLayout } from "@/panels/anchor-layout"

export type CanvasSurface = {
  module: AnchorsModule | null
  published: PublishedLayout | null
}

export type PinPlacement = { at: Point; gone: boolean }

export type PinView = "data" | "empty" | "hidden"

export function pinView(state: PanelState, tableView: boolean): PinView {
  if (tableView) return "hidden"
  if (state === "ready") return "data"
  return state === "empty" ? "empty" : "hidden"
}

const placedByPosition: ReadonlySet<Anchor["kind"]> = new Set([
  "position",
  "title",
])

function hitTestOf(surface: CanvasSurface) {
  const { module, published } = surface
  if (module === null || published === null) return null
  return (point: Point) =>
    module.hitTest(published.layout as never, point) ?? null
}

function locatorOf(surface: CanvasSurface) {
  const { module, published } = surface
  if (module?.locate === undefined || published === null) return null
  const locate = module.locate
  return (anchor: Anchor) => locate(published.layout as never, anchor)
}

export function pointerAnchor(
  surface: CanvasSurface,
  point: Point,
  body: Size
): Anchor {
  return (
    hitTestOf(surface)?.(point) ?? {
      kind: "position",
      pos: positionOf(point, body),
    }
  )
}

export const minDrag = 5

export function dragAnchor(
  surface: CanvasSurface,
  start: Point,
  end: Point
): Anchor | null {
  const { module, published } = surface
  if (module?.rangeTest === undefined || published === null) return null
  if (Math.abs(end.x - start.x) < minDrag) return null
  return module.rangeTest(published.layout as never, start, end)
}

export function titleAnchor(point: Point, body: Size): Anchor {
  return { kind: "title", pos: positionOf({ x: point.x, y: 0 }, body) }
}

export function outlineOf(
  surface: CanvasSurface,
  anchor: Anchor | null
): Rect | null {
  if (anchor === null || placedByPosition.has(anchor.kind)) return null
  return locatorOf(surface)?.(anchor) ?? null
}

export function revealAnchor(surface: CanvasSurface, anchor: Anchor): boolean {
  const { module, published } = surface
  if (module?.reveal === undefined || published === null) return false
  return module.reveal(published.layout as never, anchor)
}

export function withWindow(
  anchor: Anchor,
  window: { from: string; to: string }
): Anchor {
  if (window.from === "" || window.to === "") return anchor
  return fit({ ...anchor, window }) ?? anchor
}

function storedPoint(anchor: Anchor, body: Size): Point {
  return { x: anchor.pos.x * body.width, y: anchor.pos.y * body.height }
}

function clamp(value: number, low: number, high: number): number {
  return Math.min(high, Math.max(low, value))
}

const placedByContent: ReadonlySet<Anchor["kind"]> = new Set(["text"])

const detachedInset = 22

function detachedSpot(body: Size): Point {
  return { x: Math.max(0, body.width - detachedInset), y: detachedInset }
}

function hiddenBy(surface: CanvasSurface, anchor: Anchor): boolean {
  const { module, published } = surface
  if (module?.hides === undefined || published === null) return false
  return module.hides(published.layout as never, anchor)
}

export function placePin(
  anchor: Anchor,
  body: Size,
  surface: CanvasSurface,
  view: PinView
): PinPlacement | null {
  const stored = storedPoint(anchor, body)
  if (anchor.kind === "title") return { at: stored, gone: false }
  if (view === "hidden") return null
  if (placedByPosition.has(anchor.kind)) return { at: stored, gone: false }
  if (view === "empty") return { at: stored, gone: true }
  const locate = locatorOf(surface)
  if (locate === null) return { at: stored, gone: false }
  if (hiddenBy(surface, anchor)) return null
  const rect = locate(anchor)
  if (rect === null) {
    const at = placedByContent.has(anchor.kind) ? detachedSpot(body) : stored
    return { at, gone: true }
  }
  return {
    at: {
      x: clamp(stored.x, rect.x, rect.x + rect.width),
      y: clamp(stored.y, rect.y, rect.y + rect.height),
    },
    gone: false,
  }
}
