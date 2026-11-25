import { useLayoutEffect, useState, type RefObject } from "react"

export const besideDetailMinWidth = 1001

function shellOf(element: HTMLElement): HTMLElement {
  return element.closest<HTMLElement>("[class~='@container']") ?? element
}

function contentWidth(element: HTMLElement): number {
  const style = getComputedStyle(element)
  return (
    element.clientWidth -
    parseFloat(style.paddingLeft || "0") -
    parseFloat(style.paddingRight || "0")
  )
}

export function useBesideDetail(ref: RefObject<HTMLElement | null>): boolean {
  const [beside, setBeside] = useState(false)
  useLayoutEffect(() => {
    const element = ref.current
    if (element === null) return
    const shell = shellOf(element)
    function measure() {
      setBeside(contentWidth(shell) >= besideDetailMinWidth)
    }
    measure()
    if (typeof ResizeObserver === "undefined") return
    const observer = new ResizeObserver(measure)
    observer.observe(shell)
    return () => observer.disconnect()
  }, [ref])
  return beside
}
