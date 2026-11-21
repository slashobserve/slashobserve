import {
  clip,
  inRect,
  positionOf,
  type Anchor,
  type Point,
  type Rect,
  type Size,
} from "../anchors"
import type { Span } from "./trace-model"

export type SpanAnchorLayout = {
  body: Size
  spans: readonly {
    rect: Rect
    span: Pick<Span, "id" | "service" | "operation">
  }[]
}

export function hitTest(layout: SpanAnchorLayout, point: Point): Anchor | null {
  const found = layout.spans.find(({ rect }) => inRect(rect, point))
  if (found === undefined) return null
  const { span } = found
  return {
    kind: "span",
    pos: positionOf(point, layout.body),
    id: clip(span.id),
    quote: clip([span.service, span.operation].filter(Boolean).join(" ")),
  }
}
