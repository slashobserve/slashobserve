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

  return (
    <div ref={containerRef} className="w-full">
      {mounted && stacked ? (
        <div className="flex flex-col gap-2">
          {ordered.map(({ panel, item }) => (
            <div key={panel.id} style={{ height: pixelHeight(item.h) }}>
              {renderPanel(panel, { width, height: pixelHeight(item.h) })}
            </div>
          ))}
        </div>
      ) : null}
      {mounted && !stacked ? (
        <GridLayout
          width={width}
          layout={items}
          gridConfig={{
            cols: gridColumns,
            rowHeight: gridRowHeight,
            margin: [gridMargin, gridMargin],
            containerPadding: [0, 0],
          }}
          dragConfig={{
            enabled: editable,
            handle: ".panel-drag-handle",
            threshold: 3,
          }}
          resizeConfig={{ enabled: editable, handles: ["se"] }}
          onLayoutChange={onLayoutChange}
        >
          {ordered.map(({ panel, item }) => (
            <div key={panel.id}>
              {renderPanel(panel, {
                width: pixelWidth(width, item.w),
                height: pixelHeight(item.h),
              })}
            </div>
          ))}
        </GridLayout>
      ) : null}
    </div>
  )
}
