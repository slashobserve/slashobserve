import { useLayoutEffect, useState, type RefObject } from "react"

import { fillHeight } from "./layout"

function scrollParent(element: HTMLElement): HTMLElement | null {
  for (
    let node = element.parentElement;
    node !== null;
    node = node.parentElement
  ) {
    if (/auto|scroll|overlay/.test(getComputedStyle(node).overflowY))
      return node
  }
  return null
}

function ancestorsUpTo(element: HTMLElement, last: HTMLElement | null) {
  const ancestors: HTMLElement[] = []
  for (
    let node = element.parentElement;
    node !== null;
    node = node.parentElement
  ) {
    ancestors.push(node)
    if (node === last) break
  }
  return ancestors
}

export function useFillHeight(
  ref: RefObject<HTMLElement | null>,
  active: boolean
): number | undefined {
  const [height, setHeight] = useState<number>()

  useLayoutEffect(() => {
    const element = ref.current
    if (!active || element === null) return
    const scroller = scrollParent(element)
    function fit() {
      if (element === null) return
      const top = element.getBoundingClientRect().top
      setHeight(
        scroller === null
          ? fillHeight(window.innerHeight, top + window.scrollY)
          : fillHeight(
              scroller.clientHeight,
              top - scroller.getBoundingClientRect().top + scroller.scrollTop
            )
      )
    }
    fit()
    window.addEventListener("resize", fit)
    const observer =
      typeof ResizeObserver === "undefined" ? null : new ResizeObserver(fit)
    for (const ancestor of ancestorsUpTo(element, scroller)) {
      observer?.observe(ancestor)
    }
    return () => {
      window.removeEventListener("resize", fit)
      observer?.disconnect()
    }
  }, [ref, active])

  return active ? height : undefined
}
