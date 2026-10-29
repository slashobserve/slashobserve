import type { Layout, LayoutItem } from "react-grid-layout"
import {
  cloneLayout,
  getAllCollisions,
  moveElement,
  sortLayoutItems,
  verticalCompactor,
  type Compactor,
} from "react-grid-layout/core"

import { allPanels, isRowPanel, type Dashboard, type Panel } from "@/spec"

const columns = 24
const minWidth = 2
const minHeight = 3
const maxHeight = 100
const maxVerticalSteps = 100

export type Membership = ReadonlyMap<string, string | null>

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

export function membershipOf(spec: Dashboard): Membership {
  const rows = spec.panels
    .filter(isRowPanel)
    .sort((a, b) => a.gridPos.y - b.gridPos.y)
  const owners = new Map<string, string | null>()
  for (const panel of spec.panels) {
    if (isRowPanel(panel)) {
      for (const child of panel.panels ?? []) owners.set(child.id, panel.id)
    } else {
      const above = rows.filter((entry) => entry.gridPos.y < panel.gridPos.y)
      owners.set(panel.id, above.at(-1)?.id ?? null)
    }
  }
  return owners
}

export function applyLayout(spec: Dashboard, layout: Layout): Dashboard {
  const next = structuredClone(spec)
  const byId = new Map(layout.map((item) => [item.i, item]))
  for (const panel of layoutPanels(next)) {
    const item = byId.get(panel.id)
    if (!item) continue
    if (isRowPanel(panel)) {
      const shift = item.y - panel.gridPos.y
      panel.gridPos = { ...panel.gridPos, y: item.y }
      if (panel.collapsed) {
        for (const child of panel.panels ?? []) {
          child.gridPos = {
            ...child.gridPos,
            y: Math.max(child.gridPos.y + shift, 0),
          }
        }
      }
      continue
    }
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

function stackSections(layout: Layout, membership: Membership): Layout {
  const headers = layout.filter((item) => item.static).sort((a, b) => a.y - b.y)
  const sections = [
    { header: undefined, members: [] as LayoutItem[] },
    ...headers.map((header) => ({ header, members: [] as LayoutItem[] })),
  ]
  for (const item of layout) {
    if (item.static) continue
    const owner = membership.get(item.i)
    const section =
      sections.find((entry) => entry.header?.i === owner) ?? sections[0]
    section.members.push(item)
  }
  const placed = new Map<string, LayoutItem>()
  let cursor = 0
  for (const { header, members } of sections) {
    const reference = header ? header.y + header.h : 0
    if (header) {
      placed.set(header.i, { ...header, y: cursor })
      cursor += header.h
    }
    const local = verticalCompactor.compact(
      members.map((item) => ({ ...item, y: Math.max(item.y - reference, 0) })),
      columns
    )
    for (const item of local)
      placed.set(item.i, { ...item, y: item.y + cursor })
    cursor += Math.max(0, ...local.map((item) => item.y + item.h))
  }
  return layout.map((item) => placed.get(item.i) ?? item)
}

export function rowCompactor(membership: Membership): Compactor {
  return {
    ...verticalCompactor,
    compact: (layout) => stackSections(layout, membership),
  }
}

export function settle(layout: Layout, membership: Membership): Layout {
  return stackSections(cloneLayout(layout), membership)
}

export function resizeItemPushing(
  layout: Layout,
  membership: Membership,
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
  return settle(result, membership)
}

export function moveItemHorizontally(
  layout: Layout,
  membership: Membership,
  id: string,
  direction: 1 | -1
): Layout {
  const working = cloneLayout(layout)
  const item = working.find((entry) => entry.i === id)
  if (!item) return layout
  const x = clamp(item.x + direction, 0, columns - item.w)
  if (x === item.x) return layout
  return settle(
    moveElement(working, item, x, item.y, true, false, "vertical", columns),
    membership
  )
}

export function stepItemVertically(
  layout: Layout,
  membership: Membership,
  id: string,
  direction: 1 | -1
): Layout {
  const from = layout.find((item) => item.i === id)
  if (!from) return layout
  for (let steps = 1; steps <= maxVerticalSteps; steps++) {
    const settled = settle(
      moveItem(layout, id, 0, direction * steps),
      membership
    )
    const y = settled.find((item) => item.i === id)?.y ?? from.y
    if (direction > 0 ? y > from.y : y < from.y) return settled
  }
  return layout
}
