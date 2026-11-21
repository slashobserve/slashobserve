import { useLayoutEffect, useState, type RefObject } from "react"

import type { Size } from "@/panels/anchors"

export function useSize(ref: RefObject<HTMLElement | null>): Size {
  const [size, setSize] = useState<Size>({ width: 0, height: 0 })
  useLayoutEffect(() => {
    const element = ref.current
    if (element === null) return
    const measure = () =>
      setSize((current) =>
        current.width === element.offsetWidth &&
        current.height === element.offsetHeight
          ? current
          : { width: element.offsetWidth, height: element.offsetHeight }
      )
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(element)
    return () => observer.disconnect()
  }, [ref])
  return size
}
