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

export function parseStoredWidth(
  raw: string | null,
  bounds: WidthBounds
): number {
  if (raw === null || !/^\d+$/.test(raw)) return bounds.initial
  return Math.min(bounds.max, Math.max(bounds.min, Number(raw)))
}

export function storedWidth(key: string, bounds: WidthBounds): number {
  try {
    return parseStoredWidth(localStorage.getItem(key), bounds)
  } catch {
    return bounds.initial
  }
}

export function storeWidth(key: string, width: number): void {
  try {
    localStorage.setItem(key, String(Math.round(width)))
  } catch {
    return
  }
}

export function isStacked(bounds: WidthBounds, viewport: number): boolean {
  return viewport < pageReserve + bounds.min
}

export const fieldsWidth: WidthBounds = { min: 200, max: 560, initial: 248 }

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

const phoneBreakpoint = 900

export function dashboardActivityOpens(viewport: number): boolean {
  return viewport > phoneBreakpoint
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
