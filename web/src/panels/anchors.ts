import type { components } from "@/api/schema.gen"

export type Anchor = components["schemas"]["Anchor"]

export type Point = { x: number; y: number }
export type Size = { width: number; height: number }
export type Rect = { x: number; y: number; width: number; height: number }

export type AnchorsModule = {
  hitTest: (layout: never, point: Point) => Anchor | null
}

const maxLabelText = 200

function clampedShare(value: number, length: number): number {
  if (!(length > 0)) return 0
  return Math.min(1, Math.max(0, value / length))
}

export function positionOf(point: Point, body: Size): Anchor["pos"] {
  return {
    x: clampedShare(point.x, body.width),
    y: clampedShare(point.y, body.height),
  }
}

export function inRect(rect: Rect, point: Point): boolean {
  return (
    point.x >= rect.x &&
    point.x <= rect.x + rect.width &&
    point.y >= rect.y &&
    point.y <= rect.y + rect.height
  )
}

export function clip(text: string): string {
  return Array.from(text).slice(0, maxLabelText).join("")
}

export function seriesLabels(
  labels: Record<string, string> | undefined,
  name: string
): Record<string, string> {
  const present = Object.entries(labels ?? {})
  if (present.length === 0) return { name: clip(name) }
  return Object.fromEntries(present.map(([key, value]) => [key, clip(value)]))
}

export function msToIso(ms: number): string {
  return new Date(ms).toISOString()
}
