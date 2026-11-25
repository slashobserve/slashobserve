import { useLayoutEffect, useState, type RefObject } from "react"

export const besideDetailMinWidth = 1001

function shellOf(element: HTMLElement): HTMLElement {
  return element.closest<HTMLElement>("[class~='@container']") ?? element
}

export function useBesideDetail(ref: RefObject<HTMLElement | null>): boolean {
  const [beside, setBeside] = useState(false)
  useLayoutEffect(() => {
    const element = ref.current
    if (element === null) return
    const shell = shellOf(element)
    function measure() {
      setBeside(shell.getBoundingClientRect().width >= besideDetailMinWidth)
    }
    measure()
    if (typeof ResizeObserver === "undefined") return
    const observer = new ResizeObserver(measure)
    observer.observe(shell)
    return () => observer.disconnect()
  }, [ref])
  return beside
}
