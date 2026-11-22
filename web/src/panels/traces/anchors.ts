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
import { bottomEdge, revealKey } from "../list-anchors"
import type { Span } from "./trace-model"

export type SpanAnchorLayout = {
  body: Size
  spans: readonly {
    rect: Rect
    span: Pick<Span, "id" | "service" | "operation">
  }[]
  resolve?: (id: string) => string | undefined
  reveal?: (id: string) => void
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

function hiddenId(
  layout: SpanAnchorLayout,
  anchor: Anchor
): string | undefined {
  if (anchor.kind !== "span") return undefined
  if (layout.spans.some(({ span }) => clip(span.id) === anchor.id)) {
    return undefined
  }
  return anchor.id === undefined ? undefined : layout.resolve?.(anchor.id)
}

export function locate(layout: SpanAnchorLayout, anchor: Anchor): Rect | null {
  if (anchor.kind !== "span") return null
  const drawn = layout.spans.find(({ span }) => clip(span.id) === anchor.id)
  if (drawn !== undefined) return drawn.rect
  return hiddenId(layout, anchor) === undefined ? null : bottomEdge(layout.body)
}

export function reveal(layout: SpanAnchorLayout, anchor: Anchor): boolean {
  return revealKey(layout.reveal, hiddenId(layout, anchor))
}
