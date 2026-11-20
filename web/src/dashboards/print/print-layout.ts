import type { Selections } from "@/api/keys"
import { rendererManifests } from "@/panels"
import { isRowPanel, type Dashboard } from "@/spec"

import { expandLayout, type LayoutItem } from "../rows/expand-layout"
import {
  gridRowHeight,
  pixelHeight,
  rowsFor,
  stackedHeight,
} from "../stack-heights"

export type PrintItem = { entry: LayoutItem; height: number }

function heightOf(entry: LayoutItem): number {
  if (entry.kind !== "panel") return gridRowHeight
  const minimum = rendererManifests.get(entry.panel.type)?.minHeight
  return pixelHeight(rowsFor(stackedHeight(entry.gridPos.h, minimum)))
}

export function printItems(entries: LayoutItem[]): PrintItem[] {
  return entries
    .map((entry) => ({ entry, height: heightOf(entry) }))
    .sort(
      (a, b) =>
        a.entry.gridPos.y - b.entry.gridPos.y ||
        a.entry.gridPos.x - b.entry.gridPos.x
    )
}

export function printEntries(
  spec: Dashboard,
  selections: Selections,
  values: (name: string) => string[] | undefined
): PrintItem[] {
  const expanded = Object.fromEntries(
    spec.panels.filter(isRowPanel).map((row) => [row.id, false])
  )
  return printItems(
    expandLayout({
      spec,
      collapsed: expanded,
      selections,
      values,
      editing: false,
    })
  )
}
