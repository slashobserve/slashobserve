import { useMemo, type ReactNode } from "react"
import { GridLayout, useContainerWidth, type Layout } from "react-grid-layout"
import { verticalCompactor } from "react-grid-layout/core"
import "react-grid-layout/css/styles.css"

import type { Selections } from "@/api/keys"
import { cn } from "@/lib/utils"
import { rendererManifests } from "@/panels"
import type { Dashboard } from "@/spec"

import { membershipOf, rowCompactor } from "./edit/layout"
import {
  columnsFor,
  fitLayout,
  gridColumns,
  gridMargin,
  pixelWidth,
} from "./fit-layout"
import { expandLayout, type LayoutItem } from "./rows/expand-layout"
import { RowHeader } from "./rows/row-header"
import { useRowStore } from "./rows/row-store"

export const gridRowHeight = 30
const singleColumnBelow = 768
const fadeIn =
  "motion-safe:animate-in motion-safe:fade-in-0 motion-safe:duration-300"

type Size = { width: number; height: number }
type PanelItem = Extract<LayoutItem, { kind: "panel" }>

type Props = {
  dashboardId: string
  spec: Dashboard
  selections: Selections
  values: Record<string, string[] | undefined>
  renderPanel: (item: PanelItem, size: Size) => ReactNode
  editable?: boolean
  layout?: Layout
  onLayoutChange?: (layout: Layout) => void
  newPanelIds?: ReadonlySet<string>
}

const noRows: Record<string, boolean> = {}

function pixelHeight(h: number): number {
  return h * gridRowHeight + (h - 1) * gridMargin
}

function viewItem(entry: LayoutItem): Layout[number] {
  const { x, y, w, h } = entry.gridPos
  return { i: entry.key, x, y, w, h }
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
  newPanelIds,
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

  const compactor = useMemo(
    () => (editing ? rowCompactor(membershipOf(spec)) : verticalCompactor),
    [editing, spec]
  )

  const stacked = width < singleColumnBelow
  const gridItems = useMemo(() => {
    const items = ordered.map(({ item }) => item)
    if (editing && !stacked) return items
    const minimum = new Map(
      ordered.map(({ entry, item }) => [
        item.i,
        stacked
          ? gridColumns
          : columnsFor(
              entry.kind === "panel"
                ? rendererManifests.get(entry.panel.type)?.minWidth
                : undefined,
              width
            ),
      ])
    )
    const fixed = new Set(
      stacked
        ? []
        : ordered.flatMap(({ entry, item }) =>
            entry.kind === "panel" && entry.panel.responsive === false
              ? [item.i]
              : []
          )
    )
    return fitLayout(items, minimum, fixed)
  }, [editing, stacked, ordered, width])

  return (
    <div ref={containerRef} className="w-full">
      {mounted ? (
        <GridLayout
          width={width}
          layout={gridItems}
          compactor={compactor}
          gridConfig={{
            cols: gridColumns,
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
          {ordered.map(({ entry }, index) => (
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
              {entry.kind === "panel" ? (
                <div
                  className={cn(
                    "size-full",
                    newPanelIds?.has(entry.panel.id) && fadeIn
                  )}
                >
                  {renderPanel(entry, {
                    width: pixelWidth(width, gridItems[index].w),
                    height: pixelHeight(gridItems[index].h),
                  })}
                </div>
              ) : null}
            </div>
          ))}
        </GridLayout>
      ) : null}
    </div>
  )
}
