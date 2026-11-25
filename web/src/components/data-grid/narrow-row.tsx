import type { ICellRendererParams } from "ag-grid-community"
import { createContext, useContext, useLayoutEffect, useRef } from "react"

import type { DataGridColumn } from "./data-grid"

export const narrowRowEstimate = 72

export type NarrowRows<T> = {
  columns: readonly DataGridColumn<T>[]
  onHighlight: (rowId: string | null) => void
}

export const NarrowRowsContext = createContext<NarrowRows<unknown>>({
  columns: [],
  onHighlight: () => {},
})

export function NarrowRow<T>({ data, node, api }: ICellRendererParams<T>) {
  const { columns, onHighlight } = useContext(
    NarrowRowsContext
  ) as NarrowRows<T>
  const ref = useRef<HTMLDivElement>(null)

  useLayoutEffect(() => {
    const element = ref.current
    if (element === null) return
    function fit() {
      const height = element?.offsetHeight ?? 0
      if (height <= 0 || height === node.rowHeight || api.isDestroyed()) return
      node.setRowHeight(height)
      api.onRowHeightChanged()
    }
    fit()
    if (typeof ResizeObserver === "undefined") return
    const observer = new ResizeObserver(fit)
    observer.observe(element)
    return () => observer.disconnect()
  }, [api, node])

  if (data === undefined) return null
  const [title, ...rest] = columns
  if (title === undefined) return null
  const fields = rest.filter((column) => !column.action)
  const actions = rest.filter((column) => column.action)
  return (
    <div
      ref={ref}
      data-narrow-row=""
      className="flex flex-wrap items-center gap-x-3 gap-y-1 px-3.5 py-3 text-[13px] leading-normal"
      onPointerEnter={() => onHighlight(node.id ?? null)}
    >
      <div data-cell={title.id} className="min-w-0 basis-full font-medium">
        {title.cell(data)}
      </div>
      {fields.map((column) => (
        <div
          key={column.id}
          data-cell={column.id}
          data-label={column.header}
          className='flex min-w-0 items-center gap-1 text-xs text-muted-foreground before:content-[attr(data-label)_":"] empty:hidden'
        >
          {column.cell(data)}
        </div>
      ))}
      {actions.map((column) => (
        <div key={column.id} data-cell={column.id} className="ml-auto">
          {column.cell(data)}
        </div>
      ))}
    </div>
  )
}
