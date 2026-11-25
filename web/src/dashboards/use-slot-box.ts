import { useLayoutEffect, useState } from "react"

export type SlotBox = { left: number; width: number }

const unmeasured: SlotBox = { left: 0, width: 0 }

export function useSlotBox(node: HTMLElement | null): SlotBox {
  const [box, setBox] = useState(unmeasured)

  useLayoutEffect(() => {
    if (node === null) return
    const measure = () => {
      const { left, width } = node.getBoundingClientRect()
      setBox((current) =>
        current.left === left && current.width === width
          ? current
          : { left, width }
      )
    }
    measure()
    const resize = new ResizeObserver(measure)
    resize.observe(node)
    resize.observe(document.documentElement)
    window.addEventListener("resize", measure)
    return () => {
      resize.disconnect()
      window.removeEventListener("resize", measure)
    }
  }, [node])

  return box
}
