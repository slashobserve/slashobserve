import { freePanelId } from "@/dashboards/append-panel"
import {
  allPanels,
  findPanel,
  panelTitle,
  type Dashboard,
  type LayoutPanel,
} from "@/spec"

import type { PanelPatch } from "./queries"

const maxPanelIdLength = 64
const copySuffixRoom = 8

export function copyIdFor(spec: Dashboard, id: string): string {
  return freePanelId(
    spec,
    `${id.slice(0, maxPanelIdLength - copySuffixRoom)}-copy`
  )
}

export function duplicatePanelPatch(
  spec: Dashboard,
  panel: LayoutPanel
): PanelPatch {
  const entry = findPanel(spec, panel.id)
  const grid = panel.gridPos
  const below = grid.y + grid.h
  const copy = {
    ...panel,
    id: copyIdFor(spec, panel.id),
    title: `${panelTitle(panel)} copy`,
    gridPos: { ...grid, y: below },
  }
  const layout: NonNullable<PanelPatch["layout"]> = []
  for (const other of allPanels(spec)) {
    if (other.panel.id === panel.id) continue
    const hiddenInRow = other.row?.collapsed === true
    const startsBelow = hiddenInRow
      ? other.row !== undefined && other.row.gridPos.y >= below
      : other.panel.gridPos.y >= below
    if (!startsBelow) continue
    layout.push({
      id: other.panel.id,
      gridPos: { ...other.panel.gridPos, y: other.panel.gridPos.y + grid.h },
    })
  }
  const row = entry?.row
  if (row !== undefined) layout.push({ id: copy.id, row: row.id })
  return { upsert: [copy], layout }
}

export function removePanelPatch(panel: LayoutPanel): PanelPatch {
  return { delete: [panel.id] }
}
