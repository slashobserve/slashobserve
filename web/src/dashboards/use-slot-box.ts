import { useLayoutEffect, useState } from "react"

export type SlotBox = {
  left: number
  width: number
  bottom: number
  hidden: boolean
}

const scrolling = new Set(["auto", "scroll"])

function scroller(node: HTMLElement): HTMLElement | null {
  for (
    let parent = node.parentElement;
    parent !== null;
    parent = parent.parentElement
  ) {
    if (scrolling.has(getComputedStyle(parent).overflowY)) return parent
  }
  return null
}

export function useSlotBox(
  node: HTMLElement | null,
  bottomInset: number
): SlotBox {
  const [box, setBox] = useState<SlotBox>({
    left: 0,
    width: 0,
    bottom: bottomInset,
    hidden: false,
  })

  useLayoutEffect(() => {
    if (node === null) return
    const area = scroller(node)
    const measure = () => {
      const { left, width } = node.getBoundingClientRect()
      const edges = area?.getBoundingClientRect()
      const lift = edges ? Math.max(0, window.innerHeight - edges.bottom) : 0
      const bottom = bottomInset + lift
      const hidden =
        edges !== undefined &&
        (edges.bottom <= 0 || edges.top >= window.innerHeight)
      setBox((current) =>
        current.left === left &&
        current.width === width &&
        current.bottom === bottom &&
        current.hidden === hidden
          ? current
          : { left, width, bottom, hidden }
      )
    }
    measure()
    const resize = new ResizeObserver(measure)
    resize.observe(node)
    if (area !== null) resize.observe(area)
    resize.observe(document.documentElement)
    window.addEventListener("resize", measure)
    window.addEventListener("scroll", measure, { capture: true, passive: true })
    return () => {
      resize.disconnect()
      window.removeEventListener("resize", measure)
      window.removeEventListener("scroll", measure, { capture: true })
    }
  }, [node, bottomInset])

  return box
}
