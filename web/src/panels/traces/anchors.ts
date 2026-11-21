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
import { bottomEdge } from "../list-anchors"
import type { Span } from "./trace-model"

export type SpanAnchorLayout = {
  body: Size
  spans: readonly {
    rect: Rect
    span: Pick<Span, "id" | "service" | "operation">
  }[]
  elsewhere?: readonly string[]
}

export function hitTest(layout: SpanAnchorLayout, point: Point): Anchor | null {
  const found = layout.spans.find(({ rect }) => inRect(rect, point))
  if (found === undefined) return null
  const { span } = found
  return fit({
    kind: "span",
    pos: positionOf(point, layout.body),
    id: clip(span.id),
    quote: clip([span.service, span.operation].filter(Boolean).join(" ")),
  })
}

export function locate(layout: SpanAnchorLayout, anchor: Anchor): Rect | null {
  if (anchor.kind !== "span") return null
  const named = (id: string) => clip(id) === anchor.id
  const drawn = layout.spans.find(({ span }) => named(span.id))
  if (drawn !== undefined) return drawn.rect
  return (layout.elsewhere ?? []).some(named) ? bottomEdge(layout.body) : null
}
