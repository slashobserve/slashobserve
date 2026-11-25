import { useLayoutEffect, useState } from "react"

export type SlotBox = { left: number; width: number; clipped: boolean }

const unmeasured: SlotBox = { left: 0, width: 0, clipped: false }

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
  const [box, setBox] = useState(unmeasured)

  useLayoutEffect(() => {
    if (node === null) return
    const area = scroller(node)
    const measure = () => {
      const { left, width } = node.getBoundingClientRect()
      const clipped =
        area !== null &&
        area.getBoundingClientRect().bottom <
          window.innerHeight - bottomInset - 1
      setBox((current) =>
        current.left === left &&
        current.width === width &&
        current.clipped === clipped
          ? current
          : { left, width, clipped }
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
