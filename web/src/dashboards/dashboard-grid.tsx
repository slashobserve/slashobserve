import { useMemo, type ReactNode } from "react"
import { GridLayout, useContainerWidth, type Layout } from "react-grid-layout"
import "react-grid-layout/css/styles.css"

import type { Selections } from "@/api/keys"
import type { Dashboard } from "@/spec"

import { expandLayout, type LayoutItem } from "./rows/expand-layout"
import { RowHeader } from "./rows/row-header"
import { useRowStore } from "./rows/row-store"

export const gridColumns = 24
export const gridRowHeight = 30
export const gridMargin = 10
const singleColumnBelow = 768

type Size = { width: number; height: number }
export type PanelItem = Extract<LayoutItem, { kind: "panel" }>

type Props = {
  dashboardId: string
  spec: Dashboard
  selections: Selections
  values: Record<string, string[] | undefined>
  renderPanel: (item: PanelItem, size: Size) => ReactNode
  editable?: boolean
  layout?: Layout
  onLayoutChange?: (layout: Layout) => void
}

const noRows: Record<string, boolean> = {}

function pixelWidth(containerWidth: number, w: number): number {
  const column = (containerWidth - (gridColumns - 1) * gridMargin) / gridColumns
  return column * w + (w - 1) * gridMargin
}

function pixelHeight(h: number): number {
  return h * gridRowHeight + (h - 1) * gridMargin
}

function viewItem(entry: LayoutItem): Layout[number] {
  const { x, y, w, h } = entry.gridPos
  const fixed = entry.kind !== "panel"
  return { i: entry.key, x, y, w, h, static: fixed }
}

export function DashboardGrid({
  dashboardId,
  spec,
  selections,
  values,
  renderPanel,
  editable = false,
  layout,
  onLayoutChange,
}: Props) {
  const { width, mounted, containerRef } = useContainerWidth({
    measureBeforeMount: true,
  })
  const collapsed = useRowStore((state) => state.collapsed[dashboardId])
  const toggleRow = useRowStore((state) => state.toggle)
  const editing = layout !== undefined

  const ordered = useMemo(() => {
    const byId = new Map((layout ?? []).map((item) => [item.i, item]))
    return expandLayout({
      spec,
      collapsed: collapsed ?? noRows,
      selections,
      values: (name) => values[name],
      editing,
    })
      .flatMap((entry) => {
        const item = editing ? byId.get(entry.key) : viewItem(entry)
        return item ? [{ entry, item }] : []
      })
      .sort((a, b) => a.item.y - b.item.y || a.item.x - b.item.x)
  }, [spec, layout, collapsed, selections, values, editing])

  const stacked = width < singleColumnBelow
  const columns = stacked ? 1 : gridColumns
  const gridItems = useMemo(() => {
    if (!stacked) return ordered.map(({ item }) => item)
    let y = 0
    return ordered.map(({ item }) => {
      const stackedItem = { ...item, x: 0, y, w: 1, minW: 1 }
      y += item.h
      return stackedItem
    })
  }, [stacked, ordered])

  return (
    <div ref={containerRef} className="w-full">
      {mounted ? (
        <GridLayout
          width={width}
          layout={gridItems}
          gridConfig={{
            cols: columns,
            rowHeight: gridRowHeight,
            margin: [gridMargin, gridMargin],
            containerPadding: [0, 0],
          }}
          dragConfig={{
            enabled: editable && !stacked,
            handle: ".panel-drag-handle",
            threshold: 3,
          }}
          resizeConfig={{ enabled: editable && !stacked, handles: ["se"] }}
          onLayoutChange={stacked ? undefined : onLayoutChange}
        >
          {ordered.map(({ entry, item }) => (
            <div key={entry.key}>
              {entry.kind === "row" ? (
                <RowHeader
                  rowId={entry.row.id}
                  title={entry.title}
                  collapsed={entry.collapsed}
                  childCount={entry.childCount}
                  onToggle={
                    editing
                      ? undefined
                      : () =>
                          toggleRow(
                            dashboardId,
                            entry.row.id,
                            entry.row.collapsed ?? false
                          )
                  }
                />
              ) : null}
              {entry.kind === "note" ? (
                <p className="flex size-full items-center text-xs text-muted-foreground">
                  {entry.text}
                </p>
              ) : null}
              {entry.kind === "panel"
                ? renderPanel(entry, {
                    width: stacked ? width : pixelWidth(width, item.w),
                    height: pixelHeight(item.h),
                  })
                : null}
            </div>
          ))}
        </GridLayout>
      ) : null}
    </div>
  )
}
