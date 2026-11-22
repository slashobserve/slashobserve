import type { components } from "@/api/schema.gen"
import {
  encodedBytes,
  maxEncoded,
  maxLabels,
  maxText,
} from "@/conversation/anchor"

export type Anchor = components["schemas"]["Anchor"]

export type Point = { x: number; y: number }
export type Size = { width: number; height: number }
export type Rect = { x: number; y: number; width: number; height: number }

export type AnchorsModule = {
  hitTest: (layout: never, point: Point) => Anchor | null
  locate?: (layout: never, anchor: Anchor) => Rect | null
  rangeTest?: (layout: never, start: Point, end: Point) => Anchor | null
  reveal?: (layout: never, anchor: Anchor) => boolean
  hides?: (layout: never, anchor: Anchor) => boolean
}

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
  return Array.from(text).slice(0, maxText).join("")
}

export function labelsOf(
  entries: Iterable<readonly [string, string]>
): Record<string, string> {
  const labels: Record<string, string> = {}
  for (const [key, value] of entries) {
    if (key === "") continue
    labels[clip(key)] = clip(value)
    if (Object.keys(labels).length === maxLabels) break
  }
  return labels
}

export function seriesLabels(
  labels: Record<string, string> | undefined,
  name: string
): Record<string, string> {
  const present = labelsOf(Object.entries(labels ?? {}))
  return Object.keys(present).length === 0 ? { name: clip(name) } : present
}

export function msToIso(ms: number): string | null {
  const date = new Date(ms)
  return Number.isNaN(date.getTime()) ? null : date.toISOString()
}

function withoutLastLabel(anchor: Anchor): Anchor | null {
  for (const field of ["series", "row"] as const) {
    const labels = anchor[field]
    const keys = Object.keys(labels ?? {})
    if (labels === undefined || keys.length < 2) continue
    const kept = { ...labels }
    delete kept[keys[keys.length - 1]]
    return { ...anchor, [field]: kept }
  }
  return null
}

export function fit(anchor: Anchor): Anchor | null {
  let current: Anchor | null = anchor
  while (current !== null && encodedBytes(current) > maxEncoded) {
    current = withoutLastLabel(current)
  }
  return current
}

export function namesLabels(
  named: Record<string, string> | undefined,
  labels: Record<string, string>
): boolean {
  const entries = Object.entries(named ?? {})
  return (
    entries.length > 0 && entries.every(([key, value]) => labels[key] === value)
  )
}
