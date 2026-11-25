import type { ICellRendererParams } from "ag-grid-community"
import { createContext, useContext, useRef, type ReactNode } from "react"

import type { GridLine } from "./lines"
import { useFitRowHeight } from "./use-fit-row-height"

export const detailRowEstimate = 56

export const DetailRenderContext = createContext<
  ((row: unknown) => ReactNode) | undefined
>(undefined)

export function DetailRow<T>({
  data,
  node,
  api,
}: ICellRendererParams<GridLine<T>>) {
  const render = useContext(DetailRenderContext)
  const ref = useRef<HTMLDivElement>(null)
  useFitRowHeight(ref, node, api)
  if (data === undefined || render === undefined) return null
  return (
    <div
      ref={ref}
      data-detail-row=""
      className="px-3.5 py-2.5 text-[13px] leading-normal"
    >
      {render(data.row)}
    </div>
  )
}
