import type { GridApi } from "ag-grid-community"
import { useEffect, type RefObject } from "react"

const settleMs = 150

export function useRemeasureRows(
  ref: RefObject<HTMLElement | null>,
  apiRef: RefObject<Pick<GridApi, "resetRowHeights" | "isDestroyed"> | null>,
  active: boolean
) {
  useEffect(() => {
    const element = ref.current
    if (!active || element === null) return
    let timer: ReturnType<typeof setTimeout> | undefined
    let width = element.getBoundingClientRect().width
    let stopped = false
    function remeasure() {
      clearTimeout(timer)
      timer = setTimeout(() => {
        const api = apiRef.current
        if (stopped || api === null || api.isDestroyed()) return
        api.resetRowHeights()
      }, settleMs)
    }
    const observer =
      typeof ResizeObserver === "undefined"
        ? null
        : new ResizeObserver(() => {
            const next = element.getBoundingClientRect().width
            if (next === width) return
            width = next
            remeasure()
          })
    observer?.observe(element)
    void document.fonts?.ready.then(remeasure)
    return () => {
      stopped = true
      clearTimeout(timer)
      observer?.disconnect()
    }
  }, [ref, apiRef, active])
}
