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

function holds(target: EventTarget | null, node: Node): boolean {
  return target instanceof Node && target.contains(node)
}

export function useSlotBox(
  node: HTMLElement | null,
  bar: HTMLElement | null,
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
      const barHeight = bar?.getBoundingClientRect().height ?? 0
      const hidden =
        edges !== undefined &&
        (edges.bottom - bottomInset - barHeight < 0 ||
          edges.top >= window.innerHeight)
      setBox((current) =>
        current.left === left &&
        current.width === width &&
        current.bottom === bottom &&
        current.hidden === hidden
          ? current
          : { left, width, bottom, hidden }
      )
    }
    let frame = 0
    const schedule = () => {
      if (frame !== 0) return
      frame = requestAnimationFrame(() => {
        frame = 0
        measure()
      })
    }
    const scrolled = (event: Event) => {
      if (holds(event.target, area ?? node)) schedule()
    }
    measure()
    const resize = new ResizeObserver(measure)
    resize.observe(node)
    if (area !== null) resize.observe(area)
    if (bar !== null) resize.observe(bar)
    resize.observe(document.documentElement)
    window.addEventListener("resize", schedule)
    window.addEventListener("scroll", scrolled, {
      capture: true,
      passive: true,
    })
    return () => {
      cancelAnimationFrame(frame)
      resize.disconnect()
      window.removeEventListener("resize", schedule)
      window.removeEventListener("scroll", scrolled, { capture: true })
    }
  }, [node, bar, bottomInset])

  return box
}
