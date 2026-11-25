import { useLayoutEffect, useState, type RefObject } from "react"

export function useWidth(
  ref: RefObject<HTMLElement | null>,
  active: boolean
): number {
  const [width, setWidth] = useState(0)

  useLayoutEffect(() => {
    const element = ref.current
    if (!active || element === null) return
    function measure() {
      setWidth(element?.getBoundingClientRect().width ?? 0)
    }
    measure()
    if (typeof ResizeObserver === "undefined") return
    const observer = new ResizeObserver(measure)
    observer.observe(element)
    return () => observer.disconnect()
  }, [ref, active])

  return active ? width : 0
}
