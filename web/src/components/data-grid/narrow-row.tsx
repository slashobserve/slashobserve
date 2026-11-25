import type { ICellRendererParams } from "ag-grid-community"
import { createContext, useContext, useRef, type ReactNode } from "react"

import type { DataGridColumn } from "./data-grid"
import { useFitRowHeight } from "./use-fit-row-height"
import type { GridLine } from "./lines"

export const narrowRowEstimate = 72

export type FullWidthRows<T> = {
  is: (row: T) => boolean
  render: (row: T) => ReactNode
}

export type NarrowRows<T> = {
  columns: readonly DataGridColumn<T>[]
  onHighlight: (rowId: string | null) => void
  fullWidth?: FullWidthRows<T>
}

export const NarrowRowsContext = createContext<NarrowRows<unknown>>({
  columns: [],
  onHighlight: () => {},
})

export function NarrowRow<T>({
  data,
  node,
  api,
}: ICellRendererParams<GridLine<T>>) {
  const { columns, onHighlight, fullWidth } = useContext(
    NarrowRowsContext
  ) as NarrowRows<T>
  const ref = useRef<HTMLDivElement>(null)
  useFitRowHeight(ref, node, api)

  if (data === undefined) return null
  if (fullWidth?.is(data.row)) {
    return (
      <div ref={ref} data-full-width-row="">
        {fullWidth.render(data.row)}
      </div>
    )
  }
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
        {title.cell(data.row)}
      </div>
      {fields.map((column) => (
        <div
          key={column.id}
          data-cell={column.id}
          data-label={column.header}
          className='flex min-w-0 items-center gap-1 text-xs text-muted-foreground before:content-[attr(data-label)_":"] empty:hidden'
        >
          {column.cell(data.row)}
        </div>
      ))}
      {actions.map((column) => (
        <div key={column.id} data-cell={column.id} className="ml-auto">
          {column.cell(data.row)}
        </div>
      ))}
    </div>
  )
}
