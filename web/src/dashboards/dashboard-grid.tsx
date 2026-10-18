import { useMemo, type ReactNode } from "react"
import { GridLayout, useContainerWidth, type Layout } from "react-grid-layout"
import "react-grid-layout/css/styles.css"

import type { Dashboard, Panel } from "@/spec"

export const gridColumns = 24
export const gridRowHeight = 30
export const gridMargin = 8
const singleColumnBelow = 768

type Size = { width: number; height: number }

type Props = {
  spec: Dashboard
  renderPanel: (panel: Panel, size: Size) => ReactNode
  editable?: boolean
  layout?: Layout
  onLayoutChange?: (layout: Layout) => void
}

function specLayout(spec: Dashboard): Layout {
  return spec.panels.map((panel) => ({
    i: panel.id,
    x: panel.gridPos.x,
    y: panel.gridPos.y,
    w: panel.gridPos.w,
    h: panel.gridPos.h,
    minW: 2,
    minH: 3,
  }))
}

function pixelWidth(containerWidth: number, w: number): number {
  const column = (containerWidth - (gridColumns - 1) * gridMargin) / gridColumns
  return column * w + (w - 1) * gridMargin
}

function pixelHeight(h: number): number {
  return h * gridRowHeight + (h - 1) * gridMargin
}

export function DashboardGrid({
  spec,
  renderPanel,
  editable = false,
  layout,
  onLayoutChange,
}: Props) {
  const { width, mounted, containerRef } = useContainerWidth({
    measureBeforeMount: true,
  })
  const items = layout ?? specLayout(spec)

  const ordered = useMemo(() => {
    const byId = new Map(items.map((item) => [item.i, item]))
    return spec.panels
      .flatMap((panel) => {
        const item = byId.get(panel.id)
        return item ? [{ panel, item }] : []
      })
      .sort((a, b) => a.item.y - b.item.y || a.item.x - b.item.x)
  }, [spec.panels, items])

  const stacked = width < singleColumnBelow
  const columns = stacked ? 1 : gridColumns
  const gridItems = useMemo(() => {
    if (!stacked) return items
    let y = 0
    return ordered.map(({ panel, item }) => {
      const stackedItem = { ...item, x: 0, y, w: 1, minW: 1 }
      y += item.h
      return { ...stackedItem, i: panel.id }
    })
  }, [stacked, items, ordered])

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
          {ordered.map(({ panel, item }) => (
            <div key={panel.id}>
              {renderPanel(panel, {
                width: stacked ? width : pixelWidth(width, item.w),
                height: pixelHeight(item.h),
              })}
            </div>
          ))}
        </GridLayout>
      ) : null}
    </div>
  )
}
