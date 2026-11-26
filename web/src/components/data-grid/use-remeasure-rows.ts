import type { GridApi } from "ag-grid-community"
import { useEffect, type RefObject } from "react"

export function useRemeasureRows(
  ref: RefObject<HTMLElement | null>,
  apiRef: RefObject<Pick<GridApi, "resetRowHeights" | "isDestroyed"> | null>,
  active: boolean
) {
  useEffect(() => {
    const element = ref.current
    if (!active || element === null) return
    let frame = 0
    let width = element.getBoundingClientRect().width
    let stopped = false
    function remeasure() {
      cancelAnimationFrame(frame)
      frame = requestAnimationFrame(() => {
        const api = apiRef.current
        if (stopped || api === null || api.isDestroyed()) return
        api.resetRowHeights()
      })
    }
    const observer =
      typeof ResizeObserver === "undefined"
        ? null
        : new ResizeObserver(() => {
            const next = element?.getBoundingClientRect().width ?? 0
            if (next === width) return
            width = next
            remeasure()
          })
    observer?.observe(element)
    void document.fonts?.ready.then(remeasure)
    return () => {
      stopped = true
      cancelAnimationFrame(frame)
      observer?.disconnect()
    }
  }, [ref, apiRef, active])
}
