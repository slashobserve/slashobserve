import type { GridApi, IRowNode } from "ag-grid-community"
import { useLayoutEffect, type RefObject } from "react"

export function useFitRowHeight(
  ref: RefObject<HTMLElement | null>,
  node: IRowNode,
  api: GridApi
) {
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
  }, [ref, api, node])
}
