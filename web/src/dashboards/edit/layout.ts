import type { Layout, LayoutItem } from "react-grid-layout"
import { cloneLayout, getCompactor } from "react-grid-layout/core"

import type { Dashboard } from "@/spec"

const columns = 24
const minWidth = 2
const minHeight = 3
const maxHeight = 50
const maxVerticalSteps = 100
const compactor = getCompactor("vertical")

export function layoutFromSpec(spec: Dashboard): Layout {
  return spec.panels.map((panel) => ({
    i: panel.id,
    x: panel.gridPos.x,
    y: panel.gridPos.y,
    w: panel.gridPos.w,
    h: panel.gridPos.h,
    minW: minWidth,
    minH: minHeight,
  }))
}

export function applyLayout(spec: Dashboard, layout: Layout): Dashboard {
  const next = structuredClone(spec)
  const byId = new Map(layout.map((item) => [item.i, item]))
  for (const panel of next.panels) {
    const item = byId.get(panel.id)
    if (!item) continue
    panel.gridPos = {
      ...panel.gridPos,
      x: item.x,
      y: item.y,
      w: item.w,
      h: item.h,
    }
  }
  return next
}

export function sameLayout(a: Layout, b: Layout): boolean {
  if (a.length !== b.length) return false
  const byId = new Map(b.map((item) => [item.i, item]))
  return a.every((item) => {
    const other = byId.get(item.i)
    return (
      other !== undefined &&
      other.x === item.x &&
      other.y === item.y &&
      other.w === item.w &&
      other.h === item.h
    )
  })
}

function clamp(value: number, low: number, high: number): number {
  return Math.min(Math.max(value, low), high)
}

function changeItem(
  layout: Layout,
  id: string,
  change: (item: LayoutItem) => Partial<LayoutItem>
): Layout {
  return layout.map((item) =>
    item.i === id ? { ...item, ...change(item) } : item
  )
}

export function moveItem(
  layout: Layout,
  id: string,
  dx: number,
  dy: number
): Layout {
  return changeItem(layout, id, (item) => ({
    x: clamp(item.x + dx, 0, columns - item.w),
    y: Math.max(item.y + dy, 0),
  }))
}

export function resizeItem(
  layout: Layout,
  id: string,
  dw: number,
  dh: number
): Layout {
  return changeItem(layout, id, (item) => ({
    w: clamp(item.w + dw, minWidth, columns - item.x),
    h: clamp(item.h + dh, minHeight, maxHeight),
  }))
}

function settle(layout: Layout): Layout {
  return compactor.compact(cloneLayout(layout), columns)
}

export function stepItemVertically(
  layout: Layout,
  id: string,
  direction: 1 | -1
): Layout {
  const from = layout.find((item) => item.i === id)
  if (!from) return layout
  for (let steps = 1; steps <= maxVerticalSteps; steps++) {
    const settled = settle(moveItem(layout, id, 0, direction * steps))
    const y = settled.find((item) => item.i === id)?.y ?? from.y
    if (direction > 0 ? y > from.y : y < from.y) return settled
  }
  return layout
}
