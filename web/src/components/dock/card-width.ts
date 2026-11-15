export type WidthBounds = { min: number; max: number; initial: number }

export const activityWidth: WidthBounds = { min: 320, max: 640, initial: 384 }
export const editWidth: WidthBounds = { min: 560, max: 1040, initial: 760 }
export const panelWidth: WidthBounds = { min: 320, max: 640, initial: 384 }

// The dashboard column is at least 560px and the card has 10px of margin, so a wider card would wrap under the dashboard.
const pageReserve = 570

export function maxWidthFor(bounds: WidthBounds, viewport: number): number {
  return Math.max(bounds.min, Math.min(bounds.max, viewport - pageReserve))
}

export function clampWidth(
  value: number,
  bounds: WidthBounds,
  viewport: number
): number {
  const wanted = Number.isFinite(value) ? Math.round(value) : bounds.initial
  return Math.min(maxWidthFor(bounds, viewport), Math.max(bounds.min, wanted))
}

export function isStacked(bounds: WidthBounds, viewport: number): boolean {
  return viewport < pageReserve + bounds.min
}

export const chatPaneWidth: WidthBounds = { min: 320, max: 640, initial: 400 }
const chatPaneBreakpoint = 1200

export function chatPaneStacked(viewport: number): boolean {
  return viewport <= chatPaneBreakpoint
}

const narrowBreakpoint = 760

export function isNarrow(viewport: number): boolean {
  return viewport <= narrowBreakpoint
}

export function opensByDefault(viewport: number): boolean {
  return !isNarrow(viewport)
}

export type HandleSide = "left" | "right"

export function draggedWidth(
  side: HandleSide,
  startWidth: number,
  startX: number,
  x: number
): number {
  return side === "left" ? startWidth + (startX - x) : startWidth + (x - startX)
}
