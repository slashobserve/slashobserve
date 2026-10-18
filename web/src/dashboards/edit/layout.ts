import type { Layout, LayoutItem } from "react-grid-layout"
import {
  cloneLayout,
  getAllCollisions,
  getCompactor,
  moveElement,
  sortLayoutItems,
} from "react-grid-layout/core"

import { allPanels, isRowPanel, type Dashboard, type Panel } from "@/spec"

const columns = 24
const minWidth = 2
const minHeight = 3
const maxHeight = 100
const maxVerticalSteps = 100
const compactor = getCompactor("vertical")

export function layoutPanels(spec: Dashboard): Panel[] {
  return allPanels(spec).flatMap(({ panel, row }) =>
    row?.collapsed ? [] : [panel]
  )
}

export function layoutFromSpec(spec: Dashboard): Layout {
  return layoutPanels(spec).map((panel) =>
    isRowPanel(panel)
      ? {
          i: panel.id,
          x: 0,
          y: panel.gridPos.y,
          w: columns,
          h: panel.gridPos.h,
          static: true,
        }
      : {
          i: panel.id,
          x: panel.gridPos.x,
          y: panel.gridPos.y,
          w: panel.gridPos.w,
          h: panel.gridPos.h,
          minW: minWidth,
          minH: minHeight,
          maxH: maxHeight,
        }
  )
}

export function applyLayout(spec: Dashboard, layout: Layout): Dashboard {
  const next = structuredClone(spec)
  const byId = new Map(layout.map((item) => [item.i, item]))
  for (const panel of layoutPanels(next)) {
    if (isRowPanel(panel)) continue
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
    w:
      dw === 0
        ? item.w
        : clamp(
            item.w + dw,
            Math.min(minWidth, item.w),
            Math.max(columns - item.x, item.w)
          ),
    h:
      dh === 0
        ? item.h
        : clamp(
            item.h + dh,
            Math.min(minHeight, item.h),
            Math.max(maxHeight, item.h)
          ),
  }))
}

export function settle(layout: Layout): Layout {
  return compactor.compact(cloneLayout(layout), columns)
}

export function resizeItemPushing(
  layout: Layout,
  id: string,
  dw: number,
  dh: number
): Layout {
  const working = cloneLayout(resizeItem(layout, id, dw, dh))
  const item = working.find((entry) => entry.i === id)
  if (!item) return layout
  item.moved = true
  let result = working
  const collisions = getAllCollisions(
    sortLayoutItems(working, "vertical"),
    item
  )
  for (const collision of collisions) {
    if (collision.moved) continue
    result = moveElement(
      result,
      collision,
      undefined,
      item.y + item.h,
      false,
      false,
      "vertical",
      columns
    )
  }
  return settle(result)
}

export function moveItemHorizontally(
  layout: Layout,
  id: string,
  direction: 1 | -1
): Layout {
  const working = cloneLayout(layout)
  const item = working.find((entry) => entry.i === id)
  if (!item) return layout
  const x = clamp(item.x + direction, 0, columns - item.w)
  if (x === item.x) return layout
  return settle(
    moveElement(working, item, x, item.y, true, false, "vertical", columns)
  )
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
