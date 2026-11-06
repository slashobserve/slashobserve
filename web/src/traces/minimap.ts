import type { Span } from "@/panels/traces/trace-model"

export type Zoom = { start: number; end: number }

export type MinimapRow = {
  top: number
  left: number
  width: number
  slot: number
  error: boolean
}

export const wholeTrace: Zoom = { start: 0, end: 1 }

const clickSlack = 2

function clamp(value: number, low: number, high: number): number {
  return Math.min(Math.max(value, low), high)
}

export function isWholeTrace(zoom: Zoom): boolean {
  return zoom.start <= 0 && zoom.end >= 1
}

export function minimapRows(
  spans: readonly Span[],
  height: number
): MinimapRow[] {
  if (spans.length === 0) return []
  const lane = Math.max(height / spans.length, 1)
  return spans.map((span, index) => ({
    top: index * lane,
    left: span.offset,
    width: span.width,
    slot: span.slot,
    error: span.error,
  }))
}

export function zoomFromDrag(
  anchorX: number,
  x: number,
  width: number,
  current: Zoom
): Zoom | null {
  if (width <= 0 || Math.abs(x - anchorX) < clickSlack) return null
  const span = current.end - current.start
  const at = (position: number) =>
    current.start + clamp(position / width, 0, 1) * span
  const [low, high] = anchorX < x ? [anchorX, x] : [x, anchorX]
  const start = at(low)
  const end = at(high)
  return end > start ? { start, end } : null
}

export function zoomedOffset(
  span: Pick<Span, "offset" | "width">,
  zoom: Zoom
): { offset: number; width: number } | null {
  const window = zoom.end - zoom.start
  if (window <= 0) return null
  const start = (span.offset - zoom.start) / window
  const end = (span.offset + span.width - zoom.start) / window
  if (end < 0 || start > 1) return null
  const offset = clamp(start, 0, 1)
  return { offset, width: clamp(end, 0, 1) - offset }
}
